-- Add realized_pnl column to fills table to track per-fill P&L
-- Fills that open positions have realized_pnl = 0
-- Fills that close/reduce positions store the calculated trade score

ALTER TABLE fills ADD COLUMN IF NOT EXISTS realized_pnl NUMERIC(20, 8) NOT NULL DEFAULT 0;

-- Also add to partitioned fills table if it exists
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'fills_partitioned') THEN
        ALTER TABLE fills_partitioned ADD COLUMN IF NOT EXISTS realized_pnl NUMERIC(20, 8) NOT NULL DEFAULT 0;
    END IF;
END $$;
