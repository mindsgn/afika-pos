// Package posservice orchestrates the POS use cases on top of posstore and
// posdomain.
//
// It is transport agnostic: every method takes a business id (already resolved
// from the caller's identity) and returns typed values or a *Error that the HTTP
// layer can turn into a status code. All money is integer ZAR cents.
package posservice

import (
	"errors"
	"fmt"
)

// Kind classifies an error so the transport can pick a status code without
// string matching.
type Kind string

const (
	// KindInvalid is a client mistake in the request payload (400).
	KindInvalid Kind = "invalid"
	// KindNotFound is a resource that does not exist for this business (404).
	KindNotFound Kind = "not_found"
	// KindConflict is a state conflict, e.g. completing a failed sale (409).
	KindConflict Kind = "conflict"
	// KindInternal is an unexpected server or database failure (500).
	KindInternal Kind = "internal"
)

// Error is a POS use-case error. Code is stable and machine readable; Message is
// safe to show to a merchant.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap exposes the underlying cause for logging.
func (e *Error) Unwrap() error { return e.Err }

func invalidf(code, format string, args ...any) error {
	return &Error{Kind: KindInvalid, Code: code, Message: fmt.Sprintf(format, args...)}
}

func notFoundf(code, format string, args ...any) error {
	return &Error{Kind: KindNotFound, Code: code, Message: fmt.Sprintf(format, args...)}
}

func conflictf(code, format string, args ...any) error {
	return &Error{Kind: KindConflict, Code: code, Message: fmt.Sprintf(format, args...)}
}

func internalf(err error) error {
	return &Error{
		Kind:    KindInternal,
		Code:    "INTERNAL",
		Message: "Something went wrong. Please try again.",
		Err:     err,
	}
}

// KindOf reports the kind of err, or KindInternal for non-POS errors.
func KindOf(err error) Kind {
	var posErr *Error
	if errors.As(err, &posErr) {
		return posErr.Kind
	}
	return KindInternal
}

// CodeOf returns the stable code for err, or INTERNAL.
func CodeOf(err error) string {
	var posErr *Error
	if errors.As(err, &posErr) {
		return posErr.Code
	}
	return "INTERNAL"
}

// MessageOf returns the merchant-facing message for err. Unexpected errors are
// never echoed.
func MessageOf(err error) string {
	var posErr *Error
	if errors.As(err, &posErr) {
		return posErr.Message
	}
	return "Something went wrong. Please try again."
}

// Error codes. Keep these stable: clients branch on them.
const (
	CodeBusinessNotSetUp = "BUSINESS_NOT_SET_UP"
	CodeBusinessNotFound = "BUSINESS_NOT_FOUND"
	CodeInvalidBusiness  = "INVALID_BUSINESS"

	CodeCategoryExists   = "CATEGORY_EXISTS"
	CodeCategoryInUse    = "CATEGORY_IN_USE"
	CodeCategoryNotFound = "CATEGORY_NOT_FOUND"
	CodeInvalidName      = "INVALID_NAME"

	CodeProductNotFound   = "PRODUCT_NOT_FOUND"
	CodeProductInactive   = "PRODUCT_INACTIVE"
	CodeSKUExists         = "SKU_EXISTS"
	CodeInvalidProduct    = "INVALID_PRODUCT"
	CodeInvalidStockDelta = "INVALID_STOCK_DELTA"

	CodeCustomerNotFound  = "CUSTOMER_NOT_FOUND"
	CodeCustomerPhoneUsed = "CUSTOMER_PHONE_EXISTS"
	CodeCustomerRequired  = "CUSTOMER_REQUIRED"
	CodeInvalidCustomer   = "INVALID_CUSTOMER"

	CodeCartEmpty          = "CART_EMPTY"
	CodeCartTooLarge       = "CART_TOO_LARGE"
	CodeInvalidQuantity    = "INVALID_QUANTITY"
	CodeInvalidDiscount    = "INVALID_DISCOUNT"
	CodeInsufficientStock  = "INSUFFICIENT_STOCK"
	CodeInsufficientPoints = "INSUFFICIENT_POINTS"
	CodeInvalidPayment     = "INVALID_PAYMENT_METHOD"
	CodeInvalidCart        = "INVALID_CART"

	CodeSaleNotFound      = "SALE_NOT_FOUND"
	CodeSaleNotPending    = "SALE_NOT_PENDING"
	CodeSaleNotVoidable   = "SALE_NOT_VOIDABLE"
	CodeSaleAlreadyVoided = "SALE_ALREADY_VOIDED"
	CodeSaleNotRefundable = "SALE_NOT_REFUNDABLE"
	CodeRefundNotFound    = "REFUND_NOT_FOUND"

	CodeRefundEmpty        = "REFUND_EMPTY"
	CodeRefundDuplicate    = "REFUND_DUPLICATE_ITEM"
	CodeRefundItemNotFound = "REFUND_ITEM_NOT_IN_SALE"
	CodeRefundQtyExceeds   = "REFUND_QTY_EXCEEDS"
	CodeInvalidRefund      = "INVALID_REFUND"

	CodeIdempotencyKeyReused = "IDEMPOTENCY_KEY_REUSED"
	CodeInvalidIdempotency   = "INVALID_IDEMPOTENCY_KEY"
)
