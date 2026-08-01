-- 0014_tralent_score.up.sql
-- Implement Tralent Score system with enhanced formula
-- Formula: sum(score_contribution) where each contribution = tournament_score * participant_multiplier * rank_bonus
-- - participant_multiplier = log10(participants) / log10(1000), clamped to [0.1, 1.5]
-- - rank_bonus = 1.0 + (0.5 * (1 - rank/total))

-- ============================================================================
-- RENAME tragge_score to tralent_score in user_stats
-- ============================================================================

ALTER TABLE user_stats RENAME COLUMN tragge_score TO tralent_score;

-- Update index name
DROP INDEX IF EXISTS idx_user_stats_tragge_score;
CREATE INDEX idx_user_stats_tralent_score ON user_stats(tralent_score DESC);

-- ============================================================================
-- ADD score_contribution to user_score_history
-- ============================================================================

ALTER TABLE user_score_history
ADD COLUMN score_contribution DECIMAL(20, 4) NOT NULL DEFAULT 0;

-- Index for quick lookups
CREATE INDEX idx_user_score_history_contribution ON user_score_history(user_id, score_contribution DESC);

-- ============================================================================
-- FUNCTION: Calculate Tralent Score contribution for a single contest result
-- ============================================================================

CREATE OR REPLACE FUNCTION calculate_tralent_contribution(
    p_score DECIMAL(20, 2),
    p_rank INT,
    p_participants INT
) RETURNS DECIMAL(20, 4) AS $$
DECLARE
    v_participant_mult DECIMAL(10, 6);
    v_rank_bonus DECIMAL(10, 6);
    v_contribution DECIMAL(20, 4);
BEGIN
    -- Only positive scores contribute
    IF p_score <= 0 THEN
        RETURN 0;
    END IF;

    -- Handle edge case of no participants
    IF p_participants <= 0 THEN
        RETURN 0;
    END IF;

    -- Participant multiplier: log10(participants) / log10(1000)
    -- log10(10) = 1, log10(1000) = 3, so:
    -- 10 participants = 1/3 = 0.33
    -- 100 participants = 2/3 = 0.67
    -- 1000 participants = 3/3 = 1.0
    -- 10000 participants = 4/3 = 1.33
    v_participant_mult := LOG(GREATEST(p_participants, 1)) / LOG(1000);

    -- Clamp participant multiplier to [0.1, 1.5]
    v_participant_mult := GREATEST(0.1, LEAST(1.5, v_participant_mult));

    -- Rank bonus: 1.0 + (0.5 * (1 - rank/total))
    -- #1 in 100 = 1.0 + 0.5 * (1 - 0.01) = 1.495
    -- #50 in 100 = 1.0 + 0.5 * (1 - 0.5) = 1.25
    -- #100 in 100 = 1.0 + 0.5 * (1 - 1.0) = 1.0
    v_rank_bonus := 1.0 + (0.5 * (1.0 - (p_rank::DECIMAL / p_participants::DECIMAL)));

    -- Clamp rank bonus to [1.0, 1.5]
    v_rank_bonus := GREATEST(1.0, LEAST(1.5, v_rank_bonus));

    -- Calculate contribution
    v_contribution := p_score * v_participant_mult * v_rank_bonus;

    RETURN v_contribution;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ============================================================================
-- UPDATE trigger function to use new Tralent Score formula
-- ============================================================================

CREATE OR REPLACE FUNCTION update_user_stats()
RETURNS TRIGGER AS $$
DECLARE
    v_total_contests INT;
    v_total_wins INT;
    v_total_top3 INT;
    v_total_score DECIMAL(20, 2);
    v_total_participants INT;
    v_tralent_score DECIMAL(20, 2);
    v_win_rate DECIMAL(5, 2);
    v_total_pnl DECIMAL(20, 2);
    v_total_trades BIGINT;
    v_avg_trade_duration INT;
    v_best_market VARCHAR(20);
    v_best_market_pnl DECIMAL(20, 2);
    v_best_rank INT;
BEGIN
    -- Calculate aggregates from history
    SELECT
        COUNT(*),
        SUM(CASE WHEN rank = 1 THEN 1 ELSE 0 END),
        SUM(CASE WHEN rank <= 3 THEN 1 ELSE 0 END),
        SUM(score),
        SUM(participants),
        SUM(pnl),
        SUM(trades_count),
        COALESCE(AVG(avg_trade_duration_seconds), 0),
        MIN(rank)
    INTO
        v_total_contests,
        v_total_wins,
        v_total_top3,
        v_total_score,
        v_total_participants,
        v_total_pnl,
        v_total_trades,
        v_avg_trade_duration,
        v_best_rank
    FROM user_score_history
    WHERE user_id = NEW.user_id;

    -- Calculate Tralent Score: sum of all contest contributions
    -- Each contribution = tournament_score * participant_multiplier * rank_bonus
    SELECT COALESCE(SUM(score_contribution), 0)
    INTO v_tralent_score
    FROM user_score_history
    WHERE user_id = NEW.user_id;

    -- Calculate win rate
    IF v_total_contests > 0 THEN
        v_win_rate := (v_total_wins::DECIMAL / v_total_contests::DECIMAL) * 100;
    ELSE
        v_win_rate := 0;
    END IF;

    -- Find best market (symbol with highest total PnL)
    SELECT top_symbol, SUM(top_symbol_pnl)
    INTO v_best_market, v_best_market_pnl
    FROM user_score_history
    WHERE user_id = NEW.user_id AND top_symbol IS NOT NULL
    GROUP BY top_symbol
    ORDER BY SUM(top_symbol_pnl) DESC
    LIMIT 1;

    -- Insert or update user_stats
    INSERT INTO user_stats (
        user_id,
        total_contests,
        total_wins,
        total_top3,
        total_score,
        total_participants,
        tralent_score,
        win_rate,
        avg_trade_duration_seconds,
        best_market,
        best_market_pnl,
        total_trades,
        total_pnl,
        updated_at
    ) VALUES (
        NEW.user_id,
        v_total_contests,
        v_total_wins,
        v_total_top3,
        v_total_score,
        v_total_participants,
        v_tralent_score,
        v_win_rate,
        v_avg_trade_duration,
        v_best_market,
        COALESCE(v_best_market_pnl, 0),
        v_total_trades,
        v_total_pnl,
        NOW()
    )
    ON CONFLICT (user_id) DO UPDATE SET
        total_contests = EXCLUDED.total_contests,
        total_wins = EXCLUDED.total_wins,
        total_top3 = EXCLUDED.total_top3,
        total_score = EXCLUDED.total_score,
        total_participants = EXCLUDED.total_participants,
        tralent_score = EXCLUDED.tralent_score,
        win_rate = EXCLUDED.win_rate,
        avg_trade_duration_seconds = EXCLUDED.avg_trade_duration_seconds,
        best_market = EXCLUDED.best_market,
        best_market_pnl = EXCLUDED.best_market_pnl,
        total_trades = EXCLUDED.total_trades,
        total_pnl = EXCLUDED.total_pnl,
        updated_at = NOW();

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- BACKFILL: Calculate score_contribution for existing records
-- ============================================================================

UPDATE user_score_history
SET score_contribution = calculate_tralent_contribution(score, rank, participants)
WHERE score_contribution = 0;

-- ============================================================================
-- RECALCULATE: Update all user_stats with new Tralent Scores
-- ============================================================================

-- Force trigger to recalculate by touching all history records
UPDATE user_score_history SET score_contribution = score_contribution WHERE true;
