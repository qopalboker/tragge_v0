-- 0025_withdrawal_management.down.sql
-- Rollback admin withdrawal management columns from payouts table

-- ============================================================================
-- DROP INDEXES
-- ============================================================================

DROP INDEX IF EXISTS idx_payouts_pending_review;
DROP INDEX IF EXISTS idx_payouts_reviewed_at;
DROP INDEX IF EXISTS idx_payouts_reviewed_by;

-- ============================================================================
-- DROP COLUMNS
-- ============================================================================

ALTER TABLE payouts DROP COLUMN IF EXISTS reviewed_at;
ALTER TABLE payouts DROP COLUMN IF EXISTS reviewed_by;
ALTER TABLE payouts DROP COLUMN IF EXISTS admin_comment;

-- ============================================================================
-- NOTE: Cannot remove enum value 'rejected' from payout_status
-- PostgreSQL does not support removing enum values
-- The value will remain but be unused after rollback
-- ============================================================================
