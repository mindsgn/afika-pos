package posstore

import "strings"

// This file holds every INSERT/UPDATE/DELETE the POS issues. Keeping the SQL in
// one package means the service layer can stay focused on business rules, and
// the store integration tests can exercise each write against real SQLite.

// UpsertBusiness creates or updates the business owned by ownerID. The owner
// column is unique, so a merchant can never end up with two profiles.
func (w *Writer) UpsertBusiness(ownerID string, b Business) {
	w.Exec(
		`INSERT INTO pos_businesses (id, owner_id, name, website, address, wallet_address, logo_uri, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (owner_id) DO UPDATE SET
			name = excluded.name,
			website = excluded.website,
			address = excluded.address,
			wallet_address = excluded.wallet_address,
			logo_uri = excluded.logo_uri,
			updated_at = excluded.updated_at`,
		b.ID, ownerID, b.Name, b.Website, b.Address, b.WalletAddress,
		nullableText(b.LogoURI), b.CreatedAt, b.UpdatedAt,
	)
}

// InsertCategory adds a category.
func (w *Writer) InsertCategory(businessID string, c Category) {
	w.Exec(
		`INSERT INTO pos_categories (id, business_id, name, created_at) VALUES (?, ?, ?, ?)`,
		c.ID, businessID, c.Name, c.CreatedAt,
	)
}

// RenameCategory changes a category name.
func (w *Writer) RenameCategory(businessID, id, name string) {
	w.Exec(
		`UPDATE pos_categories SET name = ? WHERE business_id = ? AND id = ?`,
		name, businessID, id,
	)
	w.ExpectOne()
}

// DeleteCategory removes an empty category.
func (w *Writer) DeleteCategory(businessID, id string) {
	w.Exec(`DELETE FROM pos_categories WHERE business_id = ? AND id = ?`, businessID, id)
	w.ExpectOne()
}

// InsertProduct adds a product with an opening stock of zero. Opening stock is
// recorded as a stock movement by MoveStock.
func (w *Writer) InsertProduct(businessID string, p Product) {
	w.Exec(
		`INSERT INTO pos_products (id, business_id, sku, name, image_uri, category_id, price,
			cost_price, stock, reorder_level, active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?)`,
		p.ID, businessID, p.SKU, p.Name, nullableText(p.ImageURI), nullableText(p.CategoryID),
		p.Price, p.CostPrice, p.ReorderLevel, boolInt(p.Active), p.CreatedAt, p.UpdatedAt,
	)
}

// UpdateProduct edits a product's details. Stock is intentionally not writable
// here: it only changes through stock movements so history stays complete.
func (w *Writer) UpdateProduct(businessID string, p Product) {
	w.Exec(
		`UPDATE pos_products SET sku = ?, name = ?, image_uri = ?, category_id = ?, price = ?,
			cost_price = ?, reorder_level = ?, updated_at = ?
		 WHERE business_id = ? AND id = ?`,
		p.SKU, p.Name, nullableText(p.ImageURI), nullableText(p.CategoryID), p.Price,
		p.CostPrice, p.ReorderLevel, p.UpdatedAt, businessID, p.ID,
	)
	w.ExpectOne()
}

// SetProductActive archives or restores a product.
func (w *Writer) SetProductActive(businessID, id string, active bool, now int64) {
	w.Exec(
		`UPDATE pos_products SET active = ?, updated_at = ? WHERE business_id = ? AND id = ?`,
		boolInt(active), now, businessID, id,
	)
	w.ExpectOne()
}

// InsertCustomer adds a loyalty customer.
func (w *Writer) InsertCustomer(businessID string, c Customer) {
	w.Exec(
		`INSERT INTO pos_customers (id, business_id, name, phone, loyalty_points, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, businessID, c.Name, nullableText(c.Phone), c.LoyaltyPoints, c.CreatedAt,
	)
}

// UpdateCustomer changes a customer's name and phone. Loyalty points are only
// changed through sale/refund/void, which compare-and-set the balance.
func (w *Writer) UpdateCustomer(businessID, id, name, phone string) {
	w.Exec(
		`UPDATE pos_customers SET name = ?, phone = ? WHERE business_id = ? AND id = ?`,
		name, nullableText(phone), businessID, id,
	)
	w.ExpectOne()
}

// MoveStock applies a signed stock change and writes the matching movement. The
// balance_after value is read from the row after the update, inside the same
// batch. The guard aborts the batch when the product does not exist for this
// business, and the stock CHECK aborts when a sale would oversell.
func (w *Writer) MoveStock(businessID, productID, movementType string, delta int64, unitCost *int64, reference, note string, now int64) {
	w.Exec(
		`UPDATE pos_products SET stock = stock + ?, updated_at = ? WHERE business_id = ? AND id = ?`,
		delta, now, businessID, productID,
	)
	w.ExpectOne()
	w.Exec(
		`INSERT INTO pos_stock_movements (id, business_id, product_id, type, quantity, balance_after,
			unit_cost, reference, note, created_at)
		 VALUES (?, ?, ?, ?, ?, (SELECT stock FROM pos_products WHERE id = ?), ?, ?, ?, ?)`,
		newID(), businessID, productID, movementType, delta, productID,
		unitCost, nullableText(reference), nullableText(note), now,
	)
}

// SetProductCost updates the cost price recorded for a restock.
func (w *Writer) SetProductCost(businessID, productID string, cost int64, now int64) {
	w.Exec(
		`UPDATE pos_products SET cost_price = ?, updated_at = ? WHERE business_id = ? AND id = ?`,
		cost, now, businessID, productID,
	)
	w.ExpectOne()
}

// InsertSale writes a sale and its lines.
func (w *Writer) InsertSale(businessID string, s Sale, items []SaleItem) {
	w.Exec(
		`INSERT INTO pos_sales (id, business_id, receipt_no, created_at, updated_at, status, customer_id,
			subtotal, discount_type, discount_value, discount_amount, points_redeemed, points_discount,
			total, cost_total, payment_method, payment_status, crypto_amount, crypto_rate, tx_hash,
			failure_reason, points_earned, refunded_total)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, businessID, s.ReceiptNo, s.CreatedAt, s.UpdatedAt, s.Status, nullableText(s.CustomerID),
		s.Subtotal, s.DiscountType, s.DiscountValue, s.DiscountAmount, s.PointsRedeemed, s.PointsDiscount,
		s.Total, s.CostTotal, s.PaymentMethod, s.PaymentStatus, s.CryptoAmount, s.CryptoRate,
		nullableText(s.TxHash), nullableText(s.FailureReason), s.PointsEarned, s.RefundedTotal,
	)
	for _, item := range items {
		w.Exec(
			`INSERT INTO pos_sale_items (id, sale_id, product_id, sku, name, qty, unit_price, unit_cost,
				line_discount, refunded_qty, refunded_amount)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0)`,
			item.ID, s.ID, item.ProductID, item.SKU, item.Name, item.Qty,
			item.UnitPrice, item.UnitCost, item.LineDiscount,
		)
	}
}

// CompleteSale flips a pending sale to completed. The WHERE clause makes the
// transition happen at most once even if two requests race.
func (w *Writer) CompleteSale(businessID, id, txHash, cryptoAmount, cryptoRate string, pointsEarned, now int64) {
	w.Exec(
		`UPDATE pos_sales SET status = 'completed', payment_status = 'success', tx_hash = ?,
			crypto_amount = ?, crypto_rate = ?, points_earned = ?, failure_reason = NULL, updated_at = ?
		 WHERE business_id = ? AND id = ? AND status = 'pending'`,
		nullableText(txHash), cryptoAmount, cryptoRate, pointsEarned, now, businessID, id,
	)
	w.ExpectOne()
}

// FailSale flips a pending sale to failed.
func (w *Writer) FailSale(businessID, id, reason string, now int64) {
	w.Exec(
		`UPDATE pos_sales SET status = 'failed', payment_status = 'failed', failure_reason = ?, updated_at = ?
		 WHERE business_id = ? AND id = ? AND status = 'pending'`,
		nullableText(reason), now, businessID, id,
	)
	w.ExpectOne()
}

// VoidSale flips a completed sale to voided.
func (w *Writer) VoidSale(businessID, id, reason string, now int64) {
	w.Exec(
		`UPDATE pos_sales SET status = 'voided', voided_at = ?, void_reason = ?, updated_at = ?
		 WHERE business_id = ? AND id = ? AND status = 'completed'`,
		now, reason, now, businessID, id,
	)
	w.ExpectOne()
}

// InsertRefund writes a refund header and its lines.
func (w *Writer) InsertRefund(businessID string, refund Refund) {
	w.Exec(
		`INSERT INTO pos_refunds (id, business_id, sale_id, created_at, amount, reason, restocked, points_reversed)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		refund.ID, businessID, refund.SaleID, refund.CreatedAt, refund.Amount,
		nullableText(refund.Reason), boolInt(refund.Restocked), refund.PointsReversed,
	)
}

// InsertRefundLine adds one line to a refund.
func (w *Writer) InsertRefundLine(refundID string, line RefundLine) {
	w.Exec(
		`INSERT INTO pos_refund_items (id, refund_id, sale_item_id, qty, amount, cost_recovered)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		newID(), refundID, line.SaleItemID, line.Qty, line.Amount, line.CostRecovered,
	)
}

// AddRefundedQty records a refund against a sale line. The guard plus the WHERE
// clause stop a line being refunded past its quantity.
func (w *Writer) AddRefundedQty(saleItemID string, qty, amount int64) {
	w.Exec(
		`UPDATE pos_sale_items SET refunded_qty = refunded_qty + ?, refunded_amount = refunded_amount + ?
		 WHERE id = ? AND refunded_qty + ? <= qty`,
		qty, amount, saleItemID, qty,
	)
	w.ExpectOne()
}

// SetRefundPointsReversed stores the loyalty points a refund clawed back.
func (w *Writer) SetRefundPointsReversed(refundID string, points int64) {
	w.Exec(`UPDATE pos_refunds SET points_reversed = ? WHERE id = ?`, points, refundID)
	w.ExpectOne()
}

// UpdateRefundTotals moves a sale to partially_refunded or refunded.
func (w *Writer) UpdateRefundTotals(businessID, id string, amount int64, status string, now int64) {
	w.Exec(
		`UPDATE pos_sales SET refunded_total = refunded_total + ?, status = ?, updated_at = ?
		 WHERE business_id = ? AND id = ? AND status IN ('completed', 'partially_refunded')`,
		amount, status, now, businessID, id,
	)
	w.ExpectOne()
}

// SetLoyaltyPoints writes an absolute loyalty balance, but only if the balance
// is still the one the caller read. A concurrent redemption aborts the batch
// instead of silently overwriting points.
func (w *Writer) SetLoyaltyPoints(businessID, customerID string, from, to int64) {
	w.Exec(
		`UPDATE pos_customers SET loyalty_points = ?
		 WHERE business_id = ? AND id = ? AND loyalty_points = ?`,
		to, businessID, customerID, from,
	)
	w.ExpectOne()
}

// InsertIdempotency records a completed write so a retry returns the original
// response instead of applying the write twice.
func (w *Writer) InsertIdempotency(businessID, key, scope, requestHash, response string, now int64) {
	w.Exec(
		`INSERT INTO pos_idempotency (business_id, idem_key, scope, request_hash, response_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (business_id, idem_key) DO NOTHING`,
		businessID, key, scope, requestHash, response, now,
	)
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
