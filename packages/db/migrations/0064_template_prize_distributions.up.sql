-- 0064_template_prize_distributions.up.sql
-- Create template_prize_distributions table for defining how a template's
-- prize pool is split among winners by rank.
-- Named "template_prize_distributions" to distinguish from the existing
-- "prize_distributions" table (migration 0019) which tracks actual settlement payouts.

-- ============================================================================
-- TEMPLATE PRIZE DISTRIBUTIONS TABLE
-- ============================================================================

CREATE TABLE template_prize_distributions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES tournament_templates(id) ON DELETE CASCADE,
    rank INT NOT NULL,
    percentage DECIMAL(5, 2) NOT NULL,          -- percentage of prize pool, e.g. 50.00 for 1st
    min_participants INT NOT NULL DEFAULT 1,     -- minimum participants needed for this rank to pay out
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Each rank can only appear once per template
    CONSTRAINT uq_template_prize_dist_rank UNIQUE (template_id, rank),

    -- Rank must be a positive integer (1st place, 2nd place, etc.)
    CONSTRAINT chk_prize_rank_positive CHECK (rank > 0),

    -- Percentage must be between 0 (exclusive) and 100 (inclusive)
    CONSTRAINT chk_prize_percentage_valid CHECK (percentage > 0 AND percentage <= 100),

    -- Min participants must be at least 1
    CONSTRAINT chk_prize_min_participants_positive CHECK (min_participants >= 1)
);

-- ============================================================================
-- INDEXES
-- ============================================================================

CREATE INDEX idx_template_prize_dist_template_id
    ON template_prize_distributions(template_id);

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE template_prize_distributions IS
    'Defines how a tournament template''s prize pool is split among winners. '
    'Percentages for a given template should sum to 100% — enforced at the application layer.';

COMMENT ON COLUMN template_prize_distributions.rank IS 'Winner rank: 1 for 1st place, 2 for 2nd place, etc.';
COMMENT ON COLUMN template_prize_distributions.percentage IS 'Percentage of prize pool allocated to this rank, e.g. 50.00 for 50%';
COMMENT ON COLUMN template_prize_distributions.min_participants IS 'Minimum number of participants required for this rank to be paid out';
