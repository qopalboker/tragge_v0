-- 0019_settlement_tables.down.sql
-- Rollback settlement tracking tables

-- Drop triggers
DROP TRIGGER IF EXISTS set_contest_settlements_updated_at ON contest_settlements;

-- Drop tables in reverse order
DROP TABLE IF EXISTS settlement_events;
DROP TABLE IF EXISTS final_rankings;
DROP TABLE IF EXISTS prize_distributions;
DROP TABLE IF EXISTS contest_settlements;

-- Drop enums
DROP TYPE IF EXISTS prize_status;
DROP TYPE IF EXISTS settlement_status;
