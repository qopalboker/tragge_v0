-- 0057_withdraw_fee_and_expired_status.up.sql
-- Add 'withdraw_fee' to ledger_type enum for separate fee tracking (BUG #128)
-- Add 'expired' to payment_intent_status enum for orphaned intent cleanup (BUG #129)

ALTER TYPE ledger_type ADD VALUE IF NOT EXISTS 'withdraw_fee';
ALTER TYPE payment_intent_status ADD VALUE IF NOT EXISTS 'expired';
