// cmd/demo runs an end-to-end v0.4 liquidation demo.
//
// Boots a server in-process, opens a thin-margin long position via the
// MarketActor's account map (simulating a 10x leverage that v0.x's full
// margin policy can't yet open via /orders), runs the 30-min replay
// window, and reports whether a liquidation event was emitted.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/sandswind/perpCS/internal/account"
	"github.com/sandswind/perpCS/internal/actor"
	"github.com/sandswind/perpCS/internal/chaos"
	"github.com/sandswind/perpCS/internal/provider"
	"github.com/sandswind/perpCS/internal/types"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "demo: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	prov := provider.DefaultMock()
	// D-312-style crash: steep drop to model the historical -50% in 30 min.
	// At default 0.27 USD/min the test window barely moves; bump to 100/min so
	// the player's 10x long actually breaches MMR.
	prov.DropPerMinute = 100
	from := time.Date(2020, 3, 12, 0, 0, 0, 0, time.UTC)
	to := from.Add(30 * time.Minute)

	klines, err := prov.FetchKlines(ctx, "BTCUSDT", from, to, "1m")
	if err != nil {
		return err
	}
	trades, err := prov.FetchAggTrades(ctx, "BTCUSDT", from, to)
	if err != nil {
		return err
	}
	orders := provider.KlinesToSynthOrders("BTC-MED", klines, trades, provider.DefaultSynthConfig(), 0)
	actor.SortOrders(orders)
	fmt.Printf("[demo] %d klines, %d trades, %d replay orders\n",
		len(klines), len(trades), len(orders))

	rates, _ := prov.FetchFundingRates(ctx, "BTCUSDT", from, to)
	fundingPts := make([]actor.FundingPoint, len(rates))
	for i, r := range rates {
		fundingPts[i] = actor.FundingPoint{TS: r.TS, Rate: r.Rate}
	}

	sink := &actor.MemorySink{}
	cfg := actor.Config{
		Symbol:         "BTC-MED",
		SessionID:      "demo",
		LevelID:        "D-312-BTC",
		ChaosConfig:    chaos.NoChaos("BTC-MED"),
		ReplayOrders:   orders,
		Klines:         klines,
		FundingRates:   fundingPts,
		Sink:           sink,
		InitialBalance: types.QtyFromFloat(10_000),
		PlayerAddress:  "player1",
	}
	a, err := actor.New(cfg)
	if err != nil {
		return err
	}

	// Simulate a 10x long: 1 BTC @ $8000 with $800 margin (10x leverage).
	acc := a.Account("player1")
	acc.Positions["BTC-MED"] = &account.Position{
		Symbol:   "BTC-MED",
		Side:     types.SideBuy,
		Size:     types.QtyFromFloat(1.0),
		AvgEntry: types.PriceFromFloat(8000),
		Margin:   types.QtyFromFloat(800),
	}
	acc.Balance -= types.QtyFromFloat(800)
	fmt.Printf("[demo] opened 10x long: 1 BTC @ $8000, margin $800, free balance $%s\n",
		acc.Balance.String())

	if err := a.Run(ctx); err != nil && err != context.Canceled {
		return err
	}

	var liq, fund int
	var firstLiq *types.LiquidationPayload
	for _, e := range sink.Events {
		switch e.Type {
		case types.EventLiquidation:
			liq++
			if firstLiq == nil {
				var p types.LiquidationPayload
				_ = json.Unmarshal(e.Payload, &p)
				firstLiq = &p
			}
		case types.EventFunding:
			fund++
		}
	}

	fmt.Printf("\n[demo] events=%d  liquidations=%d  funding=%d\n",
		len(sink.Events), liq, fund)
	if firstLiq != nil {
		fmt.Printf("[demo] FIRST LIQUIDATION: addr=%s sym=%s size=%s mark=%s loss=%s ts=%d\n",
			firstLiq.Address, firstLiq.Symbol,
			firstLiq.Size.String(), firstLiq.MarkPrice.String(),
			firstLiq.Loss.String(), firstLiq.TS)
	}
	fmt.Printf("[demo] insurance fund after: %s USDR\n", a.InsuranceFund().String())
	if liq == 0 {
		fmt.Printf("[demo] NO LIQUIDATION (mock crash too mild for 10x; try higher leverage)\n")
	}
	return nil
}
