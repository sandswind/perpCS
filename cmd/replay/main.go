// cmd/replay is the v0.1 terminal replay engine.
//
// Usage:
//
//	go run ./cmd/replay [flags]
//	make replay LEVEL=D-312-BTC SPEED=60x PROVIDER=mock
//
// Flags:
//
//	--level    disaster level ID  (default: D-312-BTC)
//	--speed    replay speed multiplier, e.g. 1x 10x 60x (default: 60x)
//	--provider data provider: mock | binance (default: mock)
//	--seed     chaos seed uint64 (default: 42)
//	--out      directory to write events.jsonl (default: ./out)
//	--duration simulated window duration (default: 24h)
//	--interval kline candle interval (default: 1m)
//	--quiet    suppress per-tick terminal output (default: false)
//
// v0.1 uses the mock provider only; binance requires internet access.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sandswind/perpCS/internal/actor"
	"github.com/sandswind/perpCS/internal/chaos"
	"github.com/sandswind/perpCS/internal/inspector"
	"github.com/sandswind/perpCS/internal/provider"
	"github.com/sandswind/perpCS/internal/types"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// ---- flags ----
	level := flag.String("level", "D-312-BTC", "disaster level ID")
	speedStr := flag.String("speed", "60x", "replay speed (e.g. 1x, 10x, 60x, 600x)")
	providerName := flag.String("provider", "mock", "data provider: mock | binance")
	seed := flag.Uint64("seed", 42, "chaos seed")
	outDir := flag.String("out", "out", "output directory for events.jsonl")
	durStr := flag.String("duration", "24h", "simulated window duration")
	interval := flag.String("interval", "1m", "kline candle interval")
	quiet := flag.Bool("quiet", false, "suppress per-tick terminal output")
	flag.Parse()

	dur, err := time.ParseDuration(*durStr)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", *durStr, err)
	}
	tickDelay, err := parseSpeed(*speedStr)
	if err != nil {
		return fmt.Errorf("invalid speed %q: %w", *speedStr, err)
	}

	// ---- data provider ----
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	prov, err := makeProvider(*providerName)
	if err != nil {
		return err
	}

	// ---- fetch data ----
	// For D-312-BTC we use 2020-03-12 as the anchor date.
	from := time.Date(2020, 3, 12, 0, 0, 0, 0, time.UTC)
	to := from.Add(dur)

	fmt.Printf("╔══════════════════════════════════════════════════════╗\n")
	fmt.Printf("║         PERP CRISIS SANDBOX — v0.1 Replay            ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Level    : %-40s ║\n", *level)
	fmt.Printf("║  Provider : %-40s ║\n", prov.Name())
	fmt.Printf("║  Seed     : %-40d ║\n", *seed)
	fmt.Printf("║  Window   : %s → %s    ║\n",
		from.Format("2006-01-02 15:04"), to.Format("2006-01-02 15:04"))
	fmt.Printf("║  Speed    : %-40s ║\n", *speedStr)
	fmt.Printf("╚══════════════════════════════════════════════════════╝\n\n")

	fmt.Printf("[1/3] Fetching klines (%s)...\n", *interval)
	klines, err := prov.FetchKlines(ctx, "BTCUSDT", from, to, *interval)
	if err != nil {
		return fmt.Errorf("fetch klines: %w", err)
	}
	fmt.Printf("      → %d klines loaded\n", len(klines))

	fmt.Printf("[2/3] Fetching aggTrades...\n")
	trades, err := prov.FetchAggTrades(ctx, "BTCUSDT", from, to)
	if err != nil {
		return fmt.Errorf("fetch trades: %w", err)
	}
	fmt.Printf("      → %d aggTrades loaded\n", len(trades))

	// ---- build replay orders ----
	fmt.Printf("[3/3] Building replay order stream...\n")
	cfg := provider.DefaultSynthConfig()
	orders := provider.KlinesToSynthOrders("BTC-MED", klines, trades, cfg, 0)
	actor.SortOrders(orders)
	fmt.Printf("      → %d replay orders generated\n\n", len(orders))

	// ---- output sink ----
	ts := time.Now().Format("20060102-150405")
	outPath := filepath.Join(*outDir, fmt.Sprintf("events-%s-%s.jsonl", *level, ts))
	fileSink, err := actor.NewJSONLSink(outPath)
	if err != nil {
		return fmt.Errorf("create sink: %w", err)
	}
	defer fileSink.Close()

	// Inspector is the tee sink that also drives terminal output.
	ins := inspector.New(*quiet, tickDelay)
	sink := &actor.TeeSink{A: fileSink, B: ins}

	// ---- create and run actor ----
	chaosConfig := chaos.BTC_MED_L2(uint64(*seed))
	acfg := actor.Config{
		Symbol:       "BTC-MED",
		SessionID:    fmt.Sprintf("%s-%d", *level, time.Now().UnixNano()),
		LevelID:      *level,
		ChaosConfig:  chaosConfig,
		ReplayOrders: orders,
		Sink:         sink,
	}
	a, err := actor.New(acfg)
	if err != nil {
		return fmt.Errorf("create actor: %w", err)
	}

	startWall := time.Now()
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		return fmt.Errorf("replay: %w", err)
	}
	elapsed := time.Since(startWall)

	// ---- summary ----
	hash := fileSink.Hash()
	fmt.Printf("\n╔══════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                   REPLAY COMPLETE                    ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Events written : %-34d ║\n", fileSink.Count())
	fmt.Printf("║  Output file    : %-34s ║\n", outPath)
	fmt.Printf("║  Wall time      : %-34s ║\n", elapsed.Round(time.Millisecond))
	fmt.Printf("║  SHA-256        : %-34s ║\n", hex.EncodeToString(hash)[:16]+"...")
	fmt.Printf("╚══════════════════════════════════════════════════════╝\n")
	return nil
}

// parseSpeed converts "60x" → 16.6ms tick delay, "1x" → 1s (1m kline / 60 = 1s per tick).
// At 1m klines with speed=60x each kline plays in 1s wall time.
func parseSpeed(s string) (time.Duration, error) {
	var mult float64
	if _, err := fmt.Sscanf(s, "%fx", &mult); err != nil || mult <= 0 {
		return 0, fmt.Errorf("expected format NUMx (e.g. 60x), got %q", s)
	}
	// At 1m klines (60 ticks per kline in 10s step mode), 1x = real time.
	// 1 kline = 60s simulated. At mult=60x: 60s / 60 = 1s per kline wall time.
	// Per-tick delay = 1s / (60/10) = 167ms at 60x.
	// Simplified: delay = 60s / (mult * 6) ≈ 10s / mult
	base := 10 * time.Second
	delay := time.Duration(float64(base) / mult)
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	return delay, nil
}

func makeProvider(name string) (provider.IDataProvider, error) {
	switch name {
	case "mock":
		return provider.DefaultMock(), nil
	case "binance":
		return provider.NewBinance(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (valid: mock, binance)", name)
	}
}

// Ensure types is imported (used indirectly via actor/inspector).
var _ = types.SideBuy
