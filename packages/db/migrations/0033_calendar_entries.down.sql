-- 0033_calendar_entries.down.sql
-- Remove calendar entries for scheduled tournament creation

-- Drop trigger first
DROP TRIGGER IF EXISTS trg_calendar_entries_updated_at ON calendar_entries;

-- Drop function
DROP FUNCTION IF EXISTS update_calendar_entries_updated_at();

-- Drop history table first (has foreign key to calendar_entries)
DROP TABLE IF EXISTS calendar_contest_history;

-- Drop main table
DROP TABLE IF EXISTS calendar_entries;

-- Drop enums (only if not used elsewhere)
DROP TYPE IF EXISTS calendar_status;
DROP TYPE IF EXISTS recurrence_pattern;
