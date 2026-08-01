-- 0008_free_tournaments.up.sql
-- Add support for free practice tournaments and tournament templates

-- ============================================================================
-- CONTESTS TABLE ADDITIONS
-- ============================================================================

-- Add is_free flag to identify free practice tournaments
ALTER TABLE contests ADD COLUMN is_free BOOLEAN NOT NULL DEFAULT FALSE;

-- Add max_participants for capping tournament participation (nullable for unlimited)
ALTER TABLE contests ADD COLUMN max_participants INT;

-- Add auto_repeat flag for recurring tournaments
ALTER TABLE contests ADD COLUMN auto_repeat BOOLEAN NOT NULL DEFAULT FALSE;

-- Add repeat_interval for specifying how often to repeat (e.g., '1 hour', '1 day')
ALTER TABLE contests ADD COLUMN repeat_interval INTERVAL;

-- Add constraint to ensure repeat_interval is set when auto_repeat is true
ALTER TABLE contests ADD CONSTRAINT chk_auto_repeat_requires_interval
    CHECK (auto_repeat = FALSE OR repeat_interval IS NOT NULL);

-- Add constraint to ensure max_participants is positive if set
ALTER TABLE contests ADD CONSTRAINT chk_max_participants_positive
    CHECK (max_participants IS NULL OR max_participants > 0);

-- ============================================================================
-- TOURNAMENT TEMPLATES TABLE
-- ============================================================================

CREATE TABLE tournament_templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    duration_minutes INT NOT NULL,
    is_free BOOLEAN NOT NULL DEFAULT FALSE,
    entry_fee_cents INT NOT NULL DEFAULT 0,
    qty_total BIGINT NOT NULL DEFAULT 100000,
    symbols_json JSONB NOT NULL,
    prize_distribution_json JSONB,
    max_participants INT,
    auto_create BOOLEAN NOT NULL DEFAULT FALSE,
    create_cron VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_template_duration_positive CHECK (duration_minutes > 0),
    CONSTRAINT chk_template_entry_fee_positive CHECK (entry_fee_cents >= 0),
    CONSTRAINT chk_template_qty_positive CHECK (qty_total > 0),
    CONSTRAINT chk_template_max_participants_positive CHECK (max_participants IS NULL OR max_participants > 0),
    CONSTRAINT chk_template_auto_create_requires_cron CHECK (auto_create = FALSE OR create_cron IS NOT NULL),
    CONSTRAINT chk_template_free_no_fee CHECK (is_free = FALSE OR entry_fee_cents = 0)
);

-- ============================================================================
-- INDEXES FOR FREE TOURNAMENTS
-- ============================================================================

-- Index for querying free tournaments
CREATE INDEX idx_contests_is_free ON contests(is_free);

-- Composite index for finding open free tournaments
CREATE INDEX idx_contests_free_open ON contests(is_free, status, starts_at)
    WHERE is_free = TRUE;

-- Index for finding recurring tournaments
CREATE INDEX idx_contests_auto_repeat ON contests(auto_repeat)
    WHERE auto_repeat = TRUE;

-- Index for tournament templates auto-creation
CREATE INDEX idx_tournament_templates_auto_create ON tournament_templates(auto_create)
    WHERE auto_create = TRUE;

-- Index for template lookups by name
CREATE INDEX idx_tournament_templates_name ON tournament_templates(name);

-- Index for template creation time ordering
CREATE INDEX idx_tournament_templates_created_at ON tournament_templates(created_at);

-- ============================================================================
-- DEFAULT FREE TOURNAMENT TEMPLATE
-- ============================================================================

INSERT INTO tournament_templates (
    name,
    description,
    duration_minutes,
    is_free,
    entry_fee_cents,
    qty_total,
    symbols_json,
    prize_distribution_json,
    max_participants,
    auto_create,
    create_cron
) VALUES (
    'Hourly Practice',
    'Free practice tournament that runs every hour. Perfect for honing your trading skills without any risk. Compete against other traders using virtual currency.',
    60,
    TRUE,
    0,
    100000,
    '["AAPL", "GOOGL", "MSFT", "AMZN", "TSLA"]'::JSONB,
    NULL,
    1000,
    TRUE,
    '0 * * * *'
);
