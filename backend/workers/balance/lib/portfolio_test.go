package lib

import "testing"

func TestBuildPortfolio(t *testing.T) {
	stockContracts := map[string]bool{
		"0xb20000000000000000000078ee7ce2fe4908108c": true,
	}
	balances := []Balance{
		{TokenSymbol: "USDC", TokenAddress: "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", Amount: "10", USDAmount: "10"},
		{TokenSymbol: "EURC", TokenAddress: "0x60a3e35cc302bfa44cb288bc5a4f316fdb1adb42", Amount: "9.2"},
		{TokenSymbol: "ZARP", TokenAddress: "0xb755506531786c8ac63b756bab1ac387bacb0c04", Amount: "180"},
		{TokenSymbol: "NVDA", TokenAddress: "0xb20000000000000000000078ee7ce2fE4908108C", USDAmount: "200"},
		{TokenSymbol: "ETH", TokenAddress: "", Amount: "1"},
	}

	got := BuildPortfolio(balances, stockContracts, "18", "0.92")

	if got.NetWorth != "230" {
		t.Fatalf("net worth = %q, want 230", got.NetWorth)
	}
	if got.USDC != "10" {
		t.Fatalf("usdc = %q, want 10", got.USDC)
	}
	if got.EURC != "10" {
		t.Fatalf("eurc = %q, want 10", got.EURC)
	}
	if got.ZARP != "10" {
		t.Fatalf("zarp = %q, want 10", got.ZARP)
	}
	if got.Stocks != "200" {
		t.Fatalf("stocks = %q, want 200", got.Stocks)
	}
	if got.Other != "" {
		t.Fatalf("other = %q, want empty", got.Other)
	}
}

func TestBuildPortfolioEmpty(t *testing.T) {
	got := BuildPortfolio(nil, nil, "", "")
	if got.NetWorth != "" || got.USDC != "" || got.Stocks != "" {
		t.Fatalf("empty portfolio = %+v, want zero value", got)
	}
}

func TestBuildPortfolioMissingRates(t *testing.T) {
	balances := []Balance{
		{TokenSymbol: "ZARP", Amount: "180"},
		{TokenSymbol: "EURC", Amount: "9.2"},
	}
	got := BuildPortfolio(balances, nil, "", "")
	if got.NetWorth != "" {
		t.Fatalf("net worth = %q, want empty when no FX rates", got.NetWorth)
	}
}

func TestPercentChange(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		previous string
		want     string
	}{
		{name: "up", current: "110", previous: "100", want: "10.00"},
		{name: "down", current: "90", previous: "100", want: "-10.00"},
		{name: "unchanged", current: "100", previous: "100", want: "0.00"},
		{name: "zero previous", current: "100", previous: "0", want: "0"},
		{name: "empty previous", current: "100", previous: "", want: "0"},
		{name: "fractional", current: "105.5", previous: "100", want: "5.50"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PercentChange(tc.current, tc.previous); got != tc.want {
				t.Fatalf("PercentChange(%q, %q) = %q, want %q", tc.current, tc.previous, got, tc.want)
			}
		})
	}
}

func TestComputePerformance(t *testing.T) {
	now := int64(1_700_000_000_000)
	snapshots := []PortfolioSnapshot{
		{At: now - WindowDay.Milliseconds() - 1000, NetWorth: "100"},
		{At: now - WindowWeek.Milliseconds() - 1000, NetWorth: "80"},
		{At: now - WindowMonth.Milliseconds() - 1000, NetWorth: "50"},
		{At: now - WindowYear.Milliseconds() - 1000, NetWorth: "40"},
	}

	got := ComputePerformance("120", snapshots, now)

	if got.DayPct != "20.00" || got.WeekPct != "50.00" || got.MonthPct != "140.00" || got.YearPct != "200.00" {
		t.Fatalf("pct = %+v, want 20/50/140/200", got)
	}
	if got.DayAbs != "20" || got.WeekAbs != "40" || got.MonthAbs != "70" || got.YearAbs != "80" {
		t.Fatalf("abs = %+v, want 20/40/70/80", got)
	}
}

func TestComputePerformanceNoSnapshots(t *testing.T) {
	got := ComputePerformance("120", nil, 1_700_000_000_000)
	for _, v := range []string{got.DayPct, got.WeekPct, got.MonthPct, got.YearPct, got.DayAbs} {
		if v != "0" {
			t.Fatalf("performance = %+v, want zeros", got)
		}
	}
}

func TestFindSnapshotPicksMostRecentOlder(t *testing.T) {
	snapshots := []PortfolioSnapshot{
		{At: 100, NetWorth: "1"},
		{At: 300, NetWorth: "3"},
		{At: 200, NetWorth: "2"},
	}
	got, ok := FindSnapshot(snapshots, 250)
	if !ok || got.NetWorth != "2" {
		t.Fatalf("FindSnapshot = %+v ok=%v, want net worth 2", got, ok)
	}
	if _, ok := FindSnapshot(snapshots, 50); ok {
		t.Fatalf("FindSnapshot before all snapshots should not find one")
	}
}

func TestBuildPortfolioSummary(t *testing.T) {
	now := int64(1_700_000_000_000)
	balances := []Balance{{TokenSymbol: "USDC", Amount: "150", USDAmount: "150"}}
	snapshots := []PortfolioSnapshot{
		{At: now - WindowDay.Milliseconds() - 1, NetWorth: "100"},
	}
	got := BuildPortfolioSummary(balances, nil, "18", "0.92", snapshots, now)
	if got.NetWorth != "150" || got.USDC != "150" {
		t.Fatalf("summary = %+v, want net worth 150", got)
	}
	if got.DayPct != "50.00" || got.DayAbs != "50" {
		t.Fatalf("summary performance = %+v, want 50", got)
	}
	if got.UpdatedAt != now {
		t.Fatalf("UpdatedAt = %d, want %d", got.UpdatedAt, now)
	}
}
