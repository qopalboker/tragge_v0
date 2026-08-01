-- 0046_contest_commission_rate_default.up.sql
-- Change default commission_rate from 17.00 to 20.00

ALTER TABLE contests ALTER COLUMN commission_rate SET DEFAULT 20.00;

-- Update existing contests that still use the old default (17.00)
-- to the new default (20.00) only if they were never explicitly configured
UPDATE contests
SET commission_rate = 20.00
WHERE commission_rate = 17.00;

ALTER TABLE tournament_templates ALTER COLUMN commission_rate SET DEFAULT 20.00;
