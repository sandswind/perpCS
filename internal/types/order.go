package types

// Order is a placed order travelling through the matching engine.
// Fields are documented in execution-order: writer fills them progressively.
type Order struct {
	ID       OrderID     `json:"id"`
	Symbol   Symbol      `json:"symbol"`
	Side     Side        `json:"side"`
	Type     OrderType   `json:"type"`
	Price    Price       `json:"price"` // 0 for market orders
	Quantity Qty         `json:"quantity"`
	Filled   Qty         `json:"filled"`
	TS       int64       `json:"ts"` // chaos clock ns
	Source   OrderSource `json:"source"`
	Owner    string      `json:"owner"` // "replay-mm-buy", session-id, etc.
}

// Remaining is the unfilled quantity.
func (o *Order) Remaining() Qty { return o.Quantity - o.Filled }

// IsFullyFilled reports whether Filled >= Quantity.
func (o *Order) IsFullyFilled() bool { return o.Filled >= o.Quantity }

// Validate performs sanity checks. Returns nil if order is well-formed.
func (o *Order) Validate() error {
	if o.Side != SideBuy && o.Side != SideSell {
		return ErrUnknownSide
	}
	if o.Quantity <= 0 {
		return ErrInvalidQty
	}
	if o.Type == OrderTypeLimit && o.Price <= 0 {
		return ErrInvalidPrice
	}
	if o.Type != OrderTypeLimit && o.Type != OrderTypeMarket {
		return ErrInvalidOrder
	}
	return nil
}

// Trade is an execution that resulted from matching two orders.
// Maker is the resting side, Taker is the aggressive side.
type Trade struct {
	ID         TradeID `json:"id"`
	Symbol     Symbol  `json:"symbol"`
	Price      Price   `json:"price"`
	Quantity   Qty     `json:"quantity"`
	MakerID    OrderID `json:"maker_id"`
	TakerID    OrderID `json:"taker_id"`
	MakerOwner string  `json:"maker_owner"`
	TakerOwner string  `json:"taker_owner"`
	TakerSide  Side    `json:"taker_side"`
	TS         int64   `json:"ts"`
}
