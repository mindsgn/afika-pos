package lib

import "strings"

const stockDecimalsHex = "0x8"

type stockToken struct {
	Symbol   string
	Contract string
}

// baseStocks are the Coinbase B20 stocks tracked on Base mainnet.
// Addresses match stocks.md and workers/balance/lib/stocks.go.
var baseStocks = []stockToken{
	{Symbol: "NVDA", Contract: "0xb20000000000000000000078ee7ce2fE4908108C"},
	{Symbol: "META", Contract: "0xb2000000000000000000008bC8786B856E61707C"},
	{Symbol: "AAPL", Contract: "0xb200000000000000000000C2e324d24d7eEcd1fb"},
	{Symbol: "GOOGL", Contract: "0xb2000000000000000000002D0BA3164cc74f58B7"},
	{Symbol: "AMZN", Contract: "0xb200000000000000000000d9192b6B456483C2E8"},
	{Symbol: "MSFT", Contract: "0xB200000000000000000000Ab99cFa739E253872B"},
	{Symbol: "MSTR", Contract: "0xb2000000000000000000004884b426556b92883d"},
	{Symbol: "SNDK", Contract: "0xb200000000000000000000397293Cb8cda9a10c5"},
	{Symbol: "SPCX", Contract: "0xb2000000000000000000007b9fcbd005511aCBd5"},
	{Symbol: "TSLA", Contract: "0xb2000000000000000000001e800a7f5189430cD0"},
	{Symbol: "AMD", Contract: "0xB2000000000000000000000d8Ce462E99ee7A47B"},
	{Symbol: "ASTS", Contract: "0xB200000000000000000000B1a29cF17A1819288a"},
	{Symbol: "AVGO", Contract: "0xB200000000000000000000Fc737aeA6196aB5a4c"},
	{Symbol: "BE", Contract: "0xb20000000000000000000016f9dfe862feBA122b"},
	{Symbol: "CAKE", Contract: "0xb200000000000000000000f215E4C890CFb7176B"},
	{Symbol: "DJT", Contract: "0xb200000000000000000000428E3a3eebBb20692B"},
	{Symbol: "DUOL", Contract: "0xb200000000000000000000A613D12dEAfBBb1Db7"},
	{Symbol: "GME", Contract: "0xb2000000000000000000007790ed6E48e06eD935"},
	{Symbol: "HIMS", Contract: "0xB20000000000000000000043a599976181Bcf336"},
	{Symbol: "HTZ", Contract: "0xb2000000000000000000002601C5C94F435da168"},
	{Symbol: "LLY", Contract: "0xB200000000000000000000f1a0F91e34892E4718"},
	{Symbol: "MRNA", Contract: "0xB200000000000000000000e215e9B76ecBA02468"},
	{Symbol: "MRVL", Contract: "0xB200000000000000000000eC3c4c7395Cc609813"},
	{Symbol: "NFLX", Contract: "0xb20000000000000000000058B8c947e44011dFE6"},
	{Symbol: "NVAX", Contract: "0xB200000000000000000000C597c476FCf9Aed3a8"},
	{Symbol: "ORCL", Contract: "0xb200000000000000000000347AFbA223D7B6b63C"},
	{Symbol: "PFE", Contract: "0xB20000000000000000000018FE7eC7d6DfeeB528"},
	{Symbol: "PM", Contract: "0xB2000000000000000000008FC2A8C23cf5937b66"},
	{Symbol: "PTON", Contract: "0xB2000000000000000000009272A491812842Aa84"},
	{Symbol: "PYPL", Contract: "0xb200000000000000000000450ad3abE5d4846c6E"},
	{Symbol: "QUBT", Contract: "0xb200000000000000000000CA425ab42e07C35bC3"},
	{Symbol: "RBLX", Contract: "0xB2000000000000000000005bd7AE89b9E6189Bb5"},
	{Symbol: "RDDT", Contract: "0xb20000000000000000000066242d4067724cB7A1"},
	{Symbol: "SOUN", Contract: "0xB2000000000000000000002137743D4a01Fe4e88"},
	{Symbol: "TTWO", Contract: "0xB200000000000000000000f720C26062Bc3067Da"},
	{Symbol: "WEN", Contract: "0xB20000000000000000000044E3CD7a0E1028E57a"},
}

func baseStockContracts() []string {
	out := make([]string, len(baseStocks))
	for i, stock := range baseStocks {
		out[i] = stock.Contract
	}
	return out
}

func lookupStock(addr string) (stockToken, bool) {
	key := strings.ToLower(strings.TrimSpace(addr))
	for _, stock := range baseStocks {
		if strings.ToLower(stock.Contract) == key {
			return stock, true
		}
	}
	return stockToken{}, false
}

func tokenSymbol(addr, asset string) string {
	if stock, ok := lookupStock(addr); ok {
		return stock.Symbol
	}
	return strings.ToUpper(strings.TrimSpace(asset))
}

func decimalsHexForToken(addr, decimalHex string) string {
	if _, ok := lookupStock(addr); ok && ParseBlockNumber(decimalHex) == 0 {
		return stockDecimalsHex
	}
	return decimalHex
}

// MergeTransfers combines transfer batches and drops duplicates.
// Identity is tx hash, direction, and token address.
func MergeTransfers(groups ...[]TransactionItem) []TransactionItem {
	seen := map[string]int{}
	out := make([]TransactionItem, 0)
	for _, group := range groups {
		for _, item := range group {
			key := strings.ToLower(item.TxHash) + "|" + item.Direction + "|" + strings.ToLower(item.TokenAddress)
			if idx, ok := seen[key]; ok {
				if out[idx].TokenSymbol == "" && item.TokenSymbol != "" {
					out[idx].TokenSymbol = item.TokenSymbol
				}
				continue
			}
			seen[key] = len(out)
			out = append(out, item)
		}
	}
	return out
}
