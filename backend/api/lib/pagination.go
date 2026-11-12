package lib

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

const (
	cursorVersion   = 1
	maxCursorLength = 512

	balanceResource     = "balances"
	transactionResource = "transactions"
)

// ErrInvalidCursor is returned for a cursor this API did not issue.
var ErrInvalidCursor = errors.New("invalid cursor")

// BalanceCursor is the position after the last balance of a page. Balances are
// ordered by (network, token_symbol), matching the primary key.
type BalanceCursor struct {
	Network     string
	TokenSymbol string
}

// TransactionCursor is the position after the last transaction of a page.
// Transactions are ordered by (timestamp_ms DESC, tx_hash DESC, direction DESC).
type TransactionCursor struct {
	TimestampMs int64
	TxHash      string
	Direction   string
}

// cursorEnvelope is the wire format of both cursors. It is opaque to clients,
// so fields can change as long as the version is bumped.
type cursorEnvelope struct {
	Version     int    `json:"v"`
	Resource    string `json:"r"`
	Network     string `json:"network,omitempty"`
	TokenSymbol string `json:"tokenSymbol,omitempty"`
	TimestampMs int64  `json:"timestampMs"`
	TxHash      string `json:"txHash,omitempty"`
	Direction   string `json:"direction,omitempty"`
}

// EncodeBalanceCursor returns an opaque, URL-safe cursor.
func EncodeBalanceCursor(c BalanceCursor) string {
	return encodeCursor(cursorEnvelope{
		Version:     cursorVersion,
		Resource:    balanceResource,
		Network:     c.Network,
		TokenSymbol: c.TokenSymbol,
	})
}

// EncodeTransactionCursor returns an opaque, URL-safe cursor.
func EncodeTransactionCursor(c TransactionCursor) string {
	return encodeCursor(cursorEnvelope{
		Version:     cursorVersion,
		Resource:    transactionResource,
		TimestampMs: c.TimestampMs,
		TxHash:      c.TxHash,
		Direction:   c.Direction,
	})
}

// DecodeBalanceCursor parses a cursor issued by EncodeBalanceCursor.
func DecodeBalanceCursor(raw string) (BalanceCursor, error) {
	env, err := decodeCursor(raw, balanceResource)
	if err != nil {
		return BalanceCursor{}, err
	}
	if env.Network == "" || env.TokenSymbol == "" {
		return BalanceCursor{}, ErrInvalidCursor
	}
	return BalanceCursor{Network: env.Network, TokenSymbol: env.TokenSymbol}, nil
}

// DecodeTransactionCursor parses a cursor issued by EncodeTransactionCursor.
func DecodeTransactionCursor(raw string) (TransactionCursor, error) {
	env, err := decodeCursor(raw, transactionResource)
	if err != nil {
		return TransactionCursor{}, err
	}
	if env.TxHash == "" || (env.Direction != "credit" && env.Direction != "debit") {
		return TransactionCursor{}, ErrInvalidCursor
	}
	return TransactionCursor{
		TimestampMs: env.TimestampMs,
		TxHash:      env.TxHash,
		Direction:   env.Direction,
	}, nil
}

func encodeCursor(env cursorEnvelope) string {
	raw, err := json.Marshal(env)
	if err != nil {
		// A struct of strings and ints always marshals; keep the compiler honest.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(raw, resource string) (cursorEnvelope, error) {
	if raw == "" || len(raw) > maxCursorLength {
		return cursorEnvelope{}, ErrInvalidCursor
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursorEnvelope{}, ErrInvalidCursor
	}
	var env cursorEnvelope
	if err := json.Unmarshal(decoded, &env); err != nil {
		return cursorEnvelope{}, ErrInvalidCursor
	}
	if env.Version != cursorVersion || env.Resource != resource {
		return cursorEnvelope{}, ErrInvalidCursor
	}
	return env, nil
}
