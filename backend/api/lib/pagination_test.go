package lib

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestBalanceCursorRoundTrip(t *testing.T) {
	in := BalanceCursor{Network: "base-mainnet", TokenSymbol: "USDC"}

	encoded := EncodeBalanceCursor(in)
	if encoded == "" {
		t.Fatal("EncodeBalanceCursor returned an empty cursor")
	}
	if strings.ContainsAny(encoded, "+/=") {
		t.Fatalf("cursor %q is not URL safe", encoded)
	}

	out, err := DecodeBalanceCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeBalanceCursor: %v", err)
	}
	if out != in {
		t.Fatalf("round trip = %+v, want %+v", out, in)
	}
}

func TestTransactionCursorRoundTrip(t *testing.T) {
	in := TransactionCursor{TimestampMs: 1730000000000, TxHash: "0xabc", Direction: "credit"}

	encoded := EncodeTransactionCursor(in)
	out, err := DecodeTransactionCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeTransactionCursor: %v", err)
	}
	if out != in {
		t.Fatalf("round trip = %+v, want %+v", out, in)
	}
}

func TestDecodeCursorRejectsInvalidInput(t *testing.T) {
	validBalance := EncodeBalanceCursor(BalanceCursor{Network: "base-mainnet", TokenSymbol: "USDC"})
	validTx := EncodeTransactionCursor(TransactionCursor{TimestampMs: 1, TxHash: "0xabc", Direction: "credit"})

	encode := func(raw string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(raw))
	}

	tests := []struct {
		name string
		raw  string
		dec  func(string) error
	}{
		{"empty", "", func(s string) error { _, err := DecodeBalanceCursor(s); return err }},
		{"not base64", "!!!!", func(s string) error { _, err := DecodeBalanceCursor(s); return err }},
		{"not json", encode("not json"), func(s string) error { _, err := DecodeBalanceCursor(s); return err }},
		{"wrong resource", validTx, func(s string) error { _, err := DecodeBalanceCursor(s); return err }},
		{"wrong version", encode(`{"v":2,"r":"balances","network":"base-mainnet","tokenSymbol":"USDC"}`), func(s string) error { _, err := DecodeBalanceCursor(s); return err }},
		{"missing network", encode(`{"v":1,"r":"balances","tokenSymbol":"USDC"}`), func(s string) error { _, err := DecodeBalanceCursor(s); return err }},
		{"missing symbol", encode(`{"v":1,"r":"balances","network":"base-mainnet"}`), func(s string) error { _, err := DecodeBalanceCursor(s); return err }},
		{"transaction cursor for balances", validBalance, func(s string) error { _, err := DecodeTransactionCursor(s); return err }},
		{"missing tx hash", encode(`{"v":1,"r":"transactions","timestampMs":1,"direction":"credit"}`), func(s string) error { _, err := DecodeTransactionCursor(s); return err }},
		{"missing direction", encode(`{"v":1,"r":"transactions","timestampMs":1,"txHash":"0xabc"}`), func(s string) error { _, err := DecodeTransactionCursor(s); return err }},
		{"oversized", strings.Repeat("a", maxCursorLength+1), func(s string) error { _, err := DecodeBalanceCursor(s); return err }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.dec(tc.raw)
			if !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("error = %v, want ErrInvalidCursor", err)
			}
		})
	}
}
