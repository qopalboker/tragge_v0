-- Create a free practice contest for testing
-- Run this in psql or pgAdmin to create a contest that starts in 5 minutes

DO $$
DECLARE
    contest_id UUID;
    start_time TIMESTAMPTZ;
    end_time TIMESTAMPTZ;
BEGIN
    contest_id := gen_random_uuid();
    start_time := NOW() + INTERVAL '5 minutes';
    end_time := start_time + INTERVAL '1 hour';

    -- Insert the contest
    INSERT INTO contests (
        id, name, description, starts_at, ends_at, status,
        entry_fee_cents, platform_fee_bps, qty_total,
        duration_type, asset_class, duration_minutes,
        min_participants, max_participants,
        registration_deadline, auto_start, commission_rate,
        is_free, auto_generated
    ) VALUES (
        contest_id,
        'Test Free Practice - Crypto',
        'Free practice tournament for Crypto trading. No entry fee!',
        start_time,
        end_time,
        'published',
        0, -- entry_fee_cents
        0, -- platform_fee_bps
        10000.00, -- qty_total (starting balance)
        'hourly', -- duration_type
        'crypto', -- asset_class
        60, -- duration_minutes
        1, -- min_participants
        1000, -- max_participants
        start_time, -- registration_deadline (same as start)
        true, -- auto_start
        0.0, -- commission_rate
        true, -- is_free
        false -- auto_generated (manual creation)
    );

    -- Add symbols for the contest
    INSERT INTO contest_symbols (contest_id, symbol)
    SELECT contest_id, symbol
    FROM symbols
    WHERE asset_type = 'crypto' AND is_active = true;

    RAISE NOTICE 'Created contest ID: %', contest_id;
    RAISE NOTICE 'Starts at: %', start_time;
    RAISE NOTICE 'Ends at: %', end_time;
END $$;

-- Also create a forex contest
DO $$
DECLARE
    contest_id UUID;
    start_time TIMESTAMPTZ;
    end_time TIMESTAMPTZ;
BEGIN
    contest_id := gen_random_uuid();
    start_time := NOW() + INTERVAL '5 minutes';
    end_time := start_time + INTERVAL '1 hour';

    INSERT INTO contests (
        id, name, description, starts_at, ends_at, status,
        entry_fee_cents, platform_fee_bps, qty_total,
        duration_type, asset_class, duration_minutes,
        min_participants, max_participants,
        registration_deadline, auto_start, commission_rate,
        is_free, auto_generated
    ) VALUES (
        contest_id,
        'Test Free Practice - Forex',
        'Free practice tournament for Forex trading. No entry fee!',
        start_time,
        end_time,
        'published',
        0, 0, 10000.00, 'hourly', 'forex', 60,
        1, 1000, start_time, true, 0.0, true, false
    );

    INSERT INTO contest_symbols (contest_id, symbol)
    SELECT contest_id, symbol
    FROM symbols
    WHERE asset_type = 'forex' AND is_active = true;

    RAISE NOTICE 'Created Forex contest ID: %', contest_id;
END $$;

SELECT id, name, status, starts_at, ends_at, is_free
FROM contests
WHERE is_free = true
ORDER BY created_at DESC
LIMIT 5;
