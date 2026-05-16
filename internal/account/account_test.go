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
