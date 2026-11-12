package lib

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func decodePOSPayment(t *testing.T, body []byte) POSPayment {
	t.Helper()
	var env posPaymentEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode payment body %q: %v", body, err)
	}
	return env.Data
}

func posCreateBody(asset string) string {
	return `{"receiptNo":"POS-0001","amountCents":12345,"currency":"ZAR","asset":"` + asset + `","merchant":{"name":"Kofi Coffee","website":"https://kofi.example","address":"12 Long St, Cape Town"}}`
}

func TestCreatePOSPaymentRequiresAuthentication(t *testing.T) {
	store, _, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPost, "/v1/pos/payments", posCreateBody("usdc-base"), nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.posCreated) != 0 {
		t.Fatal("payment stored without authentication")
	}
}

func TestCreatePOSPayment(t *testing.T) {
	store, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
		cfg.ReceivingAddress = testAddress
	})

	rec := doRequest(t, handler, http.MethodPost, "/v1/pos/payments", posCreateBody("usdc-base"), authed(nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	payment := decodePOSPayment(t, rec.Body.Bytes())
	if len(payment.ID) != 32 {
		t.Fatalf("id = %q, want 32 hex chars", payment.ID)
	}
	if payment.Status != posStatusPending || payment.Asset != "usdc-base" || payment.Network != "base" {
		t.Fatalf("payment = %+v", payment)
	}
	if payment.Address != testAddress {
		t.Fatalf("address = %q", payment.Address)
	}
	// R123.45 at R18.50/USDC = 6.672972... rounded to 2dp for base USDC.
	if payment.CryptoAmount != "6.67" {
		t.Fatalf("crypto amount = %q", payment.CryptoAmount)
	}
	if payment.PaymentURL != "https://example.com/pay/"+payment.ID {
		t.Fatalf("payment url = %q", payment.PaymentURL)
	}
	if payment.MerchantName != "Kofi Coffee" || payment.MerchantAddress != "12 Long St, Cape Town" {
		t.Fatalf("merchant = %+v", payment)
	}
	if payment.ExpiresAt <= payment.CreatedAt {
		t.Fatalf("expiry %d <= created %d", payment.ExpiresAt, payment.CreatedAt)
	}
	if len(store.posCreated) != 1 {
		t.Fatalf("stored %d payments", len(store.posCreated))
	}
}

func TestCreatePOSPaymentUsesCheckoutBaseURL(t *testing.T) {
	_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
		cfg.CheckoutBaseURL = "https://pay.afika.example/"
	})

	rec := doRequest(t, handler, http.MethodPost, "/v1/pos/payments", posCreateBody("btc"), authed(nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	payment := decodePOSPayment(t, rec.Body.Bytes())
	if !strings.HasPrefix(payment.PaymentURL, "https://pay.afika.example/pay/") {
		t.Fatalf("payment url = %q", payment.PaymentURL)
	}
	if payment.CryptoAmount != "0.00010288" {
		t.Fatalf("btc amount = %q", payment.CryptoAmount)
	}
}

func TestCreatePOSPaymentValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing receipt", `{"amountCents":100}`},
		{"zero amount", `{"receiptNo":"POS-1","amountCents":0}`},
		{"unknown asset", `{"receiptNo":"POS-1","amountCents":100,"asset":"doge"}`},
		{"non ZAR currency", `{"receiptNo":"POS-1","amountCents":100,"currency":"USD"}`},
		{"unknown field", `{"receiptNo":"POS-1","amountCents":100,"sneaky":true}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, handler := newTestHandler(t)
			rec := doRequest(t, handler, http.MethodPost, "/v1/pos/payments", tc.body, authed(nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGetPOSPaymentIsPublic(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.posPayments = map[string]POSPayment{
		"abc123": {
			ID:           "abc123",
			ReceiptNo:    "POS-0001",
			AmountCents:  12345,
			Status:       posStatusPending,
			Asset:        "usdc-base",
			Network:      "base",
			CryptoAmount: "6.67",
			ExpiresAt:    time.Now().Add(10 * time.Minute).UnixMilli(),
		},
	}

	rec := doRequest(t, handler, http.MethodGet, "/v1/pos/payments/abc123", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := decodePOSPayment(t, rec.Body.Bytes()); got.ID != "abc123" {
		t.Fatalf("id = %q", got.ID)
	}

	rec = doRequest(t, handler, http.MethodGet, "/v1/pos/payments/missing", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", rec.Code)
	}
}

func TestGetPOSPaymentReportsExpiry(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.posPayments = map[string]POSPayment{
		"old": {
			ID:          "old",
			AmountCents: 100,
			Status:      posStatusPending,
			Asset:       "usdc-base",
			ExpiresAt:   time.Now().Add(-time.Minute).UnixMilli(),
		},
	}

	rec := doRequest(t, handler, http.MethodGet, "/v1/pos/payments/old", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decodePOSPayment(t, rec.Body.Bytes()); got.Status != posStatusExpired {
		t.Fatalf("status = %q, want expired", got.Status)
	}
}

func TestSelectPOSAsset(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.posPayments = map[string]POSPayment{
		"pay-1": {
			ID:          "pay-1",
			AmountCents: 1850,
			Status:      posStatusPending,
			Asset:       "usdc-base",
			Network:     "base",
			ExpiresAt:   time.Now().Add(10 * time.Minute).UnixMilli(),
		},
	}

	rec := doRequest(t, handler, http.MethodPost, "/v1/pos/payments/pay-1/asset",
		`{"asset":"eurc-ethereum"}`, map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got := decodePOSPayment(t, rec.Body.Bytes())
	if got.Asset != "eurc-ethereum" || got.Network != "ethereum" {
		t.Fatalf("payment = %+v", got)
	}
	// R18.50 at R20.10/EURC = 0.920398... at 6dp on ethereum.
	if got.CryptoAmount != "0.920398" {
		t.Fatalf("crypto amount = %q", got.CryptoAmount)
	}

	store.posPayments["pay-1"] = POSPayment{ID: "pay-1", Status: posStatusPaid}
	rec = doRequest(t, handler, http.MethodPost, "/v1/pos/payments/pay-1/asset",
		`{"asset":"btc"}`, map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("paid status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestConfirmPOSPayment(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.posPayments = map[string]POSPayment{
		"pay-2": {
			ID:     "pay-2",
			Status: posStatusPending,
			Asset:  "usdc-base",
		},
	}

	body := `{"txHash":"0xdeadbeef"}`
	rec := doRequest(t, handler, http.MethodPost, "/v1/pos/payments/pay-2/confirm", body, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", rec.Code)
	}

	rec = doRequest(t, handler, http.MethodPost, "/v1/pos/payments/pay-2/confirm", body, authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got := decodePOSPayment(t, rec.Body.Bytes())
	if got.Status != posStatusPaid || got.TxHash != "0xdeadbeef" {
		t.Fatalf("payment = %+v", got)
	}

	// Confirming again is idempotent.
	rec = doRequest(t, handler, http.MethodPost, "/v1/pos/payments/pay-2/confirm", body, authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("repeat status = %d", rec.Code)
	}
}

func TestPOSPaymentMethodNotAllowed(t *testing.T) {
	_, _, handler := newTestHandler(t)

	tests := []struct {
		method, path, allow string
	}{
		{http.MethodGet, "/v1/pos/payments", "POST, OPTIONS"},
		{http.MethodPut, "/v1/pos/payments/pay-1", "GET, OPTIONS"},
		{http.MethodGet, "/v1/pos/payments/pay-1/asset", "POST, OPTIONS"},
		{http.MethodGet, "/v1/pos/payments/pay-1/confirm", "POST, OPTIONS"},
	}

	for _, tc := range tests {
		rec := doRequest(t, handler, tc.method, tc.path, "", authed(nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s status = %d", tc.method, tc.path, rec.Code)
		}
		if got := rec.Header().Get("Allow"); got != tc.allow {
			t.Fatalf("%s %s allow = %q, want %q", tc.method, tc.path, got, tc.allow)
		}
	}
}

func TestPayPageRendersPendingPayment(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.posPayments = map[string]POSPayment{
		"pay-page": {
			ID:              "pay-page",
			ReceiptNo:       "POS-0007",
			AmountCents:     5000,
			Status:          posStatusPending,
			Asset:           "usdc-base",
			Network:         "base",
			Address:         testAddress,
			CryptoAmount:    "2.70",
			MerchantName:    "Kofi Coffee",
			MerchantWebsite: "https://kofi.example",
			ExpiresAt:       time.Now().Add(10 * time.Minute).UnixMilli(),
		},
	}

	rec := doRequest(t, handler, http.MethodGet, "/pay/pay-page", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"R50.00", "Kofi Coffee", "POS-0007", "2.70 USDC", testAddress, "usdc-base", "eurc-base"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing %q", want)
		}
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex" {
		t.Fatalf("robots header = %q", got)
	}
}

func TestPayPageSelectsAssetFromForm(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.posPayments = map[string]POSPayment{
		"pay-form": {
			ID:          "pay-form",
			AmountCents: 1850,
			Status:      posStatusPending,
			Asset:       "usdc-base",
			Network:     "base",
			ExpiresAt:   time.Now().Add(10 * time.Minute).UnixMilli(),
		},
	}

	form := url.Values{"asset": {"btc"}}
	req := httptest.NewRequest(http.MethodPost, "/pay/pay-form", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/pay/pay-form" {
		t.Fatalf("location = %q", got)
	}
	if got := store.posPayments["pay-form"]; got.Asset != "btc" || got.Network != "bitcoin" {
		t.Fatalf("payment = %+v", got)
	}
}

func TestPayPageNotFound(t *testing.T) {
	_, _, handler := newTestHandler(t)

	for _, path := range []string{"/pay/", "/pay/unknown-id", "/pay/a/b"} {
		rec := doRequest(t, handler, http.MethodGet, path, "", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Fatalf("%s content type = %q", path, got)
		}
	}
}

func TestPayPagePaidPayment(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.posPayments = map[string]POSPayment{
		"pay-paid": {
			ID:           "pay-paid",
			ReceiptNo:    "POS-0009",
			AmountCents:  1000,
			Status:       posStatusPaid,
			Asset:        "usdc-base",
			Network:      "base",
			CryptoAmount: "0.54",
			TxHash:       "0xfeed",
		},
	}

	rec := doRequest(t, handler, http.MethodGet, "/pay/pay-paid", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Payment received") || !strings.Contains(body, "0xfeed") {
		t.Fatalf("paid page missing confirmation: %s", body)
	}
	if strings.Contains(body, "Choose how to pay") {
		t.Fatal("paid page still offers asset choices")
	}
}

func TestCORSAllowsPOST(t *testing.T) {
	_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
		cfg.AllowedOrigins = []string{"*"}
	})

	rec := doRequest(t, handler, http.MethodOptions, "/v1/pos/payments", "", map[string]string{
		"Origin":                        "https://app.example",
		"Access-Control-Request-Method": "POST",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Fatalf("allow methods = %q", got)
	}
}
