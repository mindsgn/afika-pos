package lib

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/mindsgn-studio/afika-backend/api/internal/posservice"
	"github.com/mindsgn-studio/afika-backend/api/internal/posstore"
)

// sharedTokenOwner scopes POS data when the caller authenticates with the
// legacy shared API token instead of a Privy session. A shared token is a
// single merchant, so all of its devices see the same business.
const sharedTokenOwner = "token:shared"

// posRoutes are the POS endpoints served under /v1/pos (except payments, which
// keep their existing routes).
type posEnvelope struct {
	Data any `json:"data"`
}

type posListMeta struct {
	Count int `json:"count"`
}

type posListEnvelope struct {
	Data any         `json:"data"`
	Meta posListMeta `json:"meta"`
}

type posReceiptData struct {
	Receipt string `json:"receipt"`
}

// routePOS dispatches /v1/pos/** to the POS use cases. It is a manual router so
// the Worker keeps a single routing table and no extra dependency.
func (h *Handler) routePOS(w http.ResponseWriter, r *http.Request) (int, any, *apiError) {
	if h.pos == nil {
		return 0, nil, posNotConfigured()
	}

	owner, authErr := h.posOwner(r)
	if authErr != nil {
		return 0, nil, authErr
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/pos"), "/")
	segments := []string{}
	if rest != "" {
		segments = strings.Split(rest, "/")
	}
	if len(segments) == 0 {
		return 0, nil, notFound()
	}

	switch segments[0] {
	case "business":
		return h.routePOSBusiness(ctx, w, r, owner, segments[1:])
	case "categories":
		return h.routePOSCategories(ctx, w, r, owner, segments[1:])
	case "products":
		return h.routePOSProducts(ctx, w, r, owner, segments[1:])
	case "inventory":
		return h.routePOSInventory(ctx, w, r, owner, segments[1:])
	case "customers":
		return h.routePOSCustomers(ctx, w, r, owner, segments[1:])
	case "sales":
		return h.routePOSSales(ctx, w, r, owner, segments[1:])
	case "reports":
		return h.routePOSReports(ctx, w, r, owner, segments[1:])
	default:
		return 0, nil, notFound()
	}
}

// posOwner verifies the bearer token and resolves the business owner. Privy
// sessions use the DID; the shared token maps to one fixed owner.
func (h *Handler) posOwner(r *http.Request) (string, *apiError) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok || h.verifier == nil {
		return "", unauthenticated()
	}
	if err := h.verifier.Verify(token); err != nil {
		return "", unauthenticated()
	}
	if subjectVerifier, ok := h.verifier.(interface{ Subject(string) (string, error) }); ok {
		if subject, err := subjectVerifier.Subject(token); err == nil && strings.HasPrefix(subject, "did:privy:") {
			return subject, nil
		}
	}
	return sharedTokenOwner, nil
}

// requireBusiness resolves the caller's merchant profile for every endpoint
// that reads or writes business data.
func (h *Handler) requireBusiness(ctx context.Context, owner string) (posstore.Business, *apiError) {
	business, err := h.pos.RequireBusiness(ctx, owner)
	if err != nil {
		return posstore.Business{}, h.posError(err)
	}
	return business, nil
}

// ---------------------------------------------------------------------------
// Business
// ---------------------------------------------------------------------------

func (h *Handler) routePOSBusiness(ctx context.Context, w http.ResponseWriter, r *http.Request, owner string, rest []string) (int, any, *apiError) {
	if len(rest) != 0 {
		return 0, nil, notFound()
	}
	switch r.Method {
	case http.MethodGet:
		business, err := h.pos.Business(ctx, owner)
		if err != nil {
			return 0, nil, h.posError(err)
		}
		if business == nil {
			return http.StatusOK, posEnvelope{Data: nil}, nil
		}
		return http.StatusOK, posEnvelope{Data: business}, nil
	case http.MethodPut:
		var input posservice.BusinessInput
		if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
			return 0, nil, apiErr
		}
		business, err := h.pos.SaveBusiness(ctx, owner, input)
		if err != nil {
			return 0, nil, h.posError(err)
		}
		return http.StatusOK, posEnvelope{Data: business}, nil
	default:
		return 0, nil, methodNotAllowed("GET, PUT, OPTIONS")
	}
}

// ---------------------------------------------------------------------------
// Categories
// ---------------------------------------------------------------------------

func (h *Handler) routePOSCategories(ctx context.Context, w http.ResponseWriter, r *http.Request, owner string, rest []string) (int, any, *apiError) {
	business, apiErr := h.requireBusiness(ctx, owner)
	if apiErr != nil {
		return 0, nil, apiErr
	}

	if len(rest) == 0 {
		switch r.Method {
		case http.MethodGet:
			categories, err := h.pos.ListCategories(ctx, business.ID)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posListEnvelope{Data: categories, Meta: posListMeta{Count: len(categories)}}, nil
		case http.MethodPost:
			var input struct {
				Name string `json:"name"`
			}
			if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			category, err := h.pos.CreateCategory(ctx, business.ID, input.Name)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusCreated, posEnvelope{Data: category}, nil
		default:
			return 0, nil, methodNotAllowed("GET, POST, OPTIONS")
		}
	}

	if len(rest) == 1 {
		switch r.Method {
		case http.MethodPut:
			var input struct {
				Name string `json:"name"`
			}
			if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			category, err := h.pos.RenameCategory(ctx, business.ID, rest[0], input.Name)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: category}, nil
		case http.MethodDelete:
			if err := h.pos.DeleteCategory(ctx, business.ID, rest[0]); err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusNoContent, nil, nil
		default:
			return 0, nil, methodNotAllowed("PUT, DELETE, OPTIONS")
		}
	}
	return 0, nil, notFound()
}

// ---------------------------------------------------------------------------
// Products and inventory
// ---------------------------------------------------------------------------

func (h *Handler) routePOSProducts(ctx context.Context, w http.ResponseWriter, r *http.Request, owner string, rest []string) (int, any, *apiError) {
	business, apiErr := h.requireBusiness(ctx, owner)
	if apiErr != nil {
		return 0, nil, apiErr
	}

	if len(rest) == 0 {
		switch r.Method {
		case http.MethodGet:
			filter := posservice.ProductFilter{
				Query:           strings.TrimSpace(r.URL.Query().Get("query")),
				CategoryID:      strings.TrimSpace(r.URL.Query().Get("categoryId")),
				IncludeInactive: queryBool(r, "includeInactive"),
				LowStockOnly:    queryBool(r, "lowStock"),
			}
			limit, apiErr := queryInt(r, "limit")
			if apiErr != nil {
				return 0, nil, apiErr
			}
			filter.Limit = int(limit)
			products, err := h.pos.ListProducts(ctx, business.ID, filter)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posListEnvelope{Data: products, Meta: posListMeta{Count: len(products)}}, nil
		case http.MethodPost:
			var input posservice.ProductInput
			if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			product, err := h.pos.CreateProduct(ctx, business.ID, input)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusCreated, posEnvelope{Data: product}, nil
		default:
			return 0, nil, methodNotAllowed("GET, POST, OPTIONS")
		}
	}

	if len(rest) == 1 {
		switch r.Method {
		case http.MethodGet:
			product, err := h.pos.GetProduct(ctx, business.ID, rest[0])
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: product}, nil
		case http.MethodPut:
			var input posservice.ProductInput
			if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			product, err := h.pos.UpdateProduct(ctx, business.ID, rest[0], input)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: product}, nil
		default:
			return 0, nil, methodNotAllowed("GET, PUT, OPTIONS")
		}
	}

	if len(rest) == 2 {
		switch rest[1] {
		case "active":
			if r.Method != http.MethodPost {
				return 0, nil, methodNotAllowed("POST, OPTIONS")
			}
			var input struct {
				Active bool `json:"active"`
			}
			if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			product, err := h.pos.SetProductActive(ctx, business.ID, rest[0], input.Active)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: product}, nil
		case "stock":
			if r.Method != http.MethodPost {
				return 0, nil, methodNotAllowed("POST, OPTIONS")
			}
			var input posservice.AdjustStockInput
			if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			product, err := h.pos.AdjustStock(ctx, business.ID, rest[0], input)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: product}, nil
		}
	}
	return 0, nil, notFound()
}

func (h *Handler) routePOSInventory(ctx context.Context, w http.ResponseWriter, r *http.Request, owner string, rest []string) (int, any, *apiError) {
	if r.Method != http.MethodGet {
		return 0, nil, methodNotAllowed("GET, OPTIONS")
	}
	business, apiErr := h.requireBusiness(ctx, owner)
	if apiErr != nil {
		return 0, nil, apiErr
	}

	switch {
	case len(rest) == 1 && rest[0] == "movements":
		limit, apiErr := queryInt(r, "limit")
		if apiErr != nil {
			return 0, nil, apiErr
		}
		movements, err := h.pos.ListMovements(ctx, business.ID, strings.TrimSpace(r.URL.Query().Get("productId")), int(limit))
		if err != nil {
			return 0, nil, h.posError(err)
		}
		return http.StatusOK, posListEnvelope{Data: movements, Meta: posListMeta{Count: len(movements)}}, nil
	case len(rest) == 1 && rest[0] == "low-stock":
		products, err := h.pos.LowStock(ctx, business.ID)
		if err != nil {
			return 0, nil, h.posError(err)
		}
		return http.StatusOK, posListEnvelope{Data: products, Meta: posListMeta{Count: len(products)}}, nil
	case len(rest) == 1 && rest[0] == "valuation":
		value, err := h.pos.Valuation(ctx, business.ID)
		if err != nil {
			return 0, nil, h.posError(err)
		}
		return http.StatusOK, posEnvelope{Data: value}, nil
	case len(rest) == 1 && rest[0] == "cost-history":
		productID := strings.TrimSpace(r.URL.Query().Get("productId"))
		if productID == "" {
			return 0, nil, validation("productId is required.")
		}
		movements, err := h.pos.CostHistory(ctx, business.ID, productID)
		if err != nil {
			return 0, nil, h.posError(err)
		}
		return http.StatusOK, posListEnvelope{Data: movements, Meta: posListMeta{Count: len(movements)}}, nil
	default:
		return 0, nil, notFound()
	}
}

// ---------------------------------------------------------------------------
// Customers
// ---------------------------------------------------------------------------

func (h *Handler) routePOSCustomers(ctx context.Context, w http.ResponseWriter, r *http.Request, owner string, rest []string) (int, any, *apiError) {
	business, apiErr := h.requireBusiness(ctx, owner)
	if apiErr != nil {
		return 0, nil, apiErr
	}

	if len(rest) == 0 {
		switch r.Method {
		case http.MethodGet:
			customers, err := h.pos.ListCustomers(ctx, business.ID, r.URL.Query().Get("query"))
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posListEnvelope{Data: customers, Meta: posListMeta{Count: len(customers)}}, nil
		case http.MethodPost:
			var input struct {
				Name  string `json:"name"`
				Phone string `json:"phone"`
			}
			if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			customer, err := h.pos.CreateCustomer(ctx, business.ID, input.Name, input.Phone)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusCreated, posEnvelope{Data: customer}, nil
		default:
			return 0, nil, methodNotAllowed("GET, POST, OPTIONS")
		}
	}

	if len(rest) == 1 {
		switch r.Method {
		case http.MethodGet:
			customer, err := h.pos.GetCustomer(ctx, business.ID, rest[0])
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: customer}, nil
		case http.MethodPut:
			var input struct {
				Name  string `json:"name"`
				Phone string `json:"phone"`
			}
			if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			customer, err := h.pos.UpdateCustomer(ctx, business.ID, rest[0], input.Name, input.Phone)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: customer}, nil
		default:
			return 0, nil, methodNotAllowed("GET, PUT, OPTIONS")
		}
	}
	return 0, nil, notFound()
}

// ---------------------------------------------------------------------------
// Sales and refunds
// ---------------------------------------------------------------------------

func (h *Handler) routePOSSales(ctx context.Context, w http.ResponseWriter, r *http.Request, owner string, rest []string) (int, any, *apiError) {
	business, apiErr := h.requireBusiness(ctx, owner)
	if apiErr != nil {
		return 0, nil, apiErr
	}

	if len(rest) == 0 {
		switch r.Method {
		case http.MethodGet:
			limit, apiErr := queryInt(r, "limit")
			if apiErr != nil {
				return 0, nil, apiErr
			}
			sales, err := h.pos.ListSales(ctx, business.ID, posstore.SaleFilter{
				Status: strings.TrimSpace(r.URL.Query().Get("status")),
				Query:  strings.TrimSpace(r.URL.Query().Get("query")),
				Limit:  int(limit),
			})
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posListEnvelope{Data: sales, Meta: posListMeta{Count: len(sales)}}, nil
		case http.MethodPost:
			var input posservice.CreateSaleInput
			if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			input.IdempotencyKey = idempotencyKey(r, input.IdempotencyKey)
			detail, err := h.pos.CreateSale(ctx, business.ID, input)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusCreated, posEnvelope{Data: detail}, nil
		default:
			return 0, nil, methodNotAllowed("GET, POST, OPTIONS")
		}
	}

	saleID := rest[0]
	if len(rest) == 1 {
		if r.Method != http.MethodGet {
			return 0, nil, methodNotAllowed("GET, OPTIONS")
		}
		detail, err := h.pos.GetSale(ctx, business.ID, saleID)
		if err != nil {
			return 0, nil, h.posError(err)
		}
		return http.StatusOK, posEnvelope{Data: detail}, nil
	}

	if len(rest) == 2 {
		switch rest[1] {
		case "complete":
			if r.Method != http.MethodPost {
				return 0, nil, methodNotAllowed("POST, OPTIONS")
			}
			var input posservice.CompleteSaleInput
			if apiErr := decodeOptionalJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			detail, err := h.pos.CompleteSale(ctx, business.ID, saleID, input)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: detail}, nil
		case "fail":
			if r.Method != http.MethodPost {
				return 0, nil, methodNotAllowed("POST, OPTIONS")
			}
			var input struct {
				Reason string `json:"reason"`
			}
			if apiErr := decodeOptionalJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			detail, err := h.pos.FailSale(ctx, business.ID, saleID, input.Reason)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: detail}, nil
		case "void":
			if r.Method != http.MethodPost {
				return 0, nil, methodNotAllowed("POST, OPTIONS")
			}
			var input struct {
				Reason string `json:"reason"`
			}
			if apiErr := decodeOptionalJSONBody(w, r, &input); apiErr != nil {
				return 0, nil, apiErr
			}
			detail, err := h.pos.VoidSale(ctx, business.ID, saleID, input.Reason)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: detail}, nil
		case "receipt":
			if r.Method != http.MethodGet {
				return 0, nil, methodNotAllowed("GET, OPTIONS")
			}
			receipt, err := h.pos.SaleReceipt(ctx, business.ID, saleID)
			if err != nil {
				return 0, nil, h.posError(err)
			}
			return http.StatusOK, posEnvelope{Data: posReceiptData{Receipt: receipt}}, nil
		case "refunds":
			switch r.Method {
			case http.MethodGet:
				detail, err := h.pos.GetSale(ctx, business.ID, saleID)
				if err != nil {
					return 0, nil, h.posError(err)
				}
				return http.StatusOK, posListEnvelope{Data: detail.Refunds, Meta: posListMeta{Count: len(detail.Refunds)}}, nil
			case http.MethodPost:
				var input posservice.CreateRefundInput
				if apiErr := decodeJSONBody(w, r, &input); apiErr != nil {
					return 0, nil, apiErr
				}
				input.IdempotencyKey = idempotencyKey(r, input.IdempotencyKey)
				result, err := h.pos.CreateRefund(ctx, business.ID, saleID, input)
				if err != nil {
					return 0, nil, h.posError(err)
				}
				return http.StatusCreated, posEnvelope{Data: result}, nil
			default:
				return 0, nil, methodNotAllowed("GET, POST, OPTIONS")
			}
		}
	}

	if len(rest) == 4 && rest[1] == "refunds" && rest[3] == "receipt" {
		if r.Method != http.MethodGet {
			return 0, nil, methodNotAllowed("GET, OPTIONS")
		}
		receipt, err := h.pos.RefundReceipt(ctx, business.ID, saleID, rest[2])
		if err != nil {
			return 0, nil, h.posError(err)
		}
		return http.StatusOK, posEnvelope{Data: posReceiptData{Receipt: receipt}}, nil
	}
	return 0, nil, notFound()
}

// ---------------------------------------------------------------------------
// Reports
// ---------------------------------------------------------------------------

func (h *Handler) routePOSReports(ctx context.Context, w http.ResponseWriter, r *http.Request, owner string, rest []string) (int, any, *apiError) {
	if len(rest) != 1 || rest[0] != "profit" {
		return 0, nil, notFound()
	}
	if r.Method != http.MethodGet {
		return 0, nil, methodNotAllowed("GET, OPTIONS")
	}
	business, apiErr := h.requireBusiness(ctx, owner)
	if apiErr != nil {
		return 0, nil, apiErr
	}
	from, apiErr := queryInt(r, "from")
	if apiErr != nil {
		return 0, nil, apiErr
	}
	to, apiErr := queryInt(r, "to")
	if apiErr != nil {
		return 0, nil, apiErr
	}
	report, err := h.pos.Profit(ctx, business.ID, from, to)
	if err != nil {
		return 0, nil, h.posError(err)
	}
	return http.StatusOK, posEnvelope{Data: report}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// posNotConfigured reports that the deployment did not wire the POS service.
func posNotConfigured() *apiError {
	return &apiError{
		status:  http.StatusServiceUnavailable,
		code:    "POS_NOT_CONFIGURED",
		message: "The POS API is not configured on this deployment.",
	}
}

// posError maps a use-case error onto the API's error envelope.
func (h *Handler) posError(err error) *apiError {
	code := posservice.CodeOf(err)
	message := posservice.MessageOf(err)
	switch posservice.KindOf(err) {
	case posservice.KindInvalid:
		return &apiError{status: http.StatusBadRequest, code: code, message: message}
	case posservice.KindNotFound:
		return &apiError{status: http.StatusNotFound, code: code, message: message}
	case posservice.KindConflict:
		return &apiError{status: http.StatusConflict, code: code, message: message}
	default:
		h.logf("pos request failed: %v", err)
		return internalError()
	}
}

func idempotencyKey(r *http.Request, body string) string {
	if value := strings.TrimSpace(body); value != "" {
		return value
	}
	return strings.TrimSpace(r.Header.Get("Idempotency-Key"))
}

func queryBool(r *http.Request, name string) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(name)))
	return value == "1" || value == "true" || value == "yes"
}

func queryInt(r *http.Request, name string) (int64, *apiError) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, validation(name + " must be a non-negative integer.")
	}
	return value, nil
}

// decodeOptionalJSONBody allows POSTs that take no body (confirm, fail, void).
func decodeOptionalJSONBody(w http.ResponseWriter, r *http.Request, destination any) *apiError {
	if r.ContentLength == 0 {
		return nil
	}
	return decodeJSONBody(w, r, destination)
}
