-- 0037_multi_interval_reminders.up.sql
-- Add multi-interval reminder tracking for contests.
-- Supports configurable reminder tiers (e.g., 24h, 1h, 15m before start).

-- ============================================================================
-- CONTEST REMINDERS TRACKING TABLE
-- ============================================================================

-- Track which reminder intervals have been sent for each contest.
-- This replaces the single-column approach (starting_reminder_sent_at) with
-- a flexible per-interval tracking table.
CREATE TABLE IF NOT EXISTS contest_reminders_sent (
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    reminder_type VARCHAR(20) NOT NULL, -- e.g. '24h', '1h', '15m'
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    recipient_count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (contest_id, reminder_type)
);

-- Index for finding which reminders have been sent for a contest
CREATE INDEX IF NOT EXISTS idx_contest_reminders_sent_contest
    ON contest_reminders_sent(contest_id);

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE contest_reminders_sent IS 'Tracks which reminder intervals have been sent for each contest';
COMMENT ON COLUMN contest_reminders_sent.reminder_type IS 'Reminder tier identifier (e.g. 24h, 1h, 15m)';
COMMENT ON COLUMN contest_reminders_sent.recipient_count IS 'Number of participants who received the reminder';
