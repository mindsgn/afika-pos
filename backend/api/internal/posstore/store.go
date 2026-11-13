package posstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mindsgn-studio/afika-backend/api/internal/posdomain"
)

// ErrNotFound is returned when a business-scoped row does not exist. Callers
// must not distinguish "does not exist" from "belongs to another business".
var ErrNotFound = errors.New("pos: not found")

// Statement is one SQL statement with its bound parameters.
type Statement struct {
	SQL  string
	Args []any
}

// BatchRunner executes a list of statements as a single transaction. The
// production Cloudflare Worker uses D1Database.batch(); native builds and tests
// use database/sql transactions.
type BatchRunner interface {
	Run(ctx context.Context, statements []Statement) error
}

// DB is the POS repository. Reads go straight to database/sql; writes are
// queued on a Writer and executed atomically by the configured BatchRunner.
type DB struct {
	db     *sql.DB
	runner BatchRunner
	now    func() time.Time
}

// New wraps an open database.
func New(db *sql.DB) *DB {
	return &DB{db: db, runner: &sqlRunner{db: db}, now: time.Now}
}

// NewWithRunner wraps an open database for reads and the supplied runner for
// atomic writes.
func NewWithRunner(db *sql.DB, runner BatchRunner) *DB {
	return &DB{db: db, runner: runner, now: time.Now}
}

// SetClock overrides the clock. It exists for deterministic tests.
func (d *DB) SetClock(now func() time.Time) { d.now = now }

// Now returns the current time according to the configured clock.
func (d *DB) Now() time.Time {
	if d.now == nil {
		return time.Now()
	}
	return d.now()
}

// SQL returns the underlying handle. It is only used by tests and health checks.
func (d *DB) SQL() *sql.DB { return d.db }

// Writer accumulates the statements of one logical write.
type Writer struct {
	statements []Statement
}

// NewWriter starts an empty write batch.
func NewWriter() *Writer { return &Writer{} }

// Exec appends a statement.
func (w *Writer) Exec(query string, args ...any) {
	w.statements = append(w.statements, Statement{SQL: query, Args: args})
}

// ExpectOne guards the preceding statement, aborting the batch when it changed
// no rows. It is how conditional updates ("only while pending") stay safe
// without interactive transactions: pos_guard has CHECK (ok = 1) and is only
// written when changes() = 0, so it stays empty on success.
func (w *Writer) ExpectOne() {
	w.Exec(`INSERT INTO pos_guard (ok) SELECT 0 WHERE changes() = 0`)
}

// Len reports how many statements are queued.
func (w *Writer) Len() int { return len(w.statements) }

// Write runs the queued statements atomically.
func (d *DB) Write(ctx context.Context, w *Writer) error {
	if len(w.statements) == 0 {
		return nil
	}
	return d.runner.Run(ctx, w.statements)
}

// sqlRunner runs a batch inside a database/sql transaction. It is used for
// native builds and tests; D1 uses its own runner.
type sqlRunner struct {
	db *sql.DB
}

func (r *sqlRunner) Run(ctx context.Context, statements []Statement) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch: %w", err)
	}
	for i, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.SQL, statement.Args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("batch statement %d: %w", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Business
// ---------------------------------------------------------------------------

const businessColumns = `id, name, website, address, wallet_address, COALESCE(logo_uri, ''), created_at, updated_at`

// GetBusinessByOwner returns the business owned by an authenticated subject.
func (d *DB) GetBusinessByOwner(ctx context.Context, ownerID string) (Business, error) {
	var b Business
	err := d.db.QueryRowContext(ctx,
		`SELECT `+businessColumns+` FROM pos_businesses WHERE owner_id = ?`,
		ownerID,
	).Scan(&b.ID, &b.Name, &b.Website, &b.Address, &b.WalletAddress, &b.LogoURI, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Business{}, ErrNotFound
	}
	if err != nil {
		return Business{}, fmt.Errorf("get business: %w", err)
	}
	return b, nil
}

// GetBusinessByID returns a business by its id.
func (d *DB) GetBusinessByID(ctx context.Context, id string) (Business, error) {
	var b Business
	err := d.db.QueryRowContext(ctx,
		`SELECT `+businessColumns+` FROM pos_businesses WHERE id = ?`, id,
	).Scan(&b.ID, &b.Name, &b.Website, &b.Address, &b.WalletAddress, &b.LogoURI, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Business{}, ErrNotFound
	}
	if err != nil {
		return Business{}, fmt.Errorf("get business: %w", err)
	}
	return b, nil
}

// ---------------------------------------------------------------------------
// Categories
// ---------------------------------------------------------------------------

// ListCategories returns the business's categories ordered by name.
func (d *DB) ListCategories(ctx context.Context, businessID string) ([]Category, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, name, created_at FROM pos_categories WHERE business_id = ? ORDER BY name ASC`,
		businessID,
	)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Category, 0, 16)
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}
	return items, nil
}

// GetCategory returns one category.
func (d *DB) GetCategory(ctx context.Context, businessID, id string) (Category, error) {
	var c Category
	err := d.db.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM pos_categories WHERE business_id = ? AND id = ?`,
		businessID, id,
	).Scan(&c.ID, &c.Name, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Category{}, ErrNotFound
	}
	if err != nil {
		return Category{}, fmt.Errorf("get category: %w", err)
	}
	return c, nil
}

// CategoryNameTaken reports whether a category name is already used by the
// business. excludeID lets a rename keep its own name.
func (d *DB) CategoryNameTaken(ctx context.Context, businessID, name, excludeID string) (bool, error) {
	return d.taken(ctx,
		`SELECT COUNT(*) FROM pos_categories WHERE business_id = ? AND name = ? AND id <> ?`,
		businessID, name, excludeID,
	)
}

// CountProductsInCategory reports how many products use a category.
func (d *DB) CountProductsInCategory(ctx context.Context, businessID, categoryID string) (int64, error) {
	var count int64
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pos_products WHERE business_id = ? AND category_id = ?`,
		businessID, categoryID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count products in category: %w", err)
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// Products
// ---------------------------------------------------------------------------

const productColumns = `p.id, p.sku, p.name, COALESCE(p.image_uri, ''), COALESCE(p.category_id, ''),
	COALESCE(c.name, ''), p.price, p.cost_price, p.stock, p.reorder_level, p.active, p.created_at, p.updated_at`

const productFrom = ` FROM pos_products p LEFT JOIN pos_categories c ON c.id = p.category_id`

// ProductFilter narrows a product list.
type ProductFilter struct {
	Query           string
	CategoryID      string
	IncludeInactive bool
	LowStockOnly    bool
	Limit           int
}

// ListProducts returns the business's products ordered by name.
func (d *DB) ListProducts(ctx context.Context, businessID string, f ProductFilter) ([]Product, error) {
	var conditions []string
	args := []any{businessID}
	conditions = append(conditions, "p.business_id = ?")
	if !f.IncludeInactive {
		conditions = append(conditions, "p.active = 1")
	}
	if f.CategoryID != "" {
		conditions = append(conditions, "p.category_id = ?")
		args = append(args, f.CategoryID)
	}
	if f.LowStockOnly {
		conditions = append(conditions, "p.active = 1 AND p.reorder_level > 0 AND p.stock <= p.reorder_level")
	}
	if query := strings.TrimSpace(f.Query); query != "" {
		like := "%" + escapeLike(query) + "%"
		conditions = append(conditions, `(p.name LIKE ? ESCAPE '\' OR p.sku LIKE ? ESCAPE '\' OR c.name LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	args = append(args, limit)

	rows, err := d.db.QueryContext(ctx,
		`SELECT `+productColumns+productFrom+` WHERE `+strings.Join(conditions, " AND ")+
			` ORDER BY p.name ASC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Product, 0, 32)
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}
	return items, nil
}

// GetProduct returns one product.
func (d *DB) GetProduct(ctx context.Context, businessID, id string) (Product, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT `+productColumns+productFrom+` WHERE p.business_id = ? AND p.id = ?`,
		businessID, id,
	)
	product, err := scanProduct(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	return product, err
}

// GetProductsByIDs loads the products of a cart in one query. The returned map
// is keyed by id; missing ids are simply absent.
func (d *DB) GetProductsByIDs(ctx context.Context, businessID string, ids []string) (map[string]Product, error) {
	out := make(map[string]Product, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if len(ids) > 50 {
		return nil, fmt.Errorf("a cart cannot contain more than 50 distinct products")
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, businessID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT `+productColumns+productFrom+
			` WHERE p.business_id = ? AND p.id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("load cart products: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out[product.ID] = product
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cart products: %w", err)
	}
	return out, nil
}

// ProductSKUTaken reports whether a SKU is already used by the business.
// excludeID lets an edit keep its own SKU.
func (d *DB) ProductSKUTaken(ctx context.Context, businessID, sku, excludeID string) (bool, error) {
	return d.taken(ctx,
		`SELECT COUNT(*) FROM pos_products WHERE business_id = ? AND sku = ? AND id <> ?`,
		businessID, sku, excludeID,
	)
}

// CustomerPhoneTaken reports whether a phone number is already registered.
func (d *DB) CustomerPhoneTaken(ctx context.Context, businessID, phone string) (bool, error) {
	return d.taken(ctx,
		`SELECT COUNT(*) FROM pos_customers WHERE business_id = ? AND phone = ?`,
		businessID, phone,
	)
}

// CustomerPhoneTakenByOther reports whether another customer already uses a
// phone number. It lets an edit keep its own number.
func (d *DB) CustomerPhoneTakenByOther(ctx context.Context, businessID, phone, excludeID string) (bool, error) {
	return d.taken(ctx,
		`SELECT COUNT(*) FROM pos_customers WHERE business_id = ? AND phone = ? AND id <> ?`,
		businessID, phone, excludeID,
	)
}

// taken runs an existence query. The excludeID-aware queries always bind three
// arguments, so the helpers above share one implementation.
func (d *DB) taken(ctx context.Context, query string, args ...any) (bool, error) {
	var count int64
	if err := d.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, fmt.Errorf("check uniqueness: %w", err)
	}
	return count > 0, nil
}

// ProductStock returns the current stock of a product.
func (d *DB) ProductStock(ctx context.Context, businessID, id string) (int64, error) {
	var stock int64
	err := d.db.QueryRowContext(ctx,
		`SELECT stock FROM pos_products WHERE business_id = ? AND id = ?`, businessID, id,
	).Scan(&stock)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get product stock: %w", err)
	}
	return stock, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProduct(row scanner) (Product, error) {
	var (
		product Product
		active  int64
	)
	if err := row.Scan(
		&product.ID, &product.SKU, &product.Name, &product.ImageURI, &product.CategoryID,
		&product.CategoryName, &product.Price, &product.CostPrice, &product.Stock,
		&product.ReorderLevel, &active, &product.CreatedAt, &product.UpdatedAt,
	); err != nil {
		return Product{}, err
	}
	product.Active = active != 0
	return product, nil
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

// ---------------------------------------------------------------------------
// Customers
// ---------------------------------------------------------------------------

// ListCustomers returns customers ordered by name, optionally filtered by a
// name or phone substring.
func (d *DB) ListCustomers(ctx context.Context, businessID, query string) ([]Customer, error) {
	sqlText := `SELECT id, name, COALESCE(phone, ''), loyalty_points, created_at
	            FROM pos_customers WHERE business_id = ?`
	args := []any{businessID}
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		like := "%" + escapeLike(trimmed) + "%"
		sqlText += ` AND (name LIKE ? ESCAPE '\' OR phone LIKE ? ESCAPE '\')`
		args = append(args, like, like)
	}
	sqlText += ` ORDER BY name ASC LIMIT 500`

	rows, err := d.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]Customer, 0, 32)
	for rows.Next() {
		customer, err := scanCustomer(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate customers: %w", err)
	}
	return items, nil
}

// GetCustomer returns one customer.
func (d *DB) GetCustomer(ctx context.Context, businessID, id string) (Customer, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(phone, ''), loyalty_points, created_at
		 FROM pos_customers WHERE business_id = ? AND id = ?`,
		businessID, id,
	)
	customer, err := scanCustomer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Customer{}, ErrNotFound
	}
	if err != nil {
		return Customer{}, fmt.Errorf("get customer: %w", err)
	}
	return customer, nil
}

func scanCustomer(row scanner) (Customer, error) {
	var customer Customer
	err := row.Scan(&customer.ID, &customer.Name, &customer.Phone, &customer.LoyaltyPoints, &customer.CreatedAt)
	return customer, err
}

// ---------------------------------------------------------------------------
// Sales
// ---------------------------------------------------------------------------

const saleColumns = `id, receipt_no, created_at, updated_at, status, COALESCE(customer_id, ''),
	subtotal, discount_type, discount_value, discount_amount, points_redeemed, points_discount,
	total, cost_total, payment_method, payment_status, crypto_amount, crypto_rate,
	COALESCE(tx_hash, ''), COALESCE(failure_reason, ''), points_earned, refunded_total,
	COALESCE(voided_at, 0), COALESCE(void_reason, '')`

// SaleFilter narrows a sale list.
type SaleFilter struct {
	Status string
	Query  string
	Limit  int
}

// ListSales returns sales newest first.
func (d *DB) ListSales(ctx context.Context, businessID string, f SaleFilter) ([]Sale, error) {
	conditions := []string{"business_id = ?"}
	args := []any{businessID}
	if status := strings.TrimSpace(f.Status); status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	if query := strings.TrimSpace(f.Query); query != "" {
		conditions = append(conditions, `receipt_no LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(query)+"%")
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args = append(args, limit)

	rows, err := d.db.QueryContext(ctx,
		`SELECT `+saleColumns+` FROM pos_sales WHERE `+strings.Join(conditions, " AND ")+
			` ORDER BY created_at DESC, id DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list sales: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]Sale, 0, 32)
	for rows.Next() {
		sale, err := scanSale(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, sale)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sales: %w", err)
	}
	return items, nil
}

// GetSale returns one sale.
func (d *DB) GetSale(ctx context.Context, businessID, id string) (Sale, error) {
	row := d.db.QueryRowContext(ctx,
		`SELECT `+saleColumns+` FROM pos_sales WHERE business_id = ? AND id = ?`,
		businessID, id,
	)
	sale, err := scanSale(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Sale{}, ErrNotFound
	}
	if err != nil {
		return Sale{}, fmt.Errorf("get sale: %w", err)
	}
	return sale, nil
}

func scanSale(row scanner) (Sale, error) {
	var sale Sale
	err := row.Scan(
		&sale.ID, &sale.ReceiptNo, &sale.CreatedAt, &sale.UpdatedAt, &sale.Status, &sale.CustomerID,
		&sale.Subtotal, &sale.DiscountType, &sale.DiscountValue, &sale.DiscountAmount,
		&sale.PointsRedeemed, &sale.PointsDiscount, &sale.Total, &sale.CostTotal,
		&sale.PaymentMethod, &sale.PaymentStatus, &sale.CryptoAmount, &sale.CryptoRate,
		&sale.TxHash, &sale.FailureReason, &sale.PointsEarned, &sale.RefundedTotal,
		&sale.VoidedAt, &sale.VoidReason,
	)
	return sale, err
}

// ListSaleItems returns the lines of a sale in insertion order.
func (d *DB) ListSaleItems(ctx context.Context, saleID string) ([]SaleItem, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, sale_id, product_id, sku, name, qty, unit_price, unit_cost, line_discount,
		        refunded_qty, refunded_amount
		 FROM pos_sale_items WHERE sale_id = ? ORDER BY rowid ASC`,
		saleID,
	)
	if err != nil {
		return nil, fmt.Errorf("list sale items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]SaleItem, 0, 8)
	for rows.Next() {
		var item SaleItem
		if err := rows.Scan(
			&item.ID, &item.SaleID, &item.ProductID, &item.SKU, &item.Name, &item.Qty,
			&item.UnitPrice, &item.UnitCost, &item.LineDiscount, &item.RefundedQty, &item.RefundedAmount,
		); err != nil {
			return nil, fmt.Errorf("scan sale item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sale items: %w", err)
	}
	return items, nil
}

// ListRefunds returns a sale's refunds oldest first, without their lines.
func (d *DB) ListRefunds(ctx context.Context, businessID, saleID string) ([]Refund, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, sale_id, created_at, amount, COALESCE(reason, ''), restocked, points_reversed
		 FROM pos_refunds WHERE business_id = ? AND sale_id = ? ORDER BY created_at ASC, id ASC`,
		businessID, saleID,
	)
	if err != nil {
		return nil, fmt.Errorf("list refunds: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]Refund, 0, 4)
	for rows.Next() {
		var (
			refund    Refund
			restocked int64
		)
		if err := rows.Scan(
			&refund.ID, &refund.SaleID, &refund.CreatedAt, &refund.Amount, &refund.Reason,
			&restocked, &refund.PointsReversed,
		); err != nil {
			return nil, fmt.Errorf("scan refund: %w", err)
		}
		refund.Restocked = restocked != 0
		refund.Items = []RefundLine{}
		items = append(items, refund)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate refunds: %w", err)
	}
	return items, nil
}

// GetRefund returns one refund.
func (d *DB) GetRefund(ctx context.Context, businessID, saleID, id string) (Refund, error) {
	var (
		refund    Refund
		restocked int64
	)
	err := d.db.QueryRowContext(ctx,
		`SELECT id, sale_id, created_at, amount, COALESCE(reason, ''), restocked, points_reversed
		 FROM pos_refunds WHERE business_id = ? AND sale_id = ? AND id = ?`,
		businessID, saleID, id,
	).Scan(&refund.ID, &refund.SaleID, &refund.CreatedAt, &refund.Amount, &refund.Reason, &restocked, &refund.PointsReversed)
	if errors.Is(err, sql.ErrNoRows) {
		return Refund{}, ErrNotFound
	}
	if err != nil {
		return Refund{}, fmt.Errorf("get refund: %w", err)
	}
	refund.Restocked = restocked != 0
	refund.Items = []RefundLine{}
	return refund, nil
}

// ListRefundLines returns the lines of a refund with the sale item names.
func (d *DB) ListRefundLines(ctx context.Context, refundID string) ([]RefundLine, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT ri.sale_item_id, si.name, ri.qty, ri.amount, ri.cost_recovered
		 FROM pos_refund_items ri
		 JOIN pos_sale_items si ON si.id = ri.sale_item_id
		 WHERE ri.refund_id = ? ORDER BY si.rowid ASC`,
		refundID,
	)
	if err != nil {
		return nil, fmt.Errorf("list refund lines: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]RefundLine, 0, 4)
	for rows.Next() {
		var line RefundLine
		if err := rows.Scan(&line.SaleItemID, &line.Name, &line.Qty, &line.Amount, &line.CostRecovered); err != nil {
			return nil, fmt.Errorf("scan refund line: %w", err)
		}
		items = append(items, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate refund lines: %w", err)
	}
	return items, nil
}

// ListSaleTotals returns the per-sale figures the profit report needs.
func (d *DB) ListSaleTotals(ctx context.Context, businessID string, from, to int64) ([]posdomain.SaleTotals, error) {
	sqlText := `SELECT status, total, cost_total, refunded_total, points_earned, payment_method
	            FROM pos_sales WHERE business_id = ?`
	args := []any{businessID}
	if from > 0 {
		sqlText += ` AND created_at >= ?`
		args = append(args, from)
	}
	if to > 0 {
		sqlText += ` AND created_at <= ?`
		args = append(args, to)
	}
	sqlText += ` ORDER BY created_at ASC`

	rows, err := d.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("list sale totals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]posdomain.SaleTotals, 0, 64)
	for rows.Next() {
		var totals posdomain.SaleTotals
		if err := rows.Scan(
			&totals.Status, &totals.Total, &totals.CostTotal, &totals.RefundedTotal,
			&totals.PointsEarned, &totals.PaymentMethod,
		); err != nil {
			return nil, fmt.Errorf("scan sale totals: %w", err)
		}
		items = append(items, totals)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sale totals: %w", err)
	}
	return items, nil
}

// RefundAggregates returns the refund count and the cost recovered from
// restocked returns for sold sales in the range.
func (d *DB) RefundAggregates(ctx context.Context, businessID string, from, to int64) (int64, int64, error) {
	sqlText := `SELECT COUNT(DISTINCT r.id), COALESCE(SUM(ri.cost_recovered), 0)
	            FROM pos_refunds r
	            JOIN pos_sales s ON s.id = r.sale_id
	            LEFT JOIN pos_refund_items ri ON ri.refund_id = r.id
	            WHERE r.business_id = ?
	              AND s.status IN ('completed', 'partially_refunded', 'refunded')`
	args := []any{businessID}
	if from > 0 {
		sqlText += ` AND s.created_at >= ?`
		args = append(args, from)
	}
	if to > 0 {
		sqlText += ` AND s.created_at <= ?`
		args = append(args, to)
	}
	var count, recovered int64
	if err := d.db.QueryRowContext(ctx, sqlText, args...).Scan(&count, &recovered); err != nil {
		return 0, 0, fmt.Errorf("refund aggregates: %w", err)
	}
	return count, recovered, nil
}

// ---------------------------------------------------------------------------
// Stock movements
// ---------------------------------------------------------------------------

// MovementFilter narrows a stock movement list.
type MovementFilter struct {
	ProductID string
	Limit     int
}

// ListMovements returns stock movements newest first.
func (d *DB) ListMovements(ctx context.Context, businessID string, f MovementFilter) ([]StockMovement, error) {
	sqlText := `SELECT m.id, m.product_id, p.name, p.sku, m.type, m.quantity, m.balance_after,
	                   m.unit_cost, COALESCE(m.reference, ''), COALESCE(m.note, ''), m.created_at
	            FROM pos_stock_movements m
	            JOIN pos_products p ON p.id = m.product_id
	            WHERE m.business_id = ?`
	args := []any{businessID}
	if f.ProductID != "" {
		sqlText += ` AND m.product_id = ?`
		args = append(args, f.ProductID)
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	args = append(args, limit)
	sqlText += ` ORDER BY m.created_at DESC, m.rowid DESC LIMIT ?`

	rows, err := d.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("list movements: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]StockMovement, 0, 32)
	for rows.Next() {
		var (
			movement StockMovement
			unitCost sql.NullInt64
		)
		if err := rows.Scan(
			&movement.ID, &movement.ProductID, &movement.ProductName, &movement.SKU,
			&movement.Type, &movement.Quantity, &movement.BalanceAfter, &unitCost,
			&movement.Reference, &movement.Note, &movement.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan movement: %w", err)
		}
		if unitCost.Valid {
			movement.UnitCost = unitCost.Int64
			movement.HasUnitCost = true
		}
		items = append(items, movement)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate movements: %w", err)
	}
	return items, nil
}

// InventoryValue is the current inventory valuation.
type InventoryValue struct {
	Units        int64 `json:"units"`
	CostValue    int64 `json:"costValue"`
	RetailValue  int64 `json:"retailValue"`
	ProductCount int64 `json:"productCount"`
}

// Valuation sums stock at cost and at retail for active products.
func (d *DB) Valuation(ctx context.Context, businessID string) (InventoryValue, error) {
	var value InventoryValue
	if err := d.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(stock), 0), COALESCE(SUM(stock * cost_price), 0),
		        COALESCE(SUM(stock * price), 0), COUNT(*)
		 FROM pos_products WHERE business_id = ? AND active = 1`,
		businessID,
	).Scan(&value.Units, &value.CostValue, &value.RetailValue, &value.ProductCount); err != nil {
		return InventoryValue{}, fmt.Errorf("inventory valuation: %w", err)
	}
	return value, nil
}

// ---------------------------------------------------------------------------
// Receipts and idempotency
// ---------------------------------------------------------------------------

// AllocateReceiptNo reserves the next receipt number for a business. The single
// upsert statement is atomic, so concurrent checkouts never share a number.
func (d *DB) AllocateReceiptNo(ctx context.Context, businessID string) (int64, error) {
	var no int64
	err := d.db.QueryRowContext(ctx,
		`INSERT INTO pos_receipt_counters (business_id, next_no) VALUES (?, 1)
		 ON CONFLICT (business_id) DO UPDATE SET next_no = next_no + 1
		 RETURNING next_no`,
		businessID,
	).Scan(&no)
	if err != nil {
		return 0, fmt.Errorf("allocate receipt number: %w", err)
	}
	return no, nil
}

// GetIdempotency returns the stored response for a key, if any.
func (d *DB) GetIdempotency(ctx context.Context, businessID, key string) (string, bool, error) {
	record, ok, err := d.GetIdempotencyRecord(ctx, businessID, key)
	if err != nil || !ok {
		return "", ok, err
	}
	return record.Response, true, nil
}

// IdempotencyRecord is a stored idempotent response plus the scope and request
// hash it was recorded for.
type IdempotencyRecord struct {
	Scope       string
	RequestHash string
	Response    string
}

// GetIdempotencyRecord returns the full record for a key, if any.
func (d *DB) GetIdempotencyRecord(ctx context.Context, businessID, key string) (IdempotencyRecord, bool, error) {
	var record IdempotencyRecord
	err := d.db.QueryRowContext(ctx,
		`SELECT scope, request_hash, response_json FROM pos_idempotency
		 WHERE business_id = ? AND idem_key = ?`,
		businessID, key,
	).Scan(&record.Scope, &record.RequestHash, &record.Response)
	if errors.Is(err, sql.ErrNoRows) {
		return IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("get idempotency key: %w", err)
	}
	return record, true, nil
}
