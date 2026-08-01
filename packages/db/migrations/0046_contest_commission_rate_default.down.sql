-- 0046_contest_commission_rate_default.down.sql
-- Revert default commission_rate from 20.00 to 17.00

ALTER TABLE contests ALTER COLUMN commission_rate SET DEFAULT 17.00;

UPDATE contests
SET commission_rate = 17.00
WHERE commission_rate = 20.00;

ALTER TABLE tournament_templates ALTER COLUMN commission_rate SET DEFAULT 17.00;
