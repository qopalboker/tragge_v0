-- Script to start the test contest (change status to 'running')
-- Run this after users have joined: psql $POSTGRES_DSN -f packages/db/start_test_contest.sql

-- Update the test contest to running status
UPDATE contests
SET status = 'running',
    starts_at = NOW()  -- Ensure starts_at is in the past
WHERE name = 'Test Trading Contest'
RETURNING id, name, status, starts_at;

-- Alternative: Update by contest ID if you know it
-- UPDATE contests SET status = 'running', starts_at = NOW() WHERE id = 'your-contest-uuid-here';
