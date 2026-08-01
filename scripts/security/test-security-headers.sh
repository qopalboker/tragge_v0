#!/bin/bash
# =============================================================================
# Security Headers Test Script
# =============================================================================
# Tests HTTP security headers for the Tragge trading platform.
#
# Usage:
#   ./test-security-headers.sh [OPTIONS]
#
# Options:
#   -u, --url URL       Base URL to test (default: http://localhost:8080)
#   -p, --production    Test for production-required headers (includes HSTS)
#   -v, --verbose       Show full header output
#   -h, --help          Show this help message
#
# Examples:
#   ./test-security-headers.sh
#   ./test-security-headers.sh -u https://api.tragge.io -p
#   ./test-security-headers.sh --verbose
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
BASE_URL="${BASE_URL:-http://localhost:8080}"
PRODUCTION_MODE=false
VERBOSE=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -u|--url)
            BASE_URL="$2"
            shift 2
            ;;
        -p|--production)
            PRODUCTION_MODE=true
            shift
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -h|--help)
            head -n 22 "$0" | tail -n +2
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Counters
PASS=0
FAIL=0
WARN=0

# Print functions
print_header() {
    echo -e "\n${BLUE}=== $1 ===${NC}\n"
}

print_pass() {
    echo -e "  ${GREEN}✓${NC} $1"
    ((PASS++))
}

print_fail() {
    echo -e "  ${RED}✗${NC} $1"
    ((FAIL++))
}

print_warn() {
    echo -e "  ${YELLOW}⚠${NC} $1"
    ((WARN++))
}

print_info() {
    echo -e "  ${BLUE}ℹ${NC} $1"
}

# Check if header exists and matches expected value
check_header() {
    local endpoint="$1"
    local header="$2"
    local expected="$3"
    local required="${4:-true}"

    local headers
    headers=$(curl -s -I -X GET "${BASE_URL}${endpoint}" 2>/dev/null || echo "")

    if [[ -z "$headers" ]]; then
        print_fail "Could not connect to ${BASE_URL}${endpoint}"
        return 1
    fi

    local value
    value=$(echo "$headers" | grep -i "^${header}:" | cut -d':' -f2- | tr -d '\r' | xargs 2>/dev/null || echo "")

    if [[ -z "$value" ]]; then
        if [[ "$required" == "true" ]]; then
            print_fail "${header}: MISSING"
        else
            print_warn "${header}: Not set (optional)"
        fi
        return 1
    fi

    if [[ -n "$expected" ]]; then
        if [[ "$value" == *"$expected"* ]]; then
            print_pass "${header}: ${value}"
            return 0
        else
            print_fail "${header}: ${value} (expected: ${expected})"
            return 1
        fi
    else
        print_pass "${header}: ${value}"
        return 0
    fi
}

# Check if header does NOT exist (for security)
check_no_header() {
    local endpoint="$1"
    local header="$2"

    local headers
    headers=$(curl -s -I -X GET "${BASE_URL}${endpoint}" 2>/dev/null || echo "")

    local value
    value=$(echo "$headers" | grep -i "^${header}:" | cut -d':' -f2- | tr -d '\r' | xargs 2>/dev/null || echo "")

    if [[ -z "$value" ]]; then
        print_pass "${header}: Not exposed (good)"
        return 0
    else
        print_warn "${header}: ${value} (should not be exposed)"
        return 1
    fi
}

# Check rate limit headers
check_rate_limit_headers() {
    local endpoint="$1"

    local headers
    headers=$(curl -s -I -X GET "${BASE_URL}${endpoint}" 2>/dev/null || echo "")

    local limit
    limit=$(echo "$headers" | grep -i "^X-RateLimit-Limit:" | cut -d':' -f2- | tr -d '\r' | xargs 2>/dev/null || echo "")

    if [[ -n "$limit" ]]; then
        print_pass "X-RateLimit-Limit: ${limit}"
    else
        print_info "X-RateLimit-Limit: Not set (nginx may add on rate limit)"
    fi

    local policy
    policy=$(echo "$headers" | grep -i "^X-RateLimit-Policy:" | cut -d':' -f2- | tr -d '\r' | xargs 2>/dev/null || echo "")

    if [[ -n "$policy" ]]; then
        print_pass "X-RateLimit-Policy: ${policy}"
    fi
}

# Check CORS headers
check_cors() {
    local endpoint="$1"
    local origin="$2"

    local headers
    headers=$(curl -s -I -X OPTIONS \
        -H "Origin: ${origin}" \
        -H "Access-Control-Request-Method: GET" \
        -H "Access-Control-Request-Headers: Content-Type, Authorization" \
        "${BASE_URL}${endpoint}" 2>/dev/null || echo "")

    local allow_origin
    allow_origin=$(echo "$headers" | grep -i "^Access-Control-Allow-Origin:" | cut -d':' -f2- | tr -d '\r' | xargs 2>/dev/null || echo "")

    if [[ "$allow_origin" == "$origin" ]]; then
        print_pass "CORS allows origin: ${origin}"
    elif [[ "$allow_origin" == "*" ]]; then
        print_warn "CORS allows all origins (*) - not recommended for credentials"
    elif [[ -z "$allow_origin" ]]; then
        print_info "CORS: Origin ${origin} not allowed (may be intentional)"
    else
        print_info "CORS Access-Control-Allow-Origin: ${allow_origin}"
    fi
}

# Get verbose headers
show_verbose_headers() {
    local endpoint="$1"

    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "\n  ${BLUE}Full headers for ${endpoint}:${NC}"
        curl -s -I -X GET "${BASE_URL}${endpoint}" 2>/dev/null | sed 's/^/    /'
    fi
}

# =============================================================================
# Main Tests
# =============================================================================

echo -e "${BLUE}"
echo "╔════════════════════════════════════════════════════════════════════╗"
echo "║           Tragge Security Headers Test Suite                       ║"
echo "╚════════════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

echo -e "Testing: ${BLUE}${BASE_URL}${NC}"
echo -e "Mode: ${YELLOW}$(if $PRODUCTION_MODE; then echo "Production"; else echo "Development"; fi)${NC}"
echo ""

# =============================================================================
# Test 1: Basic Security Headers (All Endpoints)
# =============================================================================

print_header "Basic Security Headers (/health)"

check_header "/health" "X-Frame-Options" "DENY"
check_header "/health" "X-Content-Type-Options" "nosniff"
check_header "/health" "X-XSS-Protection" "1; mode=block"
check_header "/health" "Referrer-Policy" "strict-origin"
check_header "/health" "Permissions-Policy" ""
check_header "/health" "X-Request-ID" ""

# Server header should be hidden
check_no_header "/health" "Server"

show_verbose_headers "/health"

# =============================================================================
# Test 2: HSTS (Production Only)
# =============================================================================

print_header "HSTS (Strict-Transport-Security)"

if $PRODUCTION_MODE; then
    check_header "/health" "Strict-Transport-Security" "max-age=" "true"
else
    check_header "/health" "Strict-Transport-Security" "" "false"
    print_info "HSTS not required in development mode (use -p for production)"
fi

# =============================================================================
# Test 3: Content-Security-Policy per Frontend
# =============================================================================

print_header "Content-Security-Policy"

# User frontend
print_info "Testing /user endpoint..."
check_header "/user" "Content-Security-Policy" "default-src 'self'" "false"

# Trade frontend
print_info "Testing /trade endpoint..."
check_header "/trade" "Content-Security-Policy" "default-src 'self'" "false"

# Admin frontend
print_info "Testing /admin endpoint..."
check_header "/admin" "Content-Security-Policy" "default-src 'self'" "false"

# API endpoints (minimal CSP)
print_info "Testing /api/user endpoint..."
check_header "/api/user/healthz" "Content-Security-Policy" "default-src 'none'" "false"

# =============================================================================
# Test 4: Cross-Origin Headers
# =============================================================================

print_header "Cross-Origin Headers"

check_header "/health" "Cross-Origin-Opener-Policy" "" "false"
check_header "/health" "Cross-Origin-Embedder-Policy" "" "false"
check_header "/health" "Cross-Origin-Resource-Policy" "" "false"

# =============================================================================
# Test 5: Rate Limiting Headers
# =============================================================================

print_header "Rate Limiting Headers"

print_info "Testing /api/user endpoint..."
check_rate_limit_headers "/api/user/healthz"

print_info "Testing /api/user/auth/login endpoint..."
check_rate_limit_headers "/api/user/auth/login"

# =============================================================================
# Test 6: CORS Configuration
# =============================================================================

print_header "CORS Configuration"

# Test with localhost origins (development)
print_info "Testing CORS with localhost:5173..."
check_cors "/api/user/healthz" "http://localhost:5173"

print_info "Testing CORS with localhost:5174..."
check_cors "/api/user/healthz" "http://localhost:5174"

print_info "Testing CORS with unauthorized origin..."
check_cors "/api/user/healthz" "http://evil.com"

# =============================================================================
# Test 7: Cache Control Headers (API)
# =============================================================================

print_header "Cache Control (API)"

headers=$(curl -s -I -X GET "${BASE_URL}/api/user/healthz" 2>/dev/null || echo "")
cache_control=$(echo "$headers" | grep -i "^Cache-Control:" | cut -d':' -f2- | tr -d '\r' | xargs 2>/dev/null || echo "")

if [[ "$cache_control" == *"no-store"* ]] || [[ "$cache_control" == *"no-cache"* ]]; then
    print_pass "Cache-Control: ${cache_control}"
else
    print_info "Cache-Control: ${cache_control:-Not set}"
fi

# =============================================================================
# Test 8: Request ID Correlation
# =============================================================================

print_header "Request ID Correlation"

# Check that request ID is returned
request_id=$(curl -s -I -X GET "${BASE_URL}/health" 2>/dev/null | grep -i "^X-Request-ID:" | cut -d':' -f2- | tr -d '\r' | xargs 2>/dev/null || echo "")

if [[ -n "$request_id" ]]; then
    print_pass "X-Request-ID returned: ${request_id}"

    # Verify it looks like a UUID
    if [[ "$request_id" =~ ^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$ ]]; then
        print_pass "Request ID is a valid UUID format"
    else
        print_info "Request ID format: non-UUID (may be nginx-generated)"
    fi
else
    print_fail "X-Request-ID not returned"
fi

# Test request ID propagation
print_info "Testing request ID propagation..."
custom_id="test-$(date +%s)"
propagated_id=$(curl -s -I -X GET -H "X-Request-ID: ${custom_id}" "${BASE_URL}/health" 2>/dev/null | grep -i "^X-Request-ID:" | cut -d':' -f2- | tr -d '\r' | xargs 2>/dev/null || echo "")

if [[ "$propagated_id" == "$custom_id" ]]; then
    print_pass "Request ID propagated correctly"
else
    print_info "Request ID not propagated (nginx may generate new ID)"
fi

# =============================================================================
# Summary
# =============================================================================

print_header "Summary"

echo -e "  ${GREEN}Passed:${NC}  ${PASS}"
echo -e "  ${RED}Failed:${NC}  ${FAIL}"
echo -e "  ${YELLOW}Warnings:${NC} ${WARN}"
echo ""

if [[ $FAIL -eq 0 ]]; then
    echo -e "${GREEN}All security header checks passed!${NC}"
    exit 0
elif [[ $FAIL -le 3 ]]; then
    echo -e "${YELLOW}Some security headers missing or misconfigured.${NC}"
    exit 0
else
    echo -e "${RED}Multiple security header issues detected.${NC}"
    exit 1
fi
