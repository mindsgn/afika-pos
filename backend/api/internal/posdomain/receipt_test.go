package posdomain

import (
	"strings"
	"testing"
	"time"
)

func TestBuildReceipt(t *testing.T) {
	// 2023-11-14 10:13:20 UTC.
	created := time.Date(2023, 11, 14, 10, 13, 20, 0, time.UTC).UnixMilli()
	out := BuildReceipt(ReceiptInput{
		ReceiptNo:    "R-000007",
		CreatedAt:    created,
		Status:       StatusCompleted,
		CustomerName: "Thandi",
		Items: []ReceiptItem{
			{Name: "Flat White", Qty: 2, UnitPrice: 3500, LineDiscount: 0},
			{Name: "Muffin", Qty: 1, UnitPrice: 2500, LineDiscount: 500},
		},
		Subtotal:      9500,
		Discount:      500,
		Total:         9000,
		PaymentMethod: "usdc-base",
		CryptoAmount:  "4.86",
		CryptoAsset:   "USDC",
		PointsEarned:  9,
	}, ReceiptOptions{
		Merchant: "Kofi Coffee",
		Website:  "kofi.example",
		Address:  "12 Long St, Cape Town",
		Location: time.UTC,
	})

	for _, want := range []string{
		"KOFI COFFEE",
		"12 Long St, Cape Town",
		"kofi.example",
		"Receipt",
		"R-000007",
		"2023-11-14 10:13",
		"Customer",
		"Thandi",
		"2 x Flat White",
		"R70.00",
		"1 x Muffin",
		"   discount",
		"-R5.00",
		"Subtotal",
		"R95.00",
		"Discount",
		"-R5.00",
		"TOTAL",
		"R90.00",
		"Paid with",
		"4.86 USDC",
		"Points earned",
		"9",
		"Thank you!",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("receipt does not contain %q:\n%s", want, out)
		}
	}
	// Every line fits the 40-column width.
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 40 {
			t.Errorf("line %q is wider than 40 columns", line)
		}
	}
}

func TestBuildReceiptStatusBanners(t *testing.T) {
	tests := map[string]string{
		StatusPending:           "*** AWAITING PAYMENT ***",
		StatusVoided:            "*** VOIDED ***",
		StatusRefunded:          "*** REFUNDED ***",
		StatusPartiallyRefunded: "* PARTIALLY REFUNDED *",
		StatusFailed:            "*** PAYMENT FAILED ***",
	}
	for status, banner := range tests {
		t.Run(status, func(t *testing.T) {
			out := BuildReceipt(ReceiptInput{
				ReceiptNo: "R-1",
				Status:    status,
				Total:     100,
				Items:     []ReceiptItem{{Name: "Tea", Qty: 1, UnitPrice: 100}},
			}, ReceiptOptions{Location: time.UTC})
			if !strings.Contains(out, banner) {
				t.Fatalf("receipt for %q does not contain %q:\n%s", status, banner, out)
			}
		})
	}
}

func TestBuildReceiptRefundsAndFailure(t *testing.T) {
	out := BuildReceipt(ReceiptInput{
		ReceiptNo:     "R-2",
		Status:        StatusFailed,
		Items:         []ReceiptItem{{Name: "Tea", Qty: 1, UnitPrice: 100}},
		Subtotal:      100,
		Total:         100,
		RefundedTotal: 40,
		FailureReason: "underpaid",
	}, ReceiptOptions{Location: time.UTC})
	if !strings.Contains(out, "Refunded") || !strings.Contains(out, "Net") {
		t.Fatalf("missing refund lines:\n%s", out)
	}
	if !strings.Contains(out, "Reason: underpaid") {
		t.Fatalf("missing failure reason:\n%s", out)
	}
	if strings.Contains(out, "Points earned") {
		t.Fatalf("failed receipt should not print points:\n%s", out)
	}
}

func TestBuildRefundReceipt(t *testing.T) {
	at := time.Date(2023, 11, 15, 9, 0, 0, 0, time.UTC).UnixMilli()
	out := BuildRefundReceipt(RefundReceiptInput{
		ReceiptNo:  "R-000007",
		RefundedAt: at,
		Items:      []RefundReceiptItem{{Name: "Muffin", Qty: 1, Amount: 2000}},
		Amount:     2000,
		Reason:     "stale",
		Restocked:  true,
	}, ReceiptOptions{Merchant: "Kofi Coffee", Location: time.UTC})
	for _, want := range []string{"REFUND / RETURN", "R-000007", "2023-11-15 09:00", "1 x Muffin", "R20.00", "TOTAL REFUND", "Returned to stock", "Yes", "Reason: stale"} {
		if !strings.Contains(out, want) {
			t.Errorf("refund receipt does not contain %q:\n%s", want, out)
		}
	}
}

func TestReceiptLongLineIsTruncated(t *testing.T) {
	out := BuildReceipt(ReceiptInput{
		ReceiptNo: "R-1",
		Status:    StatusCompleted,
		Items:     []ReceiptItem{{Name: "An extremely long product name that cannot fit", Qty: 1, UnitPrice: 100}},
		Total:     100,
	}, ReceiptOptions{Location: time.UTC})
	if !strings.Contains(out, "…") {
		t.Fatalf("expected an ellipsis in the truncated line:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 40 {
			t.Fatalf("line %q is wider than 40 columns", line)
		}
	}
}
