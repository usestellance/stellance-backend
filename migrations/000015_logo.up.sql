CREATE TABLE IF NOT EXISTS logos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_name VARCHAR(255) NOT NULL,
    file_size BIGINT NOT NULL CHECK (file_size > 0),
    file_type VARCHAR(50) NOT NULL,
    s3_key VARCHAR(500) NOT NULL UNIQUE,
    s3_bucket VARCHAR(100) NOT NULL,
    is_default BOOLEAN DEFAULT FALSE,
    logo_presigned_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT check_valid_image_type CHECK (
        file_type IN (
            'image/png',
            'image/jpeg',
            'image/jpg'
        )
    )
);
CREATE INDEX idx_logo_presigned_url ON logos(logo_presigned_url);
CREATE INDEX idx_logos_user_id ON logos(user_id);
CREATE INDEX idx_logos_is_default ON logos(user_id, is_default)
WHERE is_default = TRUE;
CREATE INDEX idx_logos_created_at ON logos(created_at DESC);
CREATE TRIGGER set_logos_updated_at BEFORE
UPDATE ON logos FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE OR REPLACE FUNCTION ensure_single_default_logo() RETURNS TRIGGER AS $$ BEGIN IF NEW.is_default = TRUE THEN
UPDATE logos
SET is_default = FALSE
WHERE user_id = NEW.user_id
    AND id != NEW.id
    AND is_default = TRUE;
END IF;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER ensure_single_default_logo_trigger BEFORE
INSERT
    OR
UPDATE ON logos FOR EACH ROW
    WHEN (NEW.is_default = TRUE) EXECUTE FUNCTION ensure_single_default_logo();
ALTER TABLE invoice
ADD COLUMN logo_id UUID REFERENCES logos(id) ON DELETE
SET NULL;
CREATE INDEX idx_invoice_logo_id ON invoice(logo_id)
WHERE logo_id IS NOT NULL;
COMMENT ON COLUMN invoice.logo_id IS 'References the logo used for this specific invoice. Each invoice can have one logo.';

ALTER TABLE invoice
ADD COLUMN template_id VARCHAR(20);

CREATE INDEX idx_invoice_template_id ON invoice(template_id);
COMMENT ON COLUMN invoice.template_id IS 'Template ID used for invoice design (e.g., template_001, template_002)';