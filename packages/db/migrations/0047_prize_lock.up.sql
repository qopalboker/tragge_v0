-- Lock prize distribution at contest start time.
-- Once a contest transitions to "running", the prize breakdown is frozen
-- based on the final participant count so that it cannot change mid-contest.

CREATE TABLE IF NOT EXISTS contest_prize_locks (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    contest_id      UUID        NOT NULL REFERENCES contests(id),
    total_participants INT      NOT NULL,
    prize_pool_gross_cents BIGINT NOT NULL,
    prize_pool_net_cents   BIGINT NOT NULL,
    platform_fee_cents     BIGINT NOT NULL,
    commission_rate NUMERIC(5,2) NOT NULL,
    winners_count   INT         NOT NULL,
    distribution_json JSONB    NOT NULL,
    locked_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_contest_prize_locks UNIQUE (contest_id)
);

ALTER TABLE contests ADD COLUMN IF NOT EXISTS prizes_locked_at TIMESTAMPTZ;
ALTER TABLE contests ADD COLUMN IF NOT EXISTS prize_pool_net_cents BIGINT;
