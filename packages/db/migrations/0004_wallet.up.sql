-- 0004_wallet.up.sql
-- Wallet system for managing user balances, payments, and payouts

-- ============================================================================
-- WALLET STATUS ENUM
-- ============================================================================

CREATE TYPE wallet_status AS ENUM (
    'active',
    'frozen',
    'closed'
);

-- ============================================================================
-- WALLETS TABLE
-- ============================================================================

CREATE TABLE wallets (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    balance_cents BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status wallet_status NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_balance_non_negative CHECK (balance_cents >= 0)
);

CREATE INDEX idx_wallets_status ON wallets(status);
CREATE INDEX idx_wallets_created_at ON wallets(created_at);

-- ============================================================================
-- WALLET LEDGER TYPES
-- ============================================================================

CREATE TYPE ledger_type AS ENUM (
    'deposit',           -- Money added to wallet
    'withdrawal',        -- Money withdrawn from wallet
    'contest_entry',     -- Entry fee paid for contest
    'contest_refund',    -- Entry fee refunded (contest cancelled, etc.)
    'prize_credit',      -- Prize money credited from contest win
    'adjustment'         -- Manual adjustment by admin
);

CREATE TYPE ledger_ref_type AS ENUM (
    'payment_intent',    -- Reference to payment_intents table
    'payout',            -- Reference to payouts table
    'contest',           -- Reference to contests table
    'admin_action'       -- Reference to audit_logs table
);

-- ============================================================================
-- WALLET LEDGER TABLE
-- ============================================================================

CREATE TABLE wallet_ledger (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type ledger_type NOT NULL,
    amount_cents BIGINT NOT NULL,
    balance_after_cents BIGINT NOT NULL,
    ref_type ledger_ref_type,
    ref_id UUID,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_amount_non_zero CHECK (amount_cents != 0),
    CONSTRAINT chk_balance_after_non_negative CHECK (balance_after_cents >= 0)
);

CREATE INDEX idx_wallet_ledger_user_id ON wallet_ledger(user_id);
CREATE INDEX idx_wallet_ledger_type ON wallet_ledger(type);
CREATE INDEX idx_wallet_ledger_created_at ON wallet_ledger(created_at);
CREATE INDEX idx_wallet_ledger_ref ON wallet_ledger(ref_type, ref_id);
CREATE INDEX idx_wallet_ledger_user_type ON wallet_ledger(user_id, type);
CREATE INDEX idx_wallet_ledger_user_created ON wallet_ledger(user_id, created_at DESC);

-- ============================================================================
-- PAYMENT INTENT STATUS ENUM
-- ============================================================================

CREATE TYPE payment_intent_status AS ENUM (
    'pending',           -- Payment initiated but not confirmed
    'processing',        -- Payment is being processed
    'succeeded',         -- Payment completed successfully
    'failed',            -- Payment failed
    'cancelled',         -- Payment cancelled by user
    'refunded'           -- Payment was refunded
);

-- ============================================================================
-- PAYMENT INTENTS TABLE (for deposits)
-- ============================================================================

CREATE TABLE payment_intents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    provider_payment_id VARCHAR(255),
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status payment_intent_status NOT NULL DEFAULT 'pending',
    metadata_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    CONSTRAINT chk_payment_amount_positive CHECK (amount_cents > 0)
);

CREATE INDEX idx_payment_intents_user_id ON payment_intents(user_id);
CREATE INDEX idx_payment_intents_status ON payment_intents(status);
CREATE INDEX idx_payment_intents_provider ON payment_intents(provider);
CREATE INDEX idx_payment_intents_provider_payment_id ON payment_intents(provider, provider_payment_id);
CREATE INDEX idx_payment_intents_created_at ON payment_intents(created_at);
CREATE INDEX idx_payment_intents_user_status ON payment_intents(user_id, status);

-- ============================================================================
-- PAYOUT STATUS ENUM
-- ============================================================================

CREATE TYPE payout_status AS ENUM (
    'pending',           -- Payout requested but not processed
    'processing',        -- Payout is being processed
    'succeeded',         -- Payout completed successfully
    'failed',            -- Payout failed
    'cancelled'          -- Payout cancelled
);

-- ============================================================================
-- PAYOUTS TABLE (for withdrawals)
-- ============================================================================

CREATE TABLE payouts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status payout_status NOT NULL DEFAULT 'pending',
    provider VARCHAR(50),
    provider_payout_id VARCHAR(255),
    destination_type VARCHAR(50),
    destination_info_json JSONB,
    metadata_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    CONSTRAINT chk_payout_amount_positive CHECK (amount_cents > 0)
);

CREATE INDEX idx_payouts_user_id ON payouts(user_id);
CREATE INDEX idx_payouts_status ON payouts(status);
CREATE INDEX idx_payouts_created_at ON payouts(created_at);
CREATE INDEX idx_payouts_user_status ON payouts(user_id, status);

-- ============================================================================
-- TRIGGERS FOR UPDATED_AT
-- ============================================================================

CREATE TRIGGER set_wallets_updated_at
    BEFORE UPDATE ON wallets
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

CREATE TRIGGER set_payment_intents_updated_at
    BEFORE UPDATE ON payment_intents
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

CREATE TRIGGER set_payouts_updated_at
    BEFORE UPDATE ON payouts
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================================
-- AUTO-CREATE WALLET FOR NEW USERS (TRIGGER)
-- ============================================================================

CREATE OR REPLACE FUNCTION trigger_create_wallet_for_user()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO wallets (user_id) VALUES (NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER create_wallet_on_user_insert
    AFTER INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION trigger_create_wallet_for_user();

-- ============================================================================
-- CREATE WALLETS FOR EXISTING USERS
-- ============================================================================

INSERT INTO wallets (user_id)
SELECT id FROM users
WHERE id NOT IN (SELECT user_id FROM wallets);
