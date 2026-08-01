-- 0065_seed_tournament_templates.down.sql
-- Remove all tournament templates and their prize distributions seeded in 0065.

BEGIN;

-- Delete prize distributions first (FK cascade would handle this, but being explicit)
DELETE FROM template_prize_distributions
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

-- Delete the tournament templates
DELETE FROM tournament_templates
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
);

COMMIT;
