// Package account implements the isolated-margin AccountStateMachine for a single player.
//
// Design rules (CRITICAL):
//   - Account is accessed ONLY from the MarketActor goroutine — no mutex needed.
//   - All monetary values are int64 fixed-point (scale 1e8), matching types.Price/Qty.
//   - No float64 in the hot path; use types.Notional() for price×qty multiplication.
//   - Timestamps come from the chaos clock; no time.Now() here.
package account

import (
	"errors"
	"fmt"

	"github.com/sandswind/perpCS/internal/types"
)

// ErrInsufficientBalance is returned when a fill would put the balance below zero.
var ErrInsufficientBalance = errors.New("insufficient balance")

// Position holds an open isolated-margin position for one symbol.
type Position struct {
	Symbol   types.Symbol
	Side     types.Side
	Size     types.Qty   // current position size (always positive)
	AvgEntry types.Price // average entry price
	Margin   types.Qty   // isolated margin allocated (1e8 USDR)
	UPnL     types.Qty   // unrealised PnL, updated each MarkToMarket call (can be negative)
}

// Account is the player's isolated-margin account.
// It is NOT thread-safe; the caller (MarketActor) must ensure single-goroutine access.
type Account struct {
	Address    string
	SessionID  string
	Balance    types.Qty // available balance (initial deposit minus allocated margins)
	Positions  map[types.Symbol]*Position
	OpenOrders map[types.OrderID]*types.Order
}

// New creates a new Account with the given initial balance.
func New(address, sessionID string, initialBalance types.Qty) *Account {
	return &Account{
		Address:    address,
		SessionID:  sessionID,
		Balance:    initialBalance,
		Positions:  make(map[types.Symbol]*Position),
		OpenOrders: make(map[types.OrderID]*types.Order),
	}
}

// ApplyFill updates the account state after a fill (trade).
//
// For opening/increasing a position:
//   - Computes required margin = notional / leverage (leverage=1 for now, i.e. full margin)
//   - Deducts margin from Balance
//   - Updates AvgEntry and Size
//
// For closing/reducing a position (opposite side to current position):
//   - Computes realised PnL
//   - Returns margin proportionally, adds realised PnL to Balance
//   - Reduces Size; if fully closed, removes the Position entry
//
// markPrice is used for margin calculation on opens.
func (a *Account) ApplyFill(fill types.Trade, order *types.Order, markPrice types.Price) error {
	sym := fill.Symbol
	pos, exists := a.Positions[sym]

	// Determine taker side from the fill
	takerSide := fill.TakerSide
	fillQty := fill.Quantity
	fillPrice := fill.Price

	if !exists || pos.Size == 0 {
		// Opening a new position
		// Required margin = notional of fill (full isolated margin, leverage=1x)
		notional := types.Qty(types.Notional(fillPrice, fillQty))
		if notional < 0 {
			notional = -notional
		}
		if a.Balance < notional {
			return fmt.Errorf("%w: need %s have %s", ErrInsufficientBalance, notional.String(), a.Balance.String())
		}
		a.Balance -= notional
		a.Positions[sym] = &Position{
			Symbol:   sym,
			Side:     takerSide,
			Size:     fillQty,
			AvgEntry: fillPrice,
			Margin:   notional,
		}
		return nil
	}

	// Position already exists
	if pos.Side == takerSide {
		// Increasing position (same direction)
		existingNotional := types.Qty(types.Notional(pos.AvgEntry, pos.Size))
		if existingNotional < 0 {
			existingNotional = -existingNotional
		}
		addNotional := types.Qty(types.Notional(fillPrice, fillQty))
		if addNotional < 0 {
			addNotional = -addNotional
		}
		if a.Balance < addNotional {
			return fmt.Errorf("%w: need %s have %s", ErrInsufficientBalance, addNotional.String(), a.Balance.String())
		}
		// Update avg entry: (existing_notional + new_notional) / (existing_size + new_size)
		totalSize := pos.Size + fillQty
		// AvgEntry = (AvgEntry*Size + fillPrice*fillQty) / totalSize
		// Use int64 arithmetic via Notional
		numerator := existingNotional + addNotional // both in 1e8 scaled USDR
		// numerator is in 1e8 USDR; divide by totalSize (1e8 qty) → price in 1e8
		newAvg := types.Price(types.Notional(types.Price(numerator), types.Qty(types.Scale)) / int64(totalSize) * int64(types.Scale))
		// Simpler: avg = (oldAvg*oldSize + newPrice*newQty) / newTotalSize
		newAvg = types.Price((int64(pos.AvgEntry)*int64(pos.Size) + int64(fillPrice)*int64(fillQty)) / int64(totalSize))

		a.Balance -= addNotional
		pos.AvgEntry = newAvg
		pos.Size = totalSize
		pos.Margin += addNotional
	} else {
		// Closing/reducing position (opposite direction)
		closeQty := fillQty
		if closeQty > pos.Size {
			closeQty = pos.Size // cap at current size (partial close)
		}

		// Realised PnL
		var realizedPnL types.Qty
		if pos.Side == types.SideBuy {
			// long: pnl = (fillPrice - avgEntry) * closeQty
			realizedPnL = types.Qty(types.Notional(fillPrice-pos.AvgEntry, closeQty))
		} else {
			// short: pnl = (avgEntry - fillPrice) * closeQty
			realizedPnL = types.Qty(types.Notional(pos.AvgEntry-fillPrice, closeQty))
		}

		// Return proportional margin: margin * (closeQty / posSize)
		// We cannot do int64*int64 directly (overflows), so use the Scale trick:
		// returnedMargin = margin * closeQty / posSize
		// = Notional(Price(margin), closeQty) / posSize * Scale
		// But Notional already divides by Scale once, so:
		// returnedMargin = Notional(Price(margin), closeQty) / (posSize/Scale)
		// Simplest safe approach: use float64 only for this ratio (acceptable precision).
		returnedMargin := types.Qty(float64(pos.Margin) * float64(closeQty) / float64(pos.Size))

		a.Balance += returnedMargin + realizedPnL

		if closeQty >= pos.Size {
			// Fully closed
			delete(a.Positions, sym)
		} else {
			pos.Size -= closeQty
			pos.Margin -= returnedMargin
			// AvgEntry unchanged on partial close
		}

		// If there was excess qty beyond the current position (flip), open reverse
		excess := fillQty - closeQty
		if excess > 0 {
			// Open a new position in the opposite direction with the excess
			newNotional := types.Qty(types.Notional(fillPrice, excess))
			if newNotional < 0 {
				newNotional = -newNotional
			}
			if a.Balance < newNotional {
				// Best-effort: truncate to available balance
				newNotional = a.Balance
			}
			a.Balance -= newNotional
			a.Positions[sym] = &Position{
				Symbol:   sym,
				Side:     takerSide,
				Size:     excess,
				AvgEntry: fillPrice,
				Margin:   newNotional,
			}
		}
	}

	_ = markPrice // used for margin checks in future leverage support
	return nil
}

// MarkToMarket updates the UPnL of the position for the given symbol.
// Must be called every tick with the current mark price.
//
// uPnL (long)  = size × (markPrice - avgEntry)
// uPnL (short) = size × (avgEntry  - markPrice)
func (a *Account) MarkToMarket(symbol types.Symbol, markPrice types.Price) {
	pos, ok := a.Positions[symbol]
	if !ok || pos.Size == 0 {
		return
	}
	if pos.Side == types.SideBuy {
		pos.UPnL = types.Qty(types.Notional(markPrice-pos.AvgEntry, pos.Size))
	} else {
		pos.UPnL = types.Qty(types.Notional(pos.AvgEntry-markPrice, pos.Size))
	}
}

// MarginRatio returns the overall account margin ratio:
//
//	marginRatio = (balance + Σ uPnL) / Σ positionNotional
//
// positionNotional = size × markPrice for each open position.
// Returns 0 if there are no open positions (undefined; caller should treat as safe).
// markPrices maps symbol → current mark price.
func (a *Account) MarginRatio(markPrices map[types.Symbol]types.Price) float64 {
	var totalUPnL types.Qty
	var totalNotional int64

	for sym, pos := range a.Positions {
		if pos.Size == 0 {
			continue
		}
		totalUPnL += pos.UPnL
		mp, ok := markPrices[sym]
		if !ok {
			mp = pos.AvgEntry // fallback: use entry price if no mark
		}
		totalNotional += types.Notional(mp, pos.Size)
	}

	if totalNotional == 0 {
		return 0
	}
	equity := int64(a.Balance) + int64(totalUPnL)
	return float64(equity) / float64(totalNotional)
}

// MarginRatioForSymbol computes the margin ratio for a single symbol.
// Returns 0 if there is no open position for that symbol.
func (a *Account) MarginRatioForSymbol(symbol types.Symbol, markPrice types.Price) float64 {
	pos, ok := a.Positions[symbol]
	if !ok || pos.Size == 0 {
		return 0
	}
	notional := types.Notional(markPrice, pos.Size)
	if notional == 0 {
		return 0
	}
	equity := int64(a.Balance) + int64(pos.UPnL)
	return float64(equity) / float64(notional)
}

// MaintenanceMarginRatio is the minimum equity-to-notional ratio before a
// position is force-closed. 5% matches typical perpetual exchange policy.
const MaintenanceMarginRatio = 0.05

// IsLiquidatable returns whether the position for symbol has dropped below
// the maintenance margin ratio (5%) given the current mark price.
//
// The position-level UPnL is recomputed from markPrice in this call, so the
// caller does NOT need to invoke MarkToMarket beforehand. Returns false if
// there is no open position.
func (a *Account) IsLiquidatable(symbol types.Symbol, markPrice types.Price) bool {
	pos, ok := a.Positions[symbol]
	if !ok || pos.Size == 0 {
		return false
	}
	// Per-position margin ratio: (margin + uPnL) / positionNotional.
	// We use isolated margin so other positions / balance do not rescue
	// this one.
	var uPnL int64
	if pos.Side == types.SideBuy {
		uPnL = types.Notional(markPrice-pos.AvgEntry, pos.Size)
	} else {
		uPnL = types.Notional(pos.AvgEntry-markPrice, pos.Size)
	}
	notional := types.Notional(markPrice, pos.Size)
	if notional <= 0 {
		return false
	}
	equity := int64(pos.Margin) + uPnL
	if equity <= 0 {
		return true // already underwater
	}
	return float64(equity)/float64(notional) <= MaintenanceMarginRatio
}

// Liquidate performs a forced close of the position at markPrice.
//
// The flow:
//  1. Compute realised loss = -uPnL (since uPnL is negative for liquidatable longs).
//  2. Take the loss from the position's Margin first; whatever remains in Margin
//     after absorbing the loss returns to Balance (the "salvage").
//  3. If the loss exceeds Margin, the excess is reported as `loss` for the
//     insurance fund to absorb (Balance is NOT touched — isolated margin).
//  4. Position is removed from the account.
//
// Returns the size that needs to be closed in the market (taker side opposite
// to position) and `loss` = the shortfall that the insurance fund must cover.
// `loss` is always >= 0; a profitable liquidation (theoretically possible only
// if mark price stale) returns 0.
func (a *Account) Liquidate(symbol types.Symbol) (size types.Qty, loss types.Qty) {
	pos, ok := a.Positions[symbol]
	if !ok || pos.Size == 0 {
		return 0, 0
	}
	size = pos.Size

	// uPnL (already updated by MarkToMarket in the caller)
	uPnL := int64(pos.UPnL)
	margin := int64(pos.Margin)

	// equity remaining in this isolated margin position
	equity := margin + uPnL

	if equity > 0 {
		// Position still has positive equity — return it to balance.
		// (This should be small, near MMR threshold; insurance fund gets 0.)
		a.Balance += types.Qty(equity)
		loss = 0
	} else {
		// Position is underwater past margin. Balance not touched (isolated).
		// Insurance fund must absorb the deficit.
		loss = types.Qty(-equity)
	}

	delete(a.Positions, symbol)
	return size, loss
}

// ApplyFunding charges or credits the account based on the funding rate and
// position notional.
//
// Convention (matches Binance / FTX):
//
//	fundingRate > 0  → longs PAY shorts  (long balance decreases)
//	fundingRate < 0  → shorts PAY longs  (long balance increases)
//
// Charge formula:  payment = side * positionNotional * fundingRate
// where `side` is +1 for long, -1 for short. Balance is mutated in place.
func (a *Account) ApplyFunding(symbol types.Symbol, fundingRate float64, markPrice types.Price) {
	pos, ok := a.Positions[symbol]
	if !ok || pos.Size == 0 {
		return
	}
	if fundingRate == 0 {
		return
	}
	notional := types.Notional(markPrice, pos.Size)
	// Use float64 only for the rate × notional product. notional is int64,
	// rate is small (~1e-4), so precision loss is well below one satoshi.
	payment := int64(float64(notional) * fundingRate)
	if pos.Side == types.SideBuy {
		// long pays when rate > 0
		a.Balance -= types.Qty(payment)
	} else {
		// short receives when rate > 0 (and pays when rate < 0)
		a.Balance += types.Qty(payment)
	}
}

// Validate checks internal invariants. Returns an error if violated.
// Should be called after every mutation in test builds.
func (a *Account) Validate() error {
	if a.Balance < 0 {
		return fmt.Errorf("account: balance is negative: %s", a.Balance.String())
	}
	for sym, pos := range a.Positions {
		if pos.Size < 0 {
			return fmt.Errorf("account: position %s has negative size %s", sym, pos.Size.String())
		}
		if pos.Margin < 0 {
			return fmt.Errorf("account: position %s has negative margin %s", sym, pos.Margin.String())
		}
		if pos.Side != types.SideBuy && pos.Side != types.SideSell {
			return fmt.Errorf("account: position %s has unknown side %v", sym, pos.Side)
		}
	}
	return nil
}
