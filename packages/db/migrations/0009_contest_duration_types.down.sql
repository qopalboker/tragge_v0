-- 0009_contest_duration_types.down.sql
-- Rollback contest duration types

-- ============================================================================
-- DROP INDEXES
-- ============================================================================

DROP INDEX IF EXISTS idx_contests_status_duration_type;
DROP INDEX IF EXISTS idx_contests_duration_type;

-- ============================================================================
-- DROP CONTEST DURATION CONFIGS TABLE
-- ============================================================================

DROP TRIGGER IF EXISTS set_contest_duration_configs_updated_at ON contest_duration_configs;
DROP TABLE IF EXISTS contest_duration_configs;

-- ============================================================================
-- REMOVE DURATION TYPE FROM CONTESTS TABLE
-- ============================================================================

ALTER TABLE contests
    DROP COLUMN IF EXISTS duration_type;

-- ============================================================================
-- DROP ENUM TYPE
-- ============================================================================

DROP TYPE IF EXISTS contest_duration_type;
