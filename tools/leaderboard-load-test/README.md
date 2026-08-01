# Leaderboard Load Test

A load testing tool for measuring leaderboard query performance under high concurrency.

## Features

- Simulates N concurrent leaderboard requests
- Measures response latency (p50, p95, p99)
- Supports multiple contest IDs for distributed testing
- Tracks success/failure rates
- Real-time progress reporting

## Usage

```bash
# Build the tool
go build -o leaderboard-load-test .

# Run with default settings (500 concurrent requests)
./leaderboard-load-test \
  -email test@example.com \
  -password password123 \
  -contest-ids "contest-id-1,contest-id-2"

# Run with custom settings
./leaderboard-load-test \
  -concurrent 500 \
  -email test@example.com \
  -password password123 \
  -contest-ids "contest-id-1,contest-id-2,contest-id-3" \
  -duration 120s \
  -interval 100ms
```

## Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-concurrent` | 500 | Number of concurrent requests |
| `-email` | (required) | Login email |
| `-password` | (required) | Login password |
| `-contest-ids` | (required) | Contest IDs (comma-separated) |
| `-duration` | 60s | Test duration |
| `-interval` | 100ms | Interval between request batches |
| `-timeout` | 30s | Request timeout |
| `-user-bff` | http://localhost:8081 | User BFF URL |

## Test Scenario: 500 Simultaneous Leaderboard Requests

This tool is designed to test the scenario where 500 users simultaneously request leaderboard data:

```bash
./leaderboard-load-test \
  -concurrent 500 \
  -email admin@example.com \
  -password adminpass \
  -contest-ids "running-contest-id" \
  -duration 60s
```

## Output

```
=== Leaderboard Load Test Results ===

Request Metrics:
  Total Requests:  5000
  Successful:      4980
  Failed:          20
  Success Rate:    99.60%

Throughput:
  Duration:        1m0.123s
  Requests/sec:    83.15

Response Latency:
  Samples:         4980
  p50:             12.5ms
  p95:             45.2ms
  p99:             98.7ms
  min:             3.1ms
  max:             156.3ms
  avg:             18.4ms

Error Breakdown:
  status_503: 15
  timeout: 5
```

## Performance Expectations

For production systems, expect:
- p50 latency < 50ms (Redis cache hit)
- p95 latency < 200ms (database fallback)
- p99 latency < 500ms
- Success rate > 99%

## Prerequisites

1. Running tragge platform services:
   - user-bff (authentication and leaderboard API)
   - Redis (leaderboard cache)
   - PostgreSQL (fallback)

2. Valid user credentials with access to the contest(s)

3. Contest(s) with leaderboard data populated

## Architecture

```
                    ┌─────────────────┐
                    │  Leaderboard    │
                    │  Load Test      │
                    └────────┬────────┘
                             │
              ┌──────────────┴──────────────┐
              │ 500 concurrent HTTP requests│
              └──────────────┬──────────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │    user-bff     │
                    │ /api/user/      │
                    │ leaderboard     │
                    └────────┬────────┘
                             │
              ┌──────────────┴──────────────┐
              │                             │
              ▼                             ▼
      ┌───────────┐                ┌───────────────┐
      │   Redis   │                │  PostgreSQL   │
      │ (primary) │                │  (fallback)   │
      └───────────┘                └───────────────┘
```

## Troubleshooting

### High error rate
- Check user-bff logs for errors
- Verify Redis is running and accessible
- Check database connection pool exhaustion

### High latency
- Check Redis response times
- Monitor database query performance
- Review user-bff resource utilization

### Connection refused
- Verify user-bff is running
- Check port availability
- Review firewall/network configuration
