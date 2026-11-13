package posservice

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/mindsgn-studio/afika-backend/api/internal/posdomain"
	"github.com/mindsgn-studio/afika-backend/api/internal/posstore"
)

// seedCart creates the two-product catalogue used by most sale tests and
// returns both products.
func seedCart(t *testing.T, service *Service, businessID string) (posstore.Product, posstore.Product) {
	t.Helper()
	flat := seedProduct(t, service, businessID, "COF", "Flat White", 3500, 1200, 10, 5)
	muffin := seedProduct(t, service, businessID, "MUF", "Muffin", 2500, 900, 10, 5)
	return flat, muffin
}

func TestPendingCheckoutPricesOnServer(t *testing.T) {
	service, store := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, muffin := seedCart(t, service, business.ID)

	detail, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{
			{ProductID: flat.ID, Qty: 2},
			{ProductID: muffin.ID, Qty: 1},
		},
		Discount:      SaleDiscountInput{Type: "percent", Value: 10},
		PaymentMethod: "usdc-base",
	})
	if err != nil {
		t.Fatalf("create sale: %v", err)
	}

	sale := detail.Sale
	if sale.Status != posdomain.StatusPending || sale.PaymentStatus != "pending" {
		t.Fatalf("status = %s/%s", sale.Status, sale.PaymentStatus)
	}
	if sale.ReceiptNo != "R-000001" {
		t.Fatalf("receipt no = %q", sale.ReceiptNo)
	}
	if sale.Subtotal != 9500 || sale.DiscountAmount != 950 || sale.Total != 8550 {
		t.Fatalf("pricing = subtotal %d, discount %d, total %d", sale.Subtotal, sale.DiscountAmount, sale.Total)
	}
	if sale.PointsEarned != 0 || sale.CryptoAmount != "0" {
		t.Fatalf("pending sale should not settle: %+v", sale)
	}
	if len(detail.Items) != 2 {
		t.Fatalf("items = %+v", detail.Items)
	}
	if detail.Items[0].LineDiscount != 700 || detail.Items[1].LineDiscount != 250 {
		t.Fatalf("line discounts = %d/%d", detail.Items[0].LineDiscount, detail.Items[1].LineDiscount)
	}
	if detail.Items[0].Qty != 2 || detail.Items[0].UnitPrice != 3500 || detail.Items[0].UnitCost != 1200 {
		t.Fatalf("line snapshot = %+v", detail.Items[0])
	}

	// A pending sale must not touch stock.
	for _, product := range []posstore.Product{flat, muffin} {
		current, err := service.GetProduct(ctx, business.ID, product.ID)
		if err != nil || current.Stock != 10 {
			t.Fatalf("stock changed: %+v / %v", current, err)
		}
	}
	if n := countRows(t, store, `SELECT COUNT(*) FROM pos_stock_movements WHERE type = 'sale'`); n != 0 {
		t.Fatalf("sale movements = %d", n)
	}
}

func TestCheckoutMergesDuplicateLines(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, _ := seedCart(t, service, business.ID)

	detail, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{
			{ProductID: flat.ID, Qty: 1},
			{ProductID: flat.ID, Qty: 2},
		},
		PaymentMethod: "zarp-base",
	})
	if err != nil {
		t.Fatalf("create sale: %v", err)
	}
	if len(detail.Items) != 1 || detail.Items[0].Qty != 3 {
		t.Fatalf("merged items = %+v", detail.Items)
	}
	if detail.Sale.Subtotal != 10500 || detail.Sale.Total != 10500 {
		t.Fatalf("sale = %+v", detail.Sale)
	}
}

func TestCheckoutRedeemsLoyaltyPoints(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, muffin := seedCart(t, service, business.ID)
	customer := seedCustomer(t, service, business.ID, "Thandi", "0820000000")
	setPoints(t, service.Store(), business.ID, customer.ID, 50)

	detail, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{
			{ProductID: flat.ID, Qty: 2},
			{ProductID: muffin.ID, Qty: 1},
		},
		CustomerID:    customer.ID,
		RedeemPoints:  10,
		PaymentMethod: "usdc-base",
		Complete:      true,
	})
	if err != nil {
		t.Fatalf("create sale: %v", err)
	}
	sale := detail.Sale
	if sale.PointsRedeemed != 10 || sale.PointsDiscount != 100 {
		t.Fatalf("redemption = %d/%d", sale.PointsRedeemed, sale.PointsDiscount)
	}
	if sale.Total != 9400 || sale.PointsEarned != 9 {
		t.Fatalf("total/earned = %d/%d", sale.Total, sale.PointsEarned)
	}
	if detail.Items[0].LineDiscount != 74 || detail.Items[1].LineDiscount != 26 {
		t.Fatalf("allocation = %d/%d", detail.Items[0].LineDiscount, detail.Items[1].LineDiscount)
	}
	if detail.Customer == nil || detail.Customer.LoyaltyPoints != 49 {
		t.Fatalf("customer after checkout = %+v", detail.Customer)
	}
	current, err := service.GetCustomer(ctx, business.ID, customer.ID)
	if err != nil || current.LoyaltyPoints != 49 {
		t.Fatalf("stored points = %+v / %v", current, err)
	}
	if sale.CryptoAmount != "5.08" {
		// 9400 cents = R94.00; R94.00 / 18.5 = 5.08108...
		t.Fatalf("crypto amount = %q", sale.CryptoAmount)
	}
}

func TestCheckoutRejectsInvalidCarts(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, muffin := seedCart(t, service, business.ID)
	customer := seedCustomer(t, service, business.ID, "Thandi", "")
	setPoints(t, service.Store(), business.ID, customer.ID, 5)
	archived := seedProduct(t, service, business.ID, "OLD", "Archived", 1000, 100, 10, 0)
	if _, err := service.SetProductActive(ctx, business.ID, archived.ID, false); err != nil {
		t.Fatalf("archive: %v", err)
	}

	cases := []struct {
		name  string
		input CreateSaleInput
		code  string
	}{
		{"empty", CreateSaleInput{}, CodeCartEmpty},
		{"zero qty", CreateSaleInput{Items: []SaleItemInput{{ProductID: flat.ID, Qty: 0}}}, CodeInvalidQuantity},
		{"unknown product", CreateSaleInput{Items: []SaleItemInput{{ProductID: "missing", Qty: 1}}}, CodeProductNotFound},
		{"inactive product", CreateSaleInput{Items: []SaleItemInput{{ProductID: archived.ID, Qty: 1}}}, CodeProductInactive},
		{"oversell", CreateSaleInput{Items: []SaleItemInput{{ProductID: muffin.ID, Qty: 11}}}, CodeInsufficientStock},
		{"bad discount", CreateSaleInput{Items: []SaleItemInput{{ProductID: flat.ID, Qty: 1}}, Discount: SaleDiscountInput{Type: "bogus"}}, CodeInvalidDiscount},
		{"bad payment method", CreateSaleInput{Items: []SaleItemInput{{ProductID: flat.ID, Qty: 1}}, PaymentMethod: "doge"}, CodeInvalidPayment},
		{"missing customer", CreateSaleInput{Items: []SaleItemInput{{ProductID: flat.ID, Qty: 1}}, CustomerID: "missing"}, CodeCustomerNotFound},
		{"points without customer", CreateSaleInput{Items: []SaleItemInput{{ProductID: flat.ID, Qty: 1}}, RedeemPoints: 1}, CodeCustomerRequired},
		{"too many points", CreateSaleInput{Items: []SaleItemInput{{ProductID: flat.ID, Qty: 1}}, CustomerID: customer.ID, RedeemPoints: 6}, CodeInsufficientPoints},
		{"negative points", CreateSaleInput{Items: []SaleItemInput{{ProductID: flat.ID, Qty: 1}}, CustomerID: customer.ID, RedeemPoints: -1}, CodeInsufficientPoints},
		{"points exceed total", CreateSaleInput{Items: []SaleItemInput{{ProductID: flat.ID, Qty: 1}}, CustomerID: customer.ID, RedeemPoints: 5, Discount: SaleDiscountInput{Type: "percent", Value: 100}}, CodeInvalidCart},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.CreateSale(ctx, business.ID, tc.input); errCode(t, err) != tc.code {
				t.Fatalf("error = %v, want code %s", err, tc.code)
			}
		})
	}
}

func TestCompleteSaleDeductsStockAndEarnsPointsOnce(t *testing.T) {
	service, store := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, muffin := seedCart(t, service, business.ID)
	customer := seedCustomer(t, service, business.ID, "Thandi", "")
	setPoints(t, service.Store(), business.ID, customer.ID, 50)

	pending, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{
			{ProductID: flat.ID, Qty: 2},
			{ProductID: muffin.ID, Qty: 1},
		},
		CustomerID:    customer.ID,
		Discount:      SaleDiscountInput{Type: "percent", Value: 10},
		PaymentMethod: "usdc-base",
	})
	if err != nil {
		t.Fatalf("pending sale: %v", err)
	}

	completed, err := service.CompleteSale(ctx, business.ID, pending.Sale.ID, CompleteSaleInput{
		TxHash: "0xabc", CryptoAmount: "4.62", CryptoRate: "18.5",
	})
	if err != nil {
		t.Fatalf("complete sale: %v", err)
	}
	if completed.Sale.Status != posdomain.StatusCompleted || completed.Sale.PaymentStatus != "success" {
		t.Fatalf("completed sale = %+v", completed.Sale)
	}
	if completed.Sale.PointsEarned != 8 || completed.Sale.TxHash != "0xabc" {
		t.Fatalf("settlement = %+v", completed.Sale)
	}
	if completed.Sale.CryptoAmount != "4.62" || completed.Sale.CryptoRate != "18.5" {
		t.Fatalf("crypto = %q/%q", completed.Sale.CryptoAmount, completed.Sale.CryptoRate)
	}
	if completed.Customer == nil || completed.Customer.LoyaltyPoints != 58 {
		t.Fatalf("customer = %+v", completed.Customer)
	}

	flatNow, _ := service.GetProduct(ctx, business.ID, flat.ID)
	muffinNow, _ := service.GetProduct(ctx, business.ID, muffin.ID)
	if flatNow.Stock != 8 || muffinNow.Stock != 9 {
		t.Fatalf("stock = %d/%d", flatNow.Stock, muffinNow.Stock)
	}
	movements, err := service.ListMovements(ctx, business.ID, flat.ID, 10)
	if err != nil || len(movements) != 2 || movements[0].Type != "sale" || movements[0].Quantity != -2 {
		t.Fatalf("movements = %+v / %v", movements, err)
	}

	// Repeating the confirmation is safe and changes nothing.
	again, err := service.CompleteSale(ctx, business.ID, pending.Sale.ID, CompleteSaleInput{TxHash: "0xabc"})
	if err != nil {
		t.Fatalf("repeat complete: %v", err)
	}
	if again.Sale.Status != posdomain.StatusCompleted || again.Sale.PointsEarned != 8 {
		t.Fatalf("repeat completion changed the sale: %+v", again.Sale)
	}
	flatNow, _ = service.GetProduct(ctx, business.ID, flat.ID)
	customerNow, _ := service.GetCustomer(ctx, business.ID, customer.ID)
	if flatNow.Stock != 8 || customerNow.LoyaltyPoints != 58 {
		t.Fatalf("repeat completion applied twice: stock %d, points %d", flatNow.Stock, customerNow.LoyaltyPoints)
	}
	if n := countRows(t, store, `SELECT COUNT(*) FROM pos_stock_movements WHERE type = 'sale'`); n != 2 {
		t.Fatalf("sale movements = %d", n)
	}
}

func TestFailedSaleNeverChangesStockOrLoyalty(t *testing.T) {
	service, store := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, _ := seedCart(t, service, business.ID)
	customer := seedCustomer(t, service, business.ID, "Thandi", "")
	setPoints(t, service.Store(), business.ID, customer.ID, 50)

	pending, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items:         []SaleItemInput{{ProductID: flat.ID, Qty: 2}},
		CustomerID:    customer.ID,
		RedeemPoints:  5,
		PaymentMethod: "btc",
	})
	if err != nil {
		t.Fatalf("pending sale: %v", err)
	}

	failed, err := service.FailSale(ctx, business.ID, pending.Sale.ID, "expired")
	if err != nil {
		t.Fatalf("fail sale: %v", err)
	}
	if failed.Sale.Status != posdomain.StatusFailed || failed.Sale.PaymentStatus != "failed" || failed.Sale.FailureReason != "expired" {
		t.Fatalf("failed sale = %+v", failed.Sale)
	}
	flatNow, _ := service.GetProduct(ctx, business.ID, flat.ID)
	customerNow, _ := service.GetCustomer(ctx, business.ID, customer.ID)
	if flatNow.Stock != 10 || customerNow.LoyaltyPoints != 50 {
		t.Fatalf("failed payment changed state: stock %d, points %d", flatNow.Stock, customerNow.LoyaltyPoints)
	}
	if n := countRows(t, store, `SELECT COUNT(*) FROM pos_stock_movements WHERE type = 'sale'`); n != 0 {
		t.Fatalf("sale movements = %d", n)
	}

	// Failing again is a no-op.
	if _, err := service.FailSale(ctx, business.ID, pending.Sale.ID, "expired"); err != nil {
		t.Fatalf("repeat fail: %v", err)
	}
	// A failed sale can never be completed.
	if _, err := service.CompleteSale(ctx, business.ID, pending.Sale.ID, CompleteSaleInput{}); errCode(t, err) != CodeSaleNotPending {
		t.Fatalf("complete failed sale error = %v", err)
	}
}

func TestVoidRestoresStockAndReversesLoyalty(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, muffin := seedCart(t, service, business.ID)
	customer := seedCustomer(t, service, business.ID, "Thandi", "")
	setPoints(t, service.Store(), business.ID, customer.ID, 50)

	completed, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{
			{ProductID: flat.ID, Qty: 2},
			{ProductID: muffin.ID, Qty: 1},
		},
		CustomerID:    customer.ID,
		RedeemPoints:  10,
		PaymentMethod: "usdc-base",
		Complete:      true,
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	// 50 - 10 + 9 = 49
	if completed.Customer.LoyaltyPoints != 49 {
		t.Fatalf("points after sale = %d", completed.Customer.LoyaltyPoints)
	}

	voided, err := service.VoidSale(ctx, business.ID, completed.Sale.ID, "  customer changed mind  ")
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	if voided.Sale.Status != posdomain.StatusVoided || voided.Sale.VoidReason != "customer changed mind" {
		t.Fatalf("voided sale = %+v", voided.Sale)
	}
	flatNow, _ := service.GetProduct(ctx, business.ID, flat.ID)
	muffinNow, _ := service.GetProduct(ctx, business.ID, muffin.ID)
	if flatNow.Stock != 10 || muffinNow.Stock != 10 {
		t.Fatalf("stock not restored: %d/%d", flatNow.Stock, muffinNow.Stock)
	}
	customerNow, _ := service.GetCustomer(ctx, business.ID, customer.ID)
	if customerNow.LoyaltyPoints != 50 {
		t.Fatalf("points after void = %d, want 50", customerNow.LoyaltyPoints)
	}

	if _, err := service.VoidSale(ctx, business.ID, completed.Sale.ID, ""); errCode(t, err) != CodeSaleAlreadyVoided {
		t.Fatalf("double void error = %v", err)
	}

	pending, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{{ProductID: flat.ID, Qty: 1}}, PaymentMethod: "usdc-base",
	})
	if err != nil {
		t.Fatalf("pending sale: %v", err)
	}
	if _, err := service.VoidSale(ctx, business.ID, pending.Sale.ID, ""); errCode(t, err) != CodeSaleNotVoidable {
		t.Fatalf("void pending error = %v", err)
	}
}

func TestRefundPartialThenFull(t *testing.T) {
	service, store := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, muffin := seedCart(t, service, business.ID)
	customer := seedCustomer(t, service, business.ID, "Thandi", "")
	setPoints(t, service.Store(), business.ID, customer.ID, 50)

	completed, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{
			{ProductID: flat.ID, Qty: 2},
			{ProductID: muffin.ID, Qty: 1},
		},
		CustomerID:    customer.ID,
		Discount:      SaleDiscountInput{Type: "percent", Value: 10},
		PaymentMethod: "usdc-base",
		Complete:      true,
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if completed.Sale.Total != 8550 || completed.Sale.PointsEarned != 8 {
		t.Fatalf("sale = %+v", completed.Sale)
	}
	flatLine := completed.Items[0]
	muffinLine := completed.Items[1]

	// Refunding one of two flat whites takes half the discounted line total.
	first, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, CreateRefundInput{
		Items:  []RefundItemInput{{SaleItemID: flatLine.ID, Qty: 1}},
		Reason: "cold",
	})
	if err != nil {
		t.Fatalf("partial refund: %v", err)
	}
	if first.Refund.Amount != 3150 || first.Refund.PointsReversed != 2 || first.Status != posdomain.StatusPartiallyRefunded {
		t.Fatalf("first refund = %+v / %s", first.Refund, first.Status)
	}
	if first.Refund.Items[0].CostRecovered != 1200 || !first.Refund.Restocked {
		t.Fatalf("refund line = %+v", first.Refund.Items[0])
	}
	flatNow, _ := service.GetProduct(ctx, business.ID, flat.ID)
	customerNow, _ := service.GetCustomer(ctx, business.ID, customer.ID)
	if flatNow.Stock != 9 || customerNow.LoyaltyPoints != 56 {
		t.Fatalf("after first refund: stock %d, points %d", flatNow.Stock, customerNow.LoyaltyPoints)
	}

	// Asking for more than remains is rejected.
	if _, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, CreateRefundInput{
		Items: []RefundItemInput{{SaleItemID: flatLine.ID, Qty: 2}},
	}); errCode(t, err) != CodeRefundQtyExceeds {
		t.Fatalf("over-refund error = %v", err)
	}

	// The last flat white takes the rounding remainder of its line.
	second, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, CreateRefundInput{
		Items: []RefundItemInput{{SaleItemID: flatLine.ID, Qty: 1}},
	})
	if err != nil {
		t.Fatalf("second refund: %v", err)
	}
	if second.Refund.Amount != 3150 || second.Status != posdomain.StatusPartiallyRefunded {
		t.Fatalf("second refund = %+v / %s", second.Refund, second.Status)
	}

	third, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, CreateRefundInput{
		Items: []RefundItemInput{{SaleItemID: muffinLine.ID, Qty: 1}},
	})
	if err != nil {
		t.Fatalf("final refund: %v", err)
	}
	if third.Refund.Amount != 2250 || third.Status != posdomain.StatusRefunded || third.RefundedTotal != 8550 {
		t.Fatalf("final refund = %+v / %s / %d", third.Refund, third.Status, third.RefundedTotal)
	}

	detail, err := service.GetSale(ctx, business.ID, completed.Sale.ID)
	if err != nil {
		t.Fatalf("get sale: %v", err)
	}
	if detail.Sale.Status != posdomain.StatusRefunded || detail.Sale.RefundedTotal != detail.Sale.Total {
		t.Fatalf("sale after refunds = %+v", detail.Sale)
	}
	if len(detail.Refunds) != 3 {
		t.Fatalf("refunds = %d", len(detail.Refunds))
	}
	if detail.Items[0].RefundedQty != 2 || detail.Items[1].RefundedQty != 1 {
		t.Fatalf("refunded quantities = %d/%d", detail.Items[0].RefundedQty, detail.Items[1].RefundedQty)
	}

	// A fully refunded sale cannot be refunded again or voided.
	if _, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, CreateRefundInput{
		Items: []RefundItemInput{{SaleItemID: muffinLine.ID, Qty: 1}},
	}); errCode(t, err) != CodeSaleNotRefundable {
		t.Fatalf("refund after full refund error = %v", err)
	}
	if _, err := service.VoidSale(ctx, business.ID, completed.Sale.ID, ""); errCode(t, err) != CodeSaleNotVoidable {
		t.Fatalf("void refunded sale error = %v", err)
	}

	flatNow, _ = service.GetProduct(ctx, business.ID, flat.ID)
	muffinNow, _ := service.GetProduct(ctx, business.ID, muffin.ID)
	if flatNow.Stock != 10 || muffinNow.Stock != 10 {
		t.Fatalf("stock not restored: %d/%d", flatNow.Stock, muffinNow.Stock)
	}
	if n := countRows(t, store, `SELECT COUNT(*) FROM pos_stock_movements WHERE type = 'refund'`); n != 3 {
		t.Fatalf("refund movements = %d", n)
	}
}

func TestRefundWriteOffKeepsCost(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, _ := seedCart(t, service, business.ID)

	completed, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items:         []SaleItemInput{{ProductID: flat.ID, Qty: 2}},
		PaymentMethod: "usdc-base",
		Complete:      true,
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	restock := false
	result, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, CreateRefundInput{
		Items:   []RefundItemInput{{SaleItemID: completed.Items[0].ID, Qty: 2}},
		Restock: &restock,
	})
	if err != nil {
		t.Fatalf("write-off refund: %v", err)
	}
	if result.Refund.Amount != 7000 || result.Refund.Restocked || result.Status != posdomain.StatusRefunded {
		t.Fatalf("refund = %+v / %s", result.Refund, result.Status)
	}
	if result.Refund.Items[0].CostRecovered != 0 {
		t.Fatalf("write-off recovered cost = %d", result.Refund.Items[0].CostRecovered)
	}
	current, _ := service.GetProduct(ctx, business.ID, flat.ID)
	if current.Stock != 8 {
		t.Fatalf("write-off changed stock: %d", current.Stock)
	}

	report, err := service.Profit(ctx, business.ID, 0, 0)
	if err != nil {
		t.Fatalf("profit: %v", err)
	}
	// Revenue 7000-7000=0, COGS stays 2400 because the goods were written off.
	if report.NetRevenue != 0 || report.COGS != 2400 || report.Profit != -2400 {
		t.Fatalf("profit after write-off = %+v", report)
	}
}

func TestRefundValidation(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, _ := seedCart(t, service, business.ID)

	completed, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items:         []SaleItemInput{{ProductID: flat.ID, Qty: 2}},
		PaymentMethod: "usdc-base",
		Complete:      true,
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	cases := []struct {
		name  string
		input CreateRefundInput
		code  string
	}{
		{"empty", CreateRefundInput{}, CodeRefundEmpty},
		{"zero qty", CreateRefundInput{Items: []RefundItemInput{{SaleItemID: completed.Items[0].ID, Qty: 0}}}, CodeRefundEmpty},
		{
			"duplicate line",
			CreateRefundInput{Items: []RefundItemInput{
				{SaleItemID: completed.Items[0].ID, Qty: 1},
				{SaleItemID: completed.Items[0].ID, Qty: 1},
			}},
			CodeRefundDuplicate,
		},
		{"unknown line", CreateRefundInput{Items: []RefundItemInput{{SaleItemID: "missing", Qty: 1}}}, CodeRefundItemNotFound},
		{"too many", CreateRefundInput{Items: []RefundItemInput{{SaleItemID: completed.Items[0].ID, Qty: 3}}}, CodeRefundQtyExceeds},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, tc.input); errCode(t, err) != tc.code {
				t.Fatalf("error = %v, want %s", err, tc.code)
			}
		})
	}

	pending, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{{ProductID: flat.ID, Qty: 1}}, PaymentMethod: "usdc-base",
	})
	if err != nil {
		t.Fatalf("pending sale: %v", err)
	}
	if _, err := service.CreateRefund(ctx, business.ID, pending.Sale.ID, CreateRefundInput{
		Items: []RefundItemInput{{SaleItemID: pending.Items[0].ID, Qty: 1}},
	}); errCode(t, err) != CodeSaleNotRefundable {
		t.Fatalf("refund pending error = %v", err)
	}
	if _, err := service.CreateRefund(ctx, business.ID, "missing", CreateRefundInput{
		Items: []RefundItemInput{{SaleItemID: "x", Qty: 1}},
	}); errCode(t, err) != CodeSaleNotFound {
		t.Fatalf("refund missing sale error = %v", err)
	}
}

func TestIdempotentSaleCreation(t *testing.T) {
	service, store := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, _ := seedCart(t, service, business.ID)

	input := CreateSaleInput{
		Items:          []SaleItemInput{{ProductID: flat.ID, Qty: 2}},
		PaymentMethod:  "usdc-base",
		Complete:       true,
		IdempotencyKey: "checkout-1",
	}
	first, err := service.CreateSale(ctx, business.ID, input)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := service.CreateSale(ctx, business.ID, input)
	if err != nil {
		t.Fatalf("replay create: %v", err)
	}
	if second.Sale.ID != first.Sale.ID || second.Sale.ReceiptNo != first.Sale.ReceiptNo {
		t.Fatalf("replay returned a different sale: %+v vs %+v", second.Sale, first.Sale)
	}
	if n := countRows(t, store, `SELECT COUNT(*) FROM pos_sales`); n != 1 {
		t.Fatalf("sales = %d", n)
	}
	current, _ := service.GetProduct(ctx, business.ID, flat.ID)
	if current.Stock != 8 {
		t.Fatalf("stock = %d, want 8 (deducted once)", current.Stock)
	}

	reused := input
	reused.Items = []SaleItemInput{{ProductID: flat.ID, Qty: 1}}
	if _, err := service.CreateSale(ctx, business.ID, reused); errCode(t, err) != CodeIdempotencyKeyReused {
		t.Fatalf("reused key error = %v", err)
	}
	if n := countRows(t, store, `SELECT COUNT(*) FROM pos_sales`); n != 1 {
		t.Fatalf("sales after rejected reuse = %d", n)
	}
}

func TestIdempotentRefund(t *testing.T) {
	service, store := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, _ := seedCart(t, service, business.ID)

	completed, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items:         []SaleItemInput{{ProductID: flat.ID, Qty: 2}},
		PaymentMethod: "usdc-base",
		Complete:      true,
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	input := CreateRefundInput{
		Items:          []RefundItemInput{{SaleItemID: completed.Items[0].ID, Qty: 1}},
		IdempotencyKey: "refund-1",
	}
	first, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, input)
	if err != nil {
		t.Fatalf("first refund: %v", err)
	}
	second, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, input)
	if err != nil {
		t.Fatalf("replay refund: %v", err)
	}
	if second.Refund.ID != first.Refund.ID || second.Refund.Amount != first.Refund.Amount {
		t.Fatalf("replay returned a different refund: %+v vs %+v", second.Refund, first.Refund)
	}
	if n := countRows(t, store, `SELECT COUNT(*) FROM pos_refunds`); n != 1 {
		t.Fatalf("refunds = %d", n)
	}
	current, _ := service.GetProduct(ctx, business.ID, flat.ID)
	if current.Stock != 9 {
		t.Fatalf("stock = %d, want 9 (restocked once)", current.Stock)
	}
}

func TestProfitReport(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, muffin := seedCart(t, service, business.ID)

	completed, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{
			{ProductID: flat.ID, Qty: 2},
			{ProductID: muffin.ID, Qty: 1},
		},
		Discount:      SaleDiscountInput{Type: "percent", Value: 10},
		PaymentMethod: "usdc-base",
		Complete:      true,
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if _, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, CreateRefundInput{
		Items: []RefundItemInput{{SaleItemID: completed.Items[0].ID, Qty: 1}},
	}); err != nil {
		t.Fatalf("refund: %v", err)
	}

	// A voided sale and a failed sale must be counted but not earn revenue.
	voided, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{{ProductID: muffin.ID, Qty: 1}}, PaymentMethod: "btc", Complete: true,
	})
	if err != nil {
		t.Fatalf("void candidate: %v", err)
	}
	if _, err := service.VoidSale(ctx, business.ID, voided.Sale.ID, "test"); err != nil {
		t.Fatalf("void: %v", err)
	}
	failed, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{{ProductID: muffin.ID, Qty: 1}}, PaymentMethod: "zarp-base",
	})
	if err != nil {
		t.Fatalf("failed candidate: %v", err)
	}
	if _, err := service.FailSale(ctx, business.ID, failed.Sale.ID, "expired"); err != nil {
		t.Fatalf("fail: %v", err)
	}

	report, err := service.Profit(ctx, business.ID, 0, 0)
	if err != nil {
		t.Fatalf("profit: %v", err)
	}
	if report.SalesCount != 1 || report.VoidedCount != 1 || report.FailedCount != 1 {
		t.Fatalf("counters = %+v", report)
	}
	if report.GrossRevenue != 8550 || report.RefundedTotal != 3150 || report.NetRevenue != 5400 {
		t.Fatalf("revenue = %+v", report)
	}
	if report.COGS != 2100 || report.Profit != 3300 || report.MarginPct != 61.1 {
		t.Fatalf("profit = %+v", report)
	}
	if report.PointsIssued != 8 || report.RefundCount != 1 {
		t.Fatalf("loyalty/refunds = %+v", report)
	}
	if len(report.ByMethod) != 1 || report.ByMethod[0].Method != "usdc-base" || report.ByMethod[0].Revenue != 5400 {
		t.Fatalf("by method = %+v", report.ByMethod)
	}
}

func TestReceipts(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, muffin := seedCart(t, service, business.ID)

	completed, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{
			{ProductID: flat.ID, Qty: 2},
			{ProductID: muffin.ID, Qty: 1},
		},
		Discount:      SaleDiscountInput{Type: "percent", Value: 10},
		PaymentMethod: "usdc-base",
		Complete:      true,
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	receipt, err := service.SaleReceipt(ctx, business.ID, completed.Sale.ID)
	if err != nil {
		t.Fatalf("sale receipt: %v", err)
	}
	for _, want := range []string{"KOFI COFFEE", "12 Long St, Cape Town", "R-000001", "Flat White", "TOTAL", "R85.50", "Points earned"} {
		if !strings.Contains(receipt, want) {
			t.Fatalf("receipt missing %q:\n%s", want, receipt)
		}
	}

	refund, err := service.CreateRefund(ctx, business.ID, completed.Sale.ID, CreateRefundInput{
		Items:  []RefundItemInput{{SaleItemID: completed.Items[0].ID, Qty: 1}},
		Reason: "cold",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	slip, err := service.RefundReceipt(ctx, business.ID, completed.Sale.ID, refund.Refund.ID)
	if err != nil {
		t.Fatalf("refund receipt: %v", err)
	}
	for _, want := range []string{"REFUND / RETURN", "R-000001", "R31.50", "Reason: cold", "Returned to stock"} {
		if !strings.Contains(slip, want) {
			t.Fatalf("refund receipt missing %q:\n%s", want, slip)
		}
	}
	if _, err := service.RefundReceipt(ctx, business.ID, completed.Sale.ID, "missing"); errCode(t, err) != CodeRefundNotFound {
		t.Fatalf("missing refund receipt error = %v", err)
	}
}

func TestConcurrentCompletionsSettleOnce(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, _ := seedCart(t, service, business.ID)

	pending, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items: []SaleItemInput{{ProductID: flat.ID, Qty: 2}}, PaymentMethod: "usdc-base",
	})
	if err != nil {
		t.Fatalf("pending sale: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, errs[index] = service.CompleteSale(ctx, business.ID, pending.Sale.ID, CompleteSaleInput{TxHash: "0xabc"})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent complete %d: %v", i, err)
		}
	}

	current, err := service.GetProduct(ctx, business.ID, flat.ID)
	if err != nil || current.Stock != 8 {
		t.Fatalf("stock = %+v / %v, want 8", current, err)
	}
	if n := countRows(t, service.Store(), `SELECT COUNT(*) FROM pos_stock_movements WHERE type = 'sale'`); n != 1 {
		t.Fatalf("sale movements = %d", n)
	}
}

func TestConcurrentRefundsNeverOverRefund(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)
	flat, _ := seedCart(t, service, business.ID)

	completed, err := service.CreateSale(ctx, business.ID, CreateSaleInput{
		Items:         []SaleItemInput{{ProductID: flat.ID, Qty: 1}},
		PaymentMethod: "usdc-base",
		Complete:      true,
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, errs[index] = service.CreateRefund(ctx, business.ID, completed.Sale.ID, CreateRefundInput{
				Items: []RefundItemInput{{SaleItemID: completed.Items[0].ID, Qty: 1}},
			})
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful refunds = %d, want exactly 1 (%v)", successes, errs)
	}
	detail, err := service.GetSale(ctx, business.ID, completed.Sale.ID)
	if err != nil {
		t.Fatalf("get sale: %v", err)
	}
	if detail.Sale.RefundedTotal != detail.Sale.Total {
		t.Fatalf("refunded total = %d, want %d", detail.Sale.RefundedTotal, detail.Sale.Total)
	}
	current, _ := service.GetProduct(ctx, business.ID, flat.ID)
	if current.Stock != 10 {
		t.Fatalf("stock = %d, want 10", current.Stock)
	}
	if n := countRows(t, service.Store(), `SELECT COUNT(*) FROM pos_refunds`); n != 1 {
		t.Fatalf("refunds = %d", n)
	}
}

func TestSaleDetailNotFound(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	business := seedBusiness(t, service)

	if _, err := service.GetSale(ctx, business.ID, "missing"); errCode(t, err) != CodeSaleNotFound {
		t.Fatalf("missing sale error = %v", err)
	}
	if _, err := service.CompleteSale(ctx, business.ID, "missing", CompleteSaleInput{}); errCode(t, err) != CodeSaleNotFound {
		t.Fatalf("complete missing error = %v", err)
	}
	if _, err := service.FailSale(ctx, business.ID, "missing", "x"); errCode(t, err) != CodeSaleNotFound {
		t.Fatalf("fail missing error = %v", err)
	}
	if _, err := service.VoidSale(ctx, business.ID, "missing", "x"); errCode(t, err) != CodeSaleNotFound {
		t.Fatalf("void missing error = %v", err)
	}
}
