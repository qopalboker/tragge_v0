-- Add profile fields to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS username VARCHAR(50);
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url VARCHAR(500);
ALTER TABLE users ADD COLUMN IF NOT EXISTS bio VARCHAR(500);
ALTER TABLE users ADD COLUMN IF NOT EXISTS country VARCHAR(2); -- ISO 3166-1 alpha-2 country code
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone VARCHAR(20);
ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Create unique index for username (only for non-null values)
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username) WHERE username IS NOT NULL;

-- Create index for searching by country
CREATE INDEX IF NOT EXISTS idx_users_country ON users(country) WHERE country IS NOT NULL;

-- Create trigger to automatically update updated_at timestamp
CREATE TRIGGER set_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- Comments
COMMENT ON COLUMN users.username IS 'Unique username for display (3-30 alphanumeric + underscores)';
COMMENT ON COLUMN users.display_name IS 'Display name shown publicly (2-100 chars)';
COMMENT ON COLUMN users.avatar_url IS 'URL or base64 data URI for user avatar';
COMMENT ON COLUMN users.bio IS 'Short user biography (max 500 chars)';
COMMENT ON COLUMN users.country IS 'ISO 3166-1 alpha-2 country code (e.g., US, GB, IR)';
COMMENT ON COLUMN users.phone IS 'Phone number in E.164 format';
COMMENT ON COLUMN users.updated_at IS 'Timestamp of last profile update';
