-- worker-transaction schema (shared `afika` D1 database).
--
-- The `wallets` table is shared with workers/balance. It is declared with
-- CREATE TABLE IF NOT EXISTS and the same columns so this migration also works
-- on a fresh database when the balance migration has not run yet.
--
-- The migration filename differs from the balance worker's `0001_init.sql`
-- because D1 tracks migrations by name per database.
--
-- Transaction sync state lives in `wallet_transaction_cursors` rather than a
-- column on `wallets`, so this worker never alters a table owned by another
-- worker.

CREATE TABLE IF NOT EXISTS wallets (
  id TEXT PRIMARY KEY,
  address TEXT NOT NULL,
  network TEXT NOT NULL,
  testnet INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_wallets_address ON wallets (address);

CREATE TABLE IF NOT EXISTS wallet_transactions (
  wallet_address TEXT NOT NULL,
  tx_hash TEXT NOT NULL,
  from_address TEXT NOT NULL DEFAULT '',
  to_address TEXT NOT NULL DEFAULT '',
  token_address TEXT NOT NULL DEFAULT '',
  token_type TEXT NOT NULL DEFAULT '',
  token_symbol TEXT NOT NULL DEFAULT '',
  amount TEXT NOT NULL DEFAULT '',
  fee_native TEXT NOT NULL DEFAULT '',
  fee_usd TEXT NOT NULL DEFAULT '',
  fee_zar TEXT NOT NULL DEFAULT '',
  fee_eur TEXT NOT NULL DEFAULT '',
  usd_amount TEXT NOT NULL DEFAULT '',
  zar_amount TEXT NOT NULL DEFAULT '',
  eur_amount TEXT NOT NULL DEFAULT '',
  network TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT '',
  block_number INTEGER NOT NULL DEFAULT 0,
  timestamp_ms INTEGER NOT NULL DEFAULT 0,
  fetched_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (wallet_address, tx_hash, direction)
);

CREATE INDEX IF NOT EXISTS idx_wallet_transactions_wallet_time
  ON wallet_transactions (wallet_address, timestamp_ms DESC);

CREATE INDEX IF NOT EXISTS idx_wallet_transactions_block
  ON wallet_transactions (wallet_address, block_number);

CREATE TABLE IF NOT EXISTS wallet_transaction_cursors (
  wallet_address TEXT PRIMARY KEY,
  latest_block INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL DEFAULT 0
);
