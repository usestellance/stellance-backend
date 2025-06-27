CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id UUID NOT NULL REFERENCES invoice(id),
    wallet_id UUID REFERENCES wallets(id),
    transaction_hash VARCHAR(128) UNIQUE,
    amount NUMERIC(20, 6) NOT NULL,
    currency currency_type NOT NULL,
    status transaction_status NOT NULL DEFAULT 'pending',
    network_fee NUMERIC(20, 6),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    token_type currency_type NOT NULL
);
CREATE INDEX idx_transactions_invoice ON transactions(invoice_id);
CREATE INDEX idx_transactions_wallet ON transactions(wallet_id)
WHERE wallet_id IS NOT NULL;
CREATE INDEX idx_transactions_status ON transactions(status);