package posdomain

import "testing"

func TestPointsEarned(t *testing.T) {
	tests := map[int64]int64{
		-100:  0,
		0:     0,
		999:   0,
		1000:  1,
		1999:  1,
		2000:  2,
		12345: 12,
	}
	for total, want := range tests {
		if got := PointsEarned(total); got != want {
			t.Errorf("PointsEarned(%d) = %d, want %d", total, got, want)
		}
	}
}

func TestPointsValue(t *testing.T) {
	if got := PointsValue(5); got != 50 {
		t.Fatalf("PointsValue(5) = %d", got)
	}
	if got := PointsValue(-3); got != 0 {
		t.Fatalf("PointsValue(-3) = %d", got)
	}
}

func TestMaxRedeemablePoints(t *testing.T) {
	tests := []struct {
		balance, payable, want int64
	}{
		{100, 1000, 100}, // payable caps nothing
		{100, 500, 50},   // payable caps at 500/10
		{0, 1000, 0},
		{10, 5, 0},
		{-1, 1000, 0},
	}
	for _, tc := range tests {
		if got := MaxRedeemablePoints(tc.balance, tc.payable); got != tc.want {
			t.Errorf("MaxRedeemablePoints(%d, %d) = %d, want %d", tc.balance, tc.payable, got, tc.want)
		}
	}
}

func TestReversePointsForRefund(t *testing.T) {
	// A R100 sale earned 10 points; refunding R25 reverses 2 points.
	if got := ReversePointsForRefund(10, 2500, 10000, 0); got != 2 {
		t.Fatalf("reverse = %d, want 2", got)
	}
	// Already reversed 9 points: a further refund can only take 1 more.
	if got := ReversePointsForRefund(10, 5000, 10000, 9); got != 1 {
		t.Fatalf("capped reverse = %d, want 1", got)
	}
	// Nothing left to reverse.
	if got := ReversePointsForRefund(10, 5000, 10000, 10); got != 0 {
		t.Fatalf("fully reversed = %d, want 0", got)
	}
	if got := ReversePointsForRefund(10, 100, 0, 0); got != 0 {
		t.Fatalf("zero sale total = %d", got)
	}
}

func TestVoidPoints(t *testing.T) {
	// Redeemed 5, earned 12: void puts the 5 back and removes the 12.
	if got := VoidPoints(20, 5, 12); got != 13 {
		t.Fatalf("VoidPoints = %d, want 13", got)
	}
	// Never negative.
	if got := VoidPoints(0, 0, 12); got != 0 {
		t.Fatalf("VoidPoints = %d, want 0", got)
	}
}
