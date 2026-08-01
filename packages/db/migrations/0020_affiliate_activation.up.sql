-- 0020_affiliate_activation.up.sql
-- Add affiliate activation request flow

-- ============================================================================
-- ENUM: Affiliate activation status
-- ============================================================================

CREATE TYPE affiliate_activation_status AS ENUM (
    'inactive',
    'pending',
    'active',
    'rejected'
);

-- ============================================================================
-- ALTER referral_codes table
-- ============================================================================

-- Add activation columns to referral_codes
ALTER TABLE referral_codes
    ADD COLUMN activation_status affiliate_activation_status NOT NULL DEFAULT 'inactive',
    ADD COLUMN activation_requested_at TIMESTAMPTZ,
    ADD COLUMN activation_approved_at TIMESTAMPTZ,
    ADD COLUMN activation_rejected_at TIMESTAMPTZ,
    ADD COLUMN rejection_reason TEXT;

-- Update existing active codes to have 'active' activation_status
-- This ensures existing users with is_active=true keep their functionality
UPDATE referral_codes
SET activation_status = 'active',
    activation_approved_at = created_at
WHERE is_active = TRUE;

-- Set is_active to false by default for new users
-- (activation_status controls the workflow now)
ALTER TABLE referral_codes
    ALTER COLUMN is_active SET DEFAULT FALSE;

-- Create index on activation_status for admin queries
CREATE INDEX idx_referral_codes_activation_status ON referral_codes(activation_status);
CREATE INDEX idx_referral_codes_activation_requested_at ON referral_codes(activation_requested_at);

-- ============================================================================
-- Update the trigger function to set is_active=false by default
-- ============================================================================

CREATE OR REPLACE FUNCTION trigger_create_referral_code()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO referral_codes (code, user_id, is_active, activation_status)
    VALUES (generate_referral_code(), NEW.id, FALSE, 'inactive');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- Update affiliate_stats view to include activation info
-- ============================================================================

DROP VIEW IF EXISTS affiliate_stats;

CREATE VIEW affiliate_stats AS
SELECT
    rc.user_id AS referrer_id,
    rc.code,
    rc.commission_rate_bps,
    rc.is_active,
    rc.activation_status,
    rc.activation_requested_at,
    rc.activation_approved_at,
    COUNT(r.id) AS total_referrals,
    COUNT(CASE WHEN r.status IN ('qualified', 'paid') THEN 1 END) AS qualified_referrals,
    COALESCE(SUM(CASE WHEN ac.status = 'credited' THEN ac.commission_cents ELSE 0 END), 0) AS total_earned_cents,
    COALESCE(SUM(CASE WHEN ac.status = 'pending' THEN ac.commission_cents ELSE 0 END), 0) AS pending_cents
FROM referral_codes rc
LEFT JOIN referrals r ON rc.code = r.code
LEFT JOIN affiliate_commissions ac ON rc.user_id = ac.referrer_id
GROUP BY rc.user_id, rc.code, rc.commission_rate_bps, rc.is_active,
         rc.activation_status, rc.activation_requested_at, rc.activation_approved_at;
