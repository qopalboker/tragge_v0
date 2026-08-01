-- Migration: 0055_settlement_batch_indexes
-- Purpose: Add partial composite index on orders for settlement batch queries.
-- Bug #104: getUserStatsForContest runs per-user queries during finalization.
-- This index accelerates the filled-order count and orders→fills JOIN
-- when filtering by (contest_id, user_id) with status='filled'.

CREATE INDEX IF NOT EXISTS idx_orders_contest_user_filled
  ON orders(contest_id, user_id)
  WHERE status = 'filled';
