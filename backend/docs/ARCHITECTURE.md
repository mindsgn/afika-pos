# Architecture

The POS backend is an additive extension of the existing Go Cloudflare Worker
in `backend/api`. It shares the Worker, the D1 database, authentication, error
envelopes, CORS, and request-id middleware with the crypto wallet API. There is
no second Worker and no second database.

```
Expo app (offline-first, local SQLite)
        │  only POST /v1/pos/payments + GET /v1/pos/payments/{id}
        ▼
Cloudflare Worker  backend/api  (Go → Wasm)
        │
        ├─ lib            HTTP: routing, auth, envelope, CORS, payments
        ├─ posservice     use cases: validation + orchestration + transactions
        ├─ posdomain      pure rules: money, pricing, discount, loyalty, crypto
        └─ posstore       SQL: models, reads, conditional writes, batches
        │
        ▼
Shared D1 database `afika` (pos_* tables + legacy wallet tables)
```

## Request flow

1. `main_wasm.go` opens the D1 driver, builds the verifier chain
   (`AnyTokenVerifier`), and constructs `lib.NewHandler` with
   `HandlerConfig.POS = posservice.New(posstore.NewWithRunner(...))`.
2. `Handler.ServeHTTP` adds security headers, resolves/propagates the request
   id, handles CORS and `OPTIONS`, authenticates nothing itself, then calls
   `route`.
3. `route` dispatches `/v1/pos/payments` and `/v1/pos/payments/*` to the
   existing payment handlers, and everything else under `/v1/pos/` to
   `routePOS`.
4. `routePOS` is a small manual router. It checks that `h.pos != nil`
   (`503 POS_NOT_CONFIGURED`), authenticates the bearer token via `posOwner`,
   applies a request timeout, splits the path, and delegates by first segment.
5. Handlers decode JSON, call one `posservice` method, and wrap the result in
   `{"data": …}` (lists add `{"meta":{"count":n}}`).
6. `posservice` applies validation and business rules, then either:
   - reads through `posstore` query methods, or
   - builds a `posstore.Writer` with all statements for one logical change and
     commits it atomically.

`posservice` never issues one statement at a time for a multi-row change, and
`posstore` never contains business policy.

## Layers

### `internal/posdomain` — pure rules

No imports from `lib`, `posstore`, or the database. Unit-testable without
SQLite. Contains:

| File | Responsibility |
| --- | --- |
| `money.go` | Parse/format integer ZAR cents; never floats. |
| `discount.go` | Discount amount + largest-remainder allocation across lines. |
| `loyalty.go` | 1 point per whole R10 earned, 10c per point redeemed, proportional refund reversal, void reversal. |
| `pricing.go` | Server-side cart pricing, line totals, refund line amounts. |
| `payment.go` | Asset table (single source of truth) and exact `math/big` crypto conversion. |
| `receipt.go` | Plain-text sale/refund receipts. |
| `report.go` | Profit summary (`net revenue − COGS`, margin). |

### `internal/posstore` — persistence

- `models.go` — row structs with JSON tags matching the API.
- `store.go` — read queries, receipt-number allocation, idempotency reads,
  valuation and report aggregates.
- `writes.go` — `Writer` helpers that append parameterised statements.
- `ids.go` — random 128-bit hex ids.
- `batch_wasm.go` — `D1BatchRunner` (Wasm only).

`*DB` wraps `*sql.DB` plus a `BatchRunner`. `New(db)` installs the native
`sqlRunner`; `NewWithRunner` installs the D1 runner.

### `internal/posservice` — use cases

- `service.go` — business onboarding, categories, products, customers,
  inventory.
- `sales.go` — checkout, completion, failure, void, refunds, receipts, profit,
  idempotency.
- `errors.go` — a typed `Error{Kind, Code, Message}` that handlers map to HTTP
  status codes.

Public methods take `businessID` (or `ownerID` for business onboarding) as the
first argument after `context.Context`, so scoping is explicit and cannot be
forgotten.

### `lib` — HTTP boundary

- `pos_handlers.go` — manual router, `posOwner`, `requireBusiness`, all POS
  handlers, query/body helpers, `posError` status mapping.
- `payments.go` — existing checkout links. Now accepts `saleId`, stores
  `sale_id`/`business_id`, and completes the linked sale before marking paid.
- `pay_page.go` — public checkout page; uses `posdomain.AssetByID` and exact
  crypto conversion.
- `handler.go` — `HandlerConfig.POS`, routing, CORS (now allows
  `Idempotency-Key`).

## Atomic writes without transactions

The D1 Go driver does not implement `database/sql` transactions. Multi-row
writes therefore go through `posstore.BatchRunner`:

- **Native/tests** (`sqlRunner`): `BEGIN` → execute each statement →
  `COMMIT`, rolling back on error.
- **Wasm** (`D1BatchRunner`): `D1Database.batch([...])`, which Cloudflare runs
  as one SQL transaction. A failure in any statement rolls the whole batch back.

### The `pos_guard` trick

A batch cannot branch on the result of a previous statement. Conditional
updates ("only deduct stock if it is still enough", "only complete while
pending") are kept safe with:

```sql
UPDATE pos_products SET stock = stock - ? WHERE id = ? AND stock >= ?;
INSERT INTO pos_guard (ok) SELECT 0 WHERE changes() = 0;
```

`pos_guard.ok` has `CHECK (ok = 1)`. On success the guarded update changes one
row, so the INSERT selects nothing and the batch commits. If the update changed
no rows, the INSERT writes `0`, the CHECK fails, and D1 rolls back the entire
batch. `Writer.ExpectOne()` appends that guard.

This pattern is verified against modernc SQLite in tests and against real
Wrangler local D1 in `backend/scripts/test-database.sh`.

### Write examples

| Operation | Statements in one batch |
| --- | --- |
| Complete sale | guard sale status update, insert sold stock movements, guarded product stock updates, customer point update (CAS), idempotency row |
| Refund | insert refund + lines, guarded refunded-qty updates, optional restock movements + product stock, points CAS, guarded sale totals update, idempotency row |
| Void | guarded sale update, restore movements + stock, points CAS |

Loyalty uses compare-and-set (`UPDATE … WHERE loyalty_points = ?` +
`ExpectOne`) so two concurrent completions cannot double-spend points.

## Idempotency

`POST /sales` and `POST /sales/{id}/refunds` accept an `Idempotency-Key` header
or body field.

1. The service hashes the request and looks it up in `pos_idempotency`.
2. A hit with the same hash replays the stored `response_json` without writing.
3. A hit with a different hash returns `409 IDEMPOTENCY_KEY_REUSED`.
4. On a miss, the response row is inserted **in the same batch** as the sale or
   refund. Two concurrent retries race on the primary key; exactly one batch
   commits.

## Authentication and scoping

`posOwner` accepts any verifier in the chain:

- A verified Privy token yields its DID (`did:privy:…`).
- The legacy shared token maps to `token:shared`.

`pos_businesses.owner_id` is unique, so each owner has exactly one business,
and every POS row carries `business_id`. Cross-business ids are treated as
not found (`404`), never as an authorization error. The public checkout page
and `GET /v1/pos/payments/{id}` are intentionally unauthenticated; the id is an
unguessable token and the response carries no secrets.

## Payments and the receipt worker

1. The app creates a pending sale (locally today; `POST /v1/pos/sales
   {"complete":false}` is the API equivalent).
2. `POST /v1/pos/payments` creates a `pos_payments` row and returns a
   `/pay/{id}` URL. With `saleId`, the amount, receipt number, and merchant
   come from the sale and business, and mismatches are rejected.
3. `backend/workers/receipt` watches the chain and calls
   `POST /v1/pos/payments/{id}/confirm`.
4. Confirm completes the linked sale **first**, then marks the payment paid. If
   completion fails the payment stays pending so the worker can retry.

Asset ids and ZAR reference rates live in `posdomain.Assets`. The app
(`app/src/domain/payment.ts`) and the receipt worker
(`backend/workers/receipt/lib/match.go`) keep matching copies; see
`POS_FEATURE_PARITY.md`.

## Configuration

| Variable | Purpose |
| --- | --- |
| `API_TOKEN` | Legacy shared bearer token. |
| `PRIVY_APP_ID`, `PRIVY_APP_SECRET` | Privy profile API. |
| `PRIVY_VERIFICATION_KEY` | Privy JWT verification. |
| `PRIVY_CLIENT_ID` | Passwordless client id. |
| `SESSION_SECRET` | Signed website sessions. |
| `API_ALLOWED_ORIGINS` | Comma-separated CORS allow-list. |
| `POS_RECEIVING_ADDRESS` | Default crypto receiving address. |
| `CHECKOUT_BASE_URL` | Base URL for `/pay/{id}` links. |
| `DB_BINDING` | D1 binding name (default `afika`). |
| `E2E_MODE` | Uses demo Privy/email clients for end-to-end runs. |

## Deployment

The Worker and its migrations live in `backend/api`. `wrangler.jsonc` binds the
shared `afika` D1 database. Apply migrations with Wrangler before deploying a
build that uses them; `backend/scripts/test-database.sh` applies every worker's
migrations to one local database as a compatibility check.

## Design boundaries

- The app UI is unchanged; the API is additive.
- No floating point is used for money or crypto conversion.
- One business per owner; all POS reads/writes are business-scoped.
- The POS service is optional in `HandlerConfig`; deployments without D1 or POS
  configuration still serve the legacy wallet API.
