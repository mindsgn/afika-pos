// Package posdomain contains the pure business rules of the Afika POS.
//
// It mirrors the TypeScript domain layer in `app/src/domain` exactly, but
// never uses floating point for money. Every amount is an integer number of
// ZAR cents, and crypto amounts are exact decimal strings.
package posdomain

import (
	"fmt"
	"strconv"
	"strings"
)

// CentsPerRand is the number of cents in one rand.
const CentsPerRand int64 = 100

// FormatZAR renders cents as "R1 234.56", using a space as the thousands
// separator exactly like formatZAR in `app/src/domain/money.ts`.
func FormatZAR(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	rands := cents / CentsPerRand
	rem := cents % CentsPerRand
	digits := strconv.FormatInt(rands, 10)

	// Insert a space every three digits from the right.
	var grouped strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte(' ')
		}
		grouped.WriteRune(digit)
	}
	return fmt.Sprintf("%sR%s.%02d", sign, grouped.String(), rem)
}

// ParseCents converts user input ("12", "12.5", "12,50") into cents. It returns
// an error for anything parseToCents rejects, including negative values.
func ParseCents(input string) (int64, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(input), ",", ".")
	if normalized == "" {
		return 0, fmt.Errorf("amount is required")
	}

	whole, fraction := normalized, ""
	if dot := strings.IndexByte(normalized, '.'); dot >= 0 {
		whole, fraction = normalized[:dot], normalized[dot+1:]
	}
	if whole == "" || len(fraction) > 2 {
		return 0, fmt.Errorf("amount must be a number with at most two decimal places")
	}
	for _, r := range whole {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("amount must be a number with at most two decimal places")
		}
	}
	for _, r := range fraction {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("amount must be a number with at most two decimal places")
		}
	}
	if strings.Contains(normalized, ".") && fraction == "" {
		return 0, fmt.Errorf("amount must be a number with at most two decimal places")
	}

	trimmed := strings.TrimLeft(whole, "0")
	var rands int64
	if trimmed != "" {
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("amount is too large")
		}
		rands = parsed
	}
	if rands > (1<<62)/CentsPerRand {
		return 0, fmt.Errorf("amount is too large")
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	cents, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount must be a number with at most two decimal places")
	}
	return rands*CentsPerRand + cents, nil
}

// CentsToInput renders cents as a plain decimal string with two places, the
// inverse of ParseCents.
func CentsToInput(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/CentsPerRand, cents%CentsPerRand)
}

// roundHalfUpDiv divides a by b rounding halves up (towards positive infinity),
// matching JavaScript's Math.round for the positive values used here.
func roundHalfUpDiv(a, b int64) int64 {
	if b <= 0 {
		return 0
	}
	if a >= 0 {
		return (2*a + b) / (2 * b)
	}
	// For negatives Math.round(-2.5) = -2, i.e. round towards positive infinity.
	return -((-2*a + b - 1) / (2 * b))
}
