DROP TABLE IF EXISTS contest_prize_locks;
ALTER TABLE contests DROP COLUMN IF EXISTS prizes_locked_at;
ALTER TABLE contests DROP COLUMN IF EXISTS prize_pool_net_cents;
