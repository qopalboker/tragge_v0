-- Seed script for creating a test contest
-- Run this after applying all migrations: psql $POSTGRES_DSN -f packages/db/seed_test_contest.sql

-- Create a test contest that is open for registration
INSERT INTO contests (
    name,
    description,
    starts_at,
    ends_at,
    status,
    entry_fee_cents,
    platform_fee_bps,
    qty_total,
    duration_type,
    is_free
) VALUES (
    'Test Trading Contest',
    'A test contest for demo purposes. Join and start trading!',
    NOW() + INTERVAL '5 minutes',
    NOW() + INTERVAL '2 hours',
    'registration_open',
    0,                          -- Free contest
    0,                          -- No platform fee
    100000,                     -- $100,000 virtual trading capital
    'hourly',                   -- Duration type
    true                        -- Is free
) ON CONFLICT DO NOTHING
RETURNING id, name, status;

-- Get the contest ID that was just created (for adding symbols)
DO $$
DECLARE
    v_contest_id UUID;
BEGIN
    SELECT id INTO v_contest_id FROM contests WHERE name = 'Test Trading Contest' LIMIT 1;

    IF v_contest_id IS NOT NULL THEN
        -- Add trading symbols for this contest
        INSERT INTO contest_symbols (contest_id, symbol, enabled) VALUES
            (v_contest_id, 'AAPL', true),
            (v_contest_id, 'GOOGL', true),
            (v_contest_id, 'MSFT', true),
            (v_contest_id, 'AMZN', true),
            (v_contest_id, 'TSLA', true)
        ON CONFLICT (contest_id, symbol) DO UPDATE SET enabled = EXCLUDED.enabled;

        RAISE NOTICE 'Test contest created with ID: %', v_contest_id;
    ELSE
        RAISE NOTICE 'Contest already exists or could not be created';
    END IF;
END $$;

-- Show the created contest
SELECT id, name, status, starts_at, ends_at, entry_fee_cents, qty_total, duration_type
FROM contests
WHERE name = 'Test Trading Contest';

-- Show contest symbols
SELECT cs.contest_id, cs.symbol, cs.enabled
FROM contest_symbols cs
JOIN contests c ON c.id = cs.contest_id
WHERE c.name = 'Test Trading Contest';
