-- 0006_user_stats.up.sql
-- User statistics and score history for global leaderboard

-- ============================================================================
-- USER STATS
-- ============================================================================

CREATE TABLE user_stats (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    total_contests INT NOT NULL DEFAULT 0,
    total_wins INT NOT NULL DEFAULT 0,
    total_top3 INT NOT NULL DEFAULT 0,
    total_score DECIMAL(20, 2) NOT NULL DEFAULT 0,
    total_participants INT NOT NULL DEFAULT 0, -- sum of participants across all contests
    tragge_score DECIMAL(20, 2) NOT NULL DEFAULT 0, -- total_score * log(1 + total_participants)
    win_rate DECIMAL(5, 2) NOT NULL DEFAULT 0, -- percentage (0-100)
    avg_trade_duration_seconds INT NOT NULL DEFAULT 0,
    best_market VARCHAR(20),
    best_market_pnl DECIMAL(20, 2) DEFAULT 0,
    total_trades BIGINT NOT NULL DEFAULT 0,
    total_pnl DECIMAL(20, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_stats_tragge_score ON user_stats(tragge_score DESC);
CREATE INDEX idx_user_stats_total_score ON user_stats(total_score DESC);
CREATE INDEX idx_user_stats_win_rate ON user_stats(win_rate DESC);
CREATE INDEX idx_user_stats_updated_at ON user_stats(updated_at);

-- ============================================================================
-- USER SCORE HISTORY
-- ============================================================================

CREATE TABLE user_score_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    rank INT NOT NULL,
    score DECIMAL(20, 2) NOT NULL,
    participants INT NOT NULL,
    pnl DECIMAL(20, 2) NOT NULL DEFAULT 0,
    trades_count INT NOT NULL DEFAULT 0,
    avg_trade_duration_seconds INT NOT NULL DEFAULT 0,
    top_symbol VARCHAR(20),
    top_symbol_pnl DECIMAL(20, 2) DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_rank_positive CHECK (rank > 0),
    CONSTRAINT chk_participants_positive CHECK (participants > 0),
    UNIQUE(user_id, contest_id)
);

CREATE INDEX idx_user_score_history_user_id ON user_score_history(user_id);
CREATE INDEX idx_user_score_history_contest_id ON user_score_history(contest_id);
CREATE INDEX idx_user_score_history_created_at ON user_score_history(created_at);
CREATE INDEX idx_user_score_history_rank ON user_score_history(rank);

-- ============================================================================
-- FUNCTION TO UPDATE USER STATS
-- ============================================================================

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

-- Trigger to automatically update user_stats when history changes
CREATE TRIGGER trigger_update_user_stats
    AFTER INSERT OR UPDATE ON user_score_history
    FOR EACH ROW
    EXECUTE FUNCTION update_user_stats();
