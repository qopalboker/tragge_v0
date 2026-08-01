-- Update check constraints to include directional order types added in migration 0053.
-- This is split from 0053 because PostgreSQL cannot reference newly-added enum values
-- in the same transaction where they were added.

ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_limit_price_for_limit;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_stop_price_for_stop;

ALTER TABLE orders ADD CONSTRAINT chk_limit_price_for_limit CHECK (
    (type NOT IN ('limit', 'stop_limit', 'buy_limit', 'sell_limit')) OR (limit_price IS NOT NULL AND limit_price > 0)
);

ALTER TABLE orders ADD CONSTRAINT chk_stop_price_for_stop CHECK (
    (type NOT IN ('stop', 'stop_limit', 'buy_stop', 'sell_stop')) OR (stop_price IS NOT NULL AND stop_price > 0)
);

-- Add partial unique index to prevent duplicate auto-generated free contests
-- for the same asset class and time slot.
--
-- This guards against the TOCTOU race in the free-contest-generator service
-- where the duplicate check (SELECT COUNT) runs outside the transaction.

CREATE UNIQUE INDEX IF NOT EXISTS idx_contests_free_auto_unique
ON contests (asset_class, starts_at)
WHERE is_free = TRUE AND auto_generated = TRUE;
