-- 0048_wallet_reason_code.down.sql
-- Revert reason_code column addition

DROP INDEX IF EXISTS idx_wallet_ledger_user_reason_code;
DROP INDEX IF EXISTS idx_wallet_ledger_reason_code;
ALTER TABLE wallet_ledger DROP COLUMN IF EXISTS reason_code;
