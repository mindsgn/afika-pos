# POS feature parity

This document tracks what the Go Cloudflare Worker in `backend/api` supports
compared with the offline Expo app in `app/`. The app keeps working exactly as
before: the API is an additive server-side implementation of the same business
rules, not a replacement UI.

Legend:

- **API** — implemented and tested in `backend/api`.
- **App** — already implemented in the app's local SQLite services.
- **Wired** — the app currently calls the API for this flow.
- **Not wired** — the API supports it, but the app still uses local SQLite.

## Summary

| Area | App | API | Wired to API |
| --- | --- | --- | --- |
| Business onboarding | yes | yes | no |
| Categories | yes | yes | no |
| Products | yes | yes | no |
| Stock adjustments / restock / movement ledger | yes | yes | no |
| Inventory valuation / low stock / cost history | yes | yes | no |
| Customers / loyalty | yes | yes | no |
| Cart pricing, discount allocation | yes | yes | no |
| Pending sale → complete / fail | yes | yes | no |
| Voids | yes | yes | no |
| Refunds (partial, full, restock/write-off) | yes | yes | no |
| Profit report | yes | yes | no |
| Receipts (text) | yes | yes | no |
| Shareable crypto checkout links | yes | yes | **yes** (`startPayment`, `refreshPayment`) |
| Offline-first local database | yes | n/a | n/a |

## Business

| Capability | App | API | Notes |
| --- | --- | --- | --- |
| Create / update merchant profile | `saveBusiness` | `PUT /v1/pos/business` | Name ≥ 2 chars, address ≥ 4 chars, website normalised to `https://` and validated. |
| Read profile | `getBusiness` | `GET /v1/pos/business` | Returns `data: null` before onboarding so the app can route to onboarding. |
| Wallet address for payments | stored on business | `walletAddress` | Used as the default receiving address for checkout links. |
| Merchant on receipts/payments | yes | yes | Name, website, address printed on every receipt and sent with each payment. |

## Catalogue

| Capability | App | API | Notes |
| --- | --- | --- | --- |
| List categories | `listCategories` | `GET /v1/pos/categories` | |
| Create category | `createCategory` | `POST /v1/pos/categories` | Unique per business; duplicates → `CATEGORY_EXISTS`. |
| Rename category | `renameCategory` | `PUT /v1/pos/categories/{id}` | |
| Delete category | `deleteCategory` | `DELETE /v1/pos/categories/{id}` | Refuses while products use it → `CATEGORY_IN_USE`. |
| List / search products | `listProducts` | `GET /v1/pos/products` | `query`, `categoryId`, `includeInactive`, `lowStock`, `limit`. |
| Get product | `getProduct` | `GET /v1/pos/products/{id}` | |
| Create product | `createProduct` | `POST /v1/pos/products` | SKU is upper-cased and unique per business; opening stock becomes an `initial` movement. |
| Update product | `updateProduct` | `PUT /v1/pos/products/{id}` | Stock is not editable here. |
| Archive / restore | `setProductActive` | `POST /v1/pos/products/{id}/active` | Inactive products cannot be sold. |

## Inventory

| Capability | App | API | Notes |
| --- | --- | --- | --- |
| Manual adjustment | `adjustStock` | `POST /v1/pos/products/{id}/stock` | `kind` is `adjustment` (signed) or `restock` (positive). |
| Restock with new cost | `adjustStock` | same | `unitCost` updates the product's cost price and is recorded on the movement. |
| Movement ledger | `listMovements` | `GET /v1/pos/inventory/movements` | Newest first, optional `productId` and `limit`. |
| Low stock | `lowStockProducts` | `GET /v1/pos/inventory/low-stock` | Active products with `stock <= reorder_level`. |
| Valuation | `inventoryValuation` | `GET /v1/pos/inventory/valuation` | `units`, `costValue`, `retailValue`, `productCount`. |
| Cost history | `costHistory` | `GET /v1/pos/inventory/cost-history` | Movements that carry a unit cost. |

## Customers and loyalty

| Capability | App | API | Notes |
| --- | --- | --- | --- |
| List / search | `listCustomers` | `GET /v1/pos/customers` | Name or phone `query`. |
| Create | `createCustomer` | `POST /v1/pos/customers` | Phone unique per business when present. |
| Update | — | `PUT /v1/pos/customers/{id}` | API-only. |
| Read | `getCustomer` | `GET /v1/pos/customers/{id}` | |
| Earn | `pointsEarned` | sale completion | 1 point per whole R10 of the sale total. |
| Redeem | `pointsValue` | sale create | 1 point = 10 cents; never below a zero total. |
| Refund reversal | `pointsToReverse` | refund | Proportional to the refunded amount; partial refunds never return redeemed points. |
| Void | `voidSale` | void | Restores redeemed points and reverses earned points. |

## Sales

| Capability | App | API | Notes |
| --- | --- | --- | --- |
| Create pending sale | `createPendingSale` | `POST /v1/pos/sales` (`complete:false`) | Server prices the cart; no stock or loyalty changes. |
| Immediate sale | `checkout` | `POST /v1/pos/sales` (`complete:true`) | Stock and loyalty settle in the same atomic batch. |
| Complete pending sale | `completeSale` | `POST /v1/pos/sales/{id}/complete` | Idempotent: repeating returns the settled sale. |
| Fail pending sale | `failSale` | `POST /v1/pos/sales/{id}/fail` | Never changes stock or loyalty. |
| Void | `voidSale` | `POST /v1/pos/sales/{id}/void` | Completed, unrefunded sales only; stock and loyalty reverse. |
| List / search | `listSales` | `GET /v1/pos/sales` | `status`, `query`, `limit`. |
| Detail | `getSale` | `GET /v1/pos/sales/{id}` | Sale + items + refunds + customer. |
| Receipt | `receiptText` | `GET /v1/pos/sales/{id}/receipt` | Plain-text receipt. |

## Refunds

| Capability | App | API | Notes |
| --- | --- | --- | --- |
| Partial refund | `refundSale` | `POST /v1/pos/sales/{id}/refunds` | Per-line quantities; proportional discount allocation. |
| Full refund | `refundSale` | same | Status becomes `refunded`. |
| Restock choice | `refundSale` | `restock:false` | Returns stock and recovers cost; write-off keeps COGS. |
| Loyalty reversal | `refundSale` | same | Reverses earned points proportionally; never re-adds redeemed points. |
| Refund list | sale detail | `GET /v1/pos/sales/{id}/refunds` | |
| Refund receipt | `refundReceiptText` | `GET /v1/pos/sales/{id}/refunds/{refundId}/receipt` | |

## Reporting

| Capability | App | API | Notes |
| --- | --- | --- | --- |
| Profit summary | `profitSummary` | `GET /v1/pos/reports/profit` | `from`/`to` epoch milliseconds. |
| Revenue, refunds, voided/failed counts | yes | yes | Same shapes as `ProfitSummary` in the app. |
| COGS and profit | yes | yes | Net revenue − COGS; restocked returns recover cost. |
| Margin percentage | yes | yes | Rounded to one decimal place. |
| Breakdown by method | yes | yes | One row per payment option. |

## Payments and checkout links

| Capability | App | API | Notes |
| --- | --- | --- | --- |
| Create checkout link | `startPayment` | `POST /v1/pos/payments` | Existing route, now accepts `saleId`. |
| Asset selection | `app/src/domain/payment.ts` | `POST /v1/pos/payments/{id}/asset` | Seven options; rates live in `posdomain.Assets`. |
| Public status page | `refreshPayment` | `GET /v1/pos/payments/{id}` | Public by design; carries no secrets. |
| Confirm paid | receipt worker | `POST /v1/pos/payments/{id}/confirm` | Completes the linked sale before marking the link paid. |
| Expiry | app/local | `expires_at` | 30 minute TTL. |

## Deliberate differences and limitations

1. **The app has not been migrated to the API.** It remains offline-first and
   keeps using `src/services/*` against its own SQLite database. The API is
   ready for a future sync or server-authoritative mode.
2. **Ids differ.** The app uses auto-increment integers; the API uses random
   128-bit hex ids so a whole sale can be written in one D1 batch. A sync layer
   would need an id map, as described in `OFFLINE_SYNC.md`.
3. **No multi-device conflict resolution yet.** The API is safe for concurrent
   writes (conditional updates + D1 batches), but the app does not push local
   changes to it.
4. **Products are soft-deleted.** Deactivating keeps sale history intact; there
   is no hard delete endpoint.
5. **Receipts are plain text.** The app renders them; the API returns the same
   text rather than HTML or PDF.
6. **Auth scope.** A Privy session scopes POS data by DID; the legacy shared
   API token scopes everything to one fixed owner (`token:shared`). There is no
   per-staff identity yet.
