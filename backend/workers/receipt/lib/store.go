package lib

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// PaymentStore is the persistence boundary for the receipt worker. The D1
// implementation lives in this package; tests use fakes and in-memory SQLite.
type PaymentStore interface {
	ExpirePending(ctx context.Context, nowMs int64) (int64, error)
	ListPending(ctx context.Context, nowMs int64, limit int) ([]Payment, error)
	ListCredits(ctx context.Context, query CreditQuery) ([]Credit, error)
	MarkPaid(ctx context.Context, id, txHash string, nowMs int64) error
}

// Store reads and writes the shared `afika` D1 database through database/sql.
type Store struct {
	db *sql.DB
}

// NewStore wraps an already-open D1 connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Close() error {
	return s.db.Close()
}

// ExpirePending flips every stale pending payment to expired and returns how
// many rows changed. It runs before matching so an expired payment can never be
// paid at a stale rate.
func (s *Store) ExpirePending(ctx context.Context, nowMs int64) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE pos_payments
		 SET status = 'expired', updated_at = ?
		 WHERE status = 'pending' AND expires_at > 0 AND expires_at <= ?`,
		nowMs, nowMs,
	)
	if err != nil {
		return 0, fmt.Errorf("store: expire pending: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		// D1 always reports row counts; a driver without support should not
		// turn a successful expiry into a failed run.
		return 0, nil
	}
	return affected, nil
}

// ListPending returns the live pending payments, oldest first.
func (s *Store) ListPending(ctx context.Context, nowMs int64, limit int) ([]Payment, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, receipt_no, amount_cents, asset, network, address, crypto_amount,
		        created_at, expires_at
		 FROM pos_payments
		 WHERE status = 'pending' AND (expires_at = 0 OR expires_at > ?)
		 ORDER BY created_at ASC
		 LIMIT ?`,
		nowMs, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list pending: %w", err)
	}
	defer func() { _ = rows.Close() }()

	payments := make([]Payment, 0, limit)
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.ReceiptNo, &p.AmountCents, &p.Asset, &p.Network,
			&p.Address, &p.CryptoAmount, &p.CreatedAt, &p.ExpiresAt); err != nil {
			return nil, fmt.Errorf("store: scan pending: %w", err)
		}
		payments = append(payments, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate pending: %w", err)
	}
	return payments, nil
}

// ListCredits returns incoming transfers for a wallet that could settle a
// payment. Transactions already attached to a paid payment are excluded so a
// single transfer cannot settle two receipts.
func (s *Store) ListCredits(ctx context.Context, query CreditQuery) ([]Credit, error) {
	address := strings.ToLower(strings.TrimSpace(query.Address))
	if address == "" {
		return nil, errors.New("store: credit query address is required")
	}
	if strings.TrimSpace(query.Symbol) == "" {
		return nil, errors.New("store: credit query symbol is required")
	}
	if len(query.Networks) == 0 {
		return nil, errors.New("store: credit query networks are required")
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(query.Networks)), ",")
	sqlQuery := `SELECT tx_hash, token_symbol, amount, network, timestamp_ms
	             FROM wallet_transactions
	             WHERE wallet_address = ?
	               AND direction = 'credit'
	               AND timestamp_ms >= ?
	               AND lower(token_symbol) = lower(?)
	               AND network IN (` + placeholders + `)
	               AND NOT EXISTS (
	                   SELECT 1 FROM pos_payments p WHERE p.tx_hash = wallet_transactions.tx_hash
	               )
	             ORDER BY timestamp_ms ASC
	             LIMIT ?`

	args := make([]any, 0, len(query.Networks)+4)
	args = append(args, address, query.SinceMs, query.Symbol)
	for _, network := range query.Networks {
		args = append(args, network)
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list credits: %w", err)
	}
	defer func() { _ = rows.Close() }()

	credits := make([]Credit, 0, 8)
	for rows.Next() {
		var c Credit
		if err := rows.Scan(&c.TxHash, &c.TokenSymbol, &c.Amount, &c.Network, &c.TimestampMs); err != nil {
			return nil, fmt.Errorf("store: scan credit: %w", err)
		}
		credits = append(credits, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate credits: %w", err)
	}
	return credits, nil
}

// MarkPaid settles a payment. It only transitions pending rows, so re-running
// the worker never rewrites a paid payment's transaction hash.
func (s *Store) MarkPaid(ctx context.Context, id, txHash string, nowMs int64) error {
	id = strings.TrimSpace(id)
	txHash = strings.ToLower(strings.TrimSpace(txHash))
	if id == "" || txHash == "" {
		return errors.New("store: mark paid needs an id and a tx hash")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE pos_payments
		 SET status = 'paid', tx_hash = ?, updated_at = ?
		 WHERE id = ? AND status = 'pending'`,
		txHash, nowMs, id,
	); err != nil {
		return fmt.Errorf("store: mark paid %s: %w", id, err)
	}
	return nil
}
