-- Password reset tokens table for forgot password flow
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for looking up tokens by user_id (for invalidating existing tokens)
CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);

-- Index for looking up by token_hash (for validating reset requests)
CREATE INDEX idx_password_reset_tokens_token_hash ON password_reset_tokens(token_hash);

-- Index for cleaning up expired tokens
CREATE INDEX idx_password_reset_tokens_expires_at ON password_reset_tokens(expires_at);

-- Comment on table
COMMENT ON TABLE password_reset_tokens IS 'Stores password reset tokens with one-time use and expiration';
COMMENT ON COLUMN password_reset_tokens.token_hash IS 'SHA-256 hash of the reset token (raw token is sent to user)';
COMMENT ON COLUMN password_reset_tokens.expires_at IS 'Token expires 1 hour after creation';
COMMENT ON COLUMN password_reset_tokens.used_at IS 'Set when token is used to reset password (prevents reuse)';
