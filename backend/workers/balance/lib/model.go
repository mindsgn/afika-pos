package lib

type Wallet struct {
	ID      string
	Address string
	Network string
	Testnet bool
}

type Balance struct {
	WalletID     string
	Address      string
	Network      string
	TokenType    string
	TokenSymbol  string
	TokenAddress string
	Amount       string
	UIAmount     string
	Multiplier   string
	USDAmount    string
	ZARAmount    string
	EURAmount    string
	FetchedAt    int64
}

type StockQuote struct {
	Symbol     string
	Contract   string
	Decimals   int
	Price      string
	Multiplier string
}
