CREATE TYPE recurring_interval AS ENUM ('weekly', 'biweekly', 'monthly', 'quarterly', 'yearly');

CREATE TABLE IF NOT EXISTS recurring_invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    payer_email TEXT NOT NULL,
    payer_name VARCHAR(200),
    country VARCHAR(60),
    currency currency_type NOT NULL DEFAULT 'usdc',
    sub_total NUMERIC(20, 6) NOT NULL,
    service_fee NUMERIC(20, 6) DEFAULT 0,
    total NUMERIC(20, 6) NOT NULL,
    template_id VARCHAR(50),
    note TEXT,
    interval recurring_interval NOT NULL,
    next_run_at TIMESTAMPTZ NOT NULL,
    last_run_at TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT TRUE,
    invoice_items JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_recurring_user_id ON recurring_invoices(user_id);
CREATE INDEX idx_recurring_next_run ON recurring_invoices(next_run_at) WHERE is_active = TRUE;
