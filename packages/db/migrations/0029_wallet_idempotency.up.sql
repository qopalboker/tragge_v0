-- 0029_wallet_idempotency.up.sql
-- Add idempotency key to wallet_ledger for preventing duplicate prize credits

-- ============================================================================
-- ADD IDEMPOTENCY KEY COLUMN
-- ============================================================================

-- Add idempotency_key column to wallet_ledger
-- This enables idempotent operations for prize credits and other wallet transactions
ALTER TABLE wallet_ledger
ADD COLUMN idempotency_key VARCHAR(255);

-- ============================================================================
-- UNIQUE CONSTRAINT FOR IDEMPOTENCY
-- ============================================================================

-- Add unique constraint on idempotency_key
-- This prevents duplicate transactions with the same idempotency key
-- NULL values are allowed (for legacy entries and operations that don't need idempotency)
CREATE UNIQUE INDEX idx_wallet_ledger_idempotency_key
ON wallet_ledger(idempotency_key)
WHERE idempotency_key IS NOT NULL;

-- ============================================================================
-- INDEX FOR IDEMPOTENCY KEY LOOKUPS
-- ============================================================================

-- Partial index for fast idempotency checks
-- Only indexes non-null idempotency keys for efficient lookups
CREATE INDEX idx_wallet_ledger_idempotency_lookup
ON wallet_ledger(idempotency_key)
WHERE idempotency_key IS NOT NULL;

-- ============================================================================
-- COMMENT FOR DOCUMENTATION
-- ============================================================================

COMMENT ON COLUMN wallet_ledger.idempotency_key IS
'Unique key for idempotent operations. Format for prize credits: finalization:{contest_id}:{user_id}:{rank}';
