package lib

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPageSize       = 50
	maxPageSize           = 100
	maxRequestBodyBytes   = 64 << 10
	defaultRequestTimeout = 10 * time.Second
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// HandlerConfig wires the API's dependencies. Zero values select production
// defaults, except Store and Verifier which have no safe default.
type HandlerConfig struct {
	Store      Store
	Verifier   TokenVerifier
	PrivyUsers PrivyUserReader

	// Passwordless and Sessions enable the server-rendered website sign-in.
	// When either is nil the /signin and /account pages report that sign-in is
	// not configured.
	Passwordless PasswordlessClient
	Sessions     *SessionCodec
	// InsecureSessionCookie drops the Secure flag from the session cookie. It
	// exists for the local Playwright server, which runs over plain HTTP.
	InsecureSessionCookie bool

	// AllowedOrigins enables CORS for the listed browser origins. "*" allows
	// any origin. An empty list disables CORS headers.
	AllowedOrigins []string

	// ReceivingAddress is the default wallet address for POS payments when the
	// app does not send one with the request.
	ReceivingAddress string
	// CheckoutBaseURL overrides the origin used in shareable checkout links.
	// When empty the request host is used.
	CheckoutBaseURL string

	RequestID      func() string
	Logger         *log.Logger
	RequestTimeout time.Duration
}

// Handler serves the versioned REST API.
type Handler struct {
	store            Store
	verifier         TokenVerifier
	privyUsers       PrivyUserReader
	passwordless     PasswordlessClient
	sessions         *SessionCodec
	insecureCookie   bool
	allowedOrigins   []string
	receivingAddress string
	checkoutBaseURL  string
	requestID        func() string
	logger           *log.Logger
	requestTimeout   time.Duration
}

// NewHandler builds an http.Handler for the API.
func NewHandler(cfg HandlerConfig) *Handler {
	h := &Handler{
		store:            cfg.Store,
		verifier:         cfg.Verifier,
		privyUsers:       cfg.PrivyUsers,
		passwordless:     cfg.Passwordless,
		sessions:         cfg.Sessions,
		insecureCookie:   cfg.InsecureSessionCookie,
		allowedOrigins:   cfg.AllowedOrigins,
		receivingAddress: strings.TrimSpace(cfg.ReceivingAddress),
		checkoutBaseURL:  strings.TrimRight(strings.TrimSpace(cfg.CheckoutBaseURL), "/"),
		requestID:        cfg.RequestID,
		logger:           cfg.Logger,
		requestTimeout:   cfg.RequestTimeout,
	}
	if h.requestID == nil {
		h.requestID = newRequestID
	}
	if h.logger == nil {
		h.logger = log.Default()
	}
	if h.requestTimeout <= 0 {
		h.requestTimeout = defaultRequestTimeout
	}
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.servePayPage(w, r) {
		return
	}
	if h.serveAuthSite(w, r) {
		return
	}
	if handled := serveSite(w, r); handled {
		return
	}
	requestID := h.requestIDFor(r)
	w.Header().Set("X-Request-Id", requestID)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	origin, originAllowed := h.corsOrigin(r)
	if !originAllowed {
		h.writeError(w, requestID, apiError{
			status:  http.StatusForbidden,
			code:    "ORIGIN_NOT_ALLOWED",
			message: "The request origin is not allowed.",
		})
		return
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id")
		w.Header().Set("Access-Control-Max-Age", "600")
	}
	if r.Method == http.MethodOptions && origin != "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	status, body, apiErr := h.route(w, r)
	if apiErr != nil {
		if apiErr.status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="afika-api"`)
		}
		if apiErr.allow != "" {
			w.Header().Set("Allow", apiErr.allow)
		}
		h.writeError(w, requestID, *apiErr)
		return
	}
	writeJSON(w, status, body)
}

func (h *Handler) route(w http.ResponseWriter, r *http.Request) (status int, body any, apiErr *apiError) {
	defer func() {
		if recovered := recover(); recovered != nil {
			h.logf("panic serving %s %s: %v", r.Method, r.URL.Path, recovered)
			status, body, apiErr = 0, nil, internalError()
		}
	}()

	switch r.URL.Path {
	case "/v1/health":
		if r.Method != http.MethodGet {
			return 0, nil, methodNotAllowed("GET, OPTIONS")
		}
		return http.StatusOK, healthEnvelope{Data: healthData{Status: "ok"}}, nil
	case "/v1/auth/me":
		if r.Method != http.MethodGet {
			return 0, nil, methodNotAllowed("GET, OPTIONS")
		}
		return h.getPrivyProfile(w, r)
	case "/v1/wallet":
		if r.Method != http.MethodPut {
			return 0, nil, methodNotAllowed("PUT, OPTIONS")
		}
		return h.upsertWallet(w, r)
	case "/v1/balances":
		if r.Method != http.MethodGet {
			return 0, nil, methodNotAllowed("GET, OPTIONS")
		}
		return h.listBalances(w, r)
	case "/v1/transactions":
		if r.Method != http.MethodGet {
			return 0, nil, methodNotAllowed("GET, OPTIONS")
		}
		return h.listTransactions(w, r)
	case "/v1/settings":
		switch r.Method {
		case http.MethodGet:
			return h.getSettings(w, r)
		case http.MethodPut:
			return h.updateSettings(w, r)
		default:
			return 0, nil, methodNotAllowed("GET, PUT, OPTIONS")
		}
	case "/v1/fx-rates":
		if r.Method != http.MethodGet {
			return 0, nil, methodNotAllowed("GET, OPTIONS")
		}
		return h.getFXRates(w, r)
	case "/v1/pos/payments":
		if r.Method != http.MethodPost {
			return 0, nil, methodNotAllowed("POST, OPTIONS")
		}
		return h.createPOSPayment(w, r)
	default:
		if path := r.URL.Path; strings.HasPrefix(path, "/v1/pos/payments/") {
			return h.routePOSPayment(w, r)
		}
		return 0, nil, &apiError{
			status:  http.StatusNotFound,
			code:    "NOT_FOUND",
			message: "Route not found.",
		}
	}
}

// routePOSPayment dispatches the per-payment POS routes. Reading a payment is
// public: the id is an unguessable token handed to the payer, and the response
// carries no secrets.
func (h *Handler) routePOSPayment(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	path := strings.TrimRight(r.URL.Path, "/")
	switch {
	case strings.HasSuffix(path, "/asset"):
		if r.Method != http.MethodPost {
			return 0, nil, methodNotAllowed("POST, OPTIONS")
		}
		return h.selectPOSAsset(w, r)
	case strings.HasSuffix(path, "/confirm"):
		if r.Method != http.MethodPost {
			return 0, nil, methodNotAllowed("POST, OPTIONS")
		}
		return h.confirmPOSPayment(w, r)
	default:
		if r.Method != http.MethodGet {
			return 0, nil, methodNotAllowed("GET, OPTIONS")
		}
		return h.getPOSPayment(w, r)
	}
}

func (h *Handler) getFXRates(_ http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	if authErr := h.authenticate(r); authErr != nil {
		return 0, nil, authErr
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	rates, err := h.store.GetFXRates(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, requestTimeout()
		}
		h.logf("get fx rates: %v", err)
		return 0, nil, internalError()
	}
	return http.StatusOK, fxRatesEnvelope{Data: rates}, nil
}

func (h *Handler) upsertWallet(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	if authErr := h.authenticate(r); authErr != nil {
		return 0, nil, authErr
	}

	var request upsertWalletRequest
	if apiErr := decodeJSONBody(w, r, &request); apiErr != nil {
		return 0, nil, apiErr
	}

	address, err := normalizeAddress(request.Address)
	if err != nil {
		return 0, nil, validation(err.Error())
	}
	network, testnet, err := normalizeNetwork(request.Network)
	if err != nil {
		return 0, nil, validation(err.Error())
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	wallet, created, err := h.store.UpsertWallet(ctx, WalletInput{
		Address: address,
		Network: network,
		Testnet: testnet,
	})
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, requestTimeout()
		}
		h.logf("upsert wallet: %v", err)
		return 0, nil, internalError()
	}
	if ctx.Err() != nil {
		return 0, nil, requestTimeout()
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return status, walletEnvelope{Data: wallet}, nil
}

// getSettings returns a wallet's saved display preference. Wallets without a
// row get the USD default, so the app can load the screen without an upsert.
func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	if authErr := h.authenticate(r); authErr != nil {
		return 0, nil, authErr
	}

	address, apiErr := parseAddressQuery(r)
	if apiErr != nil {
		return 0, nil, apiErr
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	settings, err := h.store.GetWalletSettings(ctx, address)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, requestTimeout()
		}
		h.logf("get wallet settings: %v", err)
		return 0, nil, internalError()
	}
	if ctx.Err() != nil {
		return 0, nil, requestTimeout()
	}
	if settings.Address == "" {
		settings.Address = address
	}
	if _, err := normalizeCurrency(settings.PreferredCurrency); err != nil {
		settings.PreferredCurrency = defaultCurrency
	}
	return http.StatusOK, settingsEnvelope{Data: settings}, nil
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	if authErr := h.authenticate(r); authErr != nil {
		return 0, nil, authErr
	}

	var request upsertSettingsRequest
	if apiErr := decodeJSONBody(w, r, &request); apiErr != nil {
		return 0, nil, apiErr
	}
	address, err := normalizeAddress(request.Address)
	if err != nil {
		return 0, nil, validation(err.Error())
	}
	currency, err := normalizeCurrency(request.PreferredCurrency)
	if err != nil {
		return 0, nil, validation(err.Error())
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	settings, err := h.store.UpsertWalletSettings(ctx, WalletSettingsInput{
		Address:           address,
		PreferredCurrency: currency,
	})
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, requestTimeout()
		}
		h.logf("upsert wallet settings: %v", err)
		return 0, nil, internalError()
	}
	if ctx.Err() != nil {
		return 0, nil, requestTimeout()
	}
	return http.StatusOK, settingsEnvelope{Data: settings}, nil
}

func (h *Handler) listBalances(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	if authErr := h.authenticate(r); authErr != nil {
		return 0, nil, authErr
	}

	address, limit, rawCursor, rawCurrency, apiErr := parsePageQuery(r)
	if apiErr != nil {
		return 0, nil, apiErr
	}
	var after *BalanceCursor
	if rawCursor != "" {
		cursor, err := DecodeBalanceCursor(rawCursor)
		if err != nil {
			return 0, nil, invalidCursor()
		}
		after = &cursor
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	currency, apiErr := h.resolveCurrency(ctx, address, rawCurrency)
	if apiErr != nil {
		return 0, nil, apiErr
	}

	page, err := h.store.ListBalances(ctx, address, after, limit)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, requestTimeout()
		}
		h.logf("list balances: %v", err)
		return 0, nil, internalError()
	}
	if ctx.Err() != nil {
		return 0, nil, requestTimeout()
	}

	items := page.Items
	if items == nil {
		items = []Balance{}
	}
	for i := range items {
		items[i].DisplayCurrency = currency
		items[i].DisplayAmount = displayAmount(currency, items[i].USDAmount, items[i].ZARAmount, items[i].EURAmount)
	}
	response := pageEnvelope{
		Data:       items,
		Pagination: Pagination{Limit: limit, HasMore: page.HasMore},
		Currency:   currency,
	}
	if page.HasMore && len(items) > 0 {
		last := items[len(items)-1]
		response.Pagination.NextCursor = EncodeBalanceCursor(BalanceCursor{
			Network:     last.Network,
			TokenSymbol: last.TokenSymbol,
		})
	}
	return http.StatusOK, response, nil
}

func (h *Handler) listTransactions(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	if authErr := h.authenticate(r); authErr != nil {
		return 0, nil, authErr
	}

	address, limit, rawCursor, rawCurrency, apiErr := parsePageQuery(r)
	if apiErr != nil {
		return 0, nil, apiErr
	}
	var after *TransactionCursor
	if rawCursor != "" {
		cursor, err := DecodeTransactionCursor(rawCursor)
		if err != nil {
			return 0, nil, invalidCursor()
		}
		after = &cursor
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	currency, apiErr := h.resolveCurrency(ctx, address, rawCurrency)
	if apiErr != nil {
		return 0, nil, apiErr
	}

	page, err := h.store.ListTransactions(ctx, address, after, limit)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, requestTimeout()
		}
		h.logf("list transactions: %v", err)
		return 0, nil, internalError()
	}
	if ctx.Err() != nil {
		return 0, nil, requestTimeout()
	}

	items := page.Items
	if items == nil {
		items = []Transaction{}
	}
	for i := range items {
		items[i].DisplayCurrency = currency
		items[i].DisplayAmount = displayAmount(currency, items[i].USDAmount, items[i].ZARAmount, items[i].EURAmount)
		items[i].DisplayFee = displayAmount(currency, items[i].FeeUSD, items[i].FeeZAR, items[i].FeeEUR)
	}
	response := pageEnvelope{
		Data:       items,
		Pagination: Pagination{Limit: limit, HasMore: page.HasMore},
		Currency:   currency,
	}
	if page.HasMore && len(items) > 0 {
		last := items[len(items)-1]
		response.Pagination.NextCursor = EncodeTransactionCursor(TransactionCursor{
			TimestampMs: last.TimestampMs,
			TxHash:      last.TxHash,
			Direction:   last.Direction,
		})
	}
	return http.StatusOK, response, nil
}

// authenticate requires the shared API token. Wallet lookups are addressed by
// the address query parameter, so the token carries no user identity.
func (h *Handler) authenticate(r *http.Request) *apiError {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok || h.verifier == nil {
		return unauthenticated()
	}
	if err := h.verifier.Verify(token); err != nil {
		return unauthenticated()
	}
	return nil
}

func (h *Handler) corsOrigin(r *http.Request) (string, bool) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || len(h.allowedOrigins) == 0 {
		return "", true
	}
	for _, allowed := range h.allowedOrigins {
		if allowed == "*" || strings.EqualFold(strings.TrimSpace(allowed), origin) {
			return origin, true
		}
	}
	return "", false
}

func (h *Handler) requestIDFor(r *http.Request) string {
	if candidate := strings.TrimSpace(r.Header.Get("X-Request-Id")); requestIDPattern.MatchString(candidate) {
		return candidate
	}
	return h.requestID()
}

func (h *Handler) writeError(w http.ResponseWriter, requestID string, apiErr apiError) {
	writeJSON(w, apiErr.status, errorEnvelope{
		Error: errorDetail{
			Code:      apiErr.code,
			Message:   apiErr.message,
			RequestID: requestID,
		},
	})
}

func (h *Handler) logf(format string, args ...any) {
	if h.logger != nil {
		h.logger.Printf(format, args...)
	}
}

type apiError struct {
	status  int
	code    string
	message string
	allow   string
}

func unauthenticated() *apiError {
	return &apiError{
		status:  http.StatusUnauthorized,
		code:    "UNAUTHENTICATED",
		message: "A valid API token is required.",
	}
}

func internalError() *apiError {
	return &apiError{
		status:  http.StatusInternalServerError,
		code:    "INTERNAL_ERROR",
		message: "An unexpected error occurred.",
	}
}

func requestTimeout() *apiError {
	return &apiError{
		status:  http.StatusGatewayTimeout,
		code:    "REQUEST_TIMEOUT",
		message: "The request timed out. Try again.",
	}
}

func validation(message string) *apiError {
	return &apiError{
		status:  http.StatusBadRequest,
		code:    "VALIDATION_ERROR",
		message: message,
	}
}

func invalidJSON() *apiError {
	return &apiError{
		status:  http.StatusBadRequest,
		code:    "INVALID_JSON",
		message: "The request body must be a single valid JSON object.",
	}
}

func invalidCursor() *apiError {
	return &apiError{
		status:  http.StatusBadRequest,
		code:    "INVALID_CURSOR",
		message: "The pagination cursor is invalid or does not belong to this endpoint.",
	}
}

func unsupportedMediaType() *apiError {
	return &apiError{
		status:  http.StatusUnsupportedMediaType,
		code:    "UNSUPPORTED_MEDIA_TYPE",
		message: "Content-Type must be application/json.",
	}
}

func payloadTooLarge() *apiError {
	return &apiError{
		status:  http.StatusRequestEntityTooLarge,
		code:    "PAYLOAD_TOO_LARGE",
		message: "The request body is too large.",
	}
}

func methodNotAllowed(allow string) *apiError {
	return &apiError{
		status:  http.StatusMethodNotAllowed,
		code:    "METHOD_NOT_ALLOWED",
		message: "The method is not allowed for this route.",
		allow:   allow,
	}
}

// parsePageQuery validates the address, limit, cursor, and optional currency
// parameters shared by the list endpoints. The address is required and
// normalized to lowercase.
func parsePageQuery(r *http.Request) (string, int, string, string, *apiError) {
	query := r.URL.Query()
	for key := range query {
		if key != "address" && key != "limit" && key != "cursor" && key != "currency" {
			return "", 0, "", "", validation(fmt.Sprintf("Unknown query parameter %q.", key))
		}
	}

	values, ok := query["address"]
	if !ok {
		return "", 0, "", "", validation("address is required.")
	}
	if len(values) != 1 {
		return "", 0, "", "", validation("address must be provided once.")
	}
	address, err := normalizeAddress(values[0])
	if err != nil {
		return "", 0, "", "", validation(err.Error())
	}

	limit := defaultPageSize
	if values, ok := query["limit"]; ok {
		if len(values) != 1 {
			return "", 0, "", "", validation("limit must be provided once.")
		}
		parsed, err := strconv.Atoi(values[0])
		if err != nil || parsed < 1 || parsed > maxPageSize {
			return "", 0, "", "", validation(fmt.Sprintf("limit must be an integer between 1 and %d.", maxPageSize))
		}
		limit = parsed
	}

	cursor := ""
	if values, ok := query["cursor"]; ok {
		if len(values) != 1 {
			return "", 0, "", "", validation("cursor must be provided once.")
		}
		cursor = values[0]
	}

	currency := ""
	if values, ok := query["currency"]; ok {
		if len(values) != 1 {
			return "", 0, "", "", validation("currency must be provided once.")
		}
		currency, err = normalizeCurrency(values[0])
		if err != nil {
			return "", 0, "", "", validation(err.Error())
		}
	}
	return address, limit, cursor, currency, nil
}

// parseAddressQuery validates the address parameter used by the settings
// endpoints.
func parseAddressQuery(r *http.Request) (string, *apiError) {
	query := r.URL.Query()
	for key := range query {
		if key != "address" {
			return "", validation(fmt.Sprintf("Unknown query parameter %q.", key))
		}
	}
	values, ok := query["address"]
	if !ok {
		return "", validation("address is required.")
	}
	if len(values) != 1 {
		return "", validation("address must be provided once.")
	}
	address, err := normalizeAddress(values[0])
	if err != nil {
		return "", validation(err.Error())
	}
	return address, nil
}

// resolveCurrency returns the request's explicit currency or the wallet's saved
// preference, defaulting to USD.
func (h *Handler) resolveCurrency(ctx context.Context, address, requested string) (string, *apiError) {
	if requested != "" {
		return requested, nil
	}

	settings, err := h.store.GetWalletSettings(ctx, address)
	if err != nil {
		if ctx.Err() != nil {
			return "", requestTimeout()
		}
		h.logf("resolve currency: %v", err)
		return "", internalError()
	}
	currency, err := normalizeCurrency(settings.PreferredCurrency)
	if err != nil {
		return defaultCurrency, nil
	}
	return currency, nil
}

// displayAmount picks the fiat amount for the active currency. Missing
// converted values fall back to USD rather than showing a blank row.
func displayAmount(currency, usd, zar, eur string) string {
	var value string
	switch currency {
	case "ZAR":
		value = zar
	case "EUR":
		value = eur
	default:
		value = usd
	}
	if strings.TrimSpace(value) != "" {
		return value
	}
	return strings.TrimSpace(usd)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, destination any) *apiError {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return unsupportedMediaType()
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return unsupportedMediaType()
	}

	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return payloadTooLarge()
		}
		var typeError *json.UnmarshalTypeError
		if errors.As(err, &typeError) {
			return validation("The request body has a field with the wrong type.")
		}
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return validation(strings.TrimPrefix(err.Error(), "json: "))
		}
		return invalidJSON()
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return invalidJSON()
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func newRequestID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "request"
	}
	return hex.EncodeToString(buf[:])
}

type upsertWalletRequest struct {
	Address string `json:"address"`
	Network string `json:"network"`
}

type upsertSettingsRequest struct {
	Address           string `json:"address"`
	PreferredCurrency string `json:"preferredCurrency"`
}

type healthData struct {
	Status string `json:"status"`
}

type healthEnvelope struct {
	Data healthData `json:"data"`
}

type walletEnvelope struct {
	Data Wallet `json:"data"`
}

type settingsEnvelope struct {
	Data WalletSettings `json:"data"`
}

type fxRatesEnvelope struct {
	Data FXRates `json:"data"`
}

// Pagination is the envelope every list endpoint returns.
type Pagination struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type pageEnvelope struct {
	Data       any        `json:"data"`
	Pagination Pagination `json:"pagination"`
	Currency   string     `json:"currency,omitempty"`
}

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}
