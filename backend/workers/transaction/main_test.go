package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mindsgn-studio/afika-backend/workers/transaction/lib"
)

func testRates() *fakeRates {
	return &fakeRates{rates: map[string]string{"ZAR": "18", "EUR": "0.92"}}
}

func TestProcessTransactions(t *testing.T) {
	ctx := context.Background()
	txTime := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC).UnixMilli()
	nowMs := int64(1700000001000)

	store := &fakeStore{
		wallets: []lib.Wallet{
			{ID: "w1", Address: "0xABC", Network: "base", LatestBlock: 10},
			{ID: "w2", Address: "0xBBB", Network: "solana"},
		},
	}
	transfers := &fakeTransfers{
		assets: map[string][]lib.TransactionItem{
			"0xabc": {
				{
					TxHash: "0xH1", WalletAddress: "0xabc", Direction: "credit",
					FromAddress: "0xdef", ToAddress: "0xabc",
					TokenAddress: "0x8335", TokenType: "erc20", TokenSymbol: "USDC",
					Amount: "10", Network: "base-mainnet", State: "completed",
					BlockNumber: 12, TimestampMs: txTime,
				},
				{
					TxHash: "0xH2", WalletAddress: "0xabc", Direction: "debit",
					FromAddress: "0xabc", ToAddress: "0xdef",
					TokenType: "native", TokenSymbol: "ETH",
					Amount: "1", FeeNative: "0.001", Network: "base-mainnet",
					State: "completed", BlockNumber: 11, TimestampMs: txTime,
				},
			},
		},
		stocks: map[string][]lib.TransactionItem{
			"0xabc": {
				{
					TxHash: "0xH3", WalletAddress: "0xabc", Direction: "credit",
					TokenAddress: "0xnvda", TokenType: "erc20", TokenSymbol: "NVDA",
					Amount: "2.5", Network: "base-mainnet", State: "completed",
					BlockNumber: 13, TimestampMs: txTime,
				},
			},
		},
	}
	rates := testRates()

	result, err := processTransactions(ctx, store, transfers, rates, nowMs)
	if err != nil {
		t.Fatalf("processTransactions: %v", err)
	}
	if result.Saved != 3 || result.Failed != 1 || result.Total != 4 {
		t.Fatalf("result = %+v, want saved=3 failed=1 total=4", result)
	}
	if len(store.saved) != 3 {
		t.Fatalf("saved = %d, want 3", len(store.saved))
	}
	if len(transfers.calls) != 1 || transfers.calls[0] != "wallet:10" {
		t.Fatalf("calls = %#v, want one batched call for the healthy wallet", transfers.calls)
	}

	usdc, ok := findTransaction(store.saved, "0xH1", "credit")
	if !ok {
		t.Fatal("USDC transaction not saved")
	}
	if usdc.USDAmount != "10" || usdc.ZARAmount != "180.00000000" || usdc.EURAmount != "9.20000000" {
		t.Fatalf("USDC amounts = %+v", usdc)
	}
	if usdc.FetchedAtMs != nowMs {
		t.Fatalf("fetchedAt = %d, want %d", usdc.FetchedAtMs, nowMs)
	}

	eth, ok := findTransaction(store.saved, "0xH2", "debit")
	if !ok {
		t.Fatal("ETH transaction not saved")
	}
	if eth.USDAmount != "" || eth.ZARAmount != "" || eth.EURAmount != "" {
		t.Fatalf("ETH amounts = %+v, want empty", eth)
	}
	if eth.FeeUSD != "0.001" || eth.FeeZAR != "0.01800000" || eth.FeeEUR != "0.00092000" {
		t.Fatalf("ETH fees = %+v", eth)
	}

	nvda, ok := findTransaction(store.saved, "0xH3", "credit")
	if !ok {
		t.Fatal("NVDA transaction not saved")
	}
	if nvda.USDAmount != "" || nvda.ZARAmount != "" {
		t.Fatalf("stock amounts = %+v, want empty", nvda)
	}

	if got := store.cursors["0xabc"]; got != 13 {
		t.Fatalf("cursor = %d, want 13", got)
	}
	if rates.calls != 4 {
		t.Fatalf("rate calls = %d, want 4 (amounts + fees for the stablecoin/ETH rows)", rates.calls)
	}
}

func TestProcessTransactionsNetworkError(t *testing.T) {
	store := &fakeStore{wallets: []lib.Wallet{{ID: "w1", Address: "0xabc", Network: "solana"}}}
	transfers := &fakeTransfers{}

	result, err := processTransactions(context.Background(), store, transfers, testRates(), 1)
	if err != nil {
		t.Fatalf("processTransactions: %v", err)
	}
	if result.Saved != 0 || result.Failed != 1 || result.Total != 1 {
		t.Fatalf("result = %+v, want failed=1", result)
	}
	if len(transfers.calls) != 0 {
		t.Fatalf("calls = %#v, want none", transfers.calls)
	}
}

func TestProcessTransactionsFetchFailure(t *testing.T) {
	store := &fakeStore{wallets: []lib.Wallet{{ID: "w1", Address: "0xabc", Network: "base"}}}
	transfers := &fakeTransfers{
		walletErrs: map[string]error{"0xabc": errFetch},
	}

	result, err := processTransactions(context.Background(), store, transfers, testRates(), 1)
	if err != nil {
		t.Fatalf("processTransactions: %v", err)
	}
	if result.Saved != 0 || result.Failed != 1 || result.Total != 1 {
		t.Fatalf("result = %+v, want failed=1", result)
	}
	if len(store.cursors) != 0 {
		t.Fatalf("cursor updated after a failed fetch: %#v", store.cursors)
	}
}

func TestWalletsForRunCaps(t *testing.T) {
	wallets := make([]lib.Wallet, maxWalletsPerRun+15)
	for i := range wallets {
		wallets[i] = lib.Wallet{ID: fmt.Sprintf("w%d", i)}
	}

	selected := walletsForRun(wallets)
	if len(selected) != maxWalletsPerRun {
		t.Fatalf("length = %d, want %d", len(selected), maxWalletsPerRun)
	}
	if selected[0].ID != "w0" {
		t.Fatalf("first = %q, want w0", selected[0].ID)
	}
	if got := len(walletsForRun(wallets[:3])); got != 3 {
		t.Fatalf("small registry length = %d, want 3", got)
	}
}

func TestTransactionsForRunKeepsCutoffBlockWhole(t *testing.T) {
	items := make([]lib.TransactionItem, 0, 160)
	for i := 1; i <= 150; i++ {
		items = append(items, lib.TransactionItem{
			TxHash:      fmt.Sprintf("0x%x", i),
			Direction:   "credit",
			BlockNumber: uint64(i),
		})
	}
	// Ten extra transfers at the cutoff block must all be included so the
	// cursor can advance to block 150 without skipping them.
	for i := 0; i < 10; i++ {
		items = append(items, lib.TransactionItem{
			TxHash:      fmt.Sprintf("0xextra%x", i),
			Direction:   "debit",
			BlockNumber: 150,
		})
	}

	selected := transactionsForRun(items, 150)
	if len(selected) != 160 {
		t.Fatalf("selected = %d, want 160 (all rows at the cutoff block)", len(selected))
	}
	for _, tx := range selected {
		if tx.BlockNumber > 150 {
			t.Fatalf("selected block %d above cutoff", tx.BlockNumber)
		}
	}

	if got := transactionsForRun(items, 0); len(got) != 0 {
		t.Fatalf("limit 0 selected %d, want 0", len(got))
	}
	if got := transactionsForRun(items[:10], 150); len(got) != 10 {
		t.Fatalf("under limit selected %d, want 10", len(got))
	}
}

func TestProcessTransactionsCapsPerWallet(t *testing.T) {
	store := &fakeStore{wallets: []lib.Wallet{{ID: "w1", Address: "0xabc", Network: "base"}}}
	items := make([]lib.TransactionItem, 0, 200)
	for i := 1; i <= 200; i++ {
		items = append(items, lib.TransactionItem{
			TxHash:      fmt.Sprintf("0x%x", i),
			Direction:   "credit",
			TokenSymbol: "USDC",
			Amount:      "1",
			BlockNumber: uint64(i),
		})
	}
	transfers := &fakeTransfers{assets: map[string][]lib.TransactionItem{"0xabc": items}}

	result, err := processTransactions(context.Background(), store, transfers, testRates(), 1)
	if err != nil {
		t.Fatalf("processTransactions: %v", err)
	}
	if result.Saved != maxTransactionsPerWallet {
		t.Fatalf("saved = %d, want %d", result.Saved, maxTransactionsPerWallet)
	}
	if got := store.cursors["0xabc"]; got != maxTransactionsPerWallet {
		t.Fatalf("cursor = %d, want %d", got, maxTransactionsPerWallet)
	}
}

func TestProcessTransactionsRunBudgetDefersWallets(t *testing.T) {
	const wallets = 5
	store := &fakeStore{wallets: make([]lib.Wallet, wallets)}
	transferMap := make(map[string][]lib.TransactionItem, wallets)
	for w := 0; w < wallets; w++ {
		address := fmt.Sprintf("0x%040x", w+1)
		store.wallets[w] = lib.Wallet{ID: address, Address: address, Network: "base"}
		items := make([]lib.TransactionItem, 0, maxTransactionsPerWallet)
		for i := 1; i <= maxTransactionsPerWallet; i++ {
			items = append(items, lib.TransactionItem{
				TxHash:      fmt.Sprintf("0x%x%x", w, i),
				Direction:   "credit",
				TokenSymbol: "USDC",
				Amount:      "1",
				BlockNumber: uint64(i),
			})
		}
		transferMap[address] = items
	}
	transfers := &fakeTransfers{assets: transferMap}

	result, err := processTransactions(context.Background(), store, transfers, testRates(), 1)
	if err != nil {
		t.Fatalf("processTransactions: %v", err)
	}
	if result.Saved != maxTransactionsPerRun {
		t.Fatalf("saved = %d, want the run budget %d", result.Saved, maxTransactionsPerRun)
	}
	if len(transfers.calls) != maxTransactionsPerRun/maxTransactionsPerWallet {
		t.Fatalf("fetched %d wallets, want %d", len(transfers.calls), maxTransactionsPerRun/maxTransactionsPerWallet)
	}
}

func TestProcessTransactionsUpsertFailureDoesNotAdvanceCursor(t *testing.T) {
	store := &fakeStore{
		wallets:  []lib.Wallet{{ID: "w1", Address: "0xabc", Network: "base", LatestBlock: 1}},
		saveErrs: map[string]error{"0xH1|credit": errSave},
	}
	transfers := &fakeTransfers{
		assets: map[string][]lib.TransactionItem{
			"0xabc": {{TxHash: "0xH1", Direction: "credit", TokenSymbol: "USDC", Amount: "1", BlockNumber: 5}},
		},
	}

	result, err := processTransactions(context.Background(), store, transfers, testRates(), 1)
	if err != nil {
		t.Fatalf("processTransactions: %v", err)
	}
	if result.Saved != 0 || result.Failed != 1 {
		t.Fatalf("result = %+v, want failed=1", result)
	}
	if len(store.cursors) != 0 {
		t.Fatalf("cursor advanced after an upsert failure: %#v", store.cursors)
	}
}

func TestProcessTransactionsDeriveFailureStillSaves(t *testing.T) {
	store := &fakeStore{wallets: []lib.Wallet{{ID: "w1", Address: "0xabc", Network: "base"}}}
	transfers := &fakeTransfers{
		assets: map[string][]lib.TransactionItem{
			"0xabc": {{TxHash: "0xH1", Direction: "credit", TokenSymbol: "USDC", Amount: "1", BlockNumber: 5}},
		},
	}
	rates := &fakeRates{
		rates: map[string]string{"ZAR": "18"},
		errs:  map[string]error{"EUR": errors.New("eur down")},
	}

	result, err := processTransactions(context.Background(), store, transfers, rates, 1)
	if err != nil {
		t.Fatalf("processTransactions: %v", err)
	}
	if result.Saved != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v, want saved=1", result)
	}
	tx, ok := findTransaction(store.saved, "0xH1", "credit")
	if !ok {
		t.Fatal("transaction not saved")
	}
	if tx.ZARAmount != "18.00000000" || tx.EURAmount != "" {
		t.Fatalf("amounts = %+v, want ZAR kept and EUR empty", tx)
	}
}

func TestProcessTransactionsWalletsError(t *testing.T) {
	store := &fakeStore{walletsErr: errFetch}
	if _, err := processTransactions(context.Background(), store, &fakeTransfers{}, testRates(), 1); err == nil {
		t.Fatal("expected wallet fetch error")
	}
}

func TestProcessTransactionsNoWallets(t *testing.T) {
	result, err := processTransactions(context.Background(), &fakeStore{}, &fakeTransfers{}, testRates(), 1)
	if err != nil {
		t.Fatalf("processTransactions: %v", err)
	}
	if result != (lib.WorkerResult{}) {
		t.Fatalf("result = %+v, want zero value", result)
	}
}

func TestProcessTransactionsCursorErrorIsNonFatal(t *testing.T) {
	store := &fakeStore{
		wallets:   []lib.Wallet{{ID: "w1", Address: "0xabc", Network: "base"}},
		cursorErr: errSave,
	}
	transfers := &fakeTransfers{
		assets: map[string][]lib.TransactionItem{
			"0xabc": {{TxHash: "0xH1", Direction: "credit", TokenSymbol: "USDC", Amount: "1", BlockNumber: 5}},
		},
	}

	result, err := processTransactions(context.Background(), store, transfers, testRates(), 1)
	if err != nil {
		t.Fatalf("processTransactions: %v", err)
	}
	if result.Saved != 1 {
		t.Fatalf("result = %+v, want saved=1", result)
	}
}

func TestProcessTransactionsDefaultTimestamp(t *testing.T) {
	store := &fakeStore{wallets: []lib.Wallet{{ID: "w1", Address: "0xabc", Network: "base"}}}
	transfers := &fakeTransfers{
		assets: map[string][]lib.TransactionItem{
			"0xabc": {{TxHash: "0xH1", Direction: "credit", TokenSymbol: "ETH", Amount: "1"}},
		},
	}
	before := time.Now().UnixMilli()

	if _, err := processTransactions(context.Background(), store, transfers, testRates(), 0); err != nil {
		t.Fatalf("processTransactions: %v", err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved = %d, want 1", len(store.saved))
	}
	if store.saved[0].FetchedAtMs < before {
		t.Fatalf("fetchedAt = %d, want >= %d", store.saved[0].FetchedAtMs, before)
	}
	if store.saved[0].TimestampMs != store.saved[0].FetchedAtMs {
		t.Fatalf("timestamp = %d, want fetchedAt %d", store.saved[0].TimestampMs, store.saved[0].FetchedAtMs)
	}
}

func TestDeriveAmounts(t *testing.T) {
	rates := testRates()
	tests := []struct {
		name    string
		symbol  string
		amount  string
		wantUSD string
		wantZAR string
		wantEUR string
	}{
		{name: "USDC", symbol: "USDC", amount: "10", wantUSD: "10", wantZAR: "180.00000000", wantEUR: "9.20000000"},
		{name: "USDT", symbol: "USDT", amount: "2", wantUSD: "2", wantZAR: "36.00000000", wantEUR: "1.84000000"},
		{name: "DAI", symbol: "DAI", amount: "1.5", wantUSD: "1.5", wantZAR: "27.00000000", wantEUR: "1.38000000"},
		{name: "ZARP", symbol: "ZARP", amount: "5", wantZAR: "5"},
		{name: "ETH untouched", symbol: "ETH", amount: "1"},
		{name: "stablecoin empty", symbol: "USDC", amount: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx := &lib.TransactionItem{TokenSymbol: tc.symbol, Amount: tc.amount}
			if err := deriveAmounts(context.Background(), rates, tx); err != nil {
				t.Fatalf("deriveAmounts: %v", err)
			}
			if tx.USDAmount != tc.wantUSD || tx.ZARAmount != tc.wantZAR || tx.EURAmount != tc.wantEUR {
				t.Fatalf("usd=%q zar=%q eur=%q want %q %q %q",
					tx.USDAmount, tx.ZARAmount, tx.EURAmount, tc.wantUSD, tc.wantZAR, tc.wantEUR)
			}
		})
	}
}

func TestDeriveAmountsEURFailureKeepsZAR(t *testing.T) {
	rates := &fakeRates{
		rates: map[string]string{"ZAR": "18"},
		errs:  map[string]error{"EUR": errors.New("eur down")},
	}

	tx := &lib.TransactionItem{TokenSymbol: "USDC", Amount: "10"}
	if err := deriveAmounts(context.Background(), rates, tx); err == nil {
		t.Fatal("expected EUR failure to surface")
	}
	if tx.ZARAmount != "180.00000000" {
		t.Fatalf("zar=%q want 180.00000000", tx.ZARAmount)
	}
	if tx.EURAmount != "" {
		t.Fatalf("eur=%q want empty", tx.EURAmount)
	}
}

func TestDeriveAmountsInvalidAmount(t *testing.T) {
	tx := &lib.TransactionItem{TokenSymbol: "USDC", Amount: "not-a-number"}
	if err := deriveAmounts(context.Background(), testRates(), tx); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestDeriveFeeAmounts(t *testing.T) {
	rates := testRates()
	tests := []struct {
		name    string
		feeUSD  string
		feeNat  string
		wantUSD string
		wantZAR string
		wantEUR string
	}{
		{name: "feeUsd", feeUSD: "2", wantUSD: "2", wantZAR: "36.00000000", wantEUR: "1.84000000"},
		{name: "falls back to native", feeNat: "1", wantUSD: "1", wantZAR: "18.00000000", wantEUR: "0.92000000"},
		{name: "no fee"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx := &lib.TransactionItem{FeeUSD: tc.feeUSD, FeeNative: tc.feeNat}
			if err := deriveFeeAmounts(context.Background(), rates, tx); err != nil {
				t.Fatalf("deriveFeeAmounts: %v", err)
			}
			if tx.FeeUSD != tc.wantUSD || tx.FeeZAR != tc.wantZAR || tx.FeeEUR != tc.wantEUR {
				t.Fatalf("usd=%q zar=%q eur=%q want %q %q %q",
					tx.FeeUSD, tx.FeeZAR, tx.FeeEUR, tc.wantUSD, tc.wantZAR, tc.wantEUR)
			}
		})
	}
}

func TestMultiplyAmountByRate(t *testing.T) {
	got, err := multiplyAmountByRate("2", "18.5")
	if err != nil {
		t.Fatalf("multiplyAmountByRate: %v", err)
	}
	if got != "37.00000000" {
		t.Fatalf("got = %q, want 37.00000000", got)
	}

	if _, err := multiplyAmountByRate("nope", "18.5"); err == nil {
		t.Fatal("expected amount parse error")
	}
	if _, err := multiplyAmountByRate("2", "nope"); err == nil {
		t.Fatal("expected rate parse error")
	}
}
