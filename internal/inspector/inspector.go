// Package inspector implements the terminal inspector EventSink for v0.1.
//
// It listens to the event stream from MarketActor and renders a live
// order book view in the terminal, throttled to once per kline-tick.
//
// Output format (per tick):
//
//	[chaos-clock] BTC-MED  bid 6800.00 / ask 6801.00  spread 1.00
//	  ASK        6803.00        2.50000000
//	  ASK        6802.00        3.12000000
//	  ASK        6801.00        4.80000000
//	       --- spread 1.00000000 ---
//	  BID        6800.00        5.20000000
//	  BID        6799.00        3.80000000
//	  BID        6798.00        2.40000000
//	  TRADE  buy  0.50000000 @ 6801.00
package inspector

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sandswind/perpCS/internal/types"
)

// Inspector is an EventSink that renders events to stdout.
type Inspector struct {
	mu       sync.Mutex
	quiet    bool
	delay    time.Duration // wall-time delay between ticks (simulates replay speed)
	lastTick int64         // last chaos clock TS we printed (ns)

	// Accumulated trades since last snapshot
	pendingTrades []types.Trade
}

// New creates an Inspector.
//   - quiet: if true, suppress all output (useful for benchmarks)
//   - tickDelay: wall-time sleep injected between ticks (for replay speed)
func New(quiet bool, tickDelay time.Duration) *Inspector {
	return &Inspector{quiet: quiet, delay: tickDelay}
}

// Emit implements actor.EventSink.
func (ins *Inspector) Emit(e types.Event) error {
	if ins.quiet {
		return nil
	}
	ins.mu.Lock()
	defer ins.mu.Unlock()

	switch e.Type {
	case types.EventTrade:
		var p types.TradePayload
		if err := json.Unmarshal(e.Payload, &p); err == nil {
			ins.pendingTrades = append(ins.pendingTrades, p.Trade)
		}

	case types.EventBookSnapshot:
		var p types.BookSnapshotPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil
		}
		ins.renderTick(p.Snapshot)
		ins.pendingTrades = ins.pendingTrades[:0]
		// Throttle: sleep for the configured delay to simulate real-time.
		if ins.delay > 0 {
			time.Sleep(ins.delay)
		}

	case types.EventSessionStart:
		var p types.SessionStartPayload
		if err := json.Unmarshal(e.Payload, &p); err == nil {
			fmt.Printf("▶  SESSION START  symbol=%s  level=%s  seed=%d\n\n",
				p.Symbol, p.Level, p.Seed)
		}

	case types.EventSessionEnd:
		var p types.SessionEndPayload
		if err := json.Unmarshal(e.Payload, &p); err == nil {
			fmt.Printf("\n◼  SESSION END  trades=%d  finalTS=%s\n",
				p.TotalTrades, formatNS(p.FinalTS))
		}
	}
	return nil
}

// renderTick prints the current order book state and any pending trades.
func (ins *Inspector) renderTick(snap types.BookSnapshot) {
	ts := formatNS(snap.TS)

	// Top-of-book summary
	bestBid, bestAsk := types.Price(0), types.Price(0)
	if len(snap.Bids) > 0 {
		bestBid = snap.Bids[0].Price
	}
	if len(snap.Asks) > 0 {
		bestAsk = snap.Asks[0].Price
	}
	spread := bestAsk - bestBid

	fmt.Printf("\033[2K") // clear line
	fmt.Printf("[%s]  %-8s  bid %s / ask %s  spread %s\n",
		ts, snap.Symbol, bestBid, bestAsk, spread)

	// Full book display (top 5 per side)
	const maxLevels = 5
	nAsk := len(snap.Asks)
	if nAsk > maxLevels {
		nAsk = maxLevels
	}
	nBid := len(snap.Bids)
	if nBid > maxLevels {
		nBid = maxLevels
	}

	// Asks (reversed: worst to best, so best appears closest to mid)
	for i := nAsk - 1; i >= 0; i-- {
		l := snap.Asks[i]
		fmt.Printf("  \033[31mASK\033[0m  %14s  %14s\n", l.Price, l.Quantity)
	}
	fmt.Printf("       \033[33m--- spread %s ---\033[0m\n", spread)
	for i := 0; i < nBid; i++ {
		l := snap.Bids[i]
		fmt.Printf("  \033[32mBID\033[0m  %14s  %14s\n", l.Price, l.Quantity)
	}

	// Trades in this tick
	for _, t := range ins.pendingTrades {
		dir := "buy "
		color := "\033[32m"
		if t.TakerSide == types.SideSell {
			dir = "sell"
			color = "\033[31m"
		}
		fmt.Printf("  %sTRADE\033[0m  %s %s @ %s\n", color, dir, t.Quantity, t.Price)
	}
	fmt.Println()
}

// formatNS formats a Unix nanosecond timestamp as HH:MM:SS.
func formatNS(ns int64) string {
	if ns <= 0 {
		return "00:00:00"
	}
	t := time.Unix(0, ns).UTC()
	return t.Format("2006-01-02 15:04:05")
}
