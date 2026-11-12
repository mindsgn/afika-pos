package lib

import "testing"

func TestMultiplyDecimal(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"2", "3", "6"},
		{"0.02", "100", "2"},
		{"-2", "3", "-6"},
		{"bad", "1", ""},
		{"1", "", ""},
	}
	for _, tc := range tests {
		if got := MultiplyDecimal(tc.a, tc.b); got != tc.want {
			t.Fatalf("MultiplyDecimal(%q,%q)=%q want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestAddDecimal(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"1", "2", "3"},
		{"0.1", "0.2", "0.3"},
		{"", "", "0"},
		{"bad", "2", "0"},
	}
	for _, tc := range tests {
		if got := AddDecimal(tc.a, tc.b); got != tc.want {
			t.Fatalf("AddDecimal(%q,%q)=%q want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSubDecimal(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"3", "1", "2"},
		{"1", "1", "0"},
		{"1", "2.5", "-1.5"},
		{"", "", "0"},
	}
	for _, tc := range tests {
		if got := SubDecimal(tc.a, tc.b); got != tc.want {
			t.Fatalf("SubDecimal(%q,%q)=%q want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestDivideDecimal(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"10", "2", "5"},
		{"1", "4", "0.25"},
		{"1", "0", ""},
		{"bad", "2", ""},
	}
	for _, tc := range tests {
		if got := DivideDecimal(tc.a, tc.b); got != tc.want {
			t.Fatalf("DivideDecimal(%q,%q)=%q want %q", tc.a, tc.b, got, tc.want)
		}
	}
}
