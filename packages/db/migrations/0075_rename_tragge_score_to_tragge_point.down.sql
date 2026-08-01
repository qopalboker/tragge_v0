-- 0075_rename_tragge_score_to_tragge_point.down.sql
-- Revert tragge_point back to tragge_score

ALTER TABLE user_stats RENAME COLUMN tragge_point TO tragge_score;

DROP INDEX IF EXISTS idx_user_stats_tragge_point;
CREATE INDEX idx_user_stats_tragge_score ON user_stats(tragge_score DESC);

ALTER TABLE final_rankings RENAME COLUMN tragge_point_contribution TO tralent_score_contribution;

-- Restore the old trigger function (uses tragge_score)
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
    v_best_rank INT;
BEGIN
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

    SELECT COALESCE(SUM(score_contribution), 0)
    INTO v_tragge_score
    FROM user_score_history
    WHERE user_id = NEW.user_id;

    IF v_total_contests > 0 THEN
        v_win_rate := (v_total_wins::DECIMAL / v_total_contests::DECIMAL) * 100;
    ELSE
        v_win_rate := 0;
    END IF;

    SELECT top_symbol, SUM(top_symbol_pnl)
    INTO v_best_market, v_best_market_pnl
    FROM user_score_history
    WHERE user_id = NEW.user_id AND top_symbol IS NOT NULL
    GROUP BY top_symbol
    ORDER BY SUM(top_symbol_pnl) DESC
    LIMIT 1;

    INSERT INTO user_stats (
        user_id, total_contests, total_wins, total_top3, total_score,
        total_participants, tragge_score, win_rate, avg_trade_duration_seconds,
        best_market, best_market_pnl, total_trades, total_pnl, updated_at
    ) VALUES (
        NEW.user_id, v_total_contests, v_total_wins, v_total_top3, v_total_score,
        v_total_participants, v_tragge_score, v_win_rate, v_avg_trade_duration,
        v_best_market, COALESCE(v_best_market_pnl, 0), v_total_trades, v_total_pnl, NOW()
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

-- Restore old function name
CREATE OR REPLACE FUNCTION calculate_tragge_contribution(
    p_score DECIMAL(20, 2),
    p_rank INT,
    p_participants INT
) RETURNS DECIMAL(20, 4) AS $$
DECLARE
    v_participant_mult DECIMAL(10, 6);
    v_rank_bonus DECIMAL(10, 6);
    v_contribution DECIMAL(20, 4);
BEGIN
    IF p_score <= 0 THEN RETURN 0; END IF;
    IF p_participants <= 0 THEN RETURN 0; END IF;

    v_participant_mult := LOG(GREATEST(p_participants, 1)) / LOG(1000);
    v_participant_mult := GREATEST(0.1, LEAST(1.5, v_participant_mult));
    v_rank_bonus := 1.0 + (0.5 * (1.0 - (p_rank::DECIMAL / p_participants::DECIMAL)));
    v_rank_bonus := GREATEST(1.0, LEAST(1.5, v_rank_bonus));
    v_contribution := p_score * v_participant_mult * v_rank_bonus;

    RETURN v_contribution;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

DROP FUNCTION IF EXISTS calculate_tragge_point_contribution(DECIMAL, INT, INT);
