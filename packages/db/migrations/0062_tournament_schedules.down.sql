-- 0062_tournament_schedules.down.sql
-- Revert tournament_schedules table and weekend_behavior enum

-- ============================================================================
-- DROP TRIGGER
-- ============================================================================

DROP TRIGGER IF EXISTS set_tournament_schedules_updated_at ON tournament_schedules;
DROP TRIGGER IF EXISTS trg_validate_active_days ON tournament_schedules;
DROP FUNCTION IF EXISTS validate_active_days();

-- ============================================================================
-- DROP INDEXES
-- ============================================================================

DROP INDEX IF EXISTS idx_tournament_schedules_is_active;
DROP INDEX IF EXISTS idx_tournament_schedules_template_id;

-- ============================================================================
-- DROP TABLE
-- ============================================================================

DROP TABLE IF EXISTS tournament_schedules;

-- ============================================================================
-- DROP ENUM TYPE
-- ============================================================================

DROP TYPE IF EXISTS weekend_behavior;
