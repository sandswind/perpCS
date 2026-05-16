package orderbook

import (
	"testing"

	"github.com/sandswind/perpCS/internal/types"
)

const sym = types.Symbol("BTC-MED")

func newBuyLimit(id uint64, price, qty float64) *types.Order {
	return &types.Order{
		ID:       types.OrderID(id),
		Symbol:   sym,
		Side:     types.SideBuy,
		Type:     types.OrderTypeLimit,
		Price:    types.PriceFromFloat(price),
		Quantity: types.QtyFromFloat(qty),
		Source:   types.SourceReplay,
	}
}

func newSellLimit(id uint64, price, qty float64) *types.Order {
	return &types.Order{
		ID:       types.OrderID(id),
		Symbol:   sym,
		Side:     types.SideSell,
		Type:     types.OrderTypeLimit,
		Price:    types.PriceFromFloat(price),
		Quantity: types.QtyFromFloat(qty),
		Source:   types.SourceReplay,
	}
}

func newBuyMarket(id uint64, qty float64) *types.Order {
	return &types.Order{
		ID:       types.OrderID(id),
		Symbol:   sym,
		Side:     types.SideBuy,
		Type:     types.OrderTypeMarket,
		Quantity: types.QtyFromFloat(qty),
		Source:   types.SourceUser,
	}
}

func newSellMarket(id uint64, qty float64) *types.Order {
	return &types.Order{
		ID:       types.OrderID(id),
		Symbol:   sym,
		Side:     types.SideSell,
		Type:     types.OrderTypeMarket,
		Quantity: types.QtyFromFloat(qty),
		Source:   types.SourceUser,
	}
}

// mustAdd calls AddLimit and fatals on error.
func mustAdd(t *testing.T, b *Book, o *types.Order) {
	t.Helper()
	if err := b.AddLimit(o); err != nil {
		t.Fatalf("AddLimit(%d): %v", o.ID, err)
	}
}

// TestBestBidAsk verifies best bid/ask after adding orders.
func TestBestBidAsk(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newBuyLimit(1, 100, 1))
	mustAdd(t, b, newBuyLimit(2, 99, 1))
	mustAdd(t, b, newSellLimit(3, 101, 1))
	mustAdd(t, b, newSellLimit(4, 102, 1))

	if got := b.BestBid(); got != types.PriceFromFloat(100) {
		t.Errorf("BestBid: got %v want %v", got, types.PriceFromFloat(100))
	}
	if got := b.BestAsk(); got != types.PriceFromFloat(101) {
		t.Errorf("BestAsk: got %v want %v", got, types.PriceFromFloat(101))
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant: %v", err)
	}
}

// TestSortedLevels verifies bids descend, asks ascend.
func TestSortedLevels(t *testing.T) {
	b := New(sym)
	prices := []float64{103, 101, 105, 102, 104}
	for i, p := range prices {
		mustAdd(t, b, newBuyLimit(uint64(i+1), p, 1))
	}
	for i := 1; i < len(b.bidPrices); i++ {
		if b.bidPrices[i] >= b.bidPrices[i-1] {
			t.Errorf("bids not descending at index %d", i)
		}
	}
	asks := []float64{203, 201, 205, 202, 204}
	for i, p := range asks {
		mustAdd(t, b, newSellLimit(uint64(100+i), p, 1))
	}
	for i := 1; i < len(b.askPrices); i++ {
		if b.askPrices[i] <= b.askPrices[i-1] {
			t.Errorf("asks not ascending at index %d", i)
		}
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant: %v", err)
	}
}

// TestCancel verifies cancel removes the order and leaves the book consistent.
func TestCancel(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newBuyLimit(1, 100, 1))
	mustAdd(t, b, newBuyLimit(2, 100, 2)) // same price, FIFO
	mustAdd(t, b, newSellLimit(3, 101, 1))

	if err := b.Cancel(2); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	lv := b.bidLevels[types.PriceFromFloat(100)]
	if lv == nil {
		t.Fatal("level should still exist after partial cancel")
	}
	if lv.count != 1 {
		t.Errorf("level count: got %d want 1", lv.count)
	}
	if got := lv.total; got != types.QtyFromFloat(1) {
		t.Errorf("level total: got %v want 1", got)
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant: %v", err)
	}

	// Cancel the last order — level should disappear
	if err := b.Cancel(1); err != nil {
		t.Fatalf("cancel last: %v", err)
	}
	if _, ok := b.bidLevels[types.PriceFromFloat(100)]; ok {
		t.Error("level should be removed when empty")
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant after remove: %v", err)
	}
}

// TestCancelNotFound returns the expected error.
func TestCancelNotFound(t *testing.T) {
	b := New(sym)
	if err := b.Cancel(999); err != ErrOrderNotFound {
		t.Errorf("expected ErrOrderNotFound, got %v", err)
	}
}

// TestCancelIdempotent verifies double-cancel returns an error (not a panic).
func TestCancelIdempotent(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newBuyLimit(1, 100, 1))
	if err := b.Cancel(1); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	if err := b.Cancel(1); err != ErrOrderNotFound {
		t.Errorf("second cancel: expected ErrOrderNotFound, got %v", err)
	}
}

// TestMarketOrderFullFill verifies a market order consumes liquidity correctly.
func TestMarketOrderFullFill(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newSellLimit(1, 100, 1))
	mustAdd(t, b, newSellLimit(2, 101, 2))

	taker := newBuyMarket(10, 1)
	trades, err := b.MatchMarket(taker, 1000)
	if err != nil {
		t.Fatalf("match market: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	tr := trades[0]
	if tr.Price != types.PriceFromFloat(100) {
		t.Errorf("fill price: got %v want 100", tr.Price)
	}
	if tr.Quantity != types.QtyFromFloat(1) {
		t.Errorf("fill qty: got %v want 1", tr.Quantity)
	}
	if !taker.IsFullyFilled() {
		t.Error("taker should be fully filled")
	}
	// Level 100 should be gone, level 101 should remain
	if b.BestAsk() != types.PriceFromFloat(101) {
		t.Errorf("best ask after fill: got %v want 101", b.BestAsk())
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant: %v", err)
	}
}

// TestMarketOrderPartialFill verifies partial fill when book is thin.
func TestMarketOrderPartialFill(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newSellLimit(1, 100, 0.5))

	taker := newBuyMarket(10, 1) // wants 1 but only 0.5 available
	trades, err := b.MatchMarket(taker, 1000)
	if err != nil {
		t.Fatalf("match market: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if taker.Filled != types.QtyFromFloat(0.5) {
		t.Errorf("taker filled: got %v want 0.5", taker.Filled)
	}
	if taker.Remaining() != types.QtyFromFloat(0.5) {
		t.Errorf("taker remaining: got %v want 0.5", taker.Remaining())
	}
	if b.BestAsk() != 0 {
		t.Errorf("book should be empty, got ask %v", b.BestAsk())
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant: %v", err)
	}
}

// TestMarketOrderMultiLevel verifies a market order walks multiple price levels.
func TestMarketOrderMultiLevel(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newSellLimit(1, 100, 0.5))
	mustAdd(t, b, newSellLimit(2, 101, 0.5))
	mustAdd(t, b, newSellLimit(3, 102, 0.5))

	taker := newBuyMarket(10, 1.2)
	trades, err := b.MatchMarket(taker, 1000)
	if err != nil {
		t.Fatalf("match market: %v", err)
	}
	// Should fill 0.5@100, 0.5@101, 0.2@102
	if len(trades) != 3 {
		t.Fatalf("expected 3 trades, got %d", len(trades))
	}
	wantPrices := []float64{100, 101, 102}
	wantQtys := []float64{0.5, 0.5, 0.2}
	for i, tr := range trades {
		wantP := types.PriceFromFloat(wantPrices[i])
		wantQ := types.QtyFromFloat(wantQtys[i])
		if tr.Price != wantP {
			t.Errorf("trade %d price: got %v want %v", i, tr.Price, wantP)
		}
		if tr.Quantity != wantQ {
			t.Errorf("trade %d qty: got %v want %v", i, tr.Quantity, wantQ)
		}
	}
	if !taker.IsFullyFilled() {
		t.Error("taker should be fully filled")
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant: %v", err)
	}
}

// TestSellMarketOrder verifies sell market eats bids.
func TestSellMarketOrder(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newBuyLimit(1, 100, 1))
	mustAdd(t, b, newBuyLimit(2, 99, 1))

	taker := newSellMarket(10, 1.5)
	trades, err := b.MatchMarket(taker, 1000)
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}
	if trades[0].Price != types.PriceFromFloat(100) {
		t.Errorf("first trade should be at best bid 100, got %v", trades[0].Price)
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant: %v", err)
	}
}

// TestLimitOrderRested verifies a limit order that doesn't cross is rested.
func TestLimitOrderRested(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newSellLimit(1, 101, 1))

	buyLimitLow := &types.Order{
		ID: 10, Symbol: sym, Side: types.SideBuy, Type: types.OrderTypeLimit,
		Price: types.PriceFromFloat(100), Quantity: types.QtyFromFloat(1),
		Source: types.SourceUser,
	}
	trades, err := b.MatchLimit(buyLimitLow, 1000)
	if err != nil {
		t.Fatalf("MatchLimit: %v", err)
	}
	if len(trades) != 0 {
		t.Errorf("expected no trades for non-crossing limit, got %d", len(trades))
	}
	// Order should be rested
	if b.BestBid() != types.PriceFromFloat(100) {
		t.Errorf("rested order should set best bid to 100, got %v", b.BestBid())
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant: %v", err)
	}
}

// TestLimitOrderCrossing verifies a limit order that crosses is matched.
func TestLimitOrderCrossing(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newSellLimit(1, 100, 1))

	buyLimitHigh := &types.Order{
		ID: 10, Symbol: sym, Side: types.SideBuy, Type: types.OrderTypeLimit,
		Price: types.PriceFromFloat(101), Quantity: types.QtyFromFloat(1),
		Source: types.SourceUser,
	}
	trades, err := b.MatchLimit(buyLimitHigh, 1000)
	if err != nil {
		t.Fatalf("MatchLimit: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	// Should execute at maker price (100), not taker limit (101)
	if trades[0].Price != types.PriceFromFloat(100) {
		t.Errorf("fill price: got %v want 100", trades[0].Price)
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant: %v", err)
	}
}

// TestFIFOPriority verifies orders at the same price level are filled FIFO.
func TestFIFOPriority(t *testing.T) {
	b := New(sym)
	// Three sell orders at the same price, placed in order 1, 2, 3
	mustAdd(t, b, newSellLimit(1, 100, 0.5))
	mustAdd(t, b, newSellLimit(2, 100, 0.5))
	mustAdd(t, b, newSellLimit(3, 100, 0.5))

	taker := newBuyMarket(10, 1.0) // should fill orders 1 and 2 (oldest first)
	trades, err := b.MatchMarket(taker, 1000)
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}
	if trades[0].MakerID != 1 {
		t.Errorf("first fill should be order 1 (FIFO), got %d", trades[0].MakerID)
	}
	if trades[1].MakerID != 2 {
		t.Errorf("second fill should be order 2 (FIFO), got %d", trades[1].MakerID)
	}
	// Order 3 should still be resting
	if b.BestAsk() != types.PriceFromFloat(100) {
		t.Errorf("order 3 should remain, best ask should be 100")
	}
	if err := b.Validate(); err != nil {
		t.Errorf("invariant: %v", err)
	}
}

// TestSnapshot verifies the L2 snapshot output.
func TestSnapshot(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newBuyLimit(1, 100, 2))
	mustAdd(t, b, newBuyLimit(2, 99, 1))
	mustAdd(t, b, newSellLimit(3, 101, 3))

	snap := b.Snapshot(12345, 10)
	if len(snap.Bids) != 2 {
		t.Errorf("bids: got %d want 2", len(snap.Bids))
	}
	if len(snap.Asks) != 1 {
		t.Errorf("asks: got %d want 1", len(snap.Asks))
	}
	if snap.Bids[0].Price != types.PriceFromFloat(100) {
		t.Errorf("best bid price: got %v want 100", snap.Bids[0].Price)
	}
	if snap.Asks[0].Quantity != types.QtyFromFloat(3) {
		t.Errorf("ask qty: got %v want 3", snap.Asks[0].Quantity)
	}
}

// TestDuplicateOrderID verifies that adding a duplicate ID returns an error.
func TestDuplicateOrderID(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newBuyLimit(1, 100, 1))
	if err := b.AddLimit(newBuyLimit(1, 100, 1)); err == nil {
		t.Error("expected error for duplicate order ID")
	}
}

// TestEmptyBook verifies zero values on an empty book.
func TestEmptyBook(t *testing.T) {
	b := New(sym)
	if b.BestBid() != 0 {
		t.Errorf("empty bid: got %v want 0", b.BestBid())
	}
	if b.BestAsk() != 0 {
		t.Errorf("empty ask: got %v want 0", b.BestAsk())
	}
	snap := b.Snapshot(0, 10)
	if len(snap.Bids) != 0 || len(snap.Asks) != 0 {
		t.Error("snapshot of empty book should have no levels")
	}
	if err := b.Validate(); err != nil {
		t.Errorf("empty book invariant: %v", err)
	}
}

// TestTotalStats verifies TotalTrades and TotalVolume accumulate.
func TestTotalStats(t *testing.T) {
	b := New(sym)
	mustAdd(t, b, newSellLimit(1, 100, 2))
	taker := newBuyMarket(10, 2)
	trades, err := b.MatchMarket(taker, 1000)
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if b.TotalTrades != 1 {
		t.Errorf("TotalTrades: got %d want 1", b.TotalTrades)
	}
	if b.TotalVolume != types.QtyFromFloat(2) {
		t.Errorf("TotalVolume: got %v want 2", b.TotalVolume)
	}
}

// ---- Benchmarks ----

// BenchmarkAddLimit measures throughput of resting limit orders.
func BenchmarkAddLimit(bm *testing.B) {
	b := New(sym)
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		o := &types.Order{
			ID:       types.OrderID(i + 1),
			Symbol:   sym,
			Side:     types.SideBuy,
			Type:     types.OrderTypeLimit,
			Price:    types.PriceFromFloat(float64(100 + i%20)), // 20 distinct price levels
			Quantity: types.QtyFromFloat(1),
			Source:   types.SourceReplay,
		}
		_ = b.AddLimit(o)
	}
}

// BenchmarkMatchMarket measures throughput of market order matching.
func BenchmarkMatchMarket(bm *testing.B) {
	// Pre-populate asks at 100 levels
	b := New(sym)
	for i := 0; i < 1000; i++ {
		o := &types.Order{
			ID:       types.OrderID(i + 1),
			Symbol:   sym,
			Side:     types.SideSell,
			Type:     types.OrderTypeLimit,
			Price:    types.PriceFromFloat(float64(100 + i)),
			Quantity: types.QtyFromFloat(1),
			Source:   types.SourceReplay,
		}
		_ = b.AddLimit(o)
	}

	bm.ResetTimer()
	// Each iteration: buy 1 unit (fills 1 order at best ask), then re-add that ask.
	sellerID := types.OrderID(100_000)
	ts := int64(0)
	for i := 0; i < bm.N; i++ {
		taker := &types.Order{
			ID:       types.OrderID(200_000 + uint64(i)),
			Symbol:   sym,
			Side:     types.SideBuy,
			Type:     types.OrderTypeMarket,
			Quantity: types.QtyFromFloat(1),
			Source:   types.SourceUser,
		}
		trades, _ := b.MatchMarket(taker, ts)
		ts++
		// Re-add consumed liquidity so the bench doesn't drain the book
		if len(trades) > 0 {
			sellerID++
			refill := &types.Order{
				ID:       sellerID,
				Symbol:   sym,
				Side:     types.SideSell,
				Type:     types.OrderTypeLimit,
				Price:    trades[0].Price,
				Quantity: trades[0].Quantity,
				Source:   types.SourceReplay,
			}
			_ = b.AddLimit(refill)
		}
	}
}
