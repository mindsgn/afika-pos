# afika-balance (Cloudflare Worker)

Scheduled Cloudflare Worker that reads wallet balances from Alchemy, values them
in USD/ZAR/EUR, and stores the results in Cloudflare D1.

- Runtime: Go → WebAssembly (`GOOS=js GOARCH=wasm`) via
  [`syumai/workers-go`](https://github.com/syumai/workers-go)
- Trigger: Cron `0 * * * *` (every hour, on the hour)
- Storage: Cloudflare D1 through the `database/sql` `d1` driver
- Version: `0.1.2` (see `package.json`)

## What it does

1. `FetchWallets` reads every wallet from the D1 `wallets` table.
2. `FetchTokenRegistry` loads networks and ERC-20 tokens from D1, falling back to
   the built-in defaults in `lib/networks.go`.
3. `FetchStockQuotes` loads tokenized-stock prices from D1. The built-in Base
   stock list is always merged in, so a missing registry row cannot drop stocks.
4. For each wallet, one batched Alchemy JSON-RPC request fetches the native
   balance (`eth_getBalance`) and every registered ERC-20/stock balance
   (`alchemy_getTokenBalances`). One subrequest per wallet keeps the run below
   the Workers Free limit of 50 external subrequests per invocation; the
   previous one-call-per-token approach stopped after two wallets and left
   later wallets with no rows. The run processes at most 20 wallets because
   each wallet also costs ~41 D1 writes, and D1 calls count toward the 1,000
   internal-subrequest limit. Above 20 wallets the run processes a rotating
   slice so every wallet is still refreshed.
5. Each balance is valued with the stock price and the latest `fx_rates` rows,
   giving `usd_amount`, `zar_amount`, and `eur_amount`. Stock balances also get
   `ui_amount = amount * multiplier`.
6. Balances, the portfolio summary, and a daily net-worth snapshot are upserted
   into D1. Re-running the worker is idempotent.

## D1 schema

Schema lives in [`migrations/0001_init.sql`](migrations/0001_init.sql).

Input tables (`wallets` is written by `api`, `fx_rates` by `workers/forex`,
`networks`/`tokens`/`stocks` are seeded with SQL):

| Table | Purpose |
| --- | --- |
| `wallets` | `id`, `address`, `network` (`base`, `eth`, `gnosis`, or a canonical key), `testnet` |
| `networks` | network key → Alchemy network, native symbol, chain id |
| `tokens` | ERC-20 tokens per network |
| `stocks` | tokenized-stock contract → symbol, decimals, price, multiplier |
| `fx_rates` | `USD_ZAR`, `USD_EUR`, … |

Output tables (owned by this worker):

| Table | Key | Contents |
| --- | --- | --- |
| `balances` | `(wallet_address, network, token_symbol)` | amount, `ui_amount`, multiplier, USD/ZAR/EUR values, fetch time |
| `portfolio_summaries` | `wallet_address` | net worth, asset-class breakdown, day/week/month/year performance |
| `portfolio_snapshots` | `(wallet_address, day)` | daily net worth used for performance windows |

Balances and portfolios are keyed by the lowercase wallet address. When a
wallet has no address, its `id` is used instead.

## Setup

```bash
npm install

# The worker uses the shared `afika` database declared in wrangler.jsonc.
# Apply the schema (local + remote)
npx wrangler d1 migrations apply afika --local
npx wrangler d1 migrations apply afika --remote

# Store the Alchemy key as a Worker secret
npx wrangler secret put ALCHEMY_API_KEY

# Deploy
npm run deploy
```

Seed the input tables before the first run, for example:

```sql
INSERT INTO wallets (id, address, network, testnet)
VALUES ('user-1', '0xabc...', 'base', 0);

INSERT INTO fx_rates (pair, rate, updated_at)
VALUES ('USD_ZAR', '18.42', 1700000000000),
       ('USD_EUR', '0.92', 1700000000000);
```

`networks`, `tokens`, and `stocks` are optional: when a row is missing the
worker falls back to the defaults compiled into `lib/`. Rows that do exist
override the defaults for that network or contract.

## Local development

```bash
cp .dev.vars.example .dev.vars   # add ALCHEMY_API_KEY
npx wrangler dev --test-scheduled
```

Trigger the cron handler on demand from another terminal:

```bash
curl "http://localhost:8787/__scheduled?cron=0+*+*+*+*"
```

Inspect local D1 state through the Local Explorer:

```bash
curl http://localhost:8787/cdn-cgi/local/explorer/api/d1/database
```

`DB_BINDING` is optional and defaults to `afika`; set it only if the binding in
`wrangler.jsonc` is renamed.

## Testing

```bash
go test ./...
```

The suite runs natively (no Wasm, no network):

- `lib/store_test.go` applies the real migration to in-memory SQLite
  (`modernc.org/sqlite`) and verifies reads, upserts, validation, and snapshots.
- `lib/alchemy_test.go` uses `httptest` to cover JSON-RPC success, RPC errors,
  and per-token skip behaviour.
- `lib/portfolio_test.go`, `lib/stocks_test.go`, `lib/decimal_test.go` cover the
  valuation math and the Base stock list.
- `main_test.go` drives `processBalances` with fake store/fetcher
  implementations.

Build the Wasm bundle without deploying:

```bash
npm run build   # writes build/worker.mjs, build/app.wasm
```

`main_wasm.go` is the Cloudflare entry point (cron + D1). `main_native.go` is a
stub so native builds and tests compile; the worker itself only runs on
Cloudflare.

## Operational notes

- D1 has no interactive transactions in the Go driver. The worker therefore
  writes idempotent upserts and tolerates partial failure per balance.
- The worker runs hourly (`0 * * * *`). Alchemy usage
  scales with wallets × (1 native call + tokens per network).
- `ALCHEMY_API_KEY` is a Worker secret. Never commit `.env` or `.dev.vars`.
- The worker logs a single JSON result per run:
  `[worker-balance] done {"saved":N,"failed":M,"total":N+M}`.
