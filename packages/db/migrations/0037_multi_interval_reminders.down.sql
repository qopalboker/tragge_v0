-- 0037_multi_interval_reminders.down.sql
-- Rollback multi-interval reminder tracking

DROP INDEX IF EXISTS idx_contest_reminders_sent_contest;
DROP TABLE IF EXISTS contest_reminders_sent;
