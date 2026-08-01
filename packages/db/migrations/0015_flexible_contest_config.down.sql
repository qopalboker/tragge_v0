-- 0015_flexible_contest_config.down.sql
-- Rollback flexible contest configuration

-- ============================================================================
-- DELETE CONTEST TEMPLATES
-- ============================================================================

DELETE FROM tournament_templates WHERE template_key IN (
    'crypto_rush_30m', 'crypto_hourly', 'crypto_daily',
    'forex_rush_30m', 'forex_hourly', 'forex_4hour', 'forex_daily', 'forex_weekly',
    'stocks_daily',
    'crypto_free_practice', 'forex_free_practice',
    'crypto_high_stakes', 'forex_high_stakes'
);

-- ============================================================================
-- DROP INDEXES
-- ============================================================================

DROP INDEX IF EXISTS idx_contests_template_id;
DROP INDEX IF EXISTS idx_tournament_templates_key;
DROP INDEX IF EXISTS idx_contests_registration_deadline;
DROP INDEX IF EXISTS idx_contests_status_asset_class;
DROP INDEX IF EXISTS idx_contests_asset_class;

-- ============================================================================
-- DROP TOURNAMENT TEMPLATE CONSTRAINTS AND COLUMNS
-- ============================================================================

ALTER TABLE tournament_templates DROP CONSTRAINT IF EXISTS uq_tournament_templates_key;
ALTER TABLE tournament_templates DROP CONSTRAINT IF EXISTS chk_template_commission_rate_valid;
ALTER TABLE tournament_templates DROP CONSTRAINT IF EXISTS chk_template_min_participants_positive;

ALTER TABLE tournament_templates DROP COLUMN IF EXISTS template_key;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS auto_start;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS min_participants;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS commission_rate;
ALTER TABLE tournament_templates DROP COLUMN IF EXISTS asset_class;

-- ============================================================================
-- DROP CONTEST CONSTRAINTS AND COLUMNS
-- ============================================================================

ALTER TABLE contests DROP CONSTRAINT IF EXISTS chk_registration_deadline_valid;
ALTER TABLE contests DROP CONSTRAINT IF EXISTS chk_commission_rate_valid;
ALTER TABLE contests DROP CONSTRAINT IF EXISTS chk_duration_minutes_positive;
ALTER TABLE contests DROP CONSTRAINT IF EXISTS chk_min_participants_positive;

ALTER TABLE contests DROP COLUMN IF EXISTS commission_rate;
ALTER TABLE contests DROP COLUMN IF EXISTS template_id;
ALTER TABLE contests DROP COLUMN IF EXISTS auto_start;
ALTER TABLE contests DROP COLUMN IF EXISTS registration_deadline;
ALTER TABLE contests DROP COLUMN IF EXISTS min_participants;
ALTER TABLE contests DROP COLUMN IF EXISTS duration_minutes;
ALTER TABLE contests DROP COLUMN IF EXISTS asset_class;

-- ============================================================================
-- DROP ASSET CLASS ENUM
-- ============================================================================

DROP TYPE IF EXISTS asset_class;
