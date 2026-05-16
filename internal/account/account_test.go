package account

import (
	"testing"

	"github.com/sandswind/perpCS/internal/types"
)

const sym = types.Symbol("BTC-MED")

// helper: make a fill trade
func makeFill(side types.Side, price types.Price, qty types.Qty) types.Trade {
	return types.Trade{
		ID:        1,
		Symbol:    sym,
		Price:     price,
		Quantity:  qty,
		TakerSide: side,
	}
}

// helper: make an order (used as context for ApplyFill)
func makeOrder(side types.Side, price types.Price, qty types.Qty) *types.Order {
	return &types.Order{
		ID:       1,
		Symbol:   sym,
		Side:     side,
		Type:     types.OrderTypeLimit,
		Price:    price,
		Quantity: qty,
	}
}

// TestOpenLong verifies that opening a long position sets AvgEntry, Size, and
// deducts Margin from Balance correctly.
func TestOpenLong(t *testing.T) {
	// Initial balance: 10_000 USDR (scaled)
	initialBalance := types.QtyFromFloat(10_000)
	acc := New("addr", "session1", initialBalance)

	fillPrice := types.PriceFromFloat(8000)
	fillQty := types.QtyFromFloat(0.1) // 0.1 BTC
	mark := fillPrice

	fill := makeFill(types.SideBuy, fillPrice, fillQty)
	ord := makeOrder(types.SideBuy, fillPrice, fillQty)

	if err := acc.ApplyFill(fill, ord, mark); err != nil {
		t.Fatalf("ApplyFill: %v", err)
	}

	pos, ok := acc.Positions[sym]
	if !ok {
		t.Fatal("expected position to exist")
	}
	if pos.Side != types.SideBuy {
		t.Errorf("side: got %v want buy", pos.Side)
	}
	if pos.Size != fillQty {
		t.Errorf("size: got %s want %s", pos.Size, fillQty)
	}
	if pos.AvgEntry != fillPrice {
		t.Errorf("avgEntry: got %s want %s", pos.AvgEntry, fillPrice)
	}
	// Margin = notional = 8000 * 0.1 = 800 USDR
	expectedMargin := types.QtyFromFloat(800)
	if pos.Margin != expectedMargin {
		t.Errorf("margin: got %s want %s", pos.Margin, expectedMargin)
	}
	// Balance should be reduced by margin
	expectedBalance := initialBalance - expectedMargin
	if acc.Balance != expectedBalance {
		t.Errorf("balance: got %s want %s", acc.Balance, expectedBalance)
	}
	if err := acc.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// TestOpenShort verifies that opening a short position works symmetrically.
func TestOpenShort(t *testing.T) {
	initialBalance := types.QtyFromFloat(10_000)
	acc := New("addr", "session2", initialBalance)

	fillPrice := types.PriceFromFloat(8000)
	fillQty := types.QtyFromFloat(0.2)
	mark := fillPrice

	fill := makeFill(types.SideSell, fillPrice, fillQty)
	ord := makeOrder(types.SideSell, fillPrice, fillQty)

	if err := acc.ApplyFill(fill, ord, mark); err != nil {
		t.Fatalf("ApplyFill: %v", err)
	}

	pos, ok := acc.Positions[sym]
	if !ok {
		t.Fatal("expected position to exist")
	}
	if pos.Side != types.SideSell {
		t.Errorf("side: got %v want sell", pos.Side)
	}
	if pos.Size != fillQty {
		t.Errorf("size: got %s want %s", pos.Size, fillQty)
	}
	// Margin = 8000 * 0.2 = 1600
	expectedMargin := types.QtyFromFloat(1600)
	if pos.Margin != expectedMargin {
		t.Errorf("margin: got %s want %s", pos.Margin, expectedMargin)
	}
	if err := acc.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// TestMarkToMarket_Long verifies that price increases give positive UPnL for longs.
func TestMarkToMarket_Long(t *testing.T) {
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	entryPrice := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)
	fill := makeFill(types.SideBuy, entryPrice, qty)
	_ = acc.ApplyFill(fill, makeOrder(types.SideBuy, entryPrice, qty), entryPrice)

	// Price rises to 8500 → UPnL = (8500 - 8000) * 1.0 = +500
	markPrice := types.PriceFromFloat(8500)
	acc.MarkToMarket(sym, markPrice)

	pos := acc.Positions[sym]
	expectedUPnL := types.QtyFromFloat(500)
	if pos.UPnL != expectedUPnL {
		t.Errorf("uPnL: got %s want %s", pos.UPnL, expectedUPnL)
	}
	if pos.UPnL <= 0 {
		t.Errorf("uPnL should be positive for long when price rises")
	}
}

// TestMarkToMarket_Short verifies that price increases give negative UPnL for shorts.
func TestMarkToMarket_Short(t *testing.T) {
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	entryPrice := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)
	fill := makeFill(types.SideSell, entryPrice, qty)
	_ = acc.ApplyFill(fill, makeOrder(types.SideSell, entryPrice, qty), entryPrice)

	// Price rises to 8500 → UPnL = (8000 - 8500) * 1.0 = -500
	markPrice := types.PriceFromFloat(8500)
	acc.MarkToMarket(sym, markPrice)

	pos := acc.Positions[sym]
	expectedUPnL := types.QtyFromFloat(-500)
	if pos.UPnL != expectedUPnL {
		t.Errorf("uPnL: got %s want %s", pos.UPnL, expectedUPnL)
	}
	if pos.UPnL >= 0 {
		t.Errorf("uPnL should be negative for short when price rises")
	}
}

// TestClosePartial verifies partial position closure reduces size and
// returns the correct portion of margin + realised PnL.
func TestClosePartial(t *testing.T) {
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	entryPrice := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)

	// Open long 1.0 BTC @ 8000
	openFill := makeFill(types.SideBuy, entryPrice, qty)
	_ = acc.ApplyFill(openFill, makeOrder(types.SideBuy, entryPrice, qty), entryPrice)

	balanceAfterOpen := acc.Balance

	// Close 0.5 BTC @ 8200 (profit = (8200-8000)*0.5 = 100)
	closePrice := types.PriceFromFloat(8200)
	closeQty := types.QtyFromFloat(0.5)
	closeFill := makeFill(types.SideSell, closePrice, closeQty)
	if err := acc.ApplyFill(closeFill, makeOrder(types.SideSell, closePrice, closeQty), closePrice); err != nil {
		t.Fatalf("ApplyFill close: %v", err)
	}

	pos := acc.Positions[sym]
	if pos == nil {
		t.Fatal("expected position to remain after partial close")
	}
	expectedSize := types.QtyFromFloat(0.5)
	if pos.Size != expectedSize {
		t.Errorf("size after partial close: got %s want %s", pos.Size, expectedSize)
	}
	// Returned margin = 8000 * 0.5 = 4000; realized PnL = (8200-8000) * 0.5 = 100
	expectedPnL := types.QtyFromFloat(100)
	returnedMargin := types.QtyFromFloat(4000)
	expectedBalance := balanceAfterOpen + returnedMargin + expectedPnL
	if acc.Balance != expectedBalance {
		t.Errorf("balance after partial close: got %s want %s", acc.Balance, expectedBalance)
	}
	if err := acc.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// TestMarginRatio verifies the margin ratio formula:
//
//	marginRatio = (balance + Σ uPnL) / Σ positionNotional
func TestMarginRatio(t *testing.T) {
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	entryPrice := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)

	openFill := makeFill(types.SideBuy, entryPrice, qty)
	_ = acc.ApplyFill(openFill, makeOrder(types.SideBuy, entryPrice, qty), entryPrice)

	markPrice := types.PriceFromFloat(8000)
	acc.MarkToMarket(sym, markPrice)

	markPrices := map[types.Symbol]types.Price{sym: markPrice}
	ratio := acc.MarginRatio(markPrices)

	// balance = 10000 - 8000 = 2000; uPnL = 0; notional = 8000
	// ratio = (2000 + 0) / 8000 = 0.25
	expectedRatio := 2000.0 / 8000.0
	if diff := ratio - expectedRatio; diff > 0.001 || diff < -0.001 {
		t.Errorf("margin ratio: got %f want %f", ratio, expectedRatio)
	}
}

// TestBalance_InsufficientFunds verifies that ApplyFill returns an error when
// the balance is too low to cover the required margin.
func TestBalance_InsufficientFunds(t *testing.T) {
	// Only 100 USDR balance, trying to open 1 BTC @ 8000 (needs 8000 USDR)
	acc := New("addr", "s", types.QtyFromFloat(100))
	entryPrice := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)
	fill := makeFill(types.SideBuy, entryPrice, qty)
	err := acc.ApplyFill(fill, makeOrder(types.SideBuy, entryPrice, qty), entryPrice)
	if err == nil {
		t.Fatal("expected insufficient balance error, got nil")
	}
	// Balance should be unchanged
	if acc.Balance != types.QtyFromFloat(100) {
		t.Errorf("balance should not change on failed fill: got %s", acc.Balance)
	}
}

// ---- v0.4 liquidation + funding tests ----

// TestIsLiquidatable_BelowMMR opens a 10× long and crashes the mark price
// until the per-position margin ratio falls below MMR (5%).
func TestIsLiquidatable_BelowMMR(t *testing.T) {
	// 10x leverage simulation: open at $8000 with size 1.0 BTC.
	// Margin = $8000 (isolated, full notional in v0.x).
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	entry := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)
	_ = acc.ApplyFill(makeFill(types.SideBuy, entry, qty),
		makeOrder(types.SideBuy, entry, qty), entry)

	// Healthy state: same price, ratio = (margin + 0) / notional = 1.0
	if acc.IsLiquidatable(sym, entry) {
		t.Error("position should not be liquidatable at entry price")
	}

	// Mild drawdown: -20% → equity = 8000 - 1600 = 6400, notional = 6400, ratio = 1.0
	acc.MarkToMarket(sym, types.PriceFromFloat(6400))
	if acc.IsLiquidatable(sym, types.PriceFromFloat(6400)) {
		t.Error("position should not be liquidatable at -20%")
	}

	// Deep crash: at $4200, uPnL = -3800, equity = 4200, notional = 4200, ratio = 1.0.
	// Need a price where equity / notional <= 0.05.
	// equity = margin + uPnL = 8000 + (markPrice - 8000) * 1.0 = markPrice
	// notional = markPrice * 1.0 = markPrice  → ratio = 1.0 always with leverage=1.
	//
	// The current ApplyFill uses 1× leverage (full margin), so the position can
	// never fall below MMR under uPnL alone. To exercise the liquidation path
	// in this isolated-margin design, we manually shrink Margin to simulate
	// 10× leverage: Margin = notional/10.
	pos := acc.Positions[sym]
	pos.Margin = types.QtyFromFloat(800) // 10x leverage equivalent

	// Now equity = 800 + uPnL. At -5% (markPrice 7600): uPnL = -400, equity = 400,
	// notional = 7600, ratio = 400/7600 ≈ 5.3% — still safe.
	acc.MarkToMarket(sym, types.PriceFromFloat(7600))
	if acc.IsLiquidatable(sym, types.PriceFromFloat(7600)) {
		t.Error("position should still be safe at 5.3% ratio")
	}

	// At markPrice 7570: uPnL = -430, equity = 370, notional = 7570, ratio ≈ 4.9%
	acc.MarkToMarket(sym, types.PriceFromFloat(7570))
	if !acc.IsLiquidatable(sym, types.PriceFromFloat(7570)) {
		t.Errorf("position should be liquidatable at <5%% ratio")
	}

	// Severe crash: equity goes negative
	acc.MarkToMarket(sym, types.PriceFromFloat(7000))
	if !acc.IsLiquidatable(sym, types.PriceFromFloat(7000)) {
		t.Error("position with negative equity must be liquidatable")
	}
}

// TestLiquidate_Long verifies that liquidating a deep-loss long zeroes the
// position and reports a non-zero insurance-fund loss.
func TestLiquidate_Long(t *testing.T) {
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	entry := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)
	_ = acc.ApplyFill(makeFill(types.SideBuy, entry, qty),
		makeOrder(types.SideBuy, entry, qty), entry)

	// Simulate 10× leverage by shrinking the margin on the position,
	// so that a deep crash produces a real insurance-fund loss.
	pos := acc.Positions[sym]
	pos.Margin = types.QtyFromFloat(800)
	balanceBefore := acc.Balance

	// Crash to $7000 — uPnL = -1000, equity = 800 - 1000 = -200 (underwater)
	mark := types.PriceFromFloat(7000)
	acc.MarkToMarket(sym, mark)

	size, loss := acc.Liquidate(sym)
	if size != qty {
		t.Errorf("liquidated size: got %s want %s", size, qty)
	}
	if loss <= 0 {
		t.Errorf("expected positive loss to insurance fund, got %s", loss)
	}
	// Position must be removed
	if _, ok := acc.Positions[sym]; ok {
		t.Error("position should be removed after liquidation")
	}
	// Balance untouched (isolated margin)
	if acc.Balance != balanceBefore {
		t.Errorf("balance should not change on underwater liquidation: got %s want %s",
			acc.Balance, balanceBefore)
	}
}

// TestLiquidate_Short symmetrically verifies short liquidation.
func TestLiquidate_Short(t *testing.T) {
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	entry := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)
	_ = acc.ApplyFill(makeFill(types.SideSell, entry, qty),
		makeOrder(types.SideSell, entry, qty), entry)

	pos := acc.Positions[sym]
	pos.Margin = types.QtyFromFloat(800)

	// Pump to $9000 — uPnL = -1000 for short, equity = -200
	mark := types.PriceFromFloat(9000)
	acc.MarkToMarket(sym, mark)

	size, loss := acc.Liquidate(sym)
	if size != qty {
		t.Errorf("size: got %s want %s", size, qty)
	}
	if loss <= 0 {
		t.Errorf("expected loss > 0 for underwater short, got %s", loss)
	}
	if _, ok := acc.Positions[sym]; ok {
		t.Error("position should be removed")
	}
}

// TestLiquidate_PositiveSalvage covers the rare case where IsLiquidatable
// is true but equity is still slightly positive (right at the MMR boundary):
// the salvage must return to Balance and loss must be zero.
func TestLiquidate_PositiveSalvage(t *testing.T) {
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	entry := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)
	_ = acc.ApplyFill(makeFill(types.SideBuy, entry, qty),
		makeOrder(types.SideBuy, entry, qty), entry)

	pos := acc.Positions[sym]
	pos.Margin = types.QtyFromFloat(800)
	balanceBefore := acc.Balance

	// Mark at $7600: uPnL = -400, equity = 400 (positive but ≤ 5% ratio).
	mark := types.PriceFromFloat(7250)
	acc.MarkToMarket(sym, mark)
	// equity = 800 - 750 = 50 > 0 → salvage to balance
	_, loss := acc.Liquidate(sym)
	if loss != 0 {
		t.Errorf("loss should be 0 with positive equity, got %s", loss)
	}
	if acc.Balance <= balanceBefore {
		t.Errorf("balance should grow by salvage, got %s want >%s", acc.Balance, balanceBefore)
	}
}

// TestApplyFunding_LongPaysShort verifies that with positive funding rate,
// a long's balance decreases (it pays the rate × notional).
func TestApplyFunding_LongPaysShort(t *testing.T) {
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	entry := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)
	_ = acc.ApplyFill(makeFill(types.SideBuy, entry, qty),
		makeOrder(types.SideBuy, entry, qty), entry)

	balanceBefore := acc.Balance
	rate := 0.0001 // 1 bps
	mark := types.PriceFromFloat(8000)
	acc.ApplyFunding(sym, rate, mark)
	// payment = 8000 * 1.0 * 0.0001 = 0.8 USDR
	expectedDelta := types.QtyFromFloat(0.8)
	got := balanceBefore - acc.Balance
	if got != expectedDelta {
		t.Errorf("long balance delta: got %s want %s", got, expectedDelta)
	}
	if acc.Balance >= balanceBefore {
		t.Error("long balance must decrease when funding rate > 0")
	}
}

// TestApplyFunding_ShortPaysLong verifies that with negative funding rate,
// a short's balance decreases (longs receive).
func TestApplyFunding_ShortPaysLong(t *testing.T) {
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	entry := types.PriceFromFloat(8000)
	qty := types.QtyFromFloat(1.0)
	_ = acc.ApplyFill(makeFill(types.SideSell, entry, qty),
		makeOrder(types.SideSell, entry, qty), entry)

	balanceBefore := acc.Balance
	rate := -0.0001 // negative rate → shorts pay longs
	mark := types.PriceFromFloat(8000)
	acc.ApplyFunding(sym, rate, mark)
	if acc.Balance >= balanceBefore {
		t.Error("short balance must decrease when funding rate < 0")
	}
}

// TestApplyFunding_NoPosition is a no-op for accounts without the position.
func TestApplyFunding_NoPosition(t *testing.T) {
	acc := New("addr", "s", types.QtyFromFloat(10_000))
	balanceBefore := acc.Balance
	acc.ApplyFunding(sym, 0.001, types.PriceFromFloat(8000))
	if acc.Balance != balanceBefore {
		t.Errorf("balance must be unchanged when no position: got %s want %s",
			acc.Balance, balanceBefore)
	}
}
