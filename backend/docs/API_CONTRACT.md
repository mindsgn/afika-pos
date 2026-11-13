# POS API contract

This is the detailed contract for `/v1/pos`. `API.md` is the quick reference;
this file is the source of truth for payload shapes, validation rules, and
error codes.

## Authentication and scoping

```
Authorization: Bearer <privy-access-token | shared-api-token>
```

- A verified Privy token scopes data to `did:privy:…`.
- Any other accepted token maps to the single shared owner `token:shared`.
- Business data is always scoped by the resolved business id; ids from another
  merchant behave as if they do not exist (`404`), never as an authorization
  error.

Missing or invalid tokens return:

```json
{"error":{"code":"UNAUTHENTICATED","message":"A valid API token is required.","requestId":"…"}}
```

## Envelopes

Single resource:

```json
{"data": { ... }}
```

List:

```json
{"data": [ ... ], "meta": {"count": 2}}
```

Error:

```json
{"error": {"code": "PRODUCT_NOT_FOUND", "message": "Product not found", "requestId": "…"}}
```

## Business

`GET /v1/pos/business` → `200`

```json
{"data": null}
```

or

```json
{
  "data": {
    "id": "9f0c…",
    "name": "Kofi Coffee",
    "website": "https://kofi.example",
    "address": "12 Long St, Cape Town",
    "walletAddress": "0xabc",
    "logoUri": "",
    "createdAt": 1760000000000,
    "updatedAt": 1760000000000
  }
}
```

`PUT /v1/pos/business`

| Field | Required | Rules |
| --- | --- | --- |
| `name` | yes | ≥ 2 characters after trimming. |
| `website` | no | `https://` is added when no scheme; must match `https?://host.tld`. |
| `address` | yes | ≥ 4 characters. |
| `walletAddress` | no | Lower-cased and stored; default receiving address. |
| `logoUri` | no | Kept when a later update omits it. |

Errors: `INVALID_BUSINESS`.

## Categories

`POST /v1/pos/categories` → `201`

```json
{"data": {"id":"…","name":"Coffee","createdAt":1760000000000}}
```

- `name` is required (`INVALID_NAME`).
- Names are unique per business, case-sensitive (`CATEGORY_EXISTS`).
- `PUT` renames; `DELETE` refuses while products reference the category
  (`CATEGORY_IN_USE`) and returns `204` on success.

## Products

`Product`:

```json
{
  "id": "…",
  "sku": "FLAT-WHITE",
  "name": "Flat White",
  "imageUri": "",
  "categoryId": "…",
  "categoryName": "Coffee",
  "price": 3500,
  "costPrice": 1200,
  "stock": 10,
  "reorderLevel": 3,
  "active": true,
  "createdAt": 1760000000000,
  "updatedAt": 1760000000000
}
```

`POST /v1/pos/products` / `PUT /v1/pos/products/{id}` body:

| Field | Required | Rules |
| --- | --- | --- |
| `sku` | yes | Trimmed and upper-cased; unique per business (`SKU_EXISTS`). |
| `name` | yes | Non-empty. |
| `imageUri` | no | Trimmed. |
| `categoryId` | no | Must exist for this business (`CATEGORY_NOT_FOUND`). |
| `price` | no | ≥ 0 cents. |
| `costPrice` | no | ≥ 0 cents. |
| `stock` | create only | ≥ 0; becomes the `initial` movement. Ignored by `PUT`. |
| `reorderLevel` | no | ≥ 0. |

Errors: `INVALID_PRODUCT`, `SKU_EXISTS`, `CATEGORY_NOT_FOUND`.

`POST /v1/pos/products/{id}/active`:

```json
{"active": false}
```

`POST /v1/pos/products/{id}/stock`:

```json
{"kind": "restock", "delta": 5, "unitCost": 1250, "note": "Weekly order"}
```

- `kind` is `restock` or `adjustment` (`INVALID_STOCK_DELTA`).
- `delta` cannot be zero; `restock` must be positive.
- An `adjustment` may not take stock below zero (`INSUFFICIENT_STOCK`).
- `unitCost` must be ≥ 0 and, for a restock, also updates the product cost.

## Inventory

- `GET /inventory/movements?productId=&limit=` → newest first.

```json
{"data":[{"id":"…","productId":"…","productName":"Flat White","sku":"FLAT-WHITE","type":"sale","quantity":-1,"balanceAfter":9,"unitCost":1200,"reference":"R-000001","note":"","createdAt":1760000000000}],"meta":{"count":1}}
```

- `GET /inventory/low-stock` → `active = 1 AND stock <= reorder_level`.
- `GET /inventory/valuation` →

```json
{"data":{"units":10,"costValue":12000,"retailValue":35000,"productCount":1}}
```

- `GET /inventory/cost-history?productId=…` → movements with a unit cost.

## Customers

```json
{"data":{"id":"…","name":"Thandi","phone":"0821234567","loyaltyPoints":120,"createdAt":1760000000000}}
```

- `name` required (`INVALID_CUSTOMER`).
- `phone` optional but unique per business when present
  (`CUSTOMER_PHONE_EXISTS`).
- `PUT` applies the same rules, ignoring the customer's own row for uniqueness.
- `GET /customers?query=` matches name or phone.

## Sales

`POST /v1/pos/sales`

```json
{
  "items": [{"productId": "…", "qty": 2}],
  "discount": {"type": "percent", "value": 10},
  "customerId": "…",
  "redeemPoints": 50,
  "paymentMethod": "usdc-base",
  "complete": false,
  "txHash": "",
  "idempotencyKey": "device-7-sale-42"
}
```

Pricing rules (all server-side):

1. Duplicate `productId` lines merge into one quantity.
2. Max 50 distinct products (`CART_TOO_LARGE`).
3. Products must exist (`PRODUCT_NOT_FOUND`), be active (`PRODUCT_INACTIVE`),
   and have enough stock (`INSUFFICIENT_STOCK`). Stock is checked even for a
   pending sale, but not deducted.
4. Prices and costs come from the catalogue, never from the client.
5. A manual discount (`percent` or `fixed`) is allocated across lines by the
   same largest-remainder algorithm as the app, so line discounts always sum to
   the headline discount.
6. Loyalty redemption is 10 cents per point; it requires a customer
   (`CUSTOMER_REQUIRED`) and enough points (`INSUFFICIENT_POINTS`). Totals never
   go below zero.
7. `paymentMethod` is an asset id (`btc`, `usdc-base`, `usdc-ethereum`,
   `zarp-base`, `zarp-ethereum`, `eurc-base`, `eurc-ethereum`) or empty for
   `usdc-base` (`INVALID_PAYMENT_METHOD`).
8. `complete:false` stores a `pending` sale with no stock or loyalty changes;
   `complete:true` deducts stock, updates points, and stores the crypto amount
   in the same atomic batch.

`SaleDetail` (returned by create, complete, fail, void, get):

```json
{
  "data": {
    "sale": {
      "id": "…",
      "receiptNo": "R-000001",
      "createdAt": 1760000000000,
      "updatedAt": 1760000000000,
      "status": "completed",
      "customerId": "…",
      "subtotal": 10000,
      "discountType": "percent",
      "discountValue": 10,
      "discountAmount": 1000,
      "pointsRedeemed": 0,
      "pointsDiscount": 0,
      "total": 9000,
      "costTotal": 4000,
      "paymentMethod": "usdc-base",
      "paymentStatus": "success",
      "cryptoAmount": "4.86",
      "cryptoRate": "0",
      "txHash": "0x…",
      "pointsEarned": 9,
      "refundedTotal": 0
    },
    "items": [
      {"id":"…","saleId":"…","productId":"…","sku":"FLAT-WHITE","name":"Flat White","qty":2,"unitPrice":5000,"unitCost":2000,"lineDiscount":1000,"refundedQty":0,"refundedAmount":0}
    ],
    "refunds": [],
    "customer": {"id":"…","name":"Thandi","loyaltyPoints":129,"createdAt":1760000000000}
  }
}
```

Statuses: `pending`, `completed`, `failed`, `voided`, `refunded`,
`partially_refunded`. `paymentStatus`: `pending`, `success`, `failed`.

`POST /sales/{id}/complete`

```json
{"txHash": "0x…", "cryptoAmount": "4.86", "cryptoRate": "18.5"}
```

All fields are optional. Omitting `cryptoAmount` falls back to the sale's stored
amount, then to an exact conversion from the total. Completing a completed sale
returns it unchanged. Completing a failed/voided sale is `SALE_NOT_PENDING`.

`POST /sales/{id}/fail`

```json
{"reason": "Payment window expired"}
```

Only pending sales can fail; failed sales never touch stock or loyalty.

`POST /sales/{id}/void`

```json
{"reason": "Wrong order"}
```

Only `completed` sales with no refunds can be voided (`SALE_NOT_VOIDABLE`)
because a refund moves the status. Voiding restores stock and reverses loyalty
(returning redeemed points, removing earned points).

`GET /sales/{id}/receipt` → `{"data":{"receipt":"Kofi Coffee\n…"}}`.

## Refunds

`POST /v1/pos/sales/{id}/refunds`

```json
{
  "items": [{"saleItemId": "…", "qty": 1}],
  "reason": "Cold",
  "restock": true,
  "idempotencyKey": "device-7-refund-3"
}
```

- Each `saleItemId` must belong to the sale (`REFUND_ITEM_NOT_IN_SALE`).
- Requested quantities cannot exceed the unrefunded quantity
  (`REFUND_QTY_EXCEEDS`).
- Duplicate `saleItemId` entries are rejected (`REFUND_DUPLICATE_ITEM`).
- The refunded amount uses the line price minus its proportional share of the
  sale discount, so refunds never exceed the sale total.
- `restock` defaults to `true`: stock returns and cost is recovered. When
  `false`, stock stays down and the COGS is retained.
- Earned points are reversed proportionally to the refunded amount.
- Redeemed points are never returned by a refund (only a void restores them).

Response:

```json
{"data":{"refund":{"id":"…","saleId":"…","createdAt":1760000000000,"amount":4500,"reason":"Cold","restocked":true,"pointsReversed":4,"items":[{"saleItemId":"…","name":"Flat White","qty":1,"amount":4500,"costRecovered":2000}]},"saleId":"…","status":"partially_refunded","refundedTotal":4500}}
```

`GET /sales/{id}/refunds` → list envelope of refunds.

`GET /sales/{id}/refunds/{refundId}/receipt` → `{"data":{"receipt":"…"}}`.

## Idempotency

`POST /sales` and `POST /sales/{id}/refunds` accept a key through the body
(`idempotencyKey`) or the `Idempotency-Key` header (body wins).

- The same key with the same request returns the stored response (including the
  original id and receipt number) and performs no further writes.
- The same key with a different request returns
  `409 IDEMPOTENCY_KEY_REUSED`.
- Keys longer than 200 characters or containing control characters return
  `INVALID_IDEMPOTENCY_KEY`.
- The key is written in the same batch as the sale/refund, so two concurrent
  retries cannot both succeed.

## Reports

`GET /v1/pos/reports/profit?from=…&to=…` (epoch milliseconds, inclusive-ish
window; omit for all time)

```json
{
  "data": {
    "salesCount": 12,
    "grossRevenue": 48000,
    "refundedTotal": 5000,
    "netRevenue": 43000,
    "cogs": 16000,
    "profit": 27000,
    "marginPct": 62.8,
    "refundCount": 1,
    "voidedCount": 0,
    "failedCount": 1,
    "pointsIssued": 48,
    "byMethod": [{"method":"usdc-base","count":12,"revenue":48000}]
  }
}
```

Only completed/refunded sales count as revenue. Failed and voided sales are
reported as counts, not revenue.

## Payments

`POST /v1/pos/payments`

```json
{"saleId": "…"}
```

or the legacy body:

```json
{
  "receiptNo": "POS-0001",
  "amountCents": 12345,
  "currency": "ZAR",
  "asset": "usdc-base",
  "receiveAddress": "0x…",
  "merchant": {"name":"Kofi Coffee","website":"https://kofi.example","address":"12 Long St"}
}
```

With `saleId` the server requires a pending sale and takes `amountCents`,
`receiptNo`, merchant name/website/address, and the default address from the
sale/business. A mismatching `amountCents` or `receiptNo` is a `400`.

`POSPayment`:

```json
{
  "data": {
    "id": "…",
    "receiptNo": "R-000001",
    "amountCents": 9000,
    "currency": "ZAR",
    "status": "pending",
    "asset": "usdc-base",
    "network": "base",
    "address": "0xabc",
    "cryptoAmount": "4.86",
    "paymentUrl": "https://…/pay/…",
    "merchantName": "Kofi Coffee",
    "merchantWebsite": "https://kofi.example",
    "merchantAddress": "12 Long St, Cape Town",
    "saleId": "…",
    "createdAt": 1760000000000,
    "updatedAt": 1760000000000,
    "expiresAt": 1760001800000
  }
}
```

- Status lifecycle: `pending` → `paid` | `expired` | `failed`.
- Links expire after 30 minutes; a read after expiry reports `expired`.
- `POST /payments/{id}/confirm` completes the linked sale first, then marks the
  link paid. If the sale cannot be completed the payment stays pending so the
  receipt worker can retry.
- Confirming an already paid link is idempotent.

## Error codes

| Code | Status | Meaning |
| --- | --- | --- |
| `UNAUTHENTICATED` | 401 | Missing or invalid token. |
| `VALIDATION_ERROR` | 400 | Generic body/query validation. |
| `INVALID_JSON` | 400 | Body is not a single JSON object. |
| `UNSUPPORTED_MEDIA_TYPE` | 415 | Missing/non-JSON `Content-Type`. |
| `PAYLOAD_TOO_LARGE` | 413 | Body over 64 KiB. |
| `METHOD_NOT_ALLOWED` | 405 | Wrong method. |
| `NOT_FOUND` | 404 | Unknown route or resource. |
| `INTERNAL_ERROR` | 500 | Unexpected failure. |
| `REQUEST_TIMEOUT` | 504 | Store/use case timed out. |
| `POS_NOT_CONFIGURED` | 503 | Deployment has no POS service. |
| `BUSINESS_NOT_SET_UP` | 409 | Onboarding has not run. |
| `INVALID_BUSINESS` | 400 | Bad business profile. |
| `CATEGORY_EXISTS` | 409 | Duplicate category name. |
| `CATEGORY_IN_USE` | 409 | Category still has products. |
| `CATEGORY_NOT_FOUND` | 404 | Unknown category. |
| `INVALID_NAME` | 400 | Empty category name. |
| `PRODUCT_NOT_FOUND` | 404 | Unknown product. |
| `PRODUCT_INACTIVE` | 400 | Product archived. |
| `SKU_EXISTS` | 409 | Duplicate SKU. |
| `INVALID_PRODUCT` | 400 | Bad product fields. |
| `INVALID_STOCK_DELTA` | 400 | Bad stock adjustment. |
| `CUSTOMER_NOT_FOUND` | 404 | Unknown customer. |
| `CUSTOMER_PHONE_EXISTS` | 409 | Duplicate phone. |
| `CUSTOMER_REQUIRED` | 400 | Points without a customer. |
| `INVALID_CUSTOMER` | 400 | Bad customer fields. |
| `CART_EMPTY` | 400 | No items. |
| `CART_TOO_LARGE` | 400 | More than 50 distinct products. |
| `INVALID_QUANTITY` | 400 | Quantity ≤ 0. |
| `INVALID_DISCOUNT` | 400 | Unknown discount type/value. |
| `INSUFFICIENT_STOCK` | 400 | Not enough stock. |
| `INSUFFICIENT_POINTS` | 400 | Not enough loyalty points. |
| `INVALID_PAYMENT_METHOD` | 400 | Unknown asset id. |
| `INVALID_CART` | 400 | Pricing failure. |
| `SALE_NOT_FOUND` | 404 | Unknown sale. |
| `SALE_NOT_PENDING` | 409 | Complete/fail on a settled sale. |
| `SALE_NOT_VOIDABLE` | 409 | Void on a refunded or non-completed sale. |
| `SALE_ALREADY_VOIDED` | 409 | Repeat void. |
| `SALE_NOT_REFUNDABLE` | 409 | Refund on a non-completed sale. |
| `REFUND_NOT_FOUND` | 404 | Unknown refund. |
| `REFUND_EMPTY` | 400 | No refund lines. |
| `REFUND_DUPLICATE_ITEM` | 400 | Same sale line twice in one refund. |
| `REFUND_ITEM_NOT_IN_SALE` | 400 | Line belongs to another sale. |
| `REFUND_QTY_EXCEEDS` | 400 | More than remains. |
| `INVALID_REFUND` | 400 | Bad refund payload. |
| `IDEMPOTENCY_KEY_REUSED` | 409 | Key used for a different request. |
| `INVALID_IDEMPOTENCY_KEY` | 400 | Key too long or malformed. |
