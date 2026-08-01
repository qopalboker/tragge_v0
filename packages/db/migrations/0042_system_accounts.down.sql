-- 0042_system_accounts.down.sql
-- Reverse the system account migration.

-- 1. Remove system participant entries and drop the is_system column
DROP INDEX IF EXISTS idx_contest_participants_system;
ALTER TABLE contest_participants DROP COLUMN IF EXISTS is_system;

-- 2. Delete the Tragge Trader system account (cascades to related rows)
DELETE FROM users WHERE id = '00000000-0000-0000-0000-000000000001';

-- 3. Remove the is_system_account column from users
DROP INDEX IF EXISTS idx_users_system_account;
ALTER TABLE users DROP COLUMN IF EXISTS is_system_account;
