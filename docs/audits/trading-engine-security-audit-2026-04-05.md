# Trading Engine Security & Functional Audit

**Date**: 2026-04-05
**Scope**: `apps/trading-engine/server/*.go` (all non-test Go source files)
**Auditor**: Claude Code (automated)

## Executive Summary

The trading engine is well-architected with strong security controls. No critical or high-severity issues were found. Position ownership validation, contest boundary enforcement, quantity validation, and race condition protection are all correctly implemented. Three low-severity gaps were identified around input validation of TP/SL and limit/stop price values.

## Security Findings

### S-1: No TP/SL Price Validation [LOW]

**Files**: `order_processing.go:439-441`, `order_processing.go:1489-1506`

When a user sets Take Profit or Stop Loss via `ProcessOrder` or `ProcessModifyTPSL`, there is no validation that TP/SL values are sensible relative to the position direction:

- For LONG: TP should be > entry price, SL should be < entry price
- For SHORT: TP should be < entry price, SL should be > entry price

A user could set SL above entry for a LONG position, causing immediate SL trigger on the next tick. Setting TP=0 or negative values is also possible.

**Impact**: Low — affects only the user's own positions. Could cause confusing behavior in tournaments.

**Recommendation**: Add TP/SL direction validation in `ProcessModifyTPSL` and when processing orders with TP/SL:

```go
if order.Side == contracts.OrderSideBuy { // LONG
    if order.TakeProfit != nil && *order.TakeProfit <= fillPrice {
        return rejectOrder(ctx, order, "take profit must be above entry price for long positions")
    }
    if order.StopLoss != nil && *order.StopLoss >= fillPrice {
        return rejectOrder(ctx, order, "stop loss must be below entry price for long positions")
    }
}
```

### S-2: No LimitPrice/StopPrice Range Validation [LOW]

**File**: `order_processing.go:140-141`

`InsertOrder` accepts `LimitPrice` and `StopPrice` from the order request without validating they are positive or within reasonable ranges. A zero or negative limit price would be stored in the database.

For SellLimit orders in `pending.go:285` (`bid >= *order.LimitPrice`), a limit price of 0 would trigger immediately on any tick with a positive bid price.

**Impact**: Low — user can only affect their own orders, and the fill price is the market price (bid/ask), not the limit price.

**Recommendation**: Validate that `LimitPrice > 0` and `StopPrice > 0` when present:

```go
if order.LimitPrice != nil && *order.LimitPrice <= 0 {
    return rejectOrder(ctx, order, "limit price must be positive")
}
if order.StopPrice != nil && *order.StopPrice <= 0 {
    return rejectOrder(ctx, order, "stop price must be positive")
}
```

### S-3: Position Ownership Validation [PASS]

**Files**: `order_processing.go:960-972`, `order_processing.go:1465-1476`

`ProcessClosePosition` and `ProcessModifyTPSL` both validate:
- `dbPos.UserID != req.UserID` → rejects with "position does not belong to user"
- `dbPos.ContestID != req.ContestID` → rejects with "position is not in specified contest"

**Result**: Properly implemented. No bypass possible.

### S-4: Contest Boundary Enforcement [PASS]

**File**: `order_processing.go:58-74`

- Contest status checked (`status != "running"` → reject)
- Start/end time validated against `time.Now()`
- Fresh DB re-check before fill commit (lines 374-382, 609-633)
- Symbol allowlist validated (lines 99-111)
- Shard assignment validated (lines 51-55)

**Result**: Properly implemented with defense-in-depth.

### S-5: Order Quantity Validation [PASS]

**File**: `order_processing.go:230-265`

- `qty <= 0` → rejected
- `qty > MaxAllowedQty (10,000,000)` → rejected
- Min per trade enforced
- Max percentage of total enforced with overflow protection

**Result**: Properly implemented.

## Functional Findings

### F-1: Float64 Used for Price Storage [INFO]

**Files**: Throughout (`db.go:53-62`, `state.go:25-36`, `decimal_scoring.go:96-102`)

Position entry prices are stored as `float64`. The scoring system correctly uses `shopspring/decimal`, but weighted average entry price calculation converts back to `float64`.

**Impact**: Minor precision loss (float64 has ~15 significant digits). Acceptable for a trading tournament platform. For real money, `decimal` throughout would be needed.

### F-2: WAL Async Persistence Gap [LOW]

**File**: `wal.go:186-191`, `wal.go:310-327`

The WAL's async flusher batches writes every 10ms. On process crash, up to 10ms of entries may be lost from the file. When the channel is full, falls back to synchronous write.

**Impact**: Low — the database is the source of truth. State is fully reloaded from DB on startup via `StateReloader`. WAL is a secondary consistency mechanism.

### F-3: WAL Compact Disables Async Flusher [LOW]

**File**: `wal.go:654-666`

`Compact()` permanently closes the flusher goroutine. After compaction, all subsequent writes use synchronous fsync per entry, degrading write latency.

**Recommendation**: Restart the async flusher after compaction.

### F-4: Race Condition Protection [PASS]

**Files**: `position_lock.go`, `order_processing.go`, `db.go:232-255`

- `AcquireLockForSymbolWithTimeout` acquires both "long" and "short" locks in consistent lexicographic order (prevents deadlocks)
- Position re-read from DB after lock acquisition
- `SELECT ... FOR UPDATE` in `GetOpenPositionTx` for defense-in-depth
- 5-second lock timeout with cleanup goroutine

**Result**: Well implemented.

### F-5: WAL Correctness (Crash Recovery) [PASS]

**Files**: `wal.go`, `state_consistency.go`, `engine.go:468-567`

- WAL entries written before state mutation
- `MarkCommitted` called after successful mutation
- On crash: `ReplayWAL` replays pending entries on startup
- `StateReloader.ReloadState` fully rebuilds from DB as fallback
- `PendingOrderBook.ReloadFromDB` restores pending orders from DB
- Divergence detection and auto-reload mechanism

**Result**: Properly implemented with multiple recovery paths.

### F-6: Pending Order Evaluation [PASS]

**File**: `pending.go:262-375`

- BuyLimit: triggers when `ask <= limitPrice` (correct)
- SellLimit: triggers when `bid >= limitPrice` (correct)
- BuyStop: triggers when `ask >= stopPrice` (correct)
- SellStop: triggers when `bid <= stopPrice` (correct)
- TP/SL: LONG exits at bid, SHORT exits at ask (correct)
- Stale price retry mechanism prevents fills on stale data
- Triggered orders are copied to avoid pointer aliasing

**Result**: Correctly implemented.

### F-7: Kafka Consumer Offset Management [PASS]

**File**: `consumer_sharded.go`

- Manual commit with `DisableAutoCommit()` + `CommitUncommittedOffsets()`
- Deterministic partition assignment (modulo-based)
- Lag monitoring with Prometheus metrics
- Error metrics for fetch, commit, and processing failures

**Result**: Correctly implemented.

### F-8: Price Book Consistency [PASS]

**File**: `pricebook.go`

- Synthetic bid/ask derived from last price + spread when provider sends zero
- Proper mutex locking (RLock for reads, Lock for writes)
- Direct scalar access on hot paths (`GetBidAskDirect`)
- Timestamp normalization handles seconds vs milliseconds

**Result**: Correctly implemented.

## Summary Table

| ID | Severity | Category | Status | Description |
|----|----------|----------|--------|-------------|
| S-1 | LOW | Security | Finding | No TP/SL price direction validation |
| S-2 | LOW | Security | Finding | No limit/stop price range validation |
| S-3 | — | Security | PASS | Position ownership validation |
| S-4 | — | Security | PASS | Contest boundary enforcement |
| S-5 | — | Security | PASS | Order quantity validation |
| F-1 | INFO | Functional | Note | Float64 for prices (acceptable) |
| F-2 | LOW | Functional | Finding | WAL async persistence gap |
| F-3 | LOW | Functional | Finding | WAL compact disables flusher |
| F-4 | — | Functional | PASS | Race condition protection |
| F-5 | — | Functional | PASS | WAL crash recovery |
| F-6 | — | Functional | PASS | Pending order evaluation |
| F-7 | — | Functional | PASS | Kafka offset management |
| F-8 | — | Functional | PASS | Price book consistency |

**Critical: 0 | High: 0 | Medium: 0 | Low: 4 | Info: 1 | Pass: 8**
