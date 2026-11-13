package posservice

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mindsgn-studio/afika-backend/api/internal/posstore"
)

const (
	testOwner = "did:privy:merchant-1"
	testClock = int64(1700000000000)
)

// testService builds an in-memory POS database with the production schema.
func testService(t *testing.T) (*Service, *posstore.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// :memory: is per-connection, so keep a single connection.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	applySchema(t, db, schemaPath(t))
	store := posstore.New(db)
	store.SetClock(func() time.Time { return time.UnixMilli(testClock) })
	return New(store), store
}

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

func applySchema(t *testing.T, db *sql.DB, path string) {
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

func seedBusiness(t *testing.T, service *Service) posstore.Business {
	t.Helper()
	business, err := service.SaveBusiness(context.Background(), testOwner, BusinessInput{
		Name:    "Kofi Coffee",
		Website: "kofi.example",
		Address: "12 Long St, Cape Town",
	})
	if err != nil {
		t.Fatalf("save business: %v", err)
	}
	return business
}

func seedProduct(t *testing.T, service *Service, businessID, sku, name string, price, cost, stock, reorder int64) posstore.Product {
	t.Helper()
	product, err := service.CreateProduct(context.Background(), businessID, ProductInput{
		SKU:          sku,
		Name:         name,
		Price:        price,
		CostPrice:    cost,
		Stock:        stock,
		ReorderLevel: reorder,
	})
	if err != nil {
		t.Fatalf("create product %s: %v", name, err)
	}
	return product
}

func seedCustomer(t *testing.T, service *Service, businessID, name, phone string) posstore.Customer {
	t.Helper()
	customer, err := service.CreateCustomer(context.Background(), businessID, name, phone)
	if err != nil {
		t.Fatalf("create customer %s: %v", name, err)
	}
	return customer
}

// setPoints writes a customer's loyalty balance directly so tests can start
// from a known balance (there is no public "grant points" use case).
func setPoints(t *testing.T, store *posstore.DB, businessID, customerID string, points int64) {
	t.Helper()
	ctx := context.Background()
	writer := posstore.NewWriter()
	writer.Exec(`UPDATE pos_customers SET loyalty_points = ? WHERE business_id = ? AND id = ?`, points, businessID, customerID)
	if err := store.Write(ctx, writer); err != nil {
		t.Fatalf("set points: %v", err)
	}
}

func countRows(t *testing.T, store *posstore.DB, query string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := store.SQL().QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func errCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	return CodeOf(err)
}
