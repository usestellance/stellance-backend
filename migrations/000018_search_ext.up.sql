CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX invoice_search_idx ON invoice USING gin (
    invoice_number gin_trgm_ops,
    payer_email gin_trgm_ops,
    title gin_trgm_ops,
    payer_name gin_trgm_ops,
    template_id gin_trgm_ops
);
