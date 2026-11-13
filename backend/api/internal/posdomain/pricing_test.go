package posdomain

import "testing"

func TestPriceLines(t *testing.T) {
	lines := []PriceLine{
		{ProductID: "p1", SKU: "COF", Name: "Flat White", Qty: 2, UnitPrice: 3500, UnitCost: 1200},
		{ProductID: "p2", SKU: "MUF", Name: "Muffin", Qty: 1, UnitPrice: 2500, UnitCost: 900},
	}

	cart, err := PriceLines(lines, Discount{Type: DiscountFixed, Value: 1000}, 0)
	if err != nil {
		t.Fatalf("PriceLines: %v", err)
	}
	if cart.Subtotal != 9500 || cart.CostTotal != 3300 {
		t.Fatalf("subtotal/cost = %d/%d", cart.Subtotal, cart.CostTotal)
	}
	if cart.ManualDiscount != 1000 || cart.PointsDiscount != 0 || cart.Total != 8500 {
		t.Fatalf("discounts/total = %d/%d/%d", cart.ManualDiscount, cart.PointsDiscount, cart.Total)
	}
	// 1000 allocated across 7000:2500 weights.
	if cart.Lines[0].LineDiscount != 737 || cart.Lines[1].LineDiscount != 263 {
		t.Fatalf("line discounts = %d/%d", cart.Lines[0].LineDiscount, cart.Lines[1].LineDiscount)
	}
	if cart.PointsEarned != 8 {
		t.Fatalf("points earned = %d, want 8", cart.PointsEarned)
	}
}

func TestPriceLinesPointsRedemption(t *testing.T) {
	lines := []PriceLine{{ProductID: "p1", Qty: 1, UnitPrice: 10000, UnitCost: 4000}}
	cart, err := PriceLines(lines, Discount{Type: DiscountPercent, Value: 10}, 50)
	if err != nil {
		t.Fatalf("PriceLines: %v", err)
	}
	if cart.ManualDiscount != 1000 {
		t.Fatalf("manual discount = %d", cart.ManualDiscount)
	}
	if cart.PointsDiscount != 500 {
		t.Fatalf("points discount = %d", cart.PointsDiscount)
	}
	if cart.Total != 8500 {
		t.Fatalf("total = %d", cart.Total)
	}
}

func TestPriceLinesValidation(t *testing.T) {
	valid := []PriceLine{{ProductID: "p1", Qty: 1, UnitPrice: 1000, UnitCost: 100}}
	tests := []struct {
		name     string
		lines    []PriceLine
		discount Discount
		redeem   int64
	}{
		{"empty", nil, NoDiscount, 0},
		{"zero qty", []PriceLine{{ProductID: "p1", Qty: 0, UnitPrice: 1000}}, NoDiscount, 0},
		{"negative price", []PriceLine{{ProductID: "p1", Qty: 1, UnitPrice: -1}}, NoDiscount, 0},
		{"unknown discount", valid, Discount{Type: "bogus"}, 0},
		{"negative points", valid, NoDiscount, -1},
		{"points exceed amount", valid, NoDiscount, 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := PriceLines(tc.lines, tc.discount, tc.redeem); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestRefundLineAmount(t *testing.T) {
	// 3 x R10.00 with a R1 discount: line total 2900.
	// Refunding one piece returns floor(2900/3) = 966.
	if got := RefundLineAmount(1000, 100, 3, 0, 0, 1); got != 966 {
		t.Fatalf("first partial refund = %d, want 966", got)
	}
	// Refunding the last two pieces returns the remainder: 2900 - 966 = 1934.
	if got := RefundLineAmount(1000, 100, 3, 1, 966, 2); got != 1934 {
		t.Fatalf("last refund = %d, want 1934", got)
	}
	// A fully refunded line adds up exactly.
	first := RefundLineAmount(1000, 100, 3, 0, 0, 1)
	second := RefundLineAmount(1000, 100, 3, 1, first, 1)
	third := RefundLineAmount(1000, 100, 3, 2, first+second, 1)
	if first+second+third != 2900 {
		t.Fatalf("refunded total = %d, want 2900", first+second+third)
	}
	if got := RefundLineAmount(1000, 0, 1, 0, 0, 0); got != 0 {
		t.Fatalf("zero quantity = %d", got)
	}
}
