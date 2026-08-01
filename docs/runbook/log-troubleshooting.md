# Log Troubleshooting Guide

This guide provides LogQL queries for common troubleshooting scenarios in the tragge trading platform.

## Quick Reference

### Access Logs
- **Grafana Explore**: http://localhost:3000/explore
- **Select Datasource**: Loki
- **Log Retention**: 30 days (error logs: 60 days)

### Common Label Selectors
```logql
{service="user_bff"}        # User BFF service
{service="trade_bff"}       # Trade BFF service
{service="admin_bff"}       # Admin BFF service
{service="trading_engine"}  # Trading engine
{service="market_ingestor"} # Market data ingestor
{service="leaderboard_worker"} # Leaderboard worker
{level="error"}             # Error logs only
{level=~"error|warn"}       # Errors and warnings
```

---

## Service Health Troubleshooting

### View All Errors for a Service
```logql
{service="user_bff", level="error"}
```

### Error Rate Over Time
```logql
sum(rate({level="error"} |= `` [5m])) by (service)
```

### Recent Service Restarts
```logql
{} |~ "(?i)(starting|started|service started|initializing)"
| line_format "{{.service}}: {{.msg}}"
```

### Panic and Stack Traces
```logql
{} |~ "(?i)(panic|runtime error|stack trace|goroutine)"
```

### Service Startup Issues
```logql
{} |~ "(?i)(failed to|unable to|cannot|error initializing)"
| line_format "{{.service}}: {{.msg}}"
```

---

## Authentication Issues

### Failed Login Attempts
```logql
{service="user_bff"} |~ "(?i)(invalid credentials|invalid password|user not found)"
| json | line_format "IP: {{.remote_addr}} - {{.msg}}"
```

### Failed Login Attempts by IP (Potential Brute Force)
```logql
{service="user_bff"}
|~ "(?i)(invalid credentials|invalid password)"
| json
| line_format "{{.remote_addr}}"
| label_format ip={{.remote_addr}}
```

### Rate Limit Events
```logql
{service="user_bff"} |~ "(?i)(rate limit|too many requests)"
| json | line_format "IP: {{.remote_addr}} Path: {{.path}}"
```

### JWT Token Issues
```logql
{} |~ "(?i)(jwt|token)" |~ "(?i)(expired|invalid|malformed)"
```

### Session Management Issues
```logql
{service="user_bff"} |~ "(?i)(session)"
| json | line_format "{{.user_id}}: {{.msg}}"
```

---

## Trading Issues

### Order Processing Errors
```logql
{service="trading_engine", level="error"}
| json
| line_format "Order: {{.order_id}} User: {{.user_id}} - {{.msg}}"
```

### Rejected Orders
```logql
{service="trading_engine"} |~ "(?i)(rejected|reject)"
| json
| line_format "Order: {{.order_id}} Reason: {{.msg}}"
```

### Order Flow for Specific User
```logql
{service=~"trade_bff|trading_engine"} |= "user_id"
| json
| user_id = "USER_ID_HERE"
| line_format "[{{.service}}] {{.msg}}"
```

### Order Flow for Specific Order
```logql
{} |= "ORDER_ID_HERE"
| json
| line_format "[{{.service}}] {{.msg}}"
```

### Slow Order Processing
```logql
{service="trading_engine"}
| json
| duration > 100ms
| line_format "Order: {{.order_id}} Duration: {{.duration}}"
```

### Market Data Issues
```logql
{service="market_ingestor"}
|~ "(?i)(error|timeout|failed|stale)"
| json
| line_format "{{.symbol}}: {{.msg}}"
```

### Provider Failover Events
```logql
{service="market_ingestor"} |~ "(?i)(failover|fallback|switching)"
```

### Price Updates for Symbol
```logql
{service="market_ingestor"} |= "AAPL"
| json
| line_format "{{.symbol}}: bid={{.bid}} ask={{.ask}}"
```

---

## Database Issues

### Database Connection Errors
```logql
{} |~ "(?i)(database|pg|sql|postgres)" |~ "(?i)(error|failed|connection)"
```

### Slow Queries
```logql
{} |~ "(?i)(slow query|query took)"
| json
| line_format "{{.service}}: {{.msg}}"
```

### Connection Pool Issues
```logql
{} |~ "(?i)(connection pool|pool exhausted|no available connections)"
```

### Transaction Errors
```logql
{} |~ "(?i)(transaction|tx|commit|rollback)" |~ "(?i)(error|failed)"
```

---

## Redis/Cache Issues

### Redis Errors
```logql
{} |~ "(?i)redis" |~ "(?i)(error|failed|timeout)"
```

### Cache Miss Patterns
```logql
{} |~ "(?i)(cache miss|cache expired|not found in cache)"
```

### Session Storage Issues
```logql
{service="user_bff"} |~ "(?i)(session)" |~ "(?i)(error|failed)"
```

---

## Kafka/Message Queue Issues

### Kafka Consumer Errors
```logql
{} |~ "(?i)(kafka|redpanda|consumer)" |~ "(?i)(error|failed)"
```

### Message Processing Failures
```logql
{} |~ "(?i)(message|event)" |~ "(?i)(failed|error|unable)"
| json
| line_format "{{.service}}: {{.msg}}"
```

### Consumer Lag Warnings
```logql
{} |~ "(?i)(consumer lag|behind|backlog)"
```

---

## WebSocket Issues

### WebSocket Connection Errors
```logql
{service="trade_bff"} |~ "(?i)(websocket|ws)" |~ "(?i)(error|failed)"
```

### Client Disconnections
```logql
{service="trade_bff"} |~ "(?i)(disconnect|connection closed)"
| json
| line_format "User: {{.user_id}} - {{.msg}}"
```

### WebSocket Message Errors
```logql
{service="trade_bff"} |~ "(?i)(message)" |~ "(?i)(error|invalid)"
```

---

## Contest and Leaderboard Issues

### Contest State Changes
```logql
{service="admin_bff"} |~ "(?i)contest"
| json
| line_format "Contest: {{.contest_id}} - {{.msg}}"
```

### Leaderboard Processing Errors
```logql
{service="leaderboard_worker", level="error"}
| json
| line_format "Contest: {{.contest_id}} - {{.msg}}"
```

### Payout Issues
```logql
{} |~ "(?i)(payout|prize|distribution)"
| json
| line_format "{{.service}}: {{.msg}}"
```

### Contest Join Issues
```logql
{service="user_bff"} |~ "(?i)contest" |~ "(?i)(join|register)"
| json
| line_format "User: {{.user_id}} Contest: {{.contest_id}} - {{.msg}}"
```

---

## User Activity Tracking

### All Activity for Specific User
```logql
{} | json | user_id = "USER_ID_HERE"
| line_format "[{{.service}}] {{.msg}}"
```

### User Authentication History
```logql
{service="user_bff"} |~ "(?i)(login|register|logged)"
| json
| user_id = "USER_ID_HERE"
| line_format "{{.ts}}: {{.msg}}"
```

### User Trading Activity
```logql
{service=~"trade_bff|trading_engine"}
| json
| user_id = "USER_ID_HERE"
| line_format "[{{.service}}] {{.msg}}"
```

---

## Request Tracing

### Find Logs by Trace ID
```logql
{} |= "TRACE_ID_HERE"
| json
| line_format "[{{.service}}] {{.msg}}"
```

### Find Logs by Request ID
```logql
{} |= "REQUEST_ID_HERE"
| json
| line_format "[{{.service}}] {{.msg}}"
```

### Slow Requests (>1 second)
```logql
{}
| json
| duration > 1s
| line_format "{{.service}} {{.method}} {{.path}}: {{.duration}}"
```

### Failed HTTP Requests (5xx)
```logql
{}
| json
| status >= 500
| line_format "{{.service}} {{.method}} {{.path}}: {{.status}}"
```

### 4xx Client Errors
```logql
{}
| json
| status >= 400
| status < 500
| line_format "{{.service}} {{.method}} {{.path}}: {{.status}} - {{.msg}}"
```

---

## Alerting Queries

### High Error Rate Alert
```logql
sum(rate({level="error"} |= `` [5m])) by (service) > 0.5
```

### Authentication Failure Spike
```logql
sum(count_over_time(
  {service="user_bff"} |~ "(?i)(invalid credentials|invalid password)" [5m]
)) > 50
```

### Service Restart Detection
```logql
sum(count_over_time(
  {} |~ "(?i)(starting|started|service started)" [5m]
)) by (service) > 2
```

### Database Connection Issues
```logql
sum(count_over_time(
  {} |~ "(?i)(database|pg|sql)" |~ "(?i)(error|failed|connection)" [5m]
)) by (service) > 5
```

---

## Performance Analysis

### Request Latency Distribution
```logql
{}
| json
| line_format "{{.duration}}"
| unwrap duration
| histogram_over_time(1m)
```

### Top 10 Slowest Requests
```logql
topk(10,
  {}
  | json
  | unwrap duration
  | avg_over_time(5m) by (service, path)
)
```

### Error Rate by Endpoint
```logql
sum by (service, path) (
  rate({level="error"} | json [5m])
)
```

---

## Useful Patterns

### Filter by Time Range in Log Message
```logql
{service="trading_engine"}
| json
| ts >= "2024-01-15T10:00:00Z"
| ts <= "2024-01-15T11:00:00Z"
```

### Exclude Health Checks
```logql
{service="user_bff", level="error"} != "healthz" != "readyz"
```

### Count Logs by Service
```logql
sum by (service) (count_over_time({} [1h]))
```

### Unique Error Messages
```logql
{level="error"}
| json
| line_format "{{.msg}}"
| dedup
```

---

## Tips

1. **Use JSON parsing**: Most queries benefit from `| json` to extract structured fields.

2. **Filter early**: Put label matchers first, then line filters, then JSON parsing.

3. **Use line_format**: Format output for readability with relevant fields.

4. **Time range**: Start with short ranges (15m-1h) for faster results.

5. **Trace correlation**: Click on trace_id links to jump to Tempo traces.

6. **Save queries**: Save frequently used queries in Grafana dashboards.

---

## Related Resources

- [Incident Response Runbook](./incident-response.md)
- [Service Restart Procedures](./service-restart.md)
- [Database Recovery](./database-recovery.md)
- [LogQL Documentation](https://grafana.com/docs/loki/latest/query/)
