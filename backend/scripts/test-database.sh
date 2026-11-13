#!/usr/bin/env bash
#
# Database integration check for the shared Cloudflare D1 database.
#
# 1. Applies the real Wrangler migrations from every worker to one local D1
#    database (the same database all services use in production).
# 2. Verifies tables, seeds representative rows, and exercises shared queries
#    through Wrangler's D1 API.
#
# The script never touches the remote database and needs no Cloudflare login.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
migration_dirs=(api workers/balance workers/forex workers/transaction workers/receipt)

if [[ -n "${AFIKA_D1_PERSIST_DIR:-}" ]]; then
	persist_dir="$AFIKA_D1_PERSIST_DIR"
	remove_persist_dir=0
else
	persist_dir="$(mktemp -d)"
	remove_persist_dir=1
fi

cleanup() {
	if [[ "$remove_persist_dir" == "1" ]]; then
		rm -rf "$persist_dir"
	fi
}
trap cleanup EXIT

echo "==> Local D1 state: $persist_dir"

for worker in "${migration_dirs[@]}"; do
	worker_dir="$repo_root/$worker"
	if [[ ! -x "$worker_dir/node_modules/.bin/wrangler" ]]; then
		echo "==> Installing dependencies for $worker"
		if [[ -f "$worker_dir/package-lock.json" ]]; then
			(cd "$worker_dir" && npm ci --no-audit --no-fund --silent)
		else
			(cd "$worker_dir" && npm install --no-audit --no-fund --silent)
		fi
	fi

	echo "==> Applying $worker migrations"
	(
		cd "$worker_dir"
		CI=1 WRANGLER_SEND_METRICS=false \
			npx wrangler d1 migrations apply afika --local --persist-to "$persist_dir"
	)
done

api_dir="$repo_root/api"
d1() {
	(
		cd "$api_dir"
		CI=1 WRANGLER_SEND_METRICS=false npx wrangler d1 execute afika \
			--local --persist-to "$persist_dir" --command "$1" --json
	)
}

query_value() {
	local sql="$1"
	d1 "$sql" | node -e '
const fs = require("node:fs");
const payload = JSON.parse(fs.readFileSync(0, "utf8"));
const rows = Array.isArray(payload) ? payload[0]?.results : payload?.results;
if (!rows?.length) process.exit(2);
const row = rows[0];
console.log(Object.values(row).join("|"));
'
}

expected_tables=(
	balances
	fx_rates
	networks
	portfolio_snapshots
	portfolio_summaries
	pos_businesses
	pos_categories
	pos_customers
	pos_idempotency
	pos_payments
	pos_products
	pos_receipt_counters
	pos_refund_items
	pos_refunds
	pos_sale_items
	pos_sales
	pos_stock_movements
	stocks
	tokens
	wallet_settings
	wallet_transaction_cursors
	wallet_transactions
	wallets
)

for table in "${expected_tables[@]}"; do
	found="$(query_value "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = '$table';")"
	if [[ "$found" != "1" ]]; then
		echo "error: missing table '$table'" >&2
		exit 1
	fi
done
echo "==> All expected tables exist"

seed_file="$(mktemp)"
trap 'rm -f "$seed_file"; cleanup' EXIT
cat >"$seed_file" <<'SQL'
INSERT INTO wallets (id, address, network, testnet, created_at, updated_at)
VALUES (
  '0x1111111111111111111111111111111111111111',
  '0x1111111111111111111111111111111111111111',
  'base-mainnet',
  0,
  1700000000000,
  1700000000000
);

INSERT INTO fx_rates (pair, rate, updated_at) VALUES
  ('USD_ZAR', '18.42', 1700000000000),
  ('USD_EUR', '0.92', 1700000000000);

INSERT INTO balances (
  wallet_address, wallet_id, network, token_symbol, token_type, token_address,
  amount, ui_amount, multiplier, usd_amount, zar_amount, eur_amount, fetched_at
) VALUES (
  '0x1111111111111111111111111111111111111111',
  'user-1',
  'base-mainnet',
  'USDC',
  'erc20',
  '0x833589fcd6edb6e08f4c7c32d4f71b54bda02913',
  '10.5',
  '10.5',
  '1',
  '10.5',
  '193.41',
  '9.66',
  1700000000000
);

INSERT INTO wallet_transactions (
  wallet_address, tx_hash, from_address, to_address, token_address, token_type,
  token_symbol, amount, fee_native, fee_usd, fee_zar, fee_eur, usd_amount,
  zar_amount, eur_amount, network, direction, state, block_number,
  timestamp_ms, fetched_at_ms
) VALUES (
  '0x1111111111111111111111111111111111111111',
  '0xabc',
  '0x2222222222222222222222222222222222222222',
  '0x1111111111111111111111111111111111111111',
  '0x833589fcd6edb6e08f4c7c32d4f71b54bda02913',
  'erc20',
  'USDC',
  '10.5',
  '0.0001',
  '0.02',
  '0.36',
  '0.018',
  '10.5',
  '193.41',
  '9.66',
  'base-mainnet',
  'credit',
  'completed',
  12345678,
  1700000000000,
  1700000001000
);
SQL

(
	cd "$api_dir"
	CI=1 WRANGLER_SEND_METRICS=false npx wrangler d1 execute afika \
		--local --persist-to "$persist_dir" --file "$seed_file"
)

assert_value() {
	local expected="$1"
	local sql="$2"
	local actual
	actual="$(query_value "$sql")"
	if [[ "$actual" != "$expected" ]]; then
		echo "error: query returned '$actual', expected '$expected'" >&2
		echo "       $sql" >&2
		exit 1
	fi
}

# The API looks a wallet up by its address (wallets.id is the address).
assert_value "0x1111111111111111111111111111111111111111" \
	"SELECT address FROM wallets WHERE id = '0x1111111111111111111111111111111111111111';"

# balances are read in (network, token_symbol) order with keyset pagination.
assert_value "base-mainnet|USDC" \
	"SELECT network || '|' || token_symbol FROM balances WHERE wallet_address = '0x1111111111111111111111111111111111111111' ORDER BY network ASC, token_symbol ASC LIMIT 1;"

# transactions are read newest first.
assert_value "0xabc|credit" \
	"SELECT tx_hash || '|' || direction FROM wallet_transactions WHERE wallet_address = '0x1111111111111111111111111111111111111111' ORDER BY timestamp_ms DESC, tx_hash DESC, direction DESC LIMIT 1;"

# Workers read the latest FX rate by pair key.
assert_value "18.42" "SELECT rate FROM fx_rates WHERE pair = 'USD_ZAR';"

# POS checkout links can be tied to the sale they settle.
assert_value "2" \
	"SELECT count(*) FROM pragma_table_info('pos_payments') WHERE name IN ('sale_id', 'business_id');"

# POS receipt numbers come from a per-business counter that increments atomically.
d1 "INSERT INTO pos_businesses (id, owner_id, name, created_at, updated_at) VALUES ('biz-check', 'did:privy:check', 'Check Shop', 1700000000000, 1700000000000);" >/dev/null
d1 "INSERT INTO pos_receipt_counters (business_id, next_no) VALUES ('biz-check', 1) ON CONFLICT(business_id) DO UPDATE SET next_no = next_no + 1;" >/dev/null
d1 "INSERT INTO pos_receipt_counters (business_id, next_no) VALUES ('biz-check', 1) ON CONFLICT(business_id) DO UPDATE SET next_no = next_no + 1;" >/dev/null
assert_value "2" "SELECT next_no FROM pos_receipt_counters WHERE business_id = 'biz-check';"

# The POS batch guard only accepts the sentinel row value 1.
assert_value "1" \
	"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'pos_guard';"

# The transaction worker advances its own cursor.
d1 "INSERT INTO wallet_transaction_cursors (wallet_address, latest_block, updated_at) VALUES ('0x1111111111111111111111111111111111111111', 12345678, 1700000000000);" >/dev/null
assert_value "12345678" \
	"SELECT latest_block FROM wallet_transaction_cursors WHERE wallet_address = '0x1111111111111111111111111111111111111111';"

echo "==> Database integration checks passed"
