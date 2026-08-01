# Close Position Feature - QA Test Plan

## Overview

This document provides a comprehensive test plan for the Close Position feature in the Tragge trading platform. The feature allows users to close open positions (fully or partially) during a running contest.

**Components Under Test:**
- Frontend: `apps/frontend/src/components/EnhancedPositionsPanel.vue`
- API: `apps/trade-bff` - `POST /api/trade/positions/{position_id}/close`
- Engine: `apps/trading-engine` - `ProcessClosePosition()`
- Kafka Topics: `orders.v1`, `position_closed.v1`, `fills.v1`

---

## Prerequisites

### Environment Setup
```bash
# Start all services
make up
make migrate-up

# Start backend services
make dev-user-bff &
make dev-trade-bff &
make dev-trading-engine &
make dev-market-ingestor &
make dev-leaderboard-worker &

# Start frontend
make dev-frontend
```

### Test User Setup
1. Register a new user via frontend or API
2. Ensure user is enrolled in an active (running) contest
3. Note the user's JWT token for API testing
4. Have sufficient `qty_available` balance (default: 1,000,000)

### Tools Required
- Browser with DevTools (Network + Console tabs)
- PostgreSQL client (psql, DBeaver, or similar)
- Kafka/Redpanda Console (http://localhost:8088)
- REST client (curl, Postman, or similar)

---

## Test Scenarios

### 1. Basic Close Position (Happy Path)

**Objective:** Verify a position can be closed via the UI

**Steps:**
1. Open frontend and log in
2. Select a running contest
3. Place a MARKET BUY order for AAPL, qty=100
4. Wait for order fill (check Fills tab)
5. Navigate to Positions tab
6. Hover over the AAPL position row
7. Click the "Close" button (X icon)
8. Verify confirmation dialog appears showing:
   - Symbol: AAPL
   - Side: Long
   - Size: 100
   - Current P&L (may be 0 if price unchanged)
9. Click "Confirm" button
10. Verify position disappears from list

**Expected Results:**
- [ ] Close button visible on hover
- [ ] Confirmation dialog shows correct position details
- [ ] Position removed from Positions tab after close
- [ ] Success toast notification displayed
- [ ] Close fill appears in History/Fills tab
- [ ] `qty_available` increases by 100

**Database Verification:**
```sql
-- Check position is closed
SELECT position_id, symbol, side, qty_open, closed_at, realized_score
FROM positions
WHERE position_id = '<POSITION_ID>';
-- Expected: qty_open=0, closed_at IS NOT NULL

-- Check close order was created
SELECT order_id, symbol, side, type, status, qty
FROM orders
WHERE position_id = '<POSITION_ID>' AND type = 'market'
ORDER BY created_at DESC LIMIT 1;
-- Expected: status='filled', side='sell' (opposite of position)

-- Check fill record
SELECT fill_id, order_id, price, qty, created_at
FROM fills
WHERE order_id = '<CLOSE_ORDER_ID>';
-- Expected: qty=100

-- Check participant balance
SELECT user_id, qty_available, realized_score
FROM contest_participants
WHERE user_id = '<USER_ID>' AND contest_id = '<CONTEST_ID>';
-- Expected: qty_available increased by 100
```

---

### 2. Close with Profit

**Objective:** Verify P&L calculation when closing at a higher price

**Steps:**
1. Note current AAPL price (e.g., $150.00)
2. Open LONG position: MARKET BUY AAPL qty=50
3. Wait for price increase (or use test price injection)
4. Close the position when price is higher (e.g., $152.00)
5. Calculate expected P&L: (152.00 - 150.00) × 50 = $100.00

**Expected Results:**
- [ ] Confirmation dialog shows positive P&L (green)
- [ ] Position closed successfully
- [ ] Realized P&L matches calculation
- [ ] Leaderboard score increases

**Database Verification:**
```sql
SELECT realized_pnl, realized_score FROM positions
WHERE position_id = '<POSITION_ID>';
-- Expected: realized_pnl > 0
```

---

### 3. Close with Loss

**Objective:** Verify P&L calculation when closing at a lower price

**Steps:**
1. Note current AAPL price (e.g., $150.00)
2. Open LONG position: MARKET BUY AAPL qty=50
3. Wait for price decrease (or use test price injection)
4. Close the position when price is lower (e.g., $148.00)
5. Calculate expected P&L: (148.00 - 150.00) × 50 = -$100.00

**Expected Results:**
- [ ] Confirmation dialog shows negative P&L (red)
- [ ] Position closed successfully
- [ ] Realized P&L is negative
- [ ] Leaderboard score decreases

---

### 4. Close Short Position

**Objective:** Verify closing a short (sell) position

**Steps:**
1. Note current AAPL price (e.g., $150.00)
2. Open SHORT position: MARKET SELL AAPL qty=25
3. Close the position
4. Verify P&L calculation for short:
   - If price dropped to $148: P&L = (150.00 - 148.00) × 25 = +$50 (profit)
   - If price rose to $152: P&L = (150.00 - 152.00) × 25 = -$50 (loss)

**Expected Results:**
- [ ] Confirmation dialog shows "Short" position type
- [ ] Close order is a BUY (opposite of short)
- [ ] P&L calculation is correct for short position
- [ ] Position removed from list

---

### 5. Partial Close

**Objective:** Verify closing only a portion of a position

**Steps:**
1. Open position: MARKET BUY AAPL qty=100
2. Note position_id from database or DevTools
3. Use API to close partial quantity:
```bash
curl -X POST "http://localhost:8082/api/trade/positions/<POSITION_ID>/close" \
  -H "Authorization: Bearer <JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"qty": 40}'
```
4. Verify response: `{"order_id": "...", "message": "Close position order submitted"}`

**Expected Results:**
- [ ] Position remains in Positions tab with qty=60
- [ ] 40 units returned to qty_available
- [ ] Partial P&L calculated and recorded
- [ ] Fill record shows qty=40
- [ ] Can close remaining 60 units later

**Database Verification:**
```sql
SELECT qty_open FROM positions WHERE position_id = '<POSITION_ID>';
-- Expected: qty_open = 60

SELECT qty_available FROM contest_participants
WHERE user_id = '<USER_ID>' AND contest_id = '<CONTEST_ID>';
-- Expected: increased by 40
```

---

### 6. Full Close via API (qty=0)

**Objective:** Verify that qty=0 or omitting qty closes entire position

**Steps:**
1. Open position: MARKET BUY AAPL qty=75
2. Close via API without qty:
```bash
curl -X POST "http://localhost:8082/api/trade/positions/<POSITION_ID>/close" \
  -H "Authorization: Bearer <JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{}'
```

**Expected Results:**
- [ ] Entire position (75 units) closed
- [ ] Position marked as closed (closed_at set)
- [ ] All 75 units returned to qty_available

---

### 7. Error Case: Non-Existent Position

**Objective:** Verify proper error handling for invalid position ID

**Steps:**
```bash
curl -X POST "http://localhost:8082/api/trade/positions/00000000-0000-0000-0000-000000000000/close" \
  -H "Authorization: Bearer <JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{}'
```

**Expected Results:**
- [ ] HTTP 404 Not Found
- [ ] Response: `{"error": "Position not found"}`

---

### 8. Error Case: Another User's Position

**Objective:** Verify users cannot close others' positions

**Steps:**
1. Create position with User A
2. Get position_id
3. Try to close with User B's token:
```bash
curl -X POST "http://localhost:8082/api/trade/positions/<USER_A_POSITION_ID>/close" \
  -H "Authorization: Bearer <USER_B_JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{}'
```

**Expected Results:**
- [ ] HTTP 403 Forbidden
- [ ] Response: `{"error": "Not authorized to close this position"}`

---

### 9. Error Case: Contest Not Running

**Objective:** Verify positions cannot be closed when contest is paused/ended

**Steps:**
1. Create a position in a running contest
2. Pause or end the contest via admin API
3. Try to close the position

**Expected Results:**
- [ ] HTTP 403 Forbidden
- [ ] Response: `{"error": "Contest is not running"}`

---

### 10. Error Case: Already Closed Position

**Objective:** Verify cannot close an already-closed position

**Steps:**
1. Open and close a position
2. Try to close again using the same position_id

**Expected Results:**
- [ ] HTTP 400 Bad Request or 404 Not Found
- [ ] Response indicates position is already closed

---

### 11. Error Case: Invalid Quantity

**Objective:** Verify validation of close quantity

**Test Cases:**
```bash
# Negative quantity
curl -X POST "http://localhost:8082/api/trade/positions/<POSITION_ID>/close" \
  -H "Authorization: Bearer <JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"qty": -10}'

# Quantity exceeds position size
curl -X POST "http://localhost:8082/api/trade/positions/<POSITION_ID>/close" \
  -H "Authorization: Bearer <JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"qty": 999999}'

# Zero quantity (should close all - this is valid!)
curl -X POST "http://localhost:8082/api/trade/positions/<POSITION_ID>/close" \
  -H "Authorization: Bearer <JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"qty": 0}'
```

**Expected Results:**
- [ ] Negative qty: HTTP 400 `{"error": "Quantity must be positive"}`
- [ ] Exceeds size: HTTP 400 `{"error": "Quantity exceeds position size"}`
- [ ] Zero qty: HTTP 202 Accepted (closes entire position)

---

### 12. Concurrent Close Attempts

**Objective:** Verify only one close is processed when clicking rapidly

**Steps:**
1. Open a position
2. Open browser DevTools Network tab
3. Rapidly click the Close button 5+ times
4. Check Network tab for API calls

**Expected Results:**
- [ ] UI shows loading state, preventing additional clicks
- [ ] Only 1 API call made (button should be disabled while closing)
- [ ] No duplicate fills created
- [ ] Position closed exactly once

**Database Verification:**
```sql
SELECT COUNT(*) FROM fills WHERE position_id = '<POSITION_ID>';
-- Expected: 2 (open fill + 1 close fill)
```

---

### 13. WebSocket Updates

**Objective:** Verify real-time UI updates via WebSocket

**Steps:**
1. Open two browser tabs with frontend
2. In Tab 1: Open a position
3. Verify position appears in Tab 2 (via WebSocket)
4. In Tab 1: Close the position
5. Verify position disappears in Tab 2 without refresh

**Expected Results:**
- [ ] Position appears in both tabs after open
- [ ] Position disappears from both tabs after close
- [ ] No manual refresh needed
- [ ] WebSocket message type: `position_update`

**Console Verification:**
Open DevTools Console before test:
```javascript
// Listen for WebSocket messages (add to console before trading)
// This assumes you can access the WebSocket instance
```

---

### 14. UI State During Close

**Objective:** Verify proper loading states in UI

**Steps:**
1. Open a position
2. Click Close button
3. Observe UI during API call

**Expected Results:**
- [ ] Close button shows loading spinner
- [ ] Button is disabled during close
- [ ] Other positions' close buttons remain functional
- [ ] Position row may show "Closing..." state
- [ ] Confirmation dialog closes on success

---

### 15. Cancel Close Confirmation

**Objective:** Verify canceling the close dialog

**Steps:**
1. Open a position
2. Click Close button
3. Confirmation dialog appears
4. Click "Cancel" button

**Expected Results:**
- [ ] Dialog closes
- [ ] Position remains open
- [ ] No API call made
- [ ] Can click Close again

---

### 16. Close at Market vs Limit Price

**Objective:** Verify close order executes at market price

**Steps:**
1. Open position at price $150.00
2. Wait for price to change to $151.50
3. Close position
4. Check fill price

**Expected Results:**
- [ ] Close fill price matches current market price ($151.50)
- [ ] Not the original entry price
- [ ] P&L calculated correctly with new price

---

### 17. Multiple Positions - Same Symbol

**Objective:** Verify closing one position doesn't affect others

**Steps:**
1. Open Position A: BUY AAPL qty=50
2. Open Position B: BUY AAPL qty=30
3. Close Position A only
4. Verify Position B remains open

**Expected Results:**
- [ ] Only Position A closed
- [ ] Position B qty unchanged (30)
- [ ] Both positions have different position_ids

---

### 18. Position with Active TP/SL

**Objective:** Verify TP/SL orders are cancelled when position is manually closed

**Steps:**
1. Open position with TP and SL set
2. Close position manually before TP/SL triggers
3. Verify TP/SL orders are cancelled

**Expected Results:**
- [ ] Position closed at manual close price
- [ ] Associated TP order cancelled
- [ ] Associated SL order cancelled
- [ ] No orphaned pending orders remain

---

## Performance Tests

### P1. Close Latency

**Objective:** Measure end-to-end close latency

**Steps:**
1. Record timestamp before clicking Close
2. Record timestamp when position disappears from UI
3. Calculate difference

**Target:** < 500ms for 95th percentile

### P2. High Volume Close

**Objective:** Verify system handles multiple simultaneous closes

**Steps:**
1. Create 10 positions for same user
2. Close all 10 rapidly (or via script)
3. Verify all close successfully

**Expected Results:**
- [ ] All 10 positions closed
- [ ] No errors or timeouts
- [ ] Correct P&L for each

---

## Integration Test Script

Save as `tests/manual/close-position-test.sh`:

```bash
#!/bin/bash
# Close Position Integration Test Script

BASE_URL="http://localhost:8082"
USER_EMAIL="test@example.com"
USER_PASSWORD="TestPass123!"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

# Login and get token
echo "Logging in..."
TOKEN=$(curl -s -X POST "http://localhost:8081/api/user/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"$USER_EMAIL\", \"password\": \"$USER_PASSWORD\"}" \
  | jq -r '.access_token')

if [ "$TOKEN" == "null" ] || [ -z "$TOKEN" ]; then
  echo -e "${RED}Login failed${NC}"
  exit 1
fi
echo -e "${GREEN}Login successful${NC}"

# Get active contest
CONTEST_ID=$(curl -s "$BASE_URL/api/trade/contests/active" \
  -H "Authorization: Bearer $TOKEN" \
  | jq -r '.[0].contest_id')

echo "Contest ID: $CONTEST_ID"

# Open a position (via WebSocket order - simplified here)
echo "Note: Open a position via the UI, then enter the position_id"
read -p "Enter position_id to test: " POSITION_ID

# Test 1: Close position
echo "Test 1: Closing position..."
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  "$BASE_URL/api/trade/positions/$POSITION_ID/close" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}')

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | head -n-1)

if [ "$HTTP_CODE" == "202" ]; then
  echo -e "${GREEN}Test 1 PASSED: Position close submitted${NC}"
  echo "Response: $BODY"
else
  echo -e "${RED}Test 1 FAILED: Expected 202, got $HTTP_CODE${NC}"
  echo "Response: $BODY"
fi

# Test 2: Try to close again (should fail)
echo "Test 2: Attempting to close already-closed position..."
sleep 2  # Wait for processing

RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  "$BASE_URL/api/trade/positions/$POSITION_ID/close" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}')

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
if [ "$HTTP_CODE" == "400" ] || [ "$HTTP_CODE" == "404" ]; then
  echo -e "${GREEN}Test 2 PASSED: Cannot close already-closed position${NC}"
else
  echo -e "${RED}Test 2 FAILED: Expected 400/404, got $HTTP_CODE${NC}"
fi

# Test 3: Invalid position ID
echo "Test 3: Invalid position ID..."
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  "$BASE_URL/api/trade/positions/invalid-uuid/close" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}')

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
if [ "$HTTP_CODE" == "400" ]; then
  echo -e "${GREEN}Test 3 PASSED: Invalid UUID rejected${NC}"
else
  echo -e "${RED}Test 3 FAILED: Expected 400, got $HTTP_CODE${NC}"
fi

echo "Tests completed!"
```

---

## Database Verification Queries

Run these queries after testing to verify data integrity:

```sql
-- Summary of all positions for a contest
SELECT
  p.position_id,
  u.email,
  p.symbol,
  p.side,
  p.qty AS original_qty,
  p.qty_open AS remaining_qty,
  p.avg_price,
  p.realized_score,
  p.closed_at,
  CASE WHEN p.closed_at IS NOT NULL THEN 'CLOSED' ELSE 'OPEN' END as status
FROM positions p
JOIN users u ON p.user_id = u.user_id
WHERE p.contest_id = '<CONTEST_ID>'
ORDER BY p.created_at DESC;

-- All fills for closed positions
SELECT
  f.fill_id,
  o.symbol,
  o.side,
  o.type,
  f.price,
  f.qty,
  f.created_at,
  p.position_id,
  CASE WHEN p.closed_at IS NOT NULL THEN 'POSITION_CLOSED' ELSE 'POSITION_OPEN' END
FROM fills f
JOIN orders o ON f.order_id = o.order_id
LEFT JOIN positions p ON o.position_id = p.position_id
WHERE o.contest_id = '<CONTEST_ID>'
ORDER BY f.created_at DESC;

-- Participant balance check
SELECT
  u.email,
  cp.qty_available,
  cp.realized_score,
  cp.unrealized_score
FROM contest_participants cp
JOIN users u ON cp.user_id = u.user_id
WHERE cp.contest_id = '<CONTEST_ID>';

-- Orphaned TP/SL check (should be empty after close)
SELECT * FROM orders
WHERE position_id = '<CLOSED_POSITION_ID>'
  AND type IN ('take_profit', 'stop_loss')
  AND status = 'pending';
```

---

## Sign-Off Checklist

| Test # | Test Name | Pass/Fail | Tester | Date | Notes |
|--------|-----------|-----------|--------|------|-------|
| 1 | Basic Close (Happy Path) | | | | |
| 2 | Close with Profit | | | | |
| 3 | Close with Loss | | | | |
| 4 | Close Short Position | | | | |
| 5 | Partial Close | | | | |
| 6 | Full Close via API (qty=0) | | | | |
| 7 | Error: Non-Existent Position | | | | |
| 8 | Error: Another User's Position | | | | |
| 9 | Error: Contest Not Running | | | | |
| 10 | Error: Already Closed Position | | | | |
| 11 | Error: Invalid Quantity | | | | |
| 12 | Concurrent Close Attempts | | | | |
| 13 | WebSocket Updates | | | | |
| 14 | UI State During Close | | | | |
| 15 | Cancel Close Confirmation | | | | |
| 16 | Close at Market vs Limit Price | | | | |
| 17 | Multiple Positions - Same Symbol | | | | |
| 18 | Position with Active TP/SL | | | | |
| P1 | Close Latency | | | | |
| P2 | High Volume Close | | | | |

---

## Appendix: API Reference

### Close Position Endpoint

```
POST /api/trade/positions/{position_id}/close
```

**Headers:**
- `Authorization: Bearer <JWT_TOKEN>` (required)
- `Content-Type: application/json`

**Path Parameters:**
- `position_id` (UUID) - The position to close

**Request Body:**
```json
{
  "qty": 50  // Optional: quantity to close. Omit or 0 for full close.
}
```

**Success Response (202 Accepted):**
```json
{
  "order_id": "123e4567-e89b-12d3-a456-426614174000",
  "message": "Close position order submitted"
}
```

**Error Responses:**
- `400 Bad Request` - Invalid position_id format or quantity
- `401 Unauthorized` - Missing or invalid JWT
- `403 Forbidden` - Not authorized or contest not running
- `404 Not Found` - Position not found

---

*Document Version: 1.0*
*Last Updated: January 2026*
*Author: QA Team*
