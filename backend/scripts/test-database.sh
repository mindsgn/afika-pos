#!/usr/bin/env bash
#
# Database integration check for the shared Cloudflare D1 database.
#
# 1. Applies the real Wrangler migrations from every worker to one local D1
#    database (the same database all services use in production).
# 2. Verifies that every expected table exists.
# 3. Seeds one wallet, one FX rate, one balance and one transaction.
# 4. Runs the queries the API and workers rely on.
#
# The script never touches the remote database and needs no Cloudflare login.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workers=(balance forex transaction)

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

for worker in "${workers[@]}"; do
	worker_dir="$repo_root/workers/$worker"
	if [[ ! -x "$worker_dir/node_modules/.bin/wrangler" ]]; then
		echo "==> Installing dependencies for workers/$worker"
		(cd "$worker_dir" && npm ci --no-audit --no-fund --silent)
	fi

	echo "==> Applying workers/$worker migrations"
	(
		cd "$worker_dir"
		CI=1 WRANGLER_SEND_METRICS=false \
			npx wrangler d1 migrations apply afika --local --persist-to "$persist_dir"
	)
done

db_path="$(find "$persist_dir/v3/d1" -type f -name '*.sqlite' ! -name 'metadata.sqlite' | head -n 1)"
if [[ -z "$db_path" ]]; then
	echo "error: Wrangler did not create a local D1 database under $persist_dir" >&2
	exit 1
fi
echo "==> SQLite file: $db_path"

expected_tables=(
	balances
	fx_rates
	networks
	portfolio_snapshots
	portfolio_summaries
	stocks
	tokens
	wallet_transaction_cursors
	wallet_transactions
	wallets
)

for table in "${expected_tables[@]}"; do
	found="$(sqlite3 -batch -noheader "$db_path" \
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = '$table';")"
	if [[ "$found" != "1" ]]; then
		echo "error: missing table '$table'" >&2
		exit 1
	fi
done
echo "==> All expected tables exist"

sqlite3 -batch "$db_path" <<'SQL'
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

assert_value() {
	local expected="$1"
	local sql="$2"
	local actual
	actual="$(sqlite3 -batch -noheader "$db_path" "$sql")"
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

# The transaction worker advances its own cursor.
sqlite3 -batch "$db_path" \
	"INSERT INTO wallet_transaction_cursors (wallet_address, latest_block, updated_at) VALUES ('0x1111111111111111111111111111111111111111', 12345678, 1700000000000);"
assert_value "12345678" \
	"SELECT latest_block FROM wallet_transaction_cursors WHERE wallet_address = '0x1111111111111111111111111111111111111111';"

assert_value "ok" "PRAGMA integrity_check;"
assert_value "" "PRAGMA foreign_key_check;"

echo "==> Database integration checks passed"
