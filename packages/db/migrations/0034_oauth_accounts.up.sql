-- 0034_oauth_accounts.up.sql
-- Add OAuth accounts support for social login providers

-- ============================================================================
-- OAUTH PROVIDER ENUM
-- ============================================================================

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'oauth_provider') THEN
        CREATE TYPE oauth_provider AS ENUM (
            'google',
            'github',
            'facebook',
            'apple',
            'discord'
        );
    END IF;
END$$;

-- ============================================================================
-- OAUTH ACCOUNTS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS oauth_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider oauth_provider NOT NULL,
    provider_user_id VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    access_token TEXT,
    refresh_token TEXT,
    token_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- CONSTRAINTS
-- ============================================================================

-- Unique constraint on (provider, provider_user_id) to prevent duplicate OAuth accounts
ALTER TABLE oauth_accounts ADD CONSTRAINT uq_oauth_provider_user_id
    UNIQUE (provider, provider_user_id);

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Index on provider_user_id for fast lookups during OAuth callback
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_provider_user_id
    ON oauth_accounts(provider_user_id);

-- Index on user_id for finding all OAuth accounts for a user
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_user_id
    ON oauth_accounts(user_id);

-- Composite index for provider + provider_user_id lookups
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_provider_lookup
    ON oauth_accounts(provider, provider_user_id);

-- Index on email for looking up accounts by email
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_email
    ON oauth_accounts(email)
    WHERE email IS NOT NULL;

-- ============================================================================
-- MODIFY USERS TABLE: MAKE password_hash NULLABLE
-- ============================================================================

-- Allow password_hash to be NULL for OAuth-only users
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

-- ============================================================================
-- TRIGGER FOR UPDATED_AT
-- ============================================================================

CREATE OR REPLACE FUNCTION update_oauth_accounts_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_oauth_accounts_updated_at ON oauth_accounts;
CREATE TRIGGER trg_oauth_accounts_updated_at
    BEFORE UPDATE ON oauth_accounts
    FOR EACH ROW
    EXECUTE FUNCTION update_oauth_accounts_updated_at();

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE oauth_accounts IS
'OAuth provider accounts linked to users for social login';

COMMENT ON COLUMN oauth_accounts.provider IS
'OAuth provider: google, github, facebook, apple, discord';

COMMENT ON COLUMN oauth_accounts.provider_user_id IS
'Unique user ID from the OAuth provider';

COMMENT ON COLUMN oauth_accounts.email IS
'Email from OAuth provider (may differ from user email)';

COMMENT ON COLUMN oauth_accounts.access_token IS
'OAuth access token (encrypted at rest recommended)';

COMMENT ON COLUMN oauth_accounts.refresh_token IS
'OAuth refresh token for token renewal (encrypted at rest recommended)';

COMMENT ON COLUMN oauth_accounts.token_expires_at IS
'When the access token expires';
