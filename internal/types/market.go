package types

// Kline is an OHLCV candle in a fixed time window. Provider-agnostic.
type Kline struct {
	Symbol     Symbol `json:"symbol"`
	OpenTS     int64  `json:"open_ts"`  // start ns (inclusive)
	CloseTS    int64  `json:"close_ts"` // end ns (exclusive)
	Open       Price  `json:"open"`
	High       Price  `json:"high"`
	Low        Price  `json:"low"`
	Close      Price  `json:"close"`
	Volume     Qty    `json:"volume"`      // base asset volume
	QuoteVol   int64  `json:"quote_vol"`   // quote asset volume in Scale (price*qty)
	NumTrades  int32  `json:"num_trades"`
	TakerBuyVol Qty   `json:"taker_buy_vol"` // taker-buy base volume (informational)
}

// AggTrade is a normalized aggregated public trade from a venue.
// Mostly used by data providers; matching engine uses Trade above.
type AggTrade struct {
	Symbol    Symbol `json:"symbol"`
	TS        int64  `json:"ts"`
	Price     Price  `json:"price"`
	Quantity  Qty    `json:"quantity"`
	TakerSide Side   `json:"taker_side"` // who paid the spread
}

// PriceLevel represents one price tier of an order book snapshot.
type PriceLevel struct {
	Price    Price `json:"price"`
	Quantity Qty   `json:"qty"`
}

// BookSnapshot is an L2 order book snapshot (bids descending, asks ascending).
type BookSnapshot struct {
	Symbol Symbol       `json:"symbol"`
	TS     int64        `json:"ts"`
	Bids   []PriceLevel `json:"bids"`
	Asks   []PriceLevel `json:"asks"`
}

// BestBidAsk is a lightweight top-of-book quote.
type BestBidAsk struct {
	Symbol  Symbol `json:"symbol"`
	TS      int64  `json:"ts"`
	BidPx   Price  `json:"bid_px"`
	BidQty  Qty    `json:"bid_qty"`
	AskPx   Price  `json:"ask_px"`
	AskQty  Qty    `json:"ask_qty"`
}

// Spread returns ask - bid; negative if crossed.
func (b BestBidAsk) Spread() Price { return b.AskPx - b.BidPx }
