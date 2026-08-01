-- 0014_tralent_score.down.sql
-- Rollback Tralent Score system

-- Drop the calculation function
DROP FUNCTION IF EXISTS calculate_tralent_contribution(DECIMAL, INT, INT);

-- Drop the new index
DROP INDEX IF EXISTS idx_user_score_history_contribution;

-- Remove score_contribution column
ALTER TABLE user_score_history DROP COLUMN IF EXISTS score_contribution;

-- Rename tralent_score back to tragge_score
DROP INDEX IF EXISTS idx_user_stats_tralent_score;
ALTER TABLE user_stats RENAME COLUMN tralent_score TO tragge_score;
CREATE INDEX idx_user_stats_tragge_score ON user_stats(tragge_score DESC);

-- Restore original trigger function
CREATE OR REPLACE FUNCTION update_user_stats()
RETURNS TRIGGER AS $$
DECLARE
    v_total_contests INT;
    v_total_wins INT;
    v_total_top3 INT;
    v_total_score DECIMAL(20, 2);
    v_total_participants INT;
    v_tragge_score DECIMAL(20, 2);
    v_win_rate DECIMAL(5, 2);
    v_total_pnl DECIMAL(20, 2);
    v_total_trades BIGINT;
    v_avg_trade_duration INT;
    v_best_market VARCHAR(20);
    v_best_market_pnl DECIMAL(20, 2);
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
        COALESCE(AVG(avg_trade_duration_seconds), 0)
    INTO
        v_total_contests,
        v_total_wins,
        v_total_top3,
        v_total_score,
        v_total_participants,
        v_total_pnl,
        v_total_trades,
        v_avg_trade_duration
    FROM user_score_history
    WHERE user_id = NEW.user_id;

    -- Calculate Tragge Score: total_score * log(1 + total_participants)
    v_tragge_score := v_total_score * LN(1 + v_total_participants);

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
        tragge_score,
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
        v_tragge_score,
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
        tragge_score = EXCLUDED.tragge_score,
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
