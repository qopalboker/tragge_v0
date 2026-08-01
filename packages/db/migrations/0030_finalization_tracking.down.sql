-- 0030_finalization_tracking.down.sql
-- Remove contest finalization state tracking

-- Drop the trigger first
DROP TRIGGER IF EXISTS set_finalization_state_updated_at ON contest_finalization_state;

-- Drop the indexes
DROP INDEX IF EXISTS idx_finalization_state_incomplete;
DROP INDEX IF EXISTS idx_finalization_state_errors;
DROP INDEX IF EXISTS idx_finalization_state_completed;

-- Drop the table
DROP TABLE IF EXISTS contest_finalization_state;
