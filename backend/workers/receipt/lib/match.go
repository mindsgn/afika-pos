package lib

import (
	"math/big"
	"strings"
)

// assetSymbols maps a POS asset option id to the token symbol written by the
// transaction worker. Keep in sync with `posAssets` in backend/api/lib and
// `ZAR_PER_UNIT` in app/afika-pos/src/domain/payment.ts.
var assetSymbols = map[string]string{
	"btc":           "BTC",
	"usdc-base":     "USDC",
	"usdc-ethereum": "USDC",
	"zarp-base":     "ZARP",
	"zarp-ethereum": "ZARP",
	"eurc-base":     "EURC",
	"eurc-ethereum": "EURC",
}

// networkAliases maps the payment's network label to the labels the balance and
// transaction workers write into `wallet_transactions.network`. Both the old
// (`base`, `ethereum`) and Alchemy (`base-mainnet`, `eth-mainnet`) spellings are
// accepted so the match does not depend on which worker wrote the row.
var networkAliases = map[string][]string{
	"base":     {"base-mainnet", "base"},
	"ethereum": {"eth-mainnet", "ethereum", "mainnet"},
	"bitcoin":  {"bitcoin", "btc-mainnet"},
}

// AssetSymbol returns the token symbol for an asset option id, or "" when the
// id is unknown.
func AssetSymbol(asset string) string {
	return assetSymbols[strings.ToLower(strings.TrimSpace(asset))]
}

// NetworkAliases returns the transaction-worker network names for a payment
// network label.
func NetworkAliases(network string) []string {
	aliases := networkAliases[strings.ToLower(strings.TrimSpace(network))]
	out := make([]string, len(aliases))
	copy(out, aliases)
	return out
}

// DecimalAtLeast reports whether have >= want for two decimal strings. Values
// are compared exactly with rationals so 18-decimal ZARP amounts and 2-decimal
// quotes can share one path.
func DecimalAtLeast(have, want string) bool {
	haveRat, ok := parseDecimal(have)
	if !ok {
		return false
	}
	wantRat, ok := parseDecimal(want)
	if !ok {
		return false
	}
	return haveRat.Cmp(wantRat) >= 0
}

func parseDecimal(raw string) (*big.Rat, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false
	}
	rat, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, false
	}
	return rat, true
}

// MatchCredit picks the earliest credit that pays at least the quoted amount.
// Credits arrive ordered oldest first, so a payment is matched to the transfer
// that satisfied it rather than to a later, unrelated transfer.
func MatchCredit(wantAmount string, credits []Credit) (Credit, bool) {
	for _, credit := range credits {
		if DecimalAtLeast(credit.Amount, wantAmount) {
			return credit, true
		}
	}
	return Credit{}, false
}
