-- 0013_order_history_index.down.sql
-- Remove order history index

DROP INDEX IF EXISTS idx_orders_user_contest_created;
