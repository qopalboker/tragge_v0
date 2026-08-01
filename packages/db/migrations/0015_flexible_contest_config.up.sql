-- 0015_flexible_contest_config.up.sql
-- Add flexible contest configuration matching Tralent's variety

-- ============================================================================
-- ASSET CLASS ENUM
-- ============================================================================

CREATE TYPE asset_class AS ENUM (
    'forex',
    'crypto',
    'stocks',
    'mixed'
);

-- ============================================================================
-- CONTESTS TABLE ADDITIONS
-- ============================================================================

-- Add asset_class to identify contest category
ALTER TABLE contests ADD COLUMN asset_class asset_class NOT NULL DEFAULT 'mixed';

-- Add duration_minutes (computed from ends_at - starts_at, but useful for templates)
ALTER TABLE contests ADD COLUMN duration_minutes INT;

-- Add min_participants (minimum required to start the contest)
ALTER TABLE contests ADD COLUMN min_participants INT NOT NULL DEFAULT 2;

-- Add registration_deadline (when registration closes, defaults to starts_at)
ALTER TABLE contests ADD COLUMN registration_deadline TIMESTAMPTZ;

-- Add auto_start flag (auto-start when conditions are met)
ALTER TABLE contests ADD COLUMN auto_start BOOLEAN NOT NULL DEFAULT FALSE;

-- Add template_id to track which template was used (nullable for custom contests)
ALTER TABLE contests ADD COLUMN template_id UUID;

-- Add commission_rate for flexible commission (alternative to platform_fee_bps for percentage)
-- This is a decimal percentage like 17.00 for 17%
ALTER TABLE contests ADD COLUMN commission_rate DECIMAL(5,2) NOT NULL DEFAULT 17.00;

-- ============================================================================
-- UPDATE EXISTING CONTESTS
-- ============================================================================

-- Calculate duration_minutes from existing date range
UPDATE contests
SET duration_minutes = EXTRACT(EPOCH FROM (ends_at - starts_at)) / 60
WHERE duration_minutes IS NULL;

-- Set registration_deadline to starts_at if not set
UPDATE contests
SET registration_deadline = starts_at
WHERE registration_deadline IS NULL;

-- ============================================================================
-- CONSTRAINTS
-- ============================================================================

-- Min participants must be at least 1
ALTER TABLE contests ADD CONSTRAINT chk_min_participants_positive
    CHECK (min_participants >= 1);

-- Duration minutes must be positive if set
ALTER TABLE contests ADD CONSTRAINT chk_duration_minutes_positive
    CHECK (duration_minutes IS NULL OR duration_minutes > 0);

-- Commission rate must be between 0 and 50%
ALTER TABLE contests ADD CONSTRAINT chk_commission_rate_valid
    CHECK (commission_rate >= 0 AND commission_rate <= 50.00);

-- Registration deadline must be before or equal to starts_at
ALTER TABLE contests ADD CONSTRAINT chk_registration_deadline_valid
    CHECK (registration_deadline IS NULL OR registration_deadline <= starts_at);

-- ============================================================================
-- TOURNAMENT TEMPLATES ADDITIONS
-- ============================================================================

-- Add asset_class to tournament templates
ALTER TABLE tournament_templates ADD COLUMN asset_class asset_class NOT NULL DEFAULT 'mixed';

-- Add commission_rate to tournament templates
ALTER TABLE tournament_templates ADD COLUMN commission_rate DECIMAL(5,2) NOT NULL DEFAULT 17.00;

-- Add min_participants to tournament templates
ALTER TABLE tournament_templates ADD COLUMN min_participants INT NOT NULL DEFAULT 2;

-- Add auto_start to tournament templates
ALTER TABLE tournament_templates ADD COLUMN auto_start BOOLEAN NOT NULL DEFAULT FALSE;

-- Add template_key for programmatic lookup (e.g., 'crypto_rush_30m', 'forex_daily')
ALTER TABLE tournament_templates ADD COLUMN template_key VARCHAR(50);

-- ============================================================================
-- CONSTRAINTS FOR TOURNAMENT TEMPLATES
-- ============================================================================

ALTER TABLE tournament_templates ADD CONSTRAINT chk_template_min_participants_positive
    CHECK (min_participants >= 1);

ALTER TABLE tournament_templates ADD CONSTRAINT chk_template_commission_rate_valid
    CHECK (commission_rate >= 0 AND commission_rate <= 50.00);

-- Unique constraint on template_key
ALTER TABLE tournament_templates ADD CONSTRAINT uq_tournament_templates_key
    UNIQUE (template_key);

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Index on asset_class for filtering
CREATE INDEX idx_contests_asset_class ON contests(asset_class);

-- Composite index for discovery (status + asset_class)
CREATE INDEX idx_contests_status_asset_class ON contests(status, asset_class);

-- Composite index for registration deadline queries
CREATE INDEX idx_contests_registration_deadline ON contests(registration_deadline)
    WHERE status IN ('scheduled', 'registration_open');

-- Index on template_key for tournament templates
CREATE INDEX idx_tournament_templates_key ON tournament_templates(template_key)
    WHERE template_key IS NOT NULL;

-- Index on template_id for contests
CREATE INDEX idx_contests_template_id ON contests(template_id)
    WHERE template_id IS NOT NULL;

-- ============================================================================
-- INSERT DEFAULT CONTEST TEMPLATES
-- ============================================================================

-- Crypto Rush 30-minute
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'Crypto Rush 30min',
    'Fast-paced 30-minute crypto trading competition. Trade popular cryptocurrencies with quick results.',
    30, FALSE, 500, 50000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD"]'::JSONB,
    100, FALSE, 'crypto', 17.00, 2, FALSE, 'crypto_rush_30m'
);

-- Crypto Hourly
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'Crypto Hourly',
    'One-hour crypto trading tournament. More time to analyze and trade major cryptocurrencies.',
    60, FALSE, 1000, 100000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD", "ADA/USD", "AVAX/USD", "LINK/USD"]'::JSONB,
    200, FALSE, 'crypto', 17.00, 2, FALSE, 'crypto_hourly'
);

-- Crypto Daily
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'Crypto Daily Challenge',
    'Full-day crypto trading competition with 20 QTY allocation. Test your skills over 24 hours.',
    1440, FALSE, 5000, 200000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD", "ADA/USD", "AVAX/USD", "LINK/USD", "DOT/USD", "MATIC/USD", "SHIB/USD", "LTC/USD"]'::JSONB,
    500, FALSE, 'crypto', 17.00, 5, FALSE, 'crypto_daily'
);

-- Forex Rush 30-minute
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'Forex Rush 30min',
    'Quick 30-minute forex trading competition. Trade major currency pairs with rapid execution.',
    30, FALSE, 500, 50000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD"]'::JSONB,
    100, FALSE, 'forex', 17.00, 2, FALSE, 'forex_rush_30m'
);

-- Forex Hourly
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'Forex Hourly',
    'One-hour forex trading tournament. Trade 15+ major currency pairs.',
    60, FALSE, 1000, 100000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD"]'::JSONB,
    200, FALSE, 'forex', 17.00, 2, FALSE, 'forex_hourly'
);

-- Forex 4-Hour
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'Forex 4-Hour Tournament',
    'Extended 4-hour forex competition with comprehensive currency pair coverage.',
    240, FALSE, 2500, 150000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF"]'::JSONB,
    300, FALSE, 'forex', 17.00, 3, FALSE, 'forex_4hour'
);

-- Forex Daily
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'Forex Daily Championship',
    '24-hour forex trading championship with 33+ currency pairs. Full trading day experience.',
    1440, FALSE, 5000, 200000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF", "EUR/NZD", "GBP/NZD", "AUD/CAD", "NZD/CAD", "SGD/JPY", "USD/SGD", "USD/HKD", "USD/MXN", "USD/ZAR", "EUR/PLN", "EUR/TRY", "USD/TRY", "GBP/CAD"]'::JSONB,
    500, FALSE, 'forex', 17.00, 5, FALSE, 'forex_daily'
);

-- Forex Weekly
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'Forex Weekly Grand Prix',
    'Week-long forex competition for serious traders. Maximum strategy time with full pair coverage.',
    10080, FALSE, 10000, 500000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF", "EUR/NZD", "GBP/NZD", "AUD/CAD", "NZD/CAD", "SGD/JPY", "USD/SGD", "USD/HKD", "USD/MXN", "USD/ZAR", "EUR/PLN", "EUR/TRY", "USD/TRY", "GBP/CAD"]'::JSONB,
    1000, FALSE, 'forex', 17.00, 10, FALSE, 'forex_weekly'
);

-- Stocks Daily (Coming Soon - marked as not auto_create)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'US Stocks Daily',
    'Daily competition trading top 30 US equities. Market hours only.',
    1440, FALSE, 5000, 200000,
    '["AAPL", "MSFT", "GOOGL", "AMZN", "TSLA", "META", "NVDA", "BRK.B", "JPM", "JNJ", "V", "PG", "UNH", "HD", "MA", "DIS", "ADBE", "NFLX", "CRM", "PYPL", "INTC", "CSCO", "VZ", "KO", "PFE", "MRK", "ABT", "WMT", "NKE", "XOM"]'::JSONB,
    500, FALSE, 'stocks', 17.00, 5, FALSE, 'stocks_daily'
);

-- Free Practice Crypto (for beginners)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'Free Crypto Practice',
    'Free practice tournament to learn crypto trading. No entry fee, just compete for fun!',
    60, TRUE, 0, 100000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD"]'::JSONB,
    1000, FALSE, 'crypto', 0.00, 2, TRUE, 'crypto_free_practice'
);

-- Free Practice Forex (for beginners)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'Free Forex Practice',
    'Free practice tournament to learn forex trading. No entry fee, learn risk-free!',
    60, TRUE, 0, 100000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "AUD/USD", "USD/CAD"]'::JSONB,
    1000, FALSE, 'forex', 0.00, 2, TRUE, 'forex_free_practice'
);

-- High Stakes Crypto (Pro level)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'High Stakes Crypto',
    'Professional-level crypto trading competition with $100 entry. High risk, high reward.',
    240, FALSE, 10000, 200000,
    '["BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD", "ADA/USD", "AVAX/USD", "LINK/USD", "DOT/USD", "MATIC/USD", "SHIB/USD", "LTC/USD"]'::JSONB,
    50, FALSE, 'crypto', 17.00, 5, FALSE, 'crypto_high_stakes'
);

-- High Stakes Forex (Pro level)
INSERT INTO tournament_templates (
    name, description, duration_minutes, is_free, entry_fee_cents, qty_total,
    symbols_json, max_participants, auto_create, asset_class, commission_rate,
    min_participants, auto_start, template_key
) VALUES (
    'High Stakes Forex',
    'Professional-level forex trading competition with $100 entry. For experienced traders.',
    240, FALSE, 10000, 200000,
    '["EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD", "NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY", "AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD", "AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF"]'::JSONB,
    50, FALSE, 'forex', 17.00, 5, FALSE, 'forex_high_stakes'
);
