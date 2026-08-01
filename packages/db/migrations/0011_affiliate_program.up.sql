-- 0011_affiliate_program.up.sql
-- Affiliate/Referral program system

-- ============================================================================
-- ENUMS
-- ============================================================================

CREATE TYPE referral_status AS ENUM (
    'pending',
    'qualified',
    'paid'
);

CREATE TYPE commission_status AS ENUM (
    'pending',
    'credited',
    'cancelled'
);

-- ============================================================================
-- REFERRAL CODES
-- ============================================================================

CREATE TABLE referral_codes (
    code VARCHAR(20) PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    commission_rate_bps INT NOT NULL DEFAULT 500, -- 5% = 500 basis points
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_referral_codes_user_id UNIQUE (user_id),
    CONSTRAINT chk_commission_rate_valid CHECK (commission_rate_bps >= 0 AND commission_rate_bps <= 10000)
);

CREATE INDEX idx_referral_codes_user_id ON referral_codes(user_id);
CREATE INDEX idx_referral_codes_is_active ON referral_codes(is_active);
CREATE INDEX idx_referral_codes_created_at ON referral_codes(created_at);

-- ============================================================================
-- REFERRALS
-- ============================================================================

CREATE TABLE referrals (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    referrer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    referred_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code VARCHAR(20) NOT NULL REFERENCES referral_codes(code) ON DELETE RESTRICT,
    status referral_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    qualified_at TIMESTAMPTZ,

    CONSTRAINT uq_referrals_referred_id UNIQUE (referred_id),
    CONSTRAINT chk_referrer_not_referred CHECK (referrer_id != referred_id)
);

CREATE INDEX idx_referrals_referrer_id ON referrals(referrer_id);
CREATE INDEX idx_referrals_referred_id ON referrals(referred_id);
CREATE INDEX idx_referrals_code ON referrals(code);
CREATE INDEX idx_referrals_status ON referrals(status);
CREATE INDEX idx_referrals_created_at ON referrals(created_at);
CREATE INDEX idx_referrals_qualified_at ON referrals(qualified_at);
CREATE INDEX idx_referrals_referrer_status ON referrals(referrer_id, status);

-- ============================================================================
-- AFFILIATE COMMISSIONS
-- ============================================================================

CREATE TABLE affiliate_commissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    referrer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    referred_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type VARCHAR(50) NOT NULL, -- 'contest_entry', 'deposit'
    source_id UUID NOT NULL,
    gross_amount_cents BIGINT NOT NULL,
    commission_rate_bps INT NOT NULL,
    commission_cents BIGINT NOT NULL,
    status commission_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    credited_at TIMESTAMPTZ,

    CONSTRAINT chk_gross_amount_positive CHECK (gross_amount_cents > 0),
    CONSTRAINT chk_commission_rate_valid CHECK (commission_rate_bps >= 0 AND commission_rate_bps <= 10000),
    CONSTRAINT chk_commission_cents_valid CHECK (commission_cents >= 0)
);

CREATE INDEX idx_affiliate_commissions_referrer_id ON affiliate_commissions(referrer_id);
CREATE INDEX idx_affiliate_commissions_referred_id ON affiliate_commissions(referred_id);
CREATE INDEX idx_affiliate_commissions_source_type ON affiliate_commissions(source_type);
CREATE INDEX idx_affiliate_commissions_source_id ON affiliate_commissions(source_id);
CREATE INDEX idx_affiliate_commissions_status ON affiliate_commissions(status);
CREATE INDEX idx_affiliate_commissions_created_at ON affiliate_commissions(created_at);
CREATE INDEX idx_affiliate_commissions_credited_at ON affiliate_commissions(credited_at);
CREATE INDEX idx_affiliate_commissions_referrer_status ON affiliate_commissions(referrer_id, status);
CREATE INDEX idx_affiliate_commissions_source ON affiliate_commissions(source_type, source_id);

-- ============================================================================
-- AFFILIATE STATS VIEW
-- ============================================================================

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
-- FUNCTION: Generate unique referral code
-- ============================================================================

CREATE OR REPLACE FUNCTION generate_referral_code()
RETURNS VARCHAR(20) AS $$
DECLARE
    new_code VARCHAR(20);
    chars VARCHAR(36) := 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
    i INT;
    code_exists BOOLEAN;
BEGIN
    LOOP
        new_code := '';
        FOR i IN 1..8 LOOP
            new_code := new_code || substr(chars, floor(random() * 36 + 1)::int, 1);
        END LOOP;

        SELECT EXISTS(SELECT 1 FROM referral_codes WHERE code = new_code) INTO code_exists;

        IF NOT code_exists THEN
            RETURN new_code;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- FUNCTION: Auto-create referral code for new user
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
-- TRIGGER: Create referral code after user creation
-- ============================================================================

CREATE TRIGGER create_user_referral_code
    AFTER INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION trigger_create_referral_code();

-- ============================================================================
-- Generate referral codes for existing users
-- ============================================================================

INSERT INTO referral_codes (code, user_id)
SELECT generate_referral_code(), u.id
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM referral_codes rc WHERE rc.user_id = u.id
);
