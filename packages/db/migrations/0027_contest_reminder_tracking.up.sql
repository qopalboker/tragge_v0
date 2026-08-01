-- 0027_contest_reminder_tracking.up.sql
-- Track contest reminder notifications sent to participants

-- ============================================================================
-- CONTESTS TABLE ADDITIONS
-- ============================================================================

-- Track when the starting reminder was sent (15 minutes before start)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS starting_reminder_sent_at TIMESTAMPTZ;

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Index for finding contests needing starting reminders
-- Contests that:
-- 1. Haven't had reminder sent yet
-- 2. Are in appropriate status (scheduled, registration_open, registration_closed)
-- 3. Start within a time window (e.g., next 20 minutes)
CREATE INDEX IF NOT EXISTS idx_contests_starting_reminder_pending
    ON contests(starts_at)
    WHERE starting_reminder_sent_at IS NULL
    AND status IN ('scheduled', 'registration_open', 'registration_closed');

-- ============================================================================
-- IN-APP NOTIFICATIONS TABLE (if not exists)
-- ============================================================================

-- Create in-app notifications table for storing user notifications
CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    message TEXT,
    metadata JSONB,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for notifications
CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_user_unread ON notifications(user_id, created_at DESC)
    WHERE read_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_type ON notifications(type);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at);

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON COLUMN contests.starting_reminder_sent_at IS 'Timestamp when the 15-minute starting reminder was sent to participants';
COMMENT ON TABLE notifications IS 'In-app notifications for users';
COMMENT ON COLUMN notifications.type IS 'Notification type (e.g., contest_starting, contest_completed, withdrawal_approved)';
COMMENT ON COLUMN notifications.metadata IS 'Additional data like contest_id, amounts, etc.';
COMMENT ON COLUMN notifications.read_at IS 'When the notification was read, NULL if unread';
