-- Existing paid orders remain settled. Only new payments enter the 24-hour queue.
ALTER TABLE wallets ADD COLUMN pending_balance BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wallets ADD CONSTRAINT wallets_pending_balance_valid
  CHECK (pending_balance >= 0 AND pending_balance % 100 = 0 AND balance + pending_balance <= 9000000000000);

CREATE TABLE wallet_settlements (
  order_id UUID PRIMARY KEY REFERENCES payment_orders(id) ON DELETE RESTRICT,
  user_id UUID NOT NULL REFERENCES wallets(user_id) ON DELETE RESTRICT,
  amount BIGINT NOT NULL CHECK (amount > 0 AND amount <= 9000000000000 AND amount % 100 = 0),
  settles_at TIMESTAMPTZ(6) NOT NULL,
  settled_at TIMESTAMPTZ(6),
  CHECK (settled_at IS NULL OR settled_at >= settles_at)
);
CREATE INDEX wallet_settlements_user_id_settles_at_idx ON wallet_settlements(user_id, settles_at);
CREATE INDEX wallet_settlements_due_idx ON wallet_settlements(settles_at, user_id) WHERE settled_at IS NULL;
CREATE INDEX payment_orders_payee_id_created_at_id_idx ON payment_orders(payee_id, created_at DESC, id DESC);
