-- 0003_partition_tables.down.sql
-- Rollback database partitioning for high-volume tables

-- ============================================================================
-- DROP MONITORING VIEWS
-- ============================================================================

DROP VIEW IF EXISTS shard_distribution;
DROP VIEW IF EXISTS partition_stats;

-- ============================================================================
-- DROP BACKWARD COMPATIBILITY VIEWS
-- ============================================================================

DROP VIEW IF EXISTS orders_compat;
DROP VIEW IF EXISTS fills_compat;
DROP VIEW IF EXISTS positions_compat;

-- ============================================================================
-- DROP PARTITION MANAGEMENT FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS migrate_contest_to_shard(UUID, INT);
DROP FUNCTION IF EXISTS drop_shard_partitions(INT);
DROP FUNCTION IF EXISTS create_shard_partitions(INT);

-- ============================================================================
-- DROP TRIGGER ON PARTITIONED ORDERS TABLE
-- ============================================================================

DROP TRIGGER IF EXISTS set_orders_updated_at ON orders;

-- ============================================================================
-- RENAME PARTITIONED TABLES TO BACKUP NAMES
-- ============================================================================

ALTER TABLE orders RENAME TO orders_partitioned_backup;
ALTER TABLE fills RENAME TO fills_partitioned_backup;
ALTER TABLE positions RENAME TO positions_partitioned_backup;

-- ============================================================================
-- RESTORE ORIGINAL TABLES FROM OLD BACKUP
-- ============================================================================

-- Restore orders table
ALTER TABLE orders_old RENAME TO orders;

-- Restore fills table
ALTER TABLE fills_old RENAME TO fills;

-- Restore positions table
ALTER TABLE positions_old RENAME TO positions;

-- ============================================================================
-- RECREATE TRIGGER ON RESTORED ORDERS TABLE
-- ============================================================================

CREATE TRIGGER set_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================================
-- DROP PARTITIONED BACKUP TABLES AND THEIR PARTITIONS
-- ============================================================================

-- Drop orders partitions
DROP TABLE IF EXISTS orders_shard_0 CASCADE;
DROP TABLE IF EXISTS orders_shard_1 CASCADE;
DROP TABLE IF EXISTS orders_shard_2 CASCADE;
DROP TABLE IF EXISTS orders_shard_3 CASCADE;
DROP TABLE IF EXISTS orders_partitioned_backup CASCADE;

-- Drop fills partitions
DROP TABLE IF EXISTS fills_shard_0 CASCADE;
DROP TABLE IF EXISTS fills_shard_1 CASCADE;
DROP TABLE IF EXISTS fills_shard_2 CASCADE;
DROP TABLE IF EXISTS fills_shard_3 CASCADE;
DROP TABLE IF EXISTS fills_partitioned_backup CASCADE;

-- Drop positions partitions
DROP TABLE IF EXISTS positions_shard_0 CASCADE;
DROP TABLE IF EXISTS positions_shard_1 CASCADE;
DROP TABLE IF EXISTS positions_shard_2 CASCADE;
DROP TABLE IF EXISTS positions_shard_3 CASCADE;
DROP TABLE IF EXISTS positions_partitioned_backup CASCADE;

-- ============================================================================
-- REMOVE ADDED SHARDS FROM SHARD_CONFIG
-- ============================================================================

-- Remove shards 1, 2, 3 (keep shard 0 as default)
DELETE FROM shard_config WHERE shard_id IN (1, 2, 3);

-- ============================================================================
-- DROP HELPER FUNCTION
-- ============================================================================

DROP FUNCTION IF EXISTS get_shard_id_for_contest(UUID);

-- ============================================================================
-- CLEANUP: REMOVE OLD BACKUP TABLES IF THEY STILL EXIST
-- ============================================================================

-- These should have been renamed back, but clean up just in case
DROP TABLE IF EXISTS orders_old CASCADE;
DROP TABLE IF EXISTS fills_old CASCADE;
DROP TABLE IF EXISTS positions_old CASCADE;
