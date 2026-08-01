# WebSocket Load Test Tool

A Go-based load testing tool for the tragge trading platform WebSocket connections.

## Features

- Opens multiple concurrent WebSocket connections (supports 1000+ for storm testing)
- Reuses a single authentication token across all connections
- Built-in connection rate limiting (50 concurrent) to prevent overwhelming servers
- Measures:
  - **Connection latency** (p50/p95/p99)
  - **Messages received per second** (throughput)
  - **Tick delivery latency** (server timestamp to client receive)
  - **Dropped/timeout/error counts**
- Ramp-up period to avoid connection storms
- Graceful shutdown with Ctrl+C

## Prerequisites

1. Go 1.22+ installed
2. A running tragge platform:
   - user-bff (default: `localhost:8081`)
   - trade-bff (default: `localhost:8082`)
3. A valid user account (email/password)
4. A valid contest ID

## Installation

```bash
cd tools/ws-load-test
go mod download
go build -o ws-load-test .
```

Or run directly:

```bash
go run . [flags]
```

## Usage

### Basic Usage

```bash
./ws-load-test \
  -n 100 \
  -email test@example.com \
  -password password123 \
  -contest-id 550e8400-e29b-41d4-a716-446655440000
```

### Running with 1000 Connections

For 1000 connections, increase file descriptor limits first:

```bash
# Check current limit
ulimit -n

# Increase limit for current session (requires 1000+ available)
ulimit -n 4096
```

Then run:

```bash
./ws-load-test \
  -n 1000 \
  -email test@example.com \
  -password password123 \
  -contest-id 550e8400-e29b-41d4-a716-446655440000 \
  -duration 120s \
  -ramp-up 30s
```

### WebSocket Storm Test (1000 Concurrent Connections)

This scenario tests the platform's ability to handle 1000 concurrent WebSocket connections subscribing to price updates:

```bash
# Production-ready WebSocket storm test
./ws-load-test \
  -n 1000 \
  -email loadtest@example.com \
  -password loadtestpass123 \
  -contest-id YOUR_CONTEST_ID \
  -duration 120s \
  -ramp-up 30s \
  -compression true
```

**Expected Performance Targets:**
- Connection success rate: > 99%
- Connection latency p95: < 500ms
- Message throughput: > 900 msgs/sec across all connections
- Tick delivery latency p95: < 100ms
- Error rate: < 1%

### All Options

| Flag | Default | Description |
|------|---------|-------------|
| `-n` | 100 | Number of WebSocket connections to open |
| `-email` | (required) | Login email address |
| `-password` | (required) | Login password |
| `-contest-id` | (required) | Contest ID for WebSocket connections |
| `-user-bff` | `http://localhost:8081` | User BFF base URL |
| `-trade-bff-ws` | `ws://localhost:8082` | Trade BFF WebSocket base URL |
| `-duration` | 60s | Test duration |
| `-ramp-up` | 10s | Ramp-up duration to open all connections |
| `-connect-timeout` | 10s | Connection timeout per WebSocket |
| `-read-timeout` | 65s | Read timeout (should be > server ping interval of 54s) |

### Example with Custom URLs

```bash
./ws-load-test \
  -n 500 \
  -email loadtest@example.com \
  -password testpassword \
  -contest-id abc123 \
  -user-bff http://192.168.1.100:8081 \
  -trade-bff-ws ws://192.168.1.100:8082 \
  -duration 5m \
  -ramp-up 60s
```

## Sample Output

```
=== WebSocket Load Test ===
Connections:   1000
Contest ID:    550e8400-e29b-41d4-a716-446655440000
Duration:      2m0s
Ramp-up:       30s
User BFF:      http://localhost:8081
Trade BFF WS:  ws://localhost:8082

Authenticating...
Authentication successful!

Opening 1000 connections over 30s...
Connected: 998/1000 (99.8%)

Running load test for 2m0s...
[5s] Messages: 4985 (997.0/s), Errors: 0, Timeouts: 0, Disconnects: 0
[10s] Messages: 9970 (997.0/s), Errors: 0, Timeouts: 0, Disconnects: 0
...

Closing connections...

=== Load Test Results ===

Connection Metrics:
  Attempted:     1000
  Successful:    998
  Failed:        2
  Latency p50:   45ms
  Latency p95:   120ms
  Latency p99:   250ms

Message Throughput:
  Duration:      2m0s
  Total msgs:    119640
  Total bytes:   23.45 MB
  Rate:          997.00 msgs/sec
  Throughput:    200.12 KB/sec

Tick Delivery Latency (server timestamp to client receive):
  Samples:       119640
  p50:           2.145ms
  p95:           5.832ms
  p99:           12.456ms

Error Metrics:
  Read errors:   0
  Read timeouts: 0
  Disconnects:   2
  Ping failures: 0

Success Rate:    99.99%
```

## Understanding the Metrics

### Connection Metrics

- **Latency p50/p95/p99**: Time to establish WebSocket connection (including TLS handshake if applicable)

### Message Throughput

- **Rate**: Total messages received per second across all connections
- **Throughput**: Bytes per second received

### Tick Delivery Latency

- Measures end-to-end latency from server timestamp to client receive
- Only tracked for `tick_snapshot` messages that include a server timestamp
- Accounts for network latency, server processing, and client receive time
- Note: Requires synchronized clocks between client and server for accuracy

### Error Metrics

- **Read errors**: Unexpected errors reading from WebSocket
- **Read timeouts**: No message received within read timeout period
- **Disconnects**: Clean or abnormal connection closures
- **Ping failures**: Failed ping/pong keepalive (if applicable)

## Tips for High Connection Counts

### System Tuning (Linux)

```bash
# Increase file descriptor limit
echo "* soft nofile 65535" | sudo tee -a /etc/security/limits.conf
echo "* hard nofile 65535" | sudo tee -a /etc/security/limits.conf

# Increase port range for outgoing connections
sudo sysctl -w net.ipv4.ip_local_port_range="1024 65535"

# Enable TCP reuse
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
```

### Server-Side Considerations

For 1000+ connections, ensure the trade-bff server is configured appropriately:

1. Increase file descriptor limits on the server
2. Monitor memory usage (each connection uses ~4KB buffers)
3. Consider Kafka consumer group throughput
4. Monitor Redis pub/sub subscription limits

### Best Practices

1. **Start small**: Begin with 10-100 connections to establish baseline
2. **Increase gradually**: Double connections each run until you find limits
3. **Monitor server**: Watch CPU, memory, and network on the server during tests
4. **Use ramp-up**: Avoid connection storms with appropriate ramp-up duration
5. **Run multiple times**: Results can vary; run multiple tests for consistency

## Troubleshooting

### "too many open files"

Increase file descriptor limit:
```bash
ulimit -n 4096
```

### "connection refused"

- Ensure trade-bff is running and accessible
- Check firewall rules
- Verify the WebSocket URL is correct

### "401 Unauthorized"

- Verify email/password credentials
- Ensure user-bff is running and accessible
- Check that the user account exists

### "contest not found" or similar WebSocket errors

- Verify the contest ID exists in the database
- Ensure the contest is in a state that allows connections (e.g., "running")

### High latency / timeouts

- Check network connectivity between client and server
- Monitor server CPU and memory usage
- Reduce connection count or increase ramp-up time

## Integration with Makefile

Add to the root Makefile:

```makefile
.PHONY: load-test-ws
load-test-ws:
	cd tools/ws-load-test && go run . \
		-n $(or $(N),100) \
		-email $(EMAIL) \
		-password $(PASSWORD) \
		-contest-id $(CONTEST_ID) \
		-duration $(or $(DURATION),60s)
```

Usage:
```bash
make load-test-ws N=1000 EMAIL=test@example.com PASSWORD=pass123 CONTEST_ID=abc123 DURATION=2m
```
