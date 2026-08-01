-- 0032_decimal_scores.down.sql
-- Revert score columns back to NUMERIC(20,4)

-- ============================================================================
-- Revert contest_participants total_score
-- ============================================================================

ALTER TABLE contest_participants
ALTER COLUMN total_score TYPE NUMERIC(20, 4);

-- ============================================================================
-- Revert positions realized_score
-- ============================================================================

ALTER TABLE positions
ALTER COLUMN realized_score TYPE NUMERIC(20, 4);

-- ============================================================================
-- Revert settlement_snapshots if it exists
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'settlement_snapshots') THEN
        ALTER TABLE settlement_snapshots
        ALTER COLUMN realized_score TYPE NUMERIC(20, 4);

        ALTER TABLE settlement_snapshots
        ALTER COLUMN unrealized_score TYPE NUMERIC(20, 4);
    END IF;
END $$;

-- ============================================================================
-- Revert user_stats columns if they exist
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'user_stats' AND column_name = 'total_score') THEN
        ALTER TABLE user_stats
        ALTER COLUMN total_score TYPE DECIMAL(20, 2);
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'user_stats' AND column_name = 'tralent_score') THEN
        ALTER TABLE user_stats
        ALTER COLUMN tralent_score TYPE DECIMAL(20, 2);
    END IF;
END $$;

-- ============================================================================
-- Revert user_score_history columns if they exist
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'user_score_history' AND column_name = 'score') THEN
        ALTER TABLE user_score_history
        ALTER COLUMN score TYPE DECIMAL(20, 2);
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'user_score_history' AND column_name = 'score_contribution') THEN
        ALTER TABLE user_score_history
        ALTER COLUMN score_contribution TYPE DECIMAL(20, 4);
    END IF;
END $$;

-- ============================================================================
-- Revert leaderboard_snapshots if it exists
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'leaderboard_snapshots' AND column_name = 'score') THEN
        ALTER TABLE leaderboard_snapshots
        ALTER COLUMN score TYPE NUMERIC(20, 4);
    END IF;
END $$;

-- ============================================================================
-- Remove comments
-- ============================================================================

COMMENT ON COLUMN contest_participants.total_score IS NULL;
COMMENT ON COLUMN positions.realized_score IS NULL;
