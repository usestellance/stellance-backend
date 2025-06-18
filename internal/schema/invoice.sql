CREATE TYPE invoice_status AS ENUM (
    'draft',
    'sent',
    'viewed',
    'paid',
    'overdue',
    'cancelled',
    'refunded'
);
CREATE TYPE currency_type AS ENUM ('usdc', 'xlm')
CREATE TYPE wallet_chain AS ENUM ('stellar');


CREATE TABLE IF NOT EXISTS invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_number VARCHAR(32) NOT NULL UNIQUE,
    invoice_url TEXT NOT NULL UNIQUE,
    created_by_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    payer_id UUID REFERENCES users(id),
    payer_email TEXT,
    payer_wallet_address VARCHAR(128),
    total NUMERIC(20, 6) NOT NULL CHECK (total >= 0),
    service_fee NUMERIC(20, 6) DEFAULT 0 CHECK (service_fee >= 0),
    amount_payable NUMERIC(20, 6) GENERATED ALWAYS AS (total - service_fee) STORED,
    currency currency_type NOT NULL DEFAULT 'usdc',
    title VARCHAR(200),
    description TEXT,
    status invoice_status NOT NULL DEFAULT 'draft',
    due_date DATE,
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
CREATE INDEX idx_invoices_created_by ON invoices(created_by_id);
CREATE INDEX idx_invoices_payer_id ON invoices(payer_id)
WHERE payer_id IS NOT NULL;
CREATE INDEX idx_invoices_status ON invoices(status);
CREATE INDEX idx_invoices_due_date ON invoices(due_date)
WHERE status IN ('sent', 'viewed');
CREATE INDEX idx_invoices_created_at ON invoices(created_at DESC);
CREATE INDEX idx_invoices_number ON invoices(invoice_number);
CREATE INDEX idx_payer_wallet_address ON invoices(payer_wallet_address);

CREATE TRIGGER set_invoices_updated_at BEFORE
UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();