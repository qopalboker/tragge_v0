-- Remove realized_pnl column from fills tables

ALTER TABLE fills DROP COLUMN IF EXISTS realized_pnl;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'fills_partitioned') THEN
        ALTER TABLE fills_partitioned DROP COLUMN IF EXISTS realized_pnl;
    END IF;
END $$;
