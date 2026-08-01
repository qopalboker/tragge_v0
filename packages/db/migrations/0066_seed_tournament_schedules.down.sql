-- 0066_seed_tournament_schedules.down.sql
-- Remove all tournament schedules seeded in 0066.

BEGIN;

DELETE FROM tournament_schedules
WHERE template_id IN (
    SELECT id FROM tournament_templates
    WHERE template_key IN (
        -- Quick 30-min
        'crypto_quick_50k', 'crypto_quick_100k', 'crypto_quick_200k',
        'forex_quick_50k', 'forex_quick_100k', 'forex_quick_200k',
        -- Free 1-hour
        'crypto_free_1h', 'forex_free_1h',
        'crypto_free_featured_1h', 'forex_free_featured_1h',
        -- 4-hour
        'crypto_4h_50k', 'crypto_4h_200k',
        'forex_4h_50k', 'forex_4h_200k',
        -- Daily
        'crypto_daily_1m',
        'forex_daily_500k', 'forex_daily_1500k',
        -- Weekly
        'crypto_weekly_2500k', 'crypto_weekly_5m', 'crypto_weekly_10m',
        'forex_weekly_5m', 'forex_weekly_10m', 'forex_weekly_50m'
    )
);

COMMIT;
