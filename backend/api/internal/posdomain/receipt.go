package posdomain

import (
	"fmt"
	"strings"
	"time"
)

// ReceiptInput is everything a sale receipt prints. It mirrors ReceiptInput in
// `app/src/domain/receipt.ts` field for field.
type ReceiptInput struct {
	ReceiptNo     string
	CreatedAt     int64
	Status        string
	CustomerName  string
	Items         []ReceiptItem
	Subtotal      int64
	Discount      int64
	PointsDisc    int64
	Total         int64
	PaymentMethod string
	CryptoAmount  string
	CryptoAsset   string
	TxHash        string
	FailureReason string
	PointsEarned  int64
	PointsRedeem  int64
	RefundedTotal int64
}

// ReceiptItem is one printed sale line.
type ReceiptItem struct {
	Name         string
	Qty          int64
	UnitPrice    int64
	LineDiscount int64
}

// ReceiptOptions control the printed merchant header and time zone.
type ReceiptOptions struct {
	Width    int
	Merchant string
	Website  string
	Address  string
	Location *time.Location
}

// RefundReceiptInput is everything a refund receipt prints.
type RefundReceiptInput struct {
	ReceiptNo  string
	RefundedAt int64
	Items      []RefundReceiptItem
	Amount     int64
	Reason     string
	Restocked  bool
}

// RefundReceiptItem is one printed refunded line.
type RefundReceiptItem struct {
	Name   string
	Qty    int64
	Amount int64
}

const defaultReceiptWidth = 40

var receiptStatusBanner = map[string]string{
	"pending":            "*** AWAITING PAYMENT ***",
	"voided":             "*** VOIDED ***",
	"refunded":           "*** REFUNDED ***",
	"partially_refunded": "* PARTIALLY REFUNDED *",
	"failed":             "*** PAYMENT FAILED ***",
}

// FormatDateTime renders a millisecond timestamp as "2006-01-02 15:04" in the
// requested location, matching the app's formatDateTime.
func FormatDateTime(ms int64, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return time.UnixMilli(ms).In(loc).Format("2006-01-02 15:04")
}

// BuildReceipt renders the 40-column plain-text receipt the app prints and
// shares.
func BuildReceipt(input ReceiptInput, opts ReceiptOptions) string {
	width := receiptWidth(opts.Width)
	rule := strings.Repeat("-", width)
	lines := make([]string, 0, 32)

	merchant := strings.TrimSpace(opts.Merchant)
	if merchant == "" {
		merchant = "CRYPTO POS"
	}
	lines = append(lines, receiptCenter(strings.ToUpper(merchant), width))
	if address := strings.TrimSpace(opts.Address); address != "" {
		lines = append(lines, receiptCenter(address, width))
	}
	if website := strings.TrimSpace(opts.Website); website != "" {
		lines = append(lines, receiptCenter(website, width))
	}
	lines = append(lines, receiptCenter("Receipt", width), rule)
	lines = append(lines, receiptTwoColumn("Receipt", input.ReceiptNo, width))
	lines = append(lines, receiptTwoColumn("Date", FormatDateTime(input.CreatedAt, opts.Location), width))
	if customer := strings.TrimSpace(input.CustomerName); customer != "" {
		lines = append(lines, receiptTwoColumn("Customer", customer, width))
	}
	if banner, ok := receiptStatusBanner[input.Status]; ok {
		lines = append(lines, receiptCenter(banner, width))
	}
	lines = append(lines, rule)
	for _, item := range input.Items {
		lines = append(lines, receiptTwoColumn(
			fmt.Sprintf("%d x %s", item.Qty, item.Name),
			FormatZAR(item.Qty*item.UnitPrice),
			width,
		))
		if item.LineDiscount > 0 {
			lines = append(lines, receiptTwoColumn("   discount", "-"+FormatZAR(item.LineDiscount), width))
		}
	}
	lines = append(lines, rule)
	lines = append(lines, receiptTwoColumn("Subtotal", FormatZAR(input.Subtotal), width))
	if input.Discount > 0 {
		lines = append(lines, receiptTwoColumn("Discount", "-"+FormatZAR(input.Discount), width))
	}
	if input.PointsDisc > 0 {
		lines = append(lines, receiptTwoColumn(
			fmt.Sprintf("Points (%d)", input.PointsRedeem),
			"-"+FormatZAR(input.PointsDisc),
			width,
		))
	}
	lines = append(lines, receiptTwoColumn("TOTAL", FormatZAR(input.Total), width))
	if input.RefundedTotal > 0 {
		lines = append(lines, receiptTwoColumn("Refunded", "-"+FormatZAR(input.RefundedTotal), width))
		lines = append(lines, receiptTwoColumn("Net", FormatZAR(input.Total-input.RefundedTotal), width))
	}
	lines = append(lines, rule)
	lines = append(lines, receiptTwoColumn("Paid with", formatCrypto(input.CryptoAmount, input.PaymentMethod, input.CryptoAsset), width))
	if txHash := strings.TrimSpace(input.TxHash); txHash != "" {
		lines = append(lines, "Tx:")
		for i := 0; i < len(txHash); i += width {
			end := i + width
			if end > len(txHash) {
				end = len(txHash)
			}
			lines = append(lines, txHash[i:end])
		}
	}
	if reason := strings.TrimSpace(input.FailureReason); reason != "" {
		lines = append(lines, "Reason: "+reason)
	}
	if input.Status != "failed" && input.Status != "voided" {
		lines = append(lines, rule, receiptTwoColumn("Points earned", fmt.Sprintf("%d", input.PointsEarned), width))
	}
	lines = append(lines, rule, receiptCenter("Thank you!", width))
	return strings.Join(lines, "\n")
}

// BuildRefundReceipt renders the plain-text refund slip.
func BuildRefundReceipt(input RefundReceiptInput, opts ReceiptOptions) string {
	width := receiptWidth(opts.Width)
	rule := strings.Repeat("-", width)
	lines := make([]string, 0, 20)

	merchant := strings.TrimSpace(opts.Merchant)
	if merchant == "" {
		merchant = "CRYPTO POS"
	}
	lines = append(lines, receiptCenter(strings.ToUpper(merchant), width))
	if address := strings.TrimSpace(opts.Address); address != "" {
		lines = append(lines, receiptCenter(address, width))
	}
	if website := strings.TrimSpace(opts.Website); website != "" {
		lines = append(lines, receiptCenter(website, width))
	}
	lines = append(lines, receiptCenter("REFUND / RETURN", width), rule)
	lines = append(lines, receiptTwoColumn("Original receipt", input.ReceiptNo, width))
	lines = append(lines, receiptTwoColumn("Date", FormatDateTime(input.RefundedAt, opts.Location), width), rule)
	for _, item := range input.Items {
		lines = append(lines, receiptTwoColumn(fmt.Sprintf("%d x %s", item.Qty, item.Name), FormatZAR(item.Amount), width))
	}
	lines = append(lines, rule)
	lines = append(lines, receiptTwoColumn("TOTAL REFUND", FormatZAR(input.Amount), width))
	returned := "No"
	if input.Restocked {
		returned = "Yes"
	}
	lines = append(lines, receiptTwoColumn("Returned to stock", returned, width))
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		lines = append(lines, "Reason: "+reason)
	}
	return strings.Join(lines, "\n")
}

// formatCrypto prints an asset amount with its symbol. Older sales stored a
// symbol rather than an option id, so a bare symbol is used when no asset is
// supplied.
func formatCrypto(amount, method, asset string) string {
	value := strings.TrimSpace(amount)
	if value == "" || value == "0" {
		value = "0"
	}
	symbol := strings.TrimSpace(asset)
	if symbol == "" {
		symbol = strings.ToUpper(strings.TrimSpace(method))
	}
	if symbol == "" {
		symbol = "USDC"
	}
	return value + " " + symbol
}

func receiptWidth(width int) int {
	if width <= 0 {
		return defaultReceiptWidth
	}
	return width
}

func receiptTwoColumn(left, right string, width int) string {
	leftRunes := []rune(left)
	rightLen := len([]rune(right))
	space := width - rightLen - 1
	if len(leftRunes) > space {
		cut := space - 1
		if cut < 0 {
			cut = 0
		}
		if cut > len(leftRunes) {
			cut = len(leftRunes)
		}
		leftRunes = append(leftRunes[:cut], '…')
	}
	padding := width - len(leftRunes) - rightLen
	if padding < 1 {
		padding = 1
	}
	return string(leftRunes) + strings.Repeat(" ", padding) + right
}

func receiptCenter(value string, width int) string {
	padding := (width - len([]rune(value))) / 2
	if padding < 0 {
		padding = 0
	}
	return strings.Repeat(" ", padding) + value
}
