-- Per-wallet preferences owned by the API.
--
-- One row per wallet, keyed by the lowercased wallet address. The balance and
-- transaction endpoints read preferred_currency when the request does not pass
-- an explicit `currency` query parameter.

CREATE TABLE IF NOT EXISTS wallet_settings (
  wallet_address TEXT PRIMARY KEY,
  preferred_currency TEXT NOT NULL DEFAULT 'USD',
  created_at INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL DEFAULT 0
);
