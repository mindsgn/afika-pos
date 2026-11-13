# Offline-first and sync

The app is, and remains, offline-first. Every sale, refund, stock change, and
customer update is written to the device's local SQLite database first and
works without a network. The Go API is a server-side mirror of the same rules,
ready for a future sync or server-authoritative mode. No sync client exists yet.

## Today

```
┌───────────────────────────┐        ┌───────────────────────────┐
│ Expo app                  │        │ Go Worker + D1            │
│ src/services/*            │        │ /v1/pos/*                 │
│ local SQLite (source)     │        │ pos_* tables (server)     │
└─────────────┬─────────────┘        └─────────────▲─────────────┘
              │                                    │
              │  POST /v1/pos/payments             │
              │  GET  /v1/pos/payments/{id}        │
              └────────────────────────────────────┘
                 (only the hosted checkout flow)
```

- All business data is local. `src/services/*` reads and writes SQLite
  synchronously.
- `startPayment` creates the pending sale locally, then tries to create a
  hosted checkout link on the API. On failure it stores a `local_…` request and
  the checkout screen offers demo confirmation.
- `refreshPayment` polls the public payment status and settles the local sale
  exactly once (`settlePayment`), even though the server may also complete its
  own copy of the sale when `saleId` is sent.
- Nothing else reaches the network.

## Why the server is not authoritative yet

The app UI and services assume local, synchronous reads. Making the API
authoritative would require every screen to handle latency, retries, and
partial failures, which is out of scope for this change. The API therefore
implements the same rules without forcing a UI migration.

## Sync design

A future sync layer should be **push-then-pull**, per business, with the local
database as the source of truth while offline.

### 1. Stable local identity

Every local row needs a globally unique id before it can be pushed. Options:

- **Recommended:** add a `sync_id TEXT UNIQUE` column to each local table,
  filled with a random UUID/hex value on insert. Keep the existing integer `id`
  as the internal foreign key.
- The server already accepts caller-independent string ids only where it
  generates them, so the sync client must send its `sync_id` as a
  client-reference field, or the server must accept a client id. Adding an
  `externalId` column to `pos_*` tables is the smallest server change.

Store a mapping table locally:

```sql
CREATE TABLE sync_ids (
  entity TEXT NOT NULL,      -- product, customer, sale, refund, ...
  local_id INTEGER NOT NULL,
  remote_id TEXT,
  PRIMARY KEY (entity, local_id)
);
```

### 2. Outbox

Record every mutation in an append-only outbox in the same local transaction as
the business write:

```sql
CREATE TABLE sync_outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity TEXT NOT NULL,
  local_id INTEGER NOT NULL,
  operation TEXT NOT NULL,   -- create | update | complete | fail | void | refund
  payload TEXT NOT NULL,     -- JSON snapshot or request body
  idempotency_key TEXT NOT NULL UNIQUE,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at INTEGER NOT NULL,
  synced_at INTEGER
);
```

The outbox is drained in order. `idempotency_key` is reused across retries so
the server can replay safely.

### 3. Push

For each pending outbox row, call the matching endpoint with
`Idempotency-Key: <idempotency_key>`:

| Local mutation | Endpoint |
| --- | --- |
| Create business | `PUT /v1/pos/business` (no idempotency needed; upsert) |
| Category create/rename/delete | `POST`/`PUT`/`DELETE /v1/pos/categories` |
| Product create/update/archive | `POST`/`PUT /v1/pos/products`, `POST /products/{id}/active` |
| Stock adjustment/restock | `POST /v1/pos/products/{id}/stock` |
| Customer create/update | `POST`/`PUT /v1/pos/customers` |
| Pending sale | `POST /v1/pos/sales` `{complete:false}` |
| Completed sale | `POST /v1/pos/sales` `{complete:true}` or `POST /sales/{id}/complete` |
| Failed sale | `POST /sales/{id}/fail` |
| Void | `POST /sales/{id}/void` |
| Refund | `POST /sales/{id}/refunds` |

Because the server prices carts and validates stock/points, a push can be
rejected (for example `INSUFFICIENT_STOCK`). The sync client should mark the
row with the error and surface it in a conflict screen rather than silently
dropping the sale.

### 4. Pull

Pull changed rows since the last cursor:

- Add `updated_at` (and a monotonic `version` or `change_seq`) to server rows.
- Expose `GET /v1/pos/changes?since=<ms>&cursor=<id>` returning upserts and
  deletions grouped by entity.
- Apply server rows locally only when the local row is clean (no pending
  outbox entry) or when the server version wins by policy.

Sales and refunds are append-only; they are safe to pull and merge by id.
Catalogue and customers are mutable and need the conflict rules below.

## Conflict resolution

Suggested policy, matching how a single-merchant POS actually behaves:

| Entity | Rule |
| --- | --- |
| Sales, refunds, stock movements | Append-only. Never edit; a correction is a new refund/void/adjustment. Merge by id. |
| Products | Server wins for price/cost if the server row is newer; stock is the sum of movements, never a last-writer-wins field. |
| Categories | Name uniqueness per business. On conflict, keep the local rename and let the server return `CATEGORY_EXISTS` if it clashes. |
| Customers | Merge by phone when present; otherwise last-writer-wins on name. Points are recalculated from sales, not copied. |
| Business profile | Last-writer-wins per field, with the device that has the newest `updated_at`. |

**Stock is the important one.** Do not sync a product's `stock` integer as a
value. Sync the movement ledger (`initial`, `sale`, `refund`, `void`,
`restock`, `adjustment`) and let each side derive stock from movements. That is
why the server stores `pos_stock_movements` with `balance_after` and guards
every stock update.

**Loyalty** should likewise be derived from completed sales, refunds, and
voids rather than synced as a bare number. The server already applies those
deltas atomically and with compare-and-set.

## Ordering and idempotency

- Drain the outbox in `id` order so a sale is never pushed before its products
  and customer.
- Always send `Idempotency-Key`; the server replays the stored response and
  never double-applies a sale or refund.
- On a network timeout, retry the same key. Do not generate a new key.
- If the server returns `IDEMPOTENCY_KEY_REUSED`, the payload changed for an
  already-applied key: treat it as a conflict, not a retry.

## Payments reconciliation

Today the app and the server can both complete the same sale (the app locally,
the server when `saleId` is linked). A sync layer must make this deterministic:

1. Send `saleId` with `POST /v1/pos/payments`.
2. Treat the server's `paid` status as authoritative for the linked sale.
3. On pull, if the server sale is `completed`, settle the local sale with the
   same receipt number and tx hash; do not create a second sale.
4. If only the local sale is completed (offline settlement), push it as a
   completed sale with an idempotency key and let the server adopt it.

The payment status route is already public and safe to poll from multiple
devices.

## What exists on the server to support this

| Need | Server support |
| --- | --- |
| Safe retries | `pos_idempotency` + `Idempotency-Key` on sales/refunds. |
| Concurrent writes | Conditional updates + `pos_guard` + D1 batches. |
| No over-refund | `refunded_qty <= qty`, `refunded_total <= total`. |
| Stock from movements | `pos_stock_movements` ledger. |
| Loyalty from sales | Points applied in the same batch as completion/refund/void. |
| Stable scoping | `business_id` on every row. |
| Receipt identity | Per-business `pos_receipt_counters`. |

## What is still missing for a full sync client

1. Client-generated ids (or an `externalId` column) and a local id map.
2. An outbox table and a background drainer in the app.
3. A `GET /v1/pos/changes` feed with a cursor and `updated_at` on mutable rows.
4. Conflict UI for rejected pushes (insufficient stock/points).
5. A shared implementation of stock/loyalty derivation on both sides. The
   server already derives them; the app derives them locally too, so the two
   must be compared on receipt identity rather than by copying numbers.

Until then, the app keeps working exactly as before, and the API is a correct,
tested server-side implementation that a sync layer can adopt incrementally.
