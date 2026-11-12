# afika-forex (Cloudflare Worker)

Scheduled Cloudflare Worker that fetches foreign-exchange rates from
[Frankfurter](https://frankfurter.dev/) and stores them in Cloudflare D1.

- Runtime: Go → WebAssembly (`GOOS=js GOARCH=wasm`) via
  [`syumai/workers-go`](https://github.com/syumai/workers-go)
- Trigger: Cron `0 * * * *` (once every hour, on the hour)
- Storage: Cloudflare D1 through the `database/sql` `d1` driver
- Version: `0.1.0` (see `package.json`)

## What it does

1. `FrankfurterFetcher` calls `/v1/latest?base=<BASE>` once for every currency
   in `lib.SupportedCurrencies` (`USD`, `EUR`, `ZAR`).
2. The quote maps are combined into one `pair → rate` map, for example
   `USD/ZAR`, `EUR/USD`, and `ZAR/EUR`. Rates are stored as 8-decimal strings.
3. `processRates` upserts every pair into the D1 `fx_rates` table with the same
   `updated_at` timestamp for the whole run. A failing pair is logged and
   counted without discarding the rest of the run.
4. The worker logs one JSON summary per run:
   `[worker-forex] done {"saved":N,"failed":M,"total":N+M}`.

A full run currently publishes about 87 pairs (29 Frankfurter currencies × 3
bases).

## Pair key convention

`fx_rates.pair` is the uppercase pair with `/` replaced by `_`:

| Display pair | Stored `pair` |
| --- | --- |
| `USD/ZAR` | `USD_ZAR` |
| `EUR/USD` | `EUR_USD` |
| `ZAR/EUR` | `ZAR_EUR` |

This matches the key format read by `workers/balance` (`LatestFXRate`), so the
two workers can share the same database.

## D1 schema

Schema lives in [`migrations/0001_forex_init.sql`](migrations/0001_forex_init.sql).

| Table | Key | Contents |
| --- | --- | --- |
| `fx_rates` | `pair` | `rate` decimal string, `updated_at` Unix milliseconds |

`fx_rates` is created with `CREATE TABLE IF NOT EXISTS`, so it is safe whether
this worker or `workers/balance` creates it first. The migration filename is
intentionally `0001_forex_init.sql` rather than `0001_init.sql`: D1 tracks
migrations by name per database, and the shared `afika` database already uses
`0001_init.sql` for the balance schema.

## Setup

The worker uses the shared `afika` D1 database already referenced by the balance
worker's `wrangler.jsonc`.

```bash
npm install

# From this directory. Apply the forex migration locally and remotely.
npx wrangler d1 migrations apply afika --local
npx wrangler d1 migrations apply afika --remote

# Deploy
npm run deploy
```

If you are using a different database, update the `d1_databases` entry in
`wrangler.jsonc`, then apply the migration to that database name.

## Local development

```bash
npx wrangler dev --test-scheduled
```

Trigger the cron handler on demand from another terminal:

```bash
curl "http://localhost:8787/__scheduled?cron=0+*+*+*+*"
```

Inspect the local D1 rows:

```bash
npx wrangler d1 execute afika --local \
  --command "SELECT pair, rate, updated_at FROM fx_rates WHERE pair IN ('USD_ZAR','USD_EUR')"
```

`DB_BINDING` is optional and defaults to `afika` (the binding name in
`wrangler.jsonc`). Set it only if you rename the binding.

Do not add `"remote": true` to the D1 binding if you want `wrangler dev` to use
the local database created by `--local` migrations; that option makes local dev
connect to the remote database instead.

## Testing

```bash
go test ./...
```

The suite runs natively (no Wasm, no live network):

- `lib/frankfurter_test.go` uses `httptest` to cover multi-base success,
  request-per-base, custom bases, non-2xx responses, invalid JSON, and
  fail-fast behaviour.
- `lib/store_test.go` applies the real migration to in-memory SQLite
  (`modernc.org/sqlite`) and verifies upserts, key normalisation, timestamp
  defaults, validation, and reads.
- `lib/currencies_test.go` covers the supported currency list.
- `main_test.go` drives `processRates` with fake store/fetcher implementations,
  including partial-save failures and the default timestamp.

Build the Wasm bundle without deploying:

```bash
npm run build   # writes build/worker.mjs, build/app.wasm
```

`main_wasm.go` is the Cloudflare entry point (cron + D1 + Workers `fetch`).
`main_native.go` is a stub so native builds and tests compile; the worker itself
only runs on Cloudflare.

`net/http`'s built-in js/wasm transport cannot call Workerd's `fetch` directly
(it panics with `Illegal invocation`), so the production entry point injects
`fetch.NewClient().HTTPClient(...)` from `workers-go` into the Frankfurter
fetcher.

## Operational notes

- The worker performs one HTTP request per base (3 total) plus one D1 write per
  pair (~87). Hourly that is about 2,100 D1 writes/day. On the Workers Free plan
  the 50-subrequest-per-request limit could be exceeded; run this worker on a
  paid plan (or batch the writes) if that limit applies to you.
- `fx_rates` is upserted, so re-running the worker is idempotent.
- Provider failures abort the run before any write, so a partial set of rates is
  never stored as if it were complete. Individual D1 write failures are logged
  and counted in the summary.
- No secrets are required. Never commit `.env` or `.dev.vars`.
