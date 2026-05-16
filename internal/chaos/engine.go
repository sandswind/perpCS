// Package chaos implements the server-side chaos pipeline.
//
// CRITICAL DESIGN RULE: No time.Now() anywhere in this package.
// All randomness is derived deterministically from a seed + tick index.
// The same (seed, tick stream) MUST produce byte-identical output every run.
// This is the foundation of the replay / audit system.
//
// Chaos pipeline per tick (applied in order):
//  1. Wick injection   — randomly insert a price spike in the kline
//  2. Depth shrink     — scale down order book quantity at all levels
//  3. Mark price lag   — delay mark price from last price by N ns
//  4. Funding boost    — add extra funding rate charge (v0.4+, stub here)
package chaos

import (
	"github.com/sandswind/perpCS/internal/types"
)

// Config holds all chaos parameters for one symbol.
// Values come from the chaos_config Postgres table (loaded at startup).
// Zero values mean "no effect" for that parameter.
type Config struct {
	Symbol types.Symbol

	// Seed is locked at session start:
	//   seed = xorshift64(sessionID_hash XOR serverCommit_hash)
	// Never changes during a session.
	Seed uint64

	// WickProb is the probability [0,1] of injecting a wick per tick.
	WickProb float64
	// WickMagnitude is the wick size as a fraction of price (e.g. 0.05 = 5%).
	WickMagnitude float64

	// DepthShrink scales all book quantities: 1.0 = no change, 0.05 = 95% drain.
	DepthShrink float64

	// OracleLagNS is how many nanoseconds mark price lags behind last price.
	OracleLagNS int64

	// FundingBoost is an extra multiplier on the historical funding rate (stub).
	FundingBoost float64
}

// BTC_MED_L2 returns the default chaos config for BTC-MED (L2 difficulty).
// These match the design matrix in design.md §4.
func BTC_MED_L2(seed uint64) Config {
	return Config{
		Symbol:        "BTC-MED",
		Seed:          seed,
		WickProb:      0.05 / 600,  // 0.05 per minute = 0.05/600 per 100ms tick
		WickMagnitude: 0.05,        // 5% wick
		DepthShrink:   0.20,        // book at 20% of original depth
		OracleLagNS:   3_000_000_000, // 3 seconds
		FundingBoost:  0,
	}
}

// NoChaos returns a zero-effect config (used in tests and L1 easy mode).
func NoChaos(symbol types.Symbol) Config {
	return Config{Symbol: symbol, DepthShrink: 1.0}
}

// Engine applies the chaos pipeline to a stream of market data.
// It is NOT safe for concurrent use — one Engine per MarketActor goroutine.
type Engine struct {
	cfg      Config
	tickIdx  uint64 // monotonically increasing; used as input to xorshift
	markBuf  []markEntry
	markHead int
}

type markEntry struct {
	ts    int64
	price types.Price
}

// New creates a ChaosEngine with the given config.
func New(cfg Config) *Engine {
	// Mark lag buffer: store last N entries to simulate lag.
	// At 100ms tick rate, 3s lag = 30 entries.
	bufSize := 1
	if cfg.OracleLagNS > 0 {
		bufSize = 120 // generous buffer; sized for up to 12s lag at 100ms/tick
	}
	return &Engine{
		cfg:     cfg,
		markBuf: make([]markEntry, bufSize),
	}
}

// Config returns the current chaos configuration (read-only copy).
func (e *Engine) Config() Config { return e.cfg }

// ApplyToSnapshot modifies a BookSnapshot in-place according to depth shrink.
// Call this once per tick before injecting orders into the OrderBook.
func (e *Engine) ApplyToSnapshot(snap *types.BookSnapshot) {
	shrink := e.cfg.DepthShrink
	if shrink <= 0 || shrink >= 1.0 {
		return // no effect (0 = full drain, handled as edge case)
	}
	for i := range snap.Bids {
		snap.Bids[i].Quantity = types.Qty(float64(snap.Bids[i].Quantity) * shrink)
	}
	for i := range snap.Asks {
		snap.Asks[i].Quantity = types.Qty(float64(snap.Asks[i].Quantity) * shrink)
	}
}

// ApplyWick potentially injects a downward wick into a kline's price range.
// Returns the (possibly modified) Low price to be used when injecting
// the worst-case replay sell order.
//
// A wick appears as a single large sell order at (price * (1-magnitude)).
// The wick is below the Close to simulate a flash crash momentarily breaking
// support before recovering to Close by end of the kline.
func (e *Engine) ApplyWick(kline types.Kline) (wickLow types.Price, injected bool) {
	e.tickIdx++
	r := xorshift64(e.cfg.Seed, e.tickIdx)
	prob := e.cfg.WickProb

	// r is uint64 [0, 2^64). Convert to [0,1) float.
	rFloat := float64(r) / float64(^uint64(0))
	if rFloat > prob || prob == 0 {
		return 0, false
	}

	// Inject wick: worst price = close * (1 - magnitude)
	mag := e.cfg.WickMagnitude
	if mag <= 0 {
		return 0, false
	}
	wickPrice := types.Price(float64(kline.Close) * (1.0 - mag))
	if wickPrice <= 0 {
		wickPrice = 1
	}
	return wickPrice, true
}

// MarkPrice returns the mark price for a given (lastPrice, ts) pair,
// applying the oracle lag: the returned price is what the system would
// have seen OracleLagNS nanoseconds ago.
//
// On the first few ticks (before the buffer is full) it returns lastPrice
// to avoid a zero mark price during warmup.
func (e *Engine) MarkPrice(lastPrice types.Price, ts int64) types.Price {
	lag := e.cfg.OracleLagNS
	if lag == 0 {
		return lastPrice
	}

	// Rotate buffer
	e.markHead = (e.markHead + 1) % len(e.markBuf)
	e.markBuf[e.markHead] = markEntry{ts: ts, price: lastPrice}

	// Find the entry that is >= lagNS old
	targetTS := ts - lag
	oldest := e.markBuf[(e.markHead+1)%len(e.markBuf)] // oldest entry
	if oldest.price == 0 || oldest.ts >= targetTS {
		// Buffer not full yet or lag exceeds buffer depth → use last known
		return lastPrice
	}

	// Walk buffer from oldest to find first entry at or after targetTS
	for i := 0; i < len(e.markBuf); i++ {
		idx := (e.markHead + 1 + i) % len(e.markBuf)
		entry := e.markBuf[idx]
		if entry.ts >= targetTS {
			return entry.price
		}
	}
	return lastPrice
}

// TickIndex returns the current tick counter (useful for tests).
func (e *Engine) TickIndex() uint64 { return e.tickIdx }

// ---- Deterministic PRNG ----

// xorshift64 is a non-linear hash of (seed, n) → uint64.
// Properties:
//   - Deterministic: same inputs → same output, always.
//   - Period: full 2^64 cycle.
//   - Fast: 3 XOR-shifts, no division.
//   - NOT cryptographically secure (not needed here).
//
// We derive each random value independently from (seed, tickIdx) rather
// than maintaining state, so the sequence is non-sequential but stable.
// This means skipping ticks in a replay does not alter later wick decisions.
func xorshift64(seed, n uint64) uint64 {
	// Mix seed and n together
	x := seed ^ (n * 6364136223846793005) // LCG mixing constant
	// Xorshift64* algorithm
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	return x * 0x2545F4914F6CDD1D
}
