-- 0020_affiliate_activation.down.sql
-- Rollback affiliate activation request flow

-- ============================================================================
-- Restore original affiliate_stats view
-- ============================================================================

DROP VIEW IF EXISTS affiliate_stats;

CREATE VIEW affiliate_stats AS
SELECT
    rc.user_id AS referrer_id,
    rc.code,
    rc.commission_rate_bps,
    rc.is_active,
    COUNT(r.id) AS total_referrals,
    COUNT(CASE WHEN r.status IN ('qualified', 'paid') THEN 1 END) AS qualified_referrals,
    COALESCE(SUM(CASE WHEN ac.status = 'credited' THEN ac.commission_cents ELSE 0 END), 0) AS total_earned_cents,
    COALESCE(SUM(CASE WHEN ac.status = 'pending' THEN ac.commission_cents ELSE 0 END), 0) AS pending_cents
FROM referral_codes rc
LEFT JOIN referrals r ON rc.code = r.code
LEFT JOIN affiliate_commissions ac ON rc.user_id = ac.referrer_id
GROUP BY rc.user_id, rc.code, rc.commission_rate_bps, rc.is_active;

-- ============================================================================
-- Restore original trigger function
-- ============================================================================

CREATE OR REPLACE FUNCTION trigger_create_referral_code()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO referral_codes (code, user_id)
    VALUES (generate_referral_code(), NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- Drop indexes
-- ============================================================================

DROP INDEX IF EXISTS idx_referral_codes_activation_status;
DROP INDEX IF EXISTS idx_referral_codes_activation_requested_at;

-- ============================================================================
-- Restore is_active default and remove activation columns
-- ============================================================================

ALTER TABLE referral_codes
    ALTER COLUMN is_active SET DEFAULT TRUE;

ALTER TABLE referral_codes
    DROP COLUMN IF EXISTS activation_status,
    DROP COLUMN IF EXISTS activation_requested_at,
    DROP COLUMN IF EXISTS activation_approved_at,
    DROP COLUMN IF EXISTS activation_rejected_at,
    DROP COLUMN IF EXISTS rejection_reason;

-- ============================================================================
-- Drop enum type
-- ============================================================================

DROP TYPE IF EXISTS affiliate_activation_status;
