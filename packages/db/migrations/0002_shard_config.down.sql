-- 0002_shard_config.down.sql
-- Rollback shard configuration migration

-- Drop trigger first
DROP TRIGGER IF EXISTS set_shard_config_updated_at ON shard_config;

-- Drop shard assignment log table
DROP TABLE IF EXISTS shard_assignment_log;

-- Remove shard_id from contests (drop constraint and index first)
ALTER TABLE contests DROP CONSTRAINT IF EXISTS fk_contests_shard;
DROP INDEX IF EXISTS idx_contests_shard_id;
ALTER TABLE contests DROP COLUMN IF EXISTS shard_id;

-- Drop shard config table
DROP TABLE IF EXISTS shard_config;

-- Drop shard status enum
DROP TYPE IF EXISTS shard_status;
