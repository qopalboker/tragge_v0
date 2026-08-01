#!/bin/bash
# Create all required Kafka topics with correct configuration
# Run this BEFORE starting application services
#
# This script is idempotent - safe to run multiple times.
# Existing topics will not be modified; only missing topics are created.
#
# Usage:
#   ./scripts/kafka/create-topics.sh
#   KAFKA_BROKERS=redpanda:9092 ./scripts/kafka/create-topics.sh
#
# See also: scripts/kafka-setup.sh for a more advanced version with
# retention configuration and multiple CLI backend support.

set -euo pipefail

BROKERS="${KAFKA_BROKERS:-localhost:9092}"
REPLICATION="${REPLICATION_FACTOR:-1}"

# Wait for Kafka/Redpanda to be ready
wait_for_broker() {
    local max_attempts=30
    local attempt=1
    echo "Waiting for Kafka broker at $BROKERS..."
    while [ $attempt -le $max_attempts ]; do
        if rpk topic list -X brokers="$BROKERS" 2>/dev/null; then
            echo "Broker is ready."
            return 0
        fi
        echo "  Attempt $attempt/$max_attempts - broker not ready yet..."
        sleep 2
        attempt=$((attempt + 1))
    done
    echo "ERROR: Broker not ready after $max_attempts attempts"
    return 1
}

# Topic definitions: "name:partitions:replication"
#
# Partition counts match the established configuration in scripts/kafka-setup.sh
# and are validated by tests/integration/kafka_topics_test.go.
#
# Partitioning strategy:
#   16 partitions - High-throughput trading topics (orders, fills, ticks, etc.)
#    8 partitions - Settlement topics (lower throughput)
#    4 partitions - Notification topics
#    1 partition  - Control topics requiring global ordering (contests)
topics=(
    # =========================================================================
    # HIGH-THROUGHPUT TRADING TOPICS (16 partitions)
    # Partitioned by contest_id or symbol for local ordering
    # =========================================================================
    "orders.v1:16:$REPLICATION"
    "ticks.v1:16:$REPLICATION"
    "fills.v1:16:$REPLICATION"
    "positions.v1:16:$REPLICATION"
    "order_acks.v1:16:$REPLICATION"
    "pnl_deltas.v1:16:$REPLICATION"
    "order_cancelled.v1:16:$REPLICATION"
    "position_closed.v1:16:$REPLICATION"

    # =========================================================================
    # TRADING ENGINE COMMAND TOPICS (16 partitions)
    # User-initiated mutations routed to trading-engine
    # =========================================================================
    "close_positions.v1:16:$REPLICATION"
    "cancel_orders.v1:16:$REPLICATION"
    "modify_tpsl.v1:16:$REPLICATION"

    # =========================================================================
    # ALERTS (16 partitions, fire-and-forget)
    # =========================================================================
    "alerts.v1:16:$REPLICATION"

    # =========================================================================
    # SETTLEMENT TOPICS (8 partitions)
    # =========================================================================
    "settlement_requests.v1:8:$REPLICATION"
    "settlement_events.v1:8:$REPLICATION"
    "contest_close_positions.v1:8:$REPLICATION"
    "contest_cancel_orders.v1:8:$REPLICATION"

    # =========================================================================
    # NOTIFICATION TOPICS (4 partitions)
    # =========================================================================
    "notifications.v1:4:$REPLICATION"

    # =========================================================================
    # CONTROL TOPICS (1 partition for global ordering)
    # =========================================================================
    "contests.v1:1:$REPLICATION"
)

wait_for_broker

echo "Creating Kafka topics on $BROKERS..."
echo ""

created=0
skipped=0
failed=0

for topic_config in "${topics[@]}"; do
    IFS=':' read -r topic partitions replication <<< "$topic_config"
    printf "  %-35s (partitions=%-2s, replication=%s) ... " "$topic" "$partitions" "$replication"

    if rpk topic create "$topic" \
        -X brokers="$BROKERS" \
        --partitions "$partitions" \
        --replicas "$replication" 2>/dev/null; then
        echo "CREATED"
        created=$((created + 1))
    else
        # Topic likely already exists - verify it
        if rpk topic list -X brokers="$BROKERS" 2>/dev/null | grep -q "^$topic"; then
            echo "EXISTS"
            skipped=$((skipped + 1))
        else
            echo "FAILED"
            failed=$((failed + 1))
        fi
    fi
done

echo ""
echo "Topic creation complete: $created created, $skipped already existed, $failed failed"
echo ""

# List all topics for verification
echo "Current topics:"
rpk topic list -X brokers="$BROKERS" 2>/dev/null || echo "(could not list topics)"