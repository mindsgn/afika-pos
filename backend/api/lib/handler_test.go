package lib

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	upsertWallet  Wallet
	upsertCreated bool
	upsertErr     error
	upsertInput   WalletInput
	upsertCalls   int

	settings         WalletSettings
	settingsErr      error
	settingsAddr     string
	settingsInput    WalletSettingsInput
	settingsGetCalls int
	settingsPutCalls int

	balances      BalancePage
	balanceErr    error
	balanceAddr   string
	balanceCursor *BalanceCursor
	balanceLimit  int
	balanceCalls  int

	transactions TransactionPage
	txErr        error
	txAddr       string
	txCursor     *TransactionCursor
	txLimit      int
	txCalls      int

	posPayments  map[string]POSPayment
	posCreateErr error
	posGetErr    error
	posUpdateErr error
	posCreated   []POSPayment
	posUpdated   []POSPayment
}

func (f *fakeStore) CreatePOSPayment(_ context.Context, p POSPayment) (POSPayment, error) {
	if f.posCreateErr != nil {
		return POSPayment{}, f.posCreateErr
	}
	if f.posPayments == nil {
		f.posPayments = map[string]POSPayment{}
	}
	f.posPayments[p.ID] = p
	f.posCreated = append(f.posCreated, p)
	return p, nil
}

func (f *fakeStore) GetPOSPayment(_ context.Context, id string) (POSPayment, error) {
	if f.posGetErr != nil {
		return POSPayment{}, f.posGetErr
	}
	p, ok := f.posPayments[id]
	if !ok {
		return POSPayment{}, ErrPOSPaymentNotFound
	}
	return p, nil
}

func (f *fakeStore) UpdatePOSPayment(_ context.Context, p POSPayment) (POSPayment, error) {
	if f.posUpdateErr != nil {
		return POSPayment{}, f.posUpdateErr
	}
	if f.posPayments == nil {
		f.posPayments = map[string]POSPayment{}
	}
	if _, ok := f.posPayments[p.ID]; !ok {
		return POSPayment{}, ErrPOSPaymentNotFound
	}
	f.posPayments[p.ID] = p
	f.posUpdated = append(f.posUpdated, p)
	return p, nil
}

func (f *fakeStore) GetFXRates(context.Context) (FXRates, error) {
	return FXRates{USD: "1", ZAR: "18.5", EUR: "0.92", UpdatedAt: 123}, nil
}

func (f *fakeStore) UpsertWallet(_ context.Context, in WalletInput) (Wallet, bool, error) {
	f.upsertCalls++
	f.upsertInput = in
	return f.upsertWallet, f.upsertCreated, f.upsertErr
}

func (f *fakeStore) GetWalletSettings(_ context.Context, address string) (WalletSettings, error) {
	f.settingsGetCalls++
	f.settingsAddr = address
	return f.settings, f.settingsErr
}

func (f *fakeStore) UpsertWalletSettings(_ context.Context, in WalletSettingsInput) (WalletSettings, error) {
	f.settingsPutCalls++
	f.settingsInput = in
	return f.settings, f.settingsErr
}

func (f *fakeStore) ListBalances(_ context.Context, address string, after *BalanceCursor, limit int) (BalancePage, error) {
	f.balanceCalls++
	f.balanceAddr = address
	f.balanceCursor = after
	f.balanceLimit = limit
	return f.balances, f.balanceErr
}

func (f *fakeStore) ListTransactions(_ context.Context, address string, after *TransactionCursor, limit int) (TransactionPage, error) {
	f.txCalls++
	f.txAddr = address
	f.txCursor = after
	f.txLimit = limit
	return f.transactions, f.txErr
}

type fakeVerifier struct {
	err    error
	tokens []string
}

func (f *fakeVerifier) Verify(token string) error {
	f.tokens = append(f.tokens, token)
	return f.err
}

func newTestHandler(t *testing.T, opts ...func(*HandlerConfig)) (*fakeStore, *fakeVerifier, http.Handler) {
	t.Helper()

	store := &fakeStore{
		upsertWallet: Wallet{
			Address:   testAddress,
			Network:   "base-mainnet",
			CreatedAt: 1000,
			UpdatedAt: 1000,
		},
	}
	verifier := &fakeVerifier{}
	cfg := HandlerConfig{
		Store:     store,
		Verifier:  verifier,
		RequestID: func() string { return "req-fixed" },
		Logger:    log.New(io.Discard, "", 0),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return store, verifier, NewHandler(cfg)
}

func doRequest(t *testing.T, h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func authed(headers map[string]string) map[string]string {
	out := map[string]string{"Authorization": "Bearer test-token"}
	for key, value := range headers {
		out[key] = value
	}
	return out
}

func addresses(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "address=" + testAddress
}

type testErrorEnvelope struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"requestId"`
	} `json:"error"`
}

type pagination struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor"`
}

func decodeError(t *testing.T, body []byte) testErrorEnvelope {
	t.Helper()
	var env testErrorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode error body %q: %v", body, err)
	}
	return env
}

func TestHandlerHealth(t *testing.T) {
	_, _, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/v1/health", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Status != "ok" {
		t.Fatalf("status = %q", body.Data.Status)
	}
	if rec.Header().Get("X-Request-Id") != "req-fixed" {
		t.Fatalf("request id = %q", rec.Header().Get("X-Request-Id"))
	}
}

func TestHandlerRequiresAuthentication(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"balances", http.MethodGet, addresses("/v1/balances"), ""},
		{"transactions", http.MethodGet, addresses("/v1/transactions"), ""},
		{"upsert wallet", http.MethodPut, "/v1/wallet", `{"address":"` + testAddress + `"}`},
		{"invalid body still unauthenticated", http.MethodPut, "/v1/wallet", `{`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, verifier, handler := newTestHandler(t)
			rec := doRequest(t, handler, tc.method, tc.path, tc.body, nil)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != "UNAUTHENTICATED" {
				t.Fatalf("code = %q", got)
			}
			if len(verifier.tokens) != 0 {
				t.Fatalf("verifier called with %v", verifier.tokens)
			}
			if store.upsertCalls+store.balanceCalls+store.txCalls != 0 {
				t.Fatal("store was called without authentication")
			}
		})
	}
}

func TestHandlerRejectsInvalidAuthorization(t *testing.T) {
	_, verifier, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances"), "", map[string]string{"Authorization": "Basic abc"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(verifier.tokens) != 0 {
		t.Fatalf("verifier called with %v", verifier.tokens)
	}

	verifier.err = ErrUnauthenticated
	rec = doRequest(t, handler, http.MethodGet, addresses("/v1/balances"), "", authed(nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandlerPassesBearerTokenToVerifier(t *testing.T) {
	_, verifier, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances"), "", authed(map[string]string{
		"Authorization": "bearer actual-token",
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(verifier.tokens) != 1 || verifier.tokens[0] != "actual-token" {
		t.Fatalf("verifier tokens = %v", verifier.tokens)
	}
}

func TestHandlerUpsertWallet(t *testing.T) {
	tests := []struct {
		name       string
		created    bool
		wantStatus int
	}{
		{"created", true, http.StatusCreated},
		{"updated", false, http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, _, handler := newTestHandler(t)
			store.upsertCreated = tc.created
			store.upsertWallet = Wallet{
				Address:   testAddress,
				Network:   "base-mainnet",
				CreatedAt: 1000,
				UpdatedAt: 2000,
			}

			rec := doRequest(t, handler, http.MethodPut, "/v1/wallet",
				`{"address":"`+strings.ToUpper(testAddress)+`","network":"base"}`, authed(nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			want := WalletInput{Address: testAddress, Network: "base-mainnet", Testnet: false}
			if store.upsertInput != want {
				t.Fatalf("input = %+v, want %+v", store.upsertInput, want)
			}

			var body struct {
				Data Wallet `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Data != store.upsertWallet {
				t.Fatalf("data = %+v, want %+v", body.Data, store.upsertWallet)
			}
		})
	}
}

func TestHandlerUpsertWalletTestnetDerivedFromNetwork(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.upsertWallet = Wallet{Address: testAddress, Network: "base-sepolia", Testnet: true}

	rec := doRequest(t, handler, http.MethodPut, "/v1/wallet",
		`{"address":"`+testAddress+`","network":"base-sepolia"}`, authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := WalletInput{Address: testAddress, Network: "base-sepolia", Testnet: true}
	if store.upsertInput != want {
		t.Fatalf("input = %+v, want %+v", store.upsertInput, want)
	}
}

func TestHandlerUpsertWalletValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		headers map[string]string
		status  int
		code    string
	}{
		{"empty body", "", map[string]string{"Content-Type": "application/json"}, http.StatusBadRequest, "INVALID_JSON"},
		{"malformed json", `{`, map[string]string{"Content-Type": "application/json"}, http.StatusBadRequest, "INVALID_JSON"},
		{"trailing data", `{"address":"` + testAddress + `"}{}`, map[string]string{"Content-Type": "application/json"}, http.StatusBadRequest, "INVALID_JSON"},
		{"unknown field", `{"address":"` + testAddress + `","extra":1}`, map[string]string{"Content-Type": "application/json"}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"missing address", `{"network":"base"}`, map[string]string{"Content-Type": "application/json"}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"short address", `{"address":"0x123"}`, map[string]string{"Content-Type": "application/json"}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"non hex address", `{"address":"0xzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}`, map[string]string{"Content-Type": "application/json"}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"unsupported network", `{"address":"` + testAddress + `","network":"solana"}`, map[string]string{"Content-Type": "application/json"}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"wrong content type", `{"address":"` + testAddress + `"}`, map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, _, handler := newTestHandler(t)
			headers := authed(tc.headers)
			rec := doRequest(t, handler, http.MethodPut, "/v1/wallet", tc.body, headers)

			if rec.Code != tc.status {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != tc.code {
				t.Fatalf("code = %q, want %q", got, tc.code)
			}
			if store.upsertCalls != 0 {
				t.Fatal("store was called for an invalid request")
			}
		})
	}
}

func TestHandlerUpsertWalletRejectsOversizedBody(t *testing.T) {
	_, _, handler := newTestHandler(t)
	body := `{"address":"` + testAddress + `","padding":"` + strings.Repeat("a", maxRequestBodyBytes) + `"}`

	rec := doRequest(t, handler, http.MethodPut, "/v1/wallet", body, authed(nil))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != "PAYLOAD_TOO_LARGE" {
		t.Fatalf("code = %q", got)
	}
}

func TestHandlerBalancesPagination(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.balances = BalancePage{
		Items: []Balance{
			{WalletAddress: testAddress, Network: "base-mainnet", TokenSymbol: "USDC", Amount: "10"},
			{WalletAddress: testAddress, Network: "eth-mainnet", TokenSymbol: "DAI", Amount: "5"},
		},
		HasMore: true,
	}

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances?limit=2"), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.balanceAddr != testAddress {
		t.Fatalf("address = %q", store.balanceAddr)
	}
	if store.balanceLimit != 2 || store.balanceCursor != nil {
		t.Fatalf("limit/cursor = %d/%v", store.balanceLimit, store.balanceCursor)
	}

	var body struct {
		Data       []Balance  `json:"data"`
		Pagination pagination `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("data length = %d", len(body.Data))
	}
	if body.Pagination.Limit != 2 || !body.Pagination.HasMore {
		t.Fatalf("pagination = %+v", body.Pagination)
	}

	wantCursor := BalanceCursor{Network: "eth-mainnet", TokenSymbol: "DAI"}
	gotCursor, err := DecodeBalanceCursor(body.Pagination.NextCursor)
	if err != nil {
		t.Fatalf("decode next cursor: %v", err)
	}
	if gotCursor != wantCursor {
		t.Fatalf("cursor = %+v, want %+v", gotCursor, wantCursor)
	}

	rec = doRequest(t, handler, http.MethodGet, addresses("/v1/balances?limit=2&cursor="+body.Pagination.NextCursor), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("second page status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.balanceCursor == nil || *store.balanceCursor != wantCursor {
		t.Fatalf("second page cursor = %+v, want %+v", store.balanceCursor, wantCursor)
	}
}

func TestHandlerBalancesNormalizesAddress(t *testing.T) {
	store, _, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/v1/balances?address="+strings.ToUpper(testAddress), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.balanceAddr != testAddress {
		t.Fatalf("address = %q, want %q", store.balanceAddr, testAddress)
	}
}

func TestHandlerBalancesDefaultsAndEmptyData(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.balances = BalancePage{}

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances"), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.balanceLimit != defaultPageSize {
		t.Fatalf("limit = %d, want %d", store.balanceLimit, defaultPageSize)
	}
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("body = %s, want an empty array", rec.Body.String())
	}
}

func TestHandlerBalancesInvalidQuery(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		status int
		code   string
	}{
		{"missing address", "?limit=2", http.StatusBadRequest, "VALIDATION_ERROR"},
		{"invalid address", "?address=0x123", http.StatusBadRequest, "VALIDATION_ERROR"},
		{"duplicate address", "?address=" + testAddress + "&address=" + testAddress, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"zero limit", addresses("?limit=0"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"negative limit", addresses("?limit=-1"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"too large", addresses("?limit=101"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"not a number", addresses("?limit=abc"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"float limit", addresses("?limit=1.5"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"unknown parameter", addresses("?offset=1"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"invalid cursor", addresses("?cursor=garbage"), http.StatusBadRequest, "INVALID_CURSOR"},
		{"transaction cursor", addresses("?cursor=" + EncodeTransactionCursor(TransactionCursor{TimestampMs: 1, TxHash: "0xabc", Direction: "credit"})), http.StatusBadRequest, "INVALID_CURSOR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, _, handler := newTestHandler(t)
			rec := doRequest(t, handler, http.MethodGet, "/v1/balances"+tc.query, "", authed(nil))

			if rec.Code != tc.status {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != tc.code {
				t.Fatalf("code = %q, want %q", got, tc.code)
			}
			if store.balanceCalls != 0 {
				t.Fatal("store was called for an invalid query")
			}
		})
	}
}

func TestHandlerTransactionsPagination(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.transactions = TransactionPage{
		Items: []Transaction{
			{WalletAddress: testAddress, TxHash: "0xbbb", Direction: "credit", TimestampMs: 3000, Timestamp: 3000},
			{WalletAddress: testAddress, TxHash: "0xaaa", Direction: "debit", TimestampMs: 2000, Timestamp: 2000},
		},
		HasMore: true,
	}

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/transactions?limit=2"), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.txAddr != testAddress || store.txLimit != 2 || store.txCursor != nil {
		t.Fatalf("address/limit/cursor = %q/%d/%v", store.txAddr, store.txLimit, store.txCursor)
	}

	var body struct {
		Data       []Transaction `json:"data"`
		Pagination pagination    `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 2 || !body.Pagination.HasMore {
		t.Fatalf("body = %+v", body)
	}

	wantCursor := TransactionCursor{TimestampMs: 2000, TxHash: "0xaaa", Direction: "debit"}
	gotCursor, err := DecodeTransactionCursor(body.Pagination.NextCursor)
	if err != nil {
		t.Fatalf("decode next cursor: %v", err)
	}
	if gotCursor != wantCursor {
		t.Fatalf("cursor = %+v, want %+v", gotCursor, wantCursor)
	}

	rec = doRequest(t, handler, http.MethodGet, addresses("/v1/transactions?limit=2&cursor="+body.Pagination.NextCursor), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("second page status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.txCursor == nil || *store.txCursor != wantCursor {
		t.Fatalf("second page cursor = %+v, want %+v", store.txCursor, wantCursor)
	}
}

func TestHandlerTransactionsDefaultsAndEmptyData(t *testing.T) {
	store, _, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/transactions"), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.txLimit != defaultPageSize {
		t.Fatalf("limit = %d", store.txLimit)
	}
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("body = %s, want an empty array", rec.Body.String())
	}
}

func TestHandlerTransactionsInvalidQuery(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		status int
		code   string
	}{
		{"missing address", "?limit=2", http.StatusBadRequest, "VALIDATION_ERROR"},
		{"invalid address", "?address=0x123", http.StatusBadRequest, "VALIDATION_ERROR"},
		{"zero limit", addresses("?limit=0"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"too large", addresses("?limit=101"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"unknown parameter", addresses("?page=2"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"invalid cursor", addresses("?cursor=garbage"), http.StatusBadRequest, "INVALID_CURSOR"},
		{"balance cursor", addresses("?cursor=" + EncodeBalanceCursor(BalanceCursor{Network: "base-mainnet", TokenSymbol: "USDC"})), http.StatusBadRequest, "INVALID_CURSOR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, _, handler := newTestHandler(t)
			rec := doRequest(t, handler, http.MethodGet, "/v1/transactions"+tc.query, "", authed(nil))

			if rec.Code != tc.status {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != tc.code {
				t.Fatalf("code = %q, want %q", got, tc.code)
			}
			if store.txCalls != 0 {
				t.Fatal("store was called for an invalid query")
			}
		})
	}
}

func TestHandlerStoreFailureIsHidden(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.balanceErr = errors.New("secret db error")

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances"), "", authed(nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	env := decodeError(t, rec.Body.Bytes())
	if env.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("code = %q", env.Error.Code)
	}
	if strings.Contains(env.Error.Message, "secret db error") {
		t.Fatalf("internal error leaked: %q", env.Error.Message)
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	tests := []struct {
		method string
		path   string
		allow  string
	}{
		{http.MethodGet, "/v1/wallet", "PUT, OPTIONS"},
		{http.MethodPost, addresses("/v1/balances"), "GET, OPTIONS"},
		{http.MethodDelete, addresses("/v1/transactions"), "GET, OPTIONS"},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			_, _, handler := newTestHandler(t)
			rec := doRequest(t, handler, tc.method, tc.path, "", authed(nil))

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Allow"); got != tc.allow {
				t.Fatalf("Allow = %q, want %q", got, tc.allow)
			}
			if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != "METHOD_NOT_ALLOWED" {
				t.Fatalf("code = %q", got)
			}
		})
	}
}

func TestHandlerUnknownRoute(t *testing.T) {
	_, _, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/v1/unknown", "", authed(nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != "NOT_FOUND" {
		t.Fatalf("code = %q", got)
	}
}

func TestHandlerGetSettingsDefaultsToUSD(t *testing.T) {
	store, _, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/settings"), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.settingsAddr != testAddress {
		t.Fatalf("address = %q, want %q", store.settingsAddr, testAddress)
	}

	var body struct {
		Data WalletSettings `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Address != testAddress || body.Data.PreferredCurrency != "USD" {
		t.Fatalf("data = %+v, want USD default", body.Data)
	}
}

func TestHandlerGetSettingsReturnsSavedCurrency(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.settings = WalletSettings{
		Address:           testAddress,
		PreferredCurrency: "ZAR",
		CreatedAt:         1000,
		UpdatedAt:         2000,
	}

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/settings"), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data WalletSettings `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data != store.settings {
		t.Fatalf("data = %+v, want %+v", body.Data, store.settings)
	}
}

func TestHandlerUpdateSettings(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.settings = WalletSettings{
		Address:           testAddress,
		PreferredCurrency: "EUR",
		CreatedAt:         1000,
		UpdatedAt:         2000,
	}

	rec := doRequest(t, handler, http.MethodPut, "/v1/settings",
		`{"address":"`+strings.ToUpper(testAddress)+`","preferredCurrency":"eur"}`, authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := WalletSettingsInput{Address: testAddress, PreferredCurrency: "EUR"}
	if store.settingsInput != want {
		t.Fatalf("input = %+v, want %+v", store.settingsInput, want)
	}

	var body struct {
		Data WalletSettings `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data != store.settings {
		t.Fatalf("data = %+v, want %+v", body.Data, store.settings)
	}
}

func TestHandlerUpdateSettingsValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing address", `{"preferredCurrency":"USD"}`},
		{"invalid address", `{"address":"0x123","preferredCurrency":"USD"}`},
		{"missing currency", `{"address":"` + testAddress + `"}`},
		{"unsupported currency", `{"address":"` + testAddress + `","preferredCurrency":"GBP"}`},
		{"unknown field", `{"address":"` + testAddress + `","preferredCurrency":"USD","extra":1}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, _, handler := newTestHandler(t)
			rec := doRequest(t, handler, http.MethodPut, "/v1/settings", tc.body, authed(nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != "VALIDATION_ERROR" {
				t.Fatalf("code = %q", got)
			}
			if store.settingsPutCalls != 0 {
				t.Fatal("store was called for an invalid request")
			}
		})
	}
}

func TestHandlerSettingsRequiresAuthentication(t *testing.T) {
	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, addresses("/v1/settings"), ""},
		{http.MethodPut, "/v1/settings", `{"address":"` + testAddress + `","preferredCurrency":"USD"}`},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			store, _, handler := newTestHandler(t)
			rec := doRequest(t, handler, tc.method, tc.path, tc.body, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if store.settingsGetCalls+store.settingsPutCalls != 0 {
				t.Fatal("store was called without authentication")
			}
		})
	}
}

func TestHandlerBalancesUsesRequestedCurrency(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.balances = BalancePage{Items: []Balance{{
		WalletAddress: testAddress,
		Network:       "base-mainnet",
		TokenSymbol:   "USDC",
		USDAmount:     "10",
		ZARAmount:     "185",
		EURAmount:     "9.2",
	}}}

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances?currency=zar"), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.settingsGetCalls != 0 {
		t.Fatal("settings lookup should be skipped for an explicit currency")
	}

	var body struct {
		Data     []Balance `json:"data"`
		Currency string    `json:"currency"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Currency != "ZAR" {
		t.Fatalf("currency = %q, want ZAR", body.Currency)
	}
	if len(body.Data) != 1 || body.Data[0].DisplayCurrency != "ZAR" || body.Data[0].DisplayAmount != "185" {
		t.Fatalf("data = %+v", body.Data)
	}
}

func TestHandlerBalancesUsesSavedCurrency(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.settings = WalletSettings{Address: testAddress, PreferredCurrency: "EUR"}
	store.balances = BalancePage{Items: []Balance{{
		WalletAddress: testAddress,
		Network:       "base-mainnet",
		TokenSymbol:   "USDC",
		USDAmount:     "10",
		ZARAmount:     "185",
		EURAmount:     "9.2",
	}}}

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances"), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.settingsGetCalls != 1 || store.settingsAddr != testAddress {
		t.Fatalf("settings lookup = %d/%q", store.settingsGetCalls, store.settingsAddr)
	}

	var body struct {
		Data     []Balance `json:"data"`
		Currency string    `json:"currency"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Currency != "EUR" || body.Data[0].DisplayAmount != "9.2" {
		t.Fatalf("body = %+v", body)
	}
}

func TestHandlerTransactionsUseCurrencyForAmountAndFee(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.transactions = TransactionPage{Items: []Transaction{{
		WalletAddress: testAddress,
		TxHash:        "0xabc",
		Direction:     "credit",
		TimestampMs:   3000,
		USDAmount:     "10",
		ZARAmount:     "185",
		EURAmount:     "9.2",
		FeeUSD:        "0.02",
		FeeZAR:        "0.37",
		FeeEUR:        "0.018",
	}}}

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/transactions?currency=EUR"), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data     []Transaction `json:"data"`
		Currency string        `json:"currency"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Currency != "EUR" {
		t.Fatalf("currency = %q", body.Currency)
	}
	tx := body.Data[0]
	if tx.DisplayAmount != "9.2" || tx.DisplayFee != "0.018" || tx.DisplayCurrency != "EUR" {
		t.Fatalf("transaction = %+v", tx)
	}
}

func TestHandlerCurrencyFallsBackToUSD(t *testing.T) {
	store, _, handler := newTestHandler(t)
	store.transactions = TransactionPage{Items: []Transaction{{
		WalletAddress: testAddress,
		TxHash:        "0xabc",
		Direction:     "credit",
		USDAmount:     "10",
	}}}

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/transactions?currency=ZAR"), "", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data []Transaction `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := body.Data[0].DisplayAmount; got != "10" {
		t.Fatalf("displayAmount = %q, want USD fallback 10", got)
	}
}

func TestHandlerRejectsInvalidCurrency(t *testing.T) {
	store, _, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances?currency=GBP"), "", authed(nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != "VALIDATION_ERROR" {
		t.Fatalf("code = %q", got)
	}
	if store.balanceCalls != 0 || store.settingsGetCalls != 0 {
		t.Fatal("store was called for an invalid currency")
	}
}

func TestHandlerSettingsMethodNotAllowed(t *testing.T) {
	_, _, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPost, "/v1/settings", "", authed(nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, PUT, OPTIONS" {
		t.Fatalf("Allow = %q", got)
	}
}

func TestHandlerCORS(t *testing.T) {
	t.Run("preflight allowed", func(t *testing.T) {
		_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
			cfg.AllowedOrigins = []string{"https://app.example"}
		})

		rec := doRequest(t, handler, http.MethodOptions, addresses("/v1/balances"), "", map[string]string{
			"Origin":                        "https://app.example",
			"Access-Control-Request-Method": "GET",
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
			t.Fatalf("allow origin = %q", got)
		}
		if !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
			t.Fatalf("allow headers = %q", rec.Header().Get("Access-Control-Allow-Headers"))
		}
	})

	t.Run("origin not allowed", func(t *testing.T) {
		_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
			cfg.AllowedOrigins = []string{"https://app.example"}
		})

		rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances"), "", authed(map[string]string{
			"Origin": "https://evil.example",
		}))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d", rec.Code)
		}
		if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != "ORIGIN_NOT_ALLOWED" {
			t.Fatalf("code = %q", got)
		}
	})

	t.Run("simple request gets headers", func(t *testing.T) {
		_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
			cfg.AllowedOrigins = []string{"https://app.example"}
		})

		rec := doRequest(t, handler, http.MethodGet, "/v1/health", "", map[string]string{
			"Origin": "https://app.example",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
			t.Fatalf("allow origin = %q", got)
		}
		if got := rec.Header().Get("Vary"); !strings.Contains(got, "Origin") {
			t.Fatalf("Vary = %q", got)
		}
	})

	t.Run("wildcard origin", func(t *testing.T) {
		_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
			cfg.AllowedOrigins = []string{"*"}
		})

		rec := doRequest(t, handler, http.MethodOptions, addresses("/v1/balances"), "", map[string]string{
			"Origin": "https://anything.example",
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://anything.example" {
			t.Fatalf("allow origin = %q", got)
		}
	})
}

func TestHandlerRequestIDEchoAndSecurityHeaders(t *testing.T) {
	_, _, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances"), "", map[string]string{
		"X-Request-Id": "client-req-1",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "client-req-1" {
		t.Fatalf("request id = %q", got)
	}
	if got := decodeError(t, rec.Body.Bytes()).Error.RequestID; got != "client-req-1" {
		t.Fatalf("body request id = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("content type options = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
}

func TestHandlerRejectsInvalidRequestID(t *testing.T) {
	_, _, handler := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/v1/health", "", map[string]string{
		"X-Request-Id": "bad id!",
	})
	if got := rec.Header().Get("X-Request-Id"); got != "req-fixed" {
		t.Fatalf("request id = %q, want generated id", got)
	}
}

func TestHandlerPanicRecovery(t *testing.T) {
	_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
		cfg.Store = panicStore{}
	})

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances"), "", authed(nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != "INTERNAL_ERROR" {
		t.Fatalf("code = %q", got)
	}
}

type panicStore struct{}

func (panicStore) UpsertWallet(context.Context, WalletInput) (Wallet, bool, error) {
	panic("boom")
}
func (panicStore) GetWalletSettings(context.Context, string) (WalletSettings, error) {
	panic("boom")
}
func (panicStore) UpsertWalletSettings(context.Context, WalletSettingsInput) (WalletSettings, error) {
	panic("boom")
}
func (panicStore) ListBalances(context.Context, string, *BalanceCursor, int) (BalancePage, error) {
	panic("boom")
}
func (panicStore) ListTransactions(context.Context, string, *TransactionCursor, int) (TransactionPage, error) {
	panic("boom")
}
func (panicStore) GetFXRates(context.Context) (FXRates, error) {
	panic("boom")
}
func (panicStore) CreatePOSPayment(context.Context, POSPayment) (POSPayment, error) {
	panic("boom")
}
func (panicStore) GetPOSPayment(context.Context, string) (POSPayment, error) {
	panic("boom")
}
func (panicStore) UpdatePOSPayment(context.Context, POSPayment) (POSPayment, error) {
	panic("boom")
}

// timeoutStore blocks until the request context expires so the timeout path is
// deterministic instead of depending on how fast the fake returns.
type timeoutStore struct {
	*fakeStore
}

func (s *timeoutStore) ListBalances(ctx context.Context, _ string, _ *BalanceCursor, _ int) (BalancePage, error) {
	<-ctx.Done()
	return BalancePage{}, ctx.Err()
}

func TestHandlerTimeoutContext(t *testing.T) {
	store := &timeoutStore{fakeStore: &fakeStore{}}
	_, _, handler := newTestHandler(t, func(cfg *HandlerConfig) {
		cfg.Store = store
		cfg.RequestTimeout = 10 * time.Millisecond
	})

	rec := doRequest(t, handler, http.MethodGet, addresses("/v1/balances"), "", authed(nil))
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := decodeError(t, rec.Body.Bytes()).Error.Code; got != "REQUEST_TIMEOUT" {
		t.Fatalf("code = %q", got)
	}
}
