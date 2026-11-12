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

	schema, err := os.ReadFile(filepath.Join("..", "migrations", "0001_transaction_init.sql"))
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

func seedWallet(t *testing.T, db *sql.DB, id, address, network string, testnet bool) {
	t.Helper()
	testnetInt := 0
	if testnet {
		testnetInt = 1
	}
	_, err := db.Exec(
		`INSERT INTO wallets (id, address, network, testnet, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, 1)`,
		id, address, network, testnetInt,
	)
	if err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
}

func TestStoreFetchWallets(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	seedWallet(t, db, "w1", "0xABC", "base", false)
	seedWallet(t, db, "w2", "  ", "ethereum", true)
	seedWallet(t, db, "w3", "0xDEF", "base-mainnet", false)

	if _, err := db.Exec(
		`INSERT INTO wallet_transaction_cursors (wallet_address, latest_block, updated_at)
		 VALUES ('0xabc', 42, 100)`,
	); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}

	wallets, err := store.FetchWallets(ctx)
	if err != nil {
		t.Fatalf("FetchWallets: %v", err)
	}
	if len(wallets) != 3 {
		t.Fatalf("wallets = %d, want 3", len(wallets))
	}

	byID := map[string]Wallet{}
	for _, wallet := range wallets {
		byID[wallet.ID] = wallet
	}

	w1 := byID["w1"]
	if w1.Address != "0xabc" || w1.LatestBlock != 42 {
		t.Fatalf("w1 = %+v, want lowercase address and cursor 42", w1)
	}
	w2 := byID["w2"]
	if w2.Address != "w2" || !w2.Testnet {
		t.Fatalf("w2 = %+v, want id as address and testnet", w2)
	}
	w3 := byID["w3"]
	if w3.Address != "0xdef" || w3.LatestBlock != 0 {
		t.Fatalf("w3 = %+v, want lowercase address and no cursor", w3)
	}
}

func TestStoreFetchWalletsOrdersLeastRecentlySyncedFirst(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	seedWallet(t, db, "w1", "0xAAA", "base", false)
	seedWallet(t, db, "w2", "0xBBB", "base", false)
	seedWallet(t, db, "w3", "0xCCC", "base", false)
	if _, err := db.Exec(
		`INSERT INTO wallet_transaction_cursors (wallet_address, latest_block, updated_at)
		 VALUES ('0xaaa', 10, 300), ('0xbbb', 20, 100)`,
	); err != nil {
		t.Fatalf("seed cursors: %v", err)
	}

	wallets, err := store.FetchWallets(ctx)
	if err != nil {
		t.Fatalf("FetchWallets: %v", err)
	}
	got := make([]string, 0, len(wallets))
	for _, wallet := range wallets {
		got = append(got, wallet.ID)
	}
	want := []string{"w3", "w2", "w1"}
	if len(got) != len(want) {
		t.Fatalf("wallets = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wallets = %v, want %v", got, want)
		}
	}
}

func TestStoreFetchWalletsError(t *testing.T) {
	store, db := newTestStore(t)
	if _, err := db.Exec(`DROP TABLE wallets`); err != nil {
		t.Fatalf("drop wallets: %v", err)
	}
	if _, err := store.FetchWallets(context.Background()); err == nil {
		t.Fatal("expected query error")
	}
}

func TestStoreUpsertTransaction(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	item := TransactionItem{
		WalletAddress: " 0xABC ",
		TxHash:        " 0xHASH ",
		FromAddress:   " 0xFROM ",
		ToAddress:     " 0xTO ",
		TokenAddress:  " 0xTOKEN ",
		TokenType:     "erc20",
		TokenSymbol:   "USDC",
		Amount:        "10",
		FeeNative:     "0.001",
		FeeUSD:        "0.01",
		FeeZAR:        "0.18",
		FeeEUR:        "0.0092",
		USDAmount:     "10",
		ZARAmount:     "180.00",
		EURAmount:     "9.20",
		Network:       "base-mainnet",
		Direction:     "credit",
		State:         "completed",
		BlockNumber:   100,
		TimestampMs:   1700000000000,
		FetchedAtMs:   1700000001000,
	}
	if err := store.UpsertTransaction(ctx, item); err != nil {
		t.Fatalf("UpsertTransaction: %v", err)
	}

	var (
		walletAddress, txHash, fromAddress, toAddress, tokenAddress string
		tokenType, tokenSymbol, amount, feeNative, feeUSD           string
		feeZAR, feeEUR, usdAmount, zarAmount, eurAmount             string
		network, direction, state                                   string
		blockNumber, timestampMs, fetchedAtMs                       int64
	)
	err := db.QueryRow(
		`SELECT wallet_address, tx_hash, from_address, to_address, token_address,
		        token_type, token_symbol, amount, fee_native, fee_usd, fee_zar,
		        fee_eur, usd_amount, zar_amount, eur_amount, network, direction,
		        state, block_number, timestamp_ms, fetched_at_ms
		 FROM wallet_transactions`,
	).Scan(
		&walletAddress, &txHash, &fromAddress, &toAddress, &tokenAddress,
		&tokenType, &tokenSymbol, &amount, &feeNative, &feeUSD, &feeZAR,
		&feeEUR, &usdAmount, &zarAmount, &eurAmount, &network, &direction,
		&state, &blockNumber, &timestampMs, &fetchedAtMs,
	)
	if err != nil {
		t.Fatalf("select transaction: %v", err)
	}

	if walletAddress != "0xabc" || txHash != "0xhash" {
		t.Fatalf("wallet/hash = %q/%q", walletAddress, txHash)
	}
	if fromAddress != "0xfrom" || toAddress != "0xto" || tokenAddress != "0xtoken" {
		t.Fatalf("addresses = %q/%q/%q", fromAddress, toAddress, tokenAddress)
	}
	if tokenType != "erc20" || tokenSymbol != "USDC" || amount != "10" {
		t.Fatalf("token = %q/%q/%q", tokenType, tokenSymbol, amount)
	}
	if feeNative != "0.001" || feeUSD != "0.01" || feeZAR != "0.18" || feeEUR != "0.0092" {
		t.Fatalf("fees = %q/%q/%q/%q", feeNative, feeUSD, feeZAR, feeEUR)
	}
	if usdAmount != "10" || zarAmount != "180.00" || eurAmount != "9.20" {
		t.Fatalf("amounts = %q/%q/%q", usdAmount, zarAmount, eurAmount)
	}
	if network != "base-mainnet" || direction != "credit" || state != "completed" {
		t.Fatalf("meta = %q/%q/%q", network, direction, state)
	}
	if blockNumber != 100 || timestampMs != 1700000000000 || fetchedAtMs != 1700000001000 {
		t.Fatalf("timestamps = %d/%d/%d", blockNumber, timestampMs, fetchedAtMs)
	}
}

func TestStoreUpsertTransactionDefaultsTimestampAndOverwrites(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	first := TransactionItem{
		WalletAddress: "0xabc",
		TxHash:        "0xhash",
		Direction:     "credit",
		Amount:        "10",
		ZARAmount:     "180",
		FetchedAtMs:   1700000000000,
	}
	if err := store.UpsertTransaction(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	var timestampMs int64
	if err := db.QueryRow(`SELECT timestamp_ms FROM wallet_transactions`).Scan(&timestampMs); err != nil {
		t.Fatalf("select timestamp: %v", err)
	}
	if timestampMs != 1700000000000 {
		t.Fatalf("timestamp = %d, want fetchedAt fallback", timestampMs)
	}

	second := TransactionItem{
		WalletAddress: "0xABC",
		TxHash:        "0xHASH",
		Direction:     "credit",
		Amount:        "12",
		ZARAmount:     "",
		FetchedAtMs:   1700000005000,
	}
	if err := store.UpsertTransaction(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM wallet_transactions`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("rows = %d, want 1", count)
	}

	var amount, zarAmount string
	if err := db.QueryRow(`SELECT amount, zar_amount FROM wallet_transactions`).Scan(&amount, &zarAmount); err != nil {
		t.Fatalf("select amount: %v", err)
	}
	if amount != "12" || zarAmount != "" {
		t.Fatalf("amount = %q zar = %q, want 12/empty", amount, zarAmount)
	}
}

func TestStoreUpsertTransactionValidation(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name string
		item TransactionItem
	}{
		{name: "missing wallet", item: TransactionItem{TxHash: "0x1", Direction: "credit"}},
		{name: "blank wallet", item: TransactionItem{WalletAddress: "   ", TxHash: "0x1", Direction: "credit"}},
		{name: "missing hash", item: TransactionItem{WalletAddress: "0xabc", Direction: "credit"}},
		{name: "missing direction", item: TransactionItem{WalletAddress: "0xabc", TxHash: "0x1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.UpsertTransaction(ctx, tc.item); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestStoreLatestBlock(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	got, err := store.LatestBlock(ctx, "0xABC")
	if err != nil {
		t.Fatalf("LatestBlock: %v", err)
	}
	if got != 0 {
		t.Fatalf("missing cursor = %d, want 0", got)
	}

	if err := store.UpdateWalletLatestBlock(ctx, "0xABC", 20); err != nil {
		t.Fatalf("UpdateWalletLatestBlock: %v", err)
	}
	got, err = store.LatestBlock(ctx, "0xabc")
	if err != nil {
		t.Fatalf("LatestBlock: %v", err)
	}
	if got != 20 {
		t.Fatalf("cursor = %d, want 20", got)
	}

	// The cursor must not move backwards if a stale run finishes late.
	if err := store.UpdateWalletLatestBlock(ctx, "0xABC", 15); err != nil {
		t.Fatalf("UpdateWalletLatestBlock: %v", err)
	}
	got, err = store.LatestBlock(ctx, "0xabc")
	if err != nil {
		t.Fatalf("LatestBlock: %v", err)
	}
	if got != 20 {
		t.Fatalf("cursor = %d, want 20 after a lower update", got)
	}
}

func TestStoreUpdateWalletLatestBlockValidation(t *testing.T) {
	store, _ := newTestStore(t)
	if err := store.UpdateWalletLatestBlock(context.Background(), "  ", 1); err == nil {
		t.Fatal("expected validation error for empty address")
	}
	if _, err := store.LatestBlock(context.Background(), ""); err == nil {
		t.Fatal("expected validation error for empty LatestBlock address")
	}
}

func TestStoreClose(t *testing.T) {
	store, _ := newTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestStoreUpdateTimestampUsesNow(t *testing.T) {
	store, db := newTestStore(t)
	before := time.Now().UnixMilli()

	if err := store.UpdateWalletLatestBlock(context.Background(), "0xabc", 7); err != nil {
		t.Fatalf("UpdateWalletLatestBlock: %v", err)
	}

	var updatedAt int64
	if err := db.QueryRow(`SELECT updated_at FROM wallet_transaction_cursors`).Scan(&updatedAt); err != nil {
		t.Fatalf("select updated_at: %v", err)
	}
	if updatedAt < before {
		t.Fatalf("updated_at = %d, want >= %d", updatedAt, before)
	}
}
