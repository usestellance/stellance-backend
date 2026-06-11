ALTER TYPE transaction_type ADD VALUE IF NOT EXISTS 'path_payment';

ALTER TABLE wallets
    ADD COLUMN IF NOT EXISTS pin_hash TEXT,
    ADD COLUMN IF NOT EXISTS pin_set_at TIMESTAMPTZ;

ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS source_asset VARCHAR(12),
    ADD COLUMN IF NOT EXISTS source_amount NUMERIC(20, 6);
