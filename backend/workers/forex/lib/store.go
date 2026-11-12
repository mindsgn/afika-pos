package lib

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// FXStore is the persistence boundary for the forex worker. The D1
// implementation lives in this package; tests use fakes.
type FXStore interface {
	UpsertFXRate(ctx context.Context, pair, rate string, fetchedAt int64) error
}

// Store writes FX rates to Cloudflare D1 through database/sql.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Close() error {
	return s.db.Close()
}

// UpsertFXRate stores a rate keyed by the uppercase pair with "/" replaced by
// "_" (USD/ZAR -> USD_ZAR). That key format matches the balance worker's
// fx_rates reads. Passing fetchedAt == 0 uses the current time.
func (s *Store) UpsertFXRate(ctx context.Context, pair, rate string, fetchedAt int64) error {
	pair = strings.ToUpper(strings.TrimSpace(pair))
	if pair == "" {
		return errors.New("store: pair is required")
	}
	rate = strings.TrimSpace(rate)
	if rate == "" {
		return errors.New("store: rate is required")
	}
	if fetchedAt == 0 {
		fetchedAt = time.Now().UnixMilli()
	}

	key := FXPairKey(pair)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fx_rates (pair, rate, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT (pair) DO UPDATE SET
		 	rate = excluded.rate,
		 	updated_at = excluded.updated_at`,
		key, rate, fetchedAt,
	)
	if err != nil {
		return fmt.Errorf("store: upsert %s: %w", pair, err)
	}
	return nil
}

// LatestFXRate returns the most recently stored rate for a pair such as
// "USD/ZAR". It is used by tests and mirrors the balance worker's lookup.
func (s *Store) LatestFXRate(ctx context.Context, pair string) (string, error) {
	pair = strings.ToUpper(strings.TrimSpace(pair))
	if pair == "" {
		return "", errors.New("LatestFXRate: pair is required")
	}

	var rate string
	err := s.db.QueryRowContext(ctx,
		`SELECT rate FROM fx_rates WHERE pair = ?`, FXPairKey(pair)).Scan(&rate)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("LatestFXRate: no rate for %s", pair)
	}
	if err != nil {
		return "", fmt.Errorf("LatestFXRate: get %s: %w", pair, err)
	}
	return strings.TrimSpace(rate), nil
}

// FXPairKey normalizes a pair to its D1 primary key: uppercase with "/"
// replaced by "_".
func FXPairKey(pair string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(pair)), "/", "_")
}
