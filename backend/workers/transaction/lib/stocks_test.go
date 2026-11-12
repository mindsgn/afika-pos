package lib

import "testing"

func TestStockListMatchesTrackedSet(t *testing.T) {
	if len(baseStocks) != 36 {
		t.Fatalf("stock count %d", len(baseStocks))
	}
	contracts := baseStockContracts()
	if len(contracts) != len(baseStocks) {
		t.Fatalf("contract count %d", len(contracts))
	}
	nvda, ok := lookupStock("0xB20000000000000000000078EE7CE2FE4908108C")
	if !ok || nvda.Symbol != "NVDA" {
		t.Fatalf("nvda lookup %+v ok=%v", nvda, ok)
	}
	if _, ok := lookupStock("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"); ok {
		t.Fatal("USDC should not be a stock")
	}
}

func TestTokenSymbolAndDecimals(t *testing.T) {
	addr := "0xb20000000000000000000078ee7ce2fE4908108C"
	if got := tokenSymbol(addr, "NVIDIA"); got != "NVDA" {
		t.Fatalf("symbol %s", got)
	}
	if got := tokenSymbol("0xabc", "usdc"); got != "USDC" {
		t.Fatalf("symbol %s", got)
	}
	if got := decimalsHexForToken(addr, ""); got != stockDecimalsHex {
		t.Fatalf("decimals %s", got)
	}
	if got := decimalsHexForToken(addr, "0x8"); got != "0x8" {
		t.Fatalf("decimals %s", got)
	}
	if got := decimalsHexForToken("0xabc", ""); got != "" {
		t.Fatalf("decimals %s", got)
	}
	amount := ParseTransferAmount(nil, "0x5f5e100", decimalsHexForToken(addr, ""))
	if amount != "1.00000000" {
		t.Fatalf("amount %s", amount)
	}
}

func TestAssetTransferFilterIncludesStocks(t *testing.T) {
	filter := assetTransferFilter("0xabc", "credit", 10, baseStockContracts())
	contracts, ok := filter["contractAddresses"].([]string)
	if !ok || len(contracts) != 36 {
		t.Fatalf("contracts %#v", filter["contractAddresses"])
	}
	category, ok := filter["category"].([]string)
	if !ok || len(category) != 1 || category[0] != "erc20" {
		t.Fatalf("category %#v", filter["category"])
	}
	if filter["toAddress"] != "0xabc" {
		t.Fatalf("toAddress %#v", filter["toAddress"])
	}
	if filter["fromBlock"] != "0xa" {
		t.Fatalf("fromBlock %#v", filter["fromBlock"])
	}

	open := assetTransferFilter("0xabc", "debit", 0, nil)
	if _, ok := open["contractAddresses"]; ok {
		t.Fatal("unfiltered query should not set contractAddresses")
	}
	if open["fromAddress"] != "0xabc" {
		t.Fatalf("fromAddress %#v", open["fromAddress"])
	}
	if open["fromBlock"] != "0x0" {
		t.Fatalf("fromBlock %#v", open["fromBlock"])
	}
}

func TestMergeTransfersDedupesStockQuery(t *testing.T) {
	nvda := "0xb20000000000000000000078ee7ce2fe4908108c"
	general := []TransactionItem{{
		TxHash: "0x1", Direction: "credit", TokenAddress: nvda, TokenSymbol: "",
	}}
	stocks := []TransactionItem{{
		TxHash: "0x1", Direction: "credit", TokenAddress: nvda, TokenSymbol: "NVDA",
	}, {
		TxHash: "0x2", Direction: "debit", TokenAddress: nvda, TokenSymbol: "NVDA",
	}}
	got := MergeTransfers(general, stocks)
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].TokenSymbol != "NVDA" {
		t.Fatalf("symbol %s", got[0].TokenSymbol)
	}
}

func TestMergeTransfersKeepsSameHashDifferentDirection(t *testing.T) {
	got := MergeTransfers(
		[]TransactionItem{{TxHash: "0xA", Direction: "credit", TokenAddress: "0x1"}},
		[]TransactionItem{{TxHash: "0xa", Direction: "debit", TokenAddress: "0x1"}},
	)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}
