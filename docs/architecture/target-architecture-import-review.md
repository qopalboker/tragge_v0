# Target architecture dependency review

**Review date:** 2026-07-26

**Decision under review:**
[ADR-0001: Target runtime architecture](../adr/0001-target-runtime-architecture.md)

**Implementation status:** Target not yet implemented

## Purpose and scope

This review maps current Go source imports to ADR-0001's target dependency
rules. It is static evidence for `FND-002`; it does not change imports or
application behavior.

The review scanned Go files below `apps/` and `packages/` for imports rooted at
`github.com/Parsaeffatravesh/tragge/`, grouped each import by source and target
module, and classified app-to-app and package-to-app edges. Generated files
were not separately excluded. SQL privileges, runtime HTTP calls, broker
behavior, and dynamic configuration cannot be proven by an import scan and
remain covered by the [current-state audit](current-state-audit.md) and later
executable architecture tests.

Reproduction from the repository root:

```powershell
rg -n -F 'github.com/Parsaeffatravesh/tragge/apps/' --glob '*.go' apps packages
rg -n -F 'github.com/Parsaeffatravesh/tragge/packages/' --glob '*.go' apps packages
node scripts/target-architecture.test.mjs
```

## Observed import graph

The classified scan found:

- 506 internal Go import lines (SEC-005 adds sixteen in-boundary imports that
  route logging through the shared observability package; SEC-006 has a final
  net addition of six in-boundary imports after provider retirement);
- 176 distinct source-module to target-module pairs;
- 13 app-to-app pairs, of which three are same-module imports and ten are
  cross-module imports; and
- zero package-to-app pairs.

SEC-001 accounts for eleven additional internal import lines used by the
isolated auth implementation and tests. SEC-002 adds two `packages/auth`
imports in the trading WebSocket ticket implementation and its focused test.
SEC-003 adds eight internal import lines for explicit security-code delivery
interfaces and focused lifecycle/integration tests. It creates no new module edge.
SEC-004 adds seven approved in-boundary import occurrences without changing an
edge. SEC-005 adds sixteen import occurrences and seven shared-package edges.
SEC-006 has a final net addition of six imports for the shared validation and
rate-limit packages after provider retirement, without adding a new
source-module to target-module pair.
SEC-007 adds five in-boundary import occurrences for Admin MFA implementation
and real runtime tests without adding a source-module to target-module edge.
`apps/api-server`, `apps/worker`, and `apps/trading-core` now import
`packages/observability` directly, while `packages/audit`, `packages/config`,
`packages/notification`, and `packages/resilience` use it for centralized
redaction. These neutral utility edges do not change a bounded system boundary.
Across the completed security tasks, the eighth new in-boundary pair is
`packages/validation -> packages/auth`; package-to-app and the ten cross-module
application edges are unchanged.
The ten cross-module app edges are:

| Source module | Imported module | Evidence | Target classification |
|---|---|---|---|
| `apps/api-server` | `apps/admin-bff` | [`apps/api-server/main.go`](../../apps/api-server/main.go) | Transitional wrapper edge; remove with wrapper retirement. |
| `apps/api-server` | `apps/payment-service` | [`apps/api-server/main.go`](../../apps/api-server/main.go) | Transitional wrapper edge; capability moves behind an in-process Platform application interface. |
| `apps/api-server` | `apps/user-bff` | [`apps/api-server/main.go`](../../apps/api-server/main.go) | Transitional wrapper edge; capability moves behind an in-process Platform application interface. |
| `apps/trading-core` | `apps/market-ingestor` | [`apps/trading-core/main.go`](../../apps/trading-core/main.go) | Forbidden in target; Market Data is an independent bounded system. |
| `apps/trading-core` | `apps/trade-bff` | [`apps/trading-core/main.go`](../../apps/trading-core/main.go) | Forbidden in target; trade-facing APIs/realtime belong to Platform. |
| `apps/trading-core` | `apps/trading-engine` | [`apps/trading-core/main.go`](../../apps/trading-core/main.go) | Forbidden in target; Engine is an independent bounded system. |
| `apps/worker` | `apps/contest-scheduler` | [`apps/worker/main.go`](../../apps/worker/main.go) | Transitional wrapper edge; capability becomes a Platform worker module. |
| `apps/worker` | `apps/free-contest-generator` | [`apps/worker/main.go`](../../apps/worker/main.go) | Transitional wrapper edge; capability becomes a Platform worker module. |
| `apps/worker` | `apps/leaderboard-worker` | [`apps/worker/main.go`](../../apps/worker/main.go) | Transitional wrapper edge; capability becomes a Platform worker module. |
| `apps/worker` | `apps/settlement-service` | [`apps/worker/main.go`](../../apps/worker/main.go) | Transitional wrapper edge; capability becomes a Platform worker module. |

Same-module imports within `contest-scheduler`, `payment-service`, and
`user-bff` are ordinary internal module dependencies and are not counted as
cross-module edges.

## Rule-by-rule review

| ADR dependency rule | Current evidence | Target conformance |
|---|---|---|
| Exactly three bounded systems | The current audit records 14 Go app modules plus merged and standalone deployment generations. | Not conforming; the ADR is the target, not a description of the archive. |
| No bounded system imports another system's implementation | All ten cross-module app edges are composition imports in `api-server`, `trading-core`, or `worker`. | Transitional only; all ten must be removed. |
| Shared packages never import applications | The scan found zero `packages/*` to `apps/*` pairs. | Conforming dependency floor to preserve. |
| Platform modules communicate through in-process interfaces, not HTTP | Platform responsibilities remain shaped as BFF/service/worker applications composed by wrappers. Static imports alone cannot prove all runtime calls. | Not yet demonstrated; module interfaces and runtime integration tests are required during migration. |
| Cross-system interaction uses versioned commands/events and outbox/inbox | `packages/contracts` is widely imported, but import presence does not prove envelope metadata, durable outbox/inbox, compatibility handling, or idempotency. | Not yet demonstrated; later contract and integration tests are required. |
| Domain state belongs to one bounded system | Shared packages such as `packages/domain`, `packages/scoring`, and `packages/wallet` are imported by multiple current applications. | Ownership is ambiguous; relocate owner-specific behavior or narrow packages to neutral contracts/utilities. |
| Runtime SQL is schema/credential isolated | The current audit records shared pools and no `platform`, `engine`, and `market_data` ownership enforcement. | Not conforming; database-role and privilege tests are required before cutover. |

## Migration constraints derived from the review

1. Preserve the current zero package-to-app dependency direction.
2. Do not add cross-app implementation imports while migrating.
3. Remove the ten wrapper edges through composition-root replacement, not by
   hiding them behind a generic package.
4. Classify every shared package as Platform-owned, Engine-owned, Market
   Data-owned, a versioned contract, or a bounded-system-neutral utility before
   target cutover.
5. Do not treat moving a package as proof of runtime isolation. Schema
   privileges, outbox/inbox behavior, failure modes, and deployment artifacts
   require separate executable evidence.
6. Delete `api-server`, `trading-core`, and `worker` only after their routes,
   jobs, images, manifests, and runbook references have moved and their rollback
   window has closed.

## Review conclusion

The graph contains a useful dependency floor - shared packages do not import
applications - but it does not conform to the target architecture. The three
wrapper entrypoints own every cross-module app edge and make their transitional
status directly observable. ADR-0001 requires those wrappers and all ten edges
to be retired through staged, tested migration.
