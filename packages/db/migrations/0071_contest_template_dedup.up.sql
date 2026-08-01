-- 0071_contest_template_dedup.up.sql
-- Add unique partial index for deduplication on template_id + starts_at.
-- This is a safety net for the calendar processor which creates contests
-- from tournament_templates (using template_id, not schedule_id).
-- Migration 0067 already covers schedule_id-based dedup.

CREATE UNIQUE INDEX IF NOT EXISTS idx_contests_template_start_dedup
    ON contests(template_id, starts_at)
    WHERE template_id IS NOT NULL;
