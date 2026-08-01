-- 0019_settlement_tables.up.sql
-- Settlement tracking tables for contest completion

-- ============================================================================
-- SETTLEMENT STATUS ENUM
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE settlement_status AS ENUM (
        'pending',      -- Settlement queued
        'in_progress',  -- Settlement actively running
        'completed',    -- Settlement finished successfully
        'failed',       -- Settlement failed
        'partial'       -- Settlement completed with some failures
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE prize_status AS ENUM (
        'pending',      -- Prize calculated but not credited
        'credited',     -- Prize credited to wallet
        'failed'        -- Prize credit failed
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- ============================================================================
-- CONTEST SETTLEMENTS
-- ============================================================================

-- Tracks the settlement process for each contest
CREATE TABLE IF NOT EXISTS contest_settlements (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    contest_id UUID NOT NULL REFERENCES contests(id),
    status settlement_status NOT NULL DEFAULT 'pending',

    -- Timing
    started_at TIMESTAMPTZ,
    positions_closed_at TIMESTAMPTZ,
    rankings_calculated_at TIMESTAMPTZ,
    prizes_distributed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    -- Statistics
    total_participants INT NOT NULL DEFAULT 0,
    total_positions_closed INT NOT NULL DEFAULT 0,
    total_orders_cancelled INT NOT NULL DEFAULT 0,
    total_winners INT NOT NULL DEFAULT 0,

    -- Prize pool
    prize_pool_gross_cents BIGINT NOT NULL DEFAULT 0,
    prize_pool_net_cents BIGINT NOT NULL DEFAULT 0,
    total_distributed_cents BIGINT NOT NULL DEFAULT 0,
    platform_fee_cents BIGINT NOT NULL DEFAULT 0,

    -- Error tracking
    attempt_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    failed_at TIMESTAMPTZ,

    -- Snapshot prices used for position closing (JSON object: symbol -> price)
    snapshot_prices JSONB,
    snapshot_taken_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_contest_settlements_contest_id UNIQUE (contest_id)
);

CREATE INDEX idx_contest_settlements_contest_id ON contest_settlements(contest_id);
CREATE INDEX idx_contest_settlements_status ON contest_settlements(status);
CREATE INDEX idx_contest_settlements_created_at ON contest_settlements(created_at);

-- ============================================================================
-- PRIZE DISTRIBUTIONS
-- ============================================================================

-- Records individual prize distributions
CREATE TABLE IF NOT EXISTS prize_distributions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    settlement_id UUID NOT NULL REFERENCES contest_settlements(id) ON DELETE CASCADE,
    contest_id UUID NOT NULL REFERENCES contests(id),
    user_id UUID NOT NULL REFERENCES users(id),

    -- Ranking info
    rank INT NOT NULL,
    final_score NUMERIC(20, 4) NOT NULL,

    -- Prize info
    prize_amount_cents BIGINT NOT NULL,
    prize_percentage NUMERIC(10, 6) NOT NULL, -- Percentage of prize pool

    -- Status tracking
    status prize_status NOT NULL DEFAULT 'pending',
    credited_at TIMESTAMPTZ,
    error_message TEXT,

    -- Wallet ledger reference
    ledger_entry_id UUID,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_prize_distributions_contest_user UNIQUE (contest_id, user_id)
);

CREATE INDEX idx_prize_distributions_settlement_id ON prize_distributions(settlement_id);
CREATE INDEX idx_prize_distributions_contest_id ON prize_distributions(contest_id);
CREATE INDEX idx_prize_distributions_user_id ON prize_distributions(user_id);
CREATE INDEX idx_prize_distributions_rank ON prize_distributions(contest_id, rank);
CREATE INDEX idx_prize_distributions_status ON prize_distributions(status);

-- ============================================================================
-- FINAL RANKINGS
-- ============================================================================

-- Stores final rankings for all participants (winners and non-winners)
CREATE TABLE IF NOT EXISTS final_rankings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    settlement_id UUID NOT NULL REFERENCES contest_settlements(id) ON DELETE CASCADE,
    contest_id UUID NOT NULL REFERENCES contests(id),
    user_id UUID NOT NULL REFERENCES users(id),

    -- Ranking
    rank INT NOT NULL,
    tied_with_count INT NOT NULL DEFAULT 0, -- Number of users with same rank

    -- Score breakdown
    final_score NUMERIC(20, 4) NOT NULL,
    realized_score NUMERIC(20, 4) NOT NULL DEFAULT 0,
    unrealized_score NUMERIC(20, 4) NOT NULL DEFAULT 0,

    -- Trading stats
    total_trades INT NOT NULL DEFAULT 0,
    winning_trades INT NOT NULL DEFAULT 0,
    total_positions INT NOT NULL DEFAULT 0,

    -- Tralent Score contribution
    tralent_score_contribution NUMERIC(20, 4) NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_final_rankings_contest_user UNIQUE (contest_id, user_id)
);

CREATE INDEX idx_final_rankings_settlement_id ON final_rankings(settlement_id);
CREATE INDEX idx_final_rankings_contest_id ON final_rankings(contest_id);
CREATE INDEX idx_final_rankings_user_id ON final_rankings(user_id);
CREATE INDEX idx_final_rankings_rank ON final_rankings(contest_id, rank);

-- ============================================================================
-- SETTLEMENT EVENTS LOG
-- ============================================================================

-- Audit log for settlement process
CREATE TABLE IF NOT EXISTS settlement_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    settlement_id UUID NOT NULL REFERENCES contest_settlements(id) ON DELETE CASCADE,
    contest_id UUID NOT NULL REFERENCES contests(id),

    event_type VARCHAR(50) NOT NULL, -- started, positions_closed, rankings_calculated, prizes_distributed, completed, failed, retrying
    event_data JSONB,
    error_message TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_settlement_events_settlement_id ON settlement_events(settlement_id);
CREATE INDEX idx_settlement_events_contest_id ON settlement_events(contest_id);
CREATE INDEX idx_settlement_events_event_type ON settlement_events(event_type);
CREATE INDEX idx_settlement_events_created_at ON settlement_events(created_at);

-- ============================================================================
-- TRIGGER FOR UPDATED_AT
-- ============================================================================

CREATE TRIGGER set_contest_settlements_updated_at
    BEFORE UPDATE ON contest_settlements
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================================
-- ADD SETTLING STATUS TO CONTEST STATUS ENUM (if not exists)
-- ============================================================================

-- Note: PostgreSQL doesn't allow easy enum modification
-- The 'settling' status should already exist from migration 0016
-- If not, it can be added manually:
-- ALTER TYPE contest_status ADD VALUE IF NOT EXISTS 'settling' BEFORE 'completed';
