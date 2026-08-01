-- 0032_decimal_scores.up.sql
-- Drop dependent view first
DROP VIEW IF EXISTS positions_compat;

-- Migrate score columns to NUMERIC(20,8) for higher precision
-- This prevents floating-point accumulation errors over thousands of trades

-- ============================================================================
-- Update contest_participants total_score from NUMERIC(20,4) to NUMERIC(20,8)
-- ============================================================================

ALTER TABLE contest_participants
ALTER COLUMN total_score TYPE NUMERIC(20, 8);

-- ============================================================================
-- Update positions realized_score from NUMERIC(20,4) to NUMERIC(20,8)
-- ============================================================================

ALTER TABLE positions
ALTER COLUMN realized_score TYPE NUMERIC(20, 8);

-- ============================================================================
-- Update settlement_snapshots if it exists
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'settlement_snapshots') THEN
        ALTER TABLE settlement_snapshots
        ALTER COLUMN realized_score TYPE NUMERIC(20, 8);

        ALTER TABLE settlement_snapshots
        ALTER COLUMN unrealized_score TYPE NUMERIC(20, 8);
    END IF;
END $$;

-- ============================================================================
-- Update user_stats total_score and tralent_score if they exist
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'user_stats' AND column_name = 'total_score') THEN
        ALTER TABLE user_stats
        ALTER COLUMN total_score TYPE NUMERIC(20, 8);
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'user_stats' AND column_name = 'tralent_score') THEN
        ALTER TABLE user_stats
        ALTER COLUMN tralent_score TYPE NUMERIC(20, 8);
    END IF;
END $$;

-- ============================================================================
-- Update user_score_history score if it exists
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'user_score_history' AND column_name = 'score') THEN
        ALTER TABLE user_score_history
        ALTER COLUMN score TYPE NUMERIC(20, 8);
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'user_score_history' AND column_name = 'score_contribution') THEN
        ALTER TABLE user_score_history
        ALTER COLUMN score_contribution TYPE NUMERIC(20, 8);
    END IF;
END $$;

-- ============================================================================
-- Update leaderboard_snapshots score column if it exists
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'leaderboard_snapshots' AND column_name = 'score') THEN
        ALTER TABLE leaderboard_snapshots
        ALTER COLUMN score TYPE NUMERIC(20, 8);
    END IF;
END $$;

-- ============================================================================
-- Add comment explaining the precision change
-- ============================================================================

COMMENT ON COLUMN contest_participants.total_score IS 'Total score with 8 decimal places precision to prevent float accumulation errors';
COMMENT ON COLUMN positions.realized_score IS 'Realized score with 8 decimal places precision to prevent float accumulation errors';
