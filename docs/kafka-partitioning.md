# Kafka Partitioning Strategy

This document explains the Kafka partitioning strategy used in the Tragge trading platform, including partition key selection, optimal partition counts, and how partitions map to contest shards.

## Overview

Kafka partitioning ensures:
1. **Message Ordering**: Messages with the same key are always delivered to the same partition, maintaining order
2. **Scalability**: Multiple consumers can process messages in parallel across partitions
3. **Load Distribution**: Work is distributed evenly across consumer instances

## Partition Key Selection

### Orders Topic (`orders.v1`)

**Partition Key**: `contest_id`

```go
record := &kgo.Record{
    Topic: "orders.v1",
    Key:   []byte(order.ContestID),  // Partition by contest
    Value: orderJSON,
}
```

**Rationale**:
- All orders for a contest are processed in order by the same trading-engine instance
- Prevents race conditions in position calculations
- Enables horizontal scaling by contest

### Ticks Topic (`ticks.v1`)

**Partition Key**: `symbol`

```go
record := &kgo.Record{
    Topic: "ticks.v1",
    Key:   []byte(symbol),  // Partition by symbol
    Value: tickJSON,
}
```

**Rationale**:
- Ensures per-symbol price ordering
- Allows parallel processing of different symbols
- Each trading-engine instance can subscribe to specific partitions for its symbols

### Other Topics

| Topic | Partition Key | Rationale |
|-------|---------------|-----------|
| `fills.v1` | `contest_id` | Match ordering with orders |
| `positions.v1` | `contest_id` | Contest-local position updates |
| `order_acks.v1` | `contest_id` | Match ordering with orders |
| `pnl.v1` | `contest_id` | Contest-local PnL aggregation |
| `contests.v1` | None (single partition) | Global ordering for contest state |

## Calculating Optimal Partition Count

### Formula

```
partitions = max(
    ceil(target_throughput / throughput_per_partition),
    number_of_consumer_instances
)
```

### Factors to Consider

1. **Throughput Requirements**
   - Orders: ~10,000 orders/second peak
   - Ticks: ~1,000 ticks/second per symbol
   - Fills: ~5,000 fills/second peak

2. **Consumer Parallelism**
   - Each partition can only be consumed by one consumer in a group
   - More partitions = more potential parallelism

3. **Broker Capacity**
   - Each partition adds overhead (file handles, memory)
   - Recommended: 1,000-4,000 partitions per broker

### Recommended Partition Counts

| Topic | Partitions | Justification |
|-------|------------|---------------|
| `orders.v1` | 16 | Supports up to 16 concurrent contests at full load |
| `fills.v1` | 16 | Matches orders for consistency |
| `ticks.v1` | 16 | Supports 16 symbol groups |
| `positions.v1` | 16 | Matches orders for consistency |
| `order_acks.v1` | 16 | Matches orders for consistency |
| `pnl.v1` | 16 | Matches orders for consistency |
| `contests.v1` | 1 | Low volume, needs global ordering |

## Partition Assignment

### How Kafka Assigns Partitions

Kafka uses the **murmur2** hash function by default:

```
partition = murmur2(key) % num_partitions
```

This ensures:
- Same key always maps to same partition
- Even distribution across partitions
- Deterministic assignment (predictable)

### Calculating Partition for a Contest ID

```go
import "github.com/twmb/franz-go/pkg/kgo"

// Kafka's default partitioner uses murmur2
func getPartition(contestID string, numPartitions int32) int32 {
    // franz-go handles this automatically via kgo.Record.Key
    // The partition is computed as: murmur2(key) % numPartitions
    return int32(murmur2([]byte(contestID))) % numPartitions
}
```

Example partition assignments (16 partitions):
```
contest-001 → partition 7
contest-002 → partition 12
contest-003 → partition 3
...
```

## Mapping Shards to Partitions

### Shard Router Integration

The shard-router service uses partition-based routing to direct contests to specific trading-engine instances.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kafka Cluster                             │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    orders.v1 (16 partitions)              │   │
│  │  P0  P1  P2  P3  P4  P5  P6  P7  P8  P9 P10 P11 P12 P13 P14 P15 │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
        │         │         │         │
        ▼         ▼         ▼         ▼
   ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐
   │Trading  │ │Trading  │ │Trading  │ │Trading  │
   │Engine 0 │ │Engine 1 │ │Engine 2 │ │Engine 3 │
   │(P0-P3)  │ │(P4-P7)  │ │(P8-P11) │ │(P12-P15)│
   └─────────┘ └─────────┘ └─────────┘ └─────────┘
```

### Shard Configuration

The shard configuration in `shard_configs` table maps contests to partitions:

```sql
-- Example: Contest assigned to shard 0 (partitions 0-3)
INSERT INTO shard_configs (contest_id, shard_id, partition_start, partition_end)
VALUES ('contest-uuid', 0, 0, 3);
```

### Consumer Group Assignment

Trading-engine instances use Kafka consumer groups with partition assignment:

```go
// Each trading-engine instance consumes specific partitions
client, err := kgo.NewClient(
    kgo.SeedBrokers(brokers...),
    kgo.ConsumerGroup("trading-engine"),
    kgo.ConsumeTopics("orders.v1", "ticks.v1"),
    // Kafka automatically assigns partitions across group members
)
```

With 4 trading-engine instances and 16 partitions:
- Instance 0: Partitions 0, 1, 2, 3
- Instance 1: Partitions 4, 5, 6, 7
- Instance 2: Partitions 8, 9, 10, 11
- Instance 3: Partitions 12, 13, 14, 15

## Best Practices

### 1. Consistent Key Format

Always use consistent key formats:

```go
// Good: Consistent format
key := []byte(contestID)  // UUID string

// Bad: Inconsistent formats
key := []byte(fmt.Sprintf("contest:%s", contestID))  // Different format
```

### 2. Validate Keys Before Publishing

```go
// Validate contest_id is not empty before publishing
if order.ContestID == "" {
    return errors.New("contest_id is required for partition routing")
}
```

### 3. Monitor Partition Lag

Monitor consumer lag per partition to detect hot partitions:

```bash
# Using rpk (Redpanda)
rpk group describe trading-engine --brokers localhost:9092

# Using kafka-consumer-groups
kafka-consumer-groups.sh --bootstrap-server localhost:9092 \
    --describe --group trading-engine
```

### 4. Rebalancing Considerations

When scaling consumers:
- New consumers trigger rebalancing
- Use cooperative-sticky assignment to minimize disruption
- Plan for brief processing pauses during rebalancing

### 5. Hot Partition Handling

If a contest becomes a hot partition:
1. Consider splitting high-volume contests
2. Monitor partition throughput metrics
3. Scale trading-engine instances to handle load

## Topic Setup

Use the provided setup script to create topics with correct partition counts:

```bash
# Create all topics with default settings
./scripts/kafka-setup.sh localhost:9092

# Customize partition count
KAFKA_PARTITIONS=32 ./scripts/kafka-setup.sh localhost:9092
```

## Monitoring

### Key Metrics

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `kafka_consumer_lag` | Messages behind head | > 10,000 |
| `kafka_partition_messages_in` | Messages per partition | Varies significantly |
| `kafka_consumer_fetch_latency` | Fetch request latency | > 500ms |

### Grafana Dashboard

The Kafka/Redpanda health dashboard includes:
- Partition distribution visualization
- Consumer lag per partition
- Throughput per topic

## Troubleshooting

### Uneven Partition Distribution

**Symptom**: Some partitions have much higher load than others.

**Cause**: Non-uniform key distribution (e.g., most orders for one contest).

**Solution**:
1. Check key distribution with `rpk topic consume orders.v1 --print-key`
2. Consider adding more granular keys if needed
3. Scale consumers to handle peak partition load

### Messages Out of Order

**Symptom**: Fill events arrive before corresponding order acknowledgments.

**Cause**: Different partition keys for related messages.

**Solution**: Ensure all related messages use the same partition key (contest_id).

### High Consumer Lag

**Symptom**: Consumer lag growing continuously.

**Cause**: Consumer processing slower than production rate.

**Solution**:
1. Scale consumer instances
2. Increase partitions (requires topic recreation)
3. Optimize consumer processing logic

## References

- [Kafka Partitioning Design](https://kafka.apache.org/documentation/#design_partitioning)
- [franz-go Partitioner](https://pkg.go.dev/github.com/twmb/franz-go/pkg/kgo#Partitioner)
- [Redpanda Partitioning](https://docs.redpanda.com/current/develop/produce-data/partitioners/)
