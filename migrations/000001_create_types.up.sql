CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TYPE permission AS ENUM ('user', 'admin');
CREATE TYPE invoice_status AS ENUM (
    'draft',
    'sent',
    'viewed',
    'paid',
    'overdue',
    'cancelled',
    'refunded'
);
CREATE TYPE currency_type AS ENUM ('usdc', 'xlm');
CREATE TYPE wallet_chain AS ENUM ('stellar');
CREATE TYPE invoice_item_type AS ENUM ('per_hour', 'per_unit');
CREATE TYPE transaction_status AS ENUM (
    'pending',
    'confirmed',
    'failed',
    'aborted',
    'refunded'
);
CREATE TYPE transaction_type AS ENUM (
    'withdrawal',
    'funding',
    'payment'
);