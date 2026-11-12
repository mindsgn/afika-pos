-- worker-forex schema (shared `afika` D1 database).
--
-- `fx_rates` is also read by workers/balance. Both workers use the same
-- columns so either one can create the table first:
--   pair        uppercase pair with '/' replaced by '_' (e.g. USD_ZAR)
--   rate        decimal string exactly as published by the provider
--   updated_at  fetch time in Unix milliseconds
--
-- The migration filename intentionally differs from the balance worker's
-- `0001_init.sql` because D1 tracks migrations by name per database.

CREATE TABLE IF NOT EXISTS fx_rates (
  pair TEXT PRIMARY KEY,
  rate TEXT NOT NULL,
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_fx_rates_updated_at ON fx_rates (updated_at);
