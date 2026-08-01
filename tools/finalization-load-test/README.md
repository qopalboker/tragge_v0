# Contest Finalization Load Test

A load testing tool for measuring contest finalization and prize distribution performance with large participant counts.

## Features

- Simulates contest with configurable number of participants (up to 10,000+)
- Tests prize pool calculation and distribution
- Measures end-to-end finalization latency
- Supports both Kafka and API trigger methods
- Verifies database writes (ranks, prizes, snapshots)
- Comprehensive timing breakdown

## Usage

```bash
# Build the tool
go build -o finalization-load-test .

# Run with default settings (1000 participants, Kafka trigger)
./finalization-load-test

# Run with 1000 participants using Kafka trigger
./finalization-load-test \
  -participants 1000 \
  -trigger kafka \
  -kafka-brokers localhost:9092

# Run with API trigger (requires admin credentials)
./finalization-load-test \
  -participants 1000 \
  -trigger api \
  -admin-email admin@example.com \
  -admin-password adminpass123
```

## Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-participants` | 1000 | Number of participants to simulate |
| `-contest-id` | (auto) | Existing contest ID (creates new if empty) |
| `-create-contest` | true | Create a new contest for testing |
| `-trigger` | kafka | Trigger method: `kafka` or `api` |
| `-entry-fee` | 1000 | Entry fee in cents |
| `-platform-fee-bps` | 1000 | Platform fee in basis points (10% default) |
| `-min-score` | -10000 | Minimum score for participants |
| `-max-score` | 50000 | Maximum score for participants |
| `-postgres-dsn` | ... | PostgreSQL connection string |
| `-redis-addr` | localhost:6379 | Redis address |
| `-kafka-brokers` | localhost:9092 | Kafka brokers |
| `-admin-bff` | http://localhost:8083 | Admin BFF URL |
| `-user-bff` | http://localhost:8081 | User BFF URL |
| `-admin-email` | | Admin email (for API trigger) |
| `-admin-password` | | Admin password (for API trigger) |

## Test Scenario: Prize Distribution for 1000 Participants

This tool is designed for the scenario where a contest with 1000 participants needs to finalize and distribute prizes:

```bash
./finalization-load-test \
  -participants 1000 \
  -entry-fee 1000 \
  -platform-fee-bps 1000 \
  -trigger kafka
```

### What Gets Tested

1. **Database Writes**
   - 1000 user records
   - 1000 contest_participant records
   - Final ranks for all participants
   - Prize amounts for winners
   - Leaderboard snapshot

2. **Redis Operations**
   - Leaderboard sorted set with 1000 members
   - Range queries for ranking
   - Score retrieval

3. **Prize Calculation**
   - Gross prize pool: 1000 × $10.00 = $10,000
   - Platform fee (10%): $1,000
   - Net prize pool: $9,000
   - Winner distribution per configured rules

## Output

```
=== Contest Finalization Load Test Results ===

Configuration:
  Participants:     1000
  Entry Fee:        1000 cents
  Platform Fee:     1000 bps
  Trigger Method:   kafka

Timing:
  Setup:            125ms
  Participants:     2.35s
  Trigger:          15ms
  Finalization:     1.85s
  Total:            4.34s

Throughput:
  Participants/sec: 540.54

Results:
  Participants Created: 1000
  Ranks Written:        1000
  Prizes Distributed:   50

Verification:
  Rank Success Rate:    100.00%
  Status:               PASSED ✓

Performance Assessment:
  ✓ Excellent: Finalization completed in under 30 seconds
```

## Performance Expectations

For a contest with 1000 participants:
- Participant creation: < 5 seconds
- Finalization: < 30 seconds
- Total: < 60 seconds
- Success rate: 100%

## Architecture

```
                    ┌──────────────────────┐
                    │  Finalization Load   │
                    │       Test           │
                    └──────────┬───────────┘
                               │
        ┌──────────────────────┼──────────────────────┐
        │                      │                      │
        ▼                      ▼                      ▼
  ┌───────────┐         ┌───────────┐         ┌───────────┐
  │PostgreSQL │         │   Redis   │         │   Kafka   │
  │           │         │           │         │           │
  │ - users   │         │ - lb:id   │         │contests.v1│
  │ - contest │         │  (sorted  │         │           │
  │   partici-│         │   set)    │         │           │
  │   pants   │         │           │         │           │
  └─────┬─────┘         └─────┬─────┘         └─────┬─────┘
        │                     │                     │
        └─────────────────────┼─────────────────────┘
                              │
                              ▼
                    ┌──────────────────────┐
                    │  leaderboard-worker  │
                    │                      │
                    │  - Calculate payouts │
                    │  - Write final ranks │
                    │  - Credit wallets    │
                    │  - Create snapshot   │
                    └──────────────────────┘
```

## Prerequisites

1. Running tragge platform services:
   - PostgreSQL with schema applied
   - Redis for leaderboard cache
   - Kafka/Redpanda for event streaming
   - leaderboard-worker for finalization processing

2. Database permissions for:
   - Creating users
   - Creating contest participants
   - Creating contests

3. For API trigger method:
   - Admin user account
   - admin-bff and user-bff services running

## Testing Prize Distribution Rules

The test uses the platform's configured prize distribution rules from `packages/contracts/prize_distribution/default.json`. To test different distribution schemes:

1. Modify the distribution configuration
2. Restart leaderboard-worker
3. Run the test with different participant counts

## Troubleshooting

### Finalization timeout
- Check leaderboard-worker is running
- Verify Kafka topic `contests.v1` exists
- Check leaderboard-worker logs for errors

### Missing ranks
- Verify Redis leaderboard has all participants
- Check database connection in leaderboard-worker
- Review transaction commit status

### Prize calculation errors
- Verify entry_fee_cents is set correctly
- Check platform_fee_bps is within valid range (0-10000)
- Review prize distribution configuration
