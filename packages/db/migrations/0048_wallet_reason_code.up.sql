-- 0048_wallet_reason_code.up.sql
-- Add reason_code column to wallet_ledger for categorized transaction descriptions

-- ============================================================================
-- ADD REASON CODE COLUMN
-- ============================================================================

ALTER TABLE wallet_ledger
ADD COLUMN IF NOT EXISTS reason_code VARCHAR(50);

-- ============================================================================
-- INDEX FOR REASON CODE LOOKUPS
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_wallet_ledger_reason_code
ON wallet_ledger(reason_code)
WHERE reason_code IS NOT NULL;

-- ============================================================================
-- COMPOSITE INDEX FOR USER + REASON CODE QUERIES
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_wallet_ledger_user_reason_code
ON wallet_ledger(user_id, reason_code)
WHERE reason_code IS NOT NULL;

-- ============================================================================
-- DOCUMENTATION
-- ============================================================================

COMMENT ON COLUMN wallet_ledger.reason_code IS
'Machine-readable reason code for the transaction. Values: CONTEST_ENTRY, CONTEST_ENTRY_FREE, CONTEST_REFUND_QUORUM, CONTEST_REFUND_ADMIN, CONTEST_PRIZE, WALLET_TOPUP, WALLET_WITHDRAW';
