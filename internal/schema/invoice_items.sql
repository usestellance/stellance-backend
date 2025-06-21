CREATE TABLE IF NOT EXISTS invoice_items(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id UUID NOT NULL REFERENCES invoice(id) ON DELETE CASCADE,
    item_type invoice_item_type NOT NULL,
    description VARCHAR(100),
    quantity INT NOT NULL,
    unit_price NUMERIC(20, 6) NOT NULL CHECK (unit_price > 0),
    discount INT,
    amount NUMERIC(20, 6) NOT NULL CHECK(amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


CREATE INDEX idx_item_type ON invoice_items(item_type);

CREATE TRIGGER set_invoice_item_updated_at BEFORE
UPDATE ON invoice_items FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();