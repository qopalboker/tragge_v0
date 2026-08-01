-- 0034_oauth_accounts.down.sql
-- Remove OAuth accounts support

-- ============================================================================
-- DROP TRIGGER
-- ============================================================================

DROP TRIGGER IF EXISTS trg_oauth_accounts_updated_at ON oauth_accounts;

-- ============================================================================
-- DROP FUNCTION
-- ============================================================================

DROP FUNCTION IF EXISTS update_oauth_accounts_updated_at();

-- ============================================================================
-- DROP TABLE
-- ============================================================================

DROP TABLE IF EXISTS oauth_accounts;

-- ============================================================================
-- DROP ENUM
-- ============================================================================

DROP TYPE IF EXISTS oauth_provider;

-- ============================================================================
-- REVERT USERS TABLE: MAKE password_hash NOT NULL AGAIN
-- ============================================================================

-- First, delete any users that have NULL password_hash (OAuth-only users)
-- This is a destructive operation but necessary for clean rollback
-- In production, you may want to handle this differently
DELETE FROM users WHERE password_hash IS NULL;

-- Restore NOT NULL constraint on password_hash
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
