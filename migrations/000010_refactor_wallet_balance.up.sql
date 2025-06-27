-- Remove the old balance column
ALTER TABLE wallets DROP COLUMN IF EXISTS balance;

ALTER TABLE wallets
ADD COLUMN usdc_balance NUMERIC(20, 6) DEFAULT 0 CHECK (usdc_balance >= 0),
ADD COLUMN xlm_balance NUMERIC(20, 6) DEFAULT 0 CHECK (xlm_balance >= 0);