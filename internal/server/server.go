// Package server implements the HTTP API for v0.2 CLI Trader.
//
// Routes:
//
//	GET  /health          — liveness probe
//	POST /orders          — place a limit or market order
//	DELETE /orders/{id}   — cancel an order by ID
//	GET  /account         — return account state (balance, positions, open orders)
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
	"time"

	"github.com/sandswind/perpCS/internal/account"
	"github.com/sandswind/perpCS/internal/actor"
	"github.com/sandswind/perpCS/internal/types"
)

// Server wraps the HTTP handler for the trading API.
type Server struct {
	account *account.Account
	queue   chan *actor.UserOrder
	symbol  types.Symbol
}

// New creates a Server. queue must be the same channel passed to actor.Config.OrderQueue.
func New(acc *account.Account, queue chan *actor.UserOrder, symbol types.Symbol) *Server {
	return &Server{
		account: acc,
		queue:   queue,
		symbol:  symbol,
	}
}

// Handler returns an http.Handler with all routes registered.
// Uses Go 1.22+ ServeMux with method+path patterns.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /orders", s.handlePostOrder)
	mux.HandleFunc("DELETE /orders/{id}", s.handleDeleteOrder)
	mux.HandleFunc("GET /account", s.handleGetAccount)
	return mux
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

func (s *Server) handleGetAccount(w http.ResponseWriter, _ *http.Request) {
	if s.account == nil {
		writeError(w, http.StatusNotFound, "no account")
		return
	}
	resp := accountResponse{
		Balance:    s.account.Balance.String(),
		Positions:  make([]positionSummary, 0, len(s.account.Positions)),
		OpenOrders: make([]openOrderSummary, 0, len(s.account.OpenOrders)),
	}
	for _, pos := range s.account.Positions {
		resp.Positions = append(resp.Positions, positionSummary{
			Symbol:   string(pos.Symbol),
			Side:     pos.Side.String(),
			Size:     pos.Size.String(),
			AvgEntry: pos.AvgEntry.String(),
			Margin:   pos.Margin.String(),
			UPnL:     pos.UPnL.String(),
		})
	}
	for _, o := range s.account.OpenOrders {
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

// playerAddress returns the account owner string.
func (s *Server) playerAddress() string {
	if s.account != nil {
		return s.account.Address
	}
	return "player"
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
