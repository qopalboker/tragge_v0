-- 0027_contest_reminder_tracking.down.sql
-- Rollback contest reminder tracking

-- Drop indexes
DROP INDEX IF EXISTS idx_contests_starting_reminder_pending;
DROP INDEX IF EXISTS idx_notifications_user_id;
DROP INDEX IF EXISTS idx_notifications_user_unread;
DROP INDEX IF EXISTS idx_notifications_type;
DROP INDEX IF EXISTS idx_notifications_created_at;

-- Drop notifications table
DROP TABLE IF EXISTS notifications;

-- Remove column from contests table
ALTER TABLE contests DROP COLUMN IF EXISTS starting_reminder_sent_at;
