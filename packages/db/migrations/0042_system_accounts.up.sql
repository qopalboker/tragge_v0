-- 0042_system_accounts.up.sql
-- Add system account support for bot players (e.g., Tragge Trader).
-- Free contests require minimum 2 participants; the system bot auto-joins
-- every free contest so they always run even with a single real user.

-- ============================================================================
-- 1. Add is_system_account flag to users table
-- ============================================================================

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_system_account BOOLEAN NOT NULL DEFAULT FALSE;

-- Partial index: fast lookup of system accounts (expect very few rows)
CREATE INDEX IF NOT EXISTS idx_users_system_account
    ON users(id) WHERE is_system_account = TRUE;

-- ============================================================================
-- 2. Insert the Tragge Trader system account
-- ============================================================================
-- UUID is deterministic so other services can reference it as a constant.
-- Password hash is an invalid bcrypt string so login is impossible.

INSERT INTO users (
    id,
    email,
    password_hash,
    username,
    display_name,
    status,
    is_system_account,
    created_at
) VALUES (
    '00000000-0000-0000-0000-000000000001',
    'system-trader@tragge.internal',
    'SYSTEM_ACCOUNT_NO_LOGIN',
    'tragge_trader',
    'Tragge Trader',
    'active',
    TRUE,
    NOW()
) ON CONFLICT (id) DO NOTHING;

-- ============================================================================
-- 3. Add is_system flag to contest_participants
-- ============================================================================
-- Allows easy filtering of bot participation records in queries/leaderboards.

ALTER TABLE contest_participants
    ADD COLUMN IF NOT EXISTS is_system BOOLEAN NOT NULL DEFAULT FALSE;

-- Partial index for quickly finding system participant entries
CREATE INDEX IF NOT EXISTS idx_contest_participants_system
    ON contest_participants(contest_id) WHERE is_system = TRUE;
