package session

import (
	"context"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/sandswind/perpCS/internal/actor"
	"github.com/sandswind/perpCS/internal/chain"
)

func makeEvent(sessionHex string, player string, amountUSDR int64) *chain.SessionStartedEvent {
	ev := &chain.SessionStartedEvent{
		Player:      player,
		Amount:      new(big.Int).Mul(big.NewInt(amountUSDR), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
		Nonce:       big.NewInt(1),
		BlockNumber: big.NewInt(123),
		TxHash:      "0xdeadbeef",
		LogIdx:      0,
	}
	// session id from hex
	for i, c := range []byte(sessionHex) {
		if i >= 32 {
			break
		}
		ev.SessionID[i] = c
	}
	copy(ev.LevelID[:], "BTC-MED")
	return ev
}

func TestSvc_DedupesDuplicateEvents(t *testing.T) {
	t.Parallel()
	queue := make(chan *actor.OpenSessionRequest, 4)

	// Drain the queue in a goroutine, ack-ing every request.
	done := make(chan struct{})
	go func() {
		for req := range queue {
			req.ResultCh <- actor.OpenSessionResult{}
		}
		close(done)
	}()

	dir := t.TempDir()
	svc, err := New(Config{
		Queue:     queue,
		AuditPath: filepath.Join(dir, "sessions.jsonl"),
	})
	if err != nil {
		t.Fatal(err)
	}

	ev := makeEvent("first", "0xABC123abc", 500)
	if err := svc.HandleSessionStarted(context.Background(), ev); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	// Second time with same sessionId — should dedup (no actor request).
	if err := svc.HandleSessionStarted(context.Background(), ev); err != nil {
		t.Fatalf("dup dispatch: %v", err)
	}
	if svc.Count() != 1 {
		t.Errorf("Count = %d, want 1 after dedup", svc.Count())
	}

	// Player lookup should normalize case.
	rec := svc.LookupByPlayer("0xabc123abc")
	if rec == nil {
		t.Fatalf("LookupByPlayer returned nil")
	}
	if rec.AmountUSDR != "500" {
		t.Errorf("amount human = %s, want 500", rec.AmountUSDR)
	}

	close(queue)
	<-done
	_ = svc.Close()
}

func TestSvc_PersistsAcrossRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "sessions.jsonl")

	queue1 := make(chan *actor.OpenSessionRequest, 4)
	go func() {
		for req := range queue1 {
			req.ResultCh <- actor.OpenSessionResult{}
		}
	}()
	svc1, err := New(Config{Queue: queue1, AuditPath: auditPath})
	if err != nil {
		t.Fatal(err)
	}
	ev := makeEvent("foo", "0xdeadbeef", 1000)
	if err := svc1.HandleSessionStarted(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if err := svc1.Close(); err != nil {
		t.Fatal(err)
	}
	close(queue1)

	// Re-open: should pre-populate dedup tables.
	queue2 := make(chan *actor.OpenSessionRequest, 4)
	gotReq := make(chan *actor.OpenSessionRequest, 4)
	go func() {
		for req := range queue2 {
			gotReq <- req
			req.ResultCh <- actor.OpenSessionResult{}
		}
	}()
	svc2, err := New(Config{Queue: queue2, AuditPath: auditPath})
	if err != nil {
		t.Fatal(err)
	}
	if svc2.Count() != 1 {
		t.Errorf("after restart Count = %d, want 1 (replay)", svc2.Count())
	}
	// Re-handle the same event — should dedup, not enqueue.
	if err := svc2.HandleSessionStarted(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	close(queue2)
	select {
	case <-gotReq:
		t.Errorf("dedup failed across restart — request was enqueued")
	case <-time.After(50 * time.Millisecond):
		// good
	}
	_ = svc2.Close()
}

func TestSvc_TimesOutWhenActorBlocked(t *testing.T) {
	t.Parallel()
	queue := make(chan *actor.OpenSessionRequest, 1) // capacity 1, never drained
	svc, err := New(Config{Queue: queue})
	if err != nil {
		t.Fatal(err)
	}

	ev := makeEvent("blocked", "0x1", 100)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// First call enqueues but receives no response → ctx times out first.
	err = svc.HandleSessionStarted(ctx, ev)
	if err == nil {
		t.Fatalf("expected error when actor blocked, got nil")
	}
}

func TestWeiToHumanString(t *testing.T) {
	one := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	cases := []struct {
		in   *big.Int
		want string
	}{
		{big.NewInt(0), "0"},
		{one, "1"},
		{new(big.Int).Mul(big.NewInt(500), one), "500"},
		{new(big.Int).Add(one, big.NewInt(500_000_000_000_000_000)), "1.5"}, // 1.5
	}
	for _, c := range cases {
		got := weiToHumanString(c.in)
		if got != c.want {
			t.Errorf("weiToHumanString(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}
