CREATE TABLE IF NOT EXISTS invoice_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id UUID NOT NULL REFERENCES invoice(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE
    SET NULL,
        commenter_name VARCHAR(100) NOT NULL,
        commenter_email VARCHAR(255) NOT NULL,
        comment_text TEXT NOT NULL,
        is_verified BOOLEAN DEFAULT FALSE,
        is_guest BOOLEAN DEFAULT TRUE,
        parent_comment_id UUID REFERENCES invoice_comments(id) ON DELETE CASCADE,
        edited BOOLEAN DEFAULT FALSE,
        edited_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        CONSTRAINT check_guest_user_consistency CHECK (
            (
                user_id IS NULL
                AND is_guest = TRUE
            )
            OR (
                user_id IS NOT NULL
                AND is_guest = FALSE
            )
        )
);

CREATE INDEX idx_comments_invoice_id ON invoice_comments(invoice_id);
CREATE INDEX idx_comments_user_id ON invoice_comments(user_id)
WHERE user_id IS NOT NULL;
CREATE INDEX idx_comments_parent_id ON invoice_comments(parent_comment_id)
WHERE parent_comment_id IS NOT NULL;
CREATE INDEX idx_comments_created_at ON invoice_comments(created_at DESC);
CREATE INDEX idx_comments_email ON invoice_comments(commenter_email);

CREATE TRIGGER set_invoice_comments_updated_at BEFORE
UPDATE ON invoice_comments FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION set_comment_verified_status() RETURNS TRIGGER AS $$ BEGIN IF NEW.user_id IS NOT NULL THEN NEW.is_verified := TRUE;
NEW.is_guest := FALSE;
ELSE NEW.is_verified := FALSE;
NEW.is_guest := TRUE;
END IF;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trigger_set_comment_verified_status BEFORE
INSERT
    OR
UPDATE ON invoice_comments FOR EACH ROW EXECUTE FUNCTION set_comment_verified_status();
COMMENT ON TABLE invoice_comments IS 'Comments/discussions on invoices from both registered users and guests';
COMMENT ON COLUMN invoice_comments.user_id IS 'References registered user (NULL for guest commenters)';
COMMENT ON COLUMN invoice_comments.is_verified IS 'TRUE if commenter is a registered, verified user';
COMMENT ON COLUMN invoice_comments.is_guest IS 'TRUE if commenter is not a registered user';
COMMENT ON COLUMN invoice_comments.parent_comment_id IS 'Reference to parent comment for threaded replies';
COMMENT ON COLUMN invoice_comments.edited IS 'TRUE if comment has been edited after creation';

CREATE OR REPLACE VIEW invoice_comment_stats AS
SELECT invoice_id,
    COUNT(*) as total_comments,
    COUNT(*) FILTER (
        WHERE is_verified = TRUE
    ) as verified_comments,
    COUNT(*) FILTER (
        WHERE is_guest = TRUE
    ) as guest_comments,
    COUNT(*) FILTER (
        WHERE parent_comment_id IS NULL
    ) as top_level_comments,
    COUNT(*) FILTER (
        WHERE parent_comment_id IS NOT NULL
    ) as reply_comments,
    MAX(created_at) as latest_comment_at
FROM invoice_comments
GROUP BY invoice_id;
COMMENT ON VIEW invoice_comment_stats IS 'Aggregated comment statistics per invoice';