DROP INDEX IF EXISTS idx_users_ban_expires_at;
ALTER TABLE users DROP COLUMN IF EXISTS ban_expires_at;
