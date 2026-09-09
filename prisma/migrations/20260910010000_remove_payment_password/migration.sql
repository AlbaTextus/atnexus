ALTER TABLE wallets
    DROP COLUMN payment_password_hash,
    DROP COLUMN failed_attempts,
    DROP COLUMN locked_until;
