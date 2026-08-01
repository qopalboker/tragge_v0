-- Add unique constraint for phone numbers (only non-null values)
-- Note: CONCURRENTLY removed because golang-migrate runs migrations inside transactions.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone ON users(phone) WHERE phone IS NOT NULL;
