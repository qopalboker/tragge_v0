-- 0025_withdrawal_management.up.sql
-- Add admin withdrawal management columns to payouts table

-- ============================================================================
-- ADD REJECTED STATUS TO PAYOUT_STATUS ENUM
-- ============================================================================

-- Add 'rejected' value to payout_status enum if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
        WHERE enumlabel = 'rejected'
        AND enumtypid = (SELECT oid FROM pg_type WHERE typname = 'payout_status')
    ) THEN
        ALTER TYPE payout_status ADD VALUE 'rejected';
    END IF;
END $$;

-- ============================================================================
-- ADD ADMIN REVIEW COLUMNS TO PAYOUTS TABLE
-- ============================================================================

-- Add admin_comment column for admin notes/reasons
ALTER TABLE payouts ADD COLUMN IF NOT EXISTS admin_comment TEXT;

-- Add reviewed_by column to track which admin reviewed the withdrawal
ALTER TABLE payouts ADD COLUMN IF NOT EXISTS reviewed_by UUID REFERENCES users(id);

-- Add reviewed_at timestamp for when the review occurred
ALTER TABLE payouts ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;

-- ============================================================================
-- ADD INDEXES FOR EFFICIENT QUERIES
-- ============================================================================

-- Index for filtering by reviewer
CREATE INDEX IF NOT EXISTS idx_payouts_reviewed_by ON payouts(reviewed_by);

-- Index for filtering by reviewed_at (for reports)
CREATE INDEX IF NOT EXISTS idx_payouts_reviewed_at ON payouts(reviewed_at);

-- Composite index for pending withdrawals (commonly queried for admin review)
CREATE INDEX IF NOT EXISTS idx_payouts_pending_review ON payouts(status, created_at DESC)
WHERE status = 'pending';

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON COLUMN payouts.admin_comment IS 'Admin notes or reason for approval/rejection';
COMMENT ON COLUMN payouts.reviewed_by IS 'UUID of the admin who reviewed the withdrawal';
COMMENT ON COLUMN payouts.reviewed_at IS 'Timestamp when the withdrawal was reviewed';
