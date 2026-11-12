# afika-receipt (Cloudflare Worker)

Scheduled Cloudflare Worker that settles shareable POS checkout links.

- Runtime: Go → WebAssembly (`GOOS=js GOARCH=wasm`) via
  [`syumai/workers-go`](https://github.com/syumai/workers-go)
- Trigger: Cron `* * * * *` (once a minute)
- Storage: Cloudflare D1 (`pos_payments`, `wallet_transactions`)
- Version: `0.1.0` (see `package.json`)

## What it does

Every run:

1. Expires pending `pos_payments` whose `expires_at` has passed
   (`pending → expired`).
2. Loads up to 200 live pending payments, oldest first.
3. For each payment, reads incoming `wallet_transactions` rows for the payment
   address:
   - same token symbol (`USDC`, `ZARP`, `EURC`, `BTC`)
   - same chain (`base-mainnet`/`base`, `eth-mainnet`/`ethereum`)
   - `timestamp_ms >= created_at`
   - not already attached to another paid payment
4. Marks the payment `paid` with the earliest transfer that covers the quoted
   `crypto_amount` (`amount >= crypto_amount`, compared exactly).
5. Logs one JSON summary:
   `[worker-receipt] done {"expired":N,"checked":N,"matched":N,"failed":M}`.

The worker only transitions rows out of `pending`, so re-running it never
rewrites a settled payment, and two runs cannot claim the same transaction.

## What it does not do

Bitcoin is not watched. No current worker writes BTC rows into
`wallet_transactions` (the balance and transaction workers are EVM-only via
Alchemy), so BTC payments stay `pending` until they expire. The matcher is
wired for `bitcoin`/`btc-mainnet` rows, so adding a BTC watcher later requires
no change here. Until then merchants can confirm BTC manually from the POS
checkout screen.

## Rate table

The API quotes crypto amounts, so this worker never converts value. It only
compares asset amounts. Keep `lib/match.go`'s symbol table in sync with
`backend/api/lib/payments.go` (`posAssets`) and
`app/afika-pos/src/domain/payment.ts` (`ZAR_PER_UNIT`) when adding an asset.

## D1 schema

Schema lives in [`migrations/0001_receipt_init.sql`](migrations/0001_receipt_init.sql).
`pos_payments` is also created by `backend/api/migrations/0002_pos_payments.sql`.
Both are `CREATE TABLE IF NOT EXISTS` with identical columns, so either worker
can migrate the shared `afika` database first. The filenames differ because D1
tracks migrations by name per database.

`wallet_transactions` is owned by `workers/transaction`; this worker only reads
it.

## Setup

```bash
npm install

npx wrangler d1 migrations apply afika --local
npx wrangler d1 migrations apply afika --remote

npm run deploy
```

## Local development

```bash
npx wrangler dev --test-scheduled
curl "http://localhost:8787/__scheduled?cron=*+*+*+*+*"
```

Inspect local rows:

```bash
npx wrangler d1 execute afika --local \
  --command "SELECT id, receipt_no, status, asset, tx_hash FROM pos_payments ORDER BY created_at DESC LIMIT 20"
```

## Testing

```bash
go test ./...
```

- `lib/match_test.go` — symbol/network mapping and exact decimal comparisons.
- `lib/store_test.go` — applies the real migration to in-memory SQLite and
  covers expiry, pending selection, credit filtering, transaction reuse, and
  idempotent settlement.
- `main_test.go` — drives `processPayments` with a fake store: exact and
  over-payment, under-payment, unknown assets, per-payment failures, and
  timestamp defaults.

Build the Wasm bundle without deploying:

```bash
npm run build   # writes build/worker.mjs, build/app.wasm
```

## Operational notes

- One D1 read per pending payment per run, capped at 200 pending payments. At
  one run per minute that is at most 288k reads/day; normal POS volume is far
  below that. The cap keeps a backlog from exhausting the per-invocation D1
  limits.
- Matching relies on the `transaction` worker having already recorded the
  credit for the merchant wallet. Register the receiving wallet with the
  balance/transaction workers before accepting online payments.
- Run this worker on the same D1 database as `backend/api` and the transaction
  worker (`afika`).
