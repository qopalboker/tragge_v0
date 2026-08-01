-- 0018_tournament_calendar.up.sql
-- Add tournament calendar features: recurrence rules, calendar indexes, and prize pool calculation

-- ============================================================================
-- RECURRENCE RULE FOR TEMPLATES
-- ============================================================================

-- Add recurrence_rule column to tournament_templates for flexible scheduling
-- Examples: "HOURLY", "DAILY@09:00", "WEEKLY@MON,WED,FRI@18:00"
ALTER TABLE tournament_templates ADD COLUMN IF NOT EXISTS recurrence_rule TEXT;

-- Add next_occurrence_at to track when the next contest should be created
ALTER TABLE tournament_templates ADD COLUMN IF NOT EXISTS next_occurrence_at TIMESTAMPTZ;

-- Add last_generated_at to track when the last contest was created from this template
ALTER TABLE tournament_templates ADD COLUMN IF NOT EXISTS last_generated_at TIMESTAMPTZ;

-- ============================================================================
-- CALENDAR-SPECIFIC INDEXES
-- ============================================================================

-- Composite index for calendar queries - efficient filtering by date range, status, asset_class, and entry_fee
-- This index supports the main calendar query patterns
CREATE INDEX IF NOT EXISTS idx_contests_calendar
ON contests(starts_at, status, asset_class, entry_fee_cents);

-- Index for date range queries with status filter
CREATE INDEX IF NOT EXISTS idx_contests_calendar_range
ON contests(starts_at, ends_at, status)
WHERE status IN ('scheduled', 'registration_open', 'running');

-- Index for grouping by day (date extraction)
CREATE INDEX IF NOT EXISTS idx_contests_starts_at_date
ON contests(DATE(starts_at AT TIME ZONE 'UTC'));

-- Index for duration type grouping
CREATE INDEX IF NOT EXISTS idx_contests_duration_type_starts_at
ON contests(duration_type, starts_at)
WHERE status IN ('scheduled', 'registration_open', 'running');

-- Index for asset class grouping
CREATE INDEX IF NOT EXISTS idx_contests_asset_class_starts_at
ON contests(asset_class, starts_at)
WHERE status IN ('scheduled', 'registration_open', 'running');

-- ============================================================================
-- CONTEST TYPE ENUM (if not exists)
-- ============================================================================

-- Add contest_type for more granular type identification
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'contest_type') THEN
        CREATE TYPE contest_type AS ENUM (
            'rush',
            'standard',
            'tournament',
            'championship',
            'practice'
        );
    END IF;
END$$;

-- Add type column to contests if not exists
ALTER TABLE contests ADD COLUMN IF NOT EXISTS type contest_type DEFAULT 'standard';

-- Add type column to tournament_templates if not exists
ALTER TABLE tournament_templates ADD COLUMN IF NOT EXISTS type contest_type DEFAULT 'standard';

-- ============================================================================
-- UPDATE EXISTING DATA
-- ============================================================================

-- Set type based on duration_type and is_free for existing contests
UPDATE contests
SET type = CASE
    WHEN is_free = TRUE THEN 'practice'::contest_type
    WHEN duration_type = 'rush_30min' THEN 'rush'::contest_type
    WHEN duration_type = 'weekly' THEN 'championship'::contest_type
    ELSE 'standard'::contest_type
END
WHERE type IS NULL OR type = 'standard';

-- Set type for existing templates
UPDATE tournament_templates
SET type = CASE
    WHEN is_free = TRUE THEN 'practice'::contest_type
    WHEN duration_minutes <= 30 THEN 'rush'::contest_type
    WHEN duration_minutes >= 10080 THEN 'championship'::contest_type
    ELSE 'standard'::contest_type
END
WHERE type IS NULL OR type = 'standard';

-- ============================================================================
-- INDEX FOR RECURRENCE RULE TEMPLATES
-- ============================================================================

-- Index for templates with active recurrence rules
CREATE INDEX IF NOT EXISTS idx_tournament_templates_recurrence
ON tournament_templates(next_occurrence_at)
WHERE recurrence_rule IS NOT NULL ;

-- Index for auto-create templates that need generation
CREATE INDEX IF NOT EXISTS idx_tournament_templates_auto_generate
ON tournament_templates(next_occurrence_at, auto_create)
WHERE auto_create = TRUE;

-- ============================================================================
-- MATERIALIZED VIEW FOR CALENDAR (OPTIONAL OPTIMIZATION)
-- ============================================================================

-- Create a materialized view for calendar data that can be refreshed periodically
-- This provides fast access to calendar data without expensive joins

CREATE MATERIALIZED VIEW IF NOT EXISTS calendar_contests_mv AS
SELECT
    c.id,
    c.name,
    c.type,
    c.asset_class,
    c.entry_fee_cents,
    c.duration_minutes,
    c.starts_at,
    c.ends_at,
    c.status,
    c.is_free,
    c.max_participants,
    c.commission_rate,
    DATE(c.starts_at AT TIME ZONE 'UTC') as contest_date,
    (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id) as participant_count,
    CASE
        WHEN c.is_free THEN 0
        WHEN c.commission_rate > 0 THEN
            ROUND(c.entry_fee_cents * (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id) * (1 - c.commission_rate / 100))::INT
        ELSE
            c.entry_fee_cents * (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id)
    END as prize_pool_cents
FROM contests c
WHERE c.status IN ('scheduled', 'registration_open', 'running')
  AND c.starts_at >= NOW() - INTERVAL '1 day'
  AND c.starts_at <= NOW() + INTERVAL '30 days';

-- Index on materialized view for common queries
CREATE INDEX IF NOT EXISTS idx_calendar_contests_mv_date ON calendar_contests_mv(contest_date);
CREATE INDEX IF NOT EXISTS idx_calendar_contests_mv_asset ON calendar_contests_mv(asset_class, contest_date);
CREATE INDEX IF NOT EXISTS idx_calendar_contests_mv_type ON calendar_contests_mv(type, contest_date);

-- Create function to refresh the materialized view
CREATE OR REPLACE FUNCTION refresh_calendar_contests_mv()
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY calendar_contests_mv;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON COLUMN tournament_templates.recurrence_rule IS
'Recurrence pattern: HOURLY, DAILY@HH:MM, WEEKLY@DAY1,DAY2@HH:MM, MONTHLY@DD@HH:MM';

COMMENT ON COLUMN tournament_templates.next_occurrence_at IS
'When the next contest should be created from this template';

COMMENT ON COLUMN tournament_templates.last_generated_at IS
'When the last contest was created from this template';

COMMENT ON MATERIALIZED VIEW calendar_contests_mv IS
'Pre-computed calendar data for fast access. Refresh with refresh_calendar_contests_mv()';
