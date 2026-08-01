-- 0028_email_templates.down.sql
-- Rollback email template overrides

-- Drop indexes
DROP INDEX IF EXISTS idx_email_templates_updated_at;

-- Drop table
DROP TABLE IF EXISTS email_templates;
