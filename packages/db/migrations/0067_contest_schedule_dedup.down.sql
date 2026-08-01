-- 0067_contest_schedule_dedup.down.sql
-- Revert deduplication unique index

DROP INDEX IF EXISTS idx_contests_schedule_start_dedup;
