CREATE TABLE IF NOT EXISTS comment_reactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    comment_id UUID NOT NULL REFERENCES invoice_comments(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    guest_email VARCHAR(255),
    emoji VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reaction_user UNIQUE (comment_id, user_id, emoji),
    CONSTRAINT uq_reaction_guest UNIQUE (comment_id, guest_email, emoji),
    CONSTRAINT chk_reactor CHECK (user_id IS NOT NULL OR guest_email IS NOT NULL)
);

CREATE INDEX idx_reactions_comment_id ON comment_reactions(comment_id);
