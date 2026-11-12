package lib

import "strings"

// SupportedCurrencies are the fiat bases the forex worker publishes rates for.
// Rates are stored for every base/quote combination returned by the provider,
// so USD/EUR, EUR/USD and ZAR/EUR are all available to consumers.
var SupportedCurrencies = []string{"USD", "EUR", "ZAR"}

// IsSupportedCurrency reports whether code is in SupportedCurrencies.
// Matching is case-insensitive and ignores surrounding whitespace.
func IsSupportedCurrency(code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return false
	}
	for _, supported := range SupportedCurrencies {
		if supported == code {
			return true
		}
	}
	return false
}
