package lib

// Wallet is the API view of a row in the shared `wallets` table. The row id is
// the lowercased wallet address.
type Wallet struct {
	Address   string `json:"address"`
	Network   string `json:"network"`
	Testnet   bool   `json:"testnet"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// WalletInput is a validated wallet upsert request.
type WalletInput struct {
	Address string
	Network string
	Testnet bool
}

// WalletSettings is the per-wallet preference row. PreferredCurrency is one of
// USD, ZAR, or EUR and drives the default display currency for balances and
// transactions.
type WalletSettings struct {
	Address           string `json:"address"`
	PreferredCurrency string `json:"preferredCurrency"`
	CreatedAt         int64  `json:"createdAt"`
	UpdatedAt         int64  `json:"updatedAt"`
}

// WalletSettingsInput is a validated settings upsert request.
type WalletSettingsInput struct {
	Address           string
	PreferredCurrency string
}

// DisplayCurrency is the currency selected for a balances or transactions
// response. Every item carries DisplayAmount in this currency.
type DisplayCurrency struct {
	Currency string `json:"currency"`
}

// FXRates contains the latest USD conversion rates published by workers/forex.
type FXRates struct {
	USD       string `json:"USD"`
	ZAR       string `json:"ZAR"`
	EUR       string `json:"EUR"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Balance mirrors a row written by workers/balance. Amounts stay strings so
// decimal precision is never lost in JSON.
type Balance struct {
	WalletAddress string `json:"walletAddress"`
	WalletID      string `json:"walletId"`
	Network       string `json:"network"`
	TokenType     string `json:"tokenType"`
	TokenSymbol   string `json:"tokenSymbol"`
	TokenAddress  string `json:"tokenAddress"`
	Amount        string `json:"amount"`
	UIAmount      string `json:"uiAmount"`
	Multiplier    string `json:"multiplier"`
	USDAmount     string `json:"usdAmount"`
	ZARAmount     string `json:"zarAmount"`
	EURAmount     string `json:"eurAmount"`
	// DisplayCurrency/DisplayAmount are resolved from the `currency` query
	// parameter or the wallet's saved preference.
	DisplayCurrency string `json:"displayCurrency,omitempty"`
	DisplayAmount   string `json:"displayAmount,omitempty"`
	FetchedAt       int64  `json:"fetchedAt"`
}

// Transaction mirrors a row written by workers/transaction. Timestamp and
// FetchedAt duplicate the millisecond fields because the Expo app reads both
// names from the old Firestore documents.
type Transaction struct {
	WalletAddress string `json:"walletAddress"`
	TxHash        string `json:"txHash"`
	FromAddress   string `json:"fromAddress"`
	ToAddress     string `json:"toAddress"`
	TokenAddress  string `json:"tokenAddress"`
	TokenType     string `json:"tokenType"`
	TokenSymbol   string `json:"tokenSymbol"`
	Amount        string `json:"amount"`
	FeeNative     string `json:"feeNative"`
	FeeUSD        string `json:"feeUsd"`
	FeeZAR        string `json:"feeZar"`
	FeeEUR        string `json:"feeEur"`
	USDAmount     string `json:"usdAmount"`
	ZARAmount     string `json:"zarAmount"`
	EURAmount     string `json:"eurAmount"`
	// DisplayCurrency/DisplayAmount/DisplayFee are resolved from the `currency`
	// query parameter or the wallet's saved preference.
	DisplayCurrency string `json:"displayCurrency,omitempty"`
	DisplayAmount   string `json:"displayAmount,omitempty"`
	DisplayFee      string `json:"displayFee,omitempty"`
	Network         string `json:"network"`
	Direction       string `json:"direction"`
	State           string `json:"state"`
	BlockNumber     uint64 `json:"blockNumber"`
	TimestampMs     int64  `json:"timestampMs"`
	Timestamp       int64  `json:"timestamp"`
	FetchedAtMs     int64  `json:"fetchedAtMs"`
	FetchedAt       int64  `json:"fetchedAt"`
}

// BalancePage is one keyset page of balances.
type BalancePage struct {
	Items   []Balance
	HasMore bool
}

// TransactionPage is one keyset page of transactions.
type TransactionPage struct {
	Items   []Transaction
	HasMore bool
}
