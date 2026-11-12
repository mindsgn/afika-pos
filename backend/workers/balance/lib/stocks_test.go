package lib

import (
	"testing"
)

func TestBaseMainnetIncludesStocks(t *testing.T) {
	base := ERC20Tokens["base-mainnet"]
	bySymbol := map[string]TokenConfig{}
	for _, token := range base {
		bySymbol[token.Symbol] = token
	}
	if _, ok := bySymbol["USDC"]; !ok {
		t.Fatal("USDC missing from base-mainnet")
	}
	if len(BaseStockTokens) != 36 {
		t.Fatalf("stock count %d", len(BaseStockTokens))
	}
	for _, stock := range BaseStockTokens {
		got, ok := bySymbol[stock.Symbol]
		if !ok {
			t.Fatalf("missing %s", stock.Symbol)
		}
		if got.Decimals != StockDecimals {
			t.Fatalf("%s decimals %d", stock.Symbol, got.Decimals)
		}
		if got.Contract != stock.Contract {
			t.Fatalf("%s contract %s", stock.Symbol, got.Contract)
		}
	}
}

func TestEnsureBaseStocksWhenRegistryOmitsThem(t *testing.T) {
	tokens := map[string][]TokenConfig{
		"base-mainnet": {{Symbol: "USDC", Contract: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", Decimals: 6}},
	}
	got := EnsureBaseStocks(tokens)
	if len(got["base-mainnet"]) != 1+len(BaseStockTokens) {
		t.Fatalf("len %d", len(got["base-mainnet"]))
	}
}

func TestMergeBaseMainnetStocks(t *testing.T) {
	tokens := CloneTokenConfigMap(ERC20Tokens)
	nvda := TokenConfig{Symbol: "NVDA", Contract: "0xb20000000000000000000078ee7ce2fe4908108c", Decimals: 8}
	usdcDup := TokenConfig{Symbol: "USDC", Contract: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", Decimals: 6}

	got := MergeBaseMainnetStocks(tokens, []TokenConfig{nvda, usdcDup})
	base := got["base-mainnet"]
	var foundNVDA, usdcCount int
	for _, tkn := range base {
		if tkn.Symbol == "NVDA" {
			foundNVDA++
		}
		if tkn.Symbol == "USDC" {
			usdcCount++
		}
	}
	if foundNVDA != 1 {
		t.Fatalf("nvda count %d", foundNVDA)
	}
	if usdcCount != 1 {
		t.Fatalf("usdc should not duplicate, got %d", usdcCount)
	}
}
