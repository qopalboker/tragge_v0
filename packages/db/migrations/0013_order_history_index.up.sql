-- 0013_order_history_index.up.sql
-- Add composite index on orders for efficient order history queries
-- Supports queries filtering by (user_id, contest_id) with ORDER BY created_at DESC

CREATE INDEX IF NOT EXISTS idx_orders_user_contest_created
ON orders (user_id, contest_id, created_at DESC);
