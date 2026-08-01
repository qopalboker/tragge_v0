-- 0065_seed_tournament_templates.up.sql
-- Seed tournament templates for the Iranian market with Rial-based entry fees,
-- covering 5 duration tiers: quick (30m), free (1h), 4-hour, daily, and weekly.
-- Also seeds prize distribution rules for paid and free-featured templates.

BEGIN;

-- ============================================================================
-- TASK 2.1: QUICK 30-MINUTE TOURNAMENT TEMPLATES (6 templates)
-- All: duration=30min, commission_rate=0.20, has_prize=false
-- ============================================================================

-- Crypto Quick 50K Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto Quick 50K',
    'Fast 30-minute crypto tournament with 50,000 Rial entry. Trade top cryptocurrencies for quick results.',
    30, FALSE, 0, 50000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD"]'::JSONB,
    NULL, FALSE, 'crypto', 0.20,
    2, FALSE, 'crypto_quick_50k',
    'crypto', 'quick_30m', 50000, FALSE
) ON CONFLICT (template_key) DO NOTHING;

-- Crypto Quick 100K Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto Quick 100K',
    'Fast 30-minute crypto tournament with 100,000 Rial entry. Trade top cryptocurrencies for quick results.',
    30, FALSE, 0, 50000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD"]'::JSONB,
    NULL, FALSE, 'crypto', 0.20,
    2, FALSE, 'crypto_quick_100k',
    'crypto', 'quick_30m', 100000, FALSE
) ON CONFLICT (template_key) DO NOTHING;

-- Crypto Quick 200K Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto Quick 200K',
    'Fast 30-minute crypto tournament with 200,000 Rial entry. Trade top cryptocurrencies for quick results.',
    30, FALSE, 0, 50000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD"]'::JSONB,
    NULL, FALSE, 'crypto', 0.20,
    2, FALSE, 'crypto_quick_200k',
    'crypto', 'quick_30m', 200000, FALSE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex Quick 50K Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex Quick 50K',
    'Fast 30-minute forex tournament with 50,000 Rial entry. Trade major currency pairs for quick results.',
    30, FALSE, 0, 50000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD"]'::JSONB,
    NULL, FALSE, 'forex', 0.20,
    2, FALSE, 'forex_quick_50k',
    'forex', 'quick_30m', 50000, FALSE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex Quick 100K Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex Quick 100K',
    'Fast 30-minute forex tournament with 100,000 Rial entry. Trade major currency pairs for quick results.',
    30, FALSE, 0, 50000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD"]'::JSONB,
    NULL, FALSE, 'forex', 0.20,
    2, FALSE, 'forex_quick_100k',
    'forex', 'quick_30m', 100000, FALSE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex Quick 200K Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex Quick 200K',
    'Fast 30-minute forex tournament with 200,000 Rial entry. Trade major currency pairs for quick results.',
    30, FALSE, 0, 50000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD"]'::JSONB,
    NULL, FALSE, 'forex', 0.20,
    2, FALSE, 'forex_quick_200k',
    'forex', 'quick_30m', 200000, FALSE
) ON CONFLICT (template_key) DO NOTHING;

-- ============================================================================
-- TASK 2.2: FREE 1-HOUR TOURNAMENT TEMPLATES (4 templates)
-- All: duration=60min, entry_fee=0, is_free=TRUE, commission_rate=0.00
-- ============================================================================

-- Crypto Free 1h
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto Free',
    'Free 1-hour crypto practice tournament. No entry fee — compete for fun and experience!',
    60, TRUE, 0, 100000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD"]'::JSONB,
    NULL, FALSE, 'crypto', 0.00,
    2, FALSE, 'crypto_free_1h',
    'crypto', 'free_1h', 0, FALSE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex Free 1h
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex Free',
    'Free 1-hour forex practice tournament. No entry fee — learn risk-free!',
    60, TRUE, 0, 100000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD"]'::JSONB,
    NULL, FALSE, 'forex', 0.00,
    2, FALSE, 'forex_free_1h',
    'forex', 'free_1h', 0, FALSE
) ON CONFLICT (template_key) DO NOTHING;

-- Crypto Free Featured 1h (has_prize=true, platform-funded)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, prize_distribution_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto Free Featured',
    'Featured free 1-hour crypto tournament with a platform-funded prize. Win real prizes at no cost!',
    60, TRUE, 0, 100000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD"]'::JSONB,
    '[{"rank": 1, "type": "fixed", "amount_rials": 100000}]'::JSONB,
    NULL, FALSE, 'crypto', 0.00,
    2, FALSE, 'crypto_free_featured_1h',
    'crypto', 'free_1h', 0, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex Free Featured 1h (has_prize=true, platform-funded)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, prize_distribution_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex Free Featured',
    'Featured free 1-hour forex tournament with a platform-funded prize. Win real prizes at no cost!',
    60, TRUE, 0, 100000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD"]'::JSONB,
    '[{"rank": 1, "type": "fixed", "amount_rials": 100000}]'::JSONB,
    NULL, FALSE, 'forex', 0.00,
    2, FALSE, 'forex_free_featured_1h',
    'forex', 'free_1h', 0, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- ============================================================================
-- TASK 2.3: 4-HOUR TOURNAMENT TEMPLATES (4 templates)
-- All: duration=240min, commission_rate=0.20, has_prize=true (default)
-- ============================================================================

-- Crypto 4h 50K Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto 4-Hour 50K',
    '4-hour crypto trading tournament with 50,000 Rial entry. Extended time for deeper analysis.',
    240, FALSE, 0, 150000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD", "ADA/USD", "AVAX/USD", "LINK/USD", "DOT/USD", "MATIC/USD", "SHIB/USD", "LTC/USD"]'::JSONB,
    NULL, FALSE, 'crypto', 0.20,
    2, FALSE, 'crypto_4h_50k',
    'crypto', 'four_hour', 50000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Crypto 4h 200K Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto 4-Hour 200K',
    '4-hour crypto trading tournament with 200,000 Rial entry. Higher stakes, bigger prizes.',
    240, FALSE, 0, 150000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD", "ADA/USD", "AVAX/USD", "LINK/USD", "DOT/USD", "MATIC/USD", "SHIB/USD", "LTC/USD"]'::JSONB,
    NULL, FALSE, 'crypto', 0.20,
    2, FALSE, 'crypto_4h_200k',
    'crypto', 'four_hour', 200000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex 4h 50K Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex 4-Hour 50K',
    '4-hour forex trading tournament with 50,000 Rial entry. Trade 20 currency pairs.',
    240, FALSE, 0, 150000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF"]'::JSONB,
    NULL, FALSE, 'forex', 0.20,
    2, FALSE, 'forex_4h_50k',
    'forex', 'four_hour', 50000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex 4h 200K Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex 4-Hour 200K',
    '4-hour forex trading tournament with 200,000 Rial entry. Higher stakes with 20 currency pairs.',
    240, FALSE, 0, 150000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF"]'::JSONB,
    NULL, FALSE, 'forex', 0.20,
    2, FALSE, 'forex_4h_200k',
    'forex', 'four_hour', 200000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- ============================================================================
-- TASK 2.4: DAILY TOURNAMENT TEMPLATES (3 templates)
-- Crypto: 1440 min (24h), Forex: 1320 min (22h — ends at 22:00 IRST)
-- ============================================================================

-- Crypto Daily 1M Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto Daily 1M',
    'Full-day 24-hour crypto tournament with 1,000,000 Rial entry. Test your skills with 12 major coins.',
    1440, FALSE, 0, 200000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD", "ADA/USD", "AVAX/USD", "LINK/USD", "DOT/USD", "MATIC/USD", "SHIB/USD", "LTC/USD"]'::JSONB,
    NULL, FALSE, 'crypto', 0.20,
    2, FALSE, 'crypto_daily_1m',
    'crypto', 'daily', 1000000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex Daily 500K Toman (22h — ends at 22:00 IRST due to forex market close)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex Daily 500K',
    '22-hour forex tournament with 500,000 Rial entry. Ends at 22:00 IRST (forex market close). 33 currency pairs.',
    1320, FALSE, 0, 200000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF", "EUR/NZD", "GBP/NZD", "AUD/CAD", "NZD/CAD", "SGD/JPY", "USD/SGD", "USD/HKD", "USD/MXN", "USD/ZAR", "EUR/PLN", "EUR/TRY", "USD/TRY", "GBP/CAD"]'::JSONB,
    NULL, FALSE, 'forex', 0.20,
    2, FALSE, 'forex_daily_500k',
    'forex', 'daily', 500000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex Daily 1.5M Toman (22h — ends at 22:00 IRST due to forex market close)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex Daily 1.5M',
    '22-hour forex tournament with 1,500,000 Rial entry. Ends at 22:00 IRST (forex market close). 33 currency pairs.',
    1320, FALSE, 0, 200000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF", "EUR/NZD", "GBP/NZD", "AUD/CAD", "NZD/CAD", "SGD/JPY", "USD/SGD", "USD/HKD", "USD/MXN", "USD/ZAR", "EUR/PLN", "EUR/TRY", "USD/TRY", "GBP/CAD"]'::JSONB,
    NULL, FALSE, 'forex', 0.20,
    2, FALSE, 'forex_daily_1500k',
    'forex', 'daily', 1500000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- ============================================================================
-- TASK 2.5: WEEKLY TOURNAMENT TEMPLATES (6 templates)
-- Crypto: 10080 min (7 days, Sat 00:00 to next Sat 00:00 IRST)
-- Forex:  7320 min (~122h, Sat 00:00 to Wed 22:00 IRST)
-- ============================================================================

-- Crypto Weekly 2.5M Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto Weekly 2.5M',
    'Week-long crypto tournament (Saturday to Saturday IRST) with 2,500,000 Rial entry. 12 major coins.',
    10080, FALSE, 0, 500000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD", "ADA/USD", "AVAX/USD", "LINK/USD", "DOT/USD", "MATIC/USD", "SHIB/USD", "LTC/USD"]'::JSONB,
    NULL, FALSE, 'crypto', 0.20,
    2, FALSE, 'crypto_weekly_2500k',
    'crypto', 'weekly', 2500000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Crypto Weekly 5M Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto Weekly 5M',
    'Week-long crypto tournament (Saturday to Saturday IRST) with 5,000,000 Rial entry. 12 major coins.',
    10080, FALSE, 0, 500000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD", "ADA/USD", "AVAX/USD", "LINK/USD", "DOT/USD", "MATIC/USD", "SHIB/USD", "LTC/USD"]'::JSONB,
    NULL, FALSE, 'crypto', 0.20,
    2, FALSE, 'crypto_weekly_5m',
    'crypto', 'weekly', 5000000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Crypto Weekly 10M Toman
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Crypto Weekly 10M',
    'Week-long crypto tournament (Saturday to Saturday IRST) with 10,000,000 Rial entry. 12 major coins.',
    10080, FALSE, 0, 500000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD", "ADA/USD", "AVAX/USD", "LINK/USD", "DOT/USD", "MATIC/USD", "SHIB/USD", "LTC/USD"]'::JSONB,
    NULL, FALSE, 'crypto', 0.20,
    2, FALSE, 'crypto_weekly_10m',
    'crypto', 'weekly', 10000000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex Weekly 5M Toman (Sat 00:00 to Wed 22:00 IRST)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex Weekly 5M',
    'Forex weekly tournament (Saturday to Wednesday 22:00 IRST) with 5,000,000 Rial entry. 33 currency pairs.',
    7320, FALSE, 0, 500000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF", "EUR/NZD", "GBP/NZD", "AUD/CAD", "NZD/CAD", "SGD/JPY", "USD/SGD", "USD/HKD", "USD/MXN", "USD/ZAR", "EUR/PLN", "EUR/TRY", "USD/TRY", "GBP/CAD"]'::JSONB,
    NULL, FALSE, 'forex', 0.20,
    2, FALSE, 'forex_weekly_5m',
    'forex', 'weekly', 5000000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex Weekly 10M Toman (Sat 00:00 to Wed 22:00 IRST)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex Weekly 10M',
    'Forex weekly tournament (Saturday to Wednesday 22:00 IRST) with 10,000,000 Rial entry. 33 currency pairs.',
    7320, FALSE, 0, 500000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF", "EUR/NZD", "GBP/NZD", "AUD/CAD", "NZD/CAD", "SGD/JPY", "USD/SGD", "USD/HKD", "USD/MXN", "USD/ZAR", "EUR/PLN", "EUR/TRY", "USD/TRY", "GBP/CAD"]'::JSONB,
    NULL, FALSE, 'forex', 0.20,
    2, FALSE, 'forex_weekly_10m',
    'forex', 'weekly', 10000000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- Forex Weekly 50M Toman (Sat 00:00 to Wed 22:00 IRST)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key,
    market_type, template_duration_type, entry_fee, has_prize
) VALUES (
    'Forex Weekly 50M',
    'Forex weekly tournament (Saturday to Wednesday 22:00 IRST) with 50,000,000 Rial entry. Premium tier with 33 currency pairs.',
    7320, FALSE, 0, 500000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF", "EUR/NZD", "GBP/NZD", "AUD/CAD", "NZD/CAD", "SGD/JPY", "USD/SGD", "USD/HKD", "USD/MXN", "USD/ZAR", "EUR/PLN", "EUR/TRY", "USD/TRY", "GBP/CAD"]'::JSONB,
    NULL, FALSE, 'forex', 0.20,
    2, FALSE, 'forex_weekly_50m',
    'forex', 'weekly', 50000000, TRUE
) ON CONFLICT (template_key) DO NOTHING;

-- ============================================================================
-- TASK 2.6: PRIZE DISTRIBUTION RULES
-- Paid templates: 1st=50%, 2nd=30%, 3rd=20% from prize pool
-- Free featured: 1st=100% (fixed 100,000 Rials, platform-funded)
-- ============================================================================

-- Insert prize distributions for all PAID templates (entry_fee > 0)
-- 1st place: 50% of prize pool, requires at least 2 participants
INSERT INTO template_prize_distributions (template_id, rank, percentage, min_participants)
SELECT id, 1, 50.00, 2
FROM tournament_templates
WHERE template_key IN (
    'crypto_quick_50k', 'crypto_quick_100k', 'crypto_quick_200k',
    'forex_quick_50k', 'forex_quick_100k', 'forex_quick_200k',
    'crypto_4h_50k', 'crypto_4h_200k',
    'forex_4h_50k', 'forex_4h_200k',
    'crypto_daily_1m',
    'forex_daily_500k', 'forex_daily_1500k',
    'crypto_weekly_2500k', 'crypto_weekly_5m', 'crypto_weekly_10m',
    'forex_weekly_5m', 'forex_weekly_10m', 'forex_weekly_50m'
)
ON CONFLICT (template_id, rank) DO NOTHING;

-- 2nd place: 30% of prize pool, requires at least 5 participants
INSERT INTO template_prize_distributions (template_id, rank, percentage, min_participants)
SELECT id, 2, 30.00, 5
FROM tournament_templates
WHERE template_key IN (
    'crypto_quick_50k', 'crypto_quick_100k', 'crypto_quick_200k',
    'forex_quick_50k', 'forex_quick_100k', 'forex_quick_200k',
    'crypto_4h_50k', 'crypto_4h_200k',
    'forex_4h_50k', 'forex_4h_200k',
    'crypto_daily_1m',
    'forex_daily_500k', 'forex_daily_1500k',
    'crypto_weekly_2500k', 'crypto_weekly_5m', 'crypto_weekly_10m',
    'forex_weekly_5m', 'forex_weekly_10m', 'forex_weekly_50m'
)
ON CONFLICT (template_id, rank) DO NOTHING;

-- 3rd place: 20% of prize pool, requires at least 10 participants
INSERT INTO template_prize_distributions (template_id, rank, percentage, min_participants)
SELECT id, 3, 20.00, 10
FROM tournament_templates
WHERE template_key IN (
    'crypto_quick_50k', 'crypto_quick_100k', 'crypto_quick_200k',
    'forex_quick_50k', 'forex_quick_100k', 'forex_quick_200k',
    'crypto_4h_50k', 'crypto_4h_200k',
    'forex_4h_50k', 'forex_4h_200k',
    'crypto_daily_1m',
    'forex_daily_500k', 'forex_daily_1500k',
    'crypto_weekly_2500k', 'crypto_weekly_5m', 'crypto_weekly_10m',
    'forex_weekly_5m', 'forex_weekly_10m', 'forex_weekly_50m'
)
ON CONFLICT (template_id, rank) DO NOTHING;

-- Free featured templates: 1st place gets 100% (fixed prize, platform-funded)
INSERT INTO template_prize_distributions (template_id, rank, percentage, min_participants)
SELECT id, 1, 100.00, 2
FROM tournament_templates
WHERE template_key IN ('crypto_free_featured_1h', 'forex_free_featured_1h')
ON CONFLICT (template_id, rank) DO NOTHING;

COMMIT;
