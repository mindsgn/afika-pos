package lib

import (
	"math/big"
	"strconv"
	"strings"
	"time"
)

// Performance windows reported for a wallet's net worth.
const (
	WindowDay   = 24 * time.Hour
	WindowWeek  = 7 * 24 * time.Hour
	WindowMonth = 30 * 24 * time.Hour
	WindowYear  = 365 * 24 * time.Hour
)

// Portfolio is a basic breakdown of a wallet's net worth by asset class.
// All values are USD decimal strings.
type Portfolio struct {
	NetWorth string
	USDC     string
	EURC     string
	ZARP     string
	Stocks   string
	Other    string
}

// PortfolioSnapshot is the net worth recorded at a point in time.
type PortfolioSnapshot struct {
	At       int64
	NetWorth string
}

// PortfolioPerformance holds the change of net worth versus earlier snapshots.
// Pct values are percentages (for example "12.34") and Abs values are USD.
type PortfolioPerformance struct {
	DayPct   string
	WeekPct  string
	MonthPct string
	YearPct  string
	DayAbs   string
	WeekAbs  string
	MonthAbs string
	YearAbs  string
}

// PortfolioSummary combines the current breakdown with its performance.
type PortfolioSummary struct {
	Portfolio
	PortfolioPerformance
	UpdatedAt int64
}

// BuildPortfolio aggregates a wallet's balances into a net worth and asset
// class breakdown. USDC/USDT/DAI are treated as cash, EURC and ZARP are
// converted to USD with the supplied FX rates (USD/EUR and USD/ZAR), and any
// balance whose contract is in stockContracts counts as a stock.
func BuildPortfolio(balances []Balance, stockContracts map[string]bool, usdZar, usdEur string) Portfolio {
	var p Portfolio
	for _, b := range balances {
		symbol := strings.ToUpper(strings.TrimSpace(b.TokenSymbol))
		usd := strings.TrimSpace(b.USDAmount)
		if usd == "" {
			usd = deriveUSD(b, symbol, usdZar, usdEur)
		}
		if usd == "" {
			continue
		}

		p.NetWorth = AddDecimal(p.NetWorth, usd)

		contract := strings.ToLower(strings.TrimSpace(b.TokenAddress))
		switch {
		case symbol == "USDC" || symbol == "USDT" || symbol == "DAI":
			p.USDC = AddDecimal(p.USDC, usd)
		case symbol == "EURC":
			p.EURC = AddDecimal(p.EURC, usd)
		case symbol == "ZARP":
			p.ZARP = AddDecimal(p.ZARP, usd)
		case stockContracts[contract]:
			p.Stocks = AddDecimal(p.Stocks, usd)
		default:
			p.Other = AddDecimal(p.Other, usd)
		}
	}
	return p
}

// deriveUSD values stablecoins whose balance documents do not carry a USD
// amount yet. ZARP is ZAR-pegged and EURC is EUR-pegged, so both are converted
// through the USD pair rate.
func deriveUSD(b Balance, symbol, usdZar, usdEur string) string {
	amount := strings.TrimSpace(b.Amount)
	if amount == "" || strings.HasPrefix(amount, "-") {
		return ""
	}
	switch symbol {
	case "ZARP":
		return DivideDecimal(amount, strings.TrimSpace(usdZar))
	case "EURC":
		return DivideDecimal(amount, strings.TrimSpace(usdEur))
	default:
		return ""
	}
}

// PercentChange returns (current-previous)/previous*100 rounded to two
// decimals. It returns "0" when the previous value is unknown or zero.
func PercentChange(current, previous string) string {
	cur, ok1 := parseDecimal(zeroIfEmpty(current))
	prev, ok2 := parseDecimal(zeroIfEmpty(previous))
	if !ok1 || !ok2 || prev.Sign() == 0 {
		return "0"
	}
	diff := new(big.Float).Sub(cur, prev)
	pct := new(big.Float).Quo(diff, prev)
	pct.Mul(pct, big.NewFloat(100))
	v, _ := pct.Float64()
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// ChangeAmount returns current-previous as a USD decimal string.
func ChangeAmount(current, previous string) string {
	return SubDecimal(current, previous)
}

// FindSnapshot returns the most recent snapshot at or before targetMs.
func FindSnapshot(snapshots []PortfolioSnapshot, targetMs int64) (PortfolioSnapshot, bool) {
	var best PortfolioSnapshot
	found := false
	for _, s := range snapshots {
		if s.At <= targetMs && (!found || s.At > best.At) {
			best = s
			found = true
		}
	}
	return best, found
}

// ComputePerformance reports the net worth change versus the closest snapshot
// older than each reporting window.
func ComputePerformance(current string, snapshots []PortfolioSnapshot, nowMs int64) PortfolioPerformance {
	var p PortfolioPerformance
	windows := []struct {
		target int64
		pct    *string
		abs    *string
	}{
		{nowMs - WindowDay.Milliseconds(), &p.DayPct, &p.DayAbs},
		{nowMs - WindowWeek.Milliseconds(), &p.WeekPct, &p.WeekAbs},
		{nowMs - WindowMonth.Milliseconds(), &p.MonthPct, &p.MonthAbs},
		{nowMs - WindowYear.Milliseconds(), &p.YearPct, &p.YearAbs},
	}
	for _, w := range windows {
		snap, ok := FindSnapshot(snapshots, w.target)
		if !ok {
			*w.pct = "0"
			*w.abs = "0"
			continue
		}
		*w.pct = PercentChange(current, snap.NetWorth)
		*w.abs = ChangeAmount(current, snap.NetWorth)
	}
	return p
}

// BuildPortfolioSummary computes the current portfolio and its performance.
func BuildPortfolioSummary(balances []Balance, stockContracts map[string]bool, usdZar, usdEur string, snapshots []PortfolioSnapshot, nowMs int64) PortfolioSummary {
	p := BuildPortfolio(balances, stockContracts, usdZar, usdEur)
	return PortfolioSummary{
		Portfolio:            p,
		PortfolioPerformance: ComputePerformance(p.NetWorth, snapshots, nowMs),
		UpdatedAt:            nowMs,
	}
}
