/**
 * Plain SQL that mirrors src/db/schema.ts. Used by the app on first launch and by the
 * Jest tests (in-memory SQLite), so both always run against the same tables.
 * If you change schema.ts, change this too.
 */
export const SCHEMA_SQL = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS categories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS products (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sku TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  image_uri TEXT,
  category_id INTEGER REFERENCES categories(id),
  price INTEGER NOT NULL,
  cost_price INTEGER NOT NULL DEFAULT 0,
  stock INTEGER NOT NULL DEFAULT 0,
  reorder_level INTEGER NOT NULL DEFAULT 0,
  active INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS customers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  phone TEXT UNIQUE,
  loyalty_points INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sales (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  receipt_no TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL,
  status TEXT NOT NULL,
  customer_id INTEGER REFERENCES customers(id),
  subtotal INTEGER NOT NULL,
  discount_type TEXT NOT NULL DEFAULT 'none',
  discount_value INTEGER NOT NULL DEFAULT 0,
  discount_amount INTEGER NOT NULL DEFAULT 0,
  points_redeemed INTEGER NOT NULL DEFAULT 0,
  points_discount INTEGER NOT NULL DEFAULT 0,
  total INTEGER NOT NULL,
  cost_total INTEGER NOT NULL DEFAULT 0,
  payment_method TEXT NOT NULL,
  payment_status TEXT NOT NULL,
  crypto_amount REAL NOT NULL DEFAULT 0,
  crypto_rate REAL NOT NULL DEFAULT 0,
  tx_hash TEXT,
  failure_reason TEXT,
  points_earned INTEGER NOT NULL DEFAULT 0,
  refunded_total INTEGER NOT NULL DEFAULT 0,
  voided_at INTEGER,
  void_reason TEXT
);

CREATE TABLE IF NOT EXISTS sale_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sale_id INTEGER NOT NULL REFERENCES sales(id),
  product_id INTEGER NOT NULL REFERENCES products(id),
  sku TEXT NOT NULL,
  name TEXT NOT NULL,
  qty INTEGER NOT NULL,
  unit_price INTEGER NOT NULL,
  unit_cost INTEGER NOT NULL DEFAULT 0,
  line_discount INTEGER NOT NULL DEFAULT 0,
  refunded_qty INTEGER NOT NULL DEFAULT 0,
  refunded_amount INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS refunds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sale_id INTEGER NOT NULL REFERENCES sales(id),
  created_at INTEGER NOT NULL,
  amount INTEGER NOT NULL,
  reason TEXT,
  restocked INTEGER NOT NULL DEFAULT 1,
  points_reversed INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS refund_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  refund_id INTEGER NOT NULL REFERENCES refunds(id),
  sale_item_id INTEGER NOT NULL REFERENCES sale_items(id),
  qty INTEGER NOT NULL,
  amount INTEGER NOT NULL,
  cost_recovered INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS stock_movements (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  product_id INTEGER NOT NULL REFERENCES products(id),
  type TEXT NOT NULL,
  quantity INTEGER NOT NULL,
  balance_after INTEGER NOT NULL,
  unit_cost INTEGER,
  reference TEXT,
  note TEXT,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS business (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  website TEXT NOT NULL DEFAULT '',
  address TEXT NOT NULL DEFAULT '',
  logo_uri TEXT,
  wallet_address TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS payment_requests (
  id TEXT PRIMARY KEY,
  sale_id INTEGER NOT NULL REFERENCES sales(id),
  receipt_no TEXT NOT NULL,
  asset TEXT NOT NULL,
  network TEXT NOT NULL,
  address TEXT NOT NULL,
  amount_cents INTEGER NOT NULL,
  crypto_amount TEXT NOT NULL,
  payment_url TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  tx_hash TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_products_name ON products(name);
CREATE INDEX IF NOT EXISTS idx_sales_created ON sales(created_at);
CREATE INDEX IF NOT EXISTS idx_sale_items_sale ON sale_items(sale_id);
CREATE INDEX IF NOT EXISTS idx_movements_product ON stock_movements(product_id, created_at);
CREATE INDEX IF NOT EXISTS idx_payment_requests_sale ON payment_requests(sale_id);
`;
