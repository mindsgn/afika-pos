package posdomain

import (
	"fmt"
	"math/big"
	"strings"
)

// Asset is one payment option the checkout page offers. ZARPerOne is the ZAR
// value of one unit of the asset, held as a decimal string so no precision is
// lost moving between Go, JSON, and SQLite.
type Asset struct {
	ID        string
	Asset     string
	Network   string
	Decimals  int
	ZARPerOne string
}

// DefaultAssetID is the payment option selected when a client does not choose.
const DefaultAssetID = "usdc-base"

// AssetOrder is the stable order used by list endpoints and documentation.
var AssetOrder = []string{
	"btc",
	"usdc-base",
	"usdc-ethereum",
	"zarp-base",
	"zarp-ethereum",
	"eurc-base",
	"eurc-ethereum",
}

// Assets are the supported payment options. Keep in sync with
// `app/src/domain/payment.ts` and `backend/workers/receipt/lib/match.go`.
var Assets = map[string]Asset{
	"btc":           {ID: "btc", Asset: "BTC", Network: "bitcoin", Decimals: 8, ZARPerOne: "1200000"},
	"usdc-base":     {ID: "usdc-base", Asset: "USDC", Network: "base", Decimals: 2, ZARPerOne: "18.5"},
	"usdc-ethereum": {ID: "usdc-ethereum", Asset: "USDC", Network: "ethereum", Decimals: 6, ZARPerOne: "18.5"},
	"zarp-base":     {ID: "zarp-base", Asset: "ZARP", Network: "base", Decimals: 2, ZARPerOne: "1"},
	"zarp-ethereum": {ID: "zarp-ethereum", Asset: "ZARP", Network: "ethereum", Decimals: 6, ZARPerOne: "1"},
	"eurc-base":     {ID: "eurc-base", Asset: "EURC", Network: "base", Decimals: 2, ZARPerOne: "20.1"},
	"eurc-ethereum": {ID: "eurc-ethereum", Asset: "EURC", Network: "ethereum", Decimals: 6, ZARPerOne: "20.1"},
}

// AssetByID resolves a payment option id, defaulting to USDC on Base.
func AssetByID(id string) Asset {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		key = DefaultAssetID
	}
	if asset, ok := Assets[key]; ok {
		return asset
	}
	return Assets[DefaultAssetID]
}

// ValidateAssetID rejects payment option ids the POS does not support.
func ValidateAssetID(id string) (Asset, error) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return Assets[DefaultAssetID], nil
	}
	asset, ok := Assets[key]
	if !ok {
		return Asset{}, fmt.Errorf("asset must be one of %s", strings.Join(AssetOrder, ", "))
	}
	return asset, nil
}

// CryptoAmountFor converts a ZAR-cent amount into the asset amount at the
// option's rate, rounded half up to the asset's display precision and returned
// as an exact decimal string ("6.67", "0.00010288").
//
// It performs the division with big.Int so the result never depends on the
// platform's floating point behaviour.
func CryptoAmountFor(amountCents int64, asset Asset) (string, error) {
	if amountCents < 0 {
		return "", fmt.Errorf("amount must not be negative")
	}
	denominator, scale, err := parseDecimal(asset.ZARPerOne)
	if err != nil {
		return "", fmt.Errorf("asset %q has an invalid rate: %w", asset.ID, err)
	}
	if denominator.Sign() == 0 {
		return "", fmt.Errorf("asset %q has a zero rate", asset.ID)
	}
	if asset.Decimals < 0 || asset.Decimals > 18 {
		return "", fmt.Errorf("asset %q has an invalid precision", asset.ID)
	}

	precision := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(asset.Decimals)), nil)
	numerator := new(big.Int).Mul(big.NewInt(amountCents), precision)
	numerator.Mul(numerator, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil))
	denominator.Mul(denominator, big.NewInt(CentsPerRand))

	// units = numerator/denominator, rounded half up.
	twiceNumerator := new(big.Int).Lsh(numerator, 1)
	twiceNumerator.Add(twiceNumerator, denominator)
	twiceDenominator := new(big.Int).Lsh(denominator, 1)
	units := new(big.Int).Quo(twiceNumerator, twiceDenominator)

	return formatScaled(units, asset.Decimals), nil
}

// parseDecimal turns "18.5" into 185 scaled by 10^1 and "1200000" into
// 1200000 scaled by 10^0. The caller uses the scale to keep exact decimal
// arithmetic.
func parseDecimal(raw string) (*big.Int, int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, 0, fmt.Errorf("empty decimal")
	}
	negative := false
	if strings.HasPrefix(value, "-") {
		negative = true
		value = value[1:]
	}
	whole, fraction := value, ""
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		whole, fraction = value[:dot], value[dot+1:]
	}
	if whole == "" {
		whole = "0"
	}
	digits := whole + fraction
	for _, r := range digits {
		if r < '0' || r > '9' {
			return nil, 0, fmt.Errorf("invalid decimal %q", raw)
		}
	}
	number, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, 0, fmt.Errorf("invalid decimal %q", raw)
	}
	if negative {
		number.Neg(number)
	}
	return number, len(fraction), nil
}

// formatScaled renders an integer scaled by 10^decimals as a decimal string,
// padding the fractional part with zeros.
func formatScaled(value *big.Int, decimals int) string {
	sign := ""
	if value.Sign() < 0 {
		sign = "-"
		value = new(big.Int).Neg(value)
	}
	text := value.String()
	if decimals == 0 {
		return sign + text
	}
	for len(text) <= decimals {
		text = "0" + text
	}
	cut := len(text) - decimals
	return sign + text[:cut] + "." + text[cut:]
}
