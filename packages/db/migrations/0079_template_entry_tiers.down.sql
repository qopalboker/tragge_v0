-- Drop tier_id from contests
ALTER TABLE contests DROP COLUMN IF EXISTS tier_id;

-- Drop indexes
DROP INDEX IF EXISTS idx_contests_tier_start_dedup;
DROP INDEX IF EXISTS idx_contests_tier_id;
DROP INDEX IF EXISTS idx_tier_prize_dist_tier_id;
DROP INDEX IF EXISTS idx_template_entry_tiers_active;
DROP INDEX IF EXISTS idx_template_entry_tiers_template_id;

-- Drop tables
DROP TABLE IF EXISTS tier_prize_distributions;
DROP TABLE IF EXISTS template_entry_tiers;
