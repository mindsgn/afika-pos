# afika-api (Cloudflare Worker)

REST API and hosted POS checkout for Afika. The Go HTTP server is compiled to
Wasm and served by Cloudflare Workers through
[`workers-go`](https://github.com/syumai/workers-go). The root website describes
Afika POS for independent South African shops; `/pay/{id}` is the customer
checkout page generated from requests created by the mobile POS.

The API is a thin read/write layer over the shared `afika` D1 database that the
scheduled workers populate:

| Table | Written by | API access |
| --- | --- | --- |
| `wallets` | this API (`PUT /v1/wallet`) | read + write |
| `balances` | `workers/balance` | read |
| `wallet_transactions` | `workers/transaction` | read |
| `fx_rates` | `workers/forex` | read |
| `wallet_settings` | this API (`PUT /v1/settings`) | read + write |
| `pos_payments` | this API + `workers/receipt` | read + write |

Apply the worker and API migrations before deploying (see
[Migrations](#migrations)).

## Endpoints

All routes are versioned under `/v1` and return JSON.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/v1/health` | Liveness probe. No authentication. |
| `GET` | `/v1/auth/me` | Authenticated Privy user profile via the Privy Go SDK. |
| `PUT` | `/v1/wallet` | Register or update a wallet address. |
| `GET` | `/v1/balances?address=0x…` | Paginated token balances for a wallet. |
| `GET` | `/v1/transactions?address=0x…` | Paginated transaction history for a wallet. |
| `GET` | `/v1/fx-rates` | Latest USD/ZAR and USD/EUR rates written by the forex worker. |
| `POST` | `/v1/pos/payments` | Create a shareable POS checkout link. Authenticated. |
| `GET` | `/v1/pos/payments/{id}` | Read a POS checkout. Public (the id is the payer's token). |
| `POST` | `/v1/pos/payments/{id}/asset` | Payer picks the coin/chain to pay with. Public. |
| `POST` | `/v1/pos/payments/{id}/confirm` | Mark a checkout paid. Authenticated; used by the receipt worker and the app's demo button. |
| `GET` | `/pay/{id}` | Server-rendered checkout page for the payer (HTML, not JSON). |
| `GET` | `/signin` | Server-rendered website sign-in, email step (HTML). |
| `POST` | `/signin` | Email a one-time code via Privy. |
| `POST` | `/signin/verify` | Exchange the code for an HttpOnly session cookie. |
| `POST` | `/signout` | Clear the session cookie. |
| `GET` | `/account` | Signed-in account page; redirects to `/signin` without a session. |

## Tests

Run API unit tests and browser checkout coverage from this directory:

```bash
npm test
npm run test:e2e
```

The Playwright test starts the API with `wrangler dev` and a local D1 binding,
applies the API migrations to D1, creates a payment through the Worker, selects
an asset in Chromium, and checks the paid state after confirmation. `E2E_MODE`
enables a fake Privy that accepts the code `000000`; the harness sets it only
for the local test Worker.
Install the browser once with `npx playwright install chromium`.

### `PUT /v1/wallet`

```http
PUT /v1/wallet
Authorization: Bearer <api-token>
Content-Type: application/json

{
  "address": "0x1111111111111111111111111111111111111111",
  "network": "base"
}
```

- `address` is required and must be a `0x`-prefixed 40-character hex string. It
  is stored lowercased and becomes the `wallets.id` row key.
- `network` is optional and defaults to `base-mainnet`. Accepted aliases are
  normalised to the canonical worker keys: `base` → `base-mainnet`,
  `ethereum`/`eth` → `eth-mainnet`, `xdai`/`gnosis` → `gnosis-mainnet`. Testnets
  (`base-sepolia`, `eth-sepolia`, `gnosis-chiado`) set `testnet` automatically.
- `201 Created` on the first registration, `200 OK` when an existing row was
  updated. `createdAt` is preserved across updates.

```json
{
  "data": {
    "address": "0x1111111111111111111111111111111111111111",
    "network": "base-mainnet",
    "testnet": false,
    "createdAt": 1793227459208,
    "updatedAt": 1793227459227
  }
}
```

`wallets.id` is the lowercased address, so registering the same address again
updates it in place. The balance and transaction workers pick the change up on
their next run.

### `GET /v1/balances`

Balances are ordered by `(network, tokenSymbol)` and paginated with an opaque
keyset cursor. `address` is required.

```http
GET /v1/balances?address=0x1111111111111111111111111111111111111111&limit=50
GET /v1/balances?address=0x1111111111111111111111111111111111111111&limit=50&cursor=eyJ2IjoxLCJyIjoiYmFsYW5jZXMiLC...
```

```json
{
  "data": [
    {
      "walletAddress": "0x1111111111111111111111111111111111111111",
      "walletId": "0x1111111111111111111111111111111111111111",
      "network": "base-mainnet",
      "tokenType": "erc20",
      "tokenSymbol": "USDC",
      "tokenAddress": "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
      "amount": "10.5",
      "uiAmount": "10.5",
      "multiplier": "1",
      "usdAmount": "10.5",
      "zarAmount": "189",
      "eurAmount": "9.66",
      "fetchedAt": 1700000000000
    }
  ],
  "pagination": {
    "limit": 50,
    "hasMore": true,
    "nextCursor": "eyJ2IjoxLCJyIjoiYmFsYW5jZXMiLC..."
  }
}
```

Amounts are strings so decimal precision is never lost in JSON.

### `GET /v1/transactions`

Transactions are ordered newest first by
`(timestampMs DESC, txHash DESC, direction DESC)`. `address` is required.

```http
GET /v1/transactions?address=0x1111111111111111111111111111111111111111&limit=50
```

```json
{
  "data": [
    {
      "walletAddress": "0x1111111111111111111111111111111111111111",
      "txHash": "0xabc...",
      "fromAddress": "0xfrom...",
      "toAddress": "0xto...",
      "tokenAddress": "0x833589...",
      "tokenType": "erc20",
      "tokenSymbol": "USDC",
      "amount": "3",
      "feeNative": "0.0001",
      "feeUsd": "0.02",
      "feeZar": "0.36",
      "feeEur": "0.018",
      "usdAmount": "3",
      "zarAmount": "54",
      "eurAmount": "2.76",
      "network": "base-mainnet",
      "direction": "debit",
      "state": "completed",
      "blockNumber": 12345678,
      "timestampMs": 1700000000000,
      "timestamp": 1700000000000,
      "fetchedAtMs": 1700000001000,
      "fetchedAt": 1700000001000
    }
  ],
  "pagination": { "limit": 50, "hasMore": false }
}
```

`timestamp` and `fetchedAt` duplicate the millisecond fields for compatibility
with the Expo app.

## POS checkout

The POS app (`app/afika-pos`) rings up a sale, creates a payment here and shows
`paymentUrl` as a QR code. The payer opens the link in a browser (or the app),
picks a coin and chain, and pays the merchant's address. `workers/receipt`
watches the chain and confirms the payment; the app polls
`GET /v1/pos/payments/{id}` until it flips to `paid` and then finalises the
local sale exactly once.

### `POST /v1/pos/payments`

```http
POST /v1/pos/payments
Authorization: Bearer <api-token>
Content-Type: application/json

{
  "receiptNo": "R-000123",
  "amountCents": 3200,
  "currency": "ZAR",
  "asset": "usdc-base",
  "receiveAddress": "0x1111111111111111111111111111111111111111",
  "merchant": {
    "name": "Afika Coffee",
    "website": "https://afika.app",
    "address": "12 Long Street, Cape Town"
  }
}
```

- `receiptNo` (required, max 64 chars) and `amountCents` (required, positive,
  ZAR cents) identify the sale. `currency` must be `ZAR`.
- `asset` is one of `btc`, `usdc-base`, `usdc-ethereum`, `zarp-base`,
  `zarp-ethereum`, `eurc-base`, `eurc-ethereum` and defaults to `usdc-base`.
  The ZAR-per-unit rates live in `posAssets` (`lib/payments.go`) and must stay
  in sync with `app/afika-pos/src/domain/payment.ts` and
  `workers/receipt/lib/match.go`.
- `receiveAddress` is the merchant's wallet. When it is empty the API falls
  back to the `POS_RECEIVING_ADDRESS` variable.
- `201 Created` with the payment, whose `paymentUrl` is
  `<checkout-base-url>/pay/{id}` and whose `cryptoAmount` is quoted at the
  table rate. A payment expires 30 minutes after creation.

```json
{
  "data": {
    "id": "9f2c1e4f0a4b4c2f9d3a1f6b7c8d9e0f",
    "receiptNo": "R-000123",
    "amountCents": 3200,
    "currency": "ZAR",
    "status": "pending",
    "asset": "usdc-base",
    "network": "base",
    "address": "0x1111111111111111111111111111111111111111",
    "cryptoAmount": "1.73",
    "paymentUrl": "https://afika.phumzile.studio/pay/9f2c1e4f0a4b4c2f9d3a1f6b7c8d9e0f",
    "merchantName": "Afika Coffee",
    "merchantWebsite": "https://afika.app",
    "merchantAddress": "12 Long Street, Cape Town",
    "createdAt": 1793227459208,
    "updatedAt": 1793227459208,
    "expiresAt": 1793229259208
  }
}
```

### `GET /v1/pos/payments/{id}`

Public, so both the payer's browser and the POS app can poll it. Reading a
pending payment whose `expiresAt` has passed returns it as `expired`, so a
stale quote can never be paid before the receipt worker next runs.

### `POST /v1/pos/payments/{id}/asset`

Public. Body `{"asset": "zarp-ethereum"}` re-quotes `cryptoAmount`, `network`
and `address` for the payer's chosen coin while the payment is still pending.

### `POST /v1/pos/payments/{id}/confirm`

Authenticated. Body `{"txHash": "0x…"}` marks a pending payment `paid`.
Confirming an already-paid payment is a no-op that returns the stored row, so
retries from the worker and the app are safe.

### `GET /pay/{id}`

The shareable page itself: merchant details, ZAR amount and a form per asset.
Selecting an asset POSTs to the same URL and comes back as a `303` redirect, so
the page works without JavaScript. An HTMX status check polls
`GET /pay/{id}/status`; once payment completes, `HX-Redirect` reloads the page
into its paid receipt state.

## Pagination

Both list endpoints share the same contract:

- `address` — required. A `0x`-prefixed 40-character hex address. It is
  normalised to lowercase before the lookup.
- `limit` — integer `1..100`, default `50`.
- `cursor` — the `pagination.nextCursor` value from the previous page. Opaque:
  do not parse or construct it. Cursors are resource-scoped, so a balance cursor
  sent to `/v1/transactions` is rejected.
- `pagination.hasMore` — `true` when another page exists.
- `pagination.nextCursor` — omitted on the last page.

Unknown query parameters are rejected instead of ignored.

## Authentication

Requests to protected API routes such as `/v1/wallet`, `/v1/balances`, and
`/v1/transactions` require a valid legacy API token or configured Privy access token:

```http
Authorization: Bearer <api-token>
```

The API accepts the configured legacy `API_TOKEN` and, when configured, Privy access tokens. Privy tokens are verified against the app's JWKS endpoint and must match `PRIVY_APP_ID`; `GET /v1/auth/me` uses the Privy Go SDK and `PRIVY_APP_SECRET` to fetch the signed-in user profile. The legacy token is compared in
constant time. It carries no user identity: wallet, balance, and transaction
lookups are keyed by the `address` in the request. `GET /v1/health` is the only
route that does not require a token.

If both `API_TOKEN` and the Privy verifier settings are empty, authenticated
routes return `401`. Configure `PRIVY_APP_ID` for app sessions. The verifier
fetches signing keys from the app-specific JWKS endpoint configured in
`main_wasm.go` and refreshes its key cache periodically. Keep `API_TOKEN` for the receipt worker and any legacy clients;
set it locally in `.dev.vars` and in production with `wrangler secret put API_TOKEN`.

## Website sign-in

The site is server-rendered, so sign-in is a normal form flow with no client
JavaScript:

1. `GET /signin` renders the email form.
2. `POST /signin` validates the address and asks Privy to email a one-time code
   (`POST https://auth.privy.io/api/v1/passwordless/init`).
3. `POST /signin/verify` exchanges the code for a session
   (`/api/v1/passwordless/authenticate`), then sets an `afika_session` cookie
   and redirects to `/account`.
4. `GET /account` shows the signed-in email and Privy user id; `POST /signout`
   clears the cookie.

The cookie is `HttpOnly`, `SameSite=Lax`, and signed with HMAC-SHA256 over the
Privy user id and an expiry (`SESSION_SECRET`); it carries no other data and is
valid for 24 hours. It is marked `Secure` everywhere except the local Playwright
server.

The pages need `PRIVY_APP_ID` and `SESSION_SECRET`; without them `/signin`
reports that sign-in is not configured (`503`). Reading the profile on
`/account` additionally needs `PRIVY_APP_SECRET`. When the Privy app uses an app
client, set `PRIVY_CLIENT_ID` to that client id and `PRIVY_NATIVE_APP_ID` to a
native app identifier allowed for it (for the Expo app, `phumzile.afika.pos`).
Both values are also sent on `/api/v1/passwordless/authenticate`.

## Errors

Errors use one envelope:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "address must be a 0x-prefixed 40-character hex string",
    "requestId": "867df05020e36a79"
  }
}
```

| Status | Code | When |
| --- | --- | --- |
| 400 | `VALIDATION_ERROR` | Invalid field, missing `address`, query parameter, or unknown JSON field |
| 400 | `INVALID_JSON` | Body is not a single JSON object |
| 400 | `INVALID_CURSOR` | Cursor is malformed or belongs to another endpoint |
| 401 | `UNAUTHENTICATED` | Missing, malformed, or incorrect API token |
| 403 | `ORIGIN_NOT_ALLOWED` | Browser `Origin` is not in `API_ALLOWED_ORIGINS` |
| 404 | `NOT_FOUND` | Unknown route |
| 405 | `METHOD_NOT_ALLOWED` | Wrong method; response includes `Allow` |
| 413 | `PAYLOAD_TOO_LARGE` | Body exceeds 64 KiB |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | `Content-Type` is not `application/json` |
| 500 | `INTERNAL_ERROR` | Unexpected failure; details are logged, not returned |
| 504 | `REQUEST_TIMEOUT` | D1 call exceeded the 10 second request budget |

Every response carries `X-Request-Id` (echoed from the request when it matches
`[A-Za-z0-9._-]{1,64}`) and `Cache-Control: no-store`.

## Configuration

Set these as Worker variables/secrets (`wrangler secret put`, or `vars` in
`wrangler.jsonc`):

| Variable | Default | Purpose |
| --- | --- | --- |
| `API_TOKEN` | empty | Legacy bearer token used by the receipt worker and older clients |
| `PRIVY_APP_ID` | empty | Privy app audience for access-token verification and profile API |
| `PRIVY_APP_SECRET` | empty | Privy Go SDK credential for fetching the authenticated user profile |
| `PRIVY_CLIENT_ID` | empty | Privy app client id (same as `privyClientId` in `app/app.json`); sent with passwordless requests |
| `PRIVY_NATIVE_APP_ID` | empty | Native app identifier allowed for `PRIVY_CLIENT_ID` (e.g. `phumzile.afika.pos`); sent as `x-native-app-identifier` |
| `SESSION_SECRET` | empty | HMAC key for the website session cookie. Empty disables `/signin` and `/account` |
| `DB_BINDING` | `afika` | D1 binding name declared in `wrangler.jsonc` |
| `API_ALLOWED_ORIGINS` | empty | Comma-separated browser origins, or `*`. Empty disables CORS headers |
| `POS_RECEIVING_ADDRESS` | empty | Fallback merchant wallet for POS payments that do not send `receiveAddress` |
| `CHECKOUT_BASE_URL` | request origin | Origin used to build shareable `/pay/{id}` links (set this when the checkout page is served from another host) |

The D1 binding itself is declared in `wrangler.jsonc`:

```jsonc
"d1_databases": [
  {
    "binding": "afika",
    "database_name": "afika",
    "database_id": "b806cd75-548d-4abc-be85-213a7ba18c6a"
  }
]
```

## Development

```console
cd api
npm ci
npm test        # go test ./...
npm run test:e2e # Playwright checkout browser flow
npm run build   # compile the Wasm bundle into ./build
npm run dev     # wrangler dev on http://localhost:8787
npm run deploy  # wrangler deploy
```

`npm run build` regenerates `build/worker.mjs` and compiles `build/app.wasm`,
which is what `wrangler.jsonc` points at. `build/` is git ignored.

Unit tests are table-driven and never touch the network. D1 integration checks
and browser tests use Wrangler's local D1 database in local mode and do not
access the remote database.

```console
go test ./...
go test -race ./...
go vet ./...
```

### Local end-to-end

Create `.dev.vars` from the example:

```console
cp .dev.vars.example .dev.vars
```

`.dev.vars.example` sets `API_TOKEN=dev-token`. Apply the worker migrations to
the local D1 emulation, then start the dev server:

```console
# from api/
(cd ../workers/balance && npx wrangler d1 migrations apply afika --local)
(cd ../workers/forex && npx wrangler d1 migrations apply afika --local)
(cd ../workers/transaction && npx wrangler d1 migrations apply afika --local)
(cd ../workers/receipt && npx wrangler d1 migrations apply afika --local)
(cd . && npx wrangler d1 migrations apply afika --local)
npm run dev
```

Or start the API and every worker together (one shared local D1 directory, all
migrations applied) with `backend/scripts/backend.sh run`, or the whole stack
including the Expo app with `scripts/dev.sh` from the repository root.

Each worker keeps its own `.wrangler/` state, so use `--persist-to <dir>` on all
commands if you want the API and workers to share one local database.

```console
curl -X PUT http://localhost:8787/v1/wallet \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{"address":"0x1111111111111111111111111111111111111111","network":"base"}'

curl 'http://localhost:8787/v1/balances?address=0x1111111111111111111111111111111111111111&limit=2' \
  -H 'Authorization: Bearer dev-token'

curl 'http://localhost:8787/v1/transactions?address=0x1111111111111111111111111111111111111111&limit=2' \
  -H 'Authorization: Bearer dev-token'
```

## Migrations

Apply the shared worker schema and API-owned `wallet_settings` schema once per
environment, before the first deploy. Run the commands from `backend/`:

```console
# from the repository root (backend/)
(cd workers/balance && npx wrangler d1 migrations apply afika --remote)
(cd workers/forex && npx wrangler d1 migrations apply afika --remote)
(cd workers/transaction && npx wrangler d1 migrations apply afika --remote)
(cd workers/receipt && npx wrangler d1 migrations apply afika --remote)
(cd api && npx wrangler d1 migrations apply afika --remote)
```

## Deploy

```console
cd api
npx wrangler secret put API_TOKEN
npx wrangler secret put API_ALLOWED_ORIGINS   # optional
npm run deploy
```

Wrangler runs `npm run build` first (configured in `wrangler.jsonc`). Verify with:

```console
curl https://afika-api.<account>.workers.dev/v1/health
```

## Layout

```
api/
├── main_wasm.go   # Worker entrypoint: env wiring, D1 client
├── main_native.go # Stub so native `go build`/`go test` work
├── lib/
│   ├── handler.go    # routes, auth, pagination envelope, errors, CORS
│   ├── store.go      # D1 wallet upsert, balance/transaction keyset queries
│   ├── auth.go       # shared API token verification
│   ├── pagination.go # opaque cursor encode/decode
│   ├── validation.go # address and network normalisation
│   ├── model.go      # API response models
│   ├── payments.go   # POS payment model, D1 methods, HTTP handlers
│   ├── pay_page.go   # server-rendered /pay/{id} checkout page
│   ├── session.go    # signed website session cookie codec
│   ├── privy_passwordless.go # Privy email one-time-code HTTP client
│   └── auth_site.go  # server-rendered /signin and /account pages
└── wrangler.jsonc

`migrations/0002_pos_payments.sql` creates the `pos_payments` table.
```
