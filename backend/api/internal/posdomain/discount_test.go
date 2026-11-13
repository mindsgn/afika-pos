package posdomain

import "testing"

func TestDiscountAmount(t *testing.T) {
	tests := []struct {
		name  string
		d     Discount
		cents int64
		want  int64
	}{
		{"none", NoDiscount, 12345, 0},
		{"percent ten", Discount{Type: DiscountPercent, Value: 10}, 12345, 1235},     // 1234.5 rounds up
		{"percent fifteen", Discount{Type: DiscountPercent, Value: 15}, 12345, 1852}, // 1851.75 rounds up
		{"percent above 100", Discount{Type: DiscountPercent, Value: 150}, 12345, 12345},
		{"percent below 0", Discount{Type: DiscountPercent, Value: -5}, 12345, 0},
		{"fixed", Discount{Type: DiscountFixed, Value: 2000}, 12345, 2000},
		{"fixed capped", Discount{Type: DiscountFixed, Value: 99999}, 12345, 12345},
		{"fixed negative", Discount{Type: DiscountFixed, Value: -1}, 12345, 0},
		{"zero subtotal", Discount{Type: DiscountPercent, Value: 10}, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.d.Amount(tc.cents); got != tc.want {
				t.Fatalf("Amount = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAllocateSumsExactly(t *testing.T) {
	tests := []struct {
		total   int64
		weights []int64
		want    []int64
	}{
		{0, []int64{1, 1}, []int64{0, 0}},
		{10, []int64{1, 1}, []int64{5, 5}},
		{10, []int64{1, 1, 1}, []int64{4, 3, 3}},
		{100, []int64{50, 50, 0}, []int64{50, 50, 0}},
		{999, []int64{0, 0}, []int64{0, 0}}, // no weights: nothing to allocate
		{12345, []int64{3000, 7000}, []int64{3704, 8641}},
		{1, []int64{1, 1, 1}, []int64{1, 0, 0}},
	}
	for _, tc := range tests {
		got := Allocate(tc.total, tc.weights)
		var weightSum int64
		for _, w := range tc.weights {
			weightSum += w
		}
		var sum int64
		for _, v := range got {
			sum += v
		}
		wantSum := tc.total
		if tc.total <= 0 || weightSum <= 0 {
			wantSum = 0
		}
		if sum != wantSum {
			t.Fatalf("Allocate(%d, %v) = %v, sum %d, want %d", tc.total, tc.weights, got, sum, wantSum)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("Allocate(%d, %v) = %v, want %v", tc.total, tc.weights, got, tc.want)
			}
		}
	}
}

func TestAllocateLargestRemainder(t *testing.T) {
	// R100 split across lines worth R30 and R70 with a R10 discount: the raw
	// shares are 3 and 7 exactly, so no remainder is distributed.
	got := Allocate(1000, []int64{3000, 7000})
	if got[0] != 300 || got[1] != 700 {
		t.Fatalf("Allocate = %v, want [300 700]", got)
	}

	// A one cent discount across three equal lines goes to the first line,
	// matching the stable sort in the TypeScript version.
	got = Allocate(1, []int64{100, 100, 100})
	if got[0] != 1 || got[1] != 0 || got[2] != 0 {
		t.Fatalf("Allocate = %v, want [1 0 0]", got)
	}
}
