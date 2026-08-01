-- 0063_contests_template_fields.down.sql
-- Revert contests table template/schedule additions

-- ============================================================================
-- DROP INDEXES
-- ============================================================================

DROP INDEX IF EXISTS idx_contests_schedule_id;

-- ============================================================================
-- DROP NEW COLUMNS
-- ============================================================================

ALTER TABLE contests DROP COLUMN IF EXISTS market_close_time;
ALTER TABLE contests DROP COLUMN IF EXISTS commission_amount;
ALTER TABLE contests DROP COLUMN IF EXISTS schedule_id;

-- ============================================================================
-- DROP FK ON TEMPLATE_ID (keep the column, it was added in migration 0015)
-- ============================================================================

ALTER TABLE contests DROP CONSTRAINT IF EXISTS fk_contests_template_id;

-- ============================================================================
-- RESTORE PARTICIPANT CAP CONSTRAINT
-- ============================================================================

ALTER TABLE contests ADD CONSTRAINT chk_current_participants_lte_max
    CHECK (max_participants IS NULL OR current_participants <= max_participants);
