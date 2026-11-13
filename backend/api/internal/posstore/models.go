// Package posstore is the SQLite/D1 persistence layer for the Afika POS.
//
// It is deliberately transport-agnostic: reads use database/sql, and every
// multi-row write goes through a small batch abstraction that runs as one
// transaction on native SQLite and as one D1Database.batch() call on Cloudflare
// Workers.
package posstore

// Business is the merchant profile. There is at most one row per owner_id; the
// app's onboarding screen writes it on first launch.
type Business struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Website       string `json:"website"`
	Address       string `json:"address"`
	WalletAddress string `json:"walletAddress"`
	LogoURI       string `json:"logoUri,omitempty"`
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
}

// Category groups products.
type Category struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

// Product is a sellable item. Stock is only ever changed through stock
// movements, so the movement log can always explain the current balance.
type Product struct {
	ID           string `json:"id"`
	SKU          string `json:"sku"`
	Name         string `json:"name"`
	ImageURI     string `json:"imageUri,omitempty"`
	CategoryID   string `json:"categoryId,omitempty"`
	CategoryName string `json:"categoryName,omitempty"`
	Price        int64  `json:"price"`
	CostPrice    int64  `json:"costPrice"`
	Stock        int64  `json:"stock"`
	ReorderLevel int64  `json:"reorderLevel"`
	Active       bool   `json:"active"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

// Customer is a loyalty member. Points are integer cents-worth values: one
// point is worth 10 cents when redeemed.
type Customer struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Phone         string `json:"phone,omitempty"`
	LoyaltyPoints int64  `json:"loyaltyPoints"`
	CreatedAt     int64  `json:"createdAt"`
}

// Sale is a single receipt. CryptoAmount/CryptoRate are decimal strings.
type Sale struct {
	ID             string `json:"id"`
	ReceiptNo      string `json:"receiptNo"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
	Status         string `json:"status"`
	CustomerID     string `json:"customerId,omitempty"`
	Subtotal       int64  `json:"subtotal"`
	DiscountType   string `json:"discountType"`
	DiscountValue  int64  `json:"discountValue"`
	DiscountAmount int64  `json:"discountAmount"`
	PointsRedeemed int64  `json:"pointsRedeemed"`
	PointsDiscount int64  `json:"pointsDiscount"`
	Total          int64  `json:"total"`
	CostTotal      int64  `json:"costTotal"`
	PaymentMethod  string `json:"paymentMethod"`
	PaymentStatus  string `json:"paymentStatus"`
	CryptoAmount   string `json:"cryptoAmount"`
	CryptoRate     string `json:"cryptoRate"`
	TxHash         string `json:"txHash,omitempty"`
	FailureReason  string `json:"failureReason,omitempty"`
	PointsEarned   int64  `json:"pointsEarned"`
	RefundedTotal  int64  `json:"refundedTotal"`
	VoidedAt       int64  `json:"voidedAt,omitempty"`
	VoidReason     string `json:"voidReason,omitempty"`
}

// SaleItem is one line of a sale. Name, SKU, price, and cost are snapshots.
type SaleItem struct {
	ID             string `json:"id"`
	SaleID         string `json:"saleId"`
	ProductID      string `json:"productId"`
	SKU            string `json:"sku"`
	Name           string `json:"name"`
	Qty            int64  `json:"qty"`
	UnitPrice      int64  `json:"unitPrice"`
	UnitCost       int64  `json:"unitCost"`
	LineDiscount   int64  `json:"lineDiscount"`
	RefundedQty    int64  `json:"refundedQty"`
	RefundedAmount int64  `json:"refundedAmount"`
}

// Refund is a full or partial return against a sale.
type Refund struct {
	ID             string       `json:"id"`
	SaleID         string       `json:"saleId"`
	CreatedAt      int64        `json:"createdAt"`
	Amount         int64        `json:"amount"`
	Reason         string       `json:"reason,omitempty"`
	Restocked      bool         `json:"restocked"`
	PointsReversed int64        `json:"pointsReversed"`
	Items          []RefundLine `json:"items"`
}

// RefundLine is one line of a refund, joined with the sale item it returns.
type RefundLine struct {
	SaleItemID    string `json:"saleItemId"`
	Name          string `json:"name"`
	Qty           int64  `json:"qty"`
	Amount        int64  `json:"amount"`
	CostRecovered int64  `json:"costRecovered"`
}

// StockMovement is an immutable stock ledger entry.
type StockMovement struct {
	ID           string `json:"id"`
	ProductID    string `json:"productId"`
	ProductName  string `json:"productName,omitempty"`
	SKU          string `json:"sku,omitempty"`
	Type         string `json:"type"`
	Quantity     int64  `json:"quantity"`
	BalanceAfter int64  `json:"balanceAfter"`
	UnitCost     int64  `json:"unitCost,omitempty"`
	HasUnitCost  bool   `json:"-"`
	Reference    string `json:"reference,omitempty"`
	Note         string `json:"note,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
}
