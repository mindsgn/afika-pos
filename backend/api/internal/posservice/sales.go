package posservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/mindsgn-studio/afika-backend/api/internal/posdomain"
	"github.com/mindsgn-studio/afika-backend/api/internal/posstore"
)

// Idempotency scopes. A key is only replayed for the same operation.
const (
	scopeSaleCreate   = "sale.create"
	scopeRefundCreate = "refund.create"
)

const maxIdempotencyKeyLength = 200

// SaleItemInput is one cart line. Send the product id and quantity only: prices
// and costs are always read from the catalogue so a client cannot invent them.
type SaleItemInput struct {
	ProductID string `json:"productId"`
	Qty       int64  `json:"qty"`
}

// SaleDiscountInput is the manual discount. Percent is a whole percentage.
type SaleDiscountInput struct {
	Type  string `json:"type"`
	Value int64  `json:"value"`
}

// CreateSaleInput is a checkout. When Complete is false the sale is stored as
// pending and stock/loyalty are untouched until the payment confirms.
type CreateSaleInput struct {
	Items          []SaleItemInput   `json:"items"`
	Discount       SaleDiscountInput `json:"discount"`
	CustomerID     string            `json:"customerId"`
	RedeemPoints   int64             `json:"redeemPoints"`
	PaymentMethod  string            `json:"paymentMethod"`
	Complete       bool              `json:"complete"`
	TxHash         string            `json:"txHash"`
	IdempotencyKey string            `json:"idempotencyKey"`
}

// SaleDetail is everything a client needs to show a receipt: the sale, its
// lines, its refunds, and the loyalty customer.
type SaleDetail struct {
	Sale     posstore.Sale       `json:"sale"`
	Items    []posstore.SaleItem `json:"items"`
	Refunds  []posstore.Refund   `json:"refunds"`
	Customer *posstore.Customer  `json:"customer,omitempty"`
}

// CompleteSaleInput carries the on-chain payment details. Amounts are decimal
// strings so crypto precision is never lost.
type CompleteSaleInput struct {
	TxHash       string `json:"txHash"`
	CryptoAmount string `json:"cryptoAmount"`
	CryptoRate   string `json:"cryptoRate"`
}

// RefundItemInput asks for a quantity of one sale line to be returned.
type RefundItemInput struct {
	SaleItemID string `json:"saleItemId"`
	Qty        int64  `json:"qty"`
}

// CreateRefundInput is a full or partial return. Restock defaults to true.
type CreateRefundInput struct {
	Items          []RefundItemInput `json:"items"`
	Reason         string            `json:"reason"`
	Restock        *bool             `json:"restock"`
	IdempotencyKey string            `json:"idempotencyKey"`
}

// RefundResult is the response after a return.
type RefundResult struct {
	Refund        posstore.Refund `json:"refund"`
	SaleID        string          `json:"saleId"`
	Status        string          `json:"status"`
	RefundedTotal int64           `json:"refundedTotal"`
}

var decimalPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)

// CreateSale prices the cart server-side, records the sale, and (when Complete
// is set) deducts stock and updates loyalty in the same atomic batch.
func (s *Service) CreateSale(ctx context.Context, businessID string, input CreateSaleInput) (SaleDetail, error) {
	idem, err := newIdempotency(input.IdempotencyKey, scopeSaleCreate, input)
	if err != nil {
		return SaleDetail{}, err
	}
	if idem.enabled() {
		var stored SaleDetail
		replayed, err := s.replay(ctx, businessID, idem, &stored)
		if err != nil {
			return SaleDetail{}, err
		}
		if replayed {
			return stored, nil
		}
	}

	cart, customer, asset, err := s.priceCart(ctx, businessID, input)
	if err != nil {
		return SaleDetail{}, err
	}

	now := s.nowMS()
	receiptNo, err := s.store.AllocateReceiptNo(ctx, businessID)
	if err != nil {
		return SaleDetail{}, internalf(err)
	}
	receipt := formatReceiptNo(receiptNo)

	status := posdomain.StatusPending
	paymentStatus := "pending"
	pointsEarned := int64(0)
	cryptoAmount := "0"
	cryptoRate := "0"
	failureReason := ""
	if input.Complete {
		status = posdomain.StatusCompleted
		paymentStatus = "success"
		pointsEarned = cart.PointsEarned
		cryptoAmount, err = posdomain.CryptoAmountFor(cart.Total, asset)
		if err != nil {
			return SaleDetail{}, internalf(err)
		}
	}

	sale := posstore.Sale{
		ID:             posstore.NewID(),
		ReceiptNo:      receipt,
		CreatedAt:      now,
		UpdatedAt:      now,
		Status:         status,
		CustomerID:     customerID(customer),
		Subtotal:       cart.Subtotal,
		DiscountType:   string(cart.Discount.Type),
		DiscountValue:  storedDiscountValue(cart.Discount),
		DiscountAmount: cart.ManualDiscount,
		PointsRedeemed: cart.RedeemPoints,
		PointsDiscount: cart.PointsDiscount,
		Total:          cart.Total,
		CostTotal:      cart.CostTotal,
		PaymentMethod:  asset.ID,
		PaymentStatus:  paymentStatus,
		CryptoAmount:   cryptoAmount,
		CryptoRate:     cryptoRate,
		TxHash:         strings.TrimSpace(input.TxHash),
		FailureReason:  failureReason,
		PointsEarned:   pointsEarned,
	}
	items := make([]posstore.SaleItem, len(cart.Lines))
	for i, line := range cart.Lines {
		items[i] = posstore.SaleItem{
			ID:           posstore.NewID(),
			SaleID:       sale.ID,
			ProductID:    line.ProductID,
			SKU:          line.SKU,
			Name:         line.Name,
			Qty:          line.Qty,
			UnitPrice:    line.UnitPrice,
			UnitCost:     line.UnitCost,
			LineDiscount: line.LineDiscount,
		}
	}

	detail := SaleDetail{Sale: sale, Items: items, Refunds: []posstore.Refund{}, Customer: customer}

	writer := posstore.NewWriter()
	writer.InsertSale(businessID, sale, items)
	if input.Complete {
		for _, item := range items {
			cost := item.UnitCost
			writer.MoveStock(businessID, item.ProductID, "sale", -item.Qty, &cost, receipt, "", now)
		}
		if customer != nil {
			next := clampPoints(customer.LoyaltyPoints - cart.RedeemPoints + cart.PointsEarned)
			writer.SetLoyaltyPoints(businessID, customer.ID, customer.LoyaltyPoints, next)
		}
		detail.Customer = s.refreshedCustomer(customer, cart.RedeemPoints, cart.PointsEarned)
	}

	if err := s.commit(ctx, businessID, writer, idem, detail); err != nil {
		if idem.enabled() {
			var stored SaleDetail
			replayed, replayErr := s.replay(ctx, businessID, idem, &stored)
			if replayErr != nil {
				return SaleDetail{}, replayErr
			}
			if replayed {
				return stored, nil
			}
		}
		return SaleDetail{}, internalf(err)
	}
	return detail, nil
}

// priceCart merges duplicate lines, loads products, validates stock, discount,
// loyalty redemption and the payment option, then prices the cart with the same
// rules as the app.
func (s *Service) priceCart(ctx context.Context, businessID string, input CreateSaleInput) (posdomain.PricedCart, *posstore.Customer, posdomain.Asset, error) {
	if len(input.Items) == 0 {
		return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeCartEmpty, "The cart is empty")
	}
	if input.RedeemPoints < 0 {
		return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeInsufficientPoints, "Invalid points amount")
	}

	order := make([]string, 0, len(input.Items))
	quantities := make(map[string]int64, len(input.Items))
	for _, item := range input.Items {
		productID := strings.TrimSpace(item.ProductID)
		if productID == "" {
			return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeProductNotFound, "A product in the cart no longer exists")
		}
		if item.Qty <= 0 {
			return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeInvalidQuantity, "Quantities must be whole numbers above zero")
		}
		if _, seen := quantities[productID]; !seen {
			order = append(order, productID)
		}
		quantities[productID] += item.Qty
	}
	if len(order) > 50 {
		return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeCartTooLarge, "A cart cannot contain more than 50 distinct products")
	}

	products, err := s.store.GetProductsByIDs(ctx, businessID, order)
	if err != nil {
		return posdomain.PricedCart{}, nil, posdomain.Asset{}, internalf(err)
	}
	if len(products) != len(order) {
		return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeProductNotFound, "A product in the cart no longer exists")
	}

	lines := make([]posdomain.PriceLine, 0, len(order))
	for _, id := range order {
		product := products[id]
		qty := quantities[id]
		if !product.Active {
			return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeProductInactive, "%s is no longer for sale", product.Name)
		}
		if qty > product.Stock {
			return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeInsufficientStock, "Only %d of %s in stock", product.Stock, product.Name)
		}
		lines = append(lines, posdomain.PriceLine{
			ProductID: product.ID,
			SKU:       product.SKU,
			Name:      product.Name,
			Qty:       qty,
			UnitPrice: product.Price,
			UnitCost:  product.CostPrice,
		})
	}

	discount, err := parseDiscount(input.Discount)
	if err != nil {
		return posdomain.PricedCart{}, nil, posdomain.Asset{}, err
	}
	asset, err := posdomain.ValidateAssetID(input.PaymentMethod)
	if err != nil {
		return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeInvalidPayment, "%s", err.Error())
	}

	var customer *posstore.Customer
	if id := strings.TrimSpace(input.CustomerID); id != "" {
		found, err := s.store.GetCustomer(ctx, businessID, id)
		if err != nil {
			return posdomain.PricedCart{}, nil, posdomain.Asset{}, mapNotFound(err, CodeCustomerNotFound, "Customer not found")
		}
		customer = &found
	}
	if input.RedeemPoints > 0 {
		if customer == nil {
			return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeCustomerRequired, "Select a customer to redeem points")
		}
		if input.RedeemPoints > customer.LoyaltyPoints {
			return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeInsufficientPoints, "Customer does not have enough points")
		}
	}

	cart, err := posdomain.PriceLines(lines, discount, input.RedeemPoints)
	if err != nil {
		return posdomain.PricedCart{}, nil, posdomain.Asset{}, invalidf(CodeInvalidCart, "%s", err.Error())
	}
	return cart, customer, asset, nil
}

// parseDiscount accepts an omitted discount (none) and rejects unknown types.
func parseDiscount(input SaleDiscountInput) (posdomain.Discount, error) {
	kind := posdomain.DiscountType(strings.TrimSpace(input.Type))
	if kind == "" {
		kind = posdomain.DiscountNone
	}
	discount := posdomain.Discount{Type: kind, Value: input.Value}
	if !discount.Valid() {
		return posdomain.Discount{}, invalidf(CodeInvalidDiscount, "Discount type must be none, percent, or fixed")
	}
	return discount, nil
}

func storedDiscountValue(discount posdomain.Discount) int64 {
	if discount.Type == posdomain.DiscountNone {
		return 0
	}
	return discount.Value
}

// ListSales returns sales newest first.
func (s *Service) ListSales(ctx context.Context, businessID string, filter posstore.SaleFilter) ([]posstore.Sale, error) {
	sales, err := s.store.ListSales(ctx, businessID, filter)
	if err != nil {
		return nil, internalf(err)
	}
	return sales, nil
}

// GetSale returns a sale with its lines, refunds, and customer.
func (s *Service) GetSale(ctx context.Context, businessID, saleID string) (SaleDetail, error) {
	return s.saleDetail(ctx, businessID, saleID)
}

func (s *Service) saleDetail(ctx context.Context, businessID, saleID string) (SaleDetail, error) {
	sale, err := s.store.GetSale(ctx, businessID, saleID)
	if err != nil {
		return SaleDetail{}, mapNotFound(err, CodeSaleNotFound, "Sale not found")
	}
	items, err := s.store.ListSaleItems(ctx, sale.ID)
	if err != nil {
		return SaleDetail{}, internalf(err)
	}
	refunds, err := s.store.ListRefunds(ctx, businessID, sale.ID)
	if err != nil {
		return SaleDetail{}, internalf(err)
	}
	for i := range refunds {
		lines, err := s.store.ListRefundLines(ctx, refunds[i].ID)
		if err != nil {
			return SaleDetail{}, internalf(err)
		}
		refunds[i].Items = lines
	}
	detail := SaleDetail{Sale: sale, Items: items, Refunds: refunds}
	if sale.CustomerID != "" {
		customer, err := s.store.GetCustomer(ctx, businessID, sale.CustomerID)
		if err == nil {
			detail.Customer = &customer
		} else if !errors.Is(err, posstore.ErrNotFound) {
			return SaleDetail{}, internalf(err)
		}
	}
	return detail, nil
}

// CompleteSale settles a pending sale exactly once. Repeating the call for an
// already completed sale returns the sale unchanged, which makes payment
// confirmations safe to retry.
func (s *Service) CompleteSale(ctx context.Context, businessID, saleID string, input CompleteSaleInput) (SaleDetail, error) {
	detail, err := s.saleDetail(ctx, businessID, saleID)
	if err != nil {
		return SaleDetail{}, err
	}
	if detail.Sale.Status == posdomain.StatusCompleted {
		return detail, nil
	}
	if detail.Sale.Status != posdomain.StatusPending {
		return SaleDetail{}, conflictf(CodeSaleNotPending, "Sale is %s and cannot be completed", detail.Sale.Status)
	}

	txHash := strings.TrimSpace(input.TxHash)
	if len(txHash) > 200 {
		return SaleDetail{}, invalidf(CodeInvalidPayment, "Transaction hash is too long")
	}
	cryptoAmount, err := s.cryptoAmountFor(detail.Sale, input.CryptoAmount)
	if err != nil {
		return SaleDetail{}, err
	}
	cryptoRate, err := s.cryptoRateFor(detail.Sale, input.CryptoRate)
	if err != nil {
		return SaleDetail{}, err
	}

	earned := posdomain.PointsEarned(detail.Sale.Total)
	now := s.nowMS()

	writer := posstore.NewWriter()
	writer.CompleteSale(businessID, saleID, txHash, cryptoAmount, cryptoRate, earned, now)
	for _, item := range detail.Items {
		cost := item.UnitCost
		writer.MoveStock(businessID, item.ProductID, "sale", -item.Qty, &cost, detail.Sale.ReceiptNo, "", now)
	}
	if detail.Customer != nil {
		next := clampPoints(detail.Customer.LoyaltyPoints - detail.Sale.PointsRedeemed + earned)
		writer.SetLoyaltyPoints(businessID, detail.Customer.ID, detail.Customer.LoyaltyPoints, next)
	}
	if err := s.store.Write(ctx, writer); err != nil {
		// A concurrent confirmation may have won the race; the conditional
		// update would then abort this batch. Report the settled sale instead.
		if current, readErr := s.saleDetail(ctx, businessID, saleID); readErr == nil &&
			current.Sale.Status == posdomain.StatusCompleted {
			return current, nil
		}
		return SaleDetail{}, internalf(err)
	}
	return s.saleDetail(ctx, businessID, saleID)
}

// cryptoAmountFor prefers the amount reported by the payment watcher, then the
// amount already stored on the sale, and finally computes it from the total.
func (s *Service) cryptoAmountFor(sale posstore.Sale, provided string) (string, error) {
	if value := strings.TrimSpace(provided); value != "" {
		if len(value) > 64 || !decimalPattern.MatchString(value) {
			return "", invalidf(CodeInvalidPayment, "Crypto amount must be a positive decimal")
		}
		return value, nil
	}
	if strings.TrimSpace(sale.CryptoAmount) != "" && sale.CryptoAmount != "0" {
		return sale.CryptoAmount, nil
	}
	asset := posdomain.AssetByID(sale.PaymentMethod)
	amount, err := posdomain.CryptoAmountFor(sale.Total, asset)
	if err != nil {
		return "", internalf(err)
	}
	return amount, nil
}

func (s *Service) cryptoRateFor(sale posstore.Sale, provided string) (string, error) {
	if value := strings.TrimSpace(provided); value != "" {
		if len(value) > 64 || !decimalPattern.MatchString(value) {
			return "", invalidf(CodeInvalidPayment, "Crypto rate must be a positive decimal")
		}
		return value, nil
	}
	if strings.TrimSpace(sale.CryptoRate) != "" {
		return sale.CryptoRate, nil
	}
	return "0", nil
}

// FailSale marks a pending payment failed. Sales that already left pending are
// returned unchanged, mirroring the app's failSale.
func (s *Service) FailSale(ctx context.Context, businessID, saleID, reason string) (SaleDetail, error) {
	detail, err := s.saleDetail(ctx, businessID, saleID)
	if err != nil {
		return SaleDetail{}, err
	}
	if detail.Sale.Status != posdomain.StatusPending {
		return detail, nil
	}
	writer := posstore.NewWriter()
	writer.FailSale(businessID, saleID, strings.TrimSpace(reason), s.nowMS())
	if err := s.store.Write(ctx, writer); err != nil {
		if current, readErr := s.saleDetail(ctx, businessID, saleID); readErr == nil &&
			current.Sale.Status != posdomain.StatusPending {
			return current, nil
		}
		return SaleDetail{}, internalf(err)
	}
	return s.saleDetail(ctx, businessID, saleID)
}

// VoidSale cancels a completed sale that has no refunds. Stock returns and
// loyalty points are reversed.
func (s *Service) VoidSale(ctx context.Context, businessID, saleID, reason string) (SaleDetail, error) {
	detail, err := s.saleDetail(ctx, businessID, saleID)
	if err != nil {
		return SaleDetail{}, err
	}
	switch detail.Sale.Status {
	case posdomain.StatusVoided:
		return SaleDetail{}, conflictf(CodeSaleAlreadyVoided, "This sale is already voided")
	case posdomain.StatusCompleted:
		// Only completed sales without refunds can be voided. A refund moves
		// the status, so completed is exactly the voidable state.
	default:
		return SaleDetail{}, conflictf(CodeSaleNotVoidable, "Only completed sales without refunds can be voided")
	}

	reason = strings.TrimSpace(reason)
	voidReason := reason
	if voidReason == "" {
		voidReason = "No reason given"
	}
	now := s.nowMS()

	writer := posstore.NewWriter()
	for _, item := range detail.Items {
		cost := item.UnitCost
		writer.MoveStock(businessID, item.ProductID, "void", item.Qty, &cost, detail.Sale.ReceiptNo, reason, now)
	}
	if detail.Customer != nil {
		next := posdomain.VoidPoints(detail.Customer.LoyaltyPoints, detail.Sale.PointsRedeemed, detail.Sale.PointsEarned)
		writer.SetLoyaltyPoints(businessID, detail.Customer.ID, detail.Customer.LoyaltyPoints, next)
	}
	writer.VoidSale(businessID, saleID, voidReason, now)
	if err := s.store.Write(ctx, writer); err != nil {
		if current, readErr := s.saleDetail(ctx, businessID, saleID); readErr == nil &&
			current.Sale.Status == posdomain.StatusVoided {
			return current, nil
		}
		return SaleDetail{}, internalf(err)
	}
	return s.saleDetail(ctx, businessID, saleID)
}

// CreateRefund records a full or partial return. Restocked items go back to
// stock and recover their cost; written-off items keep their cost in COGS.
func (s *Service) CreateRefund(ctx context.Context, businessID, saleID string, input CreateRefundInput) (RefundResult, error) {
	idem, err := newIdempotency(input.IdempotencyKey, scopeRefundCreate, input)
	if err != nil {
		return RefundResult{}, err
	}
	if idem.enabled() {
		var stored RefundResult
		replayed, err := s.replay(ctx, businessID, idem, &stored)
		if err != nil {
			return RefundResult{}, err
		}
		if replayed {
			return stored, nil
		}
	}

	requested := make([]RefundItemInput, 0, len(input.Items))
	seen := make(map[string]bool, len(input.Items))
	for _, item := range input.Items {
		if item.Qty <= 0 {
			continue
		}
		if seen[item.SaleItemID] {
			return RefundResult{}, invalidf(CodeRefundDuplicate, "Each item can only appear once in a refund")
		}
		seen[item.SaleItemID] = true
		requested = append(requested, item)
	}
	if len(requested) == 0 {
		return RefundResult{}, invalidf(CodeRefundEmpty, "Choose at least one item to refund")
	}

	detail, err := s.saleDetail(ctx, businessID, saleID)
	if err != nil {
		return RefundResult{}, err
	}
	if detail.Sale.Status != posdomain.StatusCompleted && detail.Sale.Status != posdomain.StatusPartiallyRefunded {
		return RefundResult{}, conflictf(CodeSaleNotRefundable, "This sale cannot be refunded")
	}

	restock := true
	if input.Restock != nil {
		restock = *input.Restock
	}
	reason := strings.TrimSpace(input.Reason)

	byID := make(map[string]posstore.SaleItem, len(detail.Items))
	for _, item := range detail.Items {
		byID[item.ID] = item
	}

	type pendingLine struct {
		item   posstore.SaleItem
		qty    int64
		amount int64
	}
	pending := make([]pendingLine, 0, len(requested))
	var total int64
	for _, req := range requested {
		item, ok := byID[req.SaleItemID]
		if !ok {
			return RefundResult{}, invalidf(CodeRefundItemNotFound, "Item does not belong to this sale")
		}
		remaining := item.Qty - item.RefundedQty
		if req.Qty > remaining {
			return RefundResult{}, invalidf(CodeRefundQtyExceeds, "Only %d of %s left to refund", remaining, item.Name)
		}
		amount := posdomain.RefundLineAmount(item.UnitPrice, item.LineDiscount, item.Qty, item.RefundedQty, item.RefundedAmount, req.Qty)
		total += amount
		pending = append(pending, pendingLine{item: item, qty: req.Qty, amount: amount})
	}

	var alreadyReversed int64
	for _, refund := range detail.Refunds {
		alreadyReversed += refund.PointsReversed
	}
	reverse := posdomain.ReversePointsForRefund(detail.Sale.PointsEarned, total, detail.Sale.Total, alreadyReversed)

	fullyRefunded := true
	for _, item := range detail.Items {
		requestedQty := int64(0)
		for _, line := range pending {
			if line.item.ID == item.ID {
				requestedQty = line.qty
				break
			}
		}
		if item.RefundedQty+requestedQty != item.Qty {
			fullyRefunded = false
			break
		}
	}
	nextStatus := posdomain.StatusPartiallyRefunded
	if fullyRefunded {
		nextStatus = posdomain.StatusRefunded
	}

	now := s.nowMS()
	refund := posstore.Refund{
		ID:             posstore.NewID(),
		SaleID:         saleID,
		CreatedAt:      now,
		Amount:         total,
		Reason:         reason,
		Restocked:      restock,
		PointsReversed: reverse,
	}
	lines := make([]posstore.RefundLine, len(pending))
	for i, line := range pending {
		costRecovered := int64(0)
		if restock {
			costRecovered = line.item.UnitCost * line.qty
		}
		lines[i] = posstore.RefundLine{
			SaleItemID:    line.item.ID,
			Name:          line.item.Name,
			Qty:           line.qty,
			Amount:        line.amount,
			CostRecovered: costRecovered,
		}
	}
	refund.Items = lines

	result := RefundResult{
		Refund:        refund,
		SaleID:        saleID,
		Status:        nextStatus,
		RefundedTotal: detail.Sale.RefundedTotal + total,
	}

	writer := posstore.NewWriter()
	writer.InsertRefund(businessID, refund)
	for _, line := range pending {
		writer.InsertRefundLine(refund.ID, posstore.RefundLine{
			SaleItemID:    line.item.ID,
			Name:          line.item.Name,
			Qty:           line.qty,
			Amount:        line.amount,
			CostRecovered: costFor(restock, line.item.UnitCost, line.qty),
		})
		writer.AddRefundedQty(line.item.ID, line.qty, line.amount)
		if restock {
			cost := line.item.UnitCost
			writer.MoveStock(businessID, line.item.ProductID, "refund", line.qty, &cost, detail.Sale.ReceiptNo, reason, now)
		}
	}
	if detail.Customer != nil && reverse > 0 {
		next := clampPoints(detail.Customer.LoyaltyPoints - reverse)
		writer.SetLoyaltyPoints(businessID, detail.Customer.ID, detail.Customer.LoyaltyPoints, next)
	}
	writer.UpdateRefundTotals(businessID, saleID, total, nextStatus, now)

	if err := s.commit(ctx, businessID, writer, idem, result); err != nil {
		if idem.enabled() {
			var stored RefundResult
			replayed, replayErr := s.replay(ctx, businessID, idem, &stored)
			if replayErr != nil {
				return RefundResult{}, replayErr
			}
			if replayed {
				return stored, nil
			}
		}
		return RefundResult{}, internalf(err)
	}
	return result, nil
}

func costFor(restock bool, unitCost, qty int64) int64 {
	if !restock {
		return 0
	}
	return unitCost * qty
}

// SaleReceipt renders the plain-text receipt for a sale.
func (s *Service) SaleReceipt(ctx context.Context, businessID, saleID string) (string, error) {
	detail, err := s.saleDetail(ctx, businessID, saleID)
	if err != nil {
		return "", err
	}
	business, err := s.store.GetBusinessByID(ctx, businessID)
	if err != nil {
		return "", mapNotFound(err, CodeBusinessNotFound, "Business not found")
	}
	asset := posdomain.AssetByID(detail.Sale.PaymentMethod)
	input := posdomain.ReceiptInput{
		ReceiptNo:     detail.Sale.ReceiptNo,
		CreatedAt:     detail.Sale.CreatedAt,
		Status:        detail.Sale.Status,
		Items:         make([]posdomain.ReceiptItem, 0, len(detail.Items)),
		Subtotal:      detail.Sale.Subtotal,
		Discount:      detail.Sale.DiscountAmount,
		PointsDisc:    detail.Sale.PointsDiscount,
		Total:         detail.Sale.Total,
		PaymentMethod: detail.Sale.PaymentMethod,
		CryptoAmount:  detail.Sale.CryptoAmount,
		CryptoAsset:   asset.Asset,
		TxHash:        detail.Sale.TxHash,
		FailureReason: detail.Sale.FailureReason,
		PointsEarned:  detail.Sale.PointsEarned,
		PointsRedeem:  detail.Sale.PointsRedeemed,
		RefundedTotal: detail.Sale.RefundedTotal,
	}
	if detail.Customer != nil {
		input.CustomerName = detail.Customer.Name
	}
	for _, item := range detail.Items {
		input.Items = append(input.Items, posdomain.ReceiptItem{
			Name:         item.Name,
			Qty:          item.Qty,
			UnitPrice:    item.UnitPrice,
			LineDiscount: item.LineDiscount,
		})
	}
	return posdomain.BuildReceipt(input, s.receiptOptions(business)), nil
}

// RefundReceipt renders the plain-text refund slip for one refund of a sale.
func (s *Service) RefundReceipt(ctx context.Context, businessID, saleID, refundID string) (string, error) {
	detail, err := s.saleDetail(ctx, businessID, saleID)
	if err != nil {
		return "", err
	}
	var refund *posstore.Refund
	for i := range detail.Refunds {
		if detail.Refunds[i].ID == refundID {
			refund = &detail.Refunds[i]
			break
		}
	}
	if refund == nil {
		return "", notFoundf(CodeRefundNotFound, "Refund not found")
	}
	business, err := s.store.GetBusinessByID(ctx, businessID)
	if err != nil {
		return "", mapNotFound(err, CodeBusinessNotFound, "Business not found")
	}
	input := posdomain.RefundReceiptInput{
		ReceiptNo:  detail.Sale.ReceiptNo,
		RefundedAt: refund.CreatedAt,
		Items:      make([]posdomain.RefundReceiptItem, 0, len(refund.Items)),
		Amount:     refund.Amount,
		Reason:     refund.Reason,
		Restocked:  refund.Restocked,
	}
	for _, line := range refund.Items {
		input.Items = append(input.Items, posdomain.RefundReceiptItem{Name: line.Name, Qty: line.Qty, Amount: line.Amount})
	}
	return posdomain.BuildRefundReceipt(input, s.receiptOptions(business)), nil
}

func (s *Service) receiptOptions(business posstore.Business) posdomain.ReceiptOptions {
	return posdomain.ReceiptOptions{
		Merchant: business.Name,
		Website:  business.Website,
		Address:  business.Address,
		Location: s.location,
	}
}

// Profit returns the Insights report for a date range.
func (s *Service) Profit(ctx context.Context, businessID string, from, to int64) (posdomain.ProfitSummary, error) {
	sales, err := s.store.ListSaleTotals(ctx, businessID, from, to)
	if err != nil {
		return posdomain.ProfitSummary{}, internalf(err)
	}
	refundCount, recovered, err := s.store.RefundAggregates(ctx, businessID, from, to)
	if err != nil {
		return posdomain.ProfitSummary{}, internalf(err)
	}
	return posdomain.ComputeProfit(posdomain.ProfitInput{
		Sales:         sales,
		RecoveredCost: recovered,
		RefundCount:   refundCount,
	}), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// idempotency is the normalised idempotency context of one request.
type idempotency struct {
	key   string
	scope string
	hash  string
}

func (i idempotency) enabled() bool { return i.key != "" }

func newIdempotency(key, scope string, payload any) (idempotency, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return idempotency{}, nil
	}
	if len(key) > maxIdempotencyKeyLength {
		return idempotency{}, invalidf(CodeInvalidIdempotency, "Idempotency key must be at most %d characters", maxIdempotencyKeyLength)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return idempotency{}, internalf(err)
	}
	sum := sha256.Sum256(raw)
	return idempotency{key: key, scope: scope, hash: hex.EncodeToString(sum[:])}, nil
}

// replay loads a previously stored response for the key. It fails when the key
// was used for a different scope or payload.
func (s *Service) replay(ctx context.Context, businessID string, idem idempotency, target any) (bool, error) {
	record, ok, err := s.store.GetIdempotencyRecord(ctx, businessID, idem.key)
	if err != nil {
		return false, internalf(err)
	}
	if !ok {
		return false, nil
	}
	if record.Scope != idem.scope || record.RequestHash != idem.hash {
		return false, conflictf(CodeIdempotencyKeyReused, "That idempotency key was already used for a different request")
	}
	if err := json.Unmarshal([]byte(record.Response), target); err != nil {
		return false, internalf(fmt.Errorf("decode idempotent response: %w", err))
	}
	return true, nil
}

// commit runs the write batch and records the response under the idempotency
// key inside the same batch. The guard after the insert aborts the batch when
// another request already claimed the key, so a retry can never apply twice.
func (s *Service) commit(ctx context.Context, businessID string, writer *posstore.Writer, idem idempotency, response any) error {
	if idem.enabled() {
		raw, err := json.Marshal(response)
		if err != nil {
			return fmt.Errorf("encode idempotent response: %w", err)
		}
		writer.InsertIdempotency(businessID, idem.key, idem.scope, idem.hash, string(raw), s.nowMS())
		writer.ExpectOne()
	}
	return s.store.Write(ctx, writer)
}

func (s *Service) refreshedCustomer(customer *posstore.Customer, redeemed, earned int64) *posstore.Customer {
	if customer == nil {
		return nil
	}
	next := *customer
	next.LoyaltyPoints = clampPoints(next.LoyaltyPoints - redeemed + earned)
	return &next
}

func customerID(customer *posstore.Customer) string {
	if customer == nil {
		return ""
	}
	return customer.ID
}

func clampPoints(points int64) int64 {
	if points < 0 {
		return 0
	}
	return points
}

func formatReceiptNo(n int64) string {
	return fmt.Sprintf("R-%06d", n)
}

// ValidateIdempotencyKey lets the HTTP layer reject an oversized key before it
// touches a store.
func ValidateIdempotencyKey(key string) error {
	_, err := newIdempotency(key, "validate", struct{}{})
	return err
}
