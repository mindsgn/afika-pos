-- worker-receipt schema (shared `afika` D1 database).
--
-- This migration mirrors backend/api/migrations/0002_pos_payments.sql. Either
-- worker may apply the table first. Both statements are idempotent, and the
-- filenames differ because D1 tracks migrations by name per database.
--
-- `wallet_transactions` is owned by workers/transaction and read here as-is.

CREATE TABLE IF NOT EXISTS pos_payments (
  id TEXT PRIMARY KEY,
  receipt_no TEXT NOT NULL DEFAULT '',
  amount_cents INTEGER NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT 'ZAR',
  status TEXT NOT NULL DEFAULT 'pending',
  asset TEXT NOT NULL DEFAULT 'usdc-base',
  network TEXT NOT NULL DEFAULT 'base',
  address TEXT NOT NULL DEFAULT '',
  crypto_amount TEXT NOT NULL DEFAULT '',
  payment_url TEXT NOT NULL DEFAULT '',
  merchant_name TEXT NOT NULL DEFAULT '',
  merchant_website TEXT NOT NULL DEFAULT '',
  merchant_address TEXT NOT NULL DEFAULT '',
  tx_hash TEXT,
  created_at INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL DEFAULT 0,
  expires_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_pos_payments_status ON pos_payments (status, expires_at);
CREATE INDEX IF NOT EXISTS idx_pos_payments_receipt ON pos_payments (receipt_no);
