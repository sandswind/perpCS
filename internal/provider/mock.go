package provider

import (
	"context"
	"time"

	"github.com/sandswind/perpCS/internal/types"
)

// Mock is a deterministic in-memory data provider for unit tests.
// It generates a simple descending price series to simulate a crash scenario.
type Mock struct {
	// StartPrice is the price at the beginning of the simulated window.
	StartPrice float64
	// DropPerMinute is how much the price drops each minute (simulates a crash).
	DropPerMinute float64
	// BaseVolume is the approximate volume per minute.
	BaseVolume float64
}

// DefaultMock returns a mock that simulates a mild crash (~50% drop over 24h).
func DefaultMock() *Mock {
	return &Mock{
		StartPrice:    7900,
		DropPerMinute: 0.27, // ~390 USD/h, ~50% total over 24h
		BaseVolume:    5,
	}
}

func (m *Mock) Name() string { return "mock" }

func (m *Mock) FetchKlines(_ context.Context, symbol string, from, to time.Time, interval string) ([]types.Kline, error) {
	step := intervalToDuration(interval)
	if step == 0 {
		step = time.Minute
	}

	var klines []types.Kline
	price := m.StartPrice
	for ts := from; ts.Before(to); ts = ts.Add(step) {
		open := price
		close := price - m.DropPerMinute*(step.Minutes())
		if close < 1 {
			close = 1
		}
		klines = append(klines, types.Kline{
			Symbol:    types.Symbol(symbol),
			OpenTS:    ts.UnixNano(),
			CloseTS:   ts.Add(step).UnixNano(),
			Open:      types.PriceFromFloat(open),
			High:      types.PriceFromFloat(open * 1.001),
			Low:       types.PriceFromFloat(close * 0.999),
			Close:     types.PriceFromFloat(close),
			Volume:    types.QtyFromFloat(m.BaseVolume),
			NumTrades: 10,
		})
		price = close
	}
	return klines, nil
}

func (m *Mock) FetchAggTrades(_ context.Context, symbol string, from, to time.Time) ([]types.AggTrade, error) {
	// Generate one trade per 10 seconds.
	step := 10 * time.Second
	price := m.StartPrice
	drop := m.DropPerMinute / 6 // per 10s

	var trades []types.AggTrade
	side := types.SideSell // crash is sell-driven
	for ts := from; ts.Before(to); ts = ts.Add(step) {
		trades = append(trades, types.AggTrade{
			Symbol:    types.Symbol(symbol),
			TS:        ts.UnixNano(),
			Price:     types.PriceFromFloat(price),
			Quantity:  types.QtyFromFloat(m.BaseVolume / 6),
			TakerSide: side,
		})
		price -= drop
		if price < 1 {
			price = 1
		}
		// Alternate sides occasionally
		if len(trades)%7 == 0 {
			side = types.SideBuy
		} else {
			side = types.SideSell
		}
	}
	return trades, nil
}

func (m *Mock) FetchFundingRates(_ context.Context, _ string, from, to time.Time) ([]FundingPoint, error) {
	// 8-hour funding intervals
	step := 8 * time.Hour
	var pts []FundingPoint
	for ts := from; ts.Before(to); ts = ts.Add(step) {
		pts = append(pts, FundingPoint{
			TS:   ts.UnixNano(),
			Rate: -0.0001, // negative: longs pay shorts (bearish)
		})
	}
	return pts, nil
}

func intervalToDuration(s string) time.Duration {
	switch s {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	case "1s":
		return time.Second
	default:
		return 0
	}
}
