-- 0029_wallet_idempotency.down.sql
-- Rollback: Remove idempotency key from wallet_ledger

-- Drop indices first
DROP INDEX IF EXISTS idx_wallet_ledger_idempotency_lookup;
DROP INDEX IF EXISTS idx_wallet_ledger_idempotency_key;

-- Drop the column
ALTER TABLE wallet_ledger DROP COLUMN IF EXISTS idempotency_key;
