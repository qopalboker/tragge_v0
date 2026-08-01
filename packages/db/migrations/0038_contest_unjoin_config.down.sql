-- Remove allow_unjoin configuration from contests
ALTER TABLE contests DROP COLUMN IF EXISTS unjoin_deadline_minutes;
ALTER TABLE contests DROP COLUMN IF EXISTS allow_unjoin;
