-- 0053_order_type_directional.up.sql
-- Add directional order types (buy_limit, sell_limit, buy_stop, sell_stop) to order_type enum.
-- These match the contract definitions and eliminate the lossy mapping in the trading engine.
--
-- NOTE: Constraint updates are in migration 0054 because PostgreSQL does not allow
-- referencing newly-added enum values in the same transaction.

ALTER TYPE order_type ADD VALUE IF NOT EXISTS 'buy_limit';
ALTER TYPE order_type ADD VALUE IF NOT EXISTS 'sell_limit';
ALTER TYPE order_type ADD VALUE IF NOT EXISTS 'buy_stop';
ALTER TYPE order_type ADD VALUE IF NOT EXISTS 'sell_stop';
