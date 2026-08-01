-- Migration: Add pause timing tracking to contests table
-- This enables accurate contest duration when contests are paused and resumed

-- Add paused_at column to track when a contest was paused
ALTER TABLE contests
    ADD COLUMN paused_at TIMESTAMPTZ NULL;

-- Add total_paused_duration to accumulate total pause time
ALTER TABLE contests
    ADD COLUMN total_paused_duration INTERVAL NOT NULL DEFAULT '0 seconds';

-- Add index for finding paused contests efficiently
CREATE INDEX idx_contests_paused_at ON contests (paused_at)
    WHERE paused_at IS NOT NULL;

-- Add comment explaining the columns
COMMENT ON COLUMN contests.paused_at IS 'Timestamp when the contest was paused. NULL when not paused.';
COMMENT ON COLUMN contests.total_paused_duration IS 'Total accumulated duration the contest has been paused.';
