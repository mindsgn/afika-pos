package posdomain

import "fmt"

// PriceLine is one cart line before discounts are allocated. The product fields
// are snapshots taken when the cart is priced, so later catalogue edits never
// rewrite history.
type PriceLine struct {
	ProductID string
	SKU       string
	Name      string
	Qty       int64
	UnitPrice int64
	UnitCost  int64
}

// PricedLine is a cart line with its share of the sale's discounts attached.
type PricedLine struct {
	PriceLine
	LineDiscount int64
}

// PricedCart is the fully priced cart. All amounts are ZAR cents.
type PricedCart struct {
	Lines          []PricedLine
	Subtotal       int64
	CostTotal      int64
	ManualDiscount int64
	PointsDiscount int64
	Total          int64
	RedeemPoints   int64
	Discount       Discount
	PointsEarned   int64
}

// PriceLines prices a cart exactly like priceCart in
// `app/src/services/checkout.ts`: the manual discount and the loyalty
// redemption are added together and allocated across the lines in proportion to
// their undiscounted value.
//
// The caller is responsible for merging duplicate product ids, checking that
// products exist and are active, and checking stock and loyalty balances.
func PriceLines(lines []PriceLine, discount Discount, redeemPoints int64) (PricedCart, error) {
	if len(lines) == 0 {
		return PricedCart{}, fmt.Errorf("the cart is empty")
	}
	if !discount.Valid() {
		return PricedCart{}, fmt.Errorf("discount type must be none, percent, or fixed")
	}
	if redeemPoints < 0 {
		return PricedCart{}, fmt.Errorf("points to redeem cannot be negative")
	}

	priced := PricedCart{
		Lines:        make([]PricedLine, len(lines)),
		RedeemPoints: redeemPoints,
		Discount:     discount,
	}
	weights := make([]int64, len(lines))
	for i, line := range lines {
		if line.Qty <= 0 {
			return PricedCart{}, fmt.Errorf("quantities must be whole numbers above zero")
		}
		if line.UnitPrice < 0 || line.UnitCost < 0 {
			return PricedCart{}, fmt.Errorf("prices cannot be negative")
		}
		lineTotal := line.UnitPrice * line.Qty
		priced.Subtotal += lineTotal
		priced.CostTotal += line.UnitCost * line.Qty
		weights[i] = lineTotal
		priced.Lines[i] = PricedLine{PriceLine: line}
	}

	priced.ManualDiscount = discount.Amount(priced.Subtotal)
	priced.PointsDiscount = PointsValue(redeemPoints)
	if priced.PointsDiscount > priced.Subtotal-priced.ManualDiscount {
		return PricedCart{}, fmt.Errorf("points value is more than the amount due")
	}
	priced.Total = priced.Subtotal - priced.ManualDiscount - priced.PointsDiscount
	if priced.Total < 0 {
		return PricedCart{}, fmt.Errorf("discounts cannot exceed the cart subtotal")
	}

	allocation := Allocate(priced.ManualDiscount+priced.PointsDiscount, weights)
	for i := range priced.Lines {
		priced.Lines[i].LineDiscount = allocation[i]
	}
	priced.PointsEarned = PointsEarned(priced.Total)
	return priced, nil
}

// RefundLineAmount returns the cents refunded for a quantity of one sale line.
//
// It mirrors refundSale in `app/src/services/sales.ts`: the last piece of a line
// takes the rounding remainder so the refunded amounts of a fully refunded line
// add up exactly to the line total after its discount.
func RefundLineAmount(unitPrice, lineDiscount, qty, refundedQty, refundedAmount, requestQty int64) int64 {
	if requestQty <= 0 || qty <= 0 {
		return 0
	}
	lineTotal := unitPrice*qty - lineDiscount
	remaining := qty - refundedQty
	if requestQty >= remaining {
		amount := lineTotal - refundedAmount
		if amount < 0 {
			return 0
		}
		return amount
	}
	return (lineTotal * requestQty) / qty
}
