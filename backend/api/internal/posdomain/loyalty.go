package posdomain

// Loyalty rules, ported from `app/src/domain/loyalty.ts`.
//
// A customer earns one point per R10 spent (rounded down) and each point is
// worth 10 cents when redeemed, i.e. 1% back.
const (
	// EarnPerCents is the spend required to earn one loyalty point.
	EarnPerCents int64 = 1000
	// PointValueCents is the value of one loyalty point in cents.
	PointValueCents int64 = 10
)

// PointsEarned returns the points earned for a completed sale total.
func PointsEarned(totalCents int64) int64 {
	if totalCents <= 0 {
		return 0
	}
	return totalCents / EarnPerCents
}

// PointsValue returns the cents a number of points is worth.
func PointsValue(points int64) int64 {
	if points <= 0 {
		return 0
	}
	return points * PointValueCents
}

// MaxRedeemablePoints caps a redemption at the customer's balance and at the
// amount still payable.
func MaxRedeemablePoints(balance, payableCents int64) int64 {
	if balance <= 0 || payableCents <= 0 {
		return 0
	}
	cap := payableCents / PointValueCents
	if balance < cap {
		return balance
	}
	return cap
}

// PointsToReverse returns the points to claw back for a refund, proportional to
// the refunded share of the sale total. The caller still has to cap the result
// at the points actually earned and not yet reversed.
func PointsToReverse(earned, refundAmount, saleTotal int64) int64 {
	if saleTotal <= 0 || earned <= 0 || refundAmount <= 0 {
		return 0
	}
	reversed := (earned * refundAmount) / saleTotal
	if reversed > earned {
		return earned
	}
	return reversed
}

// ReversePointsForRefund computes the points a single refund reverses, capped at
// the points on the sale that have not been reversed yet.
func ReversePointsForRefund(earned, refundAmount, saleTotal, alreadyReversed int64) int64 {
	outstanding := earned - alreadyReversed
	if outstanding <= 0 {
		return 0
	}
	reverse := PointsToReverse(earned, refundAmount, saleTotal)
	if reverse > outstanding {
		return outstanding
	}
	return reverse
}

// VoidPoints returns the customer's balance after a sale is voided: the redeemed
// points come back and the earned points are taken away, never below zero.
func VoidPoints(balance, redeemed, earned int64) int64 {
	next := balance + redeemed - earned
	if next < 0 {
		return 0
	}
	return next
}
