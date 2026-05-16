// Package server implements the HTTP API for v0.2 CLI Trader and v0.3 Web Trader.
//
// Routes:
//
//	GET  /health                — liveness probe
//	POST /orders                — place a limit or market order
//	DELETE /orders/{id}         — cancel an order by ID
//	GET  /account               — return account state (balance, positions, open orders)
//	GET  /ws/market/{symbol}    — WebSocket: orderbook snapshots + trades
//	GET  /ws/account/{sid}      — WebSocket: fill + position updates for a session
//
// v0.6 routes:
//
//	POST /sessions/{addr}/close   — settle a session and compute final equity
//	GET  /sessions/{addr}/receipt — return the signed WithdrawReceipt
//	GET  /sessions/{addr}/report  — return the full debrief SessionReport
//
// Design rules:
//   - All communication with the MarketActor goes through the OrderQueue channel.
//   - No direct access to the OrderBook or Account — single-writer invariant preserved.
//   - Monetary values serialized as strings (types.Price.String() / types.Qty.String()).
//   - Timeout for order acknowledgement: 5 seconds.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sandswind/perpCS/internal/account"
	"github.com/sandswind/perpCS/internal/actor"
	"github.com/sandswind/perpCS/internal/fanout"
	"github.com/sandswind/perpCS/internal/reporting"
	"github.com/sandswind/perpCS/internal/session"
	"github.com/sandswind/perpCS/internal/types"
)

// reportingIface is the subset of reporting.Svc used by the HTTP layer.
// Defined as an interface so it can be nil-checked and mocked in tests.
type reportingIface interface {
	LookupReceipt(player string) *reporting.WithdrawReceipt
	LookupReport(player string) *reporting.SessionReport
	Settle(res actor.CloseSessionResult, player string, nonce uint64, archivePath string) (*reporting.WithdrawReceipt, error)
}

// Server wraps the HTTP handler for the trading API.
type Server struct {
	actor   *actor.Actor
	account *account.Account
	queue   chan *actor.UserOrder
	fanout  *fanout.Fanout
	symbol  types.Symbol
	// v0.5: optional on-chain session lookup. nil → /sessions endpoint
	// returns 503.
	sessionSvc *session.Svc
	// v0.5: when on-chain mode is enabled, the player address is taken from
	// the URL/query param of /account?address=0x... rather than the
	// bootstrap account, so we can serve multi-player.
	enableOnChain bool
	// v0.6: optional reporting service for receipts + debrief reports.
	reportingSvc reportingIface
}

// New creates a Server. queue must be the same channel passed to actor.Config.OrderQueue.
// fo may be nil (v0.2 compat); pass a *fanout.Fanout for WS support.
func New(acc *account.Account, queue chan *actor.UserOrder, symbol types.Symbol) *Server {
	return &Server{
		account: acc,
		queue:   queue,
		symbol:  symbol,
	}
}

// NewWithFanout creates a Server with WebSocket fanout support.
func NewWithFanout(acc *account.Account, queue chan *actor.UserOrder, symbol types.Symbol, fo *fanout.Fanout) *Server {
	return &Server{
		account: acc,
		queue:   queue,
		fanout:  fo,
		symbol:  symbol,
	}
}

// WithActor attaches the actor reference so the server can look up accounts
// by address (v0.5 multi-player path).
func (s *Server) WithActor(a *actor.Actor) *Server {
	s.actor = a
	return s
}

// WithSessions enables the /sessions/{addr} endpoint, backed by the v0.5
// session.Svc.
func (s *Server) WithSessions(svc *session.Svc) *Server {
	s.sessionSvc = svc
	s.enableOnChain = true
	return s
}

// WithReporting enables the v0.6 /sessions/{addr}/receipt and /report endpoints.
func (s *Server) WithReporting(svc *reporting.Svc) *Server {
	s.reportingSvc = svc
	return s
}

// corsMiddleware allows requests from the Next.js dev server on localhost:3000.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler returns an http.Handler with all routes registered.
// Uses Go 1.22+ ServeMux with method+path patterns.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /orders", s.handlePostOrder)
	mux.HandleFunc("DELETE /orders/{id}", s.handleDeleteOrder)
	mux.HandleFunc("GET /account", s.handleGetAccount)

	// v0.5: on-chain session lookup
	if s.sessionSvc != nil {
		mux.HandleFunc("GET /sessions/{addr}", s.handleGetSessionByAddress)
		mux.HandleFunc("GET /sessions/by-id/{sid}", s.handleGetSessionByID)
	}

	// v0.6: settlement, receipt, and debrief report
	if s.actor != nil {
		mux.HandleFunc("POST /sessions/{addr}/close", s.handleCloseSession)
		mux.HandleFunc("GET /sessions/{addr}/receipt", s.handleGetReceipt)
		mux.HandleFunc("GET /sessions/{addr}/report", s.handleGetReport)
	}

	// WebSocket routes (v0.3)
	if s.fanout != nil {
		mux.Handle("GET /ws/market/{symbol}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			symbol := r.PathValue("symbol")
			s.fanout.ServeMarket(symbol).ServeHTTP(w, r)
		}))
		mux.Handle("GET /ws/account/{sid}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sid := r.PathValue("sid")
			s.fanout.ServeAccount(sid).ServeHTTP(w, r)
		}))
	}

	return corsMiddleware(mux)
}

// ---- request / response types ----

type orderRequest struct {
	Side     string `json:"side"`
	Type     string `json:"type"`
	Quantity string `json:"quantity"`
	Price    string `json:"price"`
}

type tradeSummary struct {
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

type orderResponse struct {
	OrderID uint64         `json:"order_id"`
	Trades  []tradeSummary `json:"trades"`
}

type positionSummary struct {
	Symbol   string `json:"symbol"`
	Side     string `json:"side"`
	Size     string `json:"size"`
	AvgEntry string `json:"avg_entry"`
	Margin   string `json:"margin"`
	UPnL     string `json:"upnl"`
}

type openOrderSummary struct {
	ID       uint64 `json:"id"`
	Side     string `json:"side"`
	Type     string `json:"type"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
	Filled   string `json:"filled"`
}

type accountResponse struct {
	Balance    string             `json:"balance"`
	Positions  []positionSummary  `json:"positions"`
	OpenOrders []openOrderSummary `json:"open_orders"`
}

// ---- handlers ----

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handlePostOrder(w http.ResponseWriter, r *http.Request) {
	var req orderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	// Parse side
	var side types.Side
	switch req.Side {
	case "buy":
		side = types.SideBuy
	case "sell":
		side = types.SideSell
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid side %q: must be buy or sell", req.Side))
		return
	}

	// Parse order type
	var orderType types.OrderType
	switch req.Type {
	case "market":
		orderType = types.OrderTypeMarket
	case "limit":
		orderType = types.OrderTypeLimit
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid type %q: must be market or limit", req.Type))
		return
	}

	// Parse quantity
	qty, err := parseQty(req.Quantity)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid quantity: %v", err))
		return
	}

	// Parse price (required for limit orders)
	var price types.Price
	if orderType == types.OrderTypeLimit {
		price, err = parsePrice(req.Price)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid price: %v", err))
			return
		}
		if price <= 0 {
			writeError(w, http.StatusBadRequest, "limit order requires price > 0")
			return
		}
	}

	order := &types.Order{
		Symbol:   s.symbol,
		Side:     side,
		Type:     orderType,
		Price:    price,
		Quantity: qty,
		Source:   types.SourceUser,
		Owner:    s.playerAddress(),
	}

	// Submit to actor via channel with 5s timeout
	uo := &actor.UserOrder{
		Order:    order,
		ResultCh: make(chan actor.UserOrderResult, 1),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	select {
	case s.queue <- uo:
	case <-ctx.Done():
		writeError(w, http.StatusServiceUnavailable, "order queue full, actor busy")
		return
	}

	// Wait for result
	select {
	case res := <-uo.ResultCh:
		if res.Err != nil {
			writeError(w, http.StatusBadRequest, res.Err.Error())
			return
		}
		trades := make([]tradeSummary, len(res.Trades))
		for i, t := range res.Trades {
			trades[i] = tradeSummary{
				Price:    t.Price.String(),
				Quantity: t.Quantity.String(),
			}
		}
		writeJSON(w, http.StatusOK, orderResponse{
			OrderID: uint64(order.ID),
			Trades:  trades,
		})
	case <-ctx.Done():
		writeError(w, http.StatusGatewayTimeout, "timeout waiting for order result")
	}
}

func (s *Server) handleDeleteOrder(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid order id: %v", err))
		return
	}
	orderID := types.OrderID(idUint)

	// Send cancel request via the UserOrder queue (CancelOrderID set, Order nil)
	uo := &actor.UserOrder{
		CancelOrderID: orderID,
		ResultCh:      make(chan actor.UserOrderResult, 1),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	select {
	case s.queue <- uo:
	case <-ctx.Done():
		writeError(w, http.StatusServiceUnavailable, "order queue full")
		return
	}

	select {
	case res := <-uo.ResultCh:
		if res.Err != nil {
			writeError(w, http.StatusNotFound, res.Err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
	case <-ctx.Done():
		writeError(w, http.StatusGatewayTimeout, "timeout cancelling order")
	}
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	acc := s.resolveAccount(r)
	if acc == nil {
		writeError(w, http.StatusNotFound, "no account")
		return
	}
	resp := accountResponse{
		Balance:    acc.Balance.String(),
		Positions:  make([]positionSummary, 0, len(acc.Positions)),
		OpenOrders: make([]openOrderSummary, 0, len(acc.OpenOrders)),
	}
	for _, pos := range acc.Positions {
		resp.Positions = append(resp.Positions, positionSummary{
			Symbol:   string(pos.Symbol),
			Side:     pos.Side.String(),
			Size:     pos.Size.String(),
			AvgEntry: pos.AvgEntry.String(),
			Margin:   pos.Margin.String(),
			UPnL:     pos.UPnL.String(),
		})
	}
	for _, o := range acc.OpenOrders {
		resp.OpenOrders = append(resp.OpenOrders, openOrderSummary{
			ID:       uint64(o.ID),
			Side:     o.Side.String(),
			Type:     o.Type.String(),
			Price:    o.Price.String(),
			Quantity: o.Quantity.String(),
			Filled:   o.Filled.String(),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// resolveAccount picks the right account for the request:
//   - If ?address=0x... is set AND the actor has that address → that account.
//   - Otherwise fall back to the bootstrap s.account (v0.4 single-player demo).
func (s *Server) resolveAccount(r *http.Request) *account.Account {
	if s.actor != nil {
		if addr := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("address"))); addr != "" {
			if acc := s.actor.Account(addr); acc != nil {
				return acc
			}
			return nil
		}
	}
	return s.account
}

// playerAddress returns the account owner string for the bootstrap account
// (only used by /orders right now, where v0.4 demos remain single-player).
func (s *Server) playerAddress() string {
	if s.account != nil {
		return s.account.Address
	}
	return "player"
}

// handleGetSessionByAddress: GET /sessions/{addr}
//
// Returns 404 until the indexer has confirmed a deposit for {addr} AND the
// actor has created the corresponding vAccount. The frontend polls this
// endpoint after deposit() to know when to redirect the player to the
// trading view.
func (s *Server) handleGetSessionByAddress(w http.ResponseWriter, r *http.Request) {
	addr := strings.ToLower(r.PathValue("addr"))
	if addr == "" {
		writeError(w, http.StatusBadRequest, "missing address")
		return
	}
	rec := s.sessionSvc.LookupByPlayer(addr)
	if rec == nil {
		writeError(w, http.StatusNotFound, "no session for address")
		return
	}
	// Check that the actor goroutine has materialised the vAccount. If not,
	// the indexer has confirmed but the actor hasn't drained the queue yet —
	// return 202 so the FE knows to keep polling.
	ready := s.actor != nil && s.actor.HasAccount(addr)
	writeJSON(w, statusForReady(ready), sessionResponse{
		Ready:   ready,
		Session: rec,
	})
}

// handleGetSessionByID: GET /sessions/by-id/{sid}
func (s *Server) handleGetSessionByID(w http.ResponseWriter, r *http.Request) {
	sid := strings.ToLower(r.PathValue("sid"))
	if sid == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}
	rec := s.sessionSvc.LookupBySession(sid)
	if rec == nil {
		writeError(w, http.StatusNotFound, "no session with that id")
		return
	}
	ready := s.actor != nil && s.actor.HasAccount(rec.Player)
	writeJSON(w, statusForReady(ready), sessionResponse{
		Ready:   ready,
		Session: rec,
	})
}

// sessionResponse is the JSON shape returned by /sessions/{addr}.
type sessionResponse struct {
	Ready   bool            `json:"ready"`
	Session *session.Record `json:"session"`
}

func statusForReady(ready bool) int {
	if ready {
		return http.StatusOK
	}
	return http.StatusAccepted // 202 — confirmed on chain, vAccount still being created
}

// ---- v0.6 handlers ----

// handleCloseSession: POST /sessions/{addr}/close
//
// Triggers settlement of a player's session: forces all positions flat,
// computes final equity, signs a WithdrawReceipt.
//
// If the session is already closed the existing receipt is returned (idempotent).
func (s *Server) handleCloseSession(w http.ResponseWriter, r *http.Request) {
	addr := strings.ToLower(strings.TrimSpace(r.PathValue("addr")))
	if addr == "" {
		writeError(w, http.StatusBadRequest, "missing address")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	res, err := s.actor.CloseSession(ctx, addr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("close session: %v", err))
		return
	}

	// Settle via reporting service (produces signed receipt)
	var receipt interface{}
	if s.reportingSvc != nil {
		rec, settleErr := s.reportingSvc.Settle(res, addr, 0, "")
		if settleErr != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("settle: %v", settleErr))
			return
		}
		receipt = rec
	} else {
		// No reporting svc: return bare finalEquity
		receipt = map[string]interface{}{
			"session_id":       res.SessionID,
			"player":           addr,
			"final_equity":     res.FinalEquity.String(),
			"final_equity_raw": int64(res.FinalEquity),
		}
	}

	writeJSON(w, http.StatusOK, receipt)
}

// handleGetReceipt: GET /sessions/{addr}/receipt
//
// Returns the signed WithdrawReceipt for a settled session.
// 404 if the session hasn't been closed yet.
func (s *Server) handleGetReceipt(w http.ResponseWriter, r *http.Request) {
	addr := strings.ToLower(strings.TrimSpace(r.PathValue("addr")))
	if addr == "" {
		writeError(w, http.StatusBadRequest, "missing address")
		return
	}

	// Check if there's a closed session on the actor
	summary := s.actor.ClosedSession(addr)
	if summary == nil {
		writeError(w, http.StatusNotFound, "session not yet settled — POST /sessions/{addr}/close first")
		return
	}

	// Return the reporting receipt if available
	if s.reportingSvc != nil {
		rec := s.reportingSvc.LookupReceipt(addr)
		if rec != nil {
			writeJSON(w, http.StatusOK, rec)
			return
		}
		// Auto-settle if somehow the receipt is missing but summary exists
		closeRes := actor.CloseSessionResult{
			FinalEquity: summary.FinalEquity,
			SessionID:   summary.SessionID,
		}
		rec, err := s.reportingSvc.Settle(closeRes, addr, 0, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rec)
		return
	}

	// Bare receipt without reporting svc
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id":       summary.SessionID,
		"player":           addr,
		"final_equity":     summary.FinalEquity.String(),
		"final_equity_raw": int64(summary.FinalEquity),
		"signature":        "",
	})
}

// handleGetReport: GET /sessions/{addr}/report
//
// Returns the full SessionReport (PnL curve + debrief metrics).
// 404 if the session hasn't been settled.
func (s *Server) handleGetReport(w http.ResponseWriter, r *http.Request) {
	addr := strings.ToLower(strings.TrimSpace(r.PathValue("addr")))
	if addr == "" {
		writeError(w, http.StatusBadRequest, "missing address")
		return
	}

	summary := s.actor.ClosedSession(addr)
	if summary == nil {
		writeError(w, http.StatusNotFound, "session not yet settled")
		return
	}

	if s.reportingSvc != nil {
		rep := s.reportingSvc.LookupReport(addr)
		if rep != nil {
			writeJSON(w, http.StatusOK, rep)
			return
		}
	}

	// Fallback: minimal report from summary alone
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id":   summary.SessionID,
		"player":       addr,
		"final_equity": summary.FinalEquity.String(),
		"closed":       summary.Closed,
		"pnl_curve":    []interface{}{},
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// parsePrice parses a string to types.Price.
// Accepts both integer and decimal strings (e.g. "8000" or "8000.50").
func parsePrice(s string) (types.Price, error) {
	if s == "" || s == "0" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse price %q: %w", s, err)
	}
	return types.PriceFromFloat(f), nil
}

// parseQty parses a string to types.Qty.
func parseQty(s string) (types.Qty, error) {
	if s == "" {
		return 0, errors.New("quantity is required")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse quantity %q: %w", s, err)
	}
	if f <= 0 {
		return 0, errors.New("quantity must be > 0")
	}
	return types.QtyFromFloat(f), nil
}
