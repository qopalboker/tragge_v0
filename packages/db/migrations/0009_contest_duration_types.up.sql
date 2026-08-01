-- 0009_contest_duration_types.up.sql
-- Add contest duration types similar to Tralent (30-min rush, hourly, 4-hour, daily, weekly)

-- ============================================================================
-- CONTEST DURATION TYPE ENUM
-- ============================================================================

CREATE TYPE contest_duration_type AS ENUM (
    'rush_30min',
    'hourly',
    'four_hour',
    'daily',
    'weekly'
);

-- ============================================================================
-- ADD DURATION TYPE TO CONTESTS TABLE
-- ============================================================================

ALTER TABLE contests
    ADD COLUMN duration_type contest_duration_type;

-- ============================================================================
-- UPDATE EXISTING CONTESTS BASED ON DURATION
-- ============================================================================

UPDATE contests
SET duration_type = CASE
    -- 30 minutes or less -> rush_30min
    WHEN EXTRACT(EPOCH FROM (ends_at - starts_at)) / 60 <= 30 THEN 'rush_30min'::contest_duration_type
    -- 31-60 minutes -> hourly
    WHEN EXTRACT(EPOCH FROM (ends_at - starts_at)) / 60 <= 60 THEN 'hourly'::contest_duration_type
    -- 61-240 minutes (4 hours) -> four_hour
    WHEN EXTRACT(EPOCH FROM (ends_at - starts_at)) / 60 <= 240 THEN 'four_hour'::contest_duration_type
    -- 241-1440 minutes (24 hours) -> daily
    WHEN EXTRACT(EPOCH FROM (ends_at - starts_at)) / 60 <= 1440 THEN 'daily'::contest_duration_type
    -- More than 24 hours -> weekly
    ELSE 'weekly'::contest_duration_type
END
WHERE duration_type IS NULL;

-- Set default for new contests and make NOT NULL
ALTER TABLE contests
    ALTER COLUMN duration_type SET DEFAULT 'hourly',
    ALTER COLUMN duration_type SET NOT NULL;

-- ============================================================================
-- CONTEST DURATION CONFIGS TABLE
-- ============================================================================

CREATE TABLE contest_duration_configs (
    duration_type contest_duration_type PRIMARY KEY,
    duration_minutes INT NOT NULL,
    default_qty_total BIGINT NOT NULL,
    min_entry_fee_cents INT NOT NULL,
    max_entry_fee_cents INT NOT NULL,
    display_name_en VARCHAR(50) NOT NULL,
    display_name_fa VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_duration_minutes_positive CHECK (duration_minutes > 0),
    CONSTRAINT chk_min_entry_fee_positive CHECK (min_entry_fee_cents >= 0),
    CONSTRAINT chk_max_entry_fee_positive CHECK (max_entry_fee_cents >= 0),
    CONSTRAINT chk_entry_fee_range CHECK (max_entry_fee_cents >= min_entry_fee_cents),
    CONSTRAINT chk_default_qty_total_positive CHECK (default_qty_total > 0)
);

-- Add trigger for updated_at
CREATE TRIGGER set_contest_duration_configs_updated_at
    BEFORE UPDATE ON contest_duration_configs
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================================
-- INSERT DURATION CONFIGURATIONS
-- ============================================================================

INSERT INTO contest_duration_configs (
    duration_type,
    duration_minutes,
    default_qty_total,
    min_entry_fee_cents,
    max_entry_fee_cents,
    display_name_en,
    display_name_fa
) VALUES
    ('rush_30min', 30, 50000, 100, 1000, '30-Min Rush', 'رقابت ۳۰ دقیقه‌ای'),
    ('hourly', 60, 100000, 200, 2000, 'Hourly', 'ساعتی'),
    ('four_hour', 240, 200000, 500, 5000, '4-Hour', 'چهار ساعته'),
    ('daily', 1440, 500000, 1000, 10000, 'Daily', 'روزانه'),
    ('weekly', 10080, 1000000, 2000, 50000, 'Weekly', 'هفتگی');

-- ============================================================================
-- INDEX ON CONTESTS.DURATION_TYPE
-- ============================================================================

CREATE INDEX idx_contests_duration_type ON contests(duration_type);

-- Composite index for filtering by status and duration type
CREATE INDEX idx_contests_status_duration_type ON contests(status, duration_type);
