-- migrations/002_wallets.sql
CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    currency VARCHAR(3) NOT NULL,
    balance BIGINT NOT NULL DEFAULT 0,
    frozen BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, currency)  -- one wallet per currency per user
);

CREATE INDEX idx_wallets_user ON wallets(user_id);

-- Prevent negative balances at the DB level — defense in depth
ALTER TABLE wallets ADD CONSTRAINT balance_non_negative CHECK (balance >= 0);