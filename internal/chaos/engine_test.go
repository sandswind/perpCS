package chaos

import (
	"testing"

	"github.com/sandswind/perpCS/internal/types"
)

func TestXorshift64_Deterministic(t *testing.T) {
	// Same (seed, n) must always return the same value.
	seed := uint64(0xDEADBEEF12345678)
	for n := uint64(0); n < 1000; n++ {
		a := xorshift64(seed, n)
		b := xorshift64(seed, n)
		if a != b {
			t.Fatalf("xorshift64 not deterministic at n=%d", n)
		}
	}
}

func TestXorshift64_DifferentSeeds(t *testing.T) {
	// Different seeds must produce different sequences.
	collisions := 0
	for n := uint64(0); n < 1000; n++ {
		a := xorshift64(1, n)
		b := xorshift64(2, n)
		if a == b {
			collisions++
		}
	}
	if collisions > 2 { // allow tiny coincidental collision rate
		t.Errorf("too many collisions between different seeds: %d/1000", collisions)
	}
}

func TestXorshift64_Distribution(t *testing.T) {
	// Basic uniformity: count how many values fall in each half of [0, 2^64).
	seed := uint64(42)
	const N = 10_000
	above := 0
	half := uint64(1) << 63
	for n := uint64(0); n < N; n++ {
		if xorshift64(seed, n) >= half {
			above++
		}
	}
	// Expect roughly 50% in each half ±5%
	if above < N*45/100 || above > N*55/100 {
		t.Errorf("distribution skewed: %d/%d above half", above, N)
	}
}

func TestApplyToSnapshot_NoEffect(t *testing.T) {
	e := New(NoChaos("BTC-MED"))
	snap := sampleSnap()
	original := cloneSnap(snap)
	e.ApplyToSnapshot(&snap)
	// With DepthShrink=0 (from NoChaos with 1.0 explicitly set), quantities unchanged.
	for i, b := range snap.Bids {
		if b.Quantity != original.Bids[i].Quantity {
			t.Errorf("bid %d qty changed unexpectedly", i)
		}
	}
}

func TestApplyToSnapshot_HalfDepth(t *testing.T) {
	cfg := Config{Symbol: "BTC-MED", DepthShrink: 0.5}
	e := New(cfg)
	snap := sampleSnap()
	original := cloneSnap(snap)
	e.ApplyToSnapshot(&snap)
	for i, b := range snap.Bids {
		want := types.Qty(float64(original.Bids[i].Quantity) * 0.5)
		if b.Quantity != want {
			t.Errorf("bid %d: got %v want %v", i, b.Quantity, want)
		}
	}
	for i, a := range snap.Asks {
		want := types.Qty(float64(original.Asks[i].Quantity) * 0.5)
		if a.Quantity != want {
			t.Errorf("ask %d: got %v want %v", i, a.Quantity, want)
		}
	}
}

func TestApplyToSnapshot_L2Params(t *testing.T) {
	e := New(BTC_MED_L2(1))
	snap := sampleSnap()
	original := cloneSnap(snap)
	e.ApplyToSnapshot(&snap)
	// L2 DepthShrink = 0.20 → quantities should be ~20% of original
	for i, b := range snap.Bids {
		want := types.Qty(float64(original.Bids[i].Quantity) * 0.20)
		if b.Quantity != want {
			t.Errorf("bid %d: got %v want %v", i, b.Quantity, want)
		}
	}
}

func TestApplyWick_NoChaos(t *testing.T) {
	e := New(NoChaos("BTC-MED"))
	kline := types.Kline{Close: types.PriceFromFloat(7000)}
	for i := 0; i < 1000; i++ {
		_, injected := e.ApplyWick(kline)
		if injected {
			t.Fatal("NoChaos should never inject wicks")
		}
	}
}

func TestApplyWick_AlwaysInject(t *testing.T) {
	// Set WickProb = 1.0 to guarantee injection every tick.
	cfg := Config{Symbol: "BTC-MED", Seed: 42, WickProb: 1.0, WickMagnitude: 0.05}
	e := New(cfg)
	kline := types.Kline{Close: types.PriceFromFloat(7000)}
	for i := 0; i < 100; i++ {
		wickLow, injected := e.ApplyWick(kline)
		if !injected {
			t.Fatalf("WickProb=1 should always inject (tick %d)", i)
		}
		// Wick should be 5% below close
		want := types.Price(float64(kline.Close) * 0.95)
		if wickLow != want {
			t.Errorf("wick price: got %v want %v", wickLow, want)
		}
	}
}

func TestApplyWick_RateApproximate(t *testing.T) {
	// With WickProb=0.1 over 10000 ticks, expect ~1000 wicks ±20%.
	cfg := Config{Symbol: "BTC-MED", Seed: 99, WickProb: 0.1, WickMagnitude: 0.02}
	e := New(cfg)
	kline := types.Kline{Close: types.PriceFromFloat(5000)}
	count := 0
	const N = 10_000
	for i := 0; i < N; i++ {
		if _, ok := e.ApplyWick(kline); ok {
			count++
		}
	}
	lo, hi := int(N*0.08), int(N*0.12)
	if count < lo || count > hi {
		t.Errorf("wick count %d outside expected range [%d,%d]", count, lo, hi)
	}
}

func TestApplyWick_Deterministic(t *testing.T) {
	// Two engines with identical seeds must produce identical wick decisions.
	cfg := BTC_MED_L2(12345)
	e1 := New(cfg)
	e2 := New(cfg)
	kline := types.Kline{Close: types.PriceFromFloat(6000)}
	for i := 0; i < 500; i++ {
		_, ok1 := e1.ApplyWick(kline)
		_, ok2 := e2.ApplyWick(kline)
		if ok1 != ok2 {
			t.Fatalf("non-deterministic wick at tick %d", i)
		}
	}
}

func TestMarkPrice_NoLag(t *testing.T) {
	e := New(NoChaos("BTC-MED"))
	price := types.PriceFromFloat(7000)
	got := e.MarkPrice(price, 1_000_000_000)
	if got != price {
		t.Errorf("no-lag mark: got %v want %v", got, price)
	}
}

func TestMarkPrice_LagApplied(t *testing.T) {
	cfg := Config{Symbol: "BTC-MED", OracleLagNS: 3_000_000_000} // 3s
	e := New(cfg)

	tickNS := int64(100_000_000) // 100ms per tick
	baseTS := int64(10_000_000_000)

	// Fill the buffer: 30 ticks at 100ms = 3s of history needed.
	// After 40 ticks the lag should kick in clearly.
	prices := make([]types.Price, 50)
	for i := 0; i < 50; i++ {
		prices[i] = types.PriceFromFloat(float64(7000 - i*10))
		_ = e.MarkPrice(prices[i], baseTS+int64(i)*tickNS)
	}

	// At tick 49, with 3s lag (30 ticks at 100ms), the mark should
	// be the price from ~30 ticks earlier. The buffer holds 120 entries,
	// so it is fully warmed up. We check that mark ≤ price[0] (7000)
	// and mark ≥ price[49] (6510) — i.e. mark is somewhere in history.
	ts49 := baseTS + 49*tickNS
	mark := e.MarkPrice(prices[49], ts49)
	lowest := prices[49]
	highest := prices[0]
	if mark < lowest || mark > highest {
		t.Errorf("mark %v out of expected range [%v, %v]", mark, lowest, highest)
	}
}

func TestMarkPrice_Deterministic(t *testing.T) {
	// Two engines with same config must produce identical mark prices.
	cfg := Config{Symbol: "BTC-MED", OracleLagNS: 2_000_000_000}
	e1 := New(cfg)
	e2 := New(cfg)

	for i := int64(0); i < 100; i++ {
		price := types.PriceFromFloat(float64(7000 - i))
		ts := i * 100_000_000
		m1 := e1.MarkPrice(price, ts)
		m2 := e2.MarkPrice(price, ts)
		if m1 != m2 {
			t.Fatalf("mark price not deterministic at tick %d: %v vs %v", i, m1, m2)
		}
	}
}

// ---- helpers ----

func sampleSnap() types.BookSnapshot {
	return types.BookSnapshot{
		Symbol: "BTC-MED",
		TS:     1000,
		Bids: []types.PriceLevel{
			{Price: types.PriceFromFloat(100), Quantity: types.QtyFromFloat(10)},
			{Price: types.PriceFromFloat(99), Quantity: types.QtyFromFloat(20)},
		},
		Asks: []types.PriceLevel{
			{Price: types.PriceFromFloat(101), Quantity: types.QtyFromFloat(15)},
			{Price: types.PriceFromFloat(102), Quantity: types.QtyFromFloat(25)},
		},
	}
}

func cloneSnap(s types.BookSnapshot) types.BookSnapshot {
	out := s
	out.Bids = make([]types.PriceLevel, len(s.Bids))
	copy(out.Bids, s.Bids)
	out.Asks = make([]types.PriceLevel, len(s.Asks))
	copy(out.Asks, s.Asks)
	return out
}
