-- 0018_tournament_calendar.down.sql
-- Rollback tournament calendar features

-- Drop materialized view and function
DROP FUNCTION IF EXISTS refresh_calendar_contests_mv();
DROP MATERIALIZED VIEW IF EXISTS calendar_contests_mv;

-- Drop indexes
DROP INDEX IF EXISTS idx_tournament_templates_auto_generate;
DROP INDEX IF EXISTS idx_tournament_templates_recurrence;
DROP INDEX IF EXISTS idx_contests_asset_class_starts_at;
DROP INDEX IF EXISTS idx_contests_duration_type_starts_at;
DROP INDEX IF EXISTS idx_contests_starts_at_date;
DROP INDEX IF EXISTS idx_contests_calendar_range;
DROP INDEX IF EXISTS idx_contests_calendar;

-- Remove columns from tournament_templates
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS last_generated_at;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS next_occurrence_at;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS recurrence_rule;

-- Remove type columns
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS type;
ALTER TABLE contests DROP COLUMN IF EXISTS type;

-- Drop contest_type enum
DROP TYPE IF EXISTS contest_type;
