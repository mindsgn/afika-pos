package lib

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// POSPayment is a shareable checkout for a POS receipt. Amounts are stored in
// ZAR cents; CryptoAmount is the asset amount locked when the payment was
// created or the payer picked an asset.
type POSPayment struct {
	ID              string `json:"id"`
	ReceiptNo       string `json:"receiptNo"`
	AmountCents     int64  `json:"amountCents"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	Asset           string `json:"asset"`
	Network         string `json:"network"`
	Address         string `json:"address"`
	CryptoAmount    string `json:"cryptoAmount"`
	PaymentURL      string `json:"paymentUrl"`
	MerchantName    string `json:"merchantName"`
	MerchantWebsite string `json:"merchantWebsite"`
	MerchantAddress string `json:"merchantAddress"`
	TxHash          string `json:"txHash,omitempty"`
	CreatedAt       int64  `json:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt"`
	ExpiresAt       int64  `json:"expiresAt"`
}

type posAsset struct {
	Asset     string
	Network   string
	Decimals  int
	ZARPerOne float64
}

// posAssets is the single source of truth for the options the checkout page
// offers. Keep it in sync with src/domain/payment.ts in the app.
var posAssets = map[string]posAsset{
	"btc":           {Asset: "BTC", Network: "bitcoin", Decimals: 8, ZARPerOne: 1_200_000},
	"usdc-base":     {Asset: "USDC", Network: "base", Decimals: 2, ZARPerOne: 18.5},
	"usdc-ethereum": {Asset: "USDC", Network: "ethereum", Decimals: 6, ZARPerOne: 18.5},
	"zarp-base":     {Asset: "ZARP", Network: "base", Decimals: 2, ZARPerOne: 1},
	"zarp-ethereum": {Asset: "ZARP", Network: "ethereum", Decimals: 6, ZARPerOne: 1},
	"eurc-base":     {Asset: "EURC", Network: "base", Decimals: 2, ZARPerOne: 20.1},
	"eurc-ethereum": {Asset: "EURC", Network: "ethereum", Decimals: 6, ZARPerOne: 20.1},
}

const (
	posStatusPending = "pending"
	posStatusPaid    = "paid"
	posStatusExpired = "expired"
	posStatusFailed  = "failed"

	posPaymentTTL = 30 * time.Minute
)

func normalizePOSAsset(id string) (string, posAsset, error) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		key = "usdc-base"
	}
	asset, ok := posAssets[key]
	if !ok {
		return "", posAsset{}, fmt.Errorf("asset must be one of btc, usdc-base, usdc-ethereum, zarp-base, zarp-ethereum, eurc-base, eurc-ethereum")
	}
	return key, asset, nil
}

func normalizePOSStatus(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case posStatusPending:
		return posStatusPending, nil
	case posStatusPaid:
		return posStatusPaid, nil
	case posStatusExpired:
		return posStatusExpired, nil
	case posStatusFailed:
		return posStatusFailed, nil
	default:
		return "", fmt.Errorf("status must be pending, paid, expired, or failed")
	}
}

// cryptoAmountFor converts a ZAR-cent amount into the asset amount at the
// table rate, formatted to the asset's display precision.
func cryptoAmountFor(amountCents int64, asset posAsset) string {
	zar := float64(amountCents) / 100
	units := zar / asset.ZARPerOne
	return strconv.FormatFloat(units, 'f', asset.Decimals, 64)
}

func newPOSPaymentID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// ---------------------------------------------------------------------------
// Store interface additions (implemented on D1Store below).
// ---------------------------------------------------------------------------

// CreatePOSPayment inserts a new pending payment.
func (s *D1Store) CreatePOSPayment(ctx context.Context, p POSPayment) (POSPayment, error) {
	if strings.TrimSpace(p.ID) == "" {
		return POSPayment{}, errors.New("pos payment id is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pos_payments (id, receipt_no, amount_cents, currency, status, asset,
			network, address, crypto_amount, payment_url, merchant_name, merchant_website,
			merchant_address, tx_hash, created_at, updated_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ReceiptNo, p.AmountCents, p.Currency, p.Status, p.Asset, p.Network,
		p.Address, p.CryptoAmount, p.PaymentURL, p.MerchantName, p.MerchantWebsite,
		p.MerchantAddress, p.TxHash, p.CreatedAt, p.UpdatedAt, p.ExpiresAt,
	)
	if err != nil {
		return POSPayment{}, fmt.Errorf("insert pos payment: %w", err)
	}
	return p, nil
}

// GetPOSPayment reads a payment by id.
func (s *D1Store) GetPOSPayment(ctx context.Context, id string) (POSPayment, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return POSPayment{}, errors.New("pos payment id is required")
	}

	var (
		p      POSPayment
		txHash sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, receipt_no, amount_cents, currency, status, asset, network,
		        address, crypto_amount, payment_url, merchant_name, merchant_website,
		        merchant_address, tx_hash, created_at, updated_at, expires_at
		 FROM pos_payments WHERE id = ?`, id,
	).Scan(
		&p.ID, &p.ReceiptNo, &p.AmountCents, &p.Currency, &p.Status, &p.Asset,
		&p.Network, &p.Address, &p.CryptoAmount, &p.PaymentURL, &p.MerchantName,
		&p.MerchantWebsite, &p.MerchantAddress, &txHash, &p.CreatedAt, &p.UpdatedAt,
		&p.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return POSPayment{}, ErrPOSPaymentNotFound
	}
	if err != nil {
		return POSPayment{}, fmt.Errorf("get pos payment: %w", err)
	}
	p.TxHash = txHash.String
	return p, nil
}

// ErrPOSPaymentNotFound is returned when a payment id does not exist.
var ErrPOSPaymentNotFound = errors.New("pos payment not found")

// UpdatePOSPayment writes the mutable fields (status, asset, network, address,
// crypto amount, tx hash, timestamps).
func (s *D1Store) UpdatePOSPayment(ctx context.Context, p POSPayment) (POSPayment, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE pos_payments
		 SET status = ?, asset = ?, network = ?, address = ?, crypto_amount = ?,
		     tx_hash = ?, updated_at = ?
		 WHERE id = ?`,
		p.Status, p.Asset, p.Network, p.Address, p.CryptoAmount, nullString(p.TxHash),
		p.UpdatedAt, p.ID,
	)
	if err != nil {
		return POSPayment{}, fmt.Errorf("update pos payment: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return POSPayment{}, ErrPOSPaymentNotFound
	}
	return s.GetPOSPayment(ctx, p.ID)
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

// ---------------------------------------------------------------------------
// HTTP handlers.
// ---------------------------------------------------------------------------

type createPOSPaymentRequest struct {
	ReceiptNo      string `json:"receiptNo"`
	AmountCents    int64  `json:"amountCents"`
	Currency       string `json:"currency"`
	Asset          string `json:"asset"`
	ReceiveAddress string `json:"receiveAddress"`
	Merchant       struct {
		Name    string `json:"name"`
		Website string `json:"website"`
		Address string `json:"address"`
	} `json:"merchant"`
}

type confirmPOSPaymentRequest struct {
	TxHash string `json:"txHash"`
}

type selectPOSAssetRequest struct {
	Asset string `json:"asset"`
}

type posPaymentEnvelope struct {
	Data POSPayment `json:"data"`
}

func (h *Handler) createPOSPayment(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	if authErr := h.authenticate(r); authErr != nil {
		return 0, nil, authErr
	}

	var request createPOSPaymentRequest
	if apiErr := decodeJSONBody(w, r, &request); apiErr != nil {
		return 0, nil, apiErr
	}

	receiptNo := strings.TrimSpace(request.ReceiptNo)
	if receiptNo == "" || len(receiptNo) > 64 {
		return 0, nil, validation("receiptNo is required and must be at most 64 characters.")
	}
	if request.AmountCents <= 0 {
		return 0, nil, validation("amountCents must be greater than zero.")
	}
	assetID, asset, err := normalizePOSAsset(request.Asset)
	if err != nil {
		return 0, nil, validation(err.Error())
	}
	currency := strings.ToUpper(strings.TrimSpace(request.Currency))
	if currency == "" {
		currency = "ZAR"
	}
	if currency != "ZAR" {
		return 0, nil, validation("currency must be ZAR.")
	}
	address := strings.TrimSpace(request.ReceiveAddress)
	if address == "" {
		address = strings.TrimSpace(h.receivingAddress)
	}

	id, err := newPOSPaymentID()
	if err != nil {
		h.logf("create pos payment id: %v", err)
		return 0, nil, internalError()
	}

	now := time.Now()
	payment := POSPayment{
		ID:              id,
		ReceiptNo:       receiptNo,
		AmountCents:     request.AmountCents,
		Currency:        currency,
		Status:          posStatusPending,
		Asset:           assetID,
		Network:         asset.Network,
		Address:         address,
		CryptoAmount:    cryptoAmountFor(request.AmountCents, asset),
		PaymentURL:      h.publicBaseURL(r) + "/pay/" + id,
		MerchantName:    strings.TrimSpace(request.Merchant.Name),
		MerchantWebsite: strings.TrimSpace(request.Merchant.Website),
		MerchantAddress: strings.TrimSpace(request.Merchant.Address),
		CreatedAt:       now.UnixMilli(),
		UpdatedAt:       now.UnixMilli(),
		ExpiresAt:       now.Add(posPaymentTTL).UnixMilli(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	saved, err := h.store.CreatePOSPayment(ctx, payment)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, requestTimeout()
		}
		h.logf("create pos payment: %v", err)
		return 0, nil, internalError()
	}
	return http.StatusCreated, posPaymentEnvelope{Data: saved}, nil
}

func (h *Handler) getPOSPayment(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	id, apiErr := parsePOSPaymentID(r.URL.Path)
	if apiErr != nil {
		return 0, nil, apiErr
	}
	payment, apiErr := h.loadPOSPayment(r, id)
	if apiErr != nil {
		return 0, nil, apiErr
	}
	return http.StatusOK, posPaymentEnvelope{Data: payment}, nil
}

func (h *Handler) selectPOSAsset(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	id, apiErr := parsePOSPaymentID(strings.TrimSuffix(r.URL.Path, "/asset"))
	if apiErr != nil {
		return 0, nil, apiErr
	}
	var request selectPOSAssetRequest
	if apiErr := decodeJSONBody(w, r, &request); apiErr != nil {
		return 0, nil, apiErr
	}
	assetID, asset, err := normalizePOSAsset(request.Asset)
	if err != nil {
		return 0, nil, validation(err.Error())
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	payment, err := h.store.GetPOSPayment(ctx, id)
	if errors.Is(err, ErrPOSPaymentNotFound) {
		return 0, nil, notFound()
	}
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, requestTimeout()
		}
		h.logf("select pos asset: %v", err)
		return 0, nil, internalError()
	}
	if payment.Status != posStatusPending {
		return 0, nil, validation("This payment is no longer pending.")
	}

	payment.Asset = assetID
	payment.Network = asset.Network
	payment.CryptoAmount = cryptoAmountFor(payment.AmountCents, asset)
	payment.UpdatedAt = time.Now().UnixMilli()
	updated, err := h.store.UpdatePOSPayment(ctx, payment)
	if err != nil {
		h.logf("select pos asset update: %v", err)
		return 0, nil, internalError()
	}
	return http.StatusOK, posPaymentEnvelope{Data: updated}, nil
}

func (h *Handler) confirmPOSPayment(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	if authErr := h.authenticate(r); authErr != nil {
		return 0, nil, authErr
	}
	id, apiErr := parsePOSPaymentID(strings.TrimSuffix(r.URL.Path, "/confirm"))
	if apiErr != nil {
		return 0, nil, apiErr
	}
	var request confirmPOSPaymentRequest
	if apiErr := decodeJSONBody(w, r, &request); apiErr != nil {
		return 0, nil, apiErr
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	payment, err := h.store.GetPOSPayment(ctx, id)
	if errors.Is(err, ErrPOSPaymentNotFound) {
		return 0, nil, notFound()
	}
	if err != nil {
		h.logf("confirm pos payment: %v", err)
		return 0, nil, internalError()
	}
	if payment.Status == posStatusPaid {
		return http.StatusOK, posPaymentEnvelope{Data: payment}, nil
	}
	if payment.Status != posStatusPending {
		return 0, nil, validation("Only pending payments can be confirmed.")
	}

	payment.Status = posStatusPaid
	payment.TxHash = strings.TrimSpace(request.TxHash)
	payment.UpdatedAt = time.Now().UnixMilli()
	updated, err := h.store.UpdatePOSPayment(ctx, payment)
	if err != nil {
		h.logf("confirm pos payment update: %v", err)
		return 0, nil, internalError()
	}
	return http.StatusOK, posPaymentEnvelope{Data: updated}, nil
}

func (h *Handler) loadPOSPayment(r *http.Request, id string) (POSPayment, *apiError) {
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	payment, err := h.store.GetPOSPayment(ctx, id)
	if errors.Is(err, ErrPOSPaymentNotFound) {
		return POSPayment{}, notFound()
	}
	if err != nil {
		if ctx.Err() != nil {
			return POSPayment{}, requestTimeout()
		}
		h.logf("get pos payment: %v", err)
		return POSPayment{}, internalError()
	}
	// Expiry is enforced here so a pending payment cannot be paid after the
	// quoted rate has gone stale, even before the receipt worker runs.
	if payment.Status == posStatusPending && payment.ExpiresAt > 0 && time.Now().UnixMilli() > payment.ExpiresAt {
		payment.Status = posStatusExpired
	}
	return payment, nil
}

func parsePOSPaymentID(path string) (string, *apiError) {
	id := strings.Trim(strings.TrimPrefix(path, "/v1/pos/payments/"), "/")
	if id == "" || strings.Contains(id, "/") || len(id) > 64 {
		return "", notFound()
	}
	return id, nil
}

func notFound() *apiError {
	return &apiError{status: http.StatusNotFound, code: "NOT_FOUND", message: "Route not found."}
}

// publicBaseURL builds the origin used for shareable links, honouring the
// proxy headers set by Cloudflare.
func (h *Handler) publicBaseURL(r *http.Request) string {
	if h.checkoutBaseURL != "" {
		return h.checkoutBaseURL
	}
	scheme := "https"
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto == "http" || proto == "https" {
		scheme = proto
	} else if r.TLS == nil && (strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1")) {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}
