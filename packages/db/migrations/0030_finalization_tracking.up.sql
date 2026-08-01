-- 0030_finalization_tracking.up.sql
-- Add contest finalization state tracking for crash recovery

-- ============================================================================
-- CONTEST FINALIZATION STATE TABLE
-- ============================================================================

-- Tracks the progress of contest finalization for crash recovery.
-- If the leaderboard-worker crashes during finalization, it can check this
-- table on re-entry to skip already-completed steps.

CREATE TABLE contest_finalization_state (
    contest_id UUID PRIMARY KEY REFERENCES contests(id) ON DELETE CASCADE,

    -- When finalization started
    finalization_started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Step 1: Payouts calculated (prize distribution computed)
    payouts_calculated BOOLEAN NOT NULL DEFAULT FALSE,
    payouts_calculated_at TIMESTAMPTZ,

    -- Step 2: Ranks written to contest_participants
    ranks_written BOOLEAN NOT NULL DEFAULT FALSE,
    ranks_written_at TIMESTAMPTZ,

    -- Step 3: Wallet credits processed
    wallets_credited BOOLEAN NOT NULL DEFAULT FALSE,
    wallets_credited_at TIMESTAMPTZ,

    -- Step 4: Contest status updated to 'completed'
    status_updated BOOLEAN NOT NULL DEFAULT FALSE,
    status_updated_at TIMESTAMPTZ,

    -- When finalization fully completed
    finalization_completed_at TIMESTAMPTZ,

    -- Error tracking
    error_message TEXT,
    last_error_at TIMESTAMPTZ,
    retry_count INT NOT NULL DEFAULT 0,

    -- Metadata for auditing (payout summary, participant count, etc.)
    metadata JSONB,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Index for finding incomplete finalizations (for retry/monitoring)
CREATE INDEX idx_finalization_state_incomplete
    ON contest_finalization_state(finalization_started_at)
    WHERE finalization_completed_at IS NULL;

-- Index for finding errored finalizations
CREATE INDEX idx_finalization_state_errors
    ON contest_finalization_state(last_error_at)
    WHERE error_message IS NOT NULL;

-- Index for recent completions (for monitoring dashboard)
CREATE INDEX idx_finalization_state_completed
    ON contest_finalization_state(finalization_completed_at DESC)
    WHERE finalization_completed_at IS NOT NULL;

-- ============================================================================
-- TRIGGER: UPDATE updated_at
-- ============================================================================

CREATE TRIGGER set_finalization_state_updated_at
    BEFORE UPDATE ON contest_finalization_state
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE contest_finalization_state IS
'Tracks contest finalization progress for crash recovery. Each step is recorded
atomically so the worker can resume from the last successful step after a crash.';

COMMENT ON COLUMN contest_finalization_state.payouts_calculated IS
'True when prize distribution has been calculated (CalculateContestPayouts completed)';

COMMENT ON COLUMN contest_finalization_state.ranks_written IS
'True when final_rank and final_prize_cents have been written to contest_participants';

COMMENT ON COLUMN contest_finalization_state.wallets_credited IS
'True when wallet credits have been processed for all winners';

COMMENT ON COLUMN contest_finalization_state.status_updated IS
'True when contest status has been updated to completed';

COMMENT ON COLUMN contest_finalization_state.retry_count IS
'Number of times finalization has been retried after errors';

COMMENT ON COLUMN contest_finalization_state.metadata IS
'JSON object containing payout summary, participant count, and other audit data';
