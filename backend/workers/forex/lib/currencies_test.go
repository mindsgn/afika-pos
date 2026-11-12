package lib

import "testing"

func TestSupportedCurrenciesIncludeEuro(t *testing.T) {
	if !IsSupportedCurrency("EUR") {
		t.Fatal("EUR must be a supported currency")
	}
}

func TestSupportedCurrencies(t *testing.T) {
	want := []string{"USD", "EUR", "ZAR"}
	if len(SupportedCurrencies) != len(want) {
		t.Fatalf("SupportedCurrencies = %v, want %v", SupportedCurrencies, want)
	}
	for i, code := range want {
		if SupportedCurrencies[i] != code {
			t.Fatalf("SupportedCurrencies[%d] = %q, want %q", i, SupportedCurrencies[i], code)
		}
	}
}

func TestIsSupportedCurrency(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		{name: "USD", code: "USD", want: true},
		{name: "EUR", code: "EUR", want: true},
		{name: "ZAR", code: "ZAR", want: true},
		{name: "euro lowercase", code: "eur", want: true},
		{name: "whitespace", code: "  EUR  ", want: true},
		{name: "unsupported", code: "GBP", want: false},
		{name: "empty", code: "", want: false},
		{name: "blank", code: "   ", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSupportedCurrency(tc.code); got != tc.want {
				t.Fatalf("IsSupportedCurrency(%q) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}
