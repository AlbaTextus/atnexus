CREATE TABLE wallets (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
    balance BIGINT NOT NULL DEFAULT 0 CHECK (balance BETWEEN 0 AND 9000000000000),
    payment_password_hash VARCHAR(255) NOT NULL DEFAULT '',
    failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    locked_until TIMESTAMPTZ(6),
    updated_at TIMESTAMPTZ(6) NOT NULL DEFAULT NOW()
);

CREATE TABLE payment_orders (
    id UUID PRIMARY KEY,
    client_id VARCHAR(100) NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
    client_name VARCHAR(100) NOT NULL,
    external_order_id VARCHAR(100) NOT NULL,
    payer_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    payee_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title VARCHAR(100) NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0 AND amount <= 9000000000000),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','paid','cancelled')),
    created_at TIMESTAMPTZ(6) NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ(6) NOT NULL,
    paid_at TIMESTAMPTZ(6),
    CONSTRAINT payment_orders_client_id_external_order_id_key UNIQUE (client_id, external_order_id),
    CHECK (payer_id <> payee_id),
    CHECK ((status = 'paid') = (paid_at IS NOT NULL))
);
CREATE INDEX payment_orders_payer_id_created_at_id_idx ON payment_orders(payer_id, created_at DESC, id DESC);

CREATE TABLE wallet_entries (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES wallets(user_id) ON DELETE RESTRICT,
    order_id UUID REFERENCES payment_orders(id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount <> 0 AND amount BETWEEN -9000000000000 AND 9000000000000),
    balance_after BIGINT NOT NULL CHECK (balance_after BETWEEN 0 AND 9000000000000),
    kind VARCHAR(20) NOT NULL CHECK (kind IN ('payment','receipt','adjustment')),
    description VARCHAR(200) NOT NULL,
    actor_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    idempotency_key VARCHAR(100),
    created_at TIMESTAMPTZ(6) NOT NULL DEFAULT NOW(),
    CONSTRAINT wallet_entries_order_id_user_id_key UNIQUE (order_id, user_id),
    CONSTRAINT wallet_entries_actor_id_idempotency_key_key UNIQUE (actor_id, idempotency_key),
    CHECK ((kind = 'adjustment' AND order_id IS NULL AND actor_id IS NOT NULL AND idempotency_key IS NOT NULL)
        OR (kind = 'payment' AND amount < 0 AND order_id IS NOT NULL AND actor_id IS NULL AND idempotency_key IS NULL)
        OR (kind = 'receipt' AND amount > 0 AND order_id IS NOT NULL AND actor_id IS NULL AND idempotency_key IS NULL))
);
CREATE INDEX wallet_entries_user_id_created_at_id_idx ON wallet_entries(user_id, created_at DESC, id DESC);
