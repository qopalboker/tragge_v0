-- Migration 0068: Tournament state machine enhancements
-- Adds registration_opens_at for auto-opening registration
-- Creates tournaments_archive table for completed contest archival

-- 1. Add registration_opens_at column (when registration should auto-open)
ALTER TABLE contests ADD COLUMN IF NOT EXISTS registration_opens_at TIMESTAMPTZ;

-- Constraint: registration_opens_at must be before starts_at
ALTER TABLE contests ADD CONSTRAINT chk_registration_opens_at_valid
    CHECK (registration_opens_at IS NULL OR registration_opens_at <= starts_at);

-- Partial index for the scheduler query (only scheduled contests with a registration_opens_at)
CREATE INDEX idx_contests_registration_opens_at
    ON contests(registration_opens_at)
    WHERE status = 'scheduled' AND registration_opens_at IS NOT NULL;

-- 2. Create tournaments_archive table for completed contest archival
CREATE TABLE IF NOT EXISTS tournaments_archive (
    id              UUID PRIMARY KEY,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    starts_at       TIMESTAMPTZ NOT NULL,
    ends_at         TIMESTAMPTZ NOT NULL,
    status          contest_status NOT NULL,
    entry_fee_cents INT DEFAULT 0,
    platform_fee_bps INT DEFAULT 0,
    qty_total       BIGINT DEFAULT 100000,
    rules_json      JSONB,
    created_at      TIMESTAMPTZ DEFAULT NOW(),

    -- Extended lifecycle fields
    published_at         TIMESTAMPTZ,
    started_at           TIMESTAMPTZ,
    ended_at             TIMESTAMPTZ,
    settled_at           TIMESTAMPTZ,
    cancelled_at         TIMESTAMPTZ,
    cancellation_reason  TEXT,
    current_participants INT DEFAULT 0,
    min_participants     INT DEFAULT 2,
    max_participants     INT,
    registration_deadline TIMESTAMPTZ,
    registration_opens_at TIMESTAMPTZ,
    auto_start           BOOLEAN DEFAULT FALSE,
    commission_rate      NUMERIC DEFAULT 0,
    paused_at            TIMESTAMPTZ,
    total_paused_duration INTERVAL DEFAULT '0 seconds',

    -- Archive metadata
    archived_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_tournaments_archive_archived_at ON tournaments_archive(archived_at);
CREATE INDEX idx_tournaments_archive_status ON tournaments_archive(status);
