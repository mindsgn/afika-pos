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

const (
	testAddress  = "0x1111111111111111111111111111111111111111"
	testAddress2 = "0x2222222222222222222222222222222222222222"
)

func newTestStore(t *testing.T) (*D1Store, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// :memory: is per-connection, so keep a single connection.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	applySchema(t, db, filepath.Join("..", "..", "workers", "balance", "migrations", "0001_init.sql"))
	applySchema(t, db, filepath.Join("..", "..", "workers", "transaction", "migrations", "0001_transaction_init.sql"))
	applySchema(t, db, filepath.Join("..", "migrations", "0001_wallet_settings.sql"))

	return NewD1Store(db), db
}

func applySchema(t *testing.T, db *sql.DB, path string) {
	t.Helper()

	schema, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	for _, stmt := range strings.Split(string(schema), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("apply schema %s: %v\n%s", path, err, stmt)
		}
	}
}

func seedBalance(t *testing.T, db *sql.DB, address, walletID, network, symbol, amount, uiAmount string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO balances (
			wallet_address, wallet_id, network, token_symbol, token_type,
			token_address, amount, ui_amount, multiplier, usd_amount,
			zar_amount, eur_amount, fetched_at
		 ) VALUES (?, ?, ?, ?, 'erc20', '0xcontract', ?, ?, '1', ?, ?, ?, 1700000000000)`,
		address, walletID, network, symbol, amount, uiAmount, amount, amount, amount,
	)
	if err != nil {
		t.Fatalf("seed balance: %v", err)
	}
}

func seedTransaction(t *testing.T, db *sql.DB, address, hash, direction string, timestampMs int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO wallet_transactions (
			wallet_address, tx_hash, from_address, to_address, token_address,
			token_type, token_symbol, amount, fee_native, fee_usd, fee_zar,
			fee_eur, usd_amount, zar_amount, eur_amount, network, direction,
			state, block_number, timestamp_ms, fetched_at_ms
		 ) VALUES (?, ?, '0xfrom', '0xto', '0xtoken', 'erc20', 'USDC', '10',
		           '0.001', '0.02', '0.36', '0.018', '10', '180', '9.2',
		           'base-mainnet', ?, 'completed', 123, ?, 1700000001000)`,
		address, hash, direction, timestampMs,
	)
	if err != nil {
		t.Fatalf("seed transaction: %v", err)
	}
}

func TestD1StoreUpsertWallet(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	store.now = func() time.Time { return time.UnixMilli(1000) }
	wallet, created, err := store.UpsertWallet(ctx, WalletInput{Address: testAddress, Network: "base-mainnet"})
	if err != nil {
		t.Fatalf("UpsertWallet: %v", err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}
	if wallet.Address != testAddress {
		t.Fatalf("address = %q, want %q", wallet.Address, testAddress)
	}
	if wallet.Network != "base-mainnet" {
		t.Fatalf("network = %q", wallet.Network)
	}
	if wallet.Testnet {
		t.Fatal("testnet = true, want false")
	}
	if wallet.CreatedAt != 1000 || wallet.UpdatedAt != 1000 {
		t.Fatalf("timestamps = (%d, %d), want (1000, 1000)", wallet.CreatedAt, wallet.UpdatedAt)
	}

	store.now = func() time.Time { return time.UnixMilli(2000) }
	updated, created, err := store.UpsertWallet(ctx, WalletInput{Address: testAddress, Network: "base-sepolia", Testnet: true})
	if err != nil {
		t.Fatalf("second UpsertWallet: %v", err)
	}
	if created {
		t.Fatal("created = true on update, want false")
	}
	if updated.Address != testAddress {
		t.Fatalf("address = %q, want %q", updated.Address, testAddress)
	}
	if updated.Network != "base-sepolia" || !updated.Testnet {
		t.Fatalf("network/testnet = %q/%v", updated.Network, updated.Testnet)
	}
	if updated.CreatedAt != 1000 {
		t.Fatalf("createdAt = %d, want 1000 (preserved)", updated.CreatedAt)
	}
	if updated.UpdatedAt != 2000 {
		t.Fatalf("updatedAt = %d, want 2000", updated.UpdatedAt)
	}

	stored, err := store.walletByID(ctx, testAddress)
	if err != nil {
		t.Fatalf("walletByID: %v", err)
	}
	if stored != updated {
		t.Fatalf("stored = %+v, want %+v", stored, updated)
	}

	// A second address gets its own row.
	second, created, err := store.UpsertWallet(ctx, WalletInput{Address: testAddress2, Network: "base-mainnet"})
	if err != nil {
		t.Fatalf("third UpsertWallet: %v", err)
	}
	if !created {
		t.Fatal("created = false for a new address, want true")
	}
	if second.Address != testAddress2 {
		t.Fatalf("address = %q, want %q", second.Address, testAddress2)
	}
}

func TestD1StoreUpsertWalletValidation(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name string
		in   WalletInput
	}{
		{"missing address", WalletInput{Network: "base-mainnet"}},
		{"invalid address", WalletInput{Address: "0x123", Network: "base-mainnet"}},
		{"missing network", WalletInput{Address: testAddress}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := store.UpsertWallet(ctx, tc.in); err == nil {
				t.Fatal("UpsertWallet returned nil error")
			}
		})
	}
}

func TestD1StoreListBalancesPaginates(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	seedBalance(t, db, testAddress, "user-1", "base-mainnet", "USDC", "10", "10")
	seedBalance(t, db, testAddress, "user-1", "base-mainnet", "DAI", "5", "5")
	seedBalance(t, db, testAddress, "user-1", "base-mainnet", "NVDA", "1", "2")
	seedBalance(t, db, testAddress, "user-1", "eth-mainnet", "USDC", "3", "3")
	seedBalance(t, db, testAddress, "user-1", "eth-mainnet", "DAI", "4", "4")

	var got []Balance
	var after *BalanceCursor
	pages := 0
	for {
		page, err := store.ListBalances(ctx, strings.ToUpper(testAddress), after, 2)
		if err != nil {
			t.Fatalf("ListBalances: %v", err)
		}
		got = append(got, page.Items...)
		pages++
		if !page.HasMore {
			break
		}
		last := page.Items[len(page.Items)-1]
		after = &BalanceCursor{Network: last.Network, TokenSymbol: last.TokenSymbol}
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
	}

	if pages != 3 {
		t.Fatalf("pages = %d, want 3", pages)
	}
	want := []struct{ network, symbol string }{
		{"base-mainnet", "DAI"},
		{"base-mainnet", "NVDA"},
		{"base-mainnet", "USDC"},
		{"eth-mainnet", "DAI"},
		{"eth-mainnet", "USDC"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d balances, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Network != w.network || got[i].TokenSymbol != w.symbol {
			t.Fatalf("balance %d = %s/%s, want %s/%s", i, got[i].Network, got[i].TokenSymbol, w.network, w.symbol)
		}
	}

	first := got[0]
	if first.WalletAddress != testAddress {
		t.Fatalf("walletAddress = %q", first.WalletAddress)
	}
	if first.WalletID != "user-1" {
		t.Fatalf("walletId = %q", first.WalletID)
	}
	if first.Amount != "5" || first.UIAmount != "5" {
		t.Fatalf("amount/uiAmount = %q/%q", first.Amount, first.UIAmount)
	}
	if first.USDAmount != "5" || first.ZARAmount != "5" || first.EURAmount != "5" {
		t.Fatalf("fiat amounts = %q/%q/%q", first.USDAmount, first.ZARAmount, first.EURAmount)
	}
	if first.FetchedAt != 1700000000000 {
		t.Fatalf("fetchedAt = %d", first.FetchedAt)
	}
}

func TestD1StoreListBalancesEmpty(t *testing.T) {
	store, _ := newTestStore(t)

	page, err := store.ListBalances(context.Background(), testAddress, nil, 10)
	if err != nil {
		t.Fatalf("ListBalances: %v", err)
	}
	if page.Items == nil {
		t.Fatal("items = nil, want empty slice")
	}
	if len(page.Items) != 0 || page.HasMore {
		t.Fatalf("page = %+v, want empty", page)
	}
}

func TestD1StoreListTransactionsPaginates(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	seedTransaction(t, db, testAddress, "0xccc", "credit", 3000)
	seedTransaction(t, db, testAddress, "0xccc", "debit", 3000)
	seedTransaction(t, db, testAddress, "0xbbb", "credit", 3000)
	seedTransaction(t, db, testAddress, "0xddd", "credit", 2000)
	seedTransaction(t, db, testAddress, "0xaaa", "debit", 1000)
	seedTransaction(t, db, testAddress2, "0xeee", "credit", 9000)

	var got []Transaction
	var after *TransactionCursor
	pages := 0
	for {
		page, err := store.ListTransactions(ctx, testAddress, after, 2)
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		got = append(got, page.Items...)
		pages++
		if !page.HasMore {
			break
		}
		last := page.Items[len(page.Items)-1]
		after = &TransactionCursor{TimestampMs: last.TimestampMs, TxHash: last.TxHash, Direction: last.Direction}
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
	}

	if pages != 3 {
		t.Fatalf("pages = %d, want 3", pages)
	}
	want := []struct {
		hash      string
		direction string
		ts        int64
	}{
		{"0xccc", "debit", 3000},
		{"0xccc", "credit", 3000},
		{"0xbbb", "credit", 3000},
		{"0xddd", "credit", 2000},
		{"0xaaa", "debit", 1000},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d transactions, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].TxHash != w.hash || got[i].Direction != w.direction || got[i].TimestampMs != w.ts {
			t.Fatalf("transaction %d = %s/%s/%d, want %s/%s/%d", i, got[i].TxHash, got[i].Direction, got[i].TimestampMs, w.hash, w.direction, w.ts)
		}
	}

	first := got[0]
	if first.WalletAddress != testAddress {
		t.Fatalf("walletAddress = %q", first.WalletAddress)
	}
	if first.TokenSymbol != "USDC" || first.Amount != "10" {
		t.Fatalf("token/amount = %q/%q", first.TokenSymbol, first.Amount)
	}
	if first.FeeUSD != "0.02" || first.FeeZAR != "0.36" || first.FeeEUR != "0.018" {
		t.Fatalf("fees = %q/%q/%q", first.FeeUSD, first.FeeZAR, first.FeeEUR)
	}
	if first.BlockNumber != 123 || first.State != "completed" {
		t.Fatalf("block/state = %d/%q", first.BlockNumber, first.State)
	}
	if first.Timestamp != 3000 || first.FetchedAtMs != 1700000001000 || first.FetchedAt != 1700000001000 {
		t.Fatalf("timestamps = %d/%d/%d", first.Timestamp, first.FetchedAtMs, first.FetchedAt)
	}
}

func TestD1StoreListTransactionsEmpty(t *testing.T) {
	store, _ := newTestStore(t)

	page, err := store.ListTransactions(context.Background(), testAddress, nil, 10)
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if page.Items == nil {
		t.Fatal("items = nil, want empty slice")
	}
	if len(page.Items) != 0 || page.HasMore {
		t.Fatalf("page = %+v, want empty", page)
	}
}

func TestD1StoreWalletSettingsDefault(t *testing.T) {
	store, _ := newTestStore(t)

	settings, err := store.GetWalletSettings(context.Background(), strings.ToUpper(testAddress))
	if err != nil {
		t.Fatalf("GetWalletSettings: %v", err)
	}
	if settings.Address != testAddress {
		t.Fatalf("address = %q, want %q", settings.Address, testAddress)
	}
	if settings.PreferredCurrency != "USD" {
		t.Fatalf("currency = %q, want USD", settings.PreferredCurrency)
	}
	if settings.CreatedAt != 0 || settings.UpdatedAt != 0 {
		t.Fatalf("timestamps = (%d, %d), want zero", settings.CreatedAt, settings.UpdatedAt)
	}
}

func TestD1StoreUpsertWalletSettings(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	store.now = func() time.Time { return time.UnixMilli(1000) }
	saved, err := store.UpsertWalletSettings(ctx, WalletSettingsInput{
		Address:           strings.ToUpper(testAddress),
		PreferredCurrency: "zar",
	})
	if err != nil {
		t.Fatalf("UpsertWalletSettings: %v", err)
	}
	if saved.Address != testAddress || saved.PreferredCurrency != "ZAR" {
		t.Fatalf("saved = %+v", saved)
	}
	if saved.CreatedAt != 1000 || saved.UpdatedAt != 1000 {
		t.Fatalf("timestamps = (%d, %d), want (1000, 1000)", saved.CreatedAt, saved.UpdatedAt)
	}

	store.now = func() time.Time { return time.UnixMilli(2000) }
	updated, err := store.UpsertWalletSettings(ctx, WalletSettingsInput{
		Address:           testAddress,
		PreferredCurrency: "EUR",
	})
	if err != nil {
		t.Fatalf("second UpsertWalletSettings: %v", err)
	}
	if updated.PreferredCurrency != "EUR" {
		t.Fatalf("currency = %q, want EUR", updated.PreferredCurrency)
	}
	if updated.CreatedAt != 1000 || updated.UpdatedAt != 2000 {
		t.Fatalf("timestamps = (%d, %d), want (1000, 2000)", updated.CreatedAt, updated.UpdatedAt)
	}

	got, err := store.GetWalletSettings(ctx, testAddress)
	if err != nil {
		t.Fatalf("GetWalletSettings: %v", err)
	}
	if got != updated {
		t.Fatalf("got = %+v, want %+v", got, updated)
	}
}

func TestD1StoreWalletSettingsValidation(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name string
		in   WalletSettingsInput
	}{
		{"missing address", WalletSettingsInput{PreferredCurrency: "USD"}},
		{"invalid address", WalletSettingsInput{Address: "0x123", PreferredCurrency: "USD"}},
		{"missing currency", WalletSettingsInput{Address: testAddress}},
		{"unsupported currency", WalletSettingsInput{Address: testAddress, PreferredCurrency: "GBP"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.UpsertWalletSettings(ctx, tc.in); err == nil {
				t.Fatal("UpsertWalletSettings returned nil error")
			}
		})
	}
}
