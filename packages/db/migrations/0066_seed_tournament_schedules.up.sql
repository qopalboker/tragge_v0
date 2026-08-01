-- 0066_seed_tournament_schedules.up.sql
-- Seed tournament schedules linking to the 23 tournament templates from migration 0065.
-- Covers 5 duration tiers: quick (30m), free (1h), 4-hour, daily, and weekly.
-- All cron expressions use UTC. IRST = UTC+3:30.

BEGIN;

-- ============================================================================
-- TASK 3.1: QUICK 30-MINUTE SCHEDULES (6 schedules)
-- cron: every 10 minutes, all days, weekend_behavior: crypto_only
-- On weekdays: all 6 fire; on weekends: only 3 crypto templates fire
-- ============================================================================

-- Crypto Quick 50K
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '*/10 * * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_quick_50k';

-- Crypto Quick 100K
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '*/10 * * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_quick_100k';

-- Crypto Quick 200K
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '*/10 * * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_quick_200k';

-- Forex Quick 50K
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '*/10 * * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_quick_50k';

-- Forex Quick 100K
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '*/10 * * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_quick_100k';

-- Forex Quick 200K
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '*/10 * * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_quick_200k';

-- ============================================================================
-- TASK 3.2: FREE 1-HOUR SCHEDULES (4 schedules)
-- Regular free: every hour on the hour, all days, weekend_behavior: crypto_only
-- Featured free: peak hours 17:00 & 21:00 IRST = 13:30 & 17:30 UTC
-- ============================================================================

-- Crypto Free 1h (regular, every hour)
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '0 */1 * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_free_1h';

-- Forex Free 1h (regular, every hour)
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '0 */1 * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_free_1h';

-- Crypto Free Featured 1h (peak hours: 17:00 & 21:00 IRST = 13:30 & 17:30 UTC)
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 13,17 * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_free_featured_1h';

-- Forex Free Featured 1h (peak hours: 17:00 & 21:00 IRST = 13:30 & 17:30 UTC)
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 13,17 * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_free_featured_1h';

-- ============================================================================
-- TASK 3.3: 4-HOUR SCHEDULES (4 schedules)
-- 6 blocks at IRST: 00:00, 04:00, 08:00, 12:00, 16:00, 20:00
-- UTC equivalents: 20:30, 00:30, 04:30, 08:30, 12:30, 16:30
-- cron: "30 20,0,4,8,12,16 * * *", all days, weekend_behavior: crypto_only
-- ============================================================================

-- Crypto 4h 50K
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20,0,4,8,12,16 * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_4h_50k';

-- Crypto 4h 200K
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20,0,4,8,12,16 * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_4h_200k';

-- Forex 4h 50K
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20,0,4,8,12,16 * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_4h_50k';

-- Forex 4h 200K
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20,0,4,8,12,16 * * *', NULL, '{0,1,2,3,4,5,6}', 'crypto_only'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_4h_200k';

-- ============================================================================
-- TASK 3.4: DAILY SCHEDULES (3 schedules)
-- cron: "30 20 * * *" (20:30 UTC = 00:00 IRST, once daily)
-- Crypto: all days, weekend_behavior: normal (crypto runs every day)
-- Forex: weekdays only (active_days skips Sat=0, Sun=1), weekend_behavior: skip
-- ============================================================================

-- Crypto Daily 1M (runs every day)
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20 * * *', NULL, '{0,1,2,3,4,5,6}', 'normal'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_daily_1m';

-- Forex Daily 500K (weekdays only, skip weekends)
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20 * * *', NULL, '{2,3,4,5,6}', 'skip'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_daily_500k';

-- Forex Daily 1.5M (weekdays only, skip weekends)
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20 * * *', NULL, '{2,3,4,5,6}', 'skip'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_daily_1500k';

-- ============================================================================
-- TASK 3.5: WEEKLY SCHEDULES (6 schedules)
-- cron: "30 20 * * 6" (Saturday 00:00 IRST = Friday 20:30 UTC)
-- All templates: all active_days, weekend_behavior: normal
-- Weekly tournaments span the full week regardless of weekends
-- ============================================================================

-- Crypto Weekly 2.5M
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20 * * 6', NULL, '{0,1,2,3,4,5,6}', 'normal'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_weekly_2500k';

-- Crypto Weekly 5M
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20 * * 6', NULL, '{0,1,2,3,4,5,6}', 'normal'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_weekly_5m';

-- Crypto Weekly 10M
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20 * * 6', NULL, '{0,1,2,3,4,5,6}', 'normal'::weekend_behavior
FROM tournament_templates WHERE template_key = 'crypto_weekly_10m';

-- Forex Weekly 5M
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20 * * 6', NULL, '{0,1,2,3,4,5,6}', 'normal'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_weekly_5m';

-- Forex Weekly 10M
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20 * * 6', NULL, '{0,1,2,3,4,5,6}', 'normal'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_weekly_10m';

-- Forex Weekly 50M
INSERT INTO tournament_schedules (template_id, cron_expression, start_time_utc, active_days, weekend_behavior)
SELECT id, '30 20 * * 6', NULL, '{0,1,2,3,4,5,6}', 'normal'::weekend_behavior
FROM tournament_templates WHERE template_key = 'forex_weekly_50m';

COMMIT;
