package lib

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Store is the persistence boundary of the API. The D1 implementation lives in
// this package; handler tests use a fake.
type Store interface {
	UpsertWallet(ctx context.Context, in WalletInput) (Wallet, bool, error)
	GetWalletSettings(ctx context.Context, address string) (WalletSettings, error)
	UpsertWalletSettings(ctx context.Context, in WalletSettingsInput) (WalletSettings, error)
	ListBalances(ctx context.Context, address string, after *BalanceCursor, limit int) (BalancePage, error)
	ListTransactions(ctx context.Context, address string, after *TransactionCursor, limit int) (TransactionPage, error)
	GetFXRates(ctx context.Context) (FXRates, error)

	CreatePOSPayment(ctx context.Context, p POSPayment) (POSPayment, error)
	GetPOSPayment(ctx context.Context, id string) (POSPayment, error)
	UpdatePOSPayment(ctx context.Context, p POSPayment) (POSPayment, error)
}

// GetFXRates reads the latest USD-based rates written by workers/forex.
func (s *D1Store) GetFXRates(ctx context.Context) (FXRates, error) {
	rates := FXRates{USD: "1"}
	rows, err := s.db.QueryContext(ctx, `SELECT pair, rate, updated_at FROM fx_rates WHERE pair IN ('USD_ZAR', 'USD_EUR')`)
	if err != nil {
		return FXRates{}, fmt.Errorf("query fx rates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var pair, rate string
		var updatedAt int64
		if err := rows.Scan(&pair, &rate, &updatedAt); err != nil {
			return FXRates{}, fmt.Errorf("scan fx rate: %w", err)
		}
		switch pair {
		case "USD_ZAR":
			rates.ZAR = rate
		case "USD_EUR":
			rates.EUR = rate
		}
		if updatedAt > rates.UpdatedAt {
			rates.UpdatedAt = updatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return FXRates{}, fmt.Errorf("iterate fx rates: %w", err)
	}
	return rates, nil
}

// D1Store reads and writes the shared `afika` D1 database through database/sql.
type D1Store struct {
	db  *sql.DB
	now func() time.Time
}

// NewD1Store wraps an already-open D1 connection.
func NewD1Store(db *sql.DB) *D1Store {
	return &D1Store{db: db, now: time.Now}
}

// UpsertWallet registers a wallet address. The row id is the lowercased
// address, so registering the same address again updates the network while
// preserving created_at. D1 does not support transactions through
// database/sql, so both statements are idempotent and a retry is safe.
func (s *D1Store) UpsertWallet(ctx context.Context, in WalletInput) (Wallet, bool, error) {
	address, err := normalizeAddress(in.Address)
	if err != nil {
		return Wallet{}, false, err
	}
	if strings.TrimSpace(in.Network) == "" {
		return Wallet{}, false, fmt.Errorf("network is required")
	}
	network, testnet, err := normalizeNetwork(in.Network)
	if err != nil {
		return Wallet{}, false, err
	}
	testnet = testnet || in.Testnet
	nowMs := s.now().UnixMilli()

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO wallets (id, address, network, testnet, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO NOTHING`,
		address, address, network, boolToInt(testnet), nowMs, nowMs,
	)
	if err != nil {
		return Wallet{}, false, fmt.Errorf("insert wallet: %w", err)
	}

	created := false
	if affected, err := result.RowsAffected(); err == nil && affected > 0 {
		created = true
	}
	if !created {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE wallets
			 SET address = ?, network = ?, testnet = ?, updated_at = ?
			 WHERE id = ?`,
			address, network, boolToInt(testnet), nowMs, address,
		); err != nil {
			return Wallet{}, false, fmt.Errorf("update wallet: %w", err)
		}
	}

	wallet, err := s.walletByID(ctx, address)
	if err != nil {
		return Wallet{}, false, err
	}
	return wallet, created, nil
}

// GetWalletSettings returns the saved display currency for a wallet. Wallets
// that have never saved a preference fall back to USD without a row.
func (s *D1Store) GetWalletSettings(ctx context.Context, address string) (WalletSettings, error) {
	address, err := normalizeAddress(address)
	if err != nil {
		return WalletSettings{}, err
	}

	var settings WalletSettings
	err = s.db.QueryRowContext(ctx,
		`SELECT wallet_address, preferred_currency, created_at, updated_at
		 FROM wallet_settings
		 WHERE wallet_address = ?`,
		address,
	).Scan(&settings.Address, &settings.PreferredCurrency, &settings.CreatedAt, &settings.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return WalletSettings{Address: address, PreferredCurrency: defaultCurrency}, nil
	}
	if err != nil {
		return WalletSettings{}, fmt.Errorf("get wallet settings: %w", err)
	}

	settings.Address = strings.ToLower(strings.TrimSpace(settings.Address))
	currency, err := normalizeCurrency(settings.PreferredCurrency)
	if err != nil {
		currency = defaultCurrency
	}
	settings.PreferredCurrency = currency
	return settings, nil
}

// UpsertWalletSettings saves a wallet's display currency. Repeating the same
// value is idempotent and preserves created_at.
func (s *D1Store) UpsertWalletSettings(ctx context.Context, in WalletSettingsInput) (WalletSettings, error) {
	address, err := normalizeAddress(in.Address)
	if err != nil {
		return WalletSettings{}, err
	}
	currency, err := normalizeCurrency(in.PreferredCurrency)
	if err != nil {
		return WalletSettings{}, err
	}
	nowMs := s.now().UnixMilli()

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO wallet_settings (wallet_address, preferred_currency, created_at, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (wallet_address) DO UPDATE SET
			preferred_currency = excluded.preferred_currency,
			updated_at = excluded.updated_at`,
		address, currency, nowMs, nowMs,
	); err != nil {
		return WalletSettings{}, fmt.Errorf("upsert wallet settings: %w", err)
	}

	return s.GetWalletSettings(ctx, address)
}

// walletByID returns the wallet registered under the row id.
func (s *D1Store) walletByID(ctx context.Context, id string) (Wallet, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return Wallet{}, fmt.Errorf("wallet id is required")
	}

	var (
		wallet  Wallet
		testnet int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT address, network, testnet, created_at, updated_at
		 FROM wallets
		 WHERE id = ?`,
		id,
	).Scan(&wallet.Address, &wallet.Network, &testnet, &wallet.CreatedAt, &wallet.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Wallet{}, fmt.Errorf("wallet %q not found", id)
	}
	if err != nil {
		return Wallet{}, fmt.Errorf("select wallet: %w", err)
	}

	wallet.Address = strings.ToLower(strings.TrimSpace(wallet.Address))
	wallet.Network = strings.TrimSpace(wallet.Network)
	wallet.Testnet = testnet != 0
	return wallet, nil
}

// ListBalances returns one page of balances ordered by network and symbol.
func (s *D1Store) ListBalances(ctx context.Context, address string, after *BalanceCursor, limit int) (BalancePage, error) {
	address = strings.ToLower(strings.TrimSpace(address))
	limit = clampLimit(limit)

	query := `SELECT wallet_address, wallet_id, network, token_type, token_symbol,
	                 token_address, amount, ui_amount, multiplier, usd_amount,
	                 zar_amount, eur_amount, fetched_at
	          FROM balances
	          WHERE wallet_address = ?`
	args := []any{address}
	if after != nil {
		query += ` AND (network > ? OR (network = ? AND token_symbol > ?))`
		args = append(args, after.Network, after.Network, after.TokenSymbol)
	}
	query += ` ORDER BY network ASC, token_symbol ASC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return BalancePage{}, fmt.Errorf("list balances: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Balance, 0, limit+1)
	for rows.Next() {
		var b Balance
		if err := rows.Scan(
			&b.WalletAddress, &b.WalletID, &b.Network, &b.TokenType, &b.TokenSymbol,
			&b.TokenAddress, &b.Amount, &b.UIAmount, &b.Multiplier, &b.USDAmount,
			&b.ZARAmount, &b.EURAmount, &b.FetchedAt,
		); err != nil {
			return BalancePage{}, fmt.Errorf("scan balance: %w", err)
		}
		b.WalletAddress = strings.ToLower(strings.TrimSpace(b.WalletAddress))
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return BalancePage{}, fmt.Errorf("iterate balances: %w", err)
	}

	page := BalancePage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasMore = true
	}
	return page, nil
}

// ListTransactions returns one page of transactions ordered newest first.
func (s *D1Store) ListTransactions(ctx context.Context, address string, after *TransactionCursor, limit int) (TransactionPage, error) {
	address = strings.ToLower(strings.TrimSpace(address))
	limit = clampLimit(limit)

	query := `SELECT wallet_address, tx_hash, from_address, to_address, token_address,
	                 token_type, token_symbol, amount, fee_native, fee_usd, fee_zar,
	                 fee_eur, usd_amount, zar_amount, eur_amount, network, direction,
	                 state, block_number, timestamp_ms, fetched_at_ms
	          FROM wallet_transactions
	          WHERE wallet_address = ?`
	args := []any{address}
	if after != nil {
		query += ` AND (timestamp_ms < ?
		            OR (timestamp_ms = ?
		                AND (tx_hash < ? OR (tx_hash = ? AND direction < ?))))`
		args = append(args, after.TimestampMs, after.TimestampMs, after.TxHash, after.TxHash, after.Direction)
	}
	query += ` ORDER BY timestamp_ms DESC, tx_hash DESC, direction DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return TransactionPage{}, fmt.Errorf("list transactions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Transaction, 0, limit+1)
	for rows.Next() {
		var (
			tx          Transaction
			blockNumber int64
		)
		if err := rows.Scan(
			&tx.WalletAddress, &tx.TxHash, &tx.FromAddress, &tx.ToAddress, &tx.TokenAddress,
			&tx.TokenType, &tx.TokenSymbol, &tx.Amount, &tx.FeeNative, &tx.FeeUSD, &tx.FeeZAR,
			&tx.FeeEUR, &tx.USDAmount, &tx.ZARAmount, &tx.EURAmount, &tx.Network, &tx.Direction,
			&tx.State, &blockNumber, &tx.TimestampMs, &tx.FetchedAtMs,
		); err != nil {
			return TransactionPage{}, fmt.Errorf("scan transaction: %w", err)
		}
		if blockNumber > 0 {
			tx.BlockNumber = uint64(blockNumber)
		}
		tx.WalletAddress = strings.ToLower(strings.TrimSpace(tx.WalletAddress))
		tx.TxHash = strings.ToLower(strings.TrimSpace(tx.TxHash))
		tx.Direction = strings.TrimSpace(tx.Direction)
		tx.Timestamp = tx.TimestampMs
		tx.FetchedAt = tx.FetchedAtMs
		items = append(items, tx)
	}
	if err := rows.Err(); err != nil {
		return TransactionPage{}, fmt.Errorf("iterate transactions: %w", err)
	}

	page := TransactionPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasMore = true
	}
	return page, nil
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultPageSize
	}
	if limit > maxPageSize {
		return maxPageSize
	}
	return limit
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
