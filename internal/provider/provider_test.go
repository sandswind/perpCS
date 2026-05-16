package provider

import (
	"context"
	"testing"
	"time"

	"github.com/sandswind/perpCS/internal/types"
)

var (
	testFrom = time.Date(2020, 3, 12, 0, 0, 0, 0, time.UTC)
	testTo   = testFrom.Add(2 * time.Hour) // 2h window
)

// TestMockKlines verifies the mock provider generates valid klines.
func TestMockKlines(t *testing.T) {
	m := DefaultMock()
	klines, err := m.FetchKlines(context.Background(), "BTC-MED", testFrom, testTo, "1m")
	if err != nil {
		t.Fatalf("FetchKlines: %v", err)
	}
	if len(klines) != 120 { // 2h × 60m
		t.Errorf("expected 120 klines, got %d", len(klines))
	}
	// Price should be descending (crash simulation)
	for i := 1; i < len(klines); i++ {
		if klines[i].Close > klines[i-1].Close {
			t.Errorf("price not descending at index %d: %v > %v",
				i, klines[i].Close, klines[i-1].Close)
		}
	}
	// First kline open should match start price
	if klines[0].Open != types.PriceFromFloat(m.StartPrice) {
		t.Errorf("first open: got %v want %v", klines[0].Open, types.PriceFromFloat(m.StartPrice))
	}
	// Timestamps should be monotonically increasing and non-zero
	for i, k := range klines {
		if k.OpenTS == 0 {
			t.Errorf("kline %d has zero OpenTS", i)
		}
		if i > 0 && klines[i].OpenTS <= klines[i-1].OpenTS {
			t.Errorf("klines not sorted by time at index %d", i)
		}
	}
}

// TestMockAggTrades verifies the mock provider generates sorted aggTrades.
func TestMockAggTrades(t *testing.T) {
	m := DefaultMock()
	trades, err := m.FetchAggTrades(context.Background(), "BTC-MED", testFrom, testTo)
	if err != nil {
		t.Fatalf("FetchAggTrades: %v", err)
	}
	if len(trades) == 0 {
		t.Fatal("expected trades, got none")
	}
	// Must be sorted ascending by TS
	for i := 1; i < len(trades); i++ {
		if trades[i].TS < trades[i-1].TS {
			t.Errorf("trades not sorted at index %d", i)
		}
	}
	// Prices must be positive
	for i, tr := range trades {
		if tr.Price <= 0 {
			t.Errorf("trade %d has non-positive price %v", i, tr.Price)
		}
		if tr.Quantity <= 0 {
			t.Errorf("trade %d has non-positive qty %v", i, tr.Quantity)
		}
		if tr.TakerSide != types.SideBuy && tr.TakerSide != types.SideSell {
			t.Errorf("trade %d has unknown side %v", i, tr.TakerSide)
		}
	}
}

// TestMockFundingRates verifies funding rate generation.
func TestMockFundingRates(t *testing.T) {
	m := DefaultMock()
	from := time.Date(2020, 3, 12, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	pts, err := m.FetchFundingRates(context.Background(), "BTC-MED", from, to)
	if err != nil {
		t.Fatalf("FetchFundingRates: %v", err)
	}
	if len(pts) != 3 { // 0h, 8h, 16h — 24h is exclusive
		t.Errorf("expected 3 funding points, got %d", len(pts))
	}
	for _, p := range pts {
		if p.Rate >= 0 {
			t.Errorf("expected negative rate (bearish), got %v", p.Rate)
		}
	}
}

// TestMockImplementsInterface verifies Mock satisfies IDataProvider at compile time.
func TestMockImplementsInterface(t *testing.T) {
	var _ IDataProvider = (*Mock)(nil)
	var _ IDataProvider = (*Binance)(nil)
}

// TestSynthOrderBook verifies the synthetic depth generator.
func TestSynthOrderBook(t *testing.T) {
	cfg := DefaultSynthConfig()
	mid := types.PriceFromFloat(7000)
	snap := SynthOrderBook("BTC-MED", mid, 12345, cfg)

	if len(snap.Bids) != cfg.NumLevels {
		t.Errorf("bids: got %d want %d", len(snap.Bids), cfg.NumLevels)
	}
	if len(snap.Asks) != cfg.NumLevels {
		t.Errorf("asks: got %d want %d", len(snap.Asks), cfg.NumLevels)
	}
	// Bids should be below mid, asks above mid
	for i, b := range snap.Bids {
		if b.Price >= mid {
			t.Errorf("bid %d price %v >= mid %v", i, b.Price, mid)
		}
		if b.Quantity <= 0 {
			t.Errorf("bid %d has non-positive qty", i)
		}
	}
	for i, a := range snap.Asks {
		if a.Price <= mid {
			t.Errorf("ask %d price %v <= mid %v", i, a.Price, mid)
		}
		if a.Quantity <= 0 {
			t.Errorf("ask %d has non-positive qty", i)
		}
	}
	// Power-law: best level should have more qty than worst
	if snap.Bids[0].Quantity <= snap.Bids[cfg.NumLevels-1].Quantity {
		t.Errorf("power-law violated: best bid qty %v <= worst bid qty %v",
			snap.Bids[0].Quantity, snap.Bids[cfg.NumLevels-1].Quantity)
	}
}

// TestKlinesToSynthOrders verifies the order generation pipeline.
func TestKlinesToSynthOrders(t *testing.T) {
	m := DefaultMock()
	klines, _ := m.FetchKlines(context.Background(), "BTC-MED", testFrom, testTo, "1m")
	trades, _ := m.FetchAggTrades(context.Background(), "BTC-MED", testFrom, testTo)

	cfg := DefaultSynthConfig()
	orders := KlinesToSynthOrders("BTC-MED", klines, trades, cfg, 0)

	if len(orders) == 0 {
		t.Fatal("expected orders, got none")
	}
	// Every order must be Source=Replay, positive qty, valid type
	buys, sells, markets := 0, 0, 0
	for _, o := range orders {
		if o.Source != types.SourceReplay {
			t.Errorf("order %d: expected SourceReplay, got %v", o.ID, o.Source)
		}
		if o.Quantity <= 0 {
			t.Errorf("order %d: non-positive qty", o.ID)
		}
		if o.Type == types.OrderTypeLimit {
			if o.Price <= 0 {
				t.Errorf("limit order %d: non-positive price", o.ID)
			}
		}
		if o.Side == types.SideBuy {
			buys++
		} else {
			sells++
		}
		if o.Type == types.OrderTypeMarket {
			markets++
		}
	}
	if buys == 0 {
		t.Error("no buy orders generated")
	}
	if sells == 0 {
		t.Error("no sell orders generated")
	}
	if markets == 0 {
		t.Error("no market (trade) orders generated")
	}
	t.Logf("generated %d orders (%d buy, %d sell, %d market) from %d klines + %d trades",
		len(orders), buys, sells, markets, len(klines), len(trades))
}

// TestSynthOrders_IDsUnique verifies no duplicate order IDs.
func TestSynthOrders_IDsUnique(t *testing.T) {
	m := DefaultMock()
	klines, _ := m.FetchKlines(context.Background(), "BTC-MED", testFrom, testTo, "1m")
	trades, _ := m.FetchAggTrades(context.Background(), "BTC-MED", testFrom, testTo)
	orders := KlinesToSynthOrders("BTC-MED", klines, trades, DefaultSynthConfig(), 0)

	seen := make(map[types.OrderID]bool, len(orders))
	for _, o := range orders {
		if seen[o.ID] {
			t.Errorf("duplicate order ID %d", o.ID)
		}
		seen[o.ID] = true
	}
}
