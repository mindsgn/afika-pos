package lib

// Payment is the subset of a pos_payments row the receipt worker needs.
type Payment struct {
	ID           string
	ReceiptNo    string
	AmountCents  int64
	Asset        string // option id, e.g. "usdc-base"
	Network      string // base, ethereum, bitcoin
	Address      string
	CryptoAmount string // exact asset amount quoted to the payer
	CreatedAt    int64
	ExpiresAt    int64
}

// Credit is one incoming on-chain transfer recorded by the transaction worker.
type Credit struct {
	TxHash      string
	TokenSymbol string
	Amount      string
	Network     string
	TimestampMs int64
}

// CreditQuery selects candidate credits for one payment address.
type CreditQuery struct {
	Address  string
	Networks []string
	Symbol   string
	SinceMs  int64
	Limit    int
}
