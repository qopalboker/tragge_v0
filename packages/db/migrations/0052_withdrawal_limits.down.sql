-- 0052_withdrawal_limits.down.sql
-- Revert withdrawal limits

DROP TRIGGER IF EXISTS set_withdrawal_limits_updated_at ON withdrawal_limits;
DROP TABLE IF EXISTS withdrawal_limits;
DROP INDEX IF EXISTS idx_payouts_user_created_status;
