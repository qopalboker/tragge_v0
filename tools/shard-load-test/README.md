# Shard Load Test Tool

A shard-aware load testing tool for the tragge trading platform. This tool simulates distributed trading across multiple shards and measures per-shard performance.

## Features

- Creates contests distributed across shards
- Simulates users trading on each shard
- Measures per-shard order throughput and latency
- Tests shard affinity and routing
- Reports aggregated and per-shard metrics

## Usage

```bash
# Basic usage with defaults (4 shards, 10 users per shard)
go run . -duration 60s

# Custom configuration
go run . \
  -shards 4 \
  -users-per-shard 25 \
  -contests-per-shard 2 \
  -duration 120s \
  -orders-per-user 20 \
  -order-interval 500ms

# With custom endpoints
go run . \
  -user-bff http://localhost:8081 \
  -trade-bff-api http://localhost:8082 \
  -trade-bff-ws ws://localhost:8082 \
  -shard-router http://localhost:8090

# Verbose mode for debugging
go run . -verbose -duration 30s
```

## Configuration Options

| Flag | Default | Description |
|------|---------|-------------|
| `-shards` | 4 | Number of shards to test |
| `-contests-per-shard` | 1 | Number of contests per shard |
| `-users-per-shard` | 10 | Number of simulated users per shard |
| `-orders-per-user` | 10 | Number of orders each user places |
| `-order-interval` | 1s | Interval between orders |
| `-duration` | 60s | Test duration |
| `-symbols` | AAPL,GOOGL,MSFT,AMZN,TSLA | Trading symbols |
| `-user-bff` | http://localhost:8081 | User BFF base URL |
| `-trade-bff-api` | http://localhost:8082 | Trade BFF API URL |
| `-trade-bff-ws` | ws://localhost:8082 | Trade BFF WebSocket URL |
| `-shard-router` | http://localhost:8090 | Shard router URL |
| `-email-prefix` | shardtest | Email prefix for test users |
| `-password` | shardtest123! | Password for test users |
| `-connect-timeout` | 10s | WebSocket connection timeout |
| `-read-timeout` | 65s | WebSocket read timeout |
| `-verbose` | false | Enable verbose logging |

## Output

The tool produces:

1. **Global Statistics**: Aggregate metrics across all shards
   - Total orders sent
   - Total fills received
   - Overall orders/second
   - Overall fill rate

2. **Per-Shard Statistics**: Metrics for each individual shard
   - Orders sent
   - Fills received
   - Error counts
   - Orders/second
   - Fill rate

3. **Per-Shard Latency**: Order-to-fill latency per shard
   - p50, p95, p99 percentiles
   - min, max, average

## Example Output

```
=== Shard Load Test Results ===

Global Statistics:
  Duration:          60.012s
  Total Orders:      800
  Total Fills:       792
  Order Errors:      8
  WS Errors:         0
  Orders/sec:        13.33
  Fill Rate:         99.0%

Per-Shard Statistics:
+---------+----------+--------+--------+----------+-----------+
| Shard   | Orders   | Fills  | Errors | Rate     | Orders/s  |
+---------+----------+--------+--------+----------+-----------+
| Shard 0 | 200      | 198    | 2      | 99.0%    | 3.33      |
| Shard 1 | 200      | 198    | 2      | 99.0%    | 3.33      |
| Shard 2 | 200      | 198    | 2      | 99.0%    | 3.33      |
| Shard 3 | 200      | 198    | 2      | 99.0%    | 3.33      |
+---------+----------+--------+--------+----------+-----------+

Per-Shard Latency (Order-to-Fill):
+---------+-----------+-----------+-----------+-----------+-----------+
| Shard   | p50       | p95       | p99       | min       | max       |
+---------+-----------+-----------+-----------+-----------+-----------+
| Shard 0 | 12.5ms    | 45.2ms    | 98.7ms    | 2.1ms     | 125.3ms   |
| Shard 1 | 13.1ms    | 48.9ms    | 102.4ms   | 2.3ms     | 130.1ms   |
| Shard 2 | 11.9ms    | 42.1ms    | 95.2ms    | 1.9ms     | 118.7ms   |
| Shard 3 | 12.8ms    | 46.5ms    | 99.1ms    | 2.0ms     | 122.5ms   |
+---------+-----------+-----------+-----------+-----------+-----------+
```

## Architecture

The tool follows this flow:

1. **Contest Creation**: Creates contests distributed across shards
2. **User Registration**: Creates and authenticates users for each shard
3. **WebSocket Connection**: Establishes WebSocket connections per user
4. **Order Placement**: Each user places orders on their assigned shard
5. **Fill Tracking**: Tracks fill events and calculates latency
6. **Results Aggregation**: Compiles per-shard and global statistics

## Testing Scenarios

### Basic Shard Distribution Test
```bash
go run . -shards 4 -users-per-shard 10 -duration 30s
```

### High Load Test
```bash
go run . -shards 4 -users-per-shard 100 -orders-per-user 50 -order-interval 100ms -duration 120s
```

### Uneven Shard Test
Test with different user counts by running multiple instances targeting specific shards.

### Latency Sensitivity Test
```bash
go run . -shards 4 -users-per-shard 5 -orders-per-user 100 -order-interval 50ms -duration 60s
```
