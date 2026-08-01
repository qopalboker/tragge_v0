-- 0098_fix_migration_audit_issues.down.sql
-- Reverts the five audit fixes.

-- ============================================================================
-- 5. Remove FK on chart_drawings.contest_id
-- ============================================================================

ALTER TABLE chart_drawings DROP CONSTRAINT IF EXISTS fk_chart_drawings_contest;

-- ============================================================================
-- 4. Re-add the inline UNIQUE constraint on users.phone
--    (restores the state before fix #4)
-- ============================================================================

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints
                   WHERE table_name = 'users'
                     AND constraint_name = 'users_phone_key'
                     AND constraint_type = 'UNIQUE') THEN
        ALTER TABLE users ADD CONSTRAINT users_phone_key UNIQUE (phone);
    END IF;
END $$;

-- ============================================================================
-- 3. Revert user_stats columns back to DECIMAL(20,2)
--    (restores the buggy state from 0032 down-migration)
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'user_stats' AND column_name = 'total_score') THEN
        ALTER TABLE user_stats
        ALTER COLUMN total_score TYPE DECIMAL(20, 2);
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'user_stats' AND column_name = 'tralent_score') THEN
        ALTER TABLE user_stats
        ALTER COLUMN tralent_score TYPE DECIMAL(20, 2);
    END IF;
END $$;

-- ============================================================================
-- 2. Recreate the duplicate trigger on tournament_templates
-- ============================================================================

CREATE OR REPLACE FUNCTION update_tournament_templates_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_tournament_templates_updated_at
    BEFORE UPDATE ON tournament_templates
    FOR EACH ROW
    EXECUTE FUNCTION update_tournament_templates_updated_at();

-- ============================================================================
-- 1. Indexes from 0086/0089/0095 remain — they were already created by
--    their original migrations. The non-concurrent recreations in the up
--    migration are idempotent (IF NOT EXISTS), so nothing to revert here.
-- ============================================================================
