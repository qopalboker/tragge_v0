-- 0033_calendar_entries.up.sql
-- Add calendar entries for scheduled tournament creation

-- ============================================================================
-- RECURRENCE PATTERN ENUM
-- ============================================================================

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'recurrence_pattern') THEN
        CREATE TYPE recurrence_pattern AS ENUM (
            'daily',
            'weekly',
            'biweekly',
            'monthly',
            'custom_cron'
        );
    END IF;
END$$;

-- ============================================================================
-- CALENDAR STATUS ENUM
-- ============================================================================

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'calendar_status') THEN
        CREATE TYPE calendar_status AS ENUM (
            'active',
            'paused',
            'ended'
        );
    END IF;
END$$;

-- ============================================================================
-- CALENDAR ENTRIES TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS calendar_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES tournament_templates(id) ON DELETE RESTRICT,
    recurrence_pattern recurrence_pattern NOT NULL,
    cron_expression VARCHAR(100), -- For custom_cron pattern
    start_date TIMESTAMPTZ NOT NULL,
    end_date TIMESTAMPTZ, -- Nullable for indefinite schedules
    timezone VARCHAR(50) NOT NULL DEFAULT 'UTC',
    registration_lead_time_minutes INT NOT NULL DEFAULT 60,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    status calendar_status NOT NULL DEFAULT 'active',
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- CALENDAR CONTEST HISTORY TABLE
-- ============================================================================

-- Track contests created from calendar entries
CREATE TABLE IF NOT EXISTS calendar_contest_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    calendar_entry_id UUID NOT NULL REFERENCES calendar_entries(id) ON DELETE CASCADE,
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    scheduled_for TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- CONSTRAINTS
-- ============================================================================

-- Ensure cron_expression is set when pattern is custom_cron
ALTER TABLE calendar_entries ADD CONSTRAINT chk_cron_expression_required
    CHECK (recurrence_pattern != 'custom_cron' OR cron_expression IS NOT NULL);

-- Ensure end_date is after start_date if set
ALTER TABLE calendar_entries ADD CONSTRAINT chk_end_date_after_start
    CHECK (end_date IS NULL OR end_date > start_date);

-- Ensure registration_lead_time_minutes is positive
ALTER TABLE calendar_entries ADD CONSTRAINT chk_registration_lead_time_positive
    CHECK (registration_lead_time_minutes >= 0);

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Index for querying active calendar entries
CREATE INDEX IF NOT EXISTS idx_calendar_entries_enabled_status
    ON calendar_entries(enabled, status)
    WHERE enabled = TRUE;

-- Index for template lookups
CREATE INDEX IF NOT EXISTS idx_calendar_entries_template_id
    ON calendar_entries(template_id);

-- Index for next_run_at scheduling queries
CREATE INDEX IF NOT EXISTS idx_calendar_entries_next_run
    ON calendar_entries(next_run_at)
    WHERE enabled = TRUE AND status = 'active';

-- Index for history lookups by calendar entry
CREATE INDEX IF NOT EXISTS idx_calendar_contest_history_entry
    ON calendar_contest_history(calendar_entry_id, created_at DESC);

-- Index for history lookups by contest
CREATE INDEX IF NOT EXISTS idx_calendar_contest_history_contest
    ON calendar_contest_history(contest_id);

-- ============================================================================
-- TRIGGER FOR UPDATED_AT
-- ============================================================================

CREATE OR REPLACE FUNCTION update_calendar_entries_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_calendar_entries_updated_at ON calendar_entries;
CREATE TRIGGER trg_calendar_entries_updated_at
    BEFORE UPDATE ON calendar_entries
    FOR EACH ROW
    EXECUTE FUNCTION update_calendar_entries_updated_at();

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE calendar_entries IS
'Scheduled rules for automatic tournament creation from templates';

COMMENT ON COLUMN calendar_entries.recurrence_pattern IS
'Pattern for recurrence: daily, weekly, biweekly, monthly, or custom_cron';

COMMENT ON COLUMN calendar_entries.cron_expression IS
'Custom cron expression when recurrence_pattern is custom_cron (e.g., "0 9 * * 1-5" for weekdays at 9am)';

COMMENT ON COLUMN calendar_entries.start_date IS
'When this calendar entry becomes active';

COMMENT ON COLUMN calendar_entries.end_date IS
'When this calendar entry expires (NULL for indefinite)';

COMMENT ON COLUMN calendar_entries.timezone IS
'IANA timezone for scheduling (e.g., America/New_York, Europe/London)';

COMMENT ON COLUMN calendar_entries.registration_lead_time_minutes IS
'How many minutes before contest start to open registration';

COMMENT ON COLUMN calendar_entries.next_run_at IS
'Pre-computed timestamp for the next scheduled contest creation';

COMMENT ON TABLE calendar_contest_history IS
'History of contests created from calendar entries';
