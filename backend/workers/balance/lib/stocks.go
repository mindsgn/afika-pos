package lib

// StockDecimals is the Coinbase B20 token decimals on Base.
const StockDecimals = 8

// BaseStockTokens are the Coinbase B20 stocks tracked on Base mainnet.
// Addresses match stocks.md. Symbol is the ticker used as the balance identity.
var BaseStockTokens = []TokenConfig{
	{Symbol: "NVDA", Contract: "0xb20000000000000000000078ee7ce2fE4908108C", Decimals: StockDecimals},
	{Symbol: "META", Contract: "0xb2000000000000000000008bC8786B856E61707C", Decimals: StockDecimals},
	{Symbol: "AAPL", Contract: "0xb200000000000000000000C2e324d24d7eEcd1fb", Decimals: StockDecimals},
	{Symbol: "GOOGL", Contract: "0xb2000000000000000000002D0BA3164cc74f58B7", Decimals: StockDecimals},
	{Symbol: "AMZN", Contract: "0xb200000000000000000000d9192b6B456483C2E8", Decimals: StockDecimals},
	{Symbol: "MSFT", Contract: "0xB200000000000000000000Ab99cFa739E253872B", Decimals: StockDecimals},
	{Symbol: "MSTR", Contract: "0xb2000000000000000000004884b426556b92883d", Decimals: StockDecimals},
	{Symbol: "SNDK", Contract: "0xb200000000000000000000397293Cb8cda9a10c5", Decimals: StockDecimals},
	{Symbol: "SPCX", Contract: "0xb2000000000000000000007b9fcbd005511aCBd5", Decimals: StockDecimals},
	{Symbol: "TSLA", Contract: "0xb2000000000000000000001e800a7f5189430cD0", Decimals: StockDecimals},
	{Symbol: "AMD", Contract: "0xB2000000000000000000000d8Ce462E99ee7A47B", Decimals: StockDecimals},
	{Symbol: "ASTS", Contract: "0xB200000000000000000000B1a29cF17A1819288a", Decimals: StockDecimals},
	{Symbol: "AVGO", Contract: "0xB200000000000000000000Fc737aeA6196aB5a4c", Decimals: StockDecimals},
	{Symbol: "BE", Contract: "0xb20000000000000000000016f9dfe862feBA122b", Decimals: StockDecimals},
	{Symbol: "CAKE", Contract: "0xb200000000000000000000f215E4C890CFb7176B", Decimals: StockDecimals},
	{Symbol: "DJT", Contract: "0xb200000000000000000000428E3a3eebBb20692B", Decimals: StockDecimals},
	{Symbol: "DUOL", Contract: "0xb200000000000000000000A613D12dEAfBBb1Db7", Decimals: StockDecimals},
	{Symbol: "GME", Contract: "0xb2000000000000000000007790ed6E48e06eD935", Decimals: StockDecimals},
	{Symbol: "HIMS", Contract: "0xB20000000000000000000043a599976181Bcf336", Decimals: StockDecimals},
	{Symbol: "HTZ", Contract: "0xb2000000000000000000002601C5C94F435da168", Decimals: StockDecimals},
	{Symbol: "LLY", Contract: "0xB200000000000000000000f1a0F91e34892E4718", Decimals: StockDecimals},
	{Symbol: "MRNA", Contract: "0xB200000000000000000000e215e9B76ecBA02468", Decimals: StockDecimals},
	{Symbol: "MRVL", Contract: "0xB200000000000000000000eC3c4c7395Cc609813", Decimals: StockDecimals},
	{Symbol: "NFLX", Contract: "0xb20000000000000000000058B8c947e44011dFE6", Decimals: StockDecimals},
	{Symbol: "NVAX", Contract: "0xB200000000000000000000C597c476FCf9Aed3a8", Decimals: StockDecimals},
	{Symbol: "ORCL", Contract: "0xb200000000000000000000347AFbA223D7B6b63C", Decimals: StockDecimals},
	{Symbol: "PFE", Contract: "0xB20000000000000000000018FE7eC7d6DfeeB528", Decimals: StockDecimals},
	{Symbol: "PM", Contract: "0xB2000000000000000000008FC2A8C23cf5937b66", Decimals: StockDecimals},
	{Symbol: "PTON", Contract: "0xB2000000000000000000009272A491812842Aa84", Decimals: StockDecimals},
	{Symbol: "PYPL", Contract: "0xb200000000000000000000450ad3abE5d4846c6E", Decimals: StockDecimals},
	{Symbol: "QUBT", Contract: "0xb200000000000000000000CA425ab42e07C35bC3", Decimals: StockDecimals},
	{Symbol: "RBLX", Contract: "0xB2000000000000000000005bd7AE89b9E6189Bb5", Decimals: StockDecimals},
	{Symbol: "RDDT", Contract: "0xb20000000000000000000066242d4067724cB7A1", Decimals: StockDecimals},
	{Symbol: "SOUN", Contract: "0xB2000000000000000000002137743D4a01Fe4e88", Decimals: StockDecimals},
	{Symbol: "TTWO", Contract: "0xB200000000000000000000f720C26062Bc3067Da", Decimals: StockDecimals},
	{Symbol: "WEN", Contract: "0xB20000000000000000000044E3CD7a0E1028E57a", Decimals: StockDecimals},
}

// EnsureBaseStocks merges the tracked Base stocks into a token map.
// Existing contracts are left in place so a Firestore registry cannot drop them.
func EnsureBaseStocks(tokens map[string][]TokenConfig) map[string][]TokenConfig {
	return MergeBaseMainnetStocks(tokens, BaseStockTokens)
}
