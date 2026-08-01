-- 0052_withdrawal_limits.up.sql
-- Add withdrawal limits for AML compliance
-- Default limits enforced by application config; per-user overrides stored here.

-- ============================================================================
-- WITHDRAWAL LIMITS TABLE (per-user overrides only)
-- ============================================================================

CREATE TABLE withdrawal_limits (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    daily_amount_cents   BIGINT,   -- NULL = use system default
    monthly_amount_cents BIGINT,   -- NULL = use system default
    daily_count          INT,      -- NULL = use system default
    monthly_count        INT,      -- NULL = use system default
    notes                TEXT,     -- Admin notes explaining the override
    updated_by           UUID REFERENCES users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_wl_daily_amount_positive   CHECK (daily_amount_cents IS NULL OR daily_amount_cents > 0),
    CONSTRAINT chk_wl_monthly_amount_positive CHECK (monthly_amount_cents IS NULL OR monthly_amount_cents > 0),
    CONSTRAINT chk_wl_daily_count_positive    CHECK (daily_count IS NULL OR daily_count > 0),
    CONSTRAINT chk_wl_monthly_count_positive  CHECK (monthly_count IS NULL OR monthly_count > 0),
    CONSTRAINT chk_wl_monthly_gte_daily_amount CHECK (
        monthly_amount_cents IS NULL OR daily_amount_cents IS NULL
        OR monthly_amount_cents >= daily_amount_cents
    ),
    CONSTRAINT chk_wl_monthly_gte_daily_count CHECK (
        monthly_count IS NULL OR daily_count IS NULL
        OR monthly_count >= daily_count
    )
);

-- Trigger for updated_at
CREATE TRIGGER set_withdrawal_limits_updated_at
    BEFORE UPDATE ON withdrawal_limits
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================================
-- INDEX ON PAYOUTS FOR EFFICIENT DAILY/MONTHLY AGGREGATION
-- ============================================================================

-- Composite index: user_id + created_at + status for time-windowed aggregation
-- Covers: WHERE user_id = $1 AND created_at >= $2 AND status NOT IN ('cancelled','failed')
CREATE INDEX IF NOT EXISTS idx_payouts_user_created_status
    ON payouts(user_id, created_at DESC, status);
