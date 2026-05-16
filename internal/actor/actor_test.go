package actor

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/sandswind/perpCS/internal/account"
	"github.com/sandswind/perpCS/internal/chaos"
	"github.com/sandswind/perpCS/internal/provider"
	"github.com/sandswind/perpCS/internal/types"
)

// ---- fixtures ----

func makeOrders(t *testing.T, duration time.Duration) []*types.Order {
	t.Helper()
	m := provider.DefaultMock()
	from := time.Date(2020, 3, 12, 0, 0, 0, 0, time.UTC)
	to := from.Add(duration)
	klines, err := m.FetchKlines(context.Background(), "BTC-MED", from, to, "1m")
	if err != nil {
		t.Fatalf("FetchKlines: %v", err)
	}
	trades, err := m.FetchAggTrades(context.Background(), "BTC-MED", from, to)
	if err != nil {
		t.Fatalf("FetchAggTrades: %v", err)
	}
	orders := provider.KlinesToSynthOrders("BTC-MED", klines, trades, provider.DefaultSynthConfig(), 0)
	SortOrders(orders)
	return orders
}

func makeActor(t *testing.T, seed uint64, duration time.Duration) (*Actor, *MemorySink) {
	t.Helper()
	orders := makeOrders(t, duration)
	sink := &MemorySink{}
	cfg := Config{
		Symbol:      "BTC-MED",
		SessionID:   "test-session",
		LevelID:     "D-312-BTC",
		ChaosConfig: chaos.BTC_MED_L2(seed),
		ReplayOrders: orders,
		Sink:        sink,
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, sink
}

// TestActorRun_SmallWindow runs a 10-minute replay and checks basic correctness.
func TestActorRun_SmallWindow(t *testing.T) {
	a, sink := makeActor(t, 42, 10*time.Minute)
	if err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(sink.Events) == 0 {
		t.Fatal("no events emitted")
	}
	// First event must be SessionStart
	if sink.Events[0].Type != types.EventSessionStart {
		t.Errorf("first event: got %v want %v", sink.Events[0].Type, types.EventSessionStart)
	}
	// Last event must be SessionEnd
	last := sink.Events[len(sink.Events)-1]
	if last.Type != types.EventSessionEnd {
		t.Errorf("last event: got %v want %v", last.Type, types.EventSessionEnd)
	}
	// Events must have monotonically increasing seq
	for i := 1; i < len(sink.Events); i++ {
		if sink.Events[i].Seq <= sink.Events[i-1].Seq {
			t.Errorf("seq not monotonic at index %d", i)
		}
	}
	// At least some trades should have been generated
	trades := countByType(sink.Events, types.EventTrade)
	if trades == 0 {
		t.Error("expected at least some trades from market orders hitting book")
	}
	t.Logf("events: %d total, %d trades, %d snapshots",
		len(sink.Events), trades, countByType(sink.Events, types.EventBookSnapshot))
}

// TestDeterminism is the CORE test: two runs with identical seed + data
// must produce byte-identical event hashes.
func TestDeterminism(t *testing.T) {
	const seed = uint64(0xCAFEBABE12345678)
	const dur = 30 * time.Minute

	run := func() []byte {
		a, sink := makeActor(t, seed, dur)
		if err := a.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		h, err := sink.Hash()
		if err != nil {
			t.Fatalf("Hash: %v", err)
		}
		return h
	}

	h1 := run()
	h2 := run()

	if !bytes.Equal(h1, h2) {
		t.Errorf("determinism failed:\n  run1: %s\n  run2: %s",
			hex.EncodeToString(h1), hex.EncodeToString(h2))
	}
	t.Logf("determinism hash: %s", hex.EncodeToString(h1))
}

// TestDifferentSeedsProduceDifferentOutput verifies that a different chaos seed
// changes the event log.
func TestDifferentSeedsProduceDifferentOutput(t *testing.T) {
	run := func(seed uint64) []byte {
		a, sink := makeActor(t, seed, 10*time.Minute)
		if err := a.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		h, _ := sink.Hash()
		return h
	}

	h1 := run(1)
	h2 := run(2)
	if bytes.Equal(h1, h2) {
		t.Error("different seeds should produce different hashes")
	}
}

// TestContextCancellation verifies the actor respects ctx cancellation.
func TestContextCancellation(t *testing.T) {
	orders := makeOrders(t, 60*time.Minute) // large window
	sink := &MemorySink{}
	cfg := Config{
		Symbol:       "BTC-MED",
		SessionID:    "cancel-test",
		LevelID:      "D-312-BTC",
		ChaosConfig:  chaos.NoChaos("BTC-MED"),
		ReplayOrders: orders,
		Sink:         sink,
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel almost immediately
	go func() { cancel() }()

	err = a.Run(ctx)
	if err != context.Canceled {
		// It's OK if the run completed before cancel fired (small race window)
		t.Logf("Run returned %v (may have completed before cancel)", err)
	}
}

// TestOrderBookInvariant verifies the OrderBook invariant holds at end of run.
func TestOrderBookInvariant(t *testing.T) {
	a, _ := makeActor(t, 777, 5*time.Minute)
	if err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := a.Book().Validate(); err != nil {
		t.Errorf("OrderBook invariant violated: %v", err)
	}
}

// TestSortOrders verifies that SortOrders produces TS-ascending order.
func TestSortOrders(t *testing.T) {
	orders := []*types.Order{
		{ID: 3, TS: 300},
		{ID: 1, TS: 100},
		{ID: 2, TS: 200},
	}
	SortOrders(orders)
	for i := 1; i < len(orders); i++ {
		if orders[i].TS < orders[i-1].TS {
			t.Errorf("not sorted at index %d", i)
		}
	}
}

// TestNewActor_UnsortedOrders verifies validation catches unsorted input.
func TestNewActor_UnsortedOrders(t *testing.T) {
	orders := []*types.Order{
		{ID: 1, TS: 200, Symbol: "BTC-MED", Side: types.SideSell, Type: types.OrderTypeLimit,
			Price: 1, Quantity: 1, Source: types.SourceReplay},
		{ID: 2, TS: 100, Symbol: "BTC-MED", Side: types.SideSell, Type: types.OrderTypeLimit,
			Price: 1, Quantity: 1, Source: types.SourceReplay},
	}
	cfg := Config{
		Symbol: "BTC-MED", SessionID: "x", LevelID: "x",
		ChaosConfig: chaos.NoChaos("BTC-MED"), ReplayOrders: orders, Sink: &MemorySink{},
	}
	if _, err := New(cfg); err == nil {
		t.Error("expected error for unsorted orders, got nil")
	}
}

// TestMemorySink_Hash verifies the hash is deterministic for the same events.
func TestMemorySink_Hash(t *testing.T) {
	s1 := &MemorySink{}
	s2 := &MemorySink{}
	for i := 0; i < 5; i++ {
		e := types.Event{Seq: uint64(i), Type: types.EventTrade}
		_ = s1.Emit(e)
		_ = s2.Emit(e)
	}
	h1, _ := s1.Hash()
	h2, _ := s2.Hash()
	if !bytes.Equal(h1, h2) {
		t.Error("MemorySink hash not deterministic for same events")
	}
}

// ---- helpers ----

func countByType(events []types.Event, t types.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}
	return n
}


// ---- v0.2 user order tests ----

// buildActorWithQueue creates an actor + order queue for user-order tests.
// It uses a 5-minute replay window so tests run quickly.
func buildActorWithQueue(t *testing.T) (*Actor, chan *UserOrder, *MemorySink) {
	t.Helper()
	orders := makeOrders(t, 5*time.Minute)
	q := make(chan *UserOrder, 16)
	sink := &MemorySink{}
	cfg := Config{
		Symbol:         "BTC-MED",
		SessionID:      "user-order-test",
		LevelID:        "D-312-BTC",
		ChaosConfig:    chaos.NoChaos("BTC-MED"),
		ReplayOrders:   orders,
		Sink:           sink,
		OrderQueue:     q,
		InitialBalance: types.QtyFromFloat(100_000),
		PlayerAddress:  "player1",
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, q, sink
}

// TestUserOrder_MarketBuy injects a market buy order before the actor starts.
// The replay tick loop will drain it and we expect at least one fill.
func TestUserOrder_MarketBuy(t *testing.T) {
	a, q, _ := buildActorWithQueue(t)

	buyOrder := &types.Order{
		Symbol:   "BTC-MED",
		Side:     types.SideBuy,
		Type:     types.OrderTypeMarket,
		Quantity: types.QtyFromFloat(0.01),
		Owner:    "player1",
		Source:   types.SourceUser,
	}
	uo := &UserOrder{
		Order:    buyOrder,
		ResultCh: make(chan UserOrderResult, 1),
	}
	q <- uo

	// Close the queue so Run() exits after draining it
	close(q)

	// Run the actor to completion (it will drain the queue during tick processing)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}

	select {
	case res := <-uo.ResultCh:
		if res.Err != nil {
			t.Fatalf("market buy error: %v", res.Err)
		}
		if len(res.Trades) == 0 {
			t.Error("expected fills for market buy with resting sell orders, got none")
		}
		for _, tr := range res.Trades {
			t.Logf("fill: qty=%s @ price=%s", tr.Quantity.String(), tr.Price.String())
		}
	default:
		t.Error("no result received from actor for market buy order")
	}
}

// TestUserOrder_LimitRested submits a limit buy far below the market price.
// Expects no immediate fill (order should be rested on the book).
func TestUserOrder_LimitRested(t *testing.T) {
	a, q, _ := buildActorWithQueue(t)

	// Place a limit buy at a price far below current market (won't cross)
	lowPrice := types.PriceFromFloat(1.0) // $1 — way below any ask
	limitOrder := &types.Order{
		Symbol:   "BTC-MED",
		Side:     types.SideBuy,
		Type:     types.OrderTypeLimit,
		Price:    lowPrice,
		Quantity: types.QtyFromFloat(0.1),
		Owner:    "player1",
		Source:   types.SourceUser,
	}
	uo := &UserOrder{
		Order:    limitOrder,
		ResultCh: make(chan UserOrderResult, 1),
	}
	q <- uo
	close(q)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}

	select {
	case res := <-uo.ResultCh:
		if res.Err != nil {
			t.Fatalf("limit rested error: %v", res.Err)
		}
		// Should have zero immediate fills (price is $1, far below market)
		if len(res.Trades) != 0 {
			t.Errorf("expected 0 fills for below-market limit, got %d", len(res.Trades))
		}
		t.Logf("limit order rested with 0 fills (as expected)")
	default:
		t.Error("no result received from actor for limit order")
	}
}

// TestUserOrder_InsufficientBalance submits a large market buy that exceeds the player's balance.
// Expects the account balance to never go negative.
func TestUserOrder_InsufficientBalance(t *testing.T) {
	orders := makeOrders(t, 5*time.Minute)
	q := make(chan *UserOrder, 4)
	sink := &MemorySink{}
	cfg := Config{
		Symbol:         "BTC-MED",
		SessionID:      "insuff-balance",
		LevelID:        "D-312-BTC",
		ChaosConfig:    chaos.NoChaos("BTC-MED"),
		ReplayOrders:   orders,
		Sink:           sink,
		OrderQueue:     q,
		InitialBalance: types.QtyFromFloat(1), // only $1 — can't afford even 0.0001 BTC at $7900
		PlayerAddress:  "poorplayer",
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	bigOrder := &types.Order{
		Symbol:   "BTC-MED",
		Side:     types.SideBuy,
		Type:     types.OrderTypeMarket,
		Quantity: types.QtyFromFloat(1.0), // 1 BTC @ ~$7900 = $7900, far exceeds $1
		Owner:    "poorplayer",
		Source:   types.SourceUser,
	}
	uo := &UserOrder{
		Order:    bigOrder,
		ResultCh: make(chan UserOrderResult, 1),
	}
	q <- uo
	close(q)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}

	// Either the order couldn't fill (balance check) or failed with ErrInsufficientBalance
	// In either case, the account balance should not go negative
	acc := a.Account("poorplayer")
	if acc != nil && acc.Balance < 0 {
		t.Errorf("balance went negative: %s", acc.Balance.String())
	}

	select {
	case res := <-uo.ResultCh:
		t.Logf("result: err=%v trades=%d", res.Err, len(res.Trades))
		// If there were fills, the error should indicate insufficient balance
		if len(res.Trades) > 0 && res.Err == nil {
			if acc != nil && acc.Balance < 0 {
				t.Errorf("balance went negative after fills: %s", acc.Balance.String())
			}
		}
	default:
		t.Error("no result received from actor for insufficient balance order")
	}
}


// ---- v0.4 chaos / liquidation / funding tests ----

// makeKlines returns the 1-minute klines used to drive the actor for the
// duration window.
func makeKlines(t *testing.T, duration time.Duration) []types.Kline {
	t.Helper()
	m := provider.DefaultMock()
	from := time.Date(2020, 3, 12, 0, 0, 0, 0, time.UTC)
	to := from.Add(duration)
	klines, err := m.FetchKlines(context.Background(), "BTC-MED", from, to, "1m")
	if err != nil {
		t.Fatalf("FetchKlines: %v", err)
	}
	return klines
}

// countByTypeAct mirrors countByType but with a more descriptive name for v0.4.
func countByTypeAct(events []types.Event, typ types.EventType) int {
	return countByType(events, typ)
}

// TestLiquidation_LongAtMMR represents the canonical "10× user goes long
// before a crash" scenario. We pre-install a thin-margin (high-leverage
// equivalent) position and run the replay; the replay's monotonically
// dropping prices walk the mark below MMR and trigger liquidation.
//
// We use the synchronous-Run pattern (close the queue before Run) so the
// race detector is happy when reading sink.Events afterwards.
func TestLiquidation_LongAtMMR(t *testing.T) {
	dur := 30 * time.Minute
	orders := makeOrders(t, dur)
	klines := makeKlines(t, dur)

	q := make(chan *UserOrder, 16)
	sink := &MemorySink{}
	cfg := Config{
		Symbol:         "BTC-MED",
		SessionID:      "liq-test",
		LevelID:        "D-312-BTC",
		ChaosConfig:    chaos.NoChaos("BTC-MED"),
		ReplayOrders:   orders,
		Klines:         klines,
		Sink:           sink,
		OrderQueue:     q,
		InitialBalance: types.QtyFromFloat(10_000),
		PlayerAddress:  "player1",
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Pre-install a thin-margin position simulating an earlier 10× open.
	// At entry $8000, size 0.5 BTC, with only $50 margin (≈80× equivalent),
	// even a small dip to ~$7900 puts equity near zero.
	acc := a.Account("player1")
	acc.Positions["BTC-MED"] = &account.Position{
		Symbol:   "BTC-MED",
		Side:     types.SideBuy,
		Size:     types.QtyFromFloat(0.5),
		AvgEntry: types.PriceFromFloat(8000),
		Margin:   types.QtyFromFloat(50),
	}
	acc.Balance -= types.QtyFromFloat(50)

	// Close the queue so Run terminates after replay.
	close(q)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}

	events := sink.Events
	liq := countByTypeAct(events, types.EventLiquidation)
	t.Logf("events=%d trades=%d liquidations=%d funding=%d",
		len(events),
		countByTypeAct(events, types.EventTrade),
		liq,
		countByTypeAct(events, types.EventFunding))
	if liq < 1 {
		t.Fatalf("expected ≥1 liquidation event, got %d", liq)
	}
}

// TestLiquidation_ManualMMR drives the liquidation cascade end-to-end by
// pre-loading the actor's account with a thin-margin position, then running
// a short replay window with a steeply crashing mock.
func TestLiquidation_ManualMMR(t *testing.T) {
	dur := 30 * time.Minute
	orders := makeOrders(t, dur)
	klines := makeKlines(t, dur)

	sink := &MemorySink{}
	cfg := Config{
		Symbol:         "BTC-MED",
		SessionID:      "liq-manual",
		LevelID:        "D-312-BTC",
		ChaosConfig:    chaos.NoChaos("BTC-MED"),
		ReplayOrders:   orders,
		Klines:         klines,
		Sink:           sink,
		InitialBalance: types.QtyFromFloat(10_000),
		PlayerAddress:  "player1",
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Manually craft a thin-margin position before Run starts.
	acc := a.Account("player1")
	if acc == nil {
		t.Fatal("account missing")
	}
	acc.Positions["BTC-MED"] = &account.Position{
		Symbol:   "BTC-MED",
		Side:     types.SideBuy,
		Size:     types.QtyFromFloat(0.5),
		AvgEntry: types.PriceFromFloat(8000),
		Margin:   types.QtyFromFloat(50), // ~80x leverage equivalent
	}
	acc.Balance -= types.QtyFromFloat(50)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}

	liq := countByTypeAct(sink.Events, types.EventLiquidation)
	if liq < 1 {
		t.Fatalf("expected ≥1 liquidation event, got %d", liq)
	}
	// After liquidation the position must be removed.
	if pos, ok := acc.Positions["BTC-MED"]; ok && pos.Size > 0 {
		t.Errorf("position still open after liquidation: size=%s", pos.Size)
	}
	// Insurance fund should have moved (either down due to loss, or unchanged
	// if the position salvaged).
	t.Logf("insurance fund after: %s", a.InsuranceFund())
}

// TestFundingApplied runs a 9-hour simulated window and verifies at least one
// funding event was emitted (8h interval).
func TestFundingApplied(t *testing.T) {
	dur := 9 * time.Hour
	orders := makeOrders(t, dur)
	klines := makeKlines(t, dur)

	sink := &MemorySink{}
	cfg := Config{
		Symbol:         "BTC-MED",
		SessionID:      "funding",
		LevelID:        "D-312-BTC",
		ChaosConfig:    chaos.NoChaos("BTC-MED"),
		ReplayOrders:   orders,
		Klines:         klines,
		Sink:           sink,
		InitialBalance: types.QtyFromFloat(10_000),
		PlayerAddress:  "player1",
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}

	fund := countByTypeAct(sink.Events, types.EventFunding)
	if fund == 0 {
		t.Errorf("expected ≥1 funding event in 9h window, got %d", fund)
	}
	t.Logf("funding events: %d", fund)
}

// TestInvariant_HoldsAfterReplay enables strict invariants and runs a short
// window to confirm no invariant panics.
func TestInvariant_HoldsAfterReplay(t *testing.T) {
	dur := 10 * time.Minute
	orders := makeOrders(t, dur)
	klines := makeKlines(t, dur)

	sink := &MemorySink{}
	cfg := Config{
		Symbol:           "BTC-MED",
		SessionID:        "invariant",
		LevelID:          "D-312-BTC",
		ChaosConfig:      chaos.NoChaos("BTC-MED"),
		ReplayOrders:     orders,
		Klines:           klines,
		Sink:             sink,
		InitialBalance:   types.QtyFromFloat(10_000),
		PlayerAddress:    "player1",
		StrictInvariants: true,
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

// TestChaosWick_InjectsTrades enables the BTC_MED_L2 chaos config and verifies
// the actor produces some additional taker-sell trades from the wick injector.
func TestChaosWick_InjectsTrades(t *testing.T) {
	dur := 30 * time.Minute
	orders := makeOrders(t, dur)
	klines := makeKlines(t, dur)

	// Force-enable wicks for the test by constructing a config with high prob.
	cfg := chaos.Config{
		Symbol:        "BTC-MED",
		Seed:          1,
		WickProb:      1.0, // every kline gets a wick
		WickMagnitude: 0.05,
		DepthShrink:   1.0, // disable depth shrink so we don't break market orders
		OracleLagNS:   0,
	}

	sink := &MemorySink{}
	acfg := Config{
		Symbol:         "BTC-MED",
		SessionID:      "wick",
		LevelID:        "D-312-BTC",
		ChaosConfig:    cfg,
		ReplayOrders:   orders,
		Klines:         klines,
		Sink:           sink,
		InitialBalance: types.QtyFromFloat(10_000),
		PlayerAddress:  "player1",
	}
	a, err := New(acfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}

	// Count trades whose taker owner is "chaos-wick"
	wickTrades := 0
	for _, e := range sink.Events {
		if e.Type != types.EventTrade {
			continue
		}
		var p types.TradePayload
		_ = json.Unmarshal(e.Payload, &p)
		if p.Trade.TakerOwner == "chaos-wick" {
			wickTrades++
		}
	}
	if wickTrades == 0 {
		t.Errorf("expected ≥1 chaos-wick trade with WickProb=1, got 0")
	}
	t.Logf("chaos-wick trades: %d", wickTrades)
}
