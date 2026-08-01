-- Drop email verification tokens table
DROP TABLE IF EXISTS email_verification_tokens;

-- Remove email verification columns from users table
ALTER TABLE users DROP COLUMN IF EXISTS email_verified_at;
ALTER TABLE users DROP COLUMN IF EXISTS email_verified;
