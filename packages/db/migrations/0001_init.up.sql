-- 0001_init.up.sql
-- Initial schema for the Trading Tournament Platform

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================================
-- USERS & AUTHENTICATION
-- ============================================================================

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_created_at ON users(created_at);

CREATE TABLE roles (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE
);

-- Insert default roles
INSERT INTO roles (name) VALUES ('user'), ('admin'), ('moderator');

CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id INT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX idx_user_roles_role_id ON user_roles(role_id);

-- ============================================================================
-- CONTESTS
-- ============================================================================

CREATE TYPE contest_status AS ENUM (
    'draft',
    'scheduled',
    'registration_open',
    'running',
    'paused',
    'completed',
    'cancelled'
);

CREATE TABLE contests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    status contest_status NOT NULL DEFAULT 'draft',
    entry_fee_cents INT NOT NULL DEFAULT 0,
    platform_fee_bps INT NOT NULL DEFAULT 0, -- basis points (e.g., 250 = 2.5%)
    qty_total BIGINT NOT NULL DEFAULT 100000, -- starting virtual currency/quantity
    rules_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_contest_dates CHECK (ends_at > starts_at),
    CONSTRAINT chk_entry_fee_positive CHECK (entry_fee_cents >= 0),
    CONSTRAINT chk_platform_fee_valid CHECK (platform_fee_bps >= 0 AND platform_fee_bps <= 10000)
);

CREATE INDEX idx_contests_status ON contests(status);
CREATE INDEX idx_contests_starts_at ON contests(starts_at);
CREATE INDEX idx_contests_ends_at ON contests(ends_at);
CREATE INDEX idx_contests_created_at ON contests(created_at);

-- ============================================================================
-- CONTEST SYMBOLS
-- ============================================================================

CREATE TABLE contest_symbols (
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    symbol VARCHAR(20) NOT NULL,
    provider_symbol_twelvedata VARCHAR(50),
    provider_symbol_finnhub VARCHAR(50),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (contest_id, symbol)
);

CREATE INDEX idx_contest_symbols_contest_id ON contest_symbols(contest_id);
CREATE INDEX idx_contest_symbols_symbol ON contest_symbols(symbol);
CREATE INDEX idx_contest_symbols_enabled ON contest_symbols(contest_id, enabled);

-- ============================================================================
-- CONTEST PARTICIPANTS
-- ============================================================================

CREATE TABLE contest_participants (
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    qty_total BIGINT NOT NULL, -- total virtual currency allocated
    qty_available BIGINT NOT NULL, -- available for trading
    total_score NUMERIC(20, 4) NOT NULL DEFAULT 0, -- portfolio value/score
    final_rank INT,
    final_prize_cents INT,
    PRIMARY KEY (contest_id, user_id),

    CONSTRAINT chk_qty_available CHECK (qty_available >= 0),
    CONSTRAINT chk_qty_total CHECK (qty_total >= 0),
    CONSTRAINT chk_qty_available_lte_total CHECK (qty_available <= qty_total)
);

CREATE INDEX idx_contest_participants_contest_id ON contest_participants(contest_id);
CREATE INDEX idx_contest_participants_user_id ON contest_participants(user_id);
CREATE INDEX idx_contest_participants_joined_at ON contest_participants(joined_at);
CREATE INDEX idx_contest_participants_total_score ON contest_participants(contest_id, total_score DESC);
CREATE INDEX idx_contest_participants_final_rank ON contest_participants(contest_id, final_rank);

-- ============================================================================
-- ORDERS
-- ============================================================================

CREATE TYPE order_side AS ENUM ('buy', 'sell');
CREATE TYPE order_type AS ENUM ('market', 'limit', 'stop', 'stop_limit');
CREATE TYPE order_status AS ENUM (
    'pending',
    'open',
    'partially_filled',
    'filled',
    'cancelled',
    'rejected',
    'expired'
);

CREATE TABLE orders (
    order_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol VARCHAR(20) NOT NULL,
    side order_side NOT NULL,
    type order_type NOT NULL,
    qty BIGINT NOT NULL,
    qty_filled BIGINT NOT NULL DEFAULT 0,
    limit_price NUMERIC(20, 8),
    stop_price NUMERIC(20, 8),
    take_profit NUMERIC(20, 8),
    stop_loss NUMERIC(20, 8),
    status order_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_order_qty_positive CHECK (qty > 0),
    CONSTRAINT chk_order_qty_filled CHECK (qty_filled >= 0 AND qty_filled <= qty),
    CONSTRAINT chk_limit_price_for_limit CHECK (
        (type NOT IN ('limit', 'stop_limit')) OR (limit_price IS NOT NULL AND limit_price > 0)
    ),
    CONSTRAINT chk_stop_price_for_stop CHECK (
        (type NOT IN ('stop', 'stop_limit')) OR (stop_price IS NOT NULL AND stop_price > 0)
    )
);

CREATE INDEX idx_orders_contest_id ON orders(contest_id);
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_symbol ON orders(symbol);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_created_at ON orders(created_at);
CREATE INDEX idx_orders_contest_user ON orders(contest_id, user_id);
CREATE INDEX idx_orders_contest_user_status ON orders(contest_id, user_id, status);
CREATE INDEX idx_orders_contest_symbol ON orders(contest_id, symbol);

-- ============================================================================
-- FILLS
-- ============================================================================

CREATE TABLE fills (
    fill_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id UUID NOT NULL REFERENCES orders(order_id) ON DELETE CASCADE,
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol VARCHAR(20) NOT NULL,
    side order_side NOT NULL,
    qty BIGINT NOT NULL,
    fill_price NUMERIC(20, 8) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_fill_qty_positive CHECK (qty > 0),
    CONSTRAINT chk_fill_price_positive CHECK (fill_price > 0)
);

CREATE INDEX idx_fills_order_id ON fills(order_id);
CREATE INDEX idx_fills_contest_id ON fills(contest_id);
CREATE INDEX idx_fills_user_id ON fills(user_id);
CREATE INDEX idx_fills_symbol ON fills(symbol);
CREATE INDEX idx_fills_created_at ON fills(created_at);
CREATE INDEX idx_fills_contest_user ON fills(contest_id, user_id);
CREATE INDEX idx_fills_contest_symbol ON fills(contest_id, symbol);

-- ============================================================================
-- POSITIONS
-- ============================================================================

CREATE TYPE position_side AS ENUM ('long', 'short');

CREATE TABLE positions (
    position_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol VARCHAR(20) NOT NULL,
    side position_side NOT NULL,
    qty_open BIGINT NOT NULL DEFAULT 0,
    entry_price NUMERIC(20, 8) NOT NULL,
    qty_used BIGINT NOT NULL DEFAULT 0, -- buying power used
    realized_score NUMERIC(20, 4) NOT NULL DEFAULT 0, -- realized P&L
    opened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ,

    CONSTRAINT chk_position_qty_open CHECK (qty_open >= 0),
    CONSTRAINT chk_position_entry_price CHECK (entry_price > 0),
    CONSTRAINT chk_position_qty_used CHECK (qty_used >= 0)
);

CREATE INDEX idx_positions_contest_id ON positions(contest_id);
CREATE INDEX idx_positions_user_id ON positions(user_id);
CREATE INDEX idx_positions_symbol ON positions(symbol);
CREATE INDEX idx_positions_opened_at ON positions(opened_at);
CREATE INDEX idx_positions_closed_at ON positions(closed_at);
CREATE INDEX idx_positions_contest_user ON positions(contest_id, user_id);
CREATE INDEX idx_positions_contest_user_symbol ON positions(contest_id, user_id, symbol);
CREATE INDEX idx_positions_open ON positions(contest_id, user_id) WHERE closed_at IS NULL;

-- ============================================================================
-- LEADERBOARD SNAPSHOTS
-- ============================================================================

CREATE TABLE leaderboard_snapshots (
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    taken_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    payload_json JSONB NOT NULL,
    PRIMARY KEY (contest_id, taken_at)
);

CREATE INDEX idx_leaderboard_snapshots_contest_id ON leaderboard_snapshots(contest_id);
CREATE INDEX idx_leaderboard_snapshots_taken_at ON leaderboard_snapshots(taken_at);

-- ============================================================================
-- AUDIT LOGS
-- ============================================================================

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(100) NOT NULL,
    target_type VARCHAR(50) NOT NULL,
    target_id UUID,
    payload_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_actor_user_id ON audit_logs(actor_user_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_target_type ON audit_logs(target_type);
CREATE INDEX idx_audit_logs_target_id ON audit_logs(target_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);
CREATE INDEX idx_audit_logs_target ON audit_logs(target_type, target_id);

-- ============================================================================
-- TRIGGERS FOR UPDATED_AT
-- ============================================================================

CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER set_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();
