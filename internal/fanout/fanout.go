// Package fanout implements a WebSocket fan-out hub that bridges the MarketActor
// event stream to browser clients.
//
// Design rules:
//   - Emit() must never block the actor goroutine: send channels are buffered (128).
//     If a client's buffer is full, the message is dropped silently.
//   - Each *Conn owns its goroutine (writePump) for gorilla/websocket write safety.
//   - Subscribe / Unsubscribe are protected by a RWMutex; Emit takes a read lock
//     to iterate, so concurrent subscribers are fine.
package fanout

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sandswind/perpCS/internal/types"
)

const (
	sendBufSize    = 128
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 50 * time.Second // < pongWait
	maxMessageSize = 512 * 1024
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(_ *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 64 * 1024,
}

// Conn represents one WebSocket subscriber connection.
type Conn struct {
	send chan []byte
	done chan struct{}
}

// Fanout distributes actor events to WebSocket subscribers.
type Fanout struct {
	mu          sync.RWMutex
	marketSubs  map[string]map[*Conn]struct{} // symbol → conns
	accountSubs map[string]map[*Conn]struct{} // sessionID → conns
}

// New creates a new Fanout hub.
func New() *Fanout {
	return &Fanout{
		marketSubs:  make(map[string]map[*Conn]struct{}),
		accountSubs: make(map[string]map[*Conn]struct{}),
	}
}

// Subscribe registers a new connection for the given kind ("market"|"account") and key.
func (f *Fanout) Subscribe(kind, key string) *Conn {
	c := &Conn{
		send: make(chan []byte, sendBufSize),
		done: make(chan struct{}),
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch kind {
	case "market":
		if f.marketSubs[key] == nil {
			f.marketSubs[key] = make(map[*Conn]struct{})
		}
		f.marketSubs[key][c] = struct{}{}
	case "account":
		if f.accountSubs[key] == nil {
			f.accountSubs[key] = make(map[*Conn]struct{})
		}
		f.accountSubs[key][c] = struct{}{}
	}
	return c
}

// Unsubscribe removes a connection from the given kind/key group and closes its channels.
func (f *Fanout) Unsubscribe(kind, key string, c *Conn) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch kind {
	case "market":
		delete(f.marketSubs[key], c)
	case "account":
		delete(f.accountSubs[key], c)
	}
	select {
	case <-c.done:
		// already closed
	default:
		close(c.done)
	}
}

// wsMessage is the envelope sent over WebSocket.
type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func makeMsg(typ string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wsMessage{Type: typ, Data: data})
}

// bookSnapshotMsg is the payload for book_snapshot WS messages.
type bookSnapshotMsg struct {
	Symbol string            `json:"symbol"`
	TS     int64             `json:"ts"`
	Bids   []priceLevelMsg   `json:"bids"`
	Asks   []priceLevelMsg   `json:"asks"`
}

type priceLevelMsg struct {
	Price string `json:"price"`
	Qty   string `json:"qty"`
}

// tradeMsg is the payload for trade WS messages.
type tradeMsg struct {
	Price string `json:"price"`
	Qty   string `json:"qty"`
	Side  string `json:"side"`
	TS    int64  `json:"ts"`
}

// fillMsg is the payload for fill WS messages (account channel).
type fillMsg struct {
	OrderID uint64 `json:"order_id"`
	Price   string `json:"price"`
	Qty     string `json:"qty"`
	PnL     string `json:"pnl"`
}

// Emit implements actor.EventSink. It fans out events to subscribers.
// This method must not block — drops silently if a subscriber's buffer is full.
func (f *Fanout) Emit(e types.Event) error {
	switch e.Type {
	case types.EventBookSnapshot:
		return f.emitBookSnapshot(e)
	case types.EventTrade:
		return f.emitTrade(e)
	}
	return nil
}

func (f *Fanout) emitBookSnapshot(e types.Event) error {
	var payload types.BookSnapshotPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return err
	}
	snap := payload.Snapshot

	bids := make([]priceLevelMsg, len(snap.Bids))
	for i, b := range snap.Bids {
		bids[i] = priceLevelMsg{Price: b.Price.String(), Qty: b.Quantity.String()}
	}
	asks := make([]priceLevelMsg, len(snap.Asks))
	for i, a := range snap.Asks {
		asks[i] = priceLevelMsg{Price: a.Price.String(), Qty: a.Quantity.String()}
	}

	msg, err := makeMsg("book_snapshot", bookSnapshotMsg{
		Symbol: string(snap.Symbol),
		TS:     snap.TS,
		Bids:   bids,
		Asks:   asks,
	})
	if err != nil {
		return err
	}

	f.broadcast("market", string(snap.Symbol), msg)
	return nil
}

func (f *Fanout) emitTrade(e types.Event) error {
	var payload types.TradePayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return err
	}
	t := payload.Trade

	msg, err := makeMsg("trade", tradeMsg{
		Price: t.Price.String(),
		Qty:   t.Quantity.String(),
		Side:  t.TakerSide.String(),
		TS:    t.TS,
	})
	if err != nil {
		return err
	}

	// Broadcast to all market subscribers for this symbol
	f.broadcast("market", string(t.Symbol), msg)

	// Also send fill notification to the taker's account channel
	if t.TakerOwner != "" {
		fillData, ferr := makeMsg("fill", fillMsg{
			OrderID: uint64(t.TakerID),
			Price:   t.Price.String(),
			Qty:     t.Quantity.String(),
			PnL:     "0", // realised PnL computed by account; v0.3 sends 0
		})
		if ferr == nil {
			f.broadcast("account", t.TakerOwner, fillData)
		}
	}
	return nil
}

// broadcast sends msg to all subscribers of kind/key, dropping if buffer full.
func (f *Fanout) broadcast(kind, key string, msg []byte) {
	f.mu.RLock()
	var subs map[*Conn]struct{}
	switch kind {
	case "market":
		subs = f.marketSubs[key]
	case "account":
		subs = f.accountSubs[key]
	}
	// Copy conn set to avoid holding lock during sends
	conns := make([]*Conn, 0, len(subs))
	for c := range subs {
		conns = append(conns, c)
	}
	f.mu.RUnlock()

	for _, c := range conns {
		select {
		case c.send <- msg:
		default:
			// buffer full — drop
		}
	}
}

// ServeMarket returns an http.Handler that upgrades to WebSocket and streams
// market data (book_snapshot + trade) for the given symbol.
func (f *Fanout) ServeMarket(symbol string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.serveWS(w, r, "market", symbol)
	})
}

// ServeAccount returns an http.Handler that upgrades to WebSocket and streams
// account events (fill + position) for the given session ID.
func (f *Fanout) ServeAccount(sessionID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.serveWS(w, r, "account", sessionID)
	})
}

func (f *Fanout) serveWS(w http.ResponseWriter, r *http.Request, kind, key string) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	conn := f.Subscribe(kind, key)
	defer f.Unsubscribe(kind, key, conn)

	// Read pump: discard messages, handle pong & close
	go func() {
		defer ws.Close()
		ws.SetReadLimit(maxMessageSize)
		_ = ws.SetReadDeadline(time.Now().Add(pongWait))
		ws.SetPongHandler(func(_ string) error {
			return ws.SetReadDeadline(time.Now().Add(pongWait))
		})
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				break
			}
		}
		// Signal write pump to stop
		select {
		case <-conn.done:
		default:
			close(conn.done)
		}
	}()

	// Write pump: drain send channel
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-conn.done:
			_ = ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(writeWait))
			return
		case msg, ok := <-conn.send:
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				return
			}
			if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
