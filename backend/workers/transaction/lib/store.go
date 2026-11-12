package lib

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TransactionStore is the persistence boundary for the transaction worker. The
// D1 implementation lives in this package; tests use fakes.
type TransactionStore interface {
	FetchWallets(ctx context.Context) ([]Wallet, error)
	UpsertTransaction(ctx context.Context, t TransactionItem) error
	UpdateWalletLatestBlock(ctx context.Context, address string, blockNumber uint64) error
	Close() error
}

// Store reads wallets and writes enriched transactions to Cloudflare D1
// through database/sql.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Close() error {
	return s.db.Close()
}

// FetchWallets returns every wallet together with its transaction cursor. The
// cursor lives in wallet_transaction_cursors so this worker never has to alter
// the wallets table owned by the balance worker.
//
// Wallets are ordered least-recently-synced first (never-synced wallets first).
// combined with the per-run write budget, this keeps one busy wallet from
// starving the rest of the registry.
func (s *Store) FetchWallets(ctx context.Context) ([]Wallet, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.address, w.network, w.testnet,
		        COALESCE(c.latest_block, 0)
		 FROM wallets w
		 LEFT JOIN wallet_transaction_cursors c
		   ON c.wallet_address = CASE
		        WHEN TRIM(COALESCE(w.address, '')) = '' THEN LOWER(TRIM(w.id))
		        ELSE LOWER(TRIM(w.address))
		      END
		 ORDER BY COALESCE(c.updated_at, 0), w.address, w.id`)
	if err != nil {
		return nil, fmt.Errorf("fetch wallets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	wallets := make([]Wallet, 0)
	for rows.Next() {
		var w Wallet
		var testnet int64
		var latestBlock int64
		if err := rows.Scan(&w.ID, &w.Address, &w.Network, &testnet, &latestBlock); err != nil {
			return nil, fmt.Errorf("scan wallet: %w", err)
		}
		w.Testnet = testnet != 0
		w.ID = strings.TrimSpace(w.ID)
		w.Address = strings.ToLower(strings.TrimSpace(w.Address))
		w.Network = strings.TrimSpace(w.Network)
		if w.Address == "" {
			w.Address = strings.ToLower(w.ID)
		}
		if latestBlock > 0 {
			w.LatestBlock = uint64(latestBlock)
		}
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

// UpsertTransaction stores one transfer keyed by
// (wallet_address, tx_hash, direction), matching the old Firestore document id
// txHash_direction. Re-running the worker is idempotent.
func (s *Store) UpsertTransaction(ctx context.Context, t TransactionItem) error {
	address := strings.ToLower(strings.TrimSpace(t.WalletAddress))
	txHash := strings.ToLower(strings.TrimSpace(t.TxHash))
	direction := strings.TrimSpace(t.Direction)
	if address == "" || txHash == "" || direction == "" {
		return fmt.Errorf("walletAddress, txHash and direction are required")
	}
	if t.TimestampMs == 0 {
		t.TimestampMs = t.FetchedAtMs
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wallet_transactions (
			wallet_address, tx_hash, from_address, to_address, token_address,
			token_type, token_symbol, amount, fee_native, fee_usd, fee_zar,
			fee_eur, usd_amount, zar_amount, eur_amount, network, direction,
			state, block_number, timestamp_ms, fetched_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (wallet_address, tx_hash, direction) DO UPDATE SET
			from_address = excluded.from_address,
			to_address = excluded.to_address,
			token_address = excluded.token_address,
			token_type = excluded.token_type,
			token_symbol = excluded.token_symbol,
			amount = excluded.amount,
			fee_native = excluded.fee_native,
			fee_usd = excluded.fee_usd,
			fee_zar = excluded.fee_zar,
			fee_eur = excluded.fee_eur,
			usd_amount = excluded.usd_amount,
			zar_amount = excluded.zar_amount,
			eur_amount = excluded.eur_amount,
			network = excluded.network,
			state = excluded.state,
			block_number = excluded.block_number,
			timestamp_ms = excluded.timestamp_ms,
			fetched_at_ms = excluded.fetched_at_ms`,
		address,
		txHash,
		strings.ToLower(strings.TrimSpace(t.FromAddress)),
		strings.ToLower(strings.TrimSpace(t.ToAddress)),
		strings.ToLower(strings.TrimSpace(t.TokenAddress)),
		strings.TrimSpace(t.TokenType),
		strings.TrimSpace(t.TokenSymbol),
		strings.TrimSpace(t.Amount),
		strings.TrimSpace(t.FeeNative),
		strings.TrimSpace(t.FeeUSD),
		strings.TrimSpace(t.FeeZAR),
		strings.TrimSpace(t.FeeEUR),
		strings.TrimSpace(t.USDAmount),
		strings.TrimSpace(t.ZARAmount),
		strings.TrimSpace(t.EURAmount),
		strings.TrimSpace(t.Network),
		direction,
		strings.TrimSpace(t.State),
		int64(t.BlockNumber),
		t.TimestampMs,
		t.FetchedAtMs,
	)
	if err != nil {
		return fmt.Errorf("upsert transaction %s %s: %w", txHash, direction, err)
	}
	return nil
}

// UpdateWalletLatestBlock advances the cursor monotonically. A late, stale run
// can never move it backwards.
func (s *Store) UpdateWalletLatestBlock(ctx context.Context, address string, blockNumber uint64) error {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return fmt.Errorf("address is required")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wallet_transaction_cursors (wallet_address, latest_block, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT (wallet_address) DO UPDATE SET
			latest_block = CASE
				WHEN excluded.latest_block > wallet_transaction_cursors.latest_block
				THEN excluded.latest_block
				ELSE wallet_transaction_cursors.latest_block
			END,
			updated_at = excluded.updated_at`,
		address, int64(blockNumber), time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("update wallet latest block: %w", err)
	}
	return nil
}

// LatestBlock returns a wallet's cursor, or 0 when it has never synced.
func (s *Store) LatestBlock(ctx context.Context, address string) (uint64, error) {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return 0, fmt.Errorf("address is required")
	}

	var latestBlock int64
	err := s.db.QueryRowContext(ctx,
		`SELECT latest_block FROM wallet_transaction_cursors WHERE wallet_address = ?`,
		address,
	).Scan(&latestBlock)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get wallet latest block %s: %w", address, err)
	}
	if latestBlock < 0 {
		return 0, nil
	}
	return uint64(latestBlock), nil
}

// WalletKey resolves the storage key used for a wallet.
func WalletKey(w Wallet) string {
	if address := strings.ToLower(strings.TrimSpace(w.Address)); address != "" {
		return address
	}
	return strings.ToLower(strings.TrimSpace(w.ID))
}
