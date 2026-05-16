// cmd/server is the v0.3 HTTP + WebSocket server entry point.
//
// It starts a MarketActor (replay engine) and an HTTP API server,
// allowing CLI clients and web browsers to submit and manage orders.
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
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sandswind/perpCS/internal/actor"
	"github.com/sandswind/perpCS/internal/chain"
	"github.com/sandswind/perpCS/internal/chaos"
	"github.com/sandswind/perpCS/internal/fanout"
	"github.com/sandswind/perpCS/internal/provider"
	"github.com/sandswind/perpCS/internal/server"
	"github.com/sandswind/perpCS/internal/session"
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

	// v0.5: on-chain entry. When --chain-rpc is set, the server starts
	// the chain.Indexer + session.Svc and accepts /sessions/{addr} polls.
	chainRPC := flag.String("chain-rpc", "", "EVM JSON-RPC URL (enables on-chain entry; off when empty)")
	chainDeployments := flag.String("chain-deployments", "deployments/arbitrum-sepolia.json", "deployments JSON path")
	chainConfirmations := flag.Uint64("chain-confirmations", chain.DefaultConfirmations, "block confirmations before dispatching SessionStarted")
	chainPollMs := flag.Int("chain-poll-ms", 4000, "Indexer poll interval in milliseconds")
	chainStartBlock := flag.Uint64("chain-start-block", 0, "Indexer start block (0 → use deploy block from JSON)")
	chainStatePath := flag.String("chain-state", "out/indexer-state.json", "Indexer state file path")
	chainAuditPath := flag.String("chain-audit", "out/sessions.jsonl", "Session audit JSONL path")

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

	// ---- create fanout + sink ----
	fo := fanout.New()
	initialBalance := types.QtyFromFloat(*balanceFloat)
	queue := make(chan *actor.UserOrder, 256)

	// TeeSink: JSONL file (for audit) + WS fanout
	var sink actor.EventSink
	if jsonlSink, jsonlErr := actor.NewJSONLSink(fmt.Sprintf("out/%s-events.jsonl", *level)); jsonlErr != nil {
		fmt.Printf("[warn] could not create JSONL sink: %v — using fanout only\n", jsonlErr)
		sink = fo
	} else {
		sink = &actor.TeeSink{A: jsonlSink, B: fo}
	}

	// Pull funding rate history (8h cadence) for v0.4 settlements.
	var fundingPts []actor.FundingPoint
	if rates, ferr := prov.FetchFundingRates(ctx, "BTCUSDT", from, to); ferr == nil {
		fundingPts = make([]actor.FundingPoint, len(rates))
		for i, r := range rates {
			fundingPts[i] = actor.FundingPoint{TS: r.TS, Rate: r.Rate}
		}
	}

	chaosConfig := chaos.BTC_MED_L2(uint64(*seed))
	sessionQueue := make(chan *actor.OpenSessionRequest, 64)
	acfg := actor.Config{
		Symbol:         "BTC-MED",
		SessionID:      fmt.Sprintf("%s-%d", *level, time.Now().UnixNano()),
		LevelID:        *level,
		ChaosConfig:    chaosConfig,
		ReplayOrders:   orders,
		Klines:         klines,
		FundingRates:   fundingPts,
		Sink:           sink,
		OrderQueue:     queue,
		SessionQueue:   sessionQueue,
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
	srv := server.NewWithFanout(acc, queue, "BTC-MED", fo).WithActor(a)

	// ---- v0.5: optional on-chain entry path ----
	if *chainRPC != "" {
		dep, err := loadDeployment(*chainDeployments)
		if err != nil {
			return fmt.Errorf("load deployments %s: %w", *chainDeployments, err)
		}
		if !looksLikeAddress(dep.Vault) {
			return fmt.Errorf("deployments %s has no GameVault address", *chainDeployments)
		}
		startBlock := *chainStartBlock
		if startBlock == 0 {
			startBlock = dep.BlockNumber
		}

		client := chain.NewClient(*chainRPC, &http.Client{Timeout: 10 * time.Second})
		sessSvc, err := session.New(session.Config{
			Queue:     sessionQueue,
			AuditPath: *chainAuditPath,
		})
		if err != nil {
			return fmt.Errorf("session svc: %w", err)
		}

		idx, err := chain.New(chain.Config{
			Client:        client,
			VaultAddress:  dep.Vault,
			StartBlock:    startBlock,
			StatePath:     *chainStatePath,
			Confirmations: *chainConfirmations,
			PollInterval:  time.Duration(*chainPollMs) * time.Millisecond,
			Handler:       sessSvc,
		})
		if err != nil {
			return fmt.Errorf("indexer: %w", err)
		}

		go func() {
			fmt.Printf("[chain]  indexer starting; rpc=%s vault=%s startBlock=%d\n",
				*chainRPC, dep.Vault, startBlock)
			if err := idx.Run(ctx); err != nil && err != context.Canceled {
				fmt.Printf("[chain]  indexer error: %v\n", err)
			}
		}()

		srv = srv.WithSessions(sessSvc)
		fmt.Printf("[server] on-chain entry ENABLED (vault=%s)\n", dep.Vault)
	} else {
		fmt.Printf("[server] on-chain entry disabled (set --chain-rpc to enable)\n")
	}

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



// deployment is the on-chain JSON written by contracts/script/Deploy.s.sol.
type deployment struct {
	ChainID     uint64 `json:"chainId"`
	BlockNumber uint64 `json:"blockNumber"`
	USDR        string `json:"usdr"`
	Faucet      string `json:"faucet"`
	Vault       string `json:"vault"`
}

func loadDeployment(path string) (*deployment, error) {
	abs, _ := filepath.Abs(path)
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	var d deployment
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// looksLikeAddress is a cheap sanity check: 0x-prefixed and not all zero.
func looksLikeAddress(s string) bool {
	if len(s) != 42 || s[0] != '0' || s[1] != 'x' {
		return false
	}
	for _, c := range s[2:] {
		if c != '0' {
			return true
		}
	}
	return false
}
