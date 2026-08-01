-- 0007_leaderboard_indexes.down.sql
-- Remove leaderboard and trading query optimization indexes

DROP INDEX IF EXISTS idx_positions_leaderboard;
DROP INDEX IF EXISTS idx_orders_pending_by_symbol;
DROP INDEX IF EXISTS idx_contest_participants_user_history;
