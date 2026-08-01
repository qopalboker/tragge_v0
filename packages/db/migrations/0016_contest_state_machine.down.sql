-- 0016_contest_state_machine.down.sql
-- Rollback contest lifecycle state machine

-- ============================================================================
-- DROP TRIGGER
-- ============================================================================

DROP TRIGGER IF EXISTS trg_update_participant_count ON contest_participants;
DROP FUNCTION IF EXISTS trigger_update_contest_participant_count();

-- ============================================================================
-- DROP INDEXES
-- ============================================================================

DROP INDEX IF EXISTS idx_contests_auto_start_pending;
DROP INDEX IF EXISTS idx_contests_ready_for_settlement;
DROP INDEX IF EXISTS idx_contests_cancelled_at;

-- ============================================================================
-- DROP STATUS HISTORY TABLE
-- ============================================================================

DROP TABLE IF EXISTS contest_status_history;

-- ============================================================================
-- DROP CONSTRAINTS
-- ============================================================================

ALTER TABLE contests DROP CONSTRAINT IF EXISTS chk_current_participants_non_negative;
ALTER TABLE contests DROP CONSTRAINT IF EXISTS chk_current_participants_lte_max;

-- ============================================================================
-- DROP COLUMNS
-- ============================================================================

ALTER TABLE contests DROP COLUMN IF EXISTS published_at;
ALTER TABLE contests DROP COLUMN IF EXISTS started_at;
ALTER TABLE contests DROP COLUMN IF EXISTS ended_at;
ALTER TABLE contests DROP COLUMN IF EXISTS settled_at;
ALTER TABLE contests DROP COLUMN IF EXISTS cancelled_at;
ALTER TABLE contests DROP COLUMN IF EXISTS cancellation_reason;
ALTER TABLE contests DROP COLUMN IF EXISTS current_participants;

-- ============================================================================
-- NOTE: ENUM VALUES CANNOT BE REMOVED IN POSTGRESQL
-- ============================================================================
-- The new enum values 'registration_closed' and 'settling' will remain in the enum type.
-- This is a PostgreSQL limitation. To fully remove them would require recreating the type.
-- This is acceptable as the values won't cause issues if not used.
