-- Remove unjoin configuration columns from contests (leave functionality removed)
ALTER TABLE contests DROP COLUMN IF EXISTS allow_unjoin;
ALTER TABLE contests DROP COLUMN IF EXISTS unjoin_deadline_minutes;
