# UI to API mapping

The Expo app is still offline-first: every screen reads and writes the local
SQLite database through `app/src/services/*`. This document maps each screen's
local service calls to the equivalent `/v1/pos` endpoint so a future
server-authoritative mode (or a sync layer) can be wired screen by screen
without changing the UI.

Status legend:

- **Local only** — the API exists but the screen does not call it yet.
- **Wired** — the screen already calls the API for this flow.
- **Local + API fallback** — the app calls the API when reachable and keeps a
  local copy when it is not.

## Screen → service → endpoint

| Screen | Local service call | API endpoint | Status |
| --- | --- | --- | --- |
| `app/index.tsx` (boot) | `hasBusiness` | `GET /v1/pos/business` | Local only |
| `app/welcome.tsx` | `hasBusiness` | `GET /v1/pos/business` | Local only |
| `app/onboarding.tsx` | `getBusiness`, `saveBusiness` | `GET`/`PUT /v1/pos/business` | Local only |
| `(tabs)/wallet.tsx` | `getBusiness` | `GET /v1/pos/business` | Local only |
| `(tabs)/index.tsx` (Sell) | `listCategories`, `listProducts` | `GET /v1/pos/categories`, `GET /v1/pos/products` | Local only |
| `(tabs)/products.tsx` | `listCategories`, `listProducts` | same | Local only |
| `app/categories.tsx` | `createCategory`, `renameCategory`, `deleteCategory`, `listCategories` | `POST`/`PUT`/`DELETE`/`GET /v1/pos/categories` | Local only |
| `app/product/[id].tsx` | `createProduct`, `getProduct`, `updateProduct`, `setProductActive`, `listCategories`, `costHistory` | `POST`/`GET`/`PUT /v1/pos/products`, `POST /v1/pos/products/{id}/active`, `GET /v1/pos/inventory/cost-history` | Local only |
| `(tabs)/stock.tsx` | `listProducts`, `adjustStock`, `inventoryValuation`, `listMovements` | `GET /v1/pos/products`, `POST /v1/pos/products/{id}/stock`, `GET /v1/pos/inventory/valuation`, `GET /v1/pos/inventory/movements` | Local only |
| `(tabs)/insights.tsx` | `lowStockProducts`, `profitSummary` | `GET /v1/pos/inventory/low-stock`, `GET /v1/pos/reports/profit` | Local only |
| `(tabs)/history.tsx` | `listSales` | `GET /v1/pos/sales` | Local only |
| `app/cart.tsx` | `listCustomers`, `createCustomer`, `startPayment` | `GET`/`POST /v1/pos/customers`, `POST /v1/pos/payments` | **Wired** (payments only) |
| `app/checkout/[id].tsx` | `getPayment`, `refreshPayment`, `simulatePayment`, `abandonPayment` | `GET /v1/pos/payments/{id}`, `POST /v1/pos/payments/{id}/confirm` | **Wired** |
| `app/sale/[id].tsx` | `getSale`, `receiptText`, `refundSale`, `refundReceiptText`, `voidSale` | `GET /v1/pos/sales/{id}`, `GET /v1/pos/sales/{id}/receipt`, `POST /v1/pos/sales/{id}/refunds`, `GET /v1/pos/sales/{id}/refunds/{refundId}/receipt`, `POST /v1/pos/sales/{id}/void` | Local only |

The sell flow never calls `POST /v1/pos/sales` today. `cart.tsx` creates the
pending sale locally (`createPendingSale`), then `startPayment` posts the legacy
payment body (`receiptNo`, `amountCents`, `merchant`, …) without `saleId`.

## Field mapping

The app and the API agree on money (integer ZAR cents), timestamps (epoch
milliseconds), and status values. They differ in a few places.

### Ids

| App | API |
| --- | --- |
| `INTEGER PRIMARY KEY AUTOINCREMENT` | 128-bit random hex `TEXT` |
| Numeric route params (`sale/[id].tsx`) | String route params |

A sync layer would need an `app_id ↔ api_id` map per table; see
`OFFLINE_SYNC.md`.

### Business

| App (`business` table) | API (`pos_businesses`) |
| --- | --- |
| single row (`id = 1`) | one row per owner, `business_id` scopes all data |
| `logo_uri` nullable | `logoUri` string |
| — | `ownerId` (DID or `token:shared`) not exposed |

`GET /v1/pos/business` returns `{"data": null}` before onboarding, which maps
directly to `hasBusiness === false`.

### Products

| App `ProductWithCategory` | API `Product` |
| --- | --- |
| `id: number` | `id: string` |
| `categoryName` joined | `categoryName` joined |
| `price`, `costPrice`, `stock`, `reorderLevel` | same names, same cents |
| `active: 0\|1` | `active: boolean` |

`listProducts` filters map to:

| App option | API query |
| --- | --- |
| `query` | `query` |
| `categoryId` | `categoryId` |
| `includeInactive` | `includeInactive=true` |
| `lowStock` | `lowStock=true` |
| `limit` | `limit` |

### Stock movements

| App `MovementRow` | API `StockMovement` |
| --- | --- |
| `id: number` | `id: string` |
| `productName`, `sku` joined | same |
| `type` one of `initial\|sale\|refund\|void\|restock\|adjustment` | same CHECK constraint |
| `quantity` signed | same |
| `balanceAfter` | same |
| `unitCost` nullable | same |

`adjustStock` maps to `POST /v1/pos/products/{id}/stock`:

```ts
// app
adjustStock(db, { productId, kind: 'restock', delta: 5, unitCost: 1250, note })
// API
POST /v1/pos/products/{id}/stock
{"kind":"restock","delta":5,"unitCost":1250,"note":"…"}
```

### Customers and loyalty

| App | API |
| --- | --- |
| `loyaltyPoints` | `loyaltyPoints` |
| `phone` nullable | `phone` nullable |
| — | `PUT /v1/pos/customers/{id}` (API-only) |

### Sales

| App `SaleDetail` | API `SaleDetail` |
| --- | --- |
| `{ sale, items, refunds }` | `{ sale, items, refunds, customer }` |
| `cryptoAmount: REAL`, `cryptoRate: REAL` | `cryptoAmount: string`, `cryptoRate: string` |
| `receiptText(db, detail)` built in the app | `GET /sales/{id}/receipt` returns the text |

The app parses crypto amounts with `Number(...)`; the API keeps decimal strings
so precision survives. A UI integration must format the string rather than
round-trip it through a float.

`checkout()` in the app = `POST /v1/pos/sales` with `complete: true`.
`createPendingSale()` = the same call with `complete: false`.
`completeSale()` / `failSale()` = the two `POST /sales/{id}/…` routes.

### Refunds

| App `refundSale(input)` | API `POST /sales/{id}/refunds` |
| --- | --- |
| `items: {saleItemId, qty}` | same |
| `reason` | `reason` |
| `restock` | `restock` (default `true`) |
| returns `{refundId, amount, pointsReversed}` | returns `{refund, saleId, status, refundedTotal}` |

### Reports

`profitSummary(db, {from, to})` and `GET /v1/pos/reports/profit` return the same
`ProfitSummary` fields: `salesCount`, `grossRevenue`, `refundedTotal`,
`netRevenue`, `cogs`, `profit`, `marginPct`, `refundCount`, `voidedCount`,
`failedCount`, `pointsIssued`, `byMethod`.

`(tabs)/insights.tsx` calls `lowStockProducts` and `profitSummary`; both have
exact API equivalents.

### Payments

`startPayment` is the only local service that reaches the network. It:

1. calls `createPendingSale` locally;
2. computes a display `cryptoAmount` with `toAssetAmount`;
3. `POST /v1/pos/payments` with the **legacy** body (no `saleId`);
4. stores the returned `id`, `paymentUrl`, and `address` in the local
   `payment_requests` row, or falls back to a `local_…` id when offline.

`checkout/[id].tsx` polls `GET /v1/pos/payments/{id}` through `refreshPayment`
and settles the local sale once. It can also call
`POST /v1/pos/payments/{id}/confirm` via `simulatePayment`.

Because the app does not send `saleId`, the API cannot complete the sale for it;
the app still settles locally (`settlePayment`). When the app starts sending
`saleId` (already supported by the API), the server will complete the linked
sale before marking the payment paid, which is the multi-device-safe path.

## Suggested wiring order

1. **Business onboarding** (`GET`/`PUT /business`) — the API already returns
   `null` when absent, so the boot/onboarding flow can switch with no UI change.
2. **Catalogue + inventory** — read-through cache, then write-through with
   `Idempotency-Key` for stock adjustments.
3. **Sell/checkout** — send the cart to `POST /v1/pos/sales` and use the
   server-priced `SaleDetail` instead of local pricing.
4. **Sale detail, refunds, voids** — replace local receipts/refund math with the
   API responses and text receipts.
5. **Reports** — read directly from `/reports/profit` and
   `/inventory/low-stock`.
6. **Payments** — add `saleId` to `startPayment` so confirmation settles the
   server sale.

Each step keeps the local database as the offline fallback, so the app never
loses the ability to trade without a network.
