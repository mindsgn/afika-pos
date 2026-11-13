-- Link checkout links to the POS sale they settle.
--
-- sale_id points at pos_sales.id and is written when the app creates the
-- payment from a pending sale. business_id scopes the payment to one merchant
-- so a confirmed payment completes exactly that sale. Both are optional: the
-- older QR-only flow still works without them.

ALTER TABLE pos_payments ADD COLUMN sale_id TEXT;

ALTER TABLE pos_payments ADD COLUMN business_id TEXT;

CREATE INDEX IF NOT EXISTS idx_pos_payments_sale ON pos_payments (sale_id);

CREATE INDEX IF NOT EXISTS idx_pos_payments_business ON pos_payments (business_id, created_at);
