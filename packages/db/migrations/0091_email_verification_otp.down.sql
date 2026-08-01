-- Remove failed_attempts column from email_verification_tokens
ALTER TABLE email_verification_tokens
    DROP COLUMN IF EXISTS failed_attempts;
