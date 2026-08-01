#!/bin/bash
# Redis Sentinel Failover Test Script
# This script tests Redis Sentinel automatic failover locally using Docker Compose.
#
# Usage:
#   ./scripts/redis-failover-test.sh
#
# Prerequisites:
#   - Docker Compose stack running with Sentinel:
#     docker-compose -f infra/docker/docker-compose.yml -f infra/docker/docker-compose.redis-sentinel.yml up -d
#
# The script will:
#   1. Verify initial Redis Sentinel setup
#   2. Identify the current master
#   3. Kill the master container
#   4. Verify Sentinel promotes a replica to master
#   5. Verify clients reconnect to new master
#   6. Restore the original master as a replica

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Configuration
SENTINEL_HOST="${SENTINEL_HOST:-localhost}"
SENTINEL_PORT="${SENTINEL_PORT:-26379}"
MASTER_NAME="${MASTER_NAME:-mymaster}"
DOCKER_COMPOSE_DIR="${DOCKER_COMPOSE_DIR:-infra/docker}"
FAILOVER_TIMEOUT="${FAILOVER_TIMEOUT:-30}"

# Check if redis-cli is available
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v redis-cli &> /dev/null; then
        log_error "redis-cli not found. Please install Redis tools."
        log_info "On macOS: brew install redis"
        log_info "On Ubuntu: apt-get install redis-tools"
        exit 1
    fi

    if ! command -v docker &> /dev/null; then
        log_error "docker not found. Please install Docker."
        exit 1
    fi

    log_success "Prerequisites check passed"
}

# Get current master info from Sentinel
get_master_info() {
    local info
    info=$(redis-cli -h "$SENTINEL_HOST" -p "$SENTINEL_PORT" SENTINEL master "$MASTER_NAME" 2>/dev/null || echo "")

    if [[ -z "$info" ]]; then
        log_error "Failed to get master info from Sentinel at $SENTINEL_HOST:$SENTINEL_PORT"
        return 1
    fi

    # Parse the info (redis-cli returns key-value pairs on alternating lines)
    local master_ip master_port
    master_ip=$(echo "$info" | awk '/^ip$/{getline; print}' | head -1)
    master_port=$(echo "$info" | awk '/^port$/{getline; print}' | head -1)

    echo "$master_ip:$master_port"
}

# Get replica count
get_replica_count() {
    redis-cli -h "$SENTINEL_HOST" -p "$SENTINEL_PORT" SENTINEL slaves "$MASTER_NAME" 2>/dev/null | grep -c "^name" || echo "0"
}

# Check if Sentinel is healthy
check_sentinel_health() {
    log_info "Checking Sentinel health..."

    # Check all three sentinels
    local healthy_sentinels=0
    for port in 26379 26380 26381; do
        if redis-cli -h "$SENTINEL_HOST" -p "$port" PING 2>/dev/null | grep -q "PONG"; then
            ((healthy_sentinels++))
        fi
    done

    if [[ $healthy_sentinels -lt 2 ]]; then
        log_error "Insufficient healthy Sentinels: $healthy_sentinels (need at least 2)"
        return 1
    fi

    log_success "Sentinel health check passed ($healthy_sentinels/3 healthy)"
}

# Wait for failover to complete
wait_for_failover() {
    local old_master="$1"
    local timeout="$2"
    local start_time
    start_time=$(date +%s)

    log_info "Waiting for failover (timeout: ${timeout}s)..."

    while true; do
        local current_time
        current_time=$(date +%s)
        local elapsed=$((current_time - start_time))

        if [[ $elapsed -gt $timeout ]]; then
            log_error "Failover timeout after ${timeout}s"
            return 1
        fi

        local new_master
        new_master=$(get_master_info 2>/dev/null || echo "")

        if [[ -n "$new_master" && "$new_master" != "$old_master" ]]; then
            log_success "Failover completed in ${elapsed}s"
            log_info "New master: $new_master"
            return 0
        fi

        sleep 1
        echo -n "."
    done
}

# Test master connectivity
test_master_connectivity() {
    local master_info="$1"
    local master_ip master_port
    master_ip="${master_info%:*}"
    master_port="${master_info#*:}"

    log_info "Testing master connectivity at $master_ip:$master_port..."

    # Test basic operations
    local test_key="failover_test_$(date +%s)"
    local test_value="test_value"

    if redis-cli -h "$master_ip" -p "$master_port" SET "$test_key" "$test_value" 2>/dev/null | grep -q "OK"; then
        local result
        result=$(redis-cli -h "$master_ip" -p "$master_port" GET "$test_key" 2>/dev/null)

        if [[ "$result" == "$test_value" ]]; then
            # Cleanup
            redis-cli -h "$master_ip" -p "$master_port" DEL "$test_key" >/dev/null 2>&1
            log_success "Master read/write operations working"
            return 0
        fi
    fi

    log_error "Master connectivity test failed"
    return 1
}

# Main test function
run_failover_test() {
    echo "=============================================="
    echo "  Redis Sentinel Failover Test"
    echo "=============================================="
    echo ""

    check_prerequisites
    echo ""

    # Step 1: Check Sentinel health
    check_sentinel_health || exit 1
    echo ""

    # Step 2: Get current master
    log_info "Getting current master info..."
    local original_master
    original_master=$(get_master_info)
    if [[ -z "$original_master" ]]; then
        log_error "Could not determine current master"
        exit 1
    fi
    log_success "Current master: $original_master"

    local replica_count
    replica_count=$(get_replica_count)
    log_info "Current replica count: $replica_count"

    if [[ $replica_count -lt 1 ]]; then
        log_error "No replicas available for failover"
        exit 1
    fi
    echo ""

    # Step 3: Test master connectivity
    test_master_connectivity "$original_master" || exit 1
    echo ""

    # Step 4: Kill the master container
    log_info "Killing Redis master container..."

    # Determine which container to kill based on the master IP
    local master_container
    if [[ "$original_master" == *"redis-master"* || "$original_master" == "redis-master:6379" ]]; then
        master_container="tragge_redis_master"
    else
        # Check which container has this IP
        local master_ip="${original_master%:*}"
        master_container=$(docker ps --filter "name=tragge_redis" --format "{{.Names}}" | while read name; do
            container_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$name" 2>/dev/null || echo "")
            if [[ "$container_ip" == "$master_ip" ]]; then
                echo "$name"
                break
            fi
        done)
    fi

    if [[ -z "$master_container" ]]; then
        # Default to redis-master if we can't determine
        master_container="tragge_redis_master"
    fi

    log_info "Stopping container: $master_container"
    docker stop "$master_container" --time=1 >/dev/null 2>&1 || {
        log_warning "Container may already be stopped"
    }

    echo ""

    # Step 5: Wait for failover
    local failover_start
    failover_start=$(date +%s)

    wait_for_failover "$original_master" "$FAILOVER_TIMEOUT" || {
        log_error "Failover did not complete"
        log_info "Restarting original master..."
        docker start "$master_container" >/dev/null 2>&1
        exit 1
    }
    echo ""

    # Step 6: Get new master and verify
    local new_master
    new_master=$(get_master_info)
    log_info "New master after failover: $new_master"

    test_master_connectivity "$new_master" || {
        log_error "New master connectivity test failed"
        exit 1
    }
    echo ""

    # Step 7: Restart original master as replica
    log_info "Restarting original master container (will join as replica)..."
    docker start "$master_container" >/dev/null 2>&1 || log_warning "Failed to restart container"

    sleep 5

    local final_replica_count
    final_replica_count=$(get_replica_count)
    log_info "Final replica count: $final_replica_count"
    echo ""

    # Calculate total failover time
    local failover_end
    failover_end=$(date +%s)
    local total_time=$((failover_end - failover_start))

    # Summary
    echo "=============================================="
    echo "  Failover Test Summary"
    echo "=============================================="
    echo "  Original master:    $original_master"
    echo "  New master:         $new_master"
    echo "  Failover time:      ~${total_time}s"
    echo "  Replica count:      $final_replica_count"
    echo "  Status:             ${GREEN}SUCCESS${NC}"
    echo "=============================================="

    log_success "Failover test completed successfully!"
}

# Run the test
run_failover_test
