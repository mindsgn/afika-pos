# Afika

An offline-first point-of-sale app for Android and iOS that takes **USDC, Bitcoin and Ethereum** payments.
Built with Expo, Drizzle ORM, SQLite and TypeScript. Everything lives in a local database on the device.

> **v0.1 simulates payments.** No wallet is connected and no real funds move. Exchange rates are fixed
> placeholders in `src/domain/payment.ts`. You choose whether a payment passes or fails, or let it be random.

---

## Short description

Afika POS for small shops that accept USDC, Bitcoin and Ethereum, with stock, refunds and profit tracking.

## Full description

Afika is a mobile point-of-sale system that runs entirely offline. A cashier builds a cart from a searchable
product catalogue, applies a discount, attaches a loyalty customer, and pays in USDC, BTC or ETH. The sale is
priced in rand, converted to the chosen coin at a simulated rate, and recorded with a transaction hash.

Behind the till sits a full back office: product and category management, inventory with a complete stock
movement history, cost price tracking per sale, and a profit report that accounts for refunds and written-off stock.
Completed sales can be partially or fully refunded, or voided outright, and every action produces a readable
receipt that can be shared as text. All data is stored in SQLite through Drizzle, so the app works with no
signal and no server.

**Features**

- Checkout and cart with stock limits, quantity steppers and live totals
- Payments in USDC / BTC / ETH, simulated: **pass**, **fail** or **random** (failed attempts are logged)
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

Lets a shop sell, refund and track stock and profit offline, taking USDC, Bitcoin or Ethereum.

## Who it's for

Small South African merchants (cafes, market stalls, pop-up shops, spaza-style retailers) who want to accept
crypto at the counter without needing reliable internet or a cloud back office. It is also a reference build
for developers who want a clean offline Expo + Drizzle + SQLite codebase with real tests.

## Why I built it

Most crypto point-of-sale tools assume constant connectivity and a hosted dashboard. Shops here deal with
load-shedding, patchy data and thin margins, so they need something that keeps working offline and shows
them whether they actually made money. I wanted to prove the core till (stock, refunds, voids, loyalty, profit)
can be solid and well tested on its own before adding real wallet and exchange integrations.

## What's new in v0.1

First release.

- Offline SQLite database via Drizzle, created and seeded on first launch
- Cart and checkout with simulated USDC / BTC / ETH payments (pass, fail, random)
- Discounts, loyalty points, refunds and returns, voids
- Receipts for sales and refunds, transaction history
- Product, category and inventory management with stock movement history
- Cost price and profit tracking
- Jest unit tests for domain logic and services, and Maestro end-to-end flows

---

## Getting started

```bash
npm install
npx expo install --fix      # aligns native package versions with your Expo SDK
npx expo run:android        # or: npx expo run:ios   (needs a dev build, not Expo Go)
```

The first launch creates `crypto-pos.db` and seeds three categories, seven products and one demo customer
("Thandi Mokoena", 120 points).

## Tests

**Unit tests (Jest)** run in plain Node against an in-memory SQLite database using the same SQL as the app:

```bash
npm test
```

`better-sqlite3` is a native module; if `npm install` cannot build it, install your platform's build tools first.

| Area | Files |
| --- | --- |
| Money, discounts, cart, loyalty, payment simulation, receipts | `tests/domain/*.test.ts` |
| Catalogue, inventory, checkout, refunds, voids, history, profit | `tests/services/*.test.ts` |

**End-to-end tests (Maestro)** drive the real app. Install [Maestro](https://maestro.mobile.dev), start an
emulator or simulator with the dev build installed, then:

```bash
npm run test:e2e
```

Each flow starts with `clearState: true`, so the seeded data is always the same.

| Flow | Covers |
| --- | --- |
| `01_checkout_success` | cart, USDC payment passes, receipt, stock deducted |
| `02_payment_failed_then_retry` | failed payment, cart kept, failure logged, retry |
| `03_void_sale` | void with reason, stock restored |
| `04_refund_return` | partial refund, restock, movement history, profit report |
| `05_discount_and_loyalty` | percent discount, customer, points redemption |
| `06_product_management` | create product, search, restock, movement history |
| `07_categories` | add and delete categories, deletion guard |
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
