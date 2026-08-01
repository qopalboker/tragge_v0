#!/bin/bash
#
# Kafka Topic Setup Script for Tragge Trading Platform
#
# This script creates and configures Kafka topics with appropriate
# partition counts and retention settings for the trading platform.
#
# Usage: ./kafka-setup.sh [broker_address]
#
# Default broker: localhost:9092
#

set -euo pipefail

# Configuration
BROKER="${1:-localhost:9092}"
PARTITIONS="${KAFKA_PARTITIONS:-16}"
REPLICATION="${KAFKA_REPLICATION:-1}"
RETENTION_MS="${KAFKA_RETENTION_MS:-604800000}"  # 7 days default

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Detect which Kafka CLI tools are available
detect_kafka_cli() {
    if command -v rpk &> /dev/null; then
        echo "rpk"
    elif command -v kafka-topics &> /dev/null; then
        echo "kafka"
    elif command -v kafka-topics.sh &> /dev/null; then
        echo "kafka-sh"
    else
        echo "none"
    fi
}

# Create topic using rpk (Redpanda)
create_topic_rpk() {
    local topic=$1
    local partitions=$2
    local replication=$3
    local retention_ms=$4

    log_info "Creating topic: $topic (partitions=$partitions, replication=$replication)"

    if rpk topic list --brokers "$BROKER" | grep -q "^$topic$"; then
        log_warn "Topic $topic already exists, updating configuration..."
        rpk topic alter-config "$topic" --brokers "$BROKER" \
            --set "retention.ms=$retention_ms"
    else
        rpk topic create "$topic" --brokers "$BROKER" \
            --partitions "$partitions" \
            --replicas "$replication" \
            --topic-config "retention.ms=$retention_ms"
    fi
}

# Create topic using kafka-topics (Apache Kafka)
create_topic_kafka() {
    local topic=$1
    local partitions=$2
    local replication=$3
    local retention_ms=$4
    local cmd="kafka-topics"

    if command -v kafka-topics.sh &> /dev/null; then
        cmd="kafka-topics.sh"
    fi

    log_info "Creating topic: $topic (partitions=$partitions, replication=$replication)"

    if $cmd --bootstrap-server "$BROKER" --list | grep -q "^$topic$"; then
        log_warn "Topic $topic already exists, updating configuration..."
        kafka-configs --bootstrap-server "$BROKER" \
            --entity-type topics \
            --entity-name "$topic" \
            --alter \
            --add-config "retention.ms=$retention_ms"
    else
        $cmd --bootstrap-server "$BROKER" \
            --create \
            --topic "$topic" \
            --partitions "$partitions" \
            --replication-factor "$replication" \
            --config "retention.ms=$retention_ms"
    fi
}

# Create topic with Docker fallback
create_topic_docker() {
    local topic=$1
    local partitions=$2
    local replication=$3
    local retention_ms=$4

    log_info "Creating topic: $topic (partitions=$partitions, replication=$replication)"

    # Try redpanda container first
    if docker ps --format '{{.Names}}' | grep -q redpanda; then
        docker exec -it $(docker ps --filter name=redpanda --format '{{.ID}}' | head -1) \
            rpk topic create "$topic" \
            --partitions "$partitions" \
            --replicas "$replication" \
            --topic-config "retention.ms=$retention_ms" 2>/dev/null || \
        docker exec -it $(docker ps --filter name=redpanda --format '{{.ID}}' | head -1) \
            rpk topic alter-config "$topic" \
            --set "retention.ms=$retention_ms"
    else
        log_error "No Redpanda container found"
        return 1
    fi
}

# Main topic creation function
create_topic() {
    local topic=$1
    local partitions=${2:-$PARTITIONS}
    local replication=${3:-$REPLICATION}
    local retention_ms=${4:-$RETENTION_MS}
    local cli_type=$(detect_kafka_cli)

    case $cli_type in
        rpk)
            create_topic_rpk "$topic" "$partitions" "$replication" "$retention_ms"
            ;;
        kafka|kafka-sh)
            create_topic_kafka "$topic" "$partitions" "$replication" "$retention_ms"
            ;;
        none)
            log_warn "No Kafka CLI found, attempting Docker fallback..."
            create_topic_docker "$topic" "$partitions" "$replication" "$retention_ms"
            ;;
    esac
}

# Topic definitions
# Format: topic_name:partitions:retention_ms
#
# Partitioning Strategy:
# - orders.v1: Partitioned by contest_id for contest-local ordering
# - fills.v1: Partitioned by contest_id for contest-local ordering
# - ticks.v1: Partitioned by symbol for per-symbol ordering
# - positions.v1: Partitioned by contest_id for contest-local ordering
# - order_acks.v1: Partitioned by contest_id for contest-local ordering
# - pnl_deltas.v1: Partitioned by contest_id for contest-local ordering
# - contests.v1: Low volume, single partition for global ordering
#
# Service Topic Usage:
# - trading-engine: consumes orders.v1, ticks.v1, close_positions.v1, cancel_orders.v1, modify_tpsl.v1
#                   produces fills.v1, positions.v1, pnl_deltas.v1, order_acks.v1, position_closed.v1, order_cancelled.v1
# - leaderboard-worker: consumes pnl_deltas.v1, contests.v1
# - settlement-service: consumes contests.v1, settlement_requests.v1, position_closed.v1
#                       produces settlement_events.v1, contest_close_positions.v1, contest_cancel_orders.v1, notifications.v1
# - contest-scheduler: produces contests.v1
# - free-contest-generator: produces contests.v1
# - trade-bff: consumes ticks.v1, fills.v1, positions.v1, order_acks.v1, order_cancelled.v1, pnl_deltas.v1, contests.v1
#              produces orders.v1
# - market-ingestor: produces ticks.v1

declare -A TOPICS
TOPICS=(
    # =========================================================================
    # HIGH-THROUGHPUT TRADING TOPICS (16 partitions)
    # =========================================================================

    # Order flow topics - partitioned by contest_id
    ["orders.v1"]="16:604800000"            # Order requests from trade-bff to trading-engine - 7 days
    ["fills.v1"]="16:604800000"             # Fill events from trading-engine to trade-bff - 7 days
    ["positions.v1"]="16:604800000"         # Position updates from trading-engine to trade-bff - 7 days
    ["order_acks.v1"]="16:86400000"         # Order acknowledgments from trading-engine - 1 day
    ["pnl_deltas.v1"]="16:604800000"        # PnL updates from trading-engine to leaderboard-worker - 7 days

    # Market data topics - partitioned by symbol
    ["ticks.v1"]="16:86400000"              # Market ticks from market-ingestor - 1 day retention

    # =========================================================================
    # TRADING ENGINE COMMAND TOPICS (16 partitions)
    # Partitioned by contest_id for contest-local ordering
    # =========================================================================

    ["close_positions.v1"]="16:86400000"    # Close position requests to trading-engine - 1 day
    ["cancel_orders.v1"]="16:86400000"      # Cancel order requests to trading-engine - 1 day
    ["modify_tpsl.v1"]="16:86400000"        # Modify TP/SL requests to trading-engine - 1 day

    # =========================================================================
    # TRADING ENGINE EVENT TOPICS (16 partitions)
    # =========================================================================

    ["position_closed.v1"]="16:604800000"   # Position closed events from trading-engine - 7 days
    ["order_cancelled.v1"]="16:604800000"   # Order cancelled events from trading-engine - 7 days

    # =========================================================================
    # SETTLEMENT SERVICE TOPICS
    # =========================================================================

    ["settlement_requests.v1"]="8:604800000"    # Settlement requests to settlement-service - 7 days
    ["settlement_events.v1"]="8:2592000000"     # Settlement events from settlement-service - 30 days

    # Contest-level close/cancel (from settlement-service to trading-engine)
    ["contest_close_positions.v1"]="8:86400000" # Bulk close positions for contest - 1 day
    ["contest_cancel_orders.v1"]="8:86400000"   # Bulk cancel orders for contest - 1 day

    # =========================================================================
    # NOTIFICATION TOPICS
    # =========================================================================

    ["notifications.v1"]="4:604800000"          # General notifications - 7 days
    # =========================================================================
    # CONTROL TOPICS (1 partition for global ordering)
    # =========================================================================

    ["contests.v1"]="1:2592000000"          # Contest state changes (low volume) - 30 days retention
)

main() {
    log_info "Kafka Topic Setup for Tragge Trading Platform"
    log_info "Broker: $BROKER"
    log_info "Default partitions: $PARTITIONS"
    log_info "Default replication: $REPLICATION"
    echo ""

    # Check connectivity
    local cli_type=$(detect_kafka_cli)
    log_info "Detected CLI: $cli_type"

    if [ "$cli_type" == "rpk" ]; then
        if ! rpk cluster info --brokers "$BROKER" &>/dev/null; then
            log_error "Cannot connect to Kafka broker at $BROKER"
            exit 1
        fi
    fi

    echo ""
    log_info "Creating topics..."
    echo ""

    # Create each topic
    for topic in "${!TOPICS[@]}"; do
        IFS=':' read -r partitions retention_ms <<< "${TOPICS[$topic]}"
        create_topic "$topic" "$partitions" "$REPLICATION" "$retention_ms"
    done

    echo ""
    log_info "Topic setup complete!"
    echo ""

    # List all topics
    log_info "Current topics:"
    case $cli_type in
        rpk)
            rpk topic list --brokers "$BROKER"
            ;;
        kafka|kafka-sh)
            kafka-topics --bootstrap-server "$BROKER" --list
            ;;
        none)
            if docker ps --format '{{.Names}}' | grep -q redpanda; then
                docker exec -it $(docker ps --filter name=redpanda --format '{{.ID}}' | head -1) \
                    rpk topic list
            fi
            ;;
    esac
}

# Help message
if [ "${1:-}" == "-h" ] || [ "${1:-}" == "--help" ]; then
    echo "Kafka Topic Setup Script for Tragge Trading Platform"
    echo ""
    echo "Usage: $0 [broker_address]"
    echo ""
    echo "Environment Variables:"
    echo "  KAFKA_PARTITIONS    Default partition count (default: 16)"
    echo "  KAFKA_REPLICATION   Default replication factor (default: 1)"
    echo "  KAFKA_RETENTION_MS  Default retention in ms (default: 604800000 / 7 days)"
    echo ""
    echo "Topics created (18 total):"
    echo ""
    echo "  HIGH-THROUGHPUT TRADING TOPICS (16 partitions):"
    echo "    orders.v1           - Order requests from trade-bff to trading-engine"
    echo "    fills.v1            - Fill events from trading-engine to trade-bff"
    echo "    positions.v1        - Position updates from trading-engine"
    echo "    order_acks.v1       - Order acknowledgments from trading-engine"
    echo "    pnl_deltas.v1       - PnL updates for leaderboard-worker"
    echo "    ticks.v1            - Market ticks from market-ingestor (partitioned by symbol)"
    echo ""
    echo "  TRADING ENGINE COMMAND TOPICS (16 partitions):"
    echo "    close_positions.v1  - Close position requests to trading-engine"
    echo "    cancel_orders.v1    - Cancel order requests to trading-engine"
    echo "    modify_tpsl.v1      - Modify TP/SL requests to trading-engine"
    echo ""
    echo "  TRADING ENGINE EVENT TOPICS (16 partitions):"
    echo "    position_closed.v1  - Position closed events from trading-engine"
    echo "    order_cancelled.v1  - Order cancelled events from trading-engine"
    echo ""
    echo "  SETTLEMENT SERVICE TOPICS (8 partitions):"
    echo "    settlement_requests.v1     - Settlement requests to settlement-service"
    echo "    settlement_events.v1       - Settlement events from settlement-service"
    echo "    contest_close_positions.v1 - Bulk close positions for contest"
    echo "    contest_cancel_orders.v1   - Bulk cancel orders for contest"
    echo ""
    echo "  NOTIFICATION TOPICS (4 partitions):"
    echo "    notifications.v1    - General notifications"
    echo ""
    echo "  CONTROL TOPICS (1 partition):"
    echo "    contests.v1         - Contest state changes (global ordering)"
    echo ""
    exit 0
fi

main "$@"
