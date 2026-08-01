-- 0011_affiliate_program.down.sql
-- Rollback affiliate/referral program system

-- Drop trigger first
DROP TRIGGER IF EXISTS create_user_referral_code ON users;

-- Drop functions
DROP FUNCTION IF EXISTS trigger_create_referral_code();
DROP FUNCTION IF EXISTS generate_referral_code();

-- Drop view
DROP VIEW IF EXISTS affiliate_stats;

-- Drop tables in reverse order of creation (respecting foreign key dependencies)
DROP TABLE IF EXISTS affiliate_commissions;
DROP TABLE IF EXISTS referrals;
DROP TABLE IF EXISTS referral_codes;

-- Drop enums
DROP TYPE IF EXISTS commission_status;
DROP TYPE IF EXISTS referral_status;
