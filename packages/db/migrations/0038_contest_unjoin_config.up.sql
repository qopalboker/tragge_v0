-- Add allow_unjoin configuration to contests
ALTER TABLE contests ADD COLUMN IF NOT EXISTS allow_unjoin BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE contests ADD COLUMN IF NOT EXISTS unjoin_deadline_minutes INTEGER DEFAULT NULL;
