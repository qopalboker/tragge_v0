-- 0062_tournament_schedules.up.sql
-- Create tournament_schedules table for defining when tournaments are automatically created.
-- Supports cron-based recurrence, active day filtering (Iranian week), and weekend behavior.

-- ============================================================================
-- WEEKEND BEHAVIOR ENUM
-- ============================================================================

CREATE TYPE weekend_behavior AS ENUM (
    'crypto_only',  -- Only run crypto tournaments on weekends
    'skip',         -- Skip tournament creation on weekends
    'normal'        -- Run tournaments normally on weekends
);

-- ============================================================================
-- TOURNAMENT SCHEDULES TABLE
-- ============================================================================

CREATE TABLE tournament_schedules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES tournament_templates(id) ON DELETE CASCADE,
    cron_expression VARCHAR(100) NOT NULL,
    start_time_utc TIME,                              -- base start time in UTC, nullable for cron-only
    active_days INT[],                                -- weekday numbers: 0=Saturday to 6=Friday (Iranian week)
    weekend_behavior weekend_behavior NOT NULL DEFAULT 'normal',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- CONSTRAINTS
-- ============================================================================

-- Validate that all active_days values are between 0 and 6
-- Uses a trigger since CHECK constraints cannot contain subqueries
CREATE OR REPLACE FUNCTION validate_active_days()
RETURNS TRIGGER AS $$
DECLARE
    d INT;
BEGIN
    IF NEW.active_days IS NOT NULL THEN
        IF array_length(NEW.active_days, 1) IS NULL THEN
            RAISE EXCEPTION 'active_days must not be an empty array';
        END IF;
        FOREACH d IN ARRAY NEW.active_days LOOP
            IF d < 0 OR d > 6 THEN
                RAISE EXCEPTION 'active_days values must be between 0 and 6, got %', d;
            END IF;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_validate_active_days
    BEFORE INSERT OR UPDATE ON tournament_schedules
    FOR EACH ROW
    EXECUTE FUNCTION validate_active_days();

-- ============================================================================
-- INDEXES
-- ============================================================================

CREATE INDEX idx_tournament_schedules_template_id
    ON tournament_schedules(template_id);

CREATE INDEX idx_tournament_schedules_is_active
    ON tournament_schedules(is_active)
    WHERE is_active = TRUE;

-- ============================================================================
-- UPDATED_AT TRIGGER
-- ============================================================================

CREATE TRIGGER set_tournament_schedules_updated_at
    BEFORE UPDATE ON tournament_schedules
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE tournament_schedules IS 'Defines when tournaments are automatically created from templates';
COMMENT ON COLUMN tournament_schedules.cron_expression IS 'Cron expression for recurrence, e.g. "*/10 * * * *" for every 10 minutes';
COMMENT ON COLUMN tournament_schedules.start_time_utc IS 'Base start time in UTC, nullable for cron-only schedules';
COMMENT ON COLUMN tournament_schedules.active_days IS 'Array of weekday numbers using Iranian week: 0=Saturday, 1=Sunday, ..., 4=Wednesday, 5=Thursday, 6=Friday';
COMMENT ON COLUMN tournament_schedules.weekend_behavior IS 'How to handle tournament creation on weekends: crypto_only, skip, or normal';
