ALTER TABLE transactions
    DROP COLUMN IF EXISTS source_asset,
    DROP COLUMN IF EXISTS source_amount;

ALTER TABLE wallets
    DROP COLUMN IF EXISTS pin_hash,
    DROP COLUMN IF EXISTS pin_set_at;
