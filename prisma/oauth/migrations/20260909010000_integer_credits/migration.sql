-- Preserve the existing storage scale. Validate existing data without rounding,
-- then forbid fractional credits in balances, orders and ledger entries.
ALTER TABLE wallets ADD CONSTRAINT wallets_whole_credits CHECK (balance % 100 = 0);
ALTER TABLE payment_orders ADD CONSTRAINT payment_orders_whole_credits CHECK (amount % 100 = 0);
ALTER TABLE wallet_entries ADD CONSTRAINT wallet_entries_whole_credits CHECK (amount % 100 = 0 AND balance_after % 100 = 0);
