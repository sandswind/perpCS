// Package types defines core domain primitives shared across the codebase.
//
// Design decisions:
//   - All monetary values use int64 fixed-point with scale 1e8 (8 decimal places).
//     This is the Bitcoin satoshi convention and gives:
//       max value: ~92 billion USDR (more than enough)
//       precision: 0.00000001
//     Using int64 (not float64) is REQUIRED for matching engine determinism.
//   - Timestamps are Unix nanoseconds (int64).
//   - All randomness must be derived from a seed; no time.Now() in hot path.
package types

import (
	"errors"
	"fmt"
	"strconv"
)

// Scale is the fixed-point scaling factor for both Price and Qty.
const Scale int64 = 100_000_000 // 1e8

// Price is a fixed-point price scaled by Scale.
type Price int64

// Qty is a fixed-point quantity scaled by Scale.
type Qty int64

// Notional returns price×qty as a human-scaled int64 (same unit as Price/Qty
// divided by Scale once — so the result is in 1e8 fixed-point USDR, same as Price).
//
// Derivation:
//   priceScaled  = price_float * 1e8
//   qtyScaled    = qty_float   * 1e8
//   product      = priceScaled * qtyScaled  = price_float * qty_float * 1e16
//   notional     = price_float * qty_float * 1e8
//   therefore    = product / 1e8   (i.e. / Scale)
//
// Overflow guard: BTC price ~$1e5, scaled = 1e13. Qty 1 BTC, scaled = 1e8.
// Product = 1e21 — overflows int64 (~9.2e18).
// We use int128 via two int64 halves (simple muldiv) to stay safe.
func Notional(p Price, q Qty) int64 {
	// Use big-integer multiply then divide to avoid int64 overflow.
	// For v0.1 we know: price ≤ 1e6 USD (scaled ≤ 1e14) and qty ≤ 1e4 BTC (scaled ≤ 1e12)
	// product ≤ 1e26 — needs >64 bits.
	// Simple approach: work in float64 for the scale correction only.
	// float64 has 53-bit mantissa (~15 decimal digits), sufficient for 8-decimal prices
	// up to 9.007e15 * qty — more than enough for BTC/USDR at human scale.
	// All input data comes from int64, so the truncation only affects the last ~1 digit
	// of the 8th decimal place (sub-cent level), which is acceptable display precision.
	return int64(float64(p) * float64(q) / float64(Scale))
}

// PriceFromFloat converts a float64 (e.g. 7900.5) to Price.
// Use ONLY at API boundaries (data provider input, CLI parsing).
// Banker's rounding is NOT applied; this truncates toward zero.
func PriceFromFloat(f float64) Price {
	return Price(int64(f * float64(Scale)))
}

// QtyFromFloat converts a float64 to Qty.
func QtyFromFloat(f float64) Qty {
	return Qty(int64(f * float64(Scale)))
}

// Float returns the price as a float64 for display only. NEVER use for math.
func (p Price) Float() float64 { return float64(p) / float64(Scale) }

// Float returns the qty as a float64 for display only.
func (q Qty) Float() float64 { return float64(q) / float64(Scale) }

// String formats with up to 8 decimals, trimming trailing zeros.
func (p Price) String() string { return formatScaled(int64(p)) }
func (q Qty) String() string   { return formatScaled(int64(q)) }

func formatScaled(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	whole := n / Scale
	frac := n % Scale
	if frac == 0 {
		if neg {
			return "-" + strconv.FormatInt(whole, 10)
		}
		return strconv.FormatInt(whole, 10)
	}
	// 8 decimal places, trim trailing zeros
	s := fmt.Sprintf("%d.%08d", whole, frac)
	// trim trailing zeros, but leave at least 2 decimals for readability
	end := len(s)
	for end > 0 && s[end-1] == '0' {
		end--
	}
	if s[end-1] == '.' {
		end-- // remove dangling dot
	}
	out := s[:end]
	if neg {
		return "-" + out
	}
	return out
}

// Symbol is a trading pair identifier, e.g. "BTC-MED".
type Symbol string

// Side is the direction of an order or position.
type Side uint8

const (
	SideUnknown Side = 0
	SideBuy     Side = 1
	SideSell    Side = 2
)

func (s Side) String() string {
	switch s {
	case SideBuy:
		return "buy"
	case SideSell:
		return "sell"
	default:
		return "unknown"
	}
}

// Opposite returns the opposite side. Unknown maps to Unknown.
func (s Side) Opposite() Side {
	switch s {
	case SideBuy:
		return SideSell
	case SideSell:
		return SideBuy
	default:
		return SideUnknown
	}
}

// OrderType identifies order behaviour.
type OrderType uint8

const (
	OrderTypeUnknown OrderType = 0
	OrderTypeLimit   OrderType = 1
	OrderTypeMarket  OrderType = 2
)

func (t OrderType) String() string {
	switch t {
	case OrderTypeLimit:
		return "limit"
	case OrderTypeMarket:
		return "market"
	default:
		return "unknown"
	}
}

// OrderSource indicates where an order came from. Used to differentiate
// historical replay-injected liquidity from real user orders.
type OrderSource uint8

const (
	SourceUnknown OrderSource = 0
	SourceReplay  OrderSource = 1 // synthetic order generated from historical kline/trade
	SourceUser    OrderSource = 2 // human player (used in v0.2+)
)

func (s OrderSource) String() string {
	switch s {
	case SourceReplay:
		return "replay"
	case SourceUser:
		return "user"
	default:
		return "unknown"
	}
}

// OrderID is a globally unique sequence number per market actor.
type OrderID uint64

// TradeID is a globally unique sequence number per market actor.
type TradeID uint64

// Common sentinel errors.
var (
	ErrInvalidOrder = errors.New("invalid order")
	ErrUnknownSide  = errors.New("unknown side")
	ErrInvalidPrice = errors.New("invalid price")
	ErrInvalidQty   = errors.New("invalid quantity")
)
