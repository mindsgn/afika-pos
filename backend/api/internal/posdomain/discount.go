package posdomain

import "sort"

// DiscountType is the kind of manual discount applied to a sale.
type DiscountType string

const (
	DiscountNone    DiscountType = "none"
	DiscountPercent DiscountType = "percent"
	DiscountFixed   DiscountType = "fixed"
)

// Discount is the manual discount a merchant applies to a sale. Percent values
// are whole percentages clamped to 0..100; fixed values are ZAR cents clamped to
// the cart subtotal.
type Discount struct {
	Type  DiscountType
	Value int64
}

// NoDiscount is the zero discount.
var NoDiscount = Discount{Type: DiscountNone}

// Valid reports whether the discount type is one the POS understands.
func (d Discount) Valid() bool {
	switch d.Type {
	case DiscountNone, DiscountPercent, DiscountFixed:
		return true
	default:
		return false
	}
}

// Amount returns the discount in cents for a subtotal. It is the exact port of
// discountAmount in `app/src/domain/discount.ts`.
func (d Discount) Amount(subtotal int64) int64 {
	if subtotal <= 0 {
		return 0
	}
	switch d.Type {
	case DiscountPercent:
		pct := d.Value
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		return roundHalfUpDiv(subtotal*pct, 100)
	case DiscountFixed:
		value := d.Value
		if value < 0 {
			value = 0
		}
		if value > subtotal {
			value = subtotal
		}
		return value
	default:
		return 0
	}
}

// Allocate splits total cents across weights proportionally using the largest
// remainder method, so the returned slice always sums exactly to total.
//
// The TypeScript version computes the raw shares in floating point and breaks
// ties by sort order. This version uses exact integer remainders and a stable
// sort, which produces the same result without ever touching a float.
func Allocate(total int64, weights []int64) []int64 {
	out := make([]int64, len(weights))
	if total <= 0 || len(weights) == 0 {
		return out
	}
	var sum int64
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum <= 0 {
		return out
	}

	type share struct {
		index     int
		remainder int64
	}
	var allocated int64
	shares := make([]share, len(weights))
	for i, w := range weights {
		if w <= 0 {
			shares[i] = share{index: i}
			continue
		}
		product := total * w
		out[i] = product / sum
		shares[i] = share{index: i, remainder: product % sum}
		allocated += out[i]
	}

	remaining := total - allocated
	if remaining <= 0 {
		return out
	}
	sort.SliceStable(shares, func(a, b int) bool {
		return shares[a].remainder > shares[b].remainder
	})
	for i := 0; i < len(shares) && remaining > 0; i++ {
		if weights[shares[i].index] <= 0 {
			continue
		}
		out[shares[i].index]++
		remaining--
	}
	return out
}
