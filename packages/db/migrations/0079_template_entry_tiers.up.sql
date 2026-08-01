-- ============================================================================
-- TEMPLATE ENTRY TIERS TABLE
-- Allows each tournament template to have multiple entry fee levels.
-- When a schedule fires, one contest is created per active tier.
-- ============================================================================

CREATE TABLE template_entry_tiers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES tournament_templates(id) ON DELETE CASCADE,

    -- Entry fee for this tier (Rials)
    entry_fee BIGINT NOT NULL DEFAULT 0,

    -- Display label (e.g., "Bronze", "Silver", "Gold" or "50K", "100K", "200K")
    label VARCHAR(100),

    -- Display order
    sort_order INT NOT NULL DEFAULT 0,

    -- Whether this tier is active
    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    -- Is this a free tier?
    is_free BOOLEAN NOT NULL DEFAULT FALSE,

    -- QTY allocation override (NULL = use template default)
    qty_total_override BIGINT,

    -- Max participants override (NULL = use template default)
    max_participants_override INT,

    -- Commission rate override (NULL = use template default)
    commission_rate_override DECIMAL(5,2),

    -- Whether this tier has its own prize distribution
    has_prize_override BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Prevent duplicate entry fees per template
    CONSTRAINT uq_template_tier_entry_fee UNIQUE (template_id, entry_fee),

    -- Entry fee must be non-negative
    CONSTRAINT chk_tier_entry_fee_non_negative CHECK (entry_fee >= 0),

    -- Free tiers must have entry_fee = 0
    CONSTRAINT chk_tier_free_zero_fee CHECK (
        (is_free = FALSE) OR (entry_fee = 0)
    ),

    -- Sort order must be non-negative
    CONSTRAINT chk_tier_sort_order CHECK (sort_order >= 0)
);

-- ============================================================================
-- TIER-SPECIFIC PRIZE DISTRIBUTIONS
-- ============================================================================

CREATE TABLE tier_prize_distributions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tier_id UUID NOT NULL REFERENCES template_entry_tiers(id) ON DELETE CASCADE,
    rank INT NOT NULL,
    percentage DECIMAL(5, 2) NOT NULL,
    min_participants INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_tier_prize_dist_rank UNIQUE (tier_id, rank),
    CONSTRAINT chk_tier_prize_rank_positive CHECK (rank > 0),
    CONSTRAINT chk_tier_prize_percentage_valid CHECK (percentage > 0 AND percentage <= 100),
    CONSTRAINT chk_tier_prize_min_participants_positive CHECK (min_participants >= 1)
);

-- ============================================================================
-- ADD tier_id TO CONTESTS TABLE
-- ============================================================================

ALTER TABLE contests ADD COLUMN IF NOT EXISTS tier_id UUID
    REFERENCES template_entry_tiers(id) ON DELETE SET NULL;

-- ============================================================================
-- INDEXES
-- ============================================================================

CREATE INDEX idx_template_entry_tiers_template_id
    ON template_entry_tiers(template_id);

CREATE INDEX idx_template_entry_tiers_active
    ON template_entry_tiers(template_id, is_active)
    WHERE is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_contests_tier_id
    ON contests(tier_id) WHERE tier_id IS NOT NULL;

-- Dedup index: one contest per tier per time slot
CREATE UNIQUE INDEX IF NOT EXISTS idx_contests_tier_start_dedup
    ON contests(tier_id, starts_at)
    WHERE tier_id IS NOT NULL;

CREATE INDEX idx_tier_prize_dist_tier_id
    ON tier_prize_distributions(tier_id);

-- ============================================================================
-- UPDATED_AT TRIGGER
-- ============================================================================

CREATE TRIGGER set_template_entry_tiers_updated_at
    BEFORE UPDATE ON template_entry_tiers
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================================
-- BACKWARD COMPATIBILITY: Migrate existing templates to tiers
-- Each existing template gets one default tier matching its current entry_fee.
-- ============================================================================

INSERT INTO template_entry_tiers (template_id, entry_fee, label, sort_order, is_active, is_free)
SELECT
    id,
    entry_fee,
    CASE
        WHEN is_free THEN 'Free'
        ELSE CONCAT(entry_fee / 10, ' Toman')
    END,
    0,
    is_active,
    is_free
FROM tournament_templates
WHERE NOT EXISTS (
    SELECT 1 FROM template_entry_tiers t WHERE t.template_id = tournament_templates.id
);
