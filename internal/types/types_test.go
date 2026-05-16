package types

import (
	"encoding/json"
	"testing"
)

func TestPriceFromFloatRoundTrip(t *testing.T) {
	cases := []float64{0, 0.01, 1.5, 7900.50, 12345.678901, -100.25}
	for _, f := range cases {
		p := PriceFromFloat(f)
		got := p.Float()
		if abs(got-f) > 1e-7 {
			t.Errorf("round-trip failed: %v -> %d -> %v", f, p, got)
		}
	}
}

func TestPriceString(t *testing.T) {
	cases := []struct {
		in   Price
		want string
	}{
		{0, "0"},
		{Price(Scale), "1"},
		{Price(Scale * 7900), "7900"},
		{PriceFromFloat(7900.50), "7900.5"},
		{PriceFromFloat(0.01), "0.01"},
		{PriceFromFloat(-100.25), "-100.25"},
	}
	for _, c := range cases {
		got := c.in.String()
		if got != c.want {
			t.Errorf("String(%d): got %q want %q", c.in, got, c.want)
		}
	}
}

func TestSideOpposite(t *testing.T) {
	if SideBuy.Opposite() != SideSell {
		t.Error("buy opposite should be sell")
	}
	if SideSell.Opposite() != SideBuy {
		t.Error("sell opposite should be buy")
	}
	if SideUnknown.Opposite() != SideUnknown {
		t.Error("unknown opposite should be unknown")
	}
}

func TestOrderValidate(t *testing.T) {
	good := &Order{
		Symbol: "BTC-MED", Side: SideBuy, Type: OrderTypeLimit,
		Price: PriceFromFloat(100), Quantity: QtyFromFloat(1),
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("good order should validate: %v", err)
	}

	bad := []*Order{
		{Side: SideUnknown, Type: OrderTypeLimit, Price: 1, Quantity: 1},
		{Side: SideBuy, Type: OrderTypeLimit, Price: 0, Quantity: 1}, // zero price limit
		{Side: SideBuy, Type: OrderTypeLimit, Price: 1, Quantity: 0}, // zero qty
		{Side: SideBuy, Type: OrderTypeUnknown, Price: 1, Quantity: 1},
	}
	for i, o := range bad {
		if err := o.Validate(); err == nil {
			t.Errorf("case %d: expected error, got nil", i)
		}
	}
}

func TestOrderRemaining(t *testing.T) {
	o := &Order{Quantity: QtyFromFloat(10), Filled: QtyFromFloat(3)}
	if r := o.Remaining(); r != QtyFromFloat(7) {
		t.Errorf("remaining: got %v want %v", r, QtyFromFloat(7))
	}
	if o.IsFullyFilled() {
		t.Error("should not be fully filled")
	}
	o.Filled = o.Quantity
	if !o.IsFullyFilled() {
		t.Error("should be fully filled")
	}
}

func TestEventJSONStable(t *testing.T) {
	// Marshalling the same event twice must produce identical bytes —
	// foundation for deterministic events.jsonl.
	e := Event{
		Seq:  42,
		TS:   1583971200000000000,
		Type: EventTrade,
		Payload: mustJSON(TradePayload{Trade: Trade{
			ID: 1, Symbol: "BTC-MED", Price: PriceFromFloat(7900),
			Quantity: QtyFromFloat(0.5), TakerSide: SideBuy, TS: 1583971200000000000,
		}}),
	}
	b1, _ := json.Marshal(e)
	b2, _ := json.Marshal(e)
	if string(b1) != string(b2) {
		t.Fatal("event marshal not deterministic")
	}
}

func TestNotional(t *testing.T) {
	p := PriceFromFloat(7900)
	q := QtyFromFloat(0.5)
	got := Notional(p, q)
	want := int64(3950 * Scale)
	if got != want {
		t.Errorf("notional: got %d want %d", got, want)
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
