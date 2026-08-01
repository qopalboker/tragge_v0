DROP INDEX IF EXISTS idx_contests_free_auto_unique;

-- Revert check constraints to original form (without directional types)
ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_limit_price_for_limit;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_stop_price_for_stop;

ALTER TABLE orders ADD CONSTRAINT chk_limit_price_for_limit CHECK (
    (type NOT IN ('limit', 'stop_limit')) OR (limit_price IS NOT NULL AND limit_price > 0)
);

ALTER TABLE orders ADD CONSTRAINT chk_stop_price_for_stop CHECK (
    (type NOT IN ('stop', 'stop_limit')) OR (stop_price IS NOT NULL AND stop_price > 0)
);
