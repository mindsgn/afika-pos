package lib

import (
	"testing"
	"time"
)

func TestParseBlockNumber(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want uint64
	}{
		{name: "hex", in: "0x10", want: 16},
		{name: "hex without prefix", in: "ff", want: 255},
		{name: "zero", in: "0x0", want: 0},
		{name: "empty", in: "", want: 0},
		{name: "blank", in: "   ", want: 0},
		{name: "invalid", in: "0xzz", want: 0},
		{name: "decimal is hex", in: "10", want: 16},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseBlockNumber(tc.in); got != tc.want {
				t.Fatalf("ParseBlockNumber(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseBlockTimestamp(t *testing.T) {
	got := ParseBlockTimestamp("2024-01-02T03:04:05Z")
	want := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC).UnixMilli()
	if got != want {
		t.Fatalf("ParseBlockTimestamp = %d, want %d", got, want)
	}

	before := time.Now().UnixMilli()
	for _, in := range []string{"", "   ", "not-a-time"} {
		got := ParseBlockTimestamp(in)
		if got < before {
			t.Fatalf("ParseBlockTimestamp(%q) = %d, want a current timestamp >= %d", in, got, before)
		}
	}
}

func TestParseTransferAmount(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		rawHex   string
		decimals string
		want     string
	}{
		{name: "float value", value: float64(1.5), want: "1.50000000"},
		{name: "float zero", value: float64(0), want: "0.00000000"},
		{name: "string value", value: "2.5", want: "2.5"},
		{name: "blank string falls through", value: "  ", rawHex: "0x5f5e100", decimals: "0x8", want: "1.00000000"},
		{name: "hex with decimals", value: nil, rawHex: "0x5f5e100", decimals: "0x8", want: "1.00000000"},
		{name: "hex without decimals", value: nil, rawHex: "0x5f5e100", decimals: "", want: "100000000"},
		{name: "hex with zero decimals", value: nil, rawHex: "0xff", decimals: "0x0", want: "255"},
		{name: "missing raw", value: nil, want: "0"},
		{name: "invalid raw", value: nil, rawHex: "0xzz", decimals: "0x8", want: "0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseTransferAmount(tc.value, tc.rawHex, tc.decimals); got != tc.want {
				t.Fatalf("ParseTransferAmount = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTokenTypeFromAddress(t *testing.T) {
	if got := TokenTypeFromAddress(""); got != "native" {
		t.Fatalf("empty token type = %q, want native", got)
	}
	if got := TokenTypeFromAddress("   "); got != "native" {
		t.Fatalf("blank token type = %q, want native", got)
	}
	if got := TokenTypeFromAddress("0xabc"); got != "erc20" {
		t.Fatalf("token type = %q, want erc20", got)
	}
}

func TestWalletKey(t *testing.T) {
	tests := []struct {
		name string
		in   Wallet
		want string
	}{
		{name: "address", in: Wallet{ID: "id-1", Address: " 0xABC "}, want: "0xabc"},
		{name: "falls back to id", in: Wallet{ID: " ID-1 ", Address: "  "}, want: "id-1"},
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
