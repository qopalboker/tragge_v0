-- 0067_contest_schedule_dedup.up.sql
-- Add unique partial index for deduplication: prevents duplicate contests
-- for the same schedule and start time. This is the PostgreSQL safety net
-- backing the Redis-based fast deduplication in the tournament scheduler.

CREATE UNIQUE INDEX IF NOT EXISTS idx_contests_schedule_start_dedup
    ON contests(schedule_id, starts_at)
    WHERE schedule_id IS NOT NULL;

COMMENT ON INDEX idx_contests_schedule_start_dedup IS
    'Deduplication safety net: prevents duplicate auto-generated contests for the same schedule and start time';
