package posdomain

import "sort"

// Sale statuses understood by the POS.
const (
	StatusPending           = "pending"
	StatusCompleted         = "completed"
	StatusFailed            = "failed"
	StatusVoided            = "voided"
	StatusRefunded          = "refunded"
	StatusPartiallyRefunded = "partially_refunded"
)

// soldStatuses are the statuses that count as revenue.
var soldStatuses = map[string]bool{
	StatusCompleted:         true,
	StatusPartiallyRefunded: true,
	StatusRefunded:          true,
}

// IsSoldStatus reports whether a sale status counts towards revenue.
func IsSoldStatus(status string) bool { return soldStatuses[status] }

// SaleTotals is the per-sale input to the profit calculation.
type SaleTotals struct {
	Status        string
	Total         int64
	CostTotal     int64
	RefundedTotal int64
	PointsEarned  int64
	PaymentMethod string
}

// MethodSummary is revenue per payment method.
type MethodSummary struct {
	Method  string `json:"method"`
	Count   int64  `json:"count"`
	Revenue int64  `json:"revenue"`
}

// ProfitSummary is the Insights screen's report. It matches ProfitSummary in
// `app/src/services/reports.ts`.
type ProfitSummary struct {
	SalesCount    int64           `json:"salesCount"`
	GrossRevenue  int64           `json:"grossRevenue"`
	RefundedTotal int64           `json:"refundedTotal"`
	NetRevenue    int64           `json:"netRevenue"`
	COGS          int64           `json:"cogs"`
	Profit        int64           `json:"profit"`
	MarginPct     float64         `json:"marginPct"`
	RefundCount   int64           `json:"refundCount"`
	VoidedCount   int64           `json:"voidedCount"`
	FailedCount   int64           `json:"failedCount"`
	PointsIssued  int64           `json:"pointsIssued"`
	ByMethod      []MethodSummary `json:"byMethod"`
}

// ProfitInput feeds ComputeProfit. RecoveredCost is the cost of refunded items
// that went back to stock; the cost of written-off items is not recovered.
type ProfitInput struct {
	Sales         []SaleTotals
	RecoveredCost int64
	RefundCount   int64
}

// ComputeProfit calculates profit exactly like profitSummary in
// `app/src/services/reports.ts`:
//
//	profit = net revenue (after refunds) - cost of goods kept by customers
//
// A restocked return restores its cost, so it is subtracted from COGS. A
// non-restocked return keeps its cost in COGS.
func ComputeProfit(input ProfitInput) ProfitSummary {
	summary := ProfitSummary{
		RefundCount: input.RefundCount,
		ByMethod:    []MethodSummary{},
	}
	methods := map[string]*MethodSummary{}

	for _, sale := range input.Sales {
		switch sale.Status {
		case StatusVoided:
			summary.VoidedCount++
			continue
		case StatusFailed:
			summary.FailedCount++
			continue
		}
		if !IsSoldStatus(sale.Status) {
			continue
		}

		summary.SalesCount++
		summary.GrossRevenue += sale.Total
		summary.RefundedTotal += sale.RefundedTotal
		summary.PointsIssued += sale.PointsEarned

		method := sale.PaymentMethod
		if method == "" {
			method = "unknown"
		}
		entry, ok := methods[method]
		if !ok {
			entry = &MethodSummary{Method: method}
			methods[method] = entry
		}
		entry.Count++
		entry.Revenue += sale.Total - sale.RefundedTotal
	}

	for _, sale := range input.Sales {
		if !IsSoldStatus(sale.Status) {
			continue
		}
		summary.COGS += sale.CostTotal
	}
	summary.COGS -= input.RecoveredCost
	summary.NetRevenue = summary.GrossRevenue - summary.RefundedTotal
	summary.Profit = summary.NetRevenue - summary.COGS
	if summary.NetRevenue > 0 {
		// Math.round((profit / netRevenue) * 1000) / 10
		tenths := roundHalfUpDiv(summary.Profit*1000, summary.NetRevenue)
		summary.MarginPct = float64(tenths) / 10
	}

	for _, entry := range methods {
		summary.ByMethod = append(summary.ByMethod, *entry)
	}
	sort.Slice(summary.ByMethod, func(a, b int) bool {
		return summary.ByMethod[a].Method < summary.ByMethod[b].Method
	})
	return summary
}
