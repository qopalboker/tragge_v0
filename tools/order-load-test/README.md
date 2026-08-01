# Order Load Test

A load testing tool for measuring order-to-fill latency in the tragge trading platform.

## Features

- Simulates N users placing orders concurrently
- Measures order-to-fill latency (p50, p95, p99)
- Supports configurable test duration and order rate
- Automatic user registration and contest joining
- WebSocket connection for real-time fill events

## Usage

```bash
# Build the tool
go build -o order-load-test .

# Run with default settings (10 users, 60 second duration)
./order-load-test -contest-id <your-contest-id>

# Run with custom settings
./order-load-test \
  -contest-id <contest-id> \
  -users 50 \
  -duration 120s \
  -orders-per-user 20 \
  -order-interval 500ms \
  -symbols "AAPL,GOOGL,MSFT"
```

## Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-users` | 10 | Number of simulated users |
| `-contest-id` | (required) | Contest ID to trade in |
| `-duration` | 60s | Test duration |
| `-orders-per-user` | 10 | Orders each user places |
| `-order-interval` | 1s | Interval between orders |
| `-symbols` | AAPL,GOOGL,MSFT,AMZN,TSLA | Trading symbols |
| `-user-bff` | http://localhost:8081 | User BFF URL |
| `-trade-bff-api` | http://localhost:8082 | Trade BFF API URL |
| `-trade-bff-ws` | ws://localhost:8082 | Trade BFF WebSocket URL |
| `-email-prefix` | loadtest | Email prefix for test users |
| `-password` | loadtest123! | Password for test users |

## Prerequisites

1. The trading platform services should be running:
   - user-bff (authentication)
   - trade-bff (order API and WebSocket)
   - trading-engine (order processing)
   - PostgreSQL, Redis, and Redpanda

2. A contest should exist and be in "running" or "registration_open" status

## Output

The tool outputs metrics including:

- **Order Metrics**: Orders sent, fills received, error counts
- **Throughput**: Orders per second
- **Latency**: Order-to-fill latency percentiles (p50, p95, p99, min, max, avg)

Example output:

```
=== Order Load Test Results ===

Order Metrics:
  Orders Sent:     500
  Fills Received:  487
  Order Errors:    0
  WS Errors:       0
  Fill Rate:       97.4%

Throughput:
  Duration:        60.5s
  Orders/sec:      8.26

Order-to-Fill Latency:
  Samples:         487
  p50:             12.5ms
  p95:             45.2ms
  p99:             98.7ms
  min:             3.1ms
  max:             156.3ms
  avg:             18.4ms
```

## Architecture

```
                    ┌─────────────┐
                    │  Order      │
                    │  Load Test  │
                    └──────┬──────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
  ┌───────────┐     ┌───────────┐     ┌───────────┐
  │  user-bff │     │ trade-bff │     │ trade-bff │
  │  (Auth)   │     │  (API)    │     │   (WS)    │
  └───────────┘     └─────┬─────┘     └─────┬─────┘
                          │                 │
                          ▼                 │
                    ┌───────────┐           │
                    │  orders   │           │
                    │  topic    │           │
                    └─────┬─────┘           │
                          │                 │
                          ▼                 │
                    ┌───────────┐           │
                    │  trading  │           │
                    │  engine   │           │
                    └─────┬─────┘           │
                          │                 │
                          ▼                 │
                    ┌───────────┐           │
                    │  fills    │───────────┘
                    │  topic    │  (FillEvent → WS)
                    └───────────┘
```

## Testing Scenarios

### Basic Performance Test
```bash
./order-load-test -contest-id <id> -users 10 -duration 60s
```

### High Load Test
```bash
./order-load-test -contest-id <id> -users 100 -duration 300s -order-interval 100ms
```

### Sustained Load Test
```bash
./order-load-test -contest-id <id> -users 50 -duration 3600s -orders-per-user 1000
```

## Expected Performance Targets

When running with default settings (10 users, 60s duration), the platform should meet these targets:

- Order submission success rate: > 99%
- Order-to-acknowledgment latency p99: < 200ms
- Order-to-fill latency p99: < 500ms
- Fill rate: > 95%
- No order processing errors under normal load

For high load tests (100 users, 500+ orders/sec):

- Order submission success rate: > 98%
- Order-to-fill latency p99: < 1s
- Fill rate: > 90%
- Graceful degradation without data loss

## Troubleshooting

### No fills received
- Verify trading-engine is running and connected to Kafka
- Check that the contest is in "running" status
- Ensure users have joined the contest

### Authentication failures
- Verify user-bff is accessible
- Check that the email/password combination is valid

### WebSocket connection errors
- Verify trade-bff is accessible
- Check WebSocket URL (ws:// vs wss://)
- Verify contest_id is valid
