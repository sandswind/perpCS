package types

import "encoding/json"

// EventType is the discriminator for events written to events.jsonl.
type EventType string

const (
	EventOrderPlaced    EventType = "order_placed"
	EventOrderCancelled EventType = "order_cancelled"
	EventTrade          EventType = "trade"
	EventTickAdvance    EventType = "tick_advance"
	EventBookSnapshot   EventType = "book_snapshot"
	EventSessionStart   EventType = "session_start"
	EventSessionEnd     EventType = "session_end"
)

// Event is a tagged union written to the audit log. The Payload field
// is opaque JSON whose schema depends on Type.
//
// JSON field order is fixed (struct order) so output is byte-deterministic
// when Payload is itself produced by encoding/json on a struct (no maps).
type Event struct {
	Seq     uint64          `json:"seq"`
	TS      int64           `json:"ts"`
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Payload helpers — encode small structs deterministically.

type OrderPlacedPayload struct {
	Order Order `json:"order"`
}

type OrderCancelledPayload struct {
	OrderID OrderID `json:"order_id"`
	Reason  string  `json:"reason"`
}

type TradePayload struct {
	Trade Trade `json:"trade"`
}

type TickAdvancePayload struct {
	NewTS int64 `json:"new_ts"`
}

type BookSnapshotPayload struct {
	Snapshot BookSnapshot `json:"snapshot"`
}

type SessionStartPayload struct {
	Symbol Symbol `json:"symbol"`
	Level  string `json:"level"`
	Seed   uint64 `json:"seed"`
}

type SessionEndPayload struct {
	TotalTrades int   `json:"total_trades"`
	FinalTS     int64 `json:"final_ts"`
}
