-- Shareable POS checkout links.
--
-- One row per payment request. `id` is a random 128-bit hex token used in the
-- public /pay/{id} URL, so it must stay unguessable. Amounts are ZAR cents;
-- crypto_amount is the asset amount locked at the rate stored by the API.
--
-- Status lifecycle: pending -> paid | expired | failed. The receipt worker
-- (workers/receipt) flips pending rows to paid when a matching on-chain credit
-- is observed, and to expired once expires_at passes.

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
