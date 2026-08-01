-- 0016_contest_state_machine.up.sql
-- Contest lifecycle state machine matching Tralent's phases

-- ============================================================================
-- EXTEND CONTEST STATUS ENUM
-- ============================================================================

-- Add new states for complete lifecycle:
-- - registration_closed: Registration deadline passed, waiting for start
-- - settling: Trading ended, calculating results and distributing prizes
ALTER TYPE contest_status ADD VALUE IF NOT EXISTS 'registration_closed' AFTER 'registration_open';
ALTER TYPE contest_status ADD VALUE IF NOT EXISTS 'settling' AFTER 'running';

-- ============================================================================
-- CONTEST LIFECYCLE TIMESTAMPS
-- ============================================================================

-- When the contest was published (draft -> scheduled)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

-- When the contest actually started (-> running)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;

-- When the contest ended (running -> settling)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS ended_at TIMESTAMPTZ;

-- When settlement completed (settling -> completed)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS settled_at TIMESTAMPTZ;

-- When the contest was cancelled (-> cancelled)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

-- Reason for cancellation (for audit and user communication)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS cancellation_reason TEXT;

-- Current participant count (denormalized for performance)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS current_participants INT NOT NULL DEFAULT 0;

-- ============================================================================
-- CONTEST STATUS HISTORY (AUDIT TRAIL)
-- ============================================================================

CREATE TABLE IF NOT EXISTS contest_status_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    from_status contest_status,  -- null for initial creation
    to_status contest_status NOT NULL,
    changed_by UUID REFERENCES users(id) ON DELETE SET NULL,  -- null for automatic transitions
    reason TEXT,  -- optional reason for the transition
    metadata JSONB,  -- additional context (e.g., participant count at time of transition)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_contest_status_history_contest_id
    ON contest_status_history(contest_id);
CREATE INDEX IF NOT EXISTS idx_contest_status_history_created_at
    ON contest_status_history(created_at);
CREATE INDEX IF NOT EXISTS idx_contest_status_history_to_status
    ON contest_status_history(to_status);
CREATE INDEX IF NOT EXISTS idx_contest_status_history_changed_by
    ON contest_status_history(changed_by) WHERE changed_by IS NOT NULL;

-- ============================================================================
-- CONSTRAINTS
-- ============================================================================

-- Current participants cannot be negative
ALTER TABLE contests ADD CONSTRAINT chk_current_participants_non_negative
    CHECK (current_participants >= 0);

-- Current participants cannot exceed max_participants (if set)
ALTER TABLE contests ADD CONSTRAINT chk_current_participants_lte_max
    CHECK (max_participants IS NULL OR current_participants <= max_participants);

-- ============================================================================
-- TRIGGER: UPDATE CURRENT_PARTICIPANTS ON PARTICIPANT CHANGES
-- ============================================================================

CREATE OR REPLACE FUNCTION trigger_update_contest_participant_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE contests
        SET current_participants = current_participants + 1
        WHERE id = NEW.contest_id;
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE contests
        SET current_participants = GREATEST(0, current_participants - 1)
        WHERE id = OLD.contest_id;
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Drop trigger if exists and recreate
DROP TRIGGER IF EXISTS trg_update_participant_count ON contest_participants;

CREATE TRIGGER trg_update_participant_count
    AFTER INSERT OR DELETE ON contest_participants
    FOR EACH ROW
    EXECUTE FUNCTION trigger_update_contest_participant_count();

-- ============================================================================
-- SYNC EXISTING PARTICIPANT COUNTS
-- ============================================================================

-- Update current_participants for all existing contests
UPDATE contests c
SET current_participants = COALESCE(
    (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id),
    0
);

-- ============================================================================
-- INDEXES FOR STATE MACHINE QUERIES
-- ============================================================================

-- Index for finding contests ready for auto-start
CREATE INDEX IF NOT EXISTS idx_contests_auto_start_pending
    ON contests(starts_at)
    WHERE status IN ('scheduled', 'registration_open')
    AND auto_start = TRUE;

-- Index for finding contests ready for settlement
CREATE INDEX IF NOT EXISTS idx_contests_ready_for_settlement
    ON contests(ends_at)
    WHERE status = 'running';

-- Index for cancelled contests with reason
CREATE INDEX IF NOT EXISTS idx_contests_cancelled_at
    ON contests(cancelled_at)
    WHERE status = 'cancelled';

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON COLUMN contests.published_at IS 'Timestamp when contest transitioned from draft to scheduled';
COMMENT ON COLUMN contests.started_at IS 'Timestamp when contest actually started (transitioned to running)';
COMMENT ON COLUMN contests.ended_at IS 'Timestamp when trading period ended (transitioned to settling)';
COMMENT ON COLUMN contests.settled_at IS 'Timestamp when settlement completed (transitioned to completed)';
COMMENT ON COLUMN contests.cancelled_at IS 'Timestamp when contest was cancelled';
COMMENT ON COLUMN contests.cancellation_reason IS 'Reason for cancellation, displayed to users';
COMMENT ON COLUMN contests.current_participants IS 'Denormalized count of participants for capacity checks';

COMMENT ON TABLE contest_status_history IS 'Audit trail of all contest status transitions';
COMMENT ON COLUMN contest_status_history.changed_by IS 'User who triggered the transition, NULL for automatic transitions';
COMMENT ON COLUMN contest_status_history.reason IS 'Optional reason or context for the transition';
COMMENT ON COLUMN contest_status_history.metadata IS 'Additional context like participant count, configuration snapshot, etc.';
