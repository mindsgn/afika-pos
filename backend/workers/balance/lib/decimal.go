package lib

import (
	"math/big"
	"strings"
)

func MultiplyDecimal(a, b string) string {
	x, ok1 := parseDecimal(strings.TrimSpace(a))
	y, ok2 := parseDecimal(strings.TrimSpace(b))
	if !ok1 || !ok2 {
		return ""
	}
	out := new(big.Float).Mul(x, y)
	return trimDecimal(out)
}

// AddDecimal adds two decimal strings and returns a normalized decimal string.
// Invalid inputs fall back to "0" so callers can keep accumulating.
func AddDecimal(a, b string) string {
	x, ok1 := parseDecimal(zeroIfEmpty(a))
	y, ok2 := parseDecimal(zeroIfEmpty(b))
	if !ok1 || !ok2 {
		return "0"
	}
	return trimDecimal(new(big.Float).Add(x, y))
}

// SubDecimal subtracts b from a and returns a normalized decimal string.
// Invalid inputs fall back to "0".
func SubDecimal(a, b string) string {
	x, ok1 := parseDecimal(zeroIfEmpty(a))
	y, ok2 := parseDecimal(zeroIfEmpty(b))
	if !ok1 || !ok2 {
		return "0"
	}
	return trimDecimal(new(big.Float).Sub(x, y))
}

// DivideDecimal divides a by b and returns a normalized decimal string.
// Division by zero or invalid inputs returns an empty string.
func DivideDecimal(a, b string) string {
	x, ok1 := parseDecimal(strings.TrimSpace(a))
	y, ok2 := parseDecimal(strings.TrimSpace(b))
	if !ok1 || !ok2 || y.Sign() == 0 {
		return ""
	}
	return trimDecimal(new(big.Float).Quo(x, y))
}

func zeroIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "0"
	}
	return s
}

func trimDecimal(f *big.Float) string {
	if f == nil {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(f.Text('f', 18), "0"), ".")
}

func parseDecimal(s string) (*big.Float, bool) {
	if s == "" {
		return nil, false
	}
	f, _, err := big.ParseFloat(s, 10, 256, big.ToNearestEven)
	if err != nil {
		return nil, false
	}
	return f, true
}
