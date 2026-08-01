-- Rollback migration 0068: Tournament state machine enhancements

DROP TABLE IF EXISTS tournaments_archive;
DROP INDEX IF EXISTS idx_contests_registration_opens_at;
ALTER TABLE contests DROP CONSTRAINT IF EXISTS chk_registration_opens_at_valid;
ALTER TABLE contests DROP COLUMN IF EXISTS registration_opens_at;
