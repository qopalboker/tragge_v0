-- Password reset codes for OTP-based password recovery (replaces link-based flow)
CREATE TABLE IF NOT EXISTS password_reset_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash VARCHAR(64) NOT NULL,
    channel VARCHAR(10) NOT NULL CHECK (channel IN ('sms', 'email')),
    destination VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_password_reset_codes_user_expires
    ON password_reset_codes(user_id, expires_at)
    WHERE used_at IS NULL;

CREATE INDEX idx_password_reset_codes_user_created
    ON password_reset_codes(user_id, created_at DESC);
