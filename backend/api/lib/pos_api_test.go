package lib

// These tests exercise the POS API end to end: real httptest requests through
// the router, the real posservice use cases, and a real SQLite database loaded
// from the same migration files the Worker deploys. They are the integration
// counterpart to the unit tests in internal/posservice.

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mindsgn-studio/afika-backend/api/internal/posservice"
	"github.com/mindsgn-studio/afika-backend/api/internal/posstore"
	_ "modernc.org/sqlite"
)

// newPOSAPI builds a handler backed by an in-memory SQLite database with the
// deployed POS migrations applied.
func newPOSAPI(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// :memory: is per-connection, so keep a single connection.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	for _, name := range []string{
		"0002_pos_payments.sql",
		"0003_pos_schema.sql",
		"0004_pos_payment_sale_link.sql",
	} {
		applyMigrationFile(t, db, filepath.Join("..", "migrations", name))
	}

	handler := NewHandler(HandlerConfig{
		Store:     NewD1Store(db),
		Verifier:  &fakeVerifier{},
		POS:       posservice.New(posstore.New(db)),
		RequestID: func() string { return "req-pos" },
		Logger:    log.New(io.Discard, "", 0),
	})
	return handler, db
}

func applyMigrationFile(t *testing.T, db *sql.DB, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", path, err)
	}
	for _, statement := range strings.Split(string(raw), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("apply migration statement failed: %v\n%s", err, statement)
		}
	}
}

func posDo(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, h, method, path, body, authed(nil))
}

func decodeData[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var env struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode data from %q: %v", rec.Body.String(), err)
	}
	return env.Data
}

func decodeList[T any](t *testing.T, rec *httptest.ResponseRecorder) []T {
	t.Helper()
	var env struct {
		Data []T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode list from %q: %v", rec.Body.String(), err)
	}
	return env.Data
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, want, rec.Body.String())
	}
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != want {
		t.Fatalf("error code = %q, want %q, body = %s", got, want, rec.Body.String())
	}
}

// setupBusiness onboards the shared-token merchant and returns its profile.
func setupBusiness(t *testing.T, h http.Handler) posstore.Business {
	t.Helper()
	rec := posDo(t, h, http.MethodPut, "/v1/pos/business",
		`{"name":"Kofi Coffee","website":"kofi.example","address":"12 Long St, Cape Town","walletAddress":"0xabc"}`)
	assertStatus(t, rec, http.StatusOK)
	return decodeData[posstore.Business](t, rec)
}

type apiCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type apiProduct struct {
	ID           string `json:"id"`
	SKU          string `json:"sku"`
	Name         string `json:"name"`
	Price        int64  `json:"price"`
	CostPrice    int64  `json:"costPrice"`
	Stock        int64  `json:"stock"`
	ReorderLevel int64  `json:"reorderLevel"`
	Active       bool   `json:"active"`
}

type apiSale struct {
	ID        string `json:"id"`
	ReceiptNo string `json:"receiptNo"`
	Status    string `json:"status"`
	Total     int64  `json:"total"`
	Subtotal  int64  `json:"subtotal"`
}

type apiSaleItem struct {
	ID        string `json:"id"`
	ProductID string `json:"productId"`
	Qty       int64  `json:"qty"`
	UnitCost  int64  `json:"unitCost"`
}

type apiSaleDetail struct {
	Sale    apiSale       `json:"sale"`
	Items   []apiSaleItem `json:"items"`
	Refunds []struct {
		ID     string `json:"id"`
		Amount int64  `json:"amount"`
	} `json:"refunds"`
}

func createProduct(t *testing.T, h http.Handler, body string) apiProduct {
	t.Helper()
	rec := posDo(t, h, http.MethodPost, "/v1/pos/products", body)
	assertStatus(t, rec, http.StatusCreated)
	return decodeData[apiProduct](t, rec)
}

func TestPOSRequiresAuthenticationAndConfiguration(t *testing.T) {
	unconfigured, _, plain := newTestHandler(t)
	_ = unconfigured
	rec := doRequest(t, plain, http.MethodGet, "/v1/pos/business", "", nil)
	// The deployment flag wins over auth: without a POS service every POS
	// route reports 503 rather than pretending the caller is unauthorised.
	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorCode(t, rec, "POS_NOT_CONFIGURED")

	handler, _ := newPOSAPI(t)
	rec = doRequest(t, handler, http.MethodGet, "/v1/pos/business", "", nil)
	assertStatus(t, rec, http.StatusUnauthorized)
}

func TestPOSBusinessOnboarding(t *testing.T) {
	handler, _ := newPOSAPI(t)

	rec := posDo(t, handler, http.MethodGet, "/v1/pos/business", "")
	assertStatus(t, rec, http.StatusOK)
	if got := strings.TrimSpace(string(decodeData[json.RawMessage](t, rec))); got != "null" {
		t.Fatalf("business before onboarding = %s, want null", got)
	}

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/categories", "")
	assertStatus(t, rec, http.StatusConflict)
	assertErrorCode(t, rec, posservice.CodeBusinessNotSetUp)

	rec = posDo(t, handler, http.MethodPut, "/v1/pos/business", `{"name":"Kofi Coffee","website":"kofi.example"}`)
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorCode(t, rec, posservice.CodeInvalidBusiness)

	business := setupBusiness(t, handler)
	if business.ID == "" || business.Name != "Kofi Coffee" {
		t.Fatalf("business = %+v", business)
	}
	if business.Website != "https://kofi.example" {
		t.Fatalf("website = %q, want normalised https URL", business.Website)
	}

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/business", "")
	assertStatus(t, rec, http.StatusOK)
	if got := decodeData[posstore.Business](t, rec).ID; got != business.ID {
		t.Fatalf("business id = %q, want %q", got, business.ID)
	}
}

func TestPOSCatalogueAndInventory(t *testing.T) {
	handler, _ := newPOSAPI(t)
	setupBusiness(t, handler)

	rec := posDo(t, handler, http.MethodPost, "/v1/pos/categories", `{"name":"Coffee"}`)
	assertStatus(t, rec, http.StatusCreated)
	category := decodeData[apiCategory](t, rec)

	rec = posDo(t, handler, http.MethodPost, "/v1/pos/categories", `{"name":"Coffee"}`)
	assertStatus(t, rec, http.StatusConflict)
	assertErrorCode(t, rec, posservice.CodeCategoryExists)

	product := createProduct(t, handler,
		`{"sku":"flat-white","name":"Flat White","categoryId":"`+category.ID+`","price":3500,"costPrice":1200,"stock":3,"reorderLevel":5}`)
	if product.SKU != "FLAT-WHITE" || product.Stock != 3 || !product.Active {
		t.Fatalf("product = %+v", product)
	}

	rec = posDo(t, handler, http.MethodPost, "/v1/pos/products", `{"sku":"FLAT-WHITE","name":"Other","price":100}`)
	assertStatus(t, rec, http.StatusConflict)
	assertErrorCode(t, rec, posservice.CodeSKUExists)

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/products?query=flat", "")
	assertStatus(t, rec, http.StatusOK)
	if got := len(decodeList[apiProduct](t, rec)); got != 1 {
		t.Fatalf("search returned %d products, want 1", got)
	}

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/inventory/low-stock", "")
	assertStatus(t, rec, http.StatusOK)
	if got := len(decodeList[apiProduct](t, rec)); got != 1 {
		t.Fatalf("low stock returned %d products, want 1", got)
	}

	rec = posDo(t, handler, http.MethodPost, "/v1/pos/products/"+product.ID+"/stock",
		`{"kind":"restock","delta":7,"unitCost":1250,"note":"Weekly order"}`)
	assertStatus(t, rec, http.StatusOK)
	restocked := decodeData[apiProduct](t, rec)
	if restocked.Stock != 10 || restocked.CostPrice != 1250 {
		t.Fatalf("restocked product = %+v", restocked)
	}

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/inventory/movements?productId="+product.ID, "")
	assertStatus(t, rec, http.StatusOK)
	if got := len(decodeList[posstore.StockMovement](t, rec)); got < 2 {
		t.Fatalf("movements = %d, want opening stock plus restock", got)
	}

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/inventory/valuation", "")
	assertStatus(t, rec, http.StatusOK)
	valuation := decodeData[posstore.InventoryValue](t, rec)
	if valuation.Units != 10 || valuation.ProductCount != 1 {
		t.Fatalf("valuation = %+v", valuation)
	}
	if valuation.CostValue <= 0 || valuation.RetailValue <= valuation.CostValue {
		t.Fatalf("valuation = %+v, want positive cost below retail", valuation)
	}

	rec = posDo(t, handler, http.MethodDelete, "/v1/pos/categories/"+category.ID, "")
	assertStatus(t, rec, http.StatusConflict)
	assertErrorCode(t, rec, posservice.CodeCategoryInUse)
}

func TestPOSCheckoutCompleteRefundAndVoid(t *testing.T) {
	handler, _ := newPOSAPI(t)
	setupBusiness(t, handler)
	product := createProduct(t, handler,
		`{"sku":"FW","name":"Flat White","price":5000,"costPrice":2000,"stock":10,"reorderLevel":2}`)

	assertStock := func(want int64) {
		t.Helper()
		rec := posDo(t, handler, http.MethodGet, "/v1/pos/products/"+product.ID, "")
		assertStatus(t, rec, http.StatusOK)
		if got := decodeData[apiProduct](t, rec).Stock; got != want {
			t.Fatalf("stock = %d, want %d", got, want)
		}
	}

	// A pending sale must not touch stock: the payment has not happened yet.
	rec := posDo(t, handler, http.MethodPost, "/v1/pos/sales",
		`{"items":[{"productId":"`+product.ID+`","qty":2}]}`)
	assertStatus(t, rec, http.StatusCreated)
	pending := decodeData[apiSaleDetail](t, rec)
	if pending.Sale.Status != "pending" || pending.Sale.Total != 10000 || len(pending.Items) != 1 {
		t.Fatalf("pending sale = %+v", pending)
	}
	assertStock(10)

	rec = posDo(t, handler, http.MethodPost, "/v1/pos/sales/"+pending.Sale.ID+"/complete", `{}`)
	assertStatus(t, rec, http.StatusOK)
	completed := decodeData[apiSaleDetail](t, rec)
	if completed.Sale.Status != "completed" {
		t.Fatalf("completed sale status = %q", completed.Sale.Status)
	}
	assertStock(8)

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/sales/"+pending.Sale.ID+"/receipt", "")
	assertStatus(t, rec, http.StatusOK)
	if got := decodeData[posReceiptData](t, rec).Receipt; !strings.Contains(got, pending.Sale.ReceiptNo) {
		t.Fatalf("receipt = %q, want it to mention %q", got, pending.Sale.ReceiptNo)
	}

	// A partial refund restocks the returned unit and leaves the sale open.
	rec = posDo(t, handler, http.MethodPost, "/v1/pos/sales/"+pending.Sale.ID+"/refunds",
		`{"items":[{"saleItemId":"`+pending.Items[0].ID+`","qty":1}],"reason":"Cold"}`)
	assertStatus(t, rec, http.StatusCreated)
	refund := decodeData[struct {
		Refund struct {
			ID string `json:"id"`
		} `json:"refund"`
		Status        string `json:"status"`
		RefundedTotal int64  `json:"refundedTotal"`
	}](t, rec)
	if refund.RefundedTotal != 5000 || refund.Status != "partially_refunded" || refund.Refund.ID == "" {
		t.Fatalf("refund = %+v", refund)
	}
	assertStock(9)

	// A refund moves the sale out of the voidable state.
	rec = posDo(t, handler, http.MethodPost, "/v1/pos/sales/"+pending.Sale.ID+"/void", `{}`)
	assertStatus(t, rec, http.StatusConflict)
	assertErrorCode(t, rec, posservice.CodeSaleNotVoidable)

	// Immediate settlement via complete:true, then void restores everything.
	rec = posDo(t, handler, http.MethodPost, "/v1/pos/sales",
		`{"items":[{"productId":"`+product.ID+`","qty":3}],"complete":true}`)
	assertStatus(t, rec, http.StatusCreated)
	cash := decodeData[apiSaleDetail](t, rec)
	if cash.Sale.Status != "completed" {
		t.Fatalf("cash sale status = %q", cash.Sale.Status)
	}
	assertStock(6)

	rec = posDo(t, handler, http.MethodPost, "/v1/pos/sales/"+cash.Sale.ID+"/void", `{"reason":"Mistake"}`)
	assertStatus(t, rec, http.StatusOK)
	if got := decodeData[apiSaleDetail](t, rec).Sale.Status; got != "voided" {
		t.Fatalf("voided sale status = %q", got)
	}
	assertStock(9)
}

func TestPOSSaleIdempotency(t *testing.T) {
	handler, _ := newPOSAPI(t)
	setupBusiness(t, handler)
	product := createProduct(t, handler,
		`{"sku":"FW","name":"Flat White","price":2500,"costPrice":1000,"stock":10}`)

	body := `{"items":[{"productId":"` + product.ID + `","qty":1}],"complete":true,"idempotencyKey":"sale-key-1"}`
	rec := posDo(t, handler, http.MethodPost, "/v1/pos/sales", body)
	assertStatus(t, rec, http.StatusCreated)
	first := decodeData[apiSaleDetail](t, rec)

	// Retrying the same key returns the original sale, not a second one.
	rec = posDo(t, handler, http.MethodPost, "/v1/pos/sales", body)
	assertStatus(t, rec, http.StatusCreated)
	replay := decodeData[apiSaleDetail](t, rec)
	if replay.Sale.ID != first.Sale.ID || replay.Sale.ReceiptNo != first.Sale.ReceiptNo {
		t.Fatalf("replay = %+v, want sale %q", replay.Sale, first.Sale.ID)
	}

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/products/"+product.ID, "")
	assertStatus(t, rec, http.StatusOK)
	if got := decodeData[apiProduct](t, rec).Stock; got != 9 {
		t.Fatalf("stock = %d, want 9 after one settled sale", got)
	}

	// The same key with a different cart is a client bug, not a retry.
	rec = posDo(t, handler, http.MethodPost, "/v1/pos/sales",
		`{"items":[{"productId":"`+product.ID+`","qty":2}],"complete":true,"idempotencyKey":"sale-key-1"}`)
	assertStatus(t, rec, http.StatusConflict)
	assertErrorCode(t, rec, posservice.CodeIdempotencyKeyReused)
}

func TestPOSPaymentLinkSettlesLinkedSale(t *testing.T) {
	handler, _ := newPOSAPI(t)
	setupBusiness(t, handler)
	product := createProduct(t, handler,
		`{"sku":"FW","name":"Flat White","price":5000,"costPrice":2000,"stock":10}`)

	rec := posDo(t, handler, http.MethodPost, "/v1/pos/sales",
		`{"items":[{"productId":"`+product.ID+`","qty":1}]}`)
	assertStatus(t, rec, http.StatusCreated)
	sale := decodeData[apiSaleDetail](t, rec).Sale

	// A tampered amount is rejected instead of undercharging the merchant.
	rec = posDo(t, handler, http.MethodPost, "/v1/pos/payments",
		`{"saleId":"`+sale.ID+`","amountCents":1}`)
	assertStatus(t, rec, http.StatusBadRequest)

	rec = posDo(t, handler, http.MethodPost, "/v1/pos/payments", `{"saleId":"`+sale.ID+`"}`)
	assertStatus(t, rec, http.StatusCreated)
	payment := decodeData[POSPayment](t, rec)
	if payment.SaleID != sale.ID || payment.AmountCents != sale.Total || payment.ReceiptNo != sale.ReceiptNo {
		t.Fatalf("payment = %+v, want linked to sale %q at %d cents", payment, sale.ID, sale.Total)
	}
	if payment.MerchantName != "Kofi Coffee" || payment.Address != "0xabc" {
		t.Fatalf("payment merchant/address = %+v", payment)
	}

	rec = posDo(t, handler, http.MethodPost, "/v1/pos/payments/"+payment.ID+"/confirm", `{"txHash":"0xdeadbeef"}`)
	assertStatus(t, rec, http.StatusOK)
	confirmed := decodeData[POSPayment](t, rec)
	if confirmed.Status != "paid" || confirmed.TxHash != "0xdeadbeef" {
		t.Fatalf("confirmed payment = %+v", confirmed)
	}

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/sales/"+sale.ID, "")
	assertStatus(t, rec, http.StatusOK)
	settled := decodeData[apiSaleDetail](t, rec)
	if settled.Sale.Status != "completed" {
		t.Fatalf("linked sale status = %q, want completed", settled.Sale.Status)
	}

	// Confirming again is safe: the payment and the sale are already settled.
	rec = posDo(t, handler, http.MethodPost, "/v1/pos/payments/"+payment.ID+"/confirm", `{"txHash":"0xdeadbeef"}`)
	assertStatus(t, rec, http.StatusOK)

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/products/"+product.ID, "")
	assertStatus(t, rec, http.StatusOK)
	if got := decodeData[apiProduct](t, rec).Stock; got != 9 {
		t.Fatalf("stock = %d, want 9 after one settled sale", got)
	}
}

func TestPOSErrorMapping(t *testing.T) {
	handler, _ := newPOSAPI(t)
	setupBusiness(t, handler)

	rec := posDo(t, handler, http.MethodGet, "/v1/pos/products/missing", "")
	assertStatus(t, rec, http.StatusNotFound)
	assertErrorCode(t, rec, posservice.CodeProductNotFound)

	rec = posDo(t, handler, http.MethodPost, "/v1/pos/categories", `{"name":`)
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorCode(t, rec, "INVALID_JSON")

	rec = posDo(t, handler, http.MethodPost, "/v1/pos/sales", `{"items":[]}`)
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorCode(t, rec, posservice.CodeCartEmpty)

	rec = posDo(t, handler, http.MethodDelete, "/v1/pos/categories", "")
	assertStatus(t, rec, http.StatusMethodNotAllowed)
	if got := rec.Header().Get("Allow"); got != "GET, POST, OPTIONS" {
		t.Fatalf("allow = %q", got)
	}
}

func TestPOSProfitReport(t *testing.T) {
	handler, _ := newPOSAPI(t)
	setupBusiness(t, handler)
	product := createProduct(t, handler,
		`{"sku":"FW","name":"Flat White","price":5000,"costPrice":2000,"stock":10}`)

	rec := posDo(t, handler, http.MethodPost, "/v1/pos/sales",
		`{"items":[{"productId":"`+product.ID+`","qty":2}],"complete":true}`)
	assertStatus(t, rec, http.StatusCreated)
	sale := decodeData[apiSaleDetail](t, rec)

	rec = posDo(t, handler, http.MethodPost, "/v1/pos/sales/"+sale.Sale.ID+"/refunds",
		`{"items":[{"saleItemId":"`+sale.Items[0].ID+`","qty":1}]}`)
	assertStatus(t, rec, http.StatusCreated)

	rec = posDo(t, handler, http.MethodGet, "/v1/pos/reports/profit", "")
	assertStatus(t, rec, http.StatusOK)
	report := decodeData[map[string]any](t, rec)
	if got := report["salesCount"]; got != float64(1) {
		t.Fatalf("salesCount = %v, want 1", got)
	}
	if got := report["netRevenue"]; got != float64(5000) {
		t.Fatalf("netRevenue = %v, want 5000", got)
	}
	// One unit sold keeps its R20 cost; the returned unit's cost is recovered.
	if got := report["cogs"]; got != float64(2000) {
		t.Fatalf("cogs = %v, want 2000", got)
	}
	if got := report["profit"]; got != float64(3000) {
		t.Fatalf("profit = %v, want 3000", got)
	}
}
