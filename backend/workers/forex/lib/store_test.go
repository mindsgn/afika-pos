package lib

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// :memory: is per-connection, so keep a single connection.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	schema, err := os.ReadFile(filepath.Join("..", "migrations", "0001_forex_init.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(schema), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("apply schema: %v\n%s", err, stmt)
		}
	}
	return NewStore(db), db
}

func TestStoreUpsertFXRate(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	if err := store.UpsertFXRate(ctx, " usd/zar ", "18.75000000", 1000); err != nil {
		t.Fatalf("UpsertFXRate: %v", err)
	}

	var pair, rate string
	var updatedAt int64
	if err := db.QueryRow(`SELECT pair, rate, updated_at FROM fx_rates`).Scan(&pair, &rate, &updatedAt); err != nil {
		t.Fatalf("select rate: %v", err)
	}
	if pair != "USD_ZAR" {
		t.Fatalf("pair = %q, want USD_ZAR", pair)
	}
	if rate != "18.75000000" {
		t.Fatalf("rate = %q, want 18.75000000", rate)
	}
	if updatedAt != 1000 {
		t.Fatalf("updated_at = %d, want 1000", updatedAt)
	}
}

func TestStoreUpsertFXRateOverwrites(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	if err := store.UpsertFXRate(ctx, "USD_ZAR", "18.0", 1000); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := store.UpsertFXRate(ctx, "USD/ZAR", "19.5", 2000); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM fx_rates`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("rows = %d, want 1", count)
	}

	var rate string
	var updatedAt int64
	if err := db.QueryRow(`SELECT rate, updated_at FROM fx_rates WHERE pair = 'USD_ZAR'`).Scan(&rate, &updatedAt); err != nil {
		t.Fatalf("select rate: %v", err)
	}
	if rate != "19.5" || updatedAt != 2000 {
		t.Fatalf("rate = %q updated_at = %d, want 19.5/2000", rate, updatedAt)
	}
}

func TestStoreUpsertFXRateDefaultsFetchedAt(t *testing.T) {
	store, db := newTestStore(t)
	before := time.Now().UnixMilli()

	if err := store.UpsertFXRate(context.Background(), "EUR_USD", "1.08", 0); err != nil {
		t.Fatalf("UpsertFXRate: %v", err)
	}

	var updatedAt int64
	if err := db.QueryRow(`SELECT updated_at FROM fx_rates WHERE pair = 'EUR_USD'`).Scan(&updatedAt); err != nil {
		t.Fatalf("select rate: %v", err)
	}
	if updatedAt < before {
		t.Fatalf("updated_at = %d, want >= %d", updatedAt, before)
	}
}

func TestStoreUpsertFXRateValidation(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name string
		pair string
		rate string
	}{
		{name: "missing pair", pair: "", rate: "1"},
		{name: "blank pair", pair: "   ", rate: "1"},
		{name: "missing rate", pair: "USD/ZAR", rate: ""},
		{name: "blank rate", pair: "USD/ZAR", rate: "   "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.UpsertFXRate(ctx, tc.pair, tc.rate, 1); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestStoreLatestFXRate(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.UpsertFXRate(ctx, "USD/ZAR", "18.75", 1000); err != nil {
		t.Fatalf("UpsertFXRate: %v", err)
	}

	got, err := store.LatestFXRate(ctx, " usd/zar ")
	if err != nil {
		t.Fatalf("LatestFXRate: %v", err)
	}
	if got != "18.75" {
		t.Fatalf("rate = %q, want 18.75", got)
	}

	if _, err := store.LatestFXRate(ctx, "USD/EUR"); err == nil {
		t.Fatal("expected error for missing pair")
	}
	if _, err := store.LatestFXRate(ctx, ""); err == nil {
		t.Fatal("expected error for empty pair")
	}
}
