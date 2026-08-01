# Contracts

Versioned contracts for the tragge event-driven trading platform.

> **Legacy compatibility status:** the existing `v1` schemas and types describe
> current repository traffic; they are not blanket approval of their financial,
> market-data, lifecycle, or naming semantics. New target contracts follow the
> [canonical domain glossary and version catalog](../../docs/product/canonical-domain-glossary-and-version-catalog.md)
> and the
> [fixed product and technical policies](../../docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md).
> `DATA-001`, `MD-001`, `ENG-001`, and `ARCH-006` own the target fixed-point and
> envelope-complete replacements.

## Structure

```
contracts/
├── schemas/           # JSON Schema definitions
│   ├── tick_snapshot.v1.json
│   ├── order_request.v1.json
│   ├── order_ack.v1.json
│   ├── fill_event.v1.json
│   ├── position_update.v1.json
│   ├── pnl_delta.v1.json
│   └── contest_state.v1.json
├── v1/                # Go types (package v1)
│   ├── enums.go
│   ├── tick_snapshot.go
│   ├── order_request.go
│   ├── order_ack.go
│   ├── fill_event.go
│   ├── position_update.go
│   ├── pnl_delta.go
│   └── contest_state.go
└── ts/                # TypeScript types
    ├── v1/
    │   ├── enums.ts
    │   ├── tick-snapshot.ts
    │   ├── order-request.ts
    │   ├── order-ack.ts
    │   ├── fill-event.ts
    │   ├── position-update.ts
    │   ├── pnl-delta.ts
    │   ├── contest-state.ts
    │   └── index.ts
    └── index.ts
```

## Versioning Rules

### Semantic Versioning

Each contract version follows these rules:

1. **Version Format**: `v{major}` (e.g., `v1`, `v2`)

2. **Breaking Changes** require a new major version:
   - Removing a required field
   - Changing a field's type
   - Renaming a field
   - Changing enum values
   - Making an optional field required

3. **Non-Breaking Changes** are allowed within a version:
   - Adding new optional fields
   - Adding new enum values (if consumers handle unknown values)
   - Adding new event types

### Compatibility Guarantees and Legacy Limits

- **Producers** MUST NOT send fields not defined in the schema
- **Consumers** MUST ignore unknown fields (forward compatibility)
- **Consumers** MUST handle missing optional fields gracefully
- **Timestamps** are always Unix milliseconds (int64)
- **IDs** are always strings (UUIDs recommended)
- **Legacy v1 prices** are `float64`/`number`; this representation is
  noncanonical for target financial/execution boundaries. `DATA-001` and
  `MD-001` replace it with explicit fixed-point serialization.
- **Trading QTY** is integer-only and uses qualified `qty` names. It never means
  Real Participant count or Participant Capacity.
- **Deprecated Contest fields** such as `commission_rate` and
  `max_participants` remain legacy compatibility evidence only. New contracts
  use `platform_fee_bps` and have no product-level Participant Capacity.

### Adding a New Version

1. Create new schema files: `{event}.v2.json`
2. Create new Go package: `v2/`
3. Create new TypeScript module: `ts/v2/`
4. Update exports in `ts/index.ts`
5. Document migration path from previous version

### Deprecation Policy

1. Announce deprecation at least 2 releases before removal
2. Add `@deprecated` comments in code
3. Log warnings when deprecated versions are used
4. Remove deprecated versions only in major platform releases

## Event Types

| Event | Description |
|-------|-------------|
| `TickSnapshot` | Market tick data with bid/ask/last prices |
| `OrderRequest` | New order placement request |
| `OrderAck` | Order acceptance/rejection response |
| `FillEvent` | Order execution fill notification |
| `PositionUpdate` | User position state update |
| `PnLDelta` | Leaderboard score change |
| `ContestState` | Contest phase transition |

## Usage

### Go

```go
import v1 "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"

order := v1.OrderRequest{
    OrderID:   "uuid-here",
    UserID:    "user-123",
    ContestID: "contest-456",
    Symbol:    "BTCUSD",
    Side:      v1.OrderSideBuy,
    Type:      v1.OrderTypeMarket,
    Qty:       100,
    ClientTs:  time.Now().UnixMilli(),
}
```

### TypeScript

```typescript
import type { OrderRequest, OrderSide } from '@tragge/contracts';
// or for explicit version:
import type { OrderRequest } from '@tragge/contracts/v1';

const order: OrderRequest = {
  order_id: 'uuid-here',
  user_id: 'user-123',
  contest_id: 'contest-456',
  symbol: 'BTCUSD',
  side: 'BUY',
  type: 'MARKET',
  qty: 100,
  client_ts: Date.now(),
};
```

## Validation

JSON Schemas can be used for runtime validation:

```typescript
import Ajv from 'ajv';
import orderRequestSchema from '@tragge/contracts/schemas/order_request.v1.json';

const ajv = new Ajv();
const validate = ajv.compile(orderRequestSchema);

if (!validate(data)) {
  console.error(validate.errors);
}
```
