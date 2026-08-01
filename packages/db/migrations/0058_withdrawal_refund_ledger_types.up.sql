-- 0058_withdrawal_refund_ledger_types.up.sql
-- Add 'withdrawal_refund' and 'withdraw_fee_refund' to ledger_type enum
-- for proper tracking of refunded amounts on rejected withdrawals (BUG #148)

ALTER TYPE ledger_type ADD VALUE IF NOT EXISTS 'withdrawal_refund';
ALTER TYPE ledger_type ADD VALUE IF NOT EXISTS 'withdraw_fee_refund';
