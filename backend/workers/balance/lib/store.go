package lib

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// BalanceStore is the persistence boundary for the balance worker. The D1
// implementation lives in this package; tests use fakes.
type BalanceStore interface {
	FetchWallets(ctx context.Context) ([]Wallet, error)
	FetchTokenRegistry(ctx context.Context) (map[string]NetworkConfig, map[string][]TokenConfig, error)
	FetchStocks(ctx context.Context) ([]TokenConfig, error)
	FetchStockQuotes(ctx context.Context) (map[string]StockQuote, error)
	LatestFXRate(ctx context.Context, pair string) (string, error)
	SaveBalance(ctx context.Context, b Balance) error
	SavePortfolio(ctx context.Context, wallet string, p PortfolioSummary) error
	SavePortfolioSnapshot(ctx context.Context, wallet string, at int64, netWorth string) error
	FetchPortfolioSnapshots(ctx context.Context, wallet string) ([]PortfolioSnapshot, error)
	Close() error
}

// Store reads worker inputs and writes results to Cloudflare D1 through
// database/sql.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) FetchWallets(ctx context.Context) ([]Wallet, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, address, network, testnet FROM wallets ORDER BY address`)
	if err != nil {
		return nil, fmt.Errorf("fetch wallets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	wallets := make([]Wallet, 0)
	for rows.Next() {
		var w Wallet
		var testnet int64
		if err := rows.Scan(&w.ID, &w.Address, &w.Network, &testnet); err != nil {
			return nil, fmt.Errorf("scan wallet: %w", err)
		}
		w.Testnet = testnet != 0
		w.ID = strings.TrimSpace(w.ID)
		if strings.TrimSpace(w.Address) == "" {
			w.Address = w.ID
		}
		w.Address = strings.ToLower(strings.TrimSpace(w.Address))
		if w.Address == "" {
			continue
		}
		wallets = append(wallets, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate wallets: %w", err)
	}
	return wallets, nil
}

func (s *Store) FetchTokenRegistry(ctx context.Context) (map[string]NetworkConfig, map[string][]TokenConfig, error) {
	networks := map[string]NetworkConfig{}
	tokens := map[string][]TokenConfig{}

	if err := s.fetchNetworks(ctx, networks); err != nil {
		return CloneNetworkConfigMap(SupportedNetworks), CloneTokenConfigMap(ERC20Tokens), err
	}
	if err := s.fetchTokens(ctx, tokens); err != nil {
		return CloneNetworkConfigMap(SupportedNetworks), CloneTokenConfigMap(ERC20Tokens), err
	}
	if len(networks) == 0 {
		return CloneNetworkConfigMap(SupportedNetworks), CloneTokenConfigMap(ERC20Tokens), nil
	}

	for networkKey, defaultTokens := range ERC20Tokens {
		if _, ok := tokens[networkKey]; !ok {
			tokens[networkKey] = append([]TokenConfig(nil), defaultTokens...)
		}
	}
	for networkKey, cfg := range SupportedNetworks {
		if _, ok := networks[networkKey]; !ok {
			networks[networkKey] = cfg
		}
	}
	return networks, tokens, nil
}

func (s *Store) fetchNetworks(ctx context.Context, out map[string]NetworkConfig) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, alchemy_network, native_symbol, chain_id FROM networks`)
	if err != nil {
		return fmt.Errorf("fetch networks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var key, alchemyNetwork, nativeSymbol string
		var chainID int64
		if err := rows.Scan(&key, &alchemyNetwork, &nativeSymbol, &chainID); err != nil {
			return fmt.Errorf("scan network: %w", err)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		alchemyNetwork = strings.TrimSpace(alchemyNetwork)
		nativeSymbol = strings.TrimSpace(nativeSymbol)
		if key == "" || alchemyNetwork == "" || nativeSymbol == "" || chainID == 0 {
			continue
		}
		out[key] = NetworkConfig{
			AlchemyNetwork: alchemyNetwork,
			NativeSymbol:   nativeSymbol,
			ChainID:        int(chainID),
		}
	}
	return rows.Err()
}

func (s *Store) fetchTokens(ctx context.Context, out map[string][]TokenConfig) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT network, symbol, contract, decimals FROM tokens ORDER BY network, symbol`)
	if err != nil {
		return fmt.Errorf("fetch tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var network, symbol, contract string
		var decimals int64
		if err := rows.Scan(&network, &symbol, &contract, &decimals); err != nil {
			return fmt.Errorf("scan token: %w", err)
		}
		network = strings.ToLower(strings.TrimSpace(network))
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		contract = strings.TrimSpace(contract)
		if network == "" || symbol == "" || contract == "" || decimals <= 0 {
			continue
		}
		out[network] = append(out[network], TokenConfig{
			Symbol:   symbol,
			Contract: contract,
			Decimals: int(decimals),
		})
	}
	return rows.Err()
}

func (s *Store) LatestFXRate(ctx context.Context, pair string) (string, error) {
	pair = strings.ToUpper(strings.TrimSpace(pair))
	if pair == "" {
		return "", fmt.Errorf("LatestFXRate: pair is required")
	}
	key := strings.ReplaceAll(pair, "/", "_")

	var rate string
	err := s.db.QueryRowContext(ctx,
		`SELECT rate FROM fx_rates WHERE pair = ?`, key).Scan(&rate)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("LatestFXRate: no rate for %s", pair)
	}
	if err != nil {
		return "", fmt.Errorf("LatestFXRate: get %s: %w", pair, err)
	}
	return strings.TrimSpace(rate), nil
}

func (s *Store) SaveBalance(ctx context.Context, b Balance) error {
	if strings.TrimSpace(b.WalletID) == "" {
		return fmt.Errorf("SaveBalance: walletID is required")
	}
	walletKey := walletDocID(b)
	if walletKey == "" {
		return fmt.Errorf("SaveBalance: address or walletID is required")
	}
	network := strings.ToLower(strings.TrimSpace(b.Network))
	if network == "" {
		return fmt.Errorf("SaveBalance: network is required")
	}
	symbol := strings.ToUpper(strings.TrimSpace(b.TokenSymbol))
	if symbol == "" {
		return fmt.Errorf("SaveBalance: token symbol is required")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO balances (
			wallet_address, wallet_id, network, token_symbol, token_type,
			token_address, amount, ui_amount, multiplier,
			usd_amount, zar_amount, eur_amount, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (wallet_address, network, token_symbol) DO UPDATE SET
			wallet_id = excluded.wallet_id,
			token_type = excluded.token_type,
			token_address = excluded.token_address,
			amount = excluded.amount,
			ui_amount = excluded.ui_amount,
			multiplier = excluded.multiplier,
			usd_amount = excluded.usd_amount,
			zar_amount = excluded.zar_amount,
			eur_amount = excluded.eur_amount,
			fetched_at = excluded.fetched_at`,
		walletKey,
		strings.TrimSpace(b.WalletID),
		network,
		symbol,
		strings.TrimSpace(b.TokenType),
		strings.ToLower(strings.TrimSpace(b.TokenAddress)),
		strings.TrimSpace(b.Amount),
		strings.TrimSpace(b.UIAmount),
		strings.TrimSpace(b.Multiplier),
		strings.TrimSpace(b.USDAmount),
		strings.TrimSpace(b.ZARAmount),
		strings.TrimSpace(b.EURAmount),
		b.FetchedAt,
	)
	if err != nil {
		return fmt.Errorf("SaveBalance: upsert %s/%s: %w", walletKey, symbol, err)
	}
	return nil
}

func walletDocID(b Balance) string {
	address := strings.ToLower(strings.TrimSpace(b.Address))
	if address != "" {
		return address
	}
	return strings.ToLower(strings.TrimSpace(b.WalletID))
}

// WalletKey resolves the storage key used for a wallet.
func WalletKey(w Wallet) string {
	if address := strings.ToLower(strings.TrimSpace(w.Address)); address != "" {
		return address
	}
	return strings.ToLower(strings.TrimSpace(w.ID))
}

func portfolioHistoryKey(atMs int64) string {
	return time.UnixMilli(atMs).UTC().Format("2006-01-02")
}

func (s *Store) SavePortfolio(ctx context.Context, wallet string, p PortfolioSummary) error {
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if wallet == "" {
		return fmt.Errorf("SavePortfolio: wallet is required")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO portfolio_summaries (
			wallet_address, net_worth, usdc_value, eurc_value, zarp_value,
			stocks_value, other_value, day_pct, week_pct, month_pct, year_pct,
			day_abs, week_abs, month_abs, year_abs, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (wallet_address) DO UPDATE SET
			net_worth = excluded.net_worth,
			usdc_value = excluded.usdc_value,
			eurc_value = excluded.eurc_value,
			zarp_value = excluded.zarp_value,
			stocks_value = excluded.stocks_value,
			other_value = excluded.other_value,
			day_pct = excluded.day_pct,
			week_pct = excluded.week_pct,
			month_pct = excluded.month_pct,
			year_pct = excluded.year_pct,
			day_abs = excluded.day_abs,
			week_abs = excluded.week_abs,
			month_abs = excluded.month_abs,
			year_abs = excluded.year_abs,
			updated_at = excluded.updated_at`,
		wallet,
		p.NetWorth,
		p.USDC,
		p.EURC,
		p.ZARP,
		p.Stocks,
		p.Other,
		p.DayPct,
		p.WeekPct,
		p.MonthPct,
		p.YearPct,
		p.DayAbs,
		p.WeekAbs,
		p.MonthAbs,
		p.YearAbs,
		p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("SavePortfolio: upsert %s: %w", wallet, err)
	}
	return nil
}

func (s *Store) SavePortfolioSnapshot(ctx context.Context, wallet string, at int64, netWorth string) error {
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if wallet == "" {
		return fmt.Errorf("SavePortfolioSnapshot: wallet is required")
	}
	if at <= 0 {
		return fmt.Errorf("SavePortfolioSnapshot: timestamp is required")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO portfolio_snapshots (wallet_address, day, net_worth, at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (wallet_address, day) DO UPDATE SET
			net_worth = excluded.net_worth,
			at = excluded.at`,
		wallet,
		portfolioHistoryKey(at),
		strings.TrimSpace(netWorth),
		at,
	)
	if err != nil {
		return fmt.Errorf("SavePortfolioSnapshot: upsert %s: %w", wallet, err)
	}
	return nil
}

func (s *Store) FetchPortfolioSnapshots(ctx context.Context, wallet string) ([]PortfolioSnapshot, error) {
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if wallet == "" {
		return nil, fmt.Errorf("FetchPortfolioSnapshots: wallet is required")
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT at, net_worth FROM portfolio_snapshots WHERE wallet_address = ? ORDER BY at`,
		wallet)
	if err != nil {
		return nil, fmt.Errorf("FetchPortfolioSnapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	snapshots := make([]PortfolioSnapshot, 0)
	for rows.Next() {
		var snapshot PortfolioSnapshot
		if err := rows.Scan(&snapshot.At, &snapshot.NetWorth); err != nil {
			return nil, fmt.Errorf("FetchPortfolioSnapshots: scan: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FetchPortfolioSnapshots: iterate: %w", err)
	}
	return snapshots, nil
}

func (s *Store) FetchStocks(ctx context.Context) ([]TokenConfig, error) {
	quotes, err := s.FetchStockQuotes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TokenConfig, 0, len(quotes))
	for _, q := range quotes {
		decimals := q.Decimals
		if decimals <= 0 {
			decimals = 8
		}
		out = append(out, TokenConfig{
			Symbol:   q.Symbol,
			Contract: q.Contract,
			Decimals: decimals,
		})
	}
	return out, nil
}

func (s *Store) FetchStockQuotes(ctx context.Context) (map[string]StockQuote, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT address, symbol, decimals, price, multiplier FROM stocks`)
	if err != nil {
		return nil, fmt.Errorf("fetch stocks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]StockQuote{}
	for rows.Next() {
		var address, symbol, price, multiplier string
		var decimals int64
		if err := rows.Scan(&address, &symbol, &decimals, &price, &multiplier); err != nil {
			return nil, fmt.Errorf("fetch stocks: scan: %w", err)
		}
		address = strings.ToLower(strings.TrimSpace(address))
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" || address == "" {
			continue
		}
		if decimals <= 0 {
			decimals = 8
		}
		multiplier = strings.TrimSpace(multiplier)
		if multiplier == "" {
			multiplier = "1"
		}
		out[address] = StockQuote{
			Symbol:     symbol,
			Contract:   address,
			Decimals:   int(decimals),
			Price:      strings.TrimSpace(price),
			Multiplier: multiplier,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fetch stocks: iterate: %w", err)
	}
	return out, nil
}

func CloneNetworkConfigMap(in map[string]NetworkConfig) map[string]NetworkConfig {
	out := make(map[string]NetworkConfig, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func CloneTokenConfigMap(in map[string][]TokenConfig) map[string][]TokenConfig {
	out := make(map[string][]TokenConfig, len(in))
	for k, v := range in {
		copied := make([]TokenConfig, len(v))
		copy(copied, v)
		out[k] = copied
	}
	return out
}

func MergeBaseMainnetStocks(tokens map[string][]TokenConfig, stocks []TokenConfig) map[string][]TokenConfig {
	if tokens == nil {
		tokens = map[string][]TokenConfig{}
	}
	existing := tokens["base-mainnet"]
	seen := map[string]bool{}
	for _, t := range existing {
		seen[strings.ToLower(strings.TrimSpace(t.Contract))] = true
	}
	for _, s := range stocks {
		key := strings.ToLower(strings.TrimSpace(s.Contract))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		existing = append(existing, s)
	}
	tokens["base-mainnet"] = existing
	return tokens
}
