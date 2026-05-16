// cmd/server is the v0.2 HTTP server entry point.
//
// It starts a MarketActor (replay engine) and an HTTP API server,
// allowing CLI clients to submit and manage orders.
//
// Usage:
//
//	go run ./cmd/server [flags]
//	make server
//
// Flags:
//
//	--level     disaster level ID (default: D-312-BTC)
//	--seed      chaos seed uint64 (default: 42)
//	--duration  simulated window (default: 24h)
//	--provider  data provider: mock | binance (default: mock)
//	--port      HTTP listen port (default: 8080)
//	--balance   player initial balance in USDR (default: 10000)
//	--address   player wallet address (default: player1)
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sandswind/perpCS/internal/actor"
	"github.com/sandswind/perpCS/internal/chaos"
	"github.com/sandswind/perpCS/internal/provider"
	"github.com/sandswind/perpCS/internal/server"
	"github.com/sandswind/perpCS/internal/types"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// ---- flags ----
	level := flag.String("level", "D-312-BTC", "disaster level ID")
	seed := flag.Uint64("seed", 42, "chaos seed")
	durStr := flag.String("duration", "24h", "simulated window duration")
	providerName := flag.String("provider", "mock", "data provider: mock | binance")
	port := flag.Int("port", 8080, "HTTP listen port")
	balanceFloat := flag.Float64("balance", 10000, "player initial balance in USDR")
	address := flag.String("address", "player1", "player wallet address")
	flag.Parse()

	dur, err := time.ParseDuration(*durStr)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", *durStr, err)
	}

	// ---- context with graceful shutdown ----
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// ---- data provider ----
	prov, err := makeProvider(*providerName)
	if err != nil {
		return err
	}

	fmt.Printf("[server] level=%s seed=%d provider=%s duration=%s port=%d\n",
		*level, *seed, prov.Name(), dur, *port)

	// ---- fetch data ----
	from := time.Date(2020, 3, 12, 0, 0, 0, 0, time.UTC)
	to := from.Add(dur)

	fmt.Printf("[data]   fetching klines...\n")
	klines, err := prov.FetchKlines(ctx, "BTCUSDT", from, to, "1m")
	if err != nil {
		return fmt.Errorf("fetch klines: %w", err)
	}
	fmt.Printf("[data]   %d klines loaded\n", len(klines))

	fmt.Printf("[data]   fetching aggTrades...\n")
	trades, err := prov.FetchAggTrades(ctx, "BTCUSDT", from, to)
	if err != nil {
		return fmt.Errorf("fetch trades: %w", err)
	}
	fmt.Printf("[data]   %d aggTrades loaded\n", len(trades))

	// ---- build replay orders ----
	fmt.Printf("[data]   building replay order stream...\n")
	cfg := provider.DefaultSynthConfig()
	orders := provider.KlinesToSynthOrders("BTC-MED", klines, trades, cfg, 0)
	actor.SortOrders(orders)
	fmt.Printf("[data]   %d replay orders ready\n", len(orders))

	// ---- create actor ----
	initialBalance := types.QtyFromFloat(*balanceFloat)
	queue := make(chan *actor.UserOrder, 256)
	sink := &actor.NullSink{} // TODO v0.3: tee to JSONL + WS fanout

	chaosConfig := chaos.BTC_MED_L2(uint64(*seed))
	acfg := actor.Config{
		Symbol:         "BTC-MED",
		SessionID:      fmt.Sprintf("%s-%d", *level, time.Now().UnixNano()),
		LevelID:        *level,
		ChaosConfig:    chaosConfig,
		ReplayOrders:   orders,
		Sink:           sink,
		OrderQueue:     queue,
		InitialBalance: initialBalance,
		PlayerAddress:  *address,
	}
	a, err := actor.New(acfg)
	if err != nil {
		return fmt.Errorf("create actor: %w", err)
	}

	// ---- start actor in background ----
	actorDone := make(chan error, 1)
	go func() {
		fmt.Printf("[actor]  %s replay started, seed=%d\n", *level, *seed)
		err := a.Run(ctx)
		if err != nil && err != context.Canceled {
			fmt.Printf("[actor]  error: %v\n", err)
		} else {
			fmt.Printf("[actor]  replay complete\n")
		}
		actorDone <- err
	}()

	// ---- HTTP server ----
	acc := a.Account(*address)
	srv := server.New(acc, queue, "BTC-MED")
	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      srv.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	httpDone := make(chan error, 1)
	go func() {
		fmt.Printf("[server] listening on :%d\n", *port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			httpDone <- err
		} else {
			httpDone <- nil
		}
	}()

	// ---- wait for shutdown ----
	select {
	case <-ctx.Done():
		fmt.Println("\n[server] shutting down...")
	case err := <-actorDone:
		if err != nil && err != context.Canceled {
			fmt.Printf("[actor]  stopped with error: %v\n", err)
		} else {
			fmt.Println("[actor]  replay finished")
		}
	case err := <-httpDone:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
	}

	// Graceful HTTP shutdown
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil {
		fmt.Printf("[server] shutdown error: %v\n", err)
	}
	cancel() // ensure actor goroutine also stops
	return nil
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

// Ensure types is used (avoids import cycle if types is not otherwise needed)
var _ = types.SideBuy
