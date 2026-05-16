// Package provider defines the data provider abstraction and implementations.
//
// Design rules (CRITICAL for determinism):
//   - No time.Now() in any provider method.
//   - All methods are pure fetch/transform; they write to a channel or return a slice.
//   - The caller controls ALL scheduling and timing via the chaos clock.
//   - Network calls use context for cancellation; callers pass a deadline.
package provider

import (
	"context"
	"time"

	"github.com/sandswind/perpCS/internal/types"
)

// IDataProvider is the interface every data source must implement.
// v0.1 uses only FetchKlines + FetchAggTrades (no L2 snapshots — use depth_synth).
type IDataProvider interface {
	// Name returns a short identifier, e.g. "binance" or "mock".
	Name() string

	// FetchKlines returns OHLCV candles for the given underlying symbol
	// (e.g. "BTCUSDT") in the half-open interval [from, to).
	// interval is a provider-specific string, e.g. "1m".
	FetchKlines(ctx context.Context, symbol string, from, to time.Time, interval string) ([]types.Kline, error)

	// FetchAggTrades returns aggregated public trades in [from, to).
	// Results MUST be sorted by TS ascending.
	FetchAggTrades(ctx context.Context, symbol string, from, to time.Time) ([]types.AggTrade, error)

	// FetchFundingRates returns historical funding rate samples in [from, to).
	// Each element represents the funding rate at that timestamp.
	// Returns empty slice (not error) if funding data is unavailable.
	FetchFundingRates(ctx context.Context, symbol string, from, to time.Time) ([]FundingPoint, error)
}

// FundingPoint is a single historical funding rate observation.
type FundingPoint struct {
	TS   int64   // Unix ns
	Rate float64 // e.g. 0.0001 = 0.01%
}
