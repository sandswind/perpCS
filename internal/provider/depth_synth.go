// Package provider — depth_synth.go
//
// Synthetic L2 order book depth generator.
//
// When a provider cannot supply historical L2 snapshots (e.g. Binance only
// retains ~30 days of order book data), we synthesise a plausible depth from:
//   - the mid price from the last kline close
//   - recent volume to calibrate total depth
//
// The model is a simple power-law distribution: quantity at offset k from
// mid is proportional to 1/k^alpha (default alpha=1.5).
//
// This is ONLY used for the matching engine's resting liquidity context;
// synthetic depth is labelled SourceReplay so wash-trade detection can
// exclude it from the volume leaderboard.
package provider

import (
	"math"

	"github.com/sandswind/perpCS/internal/types"
)

// SynthDepthConfig controls how depth is generated.
type SynthDepthConfig struct {
	// NumLevels is the number of price levels per side.
	NumLevels int
	// TickSize is the price increment between levels (in float64 USDR).
	TickSize float64
	// TotalQty is the total quantity to distribute across all levels on each side.
	TotalQty float64
	// Alpha is the power-law exponent (higher = more concentrated at best).
	Alpha float64
}

// DefaultSynthConfig returns a reasonable config for BTC-MED.
func DefaultSynthConfig() SynthDepthConfig {
	return SynthDepthConfig{
		NumLevels: 20,
		TickSize:  0.5,  // $0.50 between levels
		TotalQty:  50.0, // 50 BTC total per side
		Alpha:     1.5,
	}
}

// SynthOrderBook generates a synthetic L2 snapshot centred on midPrice.
// ts is the chaos clock timestamp for the snapshot.
func SynthOrderBook(symbol types.Symbol, midPrice types.Price, ts int64, cfg SynthDepthConfig) types.BookSnapshot {
	snap := types.BookSnapshot{Symbol: symbol, TS: ts}
	snap.Bids = make([]types.PriceLevel, cfg.NumLevels)
	snap.Asks = make([]types.PriceLevel, cfg.NumLevels)

	// Precompute power-law weights: weight[k] = 1/(k+1)^alpha
	weights := make([]float64, cfg.NumLevels)
	sum := 0.0
	for k := range weights {
		weights[k] = math.Pow(float64(k+1), -cfg.Alpha)
		sum += weights[k]
	}

	for k := 0; k < cfg.NumLevels; k++ {
		qty := types.QtyFromFloat(cfg.TotalQty * weights[k] / sum)
		// Use integer arithmetic for tick offsets to avoid float64 rounding
		// causing bid >= ask at level 0.
		tickPrice := types.PriceFromFloat(cfg.TickSize)
		bidPrice := midPrice - types.Price(k+1)*tickPrice
		askPrice := midPrice + types.Price(k+1)*tickPrice
		// Safety: ensure bid < ask (should always be true with integer arithmetic)
		if bidPrice >= askPrice {
			askPrice = bidPrice + tickPrice
		}
		snap.Bids[k] = types.PriceLevel{Price: bidPrice, Quantity: qty}
		snap.Asks[k] = types.PriceLevel{Price: askPrice, Quantity: qty}
	}
	return snap
}

// KlinesToSynthOrders converts a batch of klines + aggTrades into a slice of
// synthetic replay orders suitable for injection into the OrderBook.
//
// Strategy:
//  1. For each kline, create resting limit orders around the close price using
//     SynthOrderBook parameters, refreshing depth every kline interval.
//  2. For each aggTrade, create a market-side order that walks through the book.
//
// All generated orders have Source = SourceReplay and Owner = "synth-mm-{side}".
func KlinesToSynthOrders(
	symbol types.Symbol,
	klines []types.Kline,
	trades []types.AggTrade,
	cfg SynthDepthConfig,
	startID uint64,
) []*types.Order {
	var orders []*types.Order
	id := startID

	// Index trades by ts for efficient lookup
	tradeIdx := 0

	// Precompute power-law weights
	weights := make([]float64, cfg.NumLevels)
	sum := 0.0
	for k := range weights {
		weights[k] = math.Pow(float64(k+1), -cfg.Alpha)
		sum += weights[k]
	}

	for _, k := range klines {
		mid := k.Close

		// Add resting limit orders at the kline open time
		for level := 0; level < cfg.NumLevels; level++ {
			qty := types.QtyFromFloat(cfg.TotalQty * weights[level] / sum)
			tickPrice := types.PriceFromFloat(cfg.TickSize)
			bidPrice := mid - types.Price(level+1)*tickPrice
			askPrice := mid + types.Price(level+1)*tickPrice

			// Bid
			id++
			orders = append(orders, &types.Order{
				ID:       types.OrderID(id),
				Symbol:   symbol,
				Side:     types.SideBuy,
				Type:     types.OrderTypeLimit,
				Price:    bidPrice,
				Quantity: qty,
				TS:       k.OpenTS,
				Source:   types.SourceReplay,
				Owner:    "synth-mm-buy",
			})

			// Ask
			id++
			orders = append(orders, &types.Order{
				ID:       types.OrderID(id),
				Symbol:   symbol,
				Side:     types.SideSell,
				Type:     types.OrderTypeLimit,
				Price:    askPrice,
				Quantity: qty,
				TS:       k.OpenTS,
				Source:   types.SourceReplay,
				Owner:    "synth-mm-sell",
			})
		}

		// Inject aggTrades that fall within this kline's window as market orders
		for tradeIdx < len(trades) && trades[tradeIdx].TS < k.CloseTS {
			t := trades[tradeIdx]
			id++
			orders = append(orders, &types.Order{
				ID:       types.OrderID(id),
				Symbol:   symbol,
				Side:     t.TakerSide,
				Type:     types.OrderTypeMarket,
				Quantity: t.Quantity,
				TS:       t.TS,
				Source:   types.SourceReplay,
				Owner:    "synth-trade",
			})
			tradeIdx++
		}
	}
	return orders
}
