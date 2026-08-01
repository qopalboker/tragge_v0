-- 0001_init.down.sql
-- Rollback initial schema for the Trading Tournament Platform

-- Drop triggers first
DROP TRIGGER IF EXISTS set_orders_updated_at ON orders;
DROP FUNCTION IF EXISTS trigger_set_updated_at();

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS leaderboard_snapshots;
DROP TABLE IF EXISTS positions;
DROP TABLE IF EXISTS fills;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS contest_participants;
DROP TABLE IF EXISTS contest_symbols;
DROP TABLE IF EXISTS contests;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;

-- Drop custom types
DROP TYPE IF EXISTS position_side;
DROP TYPE IF EXISTS order_status;
DROP TYPE IF EXISTS order_type;
DROP TYPE IF EXISTS order_side;
DROP TYPE IF EXISTS contest_status;

-- Note: We don't drop the uuid-ossp extension as other schemas might depend on it
