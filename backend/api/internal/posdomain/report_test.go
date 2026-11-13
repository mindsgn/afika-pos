package posdomain

import "testing"

func TestComputeProfit(t *testing.T) {
	input := ProfitInput{
		Sales: []SaleTotals{
			{Status: StatusCompleted, Total: 10000, CostTotal: 4000, PointsEarned: 10, PaymentMethod: "usdc-base"},
			{Status: StatusPartiallyRefunded, Total: 20000, CostTotal: 8000, RefundedTotal: 5000, PointsEarned: 20, PaymentMethod: "btc"},
			{Status: StatusRefunded, Total: 5000, CostTotal: 2000, RefundedTotal: 5000, PointsEarned: 5, PaymentMethod: "usdc-base"},
			{Status: StatusVoided, Total: 7000, CostTotal: 3000},
			{Status: StatusFailed, Total: 0},
			{Status: StatusPending, Total: 1000},
		},
		RecoveredCost: 1500,
		RefundCount:   2,
	}

	got := ComputeProfit(input)
	if got.SalesCount != 3 {
		t.Fatalf("salesCount = %d, want 3", got.SalesCount)
	}
	if got.GrossRevenue != 35000 {
		t.Fatalf("grossRevenue = %d, want 35000", got.GrossRevenue)
	}
	if got.RefundedTotal != 10000 {
		t.Fatalf("refundedTotal = %d, want 10000", got.RefundedTotal)
	}
	if got.NetRevenue != 25000 {
		t.Fatalf("netRevenue = %d, want 25000", got.NetRevenue)
	}
	// COGS = 4000 + 8000 + 2000 - 1500 recovered = 12500.
	if got.COGS != 12500 {
		t.Fatalf("cogs = %d, want 12500", got.COGS)
	}
	if got.Profit != 12500 {
		t.Fatalf("profit = %d, want 12500", got.Profit)
	}
	if got.MarginPct != 50 {
		t.Fatalf("marginPct = %v, want 50", got.MarginPct)
	}
	if got.VoidedCount != 1 || got.FailedCount != 1 {
		t.Fatalf("voided/failed = %d/%d", got.VoidedCount, got.FailedCount)
	}
	if got.PointsIssued != 35 {
		t.Fatalf("pointsIssued = %d, want 35", got.PointsIssued)
	}
	if len(got.ByMethod) != 2 {
		t.Fatalf("byMethod = %+v", got.ByMethod)
	}
	// Sorted by method name for a deterministic response.
	if got.ByMethod[0].Method != "btc" || got.ByMethod[0].Count != 1 || got.ByMethod[0].Revenue != 15000 {
		t.Fatalf("btc summary = %+v", got.ByMethod[0])
	}
	if got.ByMethod[1].Method != "usdc-base" || got.ByMethod[1].Count != 2 || got.ByMethod[1].Revenue != 10000 {
		t.Fatalf("usdc summary = %+v", got.ByMethod[1])
	}
}

func TestComputeProfitEmpty(t *testing.T) {
	got := ComputeProfit(ProfitInput{})
	if got.ByMethod == nil {
		t.Fatal("byMethod = nil, want empty slice")
	}
	if got.SalesCount != 0 || got.Profit != 0 || got.MarginPct != 0 {
		t.Fatalf("empty summary = %+v", got)
	}
}

func TestComputeProfitNegativeMargin(t *testing.T) {
	got := ComputeProfit(ProfitInput{Sales: []SaleTotals{
		{Status: StatusCompleted, Total: 1000, CostTotal: 1500, PaymentMethod: "usdc-base"},
	}})
	if got.Profit != -500 {
		t.Fatalf("profit = %d, want -500", got.Profit)
	}
	// Math.round((-500/1000)*1000)/10 = -50.
	if got.MarginPct != -50 {
		t.Fatalf("marginPct = %v, want -50", got.MarginPct)
	}
}
