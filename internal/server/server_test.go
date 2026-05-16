// Package server provides HTTP API tests using httptest.
// Tests use a mock actor queue (a goroutine that simulates actor responses)
// rather than a real actor, so tests run fast and deterministically.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sandswind/perpCS/internal/account"
	"github.com/sandswind/perpCS/internal/actor"
	"github.com/sandswind/perpCS/internal/types"
)

const testSymbol = types.Symbol("BTC-MED")
const testAddr = "test-player"

// mockActor runs a goroutine that responds to UserOrder requests.
// It simulates the actor: for market/limit orders it returns synthetic trades;
// for cancel requests it always returns success.
func mockActor(ctx context.Context, q chan *actor.UserOrder) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case uo, ok := <-q:
				if !ok {
					return
				}
				if uo.CancelOrderID != 0 {
					// Simulate successful cancel
					uo.ResultCh <- actor.UserOrderResult{}
					continue
				}
				// Simulate fills for market buy / sell
				var trades []types.Trade
				if uo.Order.Type == types.OrderTypeMarket {
					fillPrice := types.PriceFromFloat(7850.0)
					fillQty := uo.Order.Quantity
					uo.Order.ID = types.OrderID(1001)
					trades = []types.Trade{{
						ID:        1,
						Symbol:    testSymbol,
						Price:     fillPrice,
						Quantity:  fillQty,
						TakerID:   uo.Order.ID,
						TakerSide: uo.Order.Side,
					}}
				} else {
					// Limit order — just rest it (no fills)
					uo.Order.ID = types.OrderID(2001)
				}
				uo.ResultCh <- actor.UserOrderResult{Trades: trades}
			}
		}
	}()
}

// buildTestServer creates a Server with a mock actor queue and an account.
func buildTestServer(ctx context.Context, t *testing.T) (*Server, *account.Account, chan *actor.UserOrder) {
	t.Helper()
	acc := account.New(testAddr, "test-session", types.QtyFromFloat(10_000))
	q := make(chan *actor.UserOrder, 8)
	mockActor(ctx, q)
	srv := New(acc, q, testSymbol)
	return srv, acc, q
}

// TestHealth verifies GET /health returns {"ok":true}
func TestHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, _, _ := buildTestServer(ctx, t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]bool
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp["ok"] {
		t.Errorf("expected ok=true, got %v", resp)
	}
}

// TestPostOrder_MarketBuy verifies that a POST /orders market buy returns filled trades.
func TestPostOrder_MarketBuy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, acc, _ := buildTestServer(ctx, t)

	body := `{"side":"buy","type":"market","quantity":"0.1","price":"0"}`
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp orderResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Trades) == 0 {
		t.Error("expected fills for market buy")
	}
	t.Logf("order_id=%d trades=%+v", resp.OrderID, resp.Trades)

	// The mock actor applies fills automatically; account should be accessible
	_ = acc // account state is updated by the real actor; mock doesn't call ApplyFill
}

// TestGetAccount verifies GET /account returns balance and positions fields.
func TestGetAccount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, acc, _ := buildTestServer(ctx, t)

	// Manually populate a position to check serialization
	acc.Positions[testSymbol] = &account.Position{
		Symbol:   testSymbol,
		Side:     types.SideBuy,
		Size:     types.QtyFromFloat(0.1),
		AvgEntry: types.PriceFromFloat(7850),
		Margin:   types.QtyFromFloat(785),
		UPnL:     types.QtyFromFloat(5),
	}

	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp accountResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Balance == "" {
		t.Error("balance should not be empty")
	}
	if len(resp.Positions) == 0 {
		t.Error("expected 1 position")
	} else {
		pos := resp.Positions[0]
		if pos.Symbol != "BTC-MED" {
			t.Errorf("symbol: got %q want BTC-MED", pos.Symbol)
		}
		if pos.Side != "buy" {
			t.Errorf("side: got %q want buy", pos.Side)
		}
		t.Logf("position: %+v", pos)
	}
	t.Logf("balance=%s positions=%d", resp.Balance, len(resp.Positions))
}

// TestDeleteOrder verifies DELETE /orders/{id} returns {cancelled:true}.
func TestDeleteOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, _, _ := buildTestServer(ctx, t)

	// First post a limit order to get an order ID
	postBody := `{"side":"buy","type":"limit","quantity":"0.1","price":"7000"}`
	postReq := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(postBody))
	postReq.Header.Set("Content-Type", "application/json")
	postRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusOK {
		t.Fatalf("POST /orders expected 200, got %d: %s", postRR.Code, postRR.Body.String())
	}
	var postResp orderResponse
	if err := json.Unmarshal(postRR.Body.Bytes(), &postResp); err != nil {
		t.Fatalf("decode post: %v", err)
	}
	t.Logf("placed limit order id=%d", postResp.OrderID)

	// Give mock time to process the limit order
	time.Sleep(10 * time.Millisecond)

	// Now cancel it (mock always returns success)
	delReq := httptest.NewRequest(http.MethodDelete, "/orders/2001", nil)
	delRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delRR, delReq)

	if delRR.Code != http.StatusOK {
		t.Fatalf("DELETE /orders expected 200, got %d: %s", delRR.Code, delRR.Body.String())
	}
	var delResp map[string]bool
	if err := json.Unmarshal(delRR.Body.Bytes(), &delResp); err != nil {
		t.Fatalf("decode delete: %v", err)
	}
	if !delResp["cancelled"] {
		t.Errorf("expected cancelled=true, got %v", delResp)
	}
	t.Logf("cancel response: %v", delResp)
}
