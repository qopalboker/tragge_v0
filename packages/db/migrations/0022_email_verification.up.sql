-- Add email verification columns to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ;

-- Create email verification tokens table
CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for looking up tokens by user_id (for invalidating existing tokens)
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_user_id ON email_verification_tokens(user_id);

-- Index for looking up by token_hash (for validating verification requests)
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_token_hash ON email_verification_tokens(token_hash);

-- Index for cleaning up expired tokens
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_expires_at ON email_verification_tokens(expires_at);

-- Mark all existing users as verified (they registered before this feature)
UPDATE users SET email_verified = TRUE, email_verified_at = NOW() WHERE email_verified = FALSE;

-- Comments
COMMENT ON COLUMN users.email_verified IS 'Whether the user has verified their email address';
COMMENT ON COLUMN users.email_verified_at IS 'Timestamp when the email was verified';
COMMENT ON TABLE email_verification_tokens IS 'Stores email verification tokens with one-time use and expiration';
COMMENT ON COLUMN email_verification_tokens.token_hash IS 'SHA-256 hash of the verification token (raw token is sent to user)';
COMMENT ON COLUMN email_verification_tokens.expires_at IS 'Token expires 24 hours after creation';
COMMENT ON COLUMN email_verification_tokens.used_at IS 'Set when token is used to verify email (prevents reuse)';
