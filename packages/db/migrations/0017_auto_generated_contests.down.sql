-- 0017_auto_generated_contests.down.sql
-- Remove auto_generated flag and template_id from contests

-- Drop indexes first
DROP INDEX IF EXISTS idx_contests_auto_generated_cleanup;
DROP INDEX IF EXISTS idx_contests_auto_generated;

-- Remove columns
ALTER TABLE contests DROP COLUMN IF EXISTS template_id;
ALTER TABLE contests DROP COLUMN IF EXISTS auto_generated;
