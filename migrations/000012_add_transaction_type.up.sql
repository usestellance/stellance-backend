-- CREATE TYPE transaction_type AS ENUM (
--     'withdrawal',
--     'funding',
--     'payment'
-- );
ALTER TABLE transactions
ALTER COLUMN invoice_id DROP NOT NULL,
ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL,
ADD COLUMN transaction_type transaction_type NOT NULL;