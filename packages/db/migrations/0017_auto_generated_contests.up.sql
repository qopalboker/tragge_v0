-- 0017_auto_generated_contests.up.sql
-- Add auto_generated flag to track contests created by the free-contest-generator service

-- Add auto_generated flag to contests table
ALTER TABLE contests ADD COLUMN IF NOT EXISTS auto_generated BOOLEAN NOT NULL DEFAULT FALSE;

-- Add template_id reference for contests created from templates
ALTER TABLE contests ADD COLUMN IF NOT EXISTS template_id UUID REFERENCES tournament_templates(id);

-- Index for querying auto-generated contests
CREATE INDEX IF NOT EXISTS idx_contests_auto_generated ON contests(auto_generated)
    WHERE auto_generated = TRUE;

-- Composite index for cleanup of old auto-generated free contests
CREATE INDEX IF NOT EXISTS idx_contests_auto_generated_cleanup ON contests(auto_generated, is_free, status, ended_at)
    WHERE auto_generated = TRUE AND is_free = TRUE;
