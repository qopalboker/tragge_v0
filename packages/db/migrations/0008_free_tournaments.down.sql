-- 0008_free_tournaments.down.sql
-- Remove free practice tournaments and tournament templates support

-- ============================================================================
-- REMOVE DEFAULT TEMPLATE
-- ============================================================================

DELETE FROM tournament_templates WHERE name = 'Hourly Practice' AND is_free = TRUE;

-- ============================================================================
-- DROP TOURNAMENT TEMPLATES TABLE
-- ============================================================================

DROP TABLE IF EXISTS tournament_templates;

-- ============================================================================
-- REMOVE INDEXES FOR FREE TOURNAMENTS
-- ============================================================================

DROP INDEX IF EXISTS idx_contests_is_free;
DROP INDEX IF EXISTS idx_contests_free_open;
DROP INDEX IF EXISTS idx_contests_auto_repeat;

-- ============================================================================
-- REMOVE CONTESTS TABLE ADDITIONS
-- ============================================================================

-- Remove constraints first
ALTER TABLE contests DROP CONSTRAINT IF EXISTS chk_auto_repeat_requires_interval;
ALTER TABLE contests DROP CONSTRAINT IF EXISTS chk_max_participants_positive;

-- Remove columns
ALTER TABLE contests DROP COLUMN IF EXISTS repeat_interval;
ALTER TABLE contests DROP COLUMN IF EXISTS auto_repeat;
ALTER TABLE contests DROP COLUMN IF EXISTS max_participants;
ALTER TABLE contests DROP COLUMN IF EXISTS is_free;
