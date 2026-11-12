# Afika POS

An offline-first point-of-sale app for Android and iOS, with a hosted checkout page for shareable online payments.
Built with Expo, Drizzle ORM, SQLite and TypeScript. Till data lives in a local database on the device; online checkout requests are handled by the Go API on Cloudflare Workers.

> Offline/demo checkout can simulate a payment. Online crypto checkout creates a shareable link and depends on the configured API, receiving wallet, and payment monitoring worker.

---

## Short description

Afika POS for small shops, with offline sales and inventory, returns, profit reports, and shareable online crypto checkout.

## Full description

Afika is an offline-first mobile point-of-sale system for independent South African shops. A cashier builds a cart from a searchable product catalogue, applies a discount, attaches a loyalty customer, and records a sale. For online checkout, the app creates a shareable link where customers can pay with BTC, USDC, ZARP, or EURC on supported networks. Demo mode remains available when the API cannot be reached.

Behind the till sits a full back office: product and category management, inventory with a complete stock
movement history, cost price tracking per sale, and a profit report that accounts for refunds and written-off stock.
Completed sales can be partially or fully refunded, or voided outright, and every action produces a readable
receipt that can be shared as text. Till records live in SQLite through Drizzle, so core selling and stock work without a signal. The Go API stores online payment requests and serves the customer checkout page.

**Features**

- Checkout and cart with stock limits, quantity steppers and live totals
- Payments in BTC, USDC, ZARP, and EURC on supported networks; offline demo fallback (failed attempts are logged)
- Discounts: percentage or fixed rand amount, spread across line items so refunds stay exact
- Loyalty points: earn 1 point per R10 spent, redeem at 10c per point, reversed on refunds and voids
- Refunds and returns: partial or full, per item, with optional restock for damaged goods
- Void transactions: restores stock and loyalty points in one step
- Receipt generation for sales and refunds, shareable as text
- Transaction history with status filters and receipt-number search
- Product search by name, SKU or category
- Product and category management, archive instead of delete so history stays intact
- Inventory: restocks, adjustments, low-stock alerts, valuation at cost and retail
- Stock movement history: every sale, refund, void, restock and adjustment with running balance
- Cost price tracking: unit cost captured on every sale item and every restock
- Profit tracking: revenue, refunds, cost of goods, net profit and margin, by day, week or all time

## What it does, in one line

Lets a shop sell, refund and track stock and profit offline, with shareable online checkout for crypto payments.

## Who it's for

Small South African merchants (cafes, market stalls, pop-up shops, spaza-style retailers) who want to accept
crypto at the counter without needing reliable internet or a cloud back office. It is also a reference build
for developers who want a clean offline Expo + Drizzle + SQLite codebase with real tests.

## Why I built it

Most crypto point-of-sale tools assume constant connectivity and a hosted dashboard. Shops here deal with
load-shedding, patchy data and thin margins, so they need something that keeps working offline and shows
them whether they actually made money. I wanted to prove the core till (stock, refunds, voids, loyalty, profit)
can be solid and well tested on its own before adding real wallet and exchange integrations.

## What's new in v0.3

The backend now serves a POS-focused website, HTMX-enabled customer checkout, and email one-time-code
website sign-in (`/signin`, `/account`) alongside the app's Privy sign-in. The app sends Privy access
tokens for authenticated checkout requests; the API verifies their signature and audience and uses the
Privy Go SDK for the signed-in profile endpoint. The hosted checkout offers the same asset options as the
app and refreshes when the receipt worker confirms a payment.

- Offline SQLite database via Drizzle; optional demo data is available in `src/db/seed.ts`
- Cart and checkout with discounts, loyalty, offline fallback, and shareable online payment requests
- Discounts, loyalty points, refunds and returns, voids
- Receipts for sales and refunds, transaction history
- Product, category and inventory management with stock movement history
- Cost price and profit tracking
- Hosted merchant landing page and shareable online checkout through `backend/api`
- Playwright browser coverage for creating a checkout, selecting a payment asset, and displaying a paid receipt
- Jest unit tests for domain logic and services, and Maestro end-to-end flows

---

## Getting started

```bash
cd app
yarn install
npx expo install --fix      # aligns native package versions with your Expo SDK
npx expo run:android        # or: npx expo run:ios   (needs a dev build, not Expo Go)
```

The first launch creates `crypto-pos.db`. Demo catalog data is optional and is not inserted automatically.

Run the whole stack (Go API, every Cloudflare Worker, and the app) for local testing:

```bash
scripts/dev.sh            # backend + Expo Metro
scripts/dev.sh backend    # backend only
scripts/dev.sh app        # Expo Metro only
```

The backend applies the shared local D1 migrations and serves the API on `http://localhost:8787` plus each
worker on its own port; see `backend/scripts/backend.sh`.

## Tests

**Unit tests (Jest)** run in plain Node against an in-memory SQLite database using the same SQL as the app:

```bash
yarn test
```

Backend unit, browser, and Worker build checks run from `backend/api`:

```bash
cd backend/api
npm test
npm run test:e2e
npm run build
```

`better-sqlite3` is a native module; if `npm install` cannot build it, install your platform's build tools first.

| Area | Files |
| --- | --- |
| Money, discounts, cart, loyalty, payment, receipts | `app/src/domain/*.test.ts` |
| Catalogue, inventory, checkout, refunds, voids, history, profit | `app/src/services/*.test.ts` |

**End-to-end tests (Maestro)** drive the real app. Install [Maestro](https://maestro.mobile.dev), start an
emulator or simulator with the dev build installed, then:

```bash
yarn e2e
```

`scripts/e2e.sh` starts Metro with `EXPO_PUBLIC_E2E=1`, so the email/code sign-in screen accepts the fixed
test code and the flows stay offline; real builds still require a genuine Privy code. It wipes only the
app's SQLite database before each flow. The current flows are `onboarding`, `add-product`, and `sell`;
shared setup steps live in `.maestro/subflows/`, and every flow runs `subflows/sign-in.yaml` before
onboarding.

| Flow | Covers |
| --- | --- |
| `.maestro/onboarding.yaml` | email/code sign-in, business setup and first launch |
| `.maestro/add-product.yaml` | add a product and verify the product list |
| `.maestro/sell.yaml` | checkout a product and verify the receipt |
| `08_product_search_stock_guard` | search filtering, cart stock limit |

## How it works

```
app/                 Expo Router screens (Sell, Products, Stock, History, Insights, cart, receipt)
src/domain/          Pure logic: money, discounts, cart, loyalty, payment simulation, receipts
src/services/        Business rules on top of Drizzle: catalog, inventory, checkout, sales, reports
src/db/              schema.ts (Drizzle), bootstrap.ts (SQL), client.ts (expo-sqlite), seed.ts
src/state/cart.tsx   Cart state shared across screens
src/theme/colors.ts  Colour tokens
tests/               Jest tests
.maestro/            Maestro flows
```

- **Money** is stored as integer cents (ZAR). Crypto amounts are derived from the simulated rate and stored on the sale.
- **Services take a `PosDb`**, so the same code runs on `expo-sqlite` in the app and `better-sqlite3` in Jest.
- **Every multi-step action is one transaction** (checkout, refund, void, restock). A failure rolls everything back.
- **Tables are created from `src/db/bootstrap.ts`.** It mirrors `schema.ts`, so edit both together. `drizzle.config.ts`
  is included if you would rather generate migrations with `drizzle-kit`.

### Business rules

- Loyalty: 1 point per R10 of the amount paid, rounded down. 1 point is worth 10c when redeemed.
- A refund claws back points in proportion to the refunded share. A void restores redeemed points and removes earned points.
- Redeemed points are not returned on a partial refund.
- Only completed sales with no refunds can be voided. Voided, failed and fully refunded sales cannot be refunded again.
- Profit = net revenue (after refunds) minus cost of goods kept by customers. Restocked returns add their cost back;
  returns marked "not restocked" are treated as a write-off.
- Failed payments are recorded in history but never change stock or points.

## Roadmap

- Real wallet connections and live exchange rates
- Printing receipts over Bluetooth
- Multiple staff accounts, shifts and a cash-up report
- Export to CSV and encrypted backup
