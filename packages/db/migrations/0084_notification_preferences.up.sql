-- User notification preferences
-- Each row: one user × one category × one channel = enabled/disabled
CREATE TABLE IF NOT EXISTS user_notification_preferences (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category VARCHAR(30) NOT NULL,
    channel VARCHAR(10) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, category, channel)
);

-- Index for fast lookup by user
CREATE INDEX IF NOT EXISTS idx_user_notif_prefs_user
    ON user_notification_preferences(user_id);

-- Categories:
-- 'contest_reminders' → contest_starting, contest_ending, registration_closed
-- 'contest_results'   → contest_completed, contest_cancelled, prize_won, contest_paused, contest_resumed
-- 'contest_activity'  → contest_joined, contest_left, contest_started
-- 'transactions'      → deposit_confirmed, deposit_failed, withdrawal_update
-- 'account'           → kyc_update, system

-- Channels:
-- 'in_app'  → database notifications (shown in bell dropdown)
-- 'email'   → email notifications

COMMENT ON TABLE user_notification_preferences IS 'Per-user notification preferences. Missing row = default enabled.';
COMMENT ON COLUMN user_notification_preferences.category IS 'Notification category: contest_reminders, contest_results, contest_activity, transactions, account';
COMMENT ON COLUMN user_notification_preferences.channel IS 'Delivery channel: in_app, email';
