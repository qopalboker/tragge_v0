-- Add index to support efficient cleanup of expired verification codes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_verification_codes_cleanup
    ON verification_codes(expires_at)
    WHERE verified_at IS NULL;
