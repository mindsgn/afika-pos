package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mindsgn-studio/afika-backend/workers/balance/lib"
)

func TestProcessBalances(t *testing.T) {
	ctx := context.Background()

	store := &fakeStore{
		wallets: []lib.Wallet{
			{ID: "w1", Address: "0xABC", Network: "base-mainnet"},
			{ID: "w2", Address: "0xDEF", Network: "base-mainnet"},
		},
		networks: lib.CloneNetworkConfigMap(lib.SupportedNetworks),
		tokens:   lib.CloneTokenConfigMap(lib.ERC20Tokens),
		stocks: []lib.TokenConfig{
			{Symbol: "NVDA", Contract: "0xNVDA", Decimals: 8},
		},
		quotes: map[string]lib.StockQuote{
			"0xnvda": {Symbol: "NVDA", Contract: "0xNVDA", Decimals: 8, Price: "100", Multiplier: "2"},
		},
		rates: map[string]string{
			"USD/ZAR": "18",
			"USD/EUR": "0.92",
		},
	}

	fetcher := &fakeFetcher{
		balances: map[string][]lib.Balance{
			"w1": {
				{WalletID: "w1", Address: "0xabc", Network: "base-mainnet", TokenType: "native", TokenSymbol: "ETH", Amount: "1"},
				{WalletID: "w1", Address: "0xabc", Network: "base-mainnet", TokenType: "erc20", TokenSymbol: "USDC", TokenAddress: "0x8335", Amount: "10"},
				{WalletID: "w1", Address: "0xabc", Network: "base-mainnet", TokenType: "erc20", TokenSymbol: "NVDA", TokenAddress: "0xNVDA", Amount: "2"},
			},
		},
		errs: map[string]error{"w2": errFetch},
	}

	result, err := processBalances(ctx, store, fetcher)
	if err != nil {
		t.Fatalf("processBalances: %v", err)
	}
	if result.Saved != 3 || result.Failed != 1 || result.Total != 4 {
		t.Fatalf("result = %+v, want saved=3 failed=1 total=4", result)
	}
	if fetcher.calls != 2 {
		t.Fatalf("fetcher calls = %d, want 2", fetcher.calls)
	}
	if len(store.saved) != 3 {
		t.Fatalf("saved balances = %d, want 3", len(store.saved))
	}

	eth, ok := findBalance(store.saved, "ETH")
	if !ok {
		t.Fatal("ETH balance not saved")
	}
	if eth.USDAmount != "" || eth.ZARAmount != "" {
		t.Fatalf("ETH should have no USD value without a price, got %+v", eth)
	}

	usdc, ok := findBalance(store.saved, "USDC")
	if !ok {
		t.Fatal("USDC balance not saved")
	}
	if usdc.USDAmount != "10" || usdc.ZARAmount != "180.00000000" || usdc.EURAmount != "9.20000000" {
		t.Fatalf("USDC amounts = %+v", usdc)
	}

	nvda, ok := findBalance(store.saved, "NVDA")
	if !ok {
		t.Fatal("NVDA balance not saved")
	}
	if nvda.UIAmount != "4" || nvda.Multiplier != "2" {
		t.Fatalf("NVDA stock fields = %+v", nvda)
	}
	if nvda.USDAmount != "200" || nvda.ZARAmount != "3600.00000000" || nvda.EURAmount != "184.00000000" {
		t.Fatalf("NVDA amounts = %+v", nvda)
	}

	portfolio, ok := store.portfolios["0xabc"]
	if !ok {
		t.Fatalf("portfolio not saved for w1: %+v", store.portfolios)
	}
	if portfolio.NetWorth != "210" || portfolio.USDC != "10" || portfolio.Stocks != "200" || portfolio.Other != "" {
		t.Fatalf("portfolio = %+v, want net worth 210 with USDC 10 and stocks 200", portfolio)
	}

	snapshots := store.savedSnapshots["0xabc"]
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %+v, want 1", snapshots)
	}
	if snapshots[0].NetWorth != "210" || snapshots[0].At == 0 {
		t.Fatalf("snapshot = %+v", snapshots[0])
	}
	if _, ok := store.portfolios["0xdef"]; ok {
		t.Fatal("failed wallet should not get a portfolio")
	}
}

func TestProcessBalancesWalletsError(t *testing.T) {
	store := &fakeStore{walletsErr: errFetch}
	if _, err := processBalances(context.Background(), store, &fakeFetcher{}); err == nil {
		t.Fatal("expected wallet fetch error")
	}
}

func TestProcessBalancesNoWallets(t *testing.T) {
	result, err := processBalances(context.Background(), &fakeStore{}, &fakeFetcher{})
	if err != nil {
		t.Fatalf("processBalances: %v", err)
	}
	if result != (BalanceResult{}) {
		t.Fatalf("result = %+v, want zero value", result)
	}
}

func TestProcessBalancesSaveErrors(t *testing.T) {
	store := &fakeStore{
		wallets: []lib.Wallet{{ID: "w1", Address: "0xabc", Network: "base-mainnet"}},
		saveErr: errFetch,
	}
	fetcher := &fakeFetcher{
		balances: map[string][]lib.Balance{
			"w1": {
				{WalletID: "w1", Address: "0xabc", Network: "base-mainnet", TokenType: "native", TokenSymbol: "ETH", Amount: "1"},
			},
		},
	}

	result, err := processBalances(context.Background(), store, fetcher)
	if err != nil {
		t.Fatalf("processBalances: %v", err)
	}
	if result.Saved != 0 || result.Failed != 1 || result.Total != 1 {
		t.Fatalf("result = %+v, want failed=1", result)
	}
}

func TestDeriveAmounts(t *testing.T) {
	store := &fakeStore{rates: map[string]string{
		"USD/ZAR": "18",
		"USD/EUR": "0.92",
	}}
	tests := []struct {
		name    string
		symbol  string
		amount  string
		quote   lib.StockQuote
		wantUSD string
		wantZAR string
		wantEUR string
	}{
		{name: "USDC", symbol: "USDC", amount: "10", wantUSD: "10", wantZAR: "180.00000000", wantEUR: "9.20000000"},
		{name: "DAI", symbol: "DAI", amount: "2", wantUSD: "2", wantZAR: "36.00000000", wantEUR: "1.84000000"},
		{name: "ZARP", symbol: "ZARP", amount: "5", wantUSD: "", wantZAR: "5", wantEUR: ""},
		{name: "ETH no price", symbol: "ETH", amount: "1", wantUSD: "", wantZAR: "", wantEUR: ""},
		{name: "NVDA with price", symbol: "NVDA", amount: "2", quote: lib.StockQuote{Price: "100"}, wantUSD: "200", wantZAR: "3600.00000000", wantEUR: "184.00000000"},
		{name: "stock no price", symbol: "NVDA", amount: "2", wantUSD: "", wantZAR: "", wantEUR: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			usd, zar, eur := deriveAmounts(context.Background(), store, tc.symbol, tc.amount, tc.quote)
			if usd != tc.wantUSD || zar != tc.wantZAR || eur != tc.wantEUR {
				t.Fatalf("usd=%q zar=%q eur=%q want %q %q %q", usd, zar, eur, tc.wantUSD, tc.wantZAR, tc.wantEUR)
			}
		})
	}
}

func TestDeriveAmountsMissingEurRate(t *testing.T) {
	store := &fakeStore{rates: map[string]string{"USD/ZAR": "18"}}
	usd, zar, eur := deriveAmounts(context.Background(), store, "USDC", "10", lib.StockQuote{})
	if usd != "10" || zar != "180.00000000" || eur != "" {
		t.Fatalf("usd=%q zar=%q eur=%q want 10 180.00000000 \"\"", usd, zar, eur)
	}
}

func TestWalletsForRunCapsAndRotates(t *testing.T) {
	wallets := make([]lib.Wallet, maxWalletsPerRun+15)
	for i := range wallets {
		wallets[i] = lib.Wallet{ID: fmt.Sprintf("w%d", i)}
	}

	hour := int64(time.Hour / time.Millisecond)
	first := walletsForRun(wallets, 0)
	second := walletsForRun(wallets, hour)

	if len(first) != maxWalletsPerRun || len(second) != maxWalletsPerRun {
		t.Fatalf("lengths = %d/%d, want %d", len(first), len(second), maxWalletsPerRun)
	}
	if first[0].ID != "w0" || second[0].ID != "w1" {
		t.Fatalf("rotation = %q/%q, want w0/w1", first[0].ID, second[0].ID)
	}
}

func TestStockContractSet(t *testing.T) {
	got := stockContractSet([]lib.TokenConfig{
		{Symbol: "EXTRA", Contract: "0xEXTRA"},
		{Symbol: "BLANK", Contract: "  "},
	})
	if !got["0xextra"] {
		t.Fatal("custom stock contract missing")
	}
	if len(got) != len(lib.BaseStockTokens)+1 {
		t.Fatalf("contract count = %d, want %d", len(got), len(lib.BaseStockTokens)+1)
	}
	for _, stock := range lib.BaseStockTokens {
		if !got[strings.ToLower(stock.Contract)] {
			t.Fatalf("base stock %s missing", stock.Symbol)
		}
	}
}
