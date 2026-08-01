-- 0061_tournament_templates_schema.up.sql
-- Extend tournament_templates with market_type, duration_type, entry_fee (Rials),
-- has_prize, is_active, and updated_at columns for reusable tournament configurations.

-- ============================================================================
-- NEW ENUM TYPES
-- ============================================================================

CREATE TYPE market_type AS ENUM ('crypto', 'forex');

CREATE TYPE template_duration_type AS ENUM (
    'quick_30m',
    'free_1h',
    'four_hour',
    'daily',
    'weekly',
    'special'
);

-- ============================================================================
-- ADD COLUMNS TO EXISTING TOURNAMENT_TEMPLATES TABLE
-- ============================================================================

-- Market type for the template (narrower than asset_class: crypto or forex only)
ALTER TABLE tournament_templates ADD COLUMN IF NOT EXISTS market_type market_type;

-- Duration type categorization for templates
ALTER TABLE tournament_templates ADD COLUMN IF NOT EXISTS template_duration_type template_duration_type;

-- Entry fee in Rials (0 for free tournaments)
ALTER TABLE tournament_templates ADD COLUMN IF NOT EXISTS entry_fee BIGINT NOT NULL DEFAULT 0;

-- Whether this template awards prizes
ALTER TABLE tournament_templates ADD COLUMN IF NOT EXISTS has_prize BOOLEAN NOT NULL DEFAULT TRUE;

-- Whether this template is active and available for scheduling
ALTER TABLE tournament_templates ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;

-- Track when template was last updated
ALTER TABLE tournament_templates ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- ============================================================================
-- UPDATE COMMISSION_RATE DEFAULT
-- ============================================================================

-- Change default commission rate from 17.00 to 0.20 (20%) for new templates
ALTER TABLE tournament_templates ALTER COLUMN commission_rate SET DEFAULT 0.20;

-- ============================================================================
-- BACKFILL EXISTING DATA
-- ============================================================================

-- Backfill market_type from existing asset_class column
UPDATE tournament_templates
SET market_type = CASE
    WHEN asset_class = 'crypto' THEN 'crypto'::market_type
    WHEN asset_class = 'forex' THEN 'forex'::market_type
    ELSE 'crypto'::market_type  -- default for stocks/mixed
END
WHERE market_type IS NULL;

-- Backfill template_duration_type from existing duration_minutes
UPDATE tournament_templates
SET template_duration_type = CASE
    WHEN duration_minutes <= 30 THEN 'quick_30m'::template_duration_type
    WHEN duration_minutes <= 60 THEN 'free_1h'::template_duration_type
    WHEN duration_minutes <= 240 THEN 'four_hour'::template_duration_type
    WHEN duration_minutes <= 1440 THEN 'daily'::template_duration_type
    WHEN duration_minutes <= 10080 THEN 'weekly'::template_duration_type
    ELSE 'special'::template_duration_type
END
WHERE template_duration_type IS NULL;

-- Backfill entry_fee from existing entry_fee_cents
UPDATE tournament_templates
SET entry_fee = entry_fee_cents
WHERE entry_fee = 0 AND entry_fee_cents > 0;

-- ============================================================================
-- CONSTRAINTS
-- ============================================================================

ALTER TABLE tournament_templates ADD CONSTRAINT chk_template_entry_fee_non_negative
    CHECK (entry_fee >= 0);

-- ============================================================================
-- INDEXES
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_tournament_templates_market_type
    ON tournament_templates(market_type);

CREATE INDEX IF NOT EXISTS idx_tournament_templates_template_duration_type
    ON tournament_templates(template_duration_type);

CREATE INDEX IF NOT EXISTS idx_tournament_templates_is_active
    ON tournament_templates(is_active)
    WHERE is_active = TRUE;

-- ============================================================================
-- UPDATED_AT TRIGGER
-- ============================================================================

CREATE TRIGGER set_tournament_templates_updated_at
    BEFORE UPDATE ON tournament_templates
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON COLUMN tournament_templates.market_type IS 'Target market type: crypto or forex';
COMMENT ON COLUMN tournament_templates.template_duration_type IS 'Duration category: quick_30m, free_1h, four_hour, daily, weekly, special';
COMMENT ON COLUMN tournament_templates.entry_fee IS 'Entry fee in Rials (0 for free tournaments)';
COMMENT ON COLUMN tournament_templates.has_prize IS 'Whether this template awards prizes to winners';
COMMENT ON COLUMN tournament_templates.is_active IS 'Whether this template is active and available for scheduling';
