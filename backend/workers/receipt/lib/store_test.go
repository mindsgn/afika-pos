package lib

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// walletTransactionsSchema mirrors the subset of workers/transaction's table the
// receipt worker reads. Tests do not apply the transaction worker's migrations
// because this worker does not own that table.
const walletTransactionsSchema = `
CREATE TABLE IF NOT EXISTS wallet_transactions (
  wallet_address TEXT NOT NULL,
  tx_hash TEXT NOT NULL,
  from_address TEXT NOT NULL DEFAULT '',
  to_address TEXT NOT NULL DEFAULT '',
  token_address TEXT NOT NULL DEFAULT '',
  token_type TEXT NOT NULL DEFAULT 'erc20',
  token_symbol TEXT NOT NULL DEFAULT '',
  amount TEXT NOT NULL DEFAULT '',
  fee_native TEXT NOT NULL DEFAULT '',
  fee_usd TEXT NOT NULL DEFAULT '',
  fee_zar TEXT NOT NULL DEFAULT '',
  fee_eur TEXT NOT NULL DEFAULT '',
  usd_amount TEXT NOT NULL DEFAULT '',
  zar_amount TEXT NOT NULL DEFAULT '',
  eur_amount TEXT NOT NULL DEFAULT '',
  network TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'completed',
  block_number INTEGER NOT NULL DEFAULT 0,
  timestamp_ms INTEGER NOT NULL DEFAULT 0,
  fetched_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (wallet_address, tx_hash, direction, token_address)
);`

func newTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	schema, err := os.ReadFile(filepath.Join("..", "migrations", "0001_receipt_init.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	statements := append([]string{walletTransactionsSchema}, strings.Split(string(schema), ";")...)
	for _, stmt := range statements {
		stmt = stripSQLComments(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("apply schema: %v\n%s", err, stmt)
		}
	}
	return NewStore(db), db
}

// stripSQLComments removes full-line `--` comments so splitting a migration on
// ";" cannot leave a comment fragment behind.
func stripSQLComments(script string) string {
	lines := strings.Split(script, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func insertPayment(t *testing.T, db *sql.DB, p Payment, status string) {
	t.Helper()
	if status == "" {
		status = "pending"
	}
	_, err := db.Exec(
		`INSERT INTO pos_payments (id, receipt_no, amount_cents, asset, network, address,
		                           crypto_amount, status, created_at, updated_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ReceiptNo, p.AmountCents, p.Asset, p.Network, p.Address,
		p.CryptoAmount, status, p.CreatedAt, p.CreatedAt, p.ExpiresAt,
	)
	if err != nil {
		t.Fatalf("insert payment: %v", err)
	}
}

func insertCredit(t *testing.T, db *sql.DB, address, hash, symbol, amount, network string, ts int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO wallet_transactions (wallet_address, tx_hash, to_address, token_symbol,
		                                 amount, network, direction, state, timestamp_ms, fetched_at_ms)
		 VALUES (?, ?, ?, ?, ?, ?, 'credit', 'completed', ?, ?)`,
		address, hash, address, symbol, amount, network, ts, ts,
	)
	if err != nil {
		t.Fatalf("insert credit: %v", err)
	}
}

func paymentStatus(t *testing.T, db *sql.DB, id string) (string, string) {
	t.Helper()
	var status string
	var tx sql.NullString
	if err := db.QueryRow(`SELECT status, tx_hash FROM pos_payments WHERE id = ?`, id).Scan(&status, &tx); err != nil {
		t.Fatalf("select payment: %v", err)
	}
	return status, tx.String
}

func TestExpirePending(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	insertPayment(t, db, Payment{ID: "stale", ExpiresAt: 1000}, "pending")
	insertPayment(t, db, Payment{ID: "live", ExpiresAt: 5000}, "pending")
	insertPayment(t, db, Payment{ID: "paid", ExpiresAt: 1000}, "paid")

	expired, err := store.ExpirePending(ctx, 2000)
	if err != nil {
		t.Fatalf("ExpirePending: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expired = %d, want 1", expired)
	}
	if status, _ := paymentStatus(t, db, "stale"); status != "expired" {
		t.Fatalf("stale status = %q", status)
	}
	if status, _ := paymentStatus(t, db, "live"); status != "pending" {
		t.Fatalf("live status = %q", status)
	}
	if status, _ := paymentStatus(t, db, "paid"); status != "paid" {
		t.Fatalf("paid status = %q", status)
	}
}

func TestListPending(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	insertPayment(t, db, Payment{ID: "b", CreatedAt: 20, ExpiresAt: 9000}, "pending")
	insertPayment(t, db, Payment{ID: "a", CreatedAt: 10, ExpiresAt: 9000}, "pending")
	insertPayment(t, db, Payment{ID: "done", CreatedAt: 5, ExpiresAt: 9000}, "paid")

	pending, err := store.ListPending(ctx, 1000, 10)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
	if pending[0].ID != "a" || pending[1].ID != "b" {
		t.Fatalf("order = %s, %s; want a, b", pending[0].ID, pending[1].ID)
	}

	// The limit is honoured by the store so a backlog cannot exhaust D1 reads.
	limited, err := store.ListPending(ctx, 1000, 1)
	if err != nil {
		t.Fatalf("ListPending limit: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != "a" {
		t.Fatalf("limited = %+v", limited)
	}
}

func TestListCreditsFilters(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()
	const address = "0xabc"

	insertCredit(t, db, address, "0xmatch", "USDC", "6.67", "base-mainnet", 2000)
	insertCredit(t, db, address, "0xold", "USDC", "6.67", "base-mainnet", 500)
	insertCredit(t, db, address, "0xother-symbol", "EURC", "6.67", "base-mainnet", 2000)
	insertCredit(t, db, address, "0xother-network", "USDC", "6.67", "eth-mainnet", 2000)
	insertCredit(t, db, "0xdef", "0xother-wallet", "USDC", "6.67", "base-mainnet", 2000)

	// Debit rows must never settle a payment.
	if _, err := db.Exec(
		`INSERT INTO wallet_transactions (wallet_address, tx_hash, token_symbol, amount, network, direction, timestamp_ms)
		 VALUES (?, '0xdebit', 'USDC', '6.67', 'base-mainnet', 'debit', 2000)`, address); err != nil {
		t.Fatalf("insert debit: %v", err)
	}

	credits, err := store.ListCredits(ctx, CreditQuery{
		Address:  address,
		Networks: NetworkAliases("base"),
		Symbol:   "usdc",
		SinceMs:  1000,
	})
	if err != nil {
		t.Fatalf("ListCredits: %v", err)
	}
	if len(credits) != 1 || credits[0].TxHash != "0xmatch" {
		t.Fatalf("credits = %+v, want only 0xmatch", credits)
	}

	// A credit already attached to a paid payment is not reusable.
	insertPayment(t, db, Payment{ID: "already", Address: address, ExpiresAt: 9000}, "pending")
	if err := store.MarkPaid(ctx, "already", "0xmatch", 3000); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	credits, err = store.ListCredits(ctx, CreditQuery{
		Address:  address,
		Networks: NetworkAliases("base"),
		Symbol:   "USDC",
		SinceMs:  1000,
	})
	if err != nil {
		t.Fatalf("ListCredits after settle: %v", err)
	}
	if len(credits) != 0 {
		t.Fatalf("credits after settle = %+v, want none", credits)
	}
}

func TestListCreditsValidation(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if _, err := store.ListCredits(ctx, CreditQuery{Symbol: "USDC", Networks: []string{"base"}}); err == nil {
		t.Fatal("expected missing address error")
	}
	if _, err := store.ListCredits(ctx, CreditQuery{Address: "0xabc", Networks: []string{"base"}}); err == nil {
		t.Fatal("expected missing symbol error")
	}
	if _, err := store.ListCredits(ctx, CreditQuery{Address: "0xabc", Symbol: "USDC"}); err == nil {
		t.Fatal("expected missing networks error")
	}
}

func TestMarkPaidOnlyTransitionsPending(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	insertPayment(t, db, Payment{ID: "pay-1", ExpiresAt: 9000}, "pending")
	if err := store.MarkPaid(ctx, "pay-1", "0xAbC", 4000); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	status, hash := paymentStatus(t, db, "pay-1")
	if status != "paid" || hash != "0xabc" {
		t.Fatalf("status = %q hash = %q", status, hash)
	}

	// A second settle attempt must not overwrite the recorded transaction.
	if err := store.MarkPaid(ctx, "pay-1", "0xdef", 5000); err != nil {
		t.Fatalf("MarkPaid repeat: %v", err)
	}
	_, hash = paymentStatus(t, db, "pay-1")
	if hash != "0xabc" {
		t.Fatalf("hash overwritten: %q", hash)
	}
}
