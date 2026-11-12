package lib

import "testing"

func TestAssetSymbol(t *testing.T) {
	tests := map[string]string{
		"usdc-base":     "USDC",
		"USDC-ETHEREUM": "USDC",
		" zarp-base ":   "ZARP",
		"eurc-ethereum": "EURC",
		"btc":           "BTC",
		"doge":          "",
	}
	for asset, want := range tests {
		if got := AssetSymbol(asset); got != want {
			t.Fatalf("AssetSymbol(%q) = %q, want %q", asset, got, want)
		}
	}
}

func TestNetworkAliases(t *testing.T) {
	tests := []struct {
		network string
		want    []string
	}{
		{"base", []string{"base-mainnet", "base"}},
		{"ethereum", []string{"eth-mainnet", "ethereum", "mainnet"}},
		{"bitcoin", []string{"bitcoin", "btc-mainnet"}},
		{"solana", nil},
	}
	for _, tc := range tests {
		got := NetworkAliases(tc.network)
		if len(got) != len(tc.want) {
			t.Fatalf("NetworkAliases(%q) = %v, want %v", tc.network, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("NetworkAliases(%q) = %v, want %v", tc.network, got, tc.want)
			}
		}
	}

	// Returned slices are copies: mutating one must not corrupt the table.
	first := NetworkAliases("base")
	first[0] = "mutated"
	if NetworkAliases("base")[0] != "base-mainnet" {
		t.Fatal("NetworkAliases returned the shared slice")
	}
}

func TestDecimalAtLeast(t *testing.T) {
	tests := []struct {
		have, want string
		ok         bool
	}{
		{"6.67", "6.67", true},
		{"6.670000", "6.67", true},
		{"6.66", "6.67", false},
		{"0.00010288", "0.00010288", true},
		{"0.00010287", "0.00010288", false},
		{"180.000000000000000000", "180", true},
		{"", "1", false},
		{"1", "", false},
		{"abc", "1", false},
	}
	for _, tc := range tests {
		if got := DecimalAtLeast(tc.have, tc.want); got != tc.ok {
			t.Fatalf("DecimalAtLeast(%q, %q) = %v, want %v", tc.have, tc.want, got, tc.ok)
		}
	}
}

func TestMatchCredit(t *testing.T) {
	credits := []Credit{
		{TxHash: "0xold", Amount: "1.00"},
		{TxHash: "0xexact", Amount: "6.67"},
		{TxHash: "0xextra", Amount: "10"},
	}

	credit, ok := MatchCredit("6.67", credits)
	if !ok || credit.TxHash != "0xexact" {
		t.Fatalf("match = %+v ok=%v, want 0xexact", credit, ok)
	}

	credit, ok = MatchCredit("0.50", []Credit{{TxHash: "0xover", Amount: "0.5001"}})
	if !ok || credit.TxHash != "0xover" {
		t.Fatalf("overpay match = %+v ok=%v", credit, ok)
	}

	if _, ok := MatchCredit("100", credits); ok {
		t.Fatal("matched an insufficient credit")
	}
	if _, ok := MatchCredit("1", nil); ok {
		t.Fatal("matched with no credits")
	}
}
