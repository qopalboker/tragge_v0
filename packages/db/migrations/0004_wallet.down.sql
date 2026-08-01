-- 0004_wallet.down.sql
-- Rollback wallet system

-- Drop triggers
DROP TRIGGER IF EXISTS create_wallet_on_user_insert ON users;
DROP FUNCTION IF EXISTS trigger_create_wallet_for_user();

DROP TRIGGER IF EXISTS set_payouts_updated_at ON payouts;
DROP TRIGGER IF EXISTS set_payment_intents_updated_at ON payment_intents;
DROP TRIGGER IF EXISTS set_wallets_updated_at ON wallets;

-- Drop tables (in dependency order)
DROP TABLE IF EXISTS payouts;
DROP TABLE IF EXISTS payment_intents;
DROP TABLE IF EXISTS wallet_ledger;
DROP TABLE IF EXISTS wallets;

-- Drop enums
DROP TYPE IF EXISTS payout_status;
DROP TYPE IF EXISTS payment_intent_status;
DROP TYPE IF EXISTS ledger_ref_type;
DROP TYPE IF EXISTS ledger_type;
DROP TYPE IF EXISTS wallet_status;
