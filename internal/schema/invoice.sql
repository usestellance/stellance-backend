CREATE TABLE IF NOT EXISTS invoice_counters(
    user_id UUID NOT NULL,
    year INTEGER NOT NULL,
    last_number INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, year)
);
CREATE TABLE IF NOT EXISTS invoice (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_number VARCHAR(50) NOT NULL UNIQUE,
    invoice_url TEXT NOT NULL UNIQUE,
    created_by_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    payer_id UUID REFERENCES users(id),
    approved BOOLEAN DEFAULT FALSE,
    approved_date TIMESTAMPZ,
    rejected BOOLEAN,
    rejected_date TIMESTAMPTZ payer_email TEXT,
    payer_name TEXT,
    payer_wallet_address VARCHAR(128),
    -- to be updated later when the transaction has been confirmed to get payer wallet address
    sub_total NUMERIC(20, 6) NOT NULL CHECK (sub_total >= 0),
    service_fee NUMERIC(20, 6) DEFAULT 0 CHECK (service_fee >= 0),
    total NUMERIC(20, 6) NOT NULL,
    currency currency_type NOT NULL DEFAULT 'usdc',
    title VARCHAR(200),
    status invoice_status NOT NULL DEFAULT 'draft',
    due_date DATE,
    address_country VARCHAR(60),
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT check_paid_status CHECK (
        (
            status = 'paid'
            AND paid_at IS NOT NULL
        )
        OR (
            status != 'paid'
            AND paid_at IS NULL
        )
    )
);
CREATE INDEX idx_invoice_counter_user_year ON invoice_counters(user_id, year);
CREATE INDEX idx_invoice_created_by ON invoice(created_by_id);
CREATE INDEX idx_invoice_url ON invoice(invoice_url);
CREATE INDEX idx_invoice_payer_id ON invoice(payer_id)
WHERE payer_id IS NOT NULL;
CREATE INDEX idx_invoice_status ON invoice(status);
CREATE INDEX idx_invoice_due_date ON invoice(due_date)
WHERE status IN ('sent', 'viewed');
CREATE INDEX idx_invoice_created_at ON invoice(created_at DESC);
CREATE INDEX idx_invoice_number ON invoice(invoice_number);
CREATE INDEX idx_payer_wallet_address ON invoice(payer_wallet_address);
CREATE TRIGGER set_invoice_updated_at BEFORE
UPDATE ON invoice FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();