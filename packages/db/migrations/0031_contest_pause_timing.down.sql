-- Rollback: Remove pause timing tracking from contests table

DROP INDEX IF EXISTS idx_contests_paused_at;

ALTER TABLE contests
    DROP COLUMN IF EXISTS paused_at;

ALTER TABLE contests
    DROP COLUMN IF EXISTS total_paused_duration;
