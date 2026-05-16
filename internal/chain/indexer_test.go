package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRPC is a tiny test double for the parts of JSON-RPC the indexer uses.
type fakeRPC struct {
	mu    sync.Mutex
	head  uint64
	logs  map[uint64][]Log // logs by block number
	gets  atomic.Uint64    // call counters for assertions
	heads atomic.Uint64
}

func (f *fakeRPC) setHead(n uint64) {
	f.mu.Lock()
	f.head = n
	f.mu.Unlock()
}

func (f *fakeRPC) addLog(blockNum uint64, l Log) {
	f.mu.Lock()
	if f.logs == nil {
		f.logs = map[uint64][]Log{}
	}
	l.BlockNumber = fmt.Sprintf("0x%x", blockNum)
	f.logs[blockNum] = append(f.logs[blockNum], l)
	f.mu.Unlock()
}

func (f *fakeRPC) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			ID     uint64          `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "eth_blockNumber":
			f.heads.Add(1)
			f.mu.Lock()
			head := f.head
			f.mu.Unlock()
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":"0x%x"}`, req.ID, head)

		case "eth_getLogs":
			f.gets.Add(1)
			var params []struct {
				FromBlock string `json:"fromBlock"`
				ToBlock   string `json:"toBlock"`
			}
			_ = json.Unmarshal(req.Params, &params)
			from, _ := parseHexUint64(params[0].FromBlock)
			to, _ := parseHexUint64(params[0].ToBlock)

			f.mu.Lock()
			var out []Log
			for b := from; b <= to; b++ {
				out = append(out, f.logs[b]...)
			}
			f.mu.Unlock()
			body, _ := json.Marshal(out)
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, req.ID, body)

		default:
			http.Error(w, "unknown method "+req.Method, 400)
		}
	}
}

func makeSessionLog(blockNum, logIdx uint64, sessionID string) Log {
	return Log{
		Address: "0xvault",
		Topics: []string{
			SessionStartedTopic,
			"0x000000000000000000000000328809bc894f92807417d2dad6b7c998c1afdac6",
			"0x8a94c9452e5b27b050f1cf8886942611d6ce7d556be4b80396d14bf70a2560fe",
			sessionID,
		},
		Data: "0x" +
			"00000000000000000000000000000000000000000000001b1ae4d6e2ef500000" + // amount
			"0000000000000000000000000000000000000000000000000000000000000001" + // nonce
			fmt.Sprintf("%064x", blockNum), // blockNumber
		BlockNumber:     fmt.Sprintf("0x%x", blockNum),
		TransactionHash: fmt.Sprintf("0xtx%d", blockNum),
		LogIndex:        fmt.Sprintf("0x%x", logIdx),
	}
}

func TestIndexer_RespectsConfirmationDepth(t *testing.T) {
	t.Parallel()
	f := &fakeRPC{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	// Logs at blocks 100 and 102. Head = 105, confirmations = 5.
	// confirmed = 100 → only the block-100 log should be dispatched.
	f.addLog(100, makeSessionLog(100, 0, "0x"+paddedHex32("s100")))
	f.addLog(102, makeSessionLog(102, 0, "0x"+paddedHex32("s102")))
	f.setHead(105)

	var got []*SessionStartedEvent
	var mu sync.Mutex
	handler := HandlerFunc(func(_ context.Context, ev *SessionStartedEvent) error {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
		return nil
	})

	dir := t.TempDir()
	idx, err := New(Config{
		Client:        NewClient(srv.URL, nil),
		VaultAddress:  "0xvault",
		StartBlock:    1,
		StatePath:     filepath.Join(dir, "state.json"),
		Confirmations: 5,
		PollInterval:  10 * time.Millisecond,
		Handler:       handler,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = idx.Run(ctx)

	mu.Lock()
	gotLen := len(got)
	var firstBlock uint64
	if gotLen >= 1 {
		firstBlock = got[0].BlockNumber.Uint64()
	}
	mu.Unlock()

	if gotLen != 1 {
		t.Fatalf("expected 1 dispatched event (block 100), got %d", gotLen)
	}
	if firstBlock != 100 {
		t.Errorf("dispatched wrong block: %d", firstBlock)
	}
	if idx.LastProcessed() != 100 {
		t.Errorf("LastProcessed = %d, want 100", idx.LastProcessed())
	}

	// Now advance head so block 102 is confirmed.
	f.setHead(110)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	_ = idx.Run(ctx2)

	mu.Lock()
	gotLen = len(got)
	var secondBlock uint64
	if gotLen >= 2 {
		secondBlock = got[1].BlockNumber.Uint64()
	}
	mu.Unlock()
	if gotLen != 2 {
		t.Fatalf("expected 2 dispatched events after head advance, got %d", gotLen)
	}
	if secondBlock != 102 {
		t.Errorf("second dispatched wrong block: %d", secondBlock)
	}
}

func TestIndexer_StatePersistsAcrossRestarts(t *testing.T) {
	t.Parallel()
	f := &fakeRPC{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	f.addLog(50, makeSessionLog(50, 0, "0x"+paddedHex32("s50")))
	f.setHead(100)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	var dispatched int
	handler := HandlerFunc(func(_ context.Context, _ *SessionStartedEvent) error {
		dispatched++
		return nil
	})

	// First run: process up to block 95 (head 100 - 5 confirmations).
	idx, err := New(Config{
		Client:        NewClient(srv.URL, nil),
		VaultAddress:  "0xvault",
		StartBlock:    1,
		StatePath:     statePath,
		Confirmations: 5,
		PollInterval:  10 * time.Millisecond,
		Handler:       handler,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = idx.Run(ctx)

	if dispatched != 1 {
		t.Fatalf("first run dispatched=%d, want 1", dispatched)
	}
	if idx.LastProcessed() != 95 {
		t.Errorf("first run LastProcessed=%d", idx.LastProcessed())
	}

	// Add a new log AT block 50 (already processed) — must NOT be re-dispatched.
	// And add one at block 96 (still not confirmed at head=100, conf=5: only 95 is confirmed).
	f.addLog(96, makeSessionLog(96, 0, "0x"+paddedHex32("s96")))

	// Second run: same state file → should resume at block 96 and only see
	// confirmed blocks (none new yet at head 100).
	idx2, err := New(Config{
		Client:        NewClient(srv.URL, nil),
		VaultAddress:  "0xvault",
		StartBlock:    1, // ignored because state file exists
		StatePath:     statePath,
		Confirmations: 5,
		PollInterval:  10 * time.Millisecond,
		Handler:       handler,
	})
	if err != nil {
		t.Fatal(err)
	}
	if idx2.LastProcessed() != 95 {
		t.Errorf("second run did not resume from state: LastProcessed=%d", idx2.LastProcessed())
	}

	// No new confirmed logs yet.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	_ = idx2.Run(ctx2)
	if dispatched != 1 {
		t.Errorf("dispatched=%d after second run; should still be 1 (block 96 not confirmed)", dispatched)
	}

	// Advance head; now 96 is confirmed.
	f.setHead(101)
	ctx3, cancel3 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel3()
	_ = idx2.Run(ctx3)
	if dispatched != 2 {
		t.Errorf("dispatched=%d after head advance; want 2", dispatched)
	}
}

func TestIndexer_ChunksLargeRanges(t *testing.T) {
	t.Parallel()
	f := &fakeRPC{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	f.setHead(5_500)

	dir := t.TempDir()
	idx, err := New(Config{
		Client:        NewClient(srv.URL, nil),
		VaultAddress:  "0xvault",
		StartBlock:    1,
		StatePath:     filepath.Join(dir, "state.json"),
		Confirmations: 1, // process up to head-1
		MaxRange:      1_000,
		PollInterval:  10 * time.Millisecond,
		Handler:       HandlerFunc(func(_ context.Context, _ *SessionStartedEvent) error { return nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = idx.Run(ctx)

	if idx.LastProcessed() != 5_499 {
		t.Errorf("LastProcessed = %d, want 5499", idx.LastProcessed())
	}
	// Need ≥ 6 calls to cover [1..5499] in 1000-block chunks.
	if f.gets.Load() < 6 {
		t.Errorf("expected ≥6 eth_getLogs calls, got %d", f.gets.Load())
	}
}
