-- 0098_fix_migration_audit_issues.up.sql
-- Fixes five issues found during migration audit.

-- ============================================================================
-- 1. Re-create CONCURRENTLY indexes from 0086, 0089, 0095
--    These must run outside a transaction block. golang-migrate wraps each
--    migration in a transaction by default, so CONCURRENTLY would fail.
--    We recreate them as plain CREATE INDEX IF NOT EXISTS (non-concurrent)
--    which is safe inside a transaction and idempotent.
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_users_phone
    ON users(phone) WHERE phone IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_support_tickets_updated_at
    ON support_tickets(updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_verification_codes_cleanup
    ON verification_codes(expires_at)
    WHERE verified_at IS NULL;

-- ============================================================================
-- 2. Drop duplicate trigger on tournament_templates
--    0056 created trg_tournament_templates_updated_at (dedicated function)
--    0061 created set_tournament_templates_updated_at  (generic function)
--    Both fire BEFORE UPDATE on the same table — drop the older one and
--    its now-orphaned function.
-- ============================================================================

DROP TRIGGER IF EXISTS trg_tournament_templates_updated_at ON tournament_templates;
DROP FUNCTION IF EXISTS update_tournament_templates_updated_at();

-- ============================================================================
-- 3. Fix 0032 down-migration precision mismatch
--    The down migration used DECIMAL(20,2) for user_stats.total_score and
--    user_stats.tralent_score, which would truncate data. We widen them
--    to NUMERIC(20,4) to match the rest of the down migration.
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'user_stats' AND column_name = 'total_score') THEN
        ALTER TABLE user_stats
        ALTER COLUMN total_score TYPE NUMERIC(20, 4);
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'user_stats' AND column_name = 'tralent_score') THEN
        ALTER TABLE user_stats
        ALTER COLUMN tralent_score TYPE NUMERIC(20, 4);
    END IF;
END $$;

-- ============================================================================
-- 4. Drop duplicate unique index on users.phone
--    0092 added "phone VARCHAR(20) UNIQUE" (inline table constraint)
--    0086 created idx_users_phone (partial unique index on non-null)
--    The inline UNIQUE generated a separate index — drop it since the
--    partial index from 0086 is the correct one (allows multiple NULLs).
-- ============================================================================

-- The inline UNIQUE constraint creates an index named by convention.
-- Drop the constraint (which also removes its backing index).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.table_constraints
               WHERE table_name = 'users'
                 AND constraint_name = 'users_phone_key'
                 AND constraint_type = 'UNIQUE') THEN
        ALTER TABLE users DROP CONSTRAINT users_phone_key;
    END IF;
END $$;

-- Also drop any standalone unique index if it was created explicitly
DROP INDEX IF EXISTS idx_users_phone_unique;

-- ============================================================================
-- 5. Add missing FK on chart_drawings.contest_id → contests(id)
-- ============================================================================

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints
                   WHERE constraint_name = 'fk_chart_drawings_contest'
                     AND table_name = 'chart_drawings') THEN
        ALTER TABLE chart_drawings
            ADD CONSTRAINT fk_chart_drawings_contest
            FOREIGN KEY (contest_id) REFERENCES contests(id);
    END IF;
END $$;
