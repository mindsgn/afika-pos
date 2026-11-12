# afika-transaction (Cloudflare Worker)

Scheduled Cloudflare Worker that syncs wallet transaction history from Alchemy,
enriches transfers with historical USD/ZAR/EUR amounts and fees, and stores the
results in Cloudflare D1.

- Runtime: Go → WebAssembly (`GOOS=js GOARCH=wasm`) via
  [`syumai/workers-go`](https://github.com/syumai/workers-go)
- Trigger: Cron `*/5 * * * *` (every five minutes)
- Storage: Cloudflare D1 through the `database/sql` `d1` driver
- Version: `0.1.2` (see `package.json`)

## What it does

1. `FetchWallets` reads every wallet from D1 together with its transaction
   cursor from `wallet_transaction_cursors`, least-recently-synced first.
2. Each wallet's network is resolved to an Alchemy network key. Only Base
   mainnet (canonical `base-mainnet`) is enabled.
3. One batched JSON-RPC request per wallet carries four
   `alchemy_getAssetTransfers` calls: asset credits, asset debits, stock
   credits, and stock debits. The stock queries restrict Alchemy to the tracked
   Base stock contracts because those tokens are frequently omitted from an
   unfiltered ERC-20 query. Batching keeps the worker at one external
   subrequest per wallet instead of four (Workers Free allows 50 per
   invocation).
4. The batches are merged with duplicates removed by
   `txHash + direction + token address`.
5. Stablecoin transfers (`USDC`, `USDT`, `DAI`) get `usdAmount`, `zarAmount`,
   and `eurAmount`. `ZARP` gets a ZAR amount only. Fees use `feeUsd`, falling
   back to `feeNative`, and are converted to ZAR/EUR the same way.
6. Each run writes at most 150 transfers per wallet and 600 in total. D1 calls
   count as internal subrequests (1,000 per invocation on Workers Free), so a
   wallet with hundreds of transfers would otherwise exhaust the limit before
   later wallets were reached. The cursor advances to the last fully processed
   block, keeping that block whole, and the next run resumes there.
7. Every transfer is upserted into `wallet_transactions`. The wallet cursor is
   advanced only when every selected transfer was written successfully.
8. The worker logs one JSON summary per run:
   `[worker-transaction] done {"saved":N,"failed":M,"total":N+M}`.

Conversion uses the historical Frankfurter rate for the transaction date
(`/v2/rate/{base}/{quote}?date=YYYY-MM-DD`). Successful lookups are memoized in
memory per run, so many transactions on the same date cost one request per
quote.

## D1 schema

Schema lives in
[`migrations/0001_transaction_init.sql`](migrations/0001_transaction_init.sql).

| Table | Key | Contents |
| --- | --- | --- |
| `wallets` | `id` | shared with `afika-api`: `address`, `network`, `testnet` |
| `wallet_transactions` | `(wallet_address, tx_hash, direction)` | transfer details, token, amount, fees, USD/ZAR/EUR amounts, block, timestamps |
| `wallet_transaction_cursors` | `wallet_address` | highest block synced, `updated_at` |

The composite primary key `(wallet_address, tx_hash, direction)` makes
re-running the worker idempotent. A transfer with multiple token movements in
one transaction and direction collapses to one row.

The cursor is a separate table instead of a `latestBlock` column on `wallets`.
`wallets` is written by `api`, and this worker does not alter another
service's table. The cursor is monotonic: a late run can never move it
backwards.

The migration filename is intentionally `0001_transaction_init.sql` rather
than `0001_init.sql`: D1 tracks migrations by name per database, and the shared
`afika` database already uses `0001_init.sql` for the balance schema. It also
declares `wallets` with `CREATE TABLE IF NOT EXISTS`, so the migration works on
a fresh database where the balance migration has not run yet.

## Setup

The worker uses the shared `afika` D1 database already referenced by the balance
and forex workers.

```bash
npm install

# 1. Apply the migration (local + remote)
npx wrangler d1 migrations apply afika --local
npx wrangler d1 migrations apply afika --remote

# 2. Store the Alchemy key as a Worker secret
npx wrangler secret put ALCHEMY_API_KEY

# 3. Deploy
npm run deploy
```

Wallets come from the same `wallets` table read by `workers/balance`:

```sql
INSERT INTO wallets (id, address, network, testnet)
VALUES ('user-1', '0xabc...', 'base', 0);
```

`network` may be `base`, `base-mainnet`, `eth`, or another value understood by
`lib.ResolveNetwork`. Only `base-mainnet` has an Alchemy endpoint enabled, so
other networks are counted as failures each run until they are enabled in
`lib/networks.go`.

## Local development

```bash
cp .dev.vars.example .dev.vars   # add ALCHEMY_API_KEY
npx wrangler dev --test-scheduled
```

Trigger the cron handler on demand from another terminal:

```bash
curl "http://localhost:8787/__scheduled?cron=*/5+*+*+*+*"
```

Inspect local D1 state:

```bash
npx wrangler d1 execute afika --local \
  --command "SELECT wallet_address, tx_hash, direction, token_symbol, usd_amount, zar_amount FROM wallet_transactions LIMIT 20"

npx wrangler d1 execute afika --local \
  --command "SELECT * FROM wallet_transaction_cursors"
```

`DB_BINDING` is optional and defaults to `afika` (the binding name in
`wrangler.jsonc`). Set it only if you rename the binding.

Do not add `"remote": true` to the D1 binding if you want `wrangler dev` to use
the local database created by `--local` migrations; that option makes local dev
connect to the remote database instead.

## Testing

```bash
go test ./...
go test -race ./...
go vet ./...
```

The suite runs natively (no Wasm, no live network):

- `lib/alchemy_test.go` uses `httptest` to cover request filters, credit/debit
  queries, stock contract filters, transfer parsing, RPC errors, HTTP errors,
  invalid JSON, and network/API-key validation.
- `lib/forex_test.go` covers the historical Frankfurter endpoint, 404/5xx
  handling, date formatting, and the per-run rate cache.
- `lib/store_test.go` applies the real migration to in-memory SQLite
  (`modernc.org/sqlite`) and verifies wallet/cursor reads, upserts, validation,
  normalization, timestamp defaults, and monotonic cursors.
- `lib/amount_test.go` and `lib/stocks_test.go` cover amount parsing, the Base
  stock allowlist, stock decimals, and transfer merge/dedupe behaviour.
- `main_test.go` drives `processTransactions` with fake store/fetcher
  implementations, including partial Alchemy failures, stock failures,
  upsert failures, EUR rate failures, default timestamps, and cursor updates.

Build the Wasm bundle without deploying:

```bash
npm run build   # writes build/worker.mjs, build/app.wasm
```

`main_wasm.go` is the Cloudflare entry point (cron + D1 + Workers `fetch`).
`main_native.go` is a stub so native builds and tests compile; the worker itself
only runs on Cloudflare.

`net/http`'s built-in js/wasm transport cannot call Workerd's `fetch` directly
(it panics with `Illegal invocation`), so the production entry point injects
`fetch.NewClient().HTTPClient(...)` into the Alchemy and Frankfurter fetchers.

## Operational notes

- **Subrequests:** each wallet costs one batched Alchemy request. Historical FX
  adds up to 2 Frankfurter requests per distinct `(quote, date)` pair in a run
  (memoized in memory). On the Workers Free plan the 50-subrequest-per-invocation
  limit still applies; above 45 wallets the worker processes a rotating slice
  and relies on the per-wallet cursor to catch up over later runs.
- Alchemy returns at most 1000 transfers per query. The worker asks for
  `order: desc`, so a very large first sync may need a lower starting block or
  manual backfill.
- A wallet batch does not advance its cursor if any upsert fails, so failed
  rows are retried on the next run.
- `fx_rates` written by `workers/forex` is not used here; transaction rows need
  the rate for their own historical date.
- Historical rate failures still save the transaction with the amounts that
  could be derived. For example, if EUR fails but ZAR succeeds, the row keeps
  its ZAR amount and leaves EUR empty.
- `ALCHEMY_API_KEY` is a Worker secret. Never commit `.env` or `.dev.vars`.
- The worker only queries Base mainnet, the only network enabled in
  `lib/networks.go`.
