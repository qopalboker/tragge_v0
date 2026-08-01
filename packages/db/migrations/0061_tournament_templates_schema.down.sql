-- 0061_tournament_templates_schema.down.sql
-- Revert tournament_templates schema extensions

-- ============================================================================
-- DROP TRIGGER
-- ============================================================================

DROP TRIGGER IF EXISTS set_tournament_templates_updated_at ON tournament_templates;

-- ============================================================================
-- DROP INDEXES
-- ============================================================================

DROP INDEX IF EXISTS idx_tournament_templates_is_active;
DROP INDEX IF EXISTS idx_tournament_templates_template_duration_type;
DROP INDEX IF EXISTS idx_tournament_templates_market_type;

-- ============================================================================
-- DROP CONSTRAINTS
-- ============================================================================

ALTER TABLE tournament_templates DROP CONSTRAINT IF EXISTS chk_template_entry_fee_non_negative;

-- ============================================================================
-- REVERT COMMISSION_RATE DEFAULT
-- ============================================================================

ALTER TABLE tournament_templates ALTER COLUMN commission_rate SET DEFAULT 17.00;

-- ============================================================================
-- DROP COLUMNS
-- ============================================================================

ALTER TABLE tournament_templates DROP COLUMN IF EXISTS updated_at;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS is_active;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS has_prize;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS entry_fee;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS template_duration_type;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS market_type;

-- ============================================================================
-- DROP ENUM TYPES
-- ============================================================================

DROP TYPE IF EXISTS template_duration_type;
DROP TYPE IF EXISTS market_type;
