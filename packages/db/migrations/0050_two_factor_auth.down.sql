-- Rollback P2-P3-4: Two-Factor Authentication (2FA) support

ALTER TABLE users DROP COLUMN IF EXISTS backup_codes;
ALTER TABLE users DROP COLUMN IF EXISTS totp_verified_at;
ALTER TABLE users DROP COLUMN IF EXISTS totp_enabled;
ALTER TABLE users DROP COLUMN IF EXISTS totp_secret;
