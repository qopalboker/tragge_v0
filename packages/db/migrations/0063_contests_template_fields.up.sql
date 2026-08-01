-- 0063_contests_template_fields.up.sql
-- Add template/schedule foreign keys, commission tracking, and market close time
-- to the contests table. Remove participant cap constraint for unlimited participation.

-- ============================================================================
-- FOREIGN KEY ON EXISTING TEMPLATE_ID COLUMN
-- ============================================================================

-- template_id column was added in migration 0015 but without a FK constraint.
-- Add the FK now to enforce referential integrity.
ALTER TABLE contests ADD CONSTRAINT fk_contests_template_id
    FOREIGN KEY (template_id) REFERENCES tournament_templates(id) ON DELETE SET NULL;

-- ============================================================================
-- NEW COLUMNS
-- ============================================================================

-- Reference to the schedule that created this contest (null for manual creation)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS schedule_id UUID
    REFERENCES tournament_schedules(id) ON DELETE SET NULL;

-- Actual commission amount collected in Rials (entry_fees * commission_rate)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS commission_amount BIGINT NOT NULL DEFAULT 0;

-- For forex tournaments that end before daily market reset
ALTER TABLE contests ADD COLUMN IF NOT EXISTS market_close_time TIMESTAMPTZ;

-- ============================================================================
-- REMOVE PARTICIPANT CAP
-- ============================================================================

-- Drop the constraint that limits current_participants to max_participants.
-- Tournaments should have NO participant limit.
ALTER TABLE contests DROP CONSTRAINT IF EXISTS chk_current_participants_lte_max;

-- ============================================================================
-- INDEXES
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_contests_schedule_id
    ON contests(schedule_id)
    WHERE schedule_id IS NOT NULL;

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON COLUMN contests.schedule_id IS 'FK to tournament_schedules — null for manually created contests';
COMMENT ON COLUMN contests.commission_amount IS 'Actual commission collected in Rials (sum of entry fees * commission rate)';
COMMENT ON COLUMN contests.market_close_time IS 'For forex tournaments that end before daily market reset';
COMMENT ON COLUMN contests.template_id IS 'FK to tournament_templates — null for manually created contests';
COMMENT ON COLUMN contests.current_participants IS 'Denormalized participant count (no upper limit, maintained by trigger)';
COMMENT ON COLUMN contests.prize_pool_net_cents IS 'Dynamically calculated prize pool (sum of entry fees minus commission)';
