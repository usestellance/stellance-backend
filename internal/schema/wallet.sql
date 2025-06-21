CREATE TABLE IF NOT EXISTS wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    address VARCHAR(128) NOT NULL,
    private_key TEXT NOT NULL,
    tag VARCHAR(32),
    chain wallet_chain NOT NULL,
    currency currency_type NOT NULL,
    balance NUMERIC(20, 6) DEFAULT 0,
    is_primary BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_wallet_address UNIQUE(address, chain)
);

CREATE UNIQUE INDEX unique_user_tag ON wallets(user_id, tag)
WHERE tag IS NOT NULL;

CREATE INDEX idx_wallets_user_id ON wallets(user_id);
CREATE INDEX idx_wallets_chain_currency ON wallets(chain, currency);
CREATE INDEX idx_wallets_user_primary ON wallets(user_id, is_primary)
WHERE is_primary = TRUE;

CREATE TRIGGER set_wallets_updated_at BEFORE
UPDATE ON wallets FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();