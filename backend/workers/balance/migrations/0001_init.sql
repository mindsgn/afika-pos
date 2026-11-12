-- worker-balance schema.
--
-- Input tables (written by the API / app sync, read by this worker):
--   wallets, networks, tokens, stocks, fx_rates
-- Output tables (written by this worker):
--   balances, portfolio_summaries, portfolio_snapshots

CREATE TABLE IF NOT EXISTS wallets (
  id TEXT PRIMARY KEY,
  address TEXT NOT NULL,
  network TEXT NOT NULL,
  testnet INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_wallets_address ON wallets (address);

CREATE TABLE IF NOT EXISTS networks (
  key TEXT PRIMARY KEY,
  alchemy_network TEXT NOT NULL,
  native_symbol TEXT NOT NULL,
  chain_id INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tokens (
  network TEXT NOT NULL,
  symbol TEXT NOT NULL,
  contract TEXT NOT NULL,
  decimals INTEGER NOT NULL,
  PRIMARY KEY (network, symbol)
);

CREATE TABLE IF NOT EXISTS stocks (
  address TEXT PRIMARY KEY,
  symbol TEXT NOT NULL,
  decimals INTEGER NOT NULL DEFAULT 8,
  price TEXT NOT NULL DEFAULT '',
  multiplier TEXT NOT NULL DEFAULT '1',
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS fx_rates (
  pair TEXT PRIMARY KEY,
  rate TEXT NOT NULL,
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS balances (
  wallet_address TEXT NOT NULL,
  wallet_id TEXT NOT NULL DEFAULT '',
  network TEXT NOT NULL,
  token_symbol TEXT NOT NULL,
  token_type TEXT NOT NULL DEFAULT '',
  token_address TEXT NOT NULL DEFAULT '',
  amount TEXT NOT NULL DEFAULT '',
  ui_amount TEXT NOT NULL DEFAULT '',
  multiplier TEXT NOT NULL DEFAULT '',
  usd_amount TEXT NOT NULL DEFAULT '',
  zar_amount TEXT NOT NULL DEFAULT '',
  eur_amount TEXT NOT NULL DEFAULT '',
  fetched_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (wallet_address, network, token_symbol)
);

CREATE INDEX IF NOT EXISTS idx_balances_wallet ON balances (wallet_address);

CREATE TABLE IF NOT EXISTS portfolio_summaries (
  wallet_address TEXT PRIMARY KEY,
  net_worth TEXT NOT NULL DEFAULT '',
  usdc_value TEXT NOT NULL DEFAULT '',
  eurc_value TEXT NOT NULL DEFAULT '',
  zarp_value TEXT NOT NULL DEFAULT '',
  stocks_value TEXT NOT NULL DEFAULT '',
  other_value TEXT NOT NULL DEFAULT '',
  day_pct TEXT NOT NULL DEFAULT '',
  week_pct TEXT NOT NULL DEFAULT '',
  month_pct TEXT NOT NULL DEFAULT '',
  year_pct TEXT NOT NULL DEFAULT '',
  day_abs TEXT NOT NULL DEFAULT '',
  week_abs TEXT NOT NULL DEFAULT '',
  month_abs TEXT NOT NULL DEFAULT '',
  year_abs TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS portfolio_snapshots (
  wallet_address TEXT NOT NULL,
  day TEXT NOT NULL,
  net_worth TEXT NOT NULL DEFAULT '',
  at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (wallet_address, day)
);

CREATE INDEX IF NOT EXISTS idx_portfolio_snapshots_wallet ON portfolio_snapshots (wallet_address);
