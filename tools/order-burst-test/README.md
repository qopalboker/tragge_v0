# Order Burst Load Test

A load testing tool for measuring order processing performance under high-throughput, multi-contest scenarios.

## Features

- Achieves target orders per second across multiple contests
- Simulates realistic multi-contest trading environment
- Measures order-to-fill latency (p50, p95, p99)
- Per-contest metrics breakdown
- Rate distribution tracking
- Real-time progress reporting

## Usage

```bash
# Build the tool
go build -o order-burst-test .

# Run with default settings (100 orders/sec across 10 contests)
./order-burst-test \
  -contest-ids "contest-1,contest-2,contest-3,contest-4,contest-5,contest-6,contest-7,contest-8,contest-9,contest-10"

# Run with custom settings
./order-burst-test \
  -orders-per-second 100 \
  -num-contests 10 \
  -users-per-contest 5 \
  -contest-ids "c1,c2,c3,c4,c5,c6,c7,c8,c9,c10" \
  -duration 120s
```

## Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-orders-per-second` | 100 | Target orders per second |
| `-num-contests` | 10 | Number of contests to spread orders across |
| `-users-per-contest` | 5 | Number of users per contest |
| `-contest-ids` | (required) | Contest IDs (comma-separated) |
| `-duration` | 60s | Test duration |
| `-ramp-up` | 10s | Ramp-up duration |
| `-symbols` | AAPL,GOOGL,MSFT,AMZN,TSLA | Trading symbols |
| `-user-bff` | http://localhost:8081 | User BFF URL |
| `-trade-bff-api` | http://localhost:8082 | Trade BFF API URL |
| `-trade-bff-ws` | ws://localhost:8082 | Trade BFF WebSocket URL |
| `-email-prefix` | burst | Email prefix for test users |
| `-password` | bursttest123! | Password for test users |

## Test Scenario: 100 Orders/Second Across 10 Contests

This tool is designed for the scenario where orders are submitted at 100/sec distributed across multiple contests:

```bash
./order-burst-test \
  -orders-per-second 100 \
  -num-contests 10 \
  -contest-ids "$(echo contest-{1..10} | tr ' ' ',')" \
  -duration 60s
```

## Output

```
=== Order Burst Load Test Results ===

Order Metrics:
  Orders Sent:     6000
  Fills Received:  5850
  Order Errors:    0
  WS Errors:       0
  Fill Rate:       97.5%

Throughput:
  Duration:        1m0.123s
  Target Rate:     100 orders/sec
  Actual Rate:     99.80 orders/sec
  Achievement:     99.8%

Rate Distribution:
  Avg Rate:        99.50 orders/sec
  Min Rate:        85.00 orders/sec
  Max Rate:        115.00 orders/sec

Order-to-Fill Latency:
  Samples:         5850
  p50:             15.5ms
  p95:             55.2ms
  p99:             120.7ms
  min:             5.1ms
  max:             256.3ms
  avg:             22.4ms

Per-Contest Breakdown:
+----------------+----------+--------+----------+
| Contest        | Orders   | Fills  | Rate     |
+----------------+----------+--------+----------+
| contest-1      | 600      | 585    | 97.5%    |
| contest-2      | 600      | 590    | 98.3%    |
| contest-3      | 600      | 580    | 96.7%    |
| ...            | ...      | ...    | ...      |
+----------------+----------+--------+----------+
```

## Performance Expectations

For production systems targeting 100 orders/sec:
- Achievement rate > 95%
- p50 latency < 50ms
- p95 latency < 200ms
- p99 latency < 500ms
- Fill rate > 95%

## Architecture

```
                    ┌─────────────────┐
                    │  Order Burst    │
                    │  Load Test      │
                    └────────┬────────┘
                             │
              ┌──────────────┴──────────────┐
              │ 100 orders/sec distributed  │
              │ across 10 contests          │
              └──────────────┬──────────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
         ▼                   ▼                   ▼
  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
  │ Contest 1   │     │ Contest 2   │     │ Contest N   │
  │ (10 ops/s)  │     │ (10 ops/s)  │     │ (10 ops/s)  │
  └──────┬──────┘     └──────┬──────┘     └──────┬──────┘
         │                   │                   │
         └───────────────────┼───────────────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │   trade-bff     │
                    │  (API + WS)     │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  orders.v1      │
                    │  (Kafka topic)  │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │ trading-engine  │
                    │ (partitioned)   │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │   fills.v1      │
                    │  (Kafka topic)  │
                    └─────────────────┘
```

## Prerequisites

1. Running tragge platform services:
   - user-bff (authentication)
   - trade-bff (order API and WebSocket)
   - trading-engine (order processing)
   - PostgreSQL, Redis, and Redpanda

2. Multiple contests created and in "running" or "registration_open" status

3. Sufficient system resources:
   - Increase file descriptor limits for many connections
   - Ensure adequate network bandwidth

## System Tuning (Linux)

```bash
# Increase file descriptor limit
ulimit -n 65535

# Increase port range
sudo sysctl -w net.ipv4.ip_local_port_range="1024 65535"

# Enable TCP reuse
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
```

## Troubleshooting

### Rate not achieved
- Check trading-engine capacity
- Monitor Kafka lag
- Review network bandwidth
- Increase users per contest

### High latency
- Check trading-engine throughput
- Monitor Kafka consumer lag
- Review database query performance

### Low fill rate
- Verify trading-engine is running
- Check Kafka connectivity
- Monitor for order rejections
