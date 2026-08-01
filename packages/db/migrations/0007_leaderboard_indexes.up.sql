-- 0007_leaderboard_indexes.up.sql
-- Add missing indexes for leaderboard and trading query optimization

-- ============================================================================
-- LEADERBOARD QUERY INDEXES
-- ============================================================================

-- Index for leaderboard queries: positions ranked by realized_score for open positions
-- Used for real-time leaderboard calculations where we need to rank users by their
-- realized P&L within a contest, considering only open (active) positions
CREATE INDEX idx_positions_leaderboard ON positions(contest_id, realized_score DESC)
    WHERE closed_at IS NULL;

-- ============================================================================
-- PENDING ORDER INDEXES
-- ============================================================================

-- Composite index for pending order lookups by contest and symbol
-- The trading engine frequently queries pending orders to evaluate them against
-- incoming price ticks, ordered by creation time for FIFO processing
CREATE INDEX idx_orders_pending_by_symbol ON orders(contest_id, symbol, created_at)
    WHERE status = 'pending';

-- ============================================================================
-- USER CONTEST HISTORY INDEXES
-- ============================================================================

-- Index for user contest history queries
-- Supports queries like "show me all contests this user has participated in"
-- ordered by most recent first
CREATE INDEX idx_contest_participants_user_history ON contest_participants(user_id, joined_at DESC);
