-- Add failed_attempts column to email_verification_tokens for OTP verification
-- This supports the 6-digit OTP code flow with max 5 attempts before invalidation

ALTER TABLE email_verification_tokens
    ADD COLUMN IF NOT EXISTS failed_attempts INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN email_verification_tokens.token_hash IS 'SHA-256 hash of the 6-digit verification code';
COMMENT ON COLUMN email_verification_tokens.expires_at IS 'Code expires 10 minutes after creation';
COMMENT ON COLUMN email_verification_tokens.failed_attempts IS 'Number of failed verification attempts. Code invalidated after 5 failures.';
