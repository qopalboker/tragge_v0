#!/bin/bash
#
# Post-startup Health Check Script for Tragge Trading Platform
#
# Validates all services are healthy and connected after docker-compose up.
# Unlike check-health.sh (which checks current state), this script WAITS
# for services to become ready, making it suitable for CI/CD pipelines.
#
# Usage: ./scripts/health-check.sh [options]
#
# Options:
#   --timeout SECONDS    Maximum wait time for services (default: 120)
#   --interval SECONDS   Polling interval between checks (default: 5)
#   --skip-monitoring    Skip monitoring stack checks
#   --json               Output final results as JSON
#   --verbose            Show detailed output
#   --help               Show this help message
#
# Exit codes:
#   0 - All services healthy
#   1 - One or more services unhealthy after timeout
#
# Environment Variables:
#   GATEWAY_URL          Gateway base URL (default: http://localhost:8080)
#   POSTGRES_HOST        PostgreSQL host (default: localhost)
#   POSTGRES_PORT        PostgreSQL port (default: 5432)
#   POSTGRES_USER        PostgreSQL user (default: tragge_admin)
#   POSTGRES_DB          PostgreSQL database (default: app)
#   REDIS_HOST           Redis host (default: localhost)
#   REDIS_PORT           Redis port (default: 6379)
#

set -uo pipefail

# ─── Configuration ───────────────────────────────────────────────────────────

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-tragge_admin}"
POSTGRES_DB="${POSTGRES_DB:-app}"
REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"

TIMEOUT=120
INTERVAL=5
SKIP_MONITORING=false
JSON_OUTPUT=false
VERBOSE=false

# ─── Parse arguments ─────────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
    case $1 in
        --timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        --interval)
            INTERVAL="$2"
            shift 2
            ;;
        --skip-monitoring)
            SKIP_MONITORING=true
            shift
            ;;
        --json)
            JSON_OUTPUT=true
            shift
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --help|-h)
            sed -n '2,/^$/p' "$0" | sed 's/^# \?//'
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# ─── Colors & formatting ─────────────────────────────────────────────────────

if [ -t 1 ] && [ "$JSON_OUTPUT" = false ]; then
    GREEN='\033[0;32m'
    RED='\033[0;31m'
    YELLOW='\033[1;33m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    DIM='\033[2m'
    NC='\033[0m'
else
    GREEN='' RED='' YELLOW='' CYAN='' BOLD='' DIM='' NC=''
fi

# ─── Tracking ────────────────────────────────────────────────────────────────

PASSED=0
FAILED=0
WARNED=0
TOTAL=0
JSON_RESULTS=()

pass() {
    TOTAL=$((TOTAL + 1))
    PASSED=$((PASSED + 1))
    local detail="${2:-}"
    if [ "$JSON_OUTPUT" = true ]; then
        JSON_RESULTS+=("{\"name\":\"$1\",\"status\":\"pass\",\"detail\":\"$detail\"}")
    else
        printf "  ${GREEN}OK${NC}  %s" "$1"
        if [ "$VERBOSE" = true ] && [ -n "$detail" ]; then
            printf " ${DIM}(%s)${NC}" "$detail"
        fi
        echo
    fi
}

fail() {
    TOTAL=$((TOTAL + 1))
    FAILED=$((FAILED + 1))
    local detail="${2:-}"
    if [ "$JSON_OUTPUT" = true ]; then
        JSON_RESULTS+=("{\"name\":\"$1\",\"status\":\"fail\",\"detail\":\"$detail\"}")
    else
        printf "  ${RED}FAIL${NC} %s" "$1"
        if [ -n "$detail" ]; then
            printf " ${DIM}(%s)${NC}" "$detail"
        fi
        echo
    fi
}

warn() {
    TOTAL=$((TOTAL + 1))
    WARNED=$((WARNED + 1))
    local detail="${2:-}"
    if [ "$JSON_OUTPUT" = true ]; then
        JSON_RESULTS+=("{\"name\":\"$1\",\"status\":\"warn\",\"detail\":\"$detail\"}")
    else
        printf "  ${YELLOW}WARN${NC} %s" "$1"
        if [ -n "$detail" ]; then
            printf " ${DIM}(%s)${NC}" "$detail"
        fi
        echo
    fi
}

section() {
    if [ "$JSON_OUTPUT" = false ]; then
        echo
        echo -e "${BOLD}${CYAN}=== $1 ===${NC}"
    fi
}

log() {
    if [ "$VERBOSE" = true ] && [ "$JSON_OUTPUT" = false ]; then
        echo -e "       ${DIM}$1${NC}"
    fi
}

# ─── Wait helper ──────────────────────────────────────────────────────────────

# wait_for_url URL NAME [EXPECTED_CODE]
# Polls a URL until it returns the expected HTTP status or timeout expires.
wait_for_url() {
    local url="$1"
    local name="$2"
    local expected="${3:-200}"
    local elapsed=0

    while [ "$elapsed" -lt "$TIMEOUT" ]; do
        local code
        code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 --max-time 5 "$url" 2>/dev/null) || code="000"

        if [ "$code" = "$expected" ]; then
            pass "$name" "HTTP $code after ${elapsed}s"
            return 0
        fi

        log "Waiting for $name... (HTTP $code, ${elapsed}s/${TIMEOUT}s)"
        sleep "$INTERVAL"
        elapsed=$((elapsed + INTERVAL))
    done

    fail "$name" "timeout after ${TIMEOUT}s (last HTTP $code)"
    return 1
}

# ─── 1. Infrastructure Services ──────────────────────────────────────────────

check_infrastructure() {
    section "Infrastructure"

    # PostgreSQL
    local pg_ok=false
    local elapsed=0
    while [ "$elapsed" -lt "$TIMEOUT" ]; do
        if command -v pg_isready &>/dev/null; then
            if pg_isready -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t 3 &>/dev/null; then
                pg_ok=true
                break
            fi
        else
            if docker exec tragge_postgres pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t 3 &>/dev/null; then
                pg_ok=true
                break
            fi
        fi
        log "Waiting for PostgreSQL... (${elapsed}s/${TIMEOUT}s)"
        sleep "$INTERVAL"
        elapsed=$((elapsed + INTERVAL))
    done

    if [ "$pg_ok" = true ]; then
        pass "PostgreSQL" "${POSTGRES_HOST}:${POSTGRES_PORT} ready after ${elapsed}s"

        # Verify tables exist
        local table_count
        table_count=$(docker exec tragge_postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc \
            "SELECT count(*) FROM information_schema.tables WHERE table_schema='public';" 2>/dev/null) || table_count="0"
        if [ "$table_count" -gt 0 ] 2>/dev/null; then
            pass "Database schema" "$table_count tables"
        else
            warn "Database schema" "no tables found (run make migrate-up)"
        fi
    else
        fail "PostgreSQL" "timeout after ${TIMEOUT}s"
    fi

    # Redis
    local redis_ok=false
    elapsed=0
    while [ "$elapsed" -lt "$TIMEOUT" ]; do
        local pong
        if command -v redis-cli &>/dev/null; then
            pong=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ping 2>/dev/null)
        else
            pong=$(docker exec tragge_redis redis-cli ping 2>/dev/null)
        fi
        if [ "$pong" = "PONG" ]; then
            redis_ok=true
            break
        fi
        log "Waiting for Redis... (${elapsed}s/${TIMEOUT}s)"
        sleep "$INTERVAL"
        elapsed=$((elapsed + INTERVAL))
    done

    if [ "$redis_ok" = true ]; then
        pass "Redis" "${REDIS_HOST}:${REDIS_PORT} ready after ${elapsed}s"
    else
        fail "Redis" "timeout after ${TIMEOUT}s"
    fi

    # Redpanda (Kafka)
    local rp_ok=false
    elapsed=0
    while [ "$elapsed" -lt "$TIMEOUT" ]; do
        local health
        health=$(docker exec tragge_redpanda rpk cluster health 2>/dev/null) || health=""
        if echo "$health" | grep -q "Healthy:.*true"; then
            rp_ok=true
            break
        fi
        log "Waiting for Redpanda... (${elapsed}s/${TIMEOUT}s)"
        sleep "$INTERVAL"
        elapsed=$((elapsed + INTERVAL))
    done

    if [ "$rp_ok" = true ]; then
        pass "Redpanda" "cluster healthy after ${elapsed}s"
    else
        fail "Redpanda" "timeout after ${TIMEOUT}s"
    fi
}

# ─── 2. Go Microservices ─────────────────────────────────────────────────────

check_services() {
    section "Services (healthz)"

    # Format: display_name:host_port
    # All services expose /healthz on their host-mapped ports
    local -a SERVICES=(
        "user-bff:8081"
        "admin-bff:8083"
        "trade-bff:8085"
        "market-ingestor:8084"
        "trading-engine:8093"
        "leaderboard-worker:8094"
        "payment-service:8086"
        "contest-scheduler:8088"
        "free-contest-generator:8091"
        "settlement-service:8095"
        "shard-router:8090"
    )

    for entry in "${SERVICES[@]}"; do
        IFS=':' read -r name port <<< "$entry"
        wait_for_url "http://localhost:${port}/healthz" "$name"
    done

    # Readiness probes for services that implement /readyz
    section "Services (readyz)"

    local -a READYZ_SERVICES=(
        "user-bff:8081"
        "trade-bff:8085"
        "admin-bff:8083"
        "leaderboard-worker:8094"
    )

    for entry in "${READYZ_SERVICES[@]}"; do
        IFS=':' read -r name port <<< "$entry"
        local code
        code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 --max-time 5 \
            "http://localhost:${port}/readyz" 2>/dev/null) || code="000"
        if [ "$code" = "200" ]; then
            pass "$name /readyz" "dependencies connected"
        elif [ "$code" = "503" ]; then
            local body
            body=$(curl -s --connect-timeout 3 --max-time 5 "http://localhost:${port}/readyz" 2>/dev/null)
            warn "$name /readyz" "not ready: $body"
        else
            warn "$name /readyz" "HTTP $code"
        fi
    done
}

# ─── 3. Gateway ──────────────────────────────────────────────────────────────

check_gateway() {
    section "Gateway"

    wait_for_url "${GATEWAY_URL}/health" "Gateway /health"

    # Verify gateway can route to backend services
    local -a ROUTES=(
        "/api/user/healthz|user-bff via gateway"
        "/api/trade/healthz|trade-bff via gateway"
        "/api/admin/healthz|admin-bff via gateway"
    )

    for entry in "${ROUTES[@]}"; do
        local path="${entry%%|*}"
        local name="${entry#*|}"
        local code
        code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 --max-time 5 \
            "${GATEWAY_URL}${path}" 2>/dev/null) || code="000"
        if [ "$code" = "200" ]; then
            pass "$name" "HTTP $code"
        else
            fail "$name" "HTTP $code"
        fi
    done

    # Check frontends are reachable through gateway
    local -a FRONTENDS=(
        "/user/|frontend (user) via gateway"
        "/trade/|frontend (trade) via gateway"
        "/admin/|frontend (admin) via gateway"
    )

    for entry in "${FRONTENDS[@]}"; do
        local path="${entry%%|*}"
        local name="${entry#*|}"
        local code
        code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 --max-time 5 \
            "${GATEWAY_URL}${path}" 2>/dev/null) || code="000"
        if [ "$code" = "200" ]; then
            pass "$name" "HTTP $code"
        else
            warn "$name" "HTTP $code"
        fi
    done
}

# ─── 4. Kafka Topics ─────────────────────────────────────────────────────────

check_kafka_topics() {
    section "Kafka Topics"

    local topic_list
    topic_list=$(docker exec tragge_redpanda rpk topic list 2>/dev/null) || {
        fail "Topic listing" "cannot connect to Redpanda"
        return
    }

    local -a REQUIRED_TOPICS=(
        "orders.v1"
        "fills.v1"
        "ticks.v1"
        "positions.v1"
        "order_acks.v1"
        "pnl_deltas.v1"
        "contests.v1"
        "close_positions.v1"
        "cancel_orders.v1"
        "modify_tpsl.v1"
        "position_closed.v1"
        "order_cancelled.v1"
        "settlement_requests.v1"
        "settlement_events.v1"
        "contest_close_positions.v1"
        "contest_cancel_orders.v1"
        "notifications.v1"
    )

    local present=0
    local missing=0
    local missing_names=()

    for topic in "${REQUIRED_TOPICS[@]}"; do
        if echo "$topic_list" | grep -q "^${topic} "; then
            present=$((present + 1))
            if [ "$VERBOSE" = true ]; then
                pass "Topic: $topic"
            fi
        else
            missing=$((missing + 1))
            missing_names+=("$topic")
            if [ "$VERBOSE" = true ]; then
                fail "Topic: $topic" "missing"
            fi
        fi
    done

    if [ "$missing" -eq 0 ]; then
        if [ "$VERBOSE" = false ]; then
            pass "All ${#REQUIRED_TOPICS[@]} required topics present"
        fi
    else
        if [ "$VERBOSE" = false ]; then
            fail "Kafka topics" "$missing of ${#REQUIRED_TOPICS[@]} missing: ${missing_names[*]}"
        fi
    fi

    # Check consumer groups
    local groups
    groups=$(docker exec tragge_redpanda rpk group list 2>/dev/null | tail -n +2 | grep -c . 2>/dev/null) || groups="0"
    if [ "$groups" -gt 0 ] 2>/dev/null; then
        pass "Consumer groups" "$groups active"
    else
        warn "Consumer groups" "none yet (services may not have consumed messages)"
    fi
}

# ─── 5. Monitoring Stack ─────────────────────────────────────────────────────

check_monitoring() {
    if [ "$SKIP_MONITORING" = true ]; then
        return
    fi

    section "Monitoring"

    local -a MONITORING=(
        "http://localhost:9090/-/healthy|Prometheus"
        "http://localhost:9093/-/healthy|Alertmanager"
        "http://localhost:3000/api/health|Grafana"
        "http://localhost:3100/ready|Loki"
        "http://localhost:3200/ready|Tempo"
    )

    for entry in "${MONITORING[@]}"; do
        local url="${entry%%|*}"
        local name="${entry#*|}"
        local code
        code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 --max-time 5 "$url" 2>/dev/null) || code="000"
        if [ "$code" = "200" ]; then
            pass "$name" "HTTP $code"
        elif [ "$code" = "000" ]; then
            warn "$name" "not reachable"
        else
            warn "$name" "HTTP $code"
        fi
    done
}

# ─── Summary ──────────────────────────────────────────────────────────────────

print_summary() {
    local healthy
    if [ "$FAILED" -eq 0 ]; then
        healthy=true
    else
        healthy=false
    fi

    if [ "$JSON_OUTPUT" = true ]; then
        local results
        results=$(IFS=,; echo "${JSON_RESULTS[*]}")
        echo "{\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"total\":$TOTAL,\"passed\":$PASSED,\"failed\":$FAILED,\"warned\":$WARNED,\"healthy\":$healthy,\"results\":[$results]}"
        return
    fi

    echo
    echo -e "${BOLD}=== Summary ===${NC}"
    echo
    printf "  Total:   %d\n" "$TOTAL"
    printf "  Passed:  ${GREEN}%d${NC}\n" "$PASSED"
    if [ "$FAILED" -gt 0 ]; then
        printf "  Failed:  ${RED}%d${NC}\n" "$FAILED"
    else
        printf "  Failed:  %d\n" "$FAILED"
    fi
    if [ "$WARNED" -gt 0 ]; then
        printf "  Warned:  ${YELLOW}%d${NC}\n" "$WARNED"
    fi
    echo

    if [ "$healthy" = true ]; then
        echo -e "  ${GREEN}${BOLD}ALL SERVICES HEALTHY${NC}"
    else
        echo -e "  ${RED}${BOLD}SOME SERVICES UNHEALTHY${NC}"
    fi
    echo
}

# ─── Main ─────────────────────────────────────────────────────────────────────

main() {
    if [ "$JSON_OUTPUT" = false ]; then
        echo -e "${BOLD}Tragge Platform Health Check${NC}"
        echo -e "${DIM}$(date -u +%Y-%m-%dT%H:%M:%SZ)${NC}"
        echo -e "Gateway: ${GATEWAY_URL}"
        echo -e "Timeout: ${TIMEOUT}s  Interval: ${INTERVAL}s"
    fi

    check_infrastructure
    check_services
    check_gateway
    check_kafka_topics
    check_monitoring
    print_summary

    [ "$FAILED" -eq 0 ]
}

main
