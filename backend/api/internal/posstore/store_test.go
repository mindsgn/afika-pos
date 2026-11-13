package posstore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const testBusiness = "biz-1"

func testDB(t *testing.T) (*DB, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// :memory: is per-connection, so keep a single connection.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	applySchemaFile(t, db, schemaPath(t))
	store := New(db)
	store.SetClock(func() time.Time { return time.UnixMilli(1700000000000) })
	return store, db
}

// schemaPath locates backend/api/migrations/0003_pos_schema.sql from the test
// package directory without relying on the repository layout beyond the api
// directory.
func schemaPath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "migrations", "0003_pos_schema.sql")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not find migrations/0003_pos_schema.sql")
	return ""
}

func applySchemaFile(t *testing.T, db *sql.DB, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	for _, statement := range strings.Split(string(raw), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("apply schema statement failed: %v\n%s", err, statement)
		}
	}
}

func seedBusiness(t *testing.T, store *DB) Business {
	t.Helper()
	ctx := context.Background()
	business := Business{
		ID:        "biz-1",
		Name:      "Kofi Coffee",
		Website:   "kofi.example",
		Address:   "12 Long St, Cape Town",
		CreatedAt: 1000,
		UpdatedAt: 1000,
	}
	writer := NewWriter()
	writer.UpsertBusiness("did:privy:merchant-1", business)
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("seed business: %v", err)
	}
	return business
}

func TestApplySchemaAndBusinessUpsert(t *testing.T) {
	store, _ := testDB(t)
	ctx := context.Background()

	business := seedBusiness(t, store)
	got, err := store.GetBusinessByOwner(ctx, "did:privy:merchant-1")
	if err != nil {
		t.Fatalf("GetBusinessByOwner: %v", err)
	}
	if got.ID != business.ID || got.Name != business.Name || got.Address != business.Address {
		t.Fatalf("business = %+v", got)
	}

	// A second upsert for the same owner updates in place and keeps the id.
	writer := NewWriter()
	writer.UpsertBusiness("did:privy:merchant-1", Business{
		ID: "should-be-ignored", Name: "Kofi Coffee Co", Website: "kofi.example",
		Address: "13 Long St", CreatedAt: 999, UpdatedAt: 2000,
	})
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("update business: %v", err)
	}
	got, err = store.GetBusinessByOwner(ctx, "did:privy:merchant-1")
	if err != nil {
		t.Fatalf("GetBusinessByOwner after update: %v", err)
	}
	if got.ID != business.ID {
		t.Fatalf("id changed to %q", got.ID)
	}
	if got.Name != "Kofi Coffee Co" || got.Address != "13 Long St" {
		t.Fatalf("business not updated: %+v", got)
	}
	if got.CreatedAt != 1000 {
		t.Fatalf("createdAt = %d, want 1000", got.CreatedAt)
	}
	if _, err := store.GetBusinessByOwner(ctx, "did:privy:someone-else"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing owner error = %v, want ErrNotFound", err)
	}
}

func TestCategoriesAndProducts(t *testing.T) {
	store, _ := testDB(t)
	ctx := context.Background()
	seedBusiness(t, store)

	writer := NewWriter()
	writer.InsertCategory(testBusiness, Category{ID: "cat-1", Name: "Coffee", CreatedAt: 1000})
	writer.InsertCategory(testBusiness, Category{ID: "cat-2", Name: "Bakery", CreatedAt: 1000})
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("insert categories: %v", err)
	}

	categories, err := store.ListCategories(ctx, testBusiness)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(categories) != 2 || categories[0].Name != "Bakery" || categories[1].Name != "Coffee" {
		t.Fatalf("categories = %+v", categories)
	}

	writer = NewWriter()
	writer.InsertProduct(testBusiness, Product{
		ID: "p1", SKU: "COF", Name: "Flat White", CategoryID: "cat-1",
		Price: 3500, CostPrice: 1200, ReorderLevel: 5, Active: true, CreatedAt: 1000, UpdatedAt: 1000,
	})
	writer.MoveStock(testBusiness, "p1", "initial", 10, ptrInt64(1200), "", "Opening stock", 1000)
	writer.InsertProduct(testBusiness, Product{
		ID: "p2", SKU: "MUF", Name: "Muffin", CategoryID: "cat-2",
		Price: 2500, CostPrice: 900, ReorderLevel: 5, Active: true, CreatedAt: 1000, UpdatedAt: 1000,
	})
	writer.MoveStock(testBusiness, "p2", "initial", 2, ptrInt64(900), "", "Opening stock", 1000)
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("insert products: %v", err)
	}

	product, err := store.GetProduct(ctx, testBusiness, "p1")
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if product.Stock != 10 || product.CategoryName != "Coffee" || !product.Active {
		t.Fatalf("product = %+v", product)
	}

	products, err := store.ListProducts(ctx, testBusiness, ProductFilter{Query: "coffee"})
	if err != nil {
		t.Fatalf("ListProducts category search: %v", err)
	}
	if len(products) != 1 || products[0].ID != "p1" {
		t.Fatalf("search by category = %+v", products)
	}

	lowStock, err := store.ListProducts(ctx, testBusiness, ProductFilter{LowStockOnly: true})
	if err != nil {
		t.Fatalf("ListProducts low stock: %v", err)
	}
	if len(lowStock) != 1 || lowStock[0].ID != "p2" {
		t.Fatalf("low stock = %+v", lowStock)
	}

	value, err := store.Valuation(ctx, testBusiness)
	if err != nil {
		t.Fatalf("Valuation: %v", err)
	}
	if value.Units != 12 || value.CostValue != 10*1200+2*900 || value.RetailValue != 10*3500+2*2500 {
		t.Fatalf("valuation = %+v", value)
	}

	movements, err := store.ListMovements(ctx, testBusiness, MovementFilter{ProductID: "p1"})
	if err != nil {
		t.Fatalf("ListMovements: %v", err)
	}
	if len(movements) != 1 || movements[0].Type != "initial" || movements[0].BalanceAfter != 10 || !movements[0].HasUnitCost {
		t.Fatalf("movements = %+v", movements)
	}

	cart, err := store.GetProductsByIDs(ctx, testBusiness, []string{"p1", "p2", "missing"})
	if err != nil {
		t.Fatalf("GetProductsByIDs: %v", err)
	}
	if len(cart) != 2 {
		t.Fatalf("cart products = %+v", cart)
	}
}

func TestStockGuardRollsBackOversell(t *testing.T) {
	store, _ := testDB(t)
	ctx := context.Background()
	seedBusiness(t, store)

	writer := NewWriter()
	writer.InsertProduct(testBusiness, Product{
		ID: "p1", SKU: "COF", Name: "Flat White", Price: 3500, Active: true, CreatedAt: 1000, UpdatedAt: 1000,
	})
	writer.MoveStock(testBusiness, "p1", "initial", 1, nil, "", "Opening stock", 1000)
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	// Selling 2 of 1 must fail and leave the stock untouched.
	writer = NewWriter()
	writer.MoveStock(testBusiness, "p1", "sale", -2, nil, "R-000001", "", 2000)
	if err := store.Write(ctx, writer); err == nil {
		t.Fatal("oversell batch succeeded, want an error")
	}

	stock, err := store.ProductStock(ctx, testBusiness, "p1")
	if err != nil {
		t.Fatalf("ProductStock: %v", err)
	}
	if stock != 1 {
		t.Fatalf("stock = %d, want 1 (rolled back)", stock)
	}
	movements, err := store.ListMovements(ctx, testBusiness, MovementFilter{})
	if err != nil {
		t.Fatalf("ListMovements: %v", err)
	}
	if len(movements) != 1 {
		t.Fatalf("movements = %d, want only the opening stock", len(movements))
	}
}

func TestStockGuardRejectsUnknownProduct(t *testing.T) {
	store, _ := testDB(t)
	seedBusiness(t, store)

	writer := NewWriter()
	writer.MoveStock(testBusiness, "does-not-exist", "restock", 5, nil, "", "", 2000)
	if err := store.Write(context.Background(), writer); err == nil {
		t.Fatal("moving unknown product succeeded, want an error")
	}
}

func TestReceiptNumbersAreUniqueAndSequential(t *testing.T) {
	store, _ := testDB(t)
	ctx := context.Background()
	seedBusiness(t, store)
	seedBusiness2(t, store)

	for want := int64(1); want <= 3; want++ {
		got, err := store.AllocateReceiptNo(ctx, testBusiness)
		if err != nil {
			t.Fatalf("AllocateReceiptNo: %v", err)
		}
		if got != want {
			t.Fatalf("receipt no = %d, want %d", got, want)
		}
	}
	// A second business has its own sequence.
	got, err := store.AllocateReceiptNo(ctx, "biz-2")
	if err != nil {
		t.Fatalf("AllocateReceiptNo for second business: %v", err)
	}
	if got != 1 {
		t.Fatalf("second business receipt no = %d, want 1", got)
	}
}

func TestSaleLifecycleWrites(t *testing.T) {
	store, _ := testDB(t)
	ctx := context.Background()
	seedBusiness(t, store)

	writer := NewWriter()
	writer.InsertProduct(testBusiness, Product{
		ID: "p1", SKU: "COF", Name: "Flat White", Price: 3500, CostPrice: 1200,
		Active: true, CreatedAt: 1000, UpdatedAt: 1000,
	})
	writer.MoveStock(testBusiness, "p1", "initial", 5, ptrInt64(1200), "", "", 1000)
	writer.InsertCustomer(testBusiness, Customer{ID: "c1", Name: "Thandi", LoyaltyPoints: 20, CreatedAt: 1000})
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("seed: %v", err)
	}

	sale := Sale{
		ID: "s1", ReceiptNo: "R-000001", CreatedAt: 2000, UpdatedAt: 2000, Status: "pending",
		CustomerID: "c1", Subtotal: 7000, DiscountType: "none", Total: 7000, CostTotal: 2400,
		PaymentMethod: "usdc-base", PaymentStatus: "pending", CryptoAmount: "0", CryptoRate: "0",
	}
	writer = NewWriter()
	writer.InsertSale(testBusiness, sale, []SaleItem{
		{ID: "si1", SaleID: "s1", ProductID: "p1", SKU: "COF", Name: "Flat White", Qty: 2, UnitPrice: 3500, UnitCost: 1200},
	})
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("insert sale: %v", err)
	}

	// Completing twice must fail the second time, so stock is deducted once.
	writer = NewWriter()
	writer.CompleteSale(testBusiness, "s1", "0xtx", "3.78", "18.5", 7, 3000)
	writer.MoveStock(testBusiness, "p1", "sale", -2, nil, "R-000001", "", 3000)
	writer.SetLoyaltyPoints(testBusiness, "c1", 20, 20-0+7)
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("complete sale: %v", err)
	}

	writer = NewWriter()
	writer.CompleteSale(testBusiness, "s1", "0xtx", "3.78", "18.5", 7, 3000)
	if err := store.Write(ctx, writer); err == nil {
		t.Fatal("second completion succeeded, want an error")
	}

	stock, _ := store.ProductStock(ctx, testBusiness, "p1")
	if stock != 3 {
		t.Fatalf("stock = %d, want 3", stock)
	}
	customer, _ := store.GetCustomer(ctx, testBusiness, "c1")
	if customer.LoyaltyPoints != 27 {
		t.Fatalf("loyalty = %d, want 27", customer.LoyaltyPoints)
	}

	// A stale loyalty write is rejected instead of overwriting the balance.
	writer = NewWriter()
	writer.SetLoyaltyPoints(testBusiness, "c1", 20, 100)
	if err := store.Write(ctx, writer); err == nil {
		t.Fatal("stale loyalty write succeeded, want an error")
	}

	items, err := store.ListSaleItems(ctx, "s1")
	if err != nil {
		t.Fatalf("ListSaleItems: %v", err)
	}
	if len(items) != 1 || items[0].Qty != 2 {
		t.Fatalf("items = %+v", items)
	}

	// A refund of one unit: 3500 refunded, 1200 cost recovered.
	writer = NewWriter()
	writer.InsertRefund(testBusiness, Refund{
		ID: "r1", SaleID: "s1", CreatedAt: 4000, Amount: 3500, Restocked: true,
	})
	writer.InsertRefundLine("r1", RefundLine{SaleItemID: "si1", Qty: 1, Amount: 3500, CostRecovered: 1200})
	writer.AddRefundedQty("si1", 1, 3500)
	writer.MoveStock(testBusiness, "p1", "refund", 1, ptrInt64(1200), "R-000001", "", 4000)
	writer.SetLoyaltyPoints(testBusiness, "c1", 27, 27-3)
	writer.SetRefundPointsReversed("r1", 3)
	writer.UpdateRefundTotals(testBusiness, "s1", 3500, "partially_refunded", 4000)
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("refund: %v", err)
	}

	// Refunding the same line again past its quantity must fail.
	writer = NewWriter()
	writer.AddRefundedQty("si1", 2, 7000)
	if err := store.Write(ctx, writer); err == nil {
		t.Fatal("over-refund succeeded, want an error")
	}

	sale, err = store.GetSale(ctx, testBusiness, "s1")
	if err != nil {
		t.Fatalf("GetSale: %v", err)
	}
	if sale.Status != "partially_refunded" || sale.RefundedTotal != 3500 || sale.PointsEarned != 7 {
		t.Fatalf("sale = %+v", sale)
	}

	refunds, err := store.ListRefunds(ctx, testBusiness, "s1")
	if err != nil {
		t.Fatalf("ListRefunds: %v", err)
	}
	if len(refunds) != 1 || !refunds[0].Restocked || refunds[0].PointsReversed != 3 {
		t.Fatalf("refunds = %+v", refunds)
	}
	lines, err := store.ListRefundLines(ctx, "r1")
	if err != nil {
		t.Fatalf("ListRefundLines: %v", err)
	}
	if len(lines) != 1 || lines[0].Name != "Flat White" || lines[0].CostRecovered != 1200 {
		t.Fatalf("refund lines = %+v", lines)
	}

	count, recovered, err := store.RefundAggregates(ctx, testBusiness, 0, 0)
	if err != nil {
		t.Fatalf("RefundAggregates: %v", err)
	}
	if count != 1 || recovered != 1200 {
		t.Fatalf("aggregates = %d/%d", count, recovered)
	}
}

func TestIdempotencyRoundTrip(t *testing.T) {
	store, _ := testDB(t)
	ctx := context.Background()
	seedBusiness(t, store)

	if _, ok, err := store.GetIdempotency(ctx, testBusiness, "key-1"); err != nil || ok {
		t.Fatalf("empty lookup = %v/%v", ok, err)
	}

	writer := NewWriter()
	writer.InsertIdempotency(testBusiness, "key-1", "sale.create", "hash", `{"saleId":"s1"}`, 1000)
	// A duplicate key must not overwrite the first response.
	writer.InsertIdempotency(testBusiness, "key-1", "sale.create", "hash2", `{"saleId":"s2"}`, 2000)
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("insert idempotency: %v", err)
	}

	response, ok, err := store.GetIdempotency(ctx, testBusiness, "key-1")
	if err != nil || !ok {
		t.Fatalf("lookup = %v/%v", ok, err)
	}
	if response != `{"saleId":"s1"}` {
		t.Fatalf("response = %q", response)
	}
}

func TestListSaleTotals(t *testing.T) {
	store, _ := testDB(t)
	ctx := context.Background()
	seedBusiness(t, store)

	writer := NewWriter()
	writer.InsertSale(testBusiness, Sale{
		ID: "s1", ReceiptNo: "R-000001", CreatedAt: 1000, UpdatedAt: 1000,
		Status: "completed", DiscountType: "none", PaymentMethod: "usdc-base", PaymentStatus: "success",
		Total: 1000, CostTotal: 400,
	}, nil)
	writer.InsertSale(testBusiness, Sale{
		ID: "s2", ReceiptNo: "R-000002", CreatedAt: 5000, UpdatedAt: 5000,
		Status: "voided", DiscountType: "none", PaymentMethod: "btc", PaymentStatus: "success",
		Total: 2000, CostTotal: 800,
	}, nil)
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("insert sales: %v", err)
	}

	totals, err := store.ListSaleTotals(ctx, testBusiness, 0, 0)
	if err != nil {
		t.Fatalf("ListSaleTotals: %v", err)
	}
	if len(totals) != 2 {
		t.Fatalf("totals = %+v", totals)
	}
	totals, err = store.ListSaleTotals(ctx, testBusiness, 0, 2000)
	if err != nil {
		t.Fatalf("ListSaleTotals range: %v", err)
	}
	if len(totals) != 1 || totals[0].Status != "completed" {
		t.Fatalf("range totals = %+v", totals)
	}
}

func seedBusiness2(t *testing.T, store *DB) {
	t.Helper()
	writer := NewWriter()
	writer.UpsertBusiness("did:privy:merchant-2", Business{ID: "biz-2", Name: "Second", CreatedAt: 1000, UpdatedAt: 1000})
	if err := store.Write(context.Background(), writer); err != nil {
		t.Fatalf("seed second business: %v", err)
	}
}

func ptrInt64(value int64) *int64 { return &value }
