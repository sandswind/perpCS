package fanout

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sandswind/perpCS/internal/types"
)

// makeBookSnapshotEvent builds a types.Event of type EventBookSnapshot.
func makeBookSnapshotEvent(symbol string) types.Event {
	snap := types.BookSnapshot{
		Symbol: types.Symbol(symbol),
		TS:     1_000_000,
		Bids: []types.PriceLevel{
			{Price: types.PriceFromFloat(7900), Quantity: types.QtyFromFloat(1.0)},
			{Price: types.PriceFromFloat(7890), Quantity: types.QtyFromFloat(2.0)},
		},
		Asks: []types.PriceLevel{
			{Price: types.PriceFromFloat(7910), Quantity: types.QtyFromFloat(1.5)},
			{Price: types.PriceFromFloat(7920), Quantity: types.QtyFromFloat(0.5)},
		},
	}
	payload, _ := json.Marshal(types.BookSnapshotPayload{Snapshot: snap})
	return types.Event{
		Seq:     1,
		Type:    types.EventBookSnapshot,
		Payload: payload,
	}
}

// makeTradeEvent builds a types.Event of type EventTrade.
func makeTradeEvent(symbol, takerOwner string) types.Event {
	t := types.Trade{
		ID:         1,
		Symbol:     types.Symbol(symbol),
		Price:      types.PriceFromFloat(7850),
		Quantity:   types.QtyFromFloat(0.1),
		TakerOwner: takerOwner,
		TakerSide:  types.SideBuy,
		TS:         2_000_000,
	}
	payload, _ := json.Marshal(types.TradePayload{Trade: t})
	return types.Event{
		Seq:     2,
		Type:    types.EventTrade,
		Payload: payload,
	}
}

// recvWithTimeout reads one message from conn.send with a timeout.
func recvWithTimeout(c *Conn, d time.Duration) ([]byte, bool) {
	select {
	case msg := <-c.send:
		return msg, true
	case <-time.After(d):
		return nil, false
	}
}

// TestFanout_BookSnapshot verifies that a subscriber receives a book_snapshot message
// after Emit of an EventBookSnapshot event.
func TestFanout_BookSnapshot(t *testing.T) {
	f := New()
	conn := f.Subscribe("market", "BTC-MED")
	defer f.Unsubscribe("market", "BTC-MED", conn)

	evt := makeBookSnapshotEvent("BTC-MED")
	if err := f.Emit(evt); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	msg, ok := recvWithTimeout(conn, 100*time.Millisecond)
	if !ok {
		t.Fatal("timed out waiting for book_snapshot message")
	}

	var envelope wsMessage
	if err := json.Unmarshal(msg, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Type != "book_snapshot" {
		t.Errorf("expected type=book_snapshot, got %q", envelope.Type)
	}

	var snap bookSnapshotMsg
	if err := json.Unmarshal(envelope.Data, &snap); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if snap.Symbol != "BTC-MED" {
		t.Errorf("expected symbol BTC-MED, got %q", snap.Symbol)
	}
	if len(snap.Bids) != 2 {
		t.Errorf("expected 2 bids, got %d", len(snap.Bids))
	}
	if len(snap.Asks) != 2 {
		t.Errorf("expected 2 asks, got %d", len(snap.Asks))
	}
	t.Logf("book_snapshot: symbol=%s bids=%d asks=%d", snap.Symbol, len(snap.Bids), len(snap.Asks))
}

// TestFanout_Trade verifies that a market subscriber receives a trade message
// after Emit of an EventTrade event.
func TestFanout_Trade(t *testing.T) {
	f := New()
	conn := f.Subscribe("market", "BTC-MED")
	defer f.Unsubscribe("market", "BTC-MED", conn)

	evt := makeTradeEvent("BTC-MED", "")
	if err := f.Emit(evt); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	msg, ok := recvWithTimeout(conn, 100*time.Millisecond)
	if !ok {
		t.Fatal("timed out waiting for trade message")
	}

	var envelope wsMessage
	if err := json.Unmarshal(msg, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Type != "trade" {
		t.Errorf("expected type=trade, got %q", envelope.Type)
	}

	var tm tradeMsg
	if err := json.Unmarshal(envelope.Data, &tm); err != nil {
		t.Fatalf("unmarshal trade: %v", err)
	}
	if tm.Side != "buy" {
		t.Errorf("expected side=buy, got %q", tm.Side)
	}
	t.Logf("trade: price=%s qty=%s side=%s", tm.Price, tm.Qty, tm.Side)
}

// TestFanout_Unsubscribe verifies that after Unsubscribe, no messages are delivered.
func TestFanout_Unsubscribe(t *testing.T) {
	f := New()
	conn := f.Subscribe("market", "BTC-MED")

	// Unsubscribe before emitting
	f.Unsubscribe("market", "BTC-MED", conn)

	evt := makeBookSnapshotEvent("BTC-MED")
	if err := f.Emit(evt); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// The conn's done channel should be closed
	select {
	case <-conn.done:
		// expected
	case <-time.After(10 * time.Millisecond):
		t.Error("expected done channel to be closed after Unsubscribe")
	}

	// No message should arrive on send
	select {
	case <-conn.send:
		t.Error("received message after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// expected: no message
	}
}

// TestFanout_MultipleSubscribers verifies all subscribers receive the same message.
func TestFanout_MultipleSubscribers(t *testing.T) {
	f := New()
	conn1 := f.Subscribe("market", "BTC-MED")
	conn2 := f.Subscribe("market", "BTC-MED")
	defer f.Unsubscribe("market", "BTC-MED", conn1)
	defer f.Unsubscribe("market", "BTC-MED", conn2)

	evt := makeBookSnapshotEvent("BTC-MED")
	if err := f.Emit(evt); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	msg1, ok1 := recvWithTimeout(conn1, 100*time.Millisecond)
	msg2, ok2 := recvWithTimeout(conn2, 100*time.Millisecond)

	if !ok1 {
		t.Error("conn1: timed out")
	}
	if !ok2 {
		t.Error("conn2: timed out")
	}
	if ok1 && ok2 {
		if string(msg1) != string(msg2) {
			t.Errorf("messages differ:\n  conn1: %s\n  conn2: %s", msg1, msg2)
		}
		t.Logf("both subscribers received identical message (%d bytes)", len(msg1))
	}
}

// TestFanout_AccountFill verifies that a trade with a taker owner also sends
// a fill message to the account subscriber.
func TestFanout_AccountFill(t *testing.T) {
	f := New()
	marketConn := f.Subscribe("market", "BTC-MED")
	accountConn := f.Subscribe("account", "demo-session")
	defer f.Unsubscribe("market", "BTC-MED", marketConn)
	defer f.Unsubscribe("account", "demo-session", accountConn)

	evt := makeTradeEvent("BTC-MED", "demo-session")
	if err := f.Emit(evt); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// Both market and account should receive messages
	mktMsg, mktOk := recvWithTimeout(marketConn, 100*time.Millisecond)
	accMsg, accOk := recvWithTimeout(accountConn, 100*time.Millisecond)

	if !mktOk {
		t.Error("market subscriber: timed out")
	}
	if !accOk {
		t.Error("account subscriber: timed out")
	}

	if mktOk {
		var env wsMessage
		_ = json.Unmarshal(mktMsg, &env)
		if env.Type != "trade" {
			t.Errorf("market: expected trade, got %q", env.Type)
		}
	}
	if accOk {
		var env wsMessage
		_ = json.Unmarshal(accMsg, &env)
		if env.Type != "fill" {
			t.Errorf("account: expected fill, got %q", env.Type)
		}
		t.Logf("fill message: %s", accMsg)
	}
}
