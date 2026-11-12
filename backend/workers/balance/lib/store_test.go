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

	schema, err := os.ReadFile(filepath.Join("..", "migrations", "0001_init.sql"))
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

func TestStoreFetchWallets(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	_, err := db.Exec(`INSERT INTO wallets (id, address, network, testnet) VALUES
		('w1', '0xABC', 'base-mainnet', 0),
		('W2', '', 'eth-mainnet', 1),
		('', '   ', 'base-mainnet', 0)`)
	if err != nil {
		t.Fatalf("seed wallets: %v", err)
	}

	got, err := store.FetchWallets(ctx)
	if err != nil {
		t.Fatalf("FetchWallets: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("wallets = %d, want 2 (%+v)", len(got), got)
	}

	byID := map[string]Wallet{}
	for _, w := range got {
		byID[w.ID] = w
	}
	if w := byID["w1"]; w.Address != "0xabc" || w.Network != "base-mainnet" || w.Testnet {
		t.Fatalf("w1 = %+v", w)
	}
	if w := byID["W2"]; w.Address != "w2" || !w.Testnet {
		t.Fatalf("w2 = %+v", w)
	}
}

func TestStoreFetchTokenRegistryDefaultsWhenEmpty(t *testing.T) {
	store, _ := newTestStore(t)

	networks, tokens, err := store.FetchTokenRegistry(context.Background())
	if err != nil {
		t.Fatalf("FetchTokenRegistry: %v", err)
	}
	if len(networks) != len(SupportedNetworks) {
		t.Fatalf("networks = %d, want %d", len(networks), len(SupportedNetworks))
	}
	if len(tokens) != len(ERC20Tokens) {
		t.Fatalf("tokens = %d, want %d", len(tokens), len(ERC20Tokens))
	}
	if got := networks["base-mainnet"]; got.ChainID != 8453 || got.NativeSymbol != "ETH" {
		t.Fatalf("base-mainnet = %+v", got)
	}
	if len(tokens["base-mainnet"]) != len(ERC20Tokens["base-mainnet"]) {
		t.Fatalf("base tokens = %d, want %d", len(tokens["base-mainnet"]), len(ERC20Tokens["base-mainnet"]))
	}
}

func TestStoreFetchTokenRegistryMergesDefaults(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	_, err := db.Exec(`INSERT INTO networks (key, alchemy_network, native_symbol, chain_id) VALUES
		('base-mainnet', 'base-mainnet', 'ETH', 8453),
		('gnosis-mainnet', 'gnosis-mainnet', 'XDAI', 100),
		('invalid', '', 'ETH', 0)`)
	if err != nil {
		t.Fatalf("seed networks: %v", err)
	}
	_, err = db.Exec(`INSERT INTO tokens (network, symbol, contract, decimals) VALUES
		('base-mainnet', 'custom', '0xCustom', 6),
		('base-mainnet', 'BAD', '', 0)`)
	if err != nil {
		t.Fatalf("seed tokens: %v", err)
	}

	networks, tokens, err := store.FetchTokenRegistry(ctx)
	if err != nil {
		t.Fatalf("FetchTokenRegistry: %v", err)
	}
	if _, ok := networks["invalid"]; ok {
		t.Fatalf("invalid network row should be ignored")
	}
	// base-mainnet and gnosis-mainnet come from D1, the rest from defaults.
	if len(networks) != len(SupportedNetworks) {
		t.Fatalf("networks = %d, want %d", len(networks), len(SupportedNetworks))
	}

	base := tokens["base-mainnet"]
	if len(base) != 1 || base[0].Symbol != "CUSTOM" || base[0].Contract != "0xCustom" || base[0].Decimals != 6 {
		t.Fatalf("base tokens = %+v, want the D1 override only", base)
	}
	if got := tokens["eth-mainnet"]; len(got) != len(ERC20Tokens["eth-mainnet"]) {
		t.Fatalf("eth-mainnet should fall back to defaults, got %d", len(got))
	}
}

func TestStoreLatestFXRate(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	if _, err := db.Exec(`INSERT INTO fx_rates (pair, rate) VALUES ('USD_ZAR', '18.75')`); err != nil {
		t.Fatalf("seed fx: %v", err)
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

func TestStoreSaveBalanceUpserts(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	base := Balance{
		WalletID:     "w1",
		Address:      "0xABC",
		Network:      "Base-Mainnet",
		TokenType:    "erc20",
		TokenSymbol:  "usdc",
		TokenAddress: "0x8335",
		Amount:       "10",
		USDAmount:    "10",
		FetchedAt:    1000,
	}
	if err := store.SaveBalance(ctx, base); err != nil {
		t.Fatalf("SaveBalance: %v", err)
	}

	updated := base
	updated.Amount = "12.5"
	updated.USDAmount = "12.5"
	updated.ZARAmount = "225"
	updated.FetchedAt = 2000
	if err := store.SaveBalance(ctx, updated); err != nil {
		t.Fatalf("SaveBalance update: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM balances`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("rows = %d, want 1", count)
	}

	var wallet, network, symbol, amount, zar string
	var fetchedAt int64
	err := db.QueryRow(`SELECT wallet_address, network, token_symbol, amount, zar_amount, fetched_at FROM balances`).
		Scan(&wallet, &network, &symbol, &amount, &zar, &fetchedAt)
	if err != nil {
		t.Fatalf("select balance: %v", err)
	}
	if wallet != "0xabc" || network != "base-mainnet" || symbol != "USDC" {
		t.Fatalf("key = %q/%q/%q", wallet, network, symbol)
	}
	if amount != "12.5" || zar != "225" || fetchedAt != 2000 {
		t.Fatalf("values = amount %q zar %q fetchedAt %d", amount, zar, fetchedAt)
	}
}

func TestStoreSaveBalanceValidation(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name string
		in   Balance
	}{
		{name: "missing wallet id", in: Balance{Address: "0xabc", Network: "base-mainnet", TokenSymbol: "USDC"}},
		{name: "missing network", in: Balance{WalletID: "w1", Address: "0xabc", TokenSymbol: "USDC"}},
		{name: "missing symbol", in: Balance{WalletID: "w1", Address: "0xabc", Network: "base-mainnet"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.SaveBalance(ctx, tc.in); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestStoreFetchStockQuotes(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	_, err := db.Exec(`INSERT INTO stocks (address, symbol, decimals, price, multiplier) VALUES
		('0xB20000000000000000000078ee7ce2fE4908108C', 'nvda', 8, '123.45', '2'),
		('', 'BROKEN', 8, '1', '1')`)
	if err != nil {
		t.Fatalf("seed stocks: %v", err)
	}

	quotes, err := store.FetchStockQuotes(ctx)
	if err != nil {
		t.Fatalf("FetchStockQuotes: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("quotes = %d, want 1", len(quotes))
	}
	q, ok := quotes["0xb20000000000000000000078ee7ce2fe4908108c"]
	if !ok {
		t.Fatalf("missing lowered contract in %+v", quotes)
	}
	if q.Symbol != "NVDA" || q.Price != "123.45" || q.Multiplier != "2" || q.Decimals != 8 {
		t.Fatalf("quote = %+v", q)
	}

	stocks, err := store.FetchStocks(ctx)
	if err != nil {
		t.Fatalf("FetchStocks: %v", err)
	}
	if len(stocks) != 1 || stocks[0].Symbol != "NVDA" || stocks[0].Contract != q.Contract {
		t.Fatalf("stocks = %+v", stocks)
	}
}

func TestStorePortfolioAndSnapshots(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	summary := PortfolioSummary{
		Portfolio:            Portfolio{NetWorth: "210", USDC: "10", Stocks: "200"},
		PortfolioPerformance: PortfolioPerformance{DayPct: "5.00", DayAbs: "10"},
		UpdatedAt:            1_700_000_000_000,
	}
	if err := store.SavePortfolio(ctx, "0xABC", summary); err != nil {
		t.Fatalf("SavePortfolio: %v", err)
	}
	summary.NetWorth = "220"
	if err := store.SavePortfolio(ctx, "0xabc", summary); err != nil {
		t.Fatalf("SavePortfolio update: %v", err)
	}

	var wallet, netWorth, usdc, stocks, dayPct string
	err := db.QueryRow(`SELECT wallet_address, net_worth, usdc_value, stocks_value, day_pct FROM portfolio_summaries`).
		Scan(&wallet, &netWorth, &usdc, &stocks, &dayPct)
	if err != nil {
		t.Fatalf("select portfolio: %v", err)
	}
	if wallet != "0xabc" || netWorth != "220" || usdc != "10" || stocks != "200" || dayPct != "5.00" {
		t.Fatalf("portfolio row = %q %q %q %q %q", wallet, netWorth, usdc, stocks, dayPct)
	}

	day := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	if err := store.SavePortfolioSnapshot(ctx, "0xabc", day.UnixMilli(), "200"); err != nil {
		t.Fatalf("SavePortfolioSnapshot: %v", err)
	}
	if err := store.SavePortfolioSnapshot(ctx, "0xabc", day.Add(time.Hour).UnixMilli(), "220"); err != nil {
		t.Fatalf("SavePortfolioSnapshot update: %v", err)
	}
	if err := store.SavePortfolioSnapshot(ctx, "0xabc", day.AddDate(0, 0, 1).UnixMilli(), "230"); err != nil {
		t.Fatalf("SavePortfolioSnapshot next day: %v", err)
	}

	snapshots, err := store.FetchPortfolioSnapshots(ctx, "0xABC")
	if err != nil {
		t.Fatalf("FetchPortfolioSnapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshots = %d, want 2 (%+v)", len(snapshots), snapshots)
	}
	if snapshots[0].NetWorth != "220" {
		t.Fatalf("same-day snapshot should be overwritten, got %+v", snapshots[0])
	}
	if snapshots[1].NetWorth != "230" {
		t.Fatalf("next-day snapshot = %+v", snapshots[1])
	}

	if err := store.SavePortfolioSnapshot(ctx, "", day.UnixMilli(), "1"); err == nil {
		t.Fatal("expected error for empty wallet")
	}
	if err := store.SavePortfolioSnapshot(ctx, "0xabc", 0, "1"); err == nil {
		t.Fatal("expected error for missing timestamp")
	}
	if _, err := store.FetchPortfolioSnapshots(ctx, ""); err == nil {
		t.Fatal("expected error for empty wallet")
	}
}

func TestWalletKey(t *testing.T) {
	tests := []struct {
		name string
		in   Wallet
		want string
	}{
		{name: "prefers address", in: Wallet{ID: "w1", Address: " 0xABC "}, want: "0xabc"},
		{name: "falls back to id", in: Wallet{ID: "W1"}, want: "w1"},
		{name: "empty", in: Wallet{}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := WalletKey(tc.in); got != tc.want {
				t.Fatalf("WalletKey = %q, want %q", got, tc.want)
			}
		})
	}
}
