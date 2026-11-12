package lib

// Wallet is a wallet row from D1 together with its transaction cursor.
type Wallet struct {
	ID          string
	Address     string
	Network     string
	Testnet     bool
	LatestBlock uint64
}

// TransactionItem is one wallet transfer enriched with fiat amounts.
type TransactionItem struct {
	WalletAddress string
	TxHash        string
	FromAddress   string
	ToAddress     string
	TokenAddress  string
	TokenType     string
	TokenSymbol   string
	Amount        string
	FeeNative     string
	FeeUSD        string
	FeeZAR        string
	FeeEUR        string
	USDAmount     string
	ZARAmount     string
	EURAmount     string
	Network       string
	Direction     string
	State         string
	BlockNumber   uint64
	TimestampMs   int64
	FetchedAtMs   int64
}

// WorkerResult is the summary returned by a worker run.
type WorkerResult struct {
	Saved  int `json:"saved"`
	Failed int `json:"failed"`
	Total  int `json:"total"`
}
