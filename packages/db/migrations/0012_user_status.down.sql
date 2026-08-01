-- 0012_user_status.down.sql
-- Remove status column from users table

-- Drop indexes
DROP INDEX IF EXISTS idx_users_email_status;
DROP INDEX IF EXISTS idx_users_status;

-- Remove column
ALTER TABLE users DROP COLUMN IF EXISTS status;

-- Drop enum type
DROP TYPE IF EXISTS user_status;
