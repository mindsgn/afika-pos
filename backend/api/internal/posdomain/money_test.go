package posdomain

import "testing"

func TestFormatZAR(t *testing.T) {
	tests := []struct {
		cents int64
		want  string
	}{
		{0, "R0.00"},
		{5, "R0.05"},
		{99, "R0.99"},
		{100, "R1.00"},
		{123456, "R1 234.56"},
		{100000000, "R1 000 000.00"},
		{-123456, "-R1 234.56"},
	}
	for _, tc := range tests {
		if got := FormatZAR(tc.cents); got != tc.want {
			t.Errorf("FormatZAR(%d) = %q, want %q", tc.cents, got, tc.want)
		}
	}
}

func TestParseCents(t *testing.T) {
	valid := map[string]int64{
		"12":      1200,
		"12.5":    1250,
		"12,50":   1250,
		" 7 ":     700,
		"0":       0,
		"0.05":    5,
		"0012.00": 1200,
	}
	for input, want := range valid {
		got, err := ParseCents(input)
		if err != nil {
			t.Errorf("ParseCents(%q) error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ParseCents(%q) = %d, want %d", input, got, want)
		}
	}

	for _, input := range []string{"", "-1", "1.234", "abc", "1.", ".5", "1,2,3", "1 000"} {
		if _, err := ParseCents(input); err == nil {
			t.Errorf("ParseCents(%q) = nil error, want error", input)
		}
	}
}

func TestCentsToInputRoundTrip(t *testing.T) {
	for _, cents := range []int64{0, 5, 100, 123456, 99999999} {
		parsed, err := ParseCents(CentsToInput(cents))
		if err != nil || parsed != cents {
			t.Fatalf("round trip %d -> %q -> %d (%v)", cents, CentsToInput(cents), parsed, err)
		}
	}
}
