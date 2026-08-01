-- Add ban_expires_at column to users table for temporary ban enforcement
ALTER TABLE users ADD COLUMN IF NOT EXISTS ban_expires_at TIMESTAMPTZ;

-- Index for the background sweeper query: find suspended users whose ban has expired
CREATE INDEX idx_users_ban_expires_at
    ON users(ban_expires_at)
    WHERE status = 'suspended' AND ban_expires_at IS NOT NULL;
