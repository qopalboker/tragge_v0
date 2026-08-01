-- P0-5: Add password_changed_at column for DB-backed session invalidation
-- This provides a fallback when Redis is unavailable for session deletion.
-- JWT tokens issued before password_changed_at are rejected by middleware.
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_changed_at TIMESTAMPTZ;
