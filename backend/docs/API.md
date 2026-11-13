# POS API reference

Base path: `/v1/pos`. Every route requires `Authorization: Bearer <token>`
except the public checkout page and `GET /v1/pos/payments/{id}`. Supported
tokens are a Privy access token or the shared `API_TOKEN`.

- Privy tokens scope data to the caller's DID.
- The shared token scopes everything to one merchant (`token:shared`).
- All routes except the bare `GET /v1/pos/payments/{id}` need a business
  profile; without one they return `409 BUSINESS_NOT_SET_UP`.

Money is integer ZAR cents. Timestamps are epoch milliseconds. Crypto amounts
and rates are decimal strings.

## Conventions

| Item | Value |
| --- | --- |
| Success envelope | `{"data": ...}` |
| List envelope | `{"data": [...], "meta": {"count": n}}` |
| Error envelope | `{"error": {"code": "...", "message": "...", "requestId": "..."}}` |
| Default list limit | 50 (products/sales accept `limit`, max 500 where noted) |
| Request body | `Content-Type: application/json` |
| Idempotency | `Idempotency-Key` header or `idempotencyKey` body field on sale/refund create |

## Business

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/business` | Current merchant profile, or `data: null`. |
| `PUT` | `/business` | Create or update the profile. |

```bash
curl -X PUT "$API/v1/pos/business" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Kofi Coffee","website":"kofi.example","address":"12 Long St, Cape Town","walletAddress":"0xabc"}'
```

## Categories

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/categories` | List categories. |
| `POST` | `/categories` | Create (`{"name":"Coffee"}`). |
| `PUT` | `/categories/{id}` | Rename. |
| `DELETE` | `/categories/{id}` | Delete an empty category. |

## Products

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/products` | List/filter: `query`, `categoryId`, `includeInactive`, `lowStock`, `limit`. |
| `POST` | `/products` | Create with opening stock. |
| `GET` | `/products/{id}` | Read one. |
| `PUT` | `/products/{id}` | Update details (not stock). |
| `POST` | `/products/{id}/active` | `{"active": false}` archives, `true` restores. |
| `POST` | `/products/{id}/stock` | Adjust or restock. |

```bash
# Create
curl -X POST "$API/v1/pos/products" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"sku":"FW","name":"Flat White","price":3500,"costPrice":1200,"stock":10,"reorderLevel":3}'

# Restock 5 units at a new unit cost
curl -X POST "$API/v1/pos/products/$PRODUCT/stock" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"kind":"restock","delta":5,"unitCost":1250,"note":"Weekly order"}'
```

## Inventory

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/inventory/movements` | Stock ledger; `productId`, `limit`. |
| `GET` | `/inventory/low-stock` | Active products at or below reorder level. |
| `GET` | `/inventory/valuation` | Units, cost value, retail value, product count. |
| `GET` | `/inventory/cost-history` | Movements with a unit cost; `productId` required. |

## Customers

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/customers` | List/search by `query` (name or phone). |
| `POST` | `/customers` | Create (`{"name":"Thandi","phone":"082..."}`). |
| `GET` | `/customers/{id}` | Read one. |
| `PUT` | `/customers/{id}` | Update name/phone. |

## Sales

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/sales` | List newest first: `status`, `query`, `limit`. |
| `POST` | `/sales` | Create a sale, optionally settling immediately. |
| `GET` | `/sales/{id}` | Sale detail (sale, items, refunds, customer). |
| `POST` | `/sales/{id}/complete` | Settle a pending sale (idempotent). |
| `POST` | `/sales/{id}/fail` | Mark a pending sale failed. |
| `POST` | `/sales/{id}/void` | Void a completed, unrefunded sale. |
| `GET` | `/sales/{id}/receipt` | Plain-text receipt. |

```bash
# Pending sale for an online payment
curl -X POST "$API/v1/pos/sales" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"items":[{"productId":"'$PRODUCT'","qty":2}],"paymentMethod":"usdc-base"}'

# Immediate sale with a 10% discount and loyalty redemption
curl -X POST "$API/v1/pos/sales" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: counter-1-sale-7' \
  -d '{"items":[{"productId":"'$PRODUCT'","qty":1}],"discount":{"type":"percent","value":10},"customerId":"'$CUSTOMER'","redeemPoints":50,"complete":true}'
```

`POST /sales` body:

| Field | Type | Notes |
| --- | --- | --- |
| `items` | array | `{productId, qty}`; duplicates merge; max 50 distinct products. |
| `discount` | object | `{type: "none"|"percent"|"fixed", value}`. |
| `customerId` | string | Optional loyalty member. |
| `redeemPoints` | integer | Requires `customerId`. |
| `paymentMethod` | string | Asset id (`btc`, `usdc-base`, …) or empty for the default. |
| `complete` | boolean | `true` settles stock/loyalty immediately. |
| `txHash` | string | Optional on-chain reference. |
| `idempotencyKey` | string | Optional; the `Idempotency-Key` header also works. |

## Refunds

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/sales/{id}/refunds` | Refunds for a sale. |
| `POST` | `/sales/{id}/refunds` | Create a partial or full refund. |
| `GET` | `/sales/{id}/refunds/{refundId}/receipt` | Refund receipt text. |

```bash
curl -X POST "$API/v1/pos/sales/$SALE/refunds" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"items":[{"saleItemId":"'$ITEM'","qty":1}],"reason":"Cold","restock":true}'
```

## Reports

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/reports/profit` | Profit summary; `from` and `to` epoch ms. |

## Payments (existing routes)

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `POST` | `/payments` | yes | Create a checkout link; optional `saleId`. |
| `GET` | `/payments/{id}` | public | Payment status. |
| `POST` | `/payments/{id}/asset` | public | Change the asset while pending. |
| `POST` | `/payments/{id}/confirm` | yes | Mark paid and settle the linked sale. |

```bash
curl -X POST "$API/v1/pos/payments" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"saleId":"'$SALE'"}'
```

When `saleId` is present the server reads `amountCents`, `receiptNo`, and the
merchant details from the sale. Sending a different `amountCents` is a `400`.

## Status codes

| Status | When |
| --- | --- |
| `200` | Read/update succeeded, or an idempotent repeat. |
| `201` | Resource created (category, product, customer, sale, refund, payment). |
| `204` | Category deleted. |
| `400` | Invalid payload, cart, discount, stock delta, refund, or idempotency key. |
| `401` | Missing/invalid bearer token. |
| `403` | Browser origin not allowed. |
| `404` | Unknown route or resource. |
| `405` | Wrong method; `Allow` header lists valid methods. |
| `409` | Business missing, duplicate name/SKU/phone, wrong sale state, or a reused idempotency key. |
| `500` | Unexpected server error; details are logged, not returned. |
| `503` | POS service not configured on the deployment. |
