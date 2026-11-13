# Testing

The POS backend is covered at four levels: pure rules, use cases, persistence,
and HTTP. The existing app and Worker tests are unchanged and must keep passing.

## Commands

Run from the repository root unless noted.

```bash
# All Go tests (POS + legacy wallet/API)
(cd backend/api && go test ./...)

# Race detector (concurrency invariants)
(cd backend/api && go test -race ./...)

# Static checks
(cd backend/api && go vet ./...)
(cd backend/api && gofmt -l .)

# Production build target
(cd backend/api && GOOS=js GOARCH=wasm go build ./...)

# Real Wrangler local D1: migrations, schema, constraints, guard behaviour
bash backend/scripts/test-database.sh

# Browser end-to-end (checkout page + sign-in), starts wrangler dev
(cd backend/api && npx playwright test)

# App: domain and payment service tests, plus typecheck
(cd app && yarn test)
(cd app && yarn typecheck)

# App: Maestro device flows (onboarding, add product, sell)
(cd app && yarn e2e)
```

`backend/scripts/test-database.sh` applies every worker's migrations to one
temporary local D1 database and never touches the remote database or requires a
Cloudflare login.

## Suites

152 Go tests in total: 56 for POS internals and 96 for the existing API
surface (including 8 new POS HTTP integration tests).

### `internal/posdomain` — pure rules

No SQLite, no HTTP. These tests pin the business rules:

| Test | Invariant |
| --- | --- |
| `TestAllocateSumsExactly`, `TestAllocateLargestRemainder` | Line discounts always sum to the headline discount. |
| `TestDiscountAmount` | Percent/fixed discount rounding. |
| `TestPointsEarned` | 1 point per whole R10, floored. |
| `TestPointsValue`, `TestMaxRedeemablePoints` | 10c per point; totals never go below zero. |
| `TestReversePointsForRefund` | Earned points reverse proportionally. |
| `TestVoidPoints` | Void restores redeemed and removes earned. |
| `TestPriceLines*` | Server pricing, duplicate-line merge, free-item handling. |
| `TestRefundLineAmount` | Refund amount excludes the line's proportional discount. |
| `TestCryptoAmountFor`, `TestCryptoAmountForNeverUsesFloat` | Exact decimal conversion via `math/big`. |
| `TestAssetTableIsComplete`, `TestValidateAssetID` | Seven assets and their networks. |
| `TestBuildReceipt*` | Receipt text, status banners, refunds, truncation. |
| `TestComputeProfit*` | Net revenue − COGS, margin, empty and negative cases. |

### `internal/posservice` — use cases

In-memory/temporary SQLite through the native `sqlRunner`, so each test
exercises real SQL and real batches.

| Test | Covers |
| --- | --- |
| `TestBusinessLifecycle` | Onboarding, updates, website normalisation, per-owner scoping. |
| `TestCategories`, `TestProductsAndInventory`, `TestCustomers` | CRUD, uniqueness, stock movements, valuation, cost history. |
| `TestPendingCheckoutPricesOnServer` | Client prices are ignored; pending sales do not move stock. |
| `TestCheckoutMergesDuplicateLines` | Duplicate product ids merge. |
| `TestCheckoutRedeemsLoyaltyPoints` | Redemption and earned points. |
| `TestCheckoutRejectsInvalidCarts` | Empty/oversized carts, inactive products, bad discounts, insufficient stock/points. |
| `TestCompleteSaleDeductsStockAndEarnsPointsOnce` | Completion is idempotent. |
| `TestFailedSaleNeverChangesStockOrLoyalty` | Failed payments are inert. |
| `TestVoidRestoresStockAndReversesLoyalty` | Void rules. |
| `TestRefundPartialThenFull` | Proportional amounts, status transitions, loyalty reversal. |
| `TestRefundWriteOffKeepsCost` | `restock:false` retains COGS. |
| `TestRefundValidation` | Unknown line, over-refund, duplicate line, non-completed sale. |
| `TestIdempotentSaleCreation`, `TestIdempotentRefund` | Replay and reuse conflict. |
| `TestProfitReport` | Revenue, refunds, COGS, profit, margin, method breakdown. |
| `TestReceipts` | Sale and refund receipt text from real rows. |
| `TestConcurrentCompletionsSettleOnce`, `TestConcurrentRefundsNeverOverRefund` | Guarded conditional updates under a race. |
| `TestSaleDetailNotFound` | Cross-business ids behave as not found. |

### `internal/posstore` — persistence

| Test | Covers |
| --- | --- |
| `TestApplySchemaAndBusinessUpsert` | Migrations apply cleanly; one business per owner. |
| `TestCategoriesAndProducts` | Filters, joins, SKU uniqueness. |
| `TestStockGuardRollsBackOversell`, `TestStockGuardRejectsUnknownProduct` | `pos_guard` aborts the batch on a failed conditional update. |
| `TestReceiptNumbersAreUniqueAndSequential` | Atomic per-business counters. |
| `TestSaleLifecycleWrites` | Insert/complete/refund statements and status transitions. |
| `TestIdempotencyRoundTrip` | Stored response replay. |
| `TestListSaleTotals` | Report aggregation query. |

### `lib` — HTTP integration

`lib/pos_api_test.go` builds a real `Handler` with a temporary SQLite database
and the POS service, then drives it with `httptest`:

| Test | Covers |
| --- | --- |
| `TestPOSRequiresAuthenticationAndConfiguration` | `401` without a token, `503` when POS is not wired. |
| `TestPOSBusinessOnboarding` | `data:null` before setup, `PUT`, scoping, `409` elsewhere. |
| `TestPOSCatalogueAndInventory` | Categories → products → stock → valuation/movements end to end. |
| `TestPOSCheckoutCompleteRefundAndVoid` | Full sale lifecycle through HTTP. |
| `TestPOSSaleIdempotency` | Replay and `IDEMPOTENCY_KEY_REUSED`. |
| `TestPOSPaymentLinkSettlesLinkedSale` | `saleId` link, tampered amount rejection, confirm completes the sale. |
| `TestPOSErrorMapping` | Domain errors map to the documented status codes. |
| `TestPOSProfitReport` | Report JSON shape after real sales/refunds. |

The existing `lib/payments_test.go`, `handler_test.go`, `store_test.go`, and
auth/session tests continue to cover the legacy API, including the refactor of
asset handling onto `posdomain`.

### App and browser tests

- `app/src/domain/*.test.ts` — money, cart, discount, loyalty, payment rates.
- `app/src/services/payments.test.ts` — local payment request lifecycle and API
  fallback.
- `app/.maestro/*.yaml` — onboarding, add product, sell on an iOS simulator.
- `backend/api/e2e/*.spec.ts` — Playwright sign-in and the public checkout page
  (asset selection and paid status).

## What is intentionally tested twice

Business rules live in `posdomain` and are asserted there with plain numbers.
`posservice` tests then assert the same rules against real SQLite rows (stock,
loyalty, refunds, totals), and `lib/pos_api_test.go` asserts the HTTP contract.
This layering means a regression is caught even if one layer is refactored.

## Database integration checks

`backend/scripts/test-database.sh` verifies:

- every expected table exists after applying all worker migrations;
- `pos_payments` has `sale_id` and `business_id`;
- the receipt counter increments atomically under `ON CONFLICT`;
- `pos_guard` exists with its CHECK constraint;
- legacy wallet queries still return the expected rows.

It also confirms the migrations work on real Wrangler local D1, which enforces
foreign keys and differs subtly from modernc SQLite.

## Adding tests

- Put pure-rule tests in `internal/posdomain` and avoid I/O.
- Put use-case tests in `internal/posservice`; use `helpers_test.go` to create a
  temporary database and apply migrations.
- Put HTTP tests in `lib`; build the handler with `posservice.New` over a
  temporary database and assert status, envelope, and error codes.
- Never assert on client-supplied totals: send them and assert the server
  ignores them.

### Schema helper caveat

The test helpers split migration SQL on `;`. Migrations must not contain
semicolons inside statements or comments, or the split will break. Keep one
statement per line group as the existing migrations do.

## Continuous verification

Before committing a POS change, run:

```bash
(cd backend/api && gofmt -l . && go vet ./... && go test ./... && go test -race ./... && GOOS=js GOARCH=wasm go build ./...)
bash backend/scripts/test-database.sh
(cd app && yarn test && yarn typecheck)
```

All of the above pass on the current branch.
