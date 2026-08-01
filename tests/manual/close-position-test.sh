#!/bin/bash
# Close Position Integration Test Script
# Usage: ./close-position-test.sh [email] [password]

set -e

# Configuration
BASE_URL="${BASE_URL:-http://localhost:8082}"
USER_BFF_URL="${USER_BFF_URL:-http://localhost:8081}"
USER_EMAIL="${1:-test@example.com}"
USER_PASSWORD="${2:-TestPass123!}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
PASSED=0
FAILED=0
SKIPPED=0

# Helper functions
log_info() {
  echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
  echo -e "${GREEN}[PASS]${NC} $1"
  ((PASSED++))
}

log_fail() {
  echo -e "${RED}[FAIL]${NC} $1"
  ((FAILED++))
}

log_skip() {
  echo -e "${YELLOW}[SKIP]${NC} $1"
  ((SKIPPED++))
}

log_header() {
  echo ""
  echo -e "${BLUE}========================================${NC}"
  echo -e "${BLUE}$1${NC}"
  echo -e "${BLUE}========================================${NC}"
}

# Check if jq is installed
if ! command -v jq &> /dev/null; then
  echo -e "${RED}Error: jq is required but not installed.${NC}"
  echo "Install with: apt-get install jq (Debian/Ubuntu) or brew install jq (Mac)"
  exit 1
fi

log_header "Close Position Test Suite"
echo "User: $USER_EMAIL"
echo "Trade BFF: $BASE_URL"
echo "User BFF: $USER_BFF_URL"
echo ""

# Step 1: Login
log_info "Logging in..."
LOGIN_RESPONSE=$(curl -s -X POST "$USER_BFF_URL/api/user/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"$USER_EMAIL\", \"password\": \"$USER_PASSWORD\"}" 2>/dev/null || echo '{"error":"connection failed"}')

TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.access_token // empty')

if [ -z "$TOKEN" ]; then
  echo -e "${RED}Login failed. Response:${NC}"
  echo "$LOGIN_RESPONSE" | jq . 2>/dev/null || echo "$LOGIN_RESPONSE"
  exit 1
fi
log_success "Login successful"

# Step 2: Get active contests
log_info "Fetching active contests..."
CONTESTS_RESPONSE=$(curl -s "$BASE_URL/api/trade/contests" \
  -H "Authorization: Bearer $TOKEN" 2>/dev/null || echo '[]')

CONTEST_ID=$(echo "$CONTESTS_RESPONSE" | jq -r '.[0].contest_id // empty')

if [ -z "$CONTEST_ID" ]; then
  echo -e "${YELLOW}No active contests found. Some tests will be skipped.${NC}"
  CONTEST_ID="00000000-0000-0000-0000-000000000000"
fi
log_info "Using contest: $CONTEST_ID"

# ========================================
# Test Suite
# ========================================

log_header "Test 1: Invalid Position ID Format"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  "$BASE_URL/api/trade/positions/invalid-uuid/close" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}' 2>/dev/null)

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | head -n-1)

if [ "$HTTP_CODE" == "400" ]; then
  log_success "Invalid UUID format rejected (HTTP $HTTP_CODE)"
else
  log_fail "Expected HTTP 400, got $HTTP_CODE"
  echo "Response: $BODY"
fi

log_header "Test 2: Non-Existent Position ID"
FAKE_UUID="00000000-0000-0000-0000-000000000000"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  "$BASE_URL/api/trade/positions/$FAKE_UUID/close" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}' 2>/dev/null)

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | head -n-1)

if [ "$HTTP_CODE" == "404" ] || [ "$HTTP_CODE" == "400" ]; then
  log_success "Non-existent position rejected (HTTP $HTTP_CODE)"
else
  log_fail "Expected HTTP 404 or 400, got $HTTP_CODE"
  echo "Response: $BODY"
fi

log_header "Test 3: Missing Authorization Header"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  "$BASE_URL/api/trade/positions/$FAKE_UUID/close" \
  -H "Content-Type: application/json" \
  -d '{}' 2>/dev/null)

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | head -n-1)

if [ "$HTTP_CODE" == "401" ]; then
  log_success "Unauthorized request rejected (HTTP $HTTP_CODE)"
else
  log_fail "Expected HTTP 401, got $HTTP_CODE"
  echo "Response: $BODY"
fi

log_header "Test 4: Invalid Authorization Token"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  "$BASE_URL/api/trade/positions/$FAKE_UUID/close" \
  -H "Authorization: Bearer invalid_token_12345" \
  -H "Content-Type: application/json" \
  -d '{}' 2>/dev/null)

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | head -n-1)

if [ "$HTTP_CODE" == "401" ]; then
  log_success "Invalid token rejected (HTTP $HTTP_CODE)"
else
  log_fail "Expected HTTP 401, got $HTTP_CODE"
  echo "Response: $BODY"
fi

log_header "Test 5: Negative Quantity"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  "$BASE_URL/api/trade/positions/$FAKE_UUID/close" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"qty": -10}' 2>/dev/null)

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | head -n-1)

if [ "$HTTP_CODE" == "400" ]; then
  log_success "Negative quantity rejected (HTTP $HTTP_CODE)"
else
  log_fail "Expected HTTP 400, got $HTTP_CODE"
  echo "Response: $BODY"
fi

log_header "Test 6: Check Health Endpoint"
RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/healthz" 2>/dev/null)
HTTP_CODE=$(echo "$RESPONSE" | tail -n1)

if [ "$HTTP_CODE" == "200" ]; then
  log_success "Trade BFF is healthy (HTTP $HTTP_CODE)"
else
  log_fail "Health check failed (HTTP $HTTP_CODE)"
fi

# ========================================
# Interactive Position Test
# ========================================

log_header "Interactive Position Close Test"
echo ""
echo "To test actual position closing, you need an open position."
echo "Would you like to test with a real position? (requires manual position creation)"
echo ""
read -p "Enter position_id to test (or press Enter to skip): " POSITION_ID

if [ -n "$POSITION_ID" ]; then
  log_info "Testing close for position: $POSITION_ID"

  # Test 7: Close actual position
  log_header "Test 7: Close Position"
  RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
    "$BASE_URL/api/trade/positions/$POSITION_ID/close" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{}' 2>/dev/null)

  HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
  BODY=$(echo "$RESPONSE" | head -n-1)

  if [ "$HTTP_CODE" == "202" ]; then
    log_success "Position close submitted (HTTP $HTTP_CODE)"
    ORDER_ID=$(echo "$BODY" | jq -r '.order_id // empty')
    if [ -n "$ORDER_ID" ]; then
      echo "  Order ID: $ORDER_ID"
    fi
  elif [ "$HTTP_CODE" == "404" ]; then
    log_fail "Position not found (HTTP $HTTP_CODE)"
  elif [ "$HTTP_CODE" == "403" ]; then
    log_fail "Not authorized or contest not running (HTTP $HTTP_CODE)"
  else
    log_fail "Unexpected response (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
  fi

  # Wait for processing
  log_info "Waiting 2 seconds for order processing..."
  sleep 2

  # Test 8: Try to close again (should fail)
  log_header "Test 8: Double Close Attempt"
  RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
    "$BASE_URL/api/trade/positions/$POSITION_ID/close" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{}' 2>/dev/null)

  HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
  BODY=$(echo "$RESPONSE" | head -n-1)

  if [ "$HTTP_CODE" == "400" ] || [ "$HTTP_CODE" == "404" ]; then
    log_success "Double close prevented (HTTP $HTTP_CODE)"
  else
    log_fail "Double close should fail (got HTTP $HTTP_CODE)"
    echo "Response: $BODY"
  fi
else
  log_skip "Interactive position close test (no position_id provided)"
  ((SKIPPED+=2))
fi

# ========================================
# Summary
# ========================================

log_header "Test Summary"
TOTAL=$((PASSED + FAILED + SKIPPED))
echo -e "  ${GREEN}Passed:${NC}  $PASSED"
echo -e "  ${RED}Failed:${NC}  $FAILED"
echo -e "  ${YELLOW}Skipped:${NC} $SKIPPED"
echo -e "  ${BLUE}Total:${NC}   $TOTAL"
echo ""

if [ $FAILED -eq 0 ]; then
  echo -e "${GREEN}All tests passed!${NC}"
  exit 0
else
  echo -e "${RED}Some tests failed.${NC}"
  exit 1
fi
