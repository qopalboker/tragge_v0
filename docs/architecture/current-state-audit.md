# Tragge current-state production baseline

**Task:** `FND-001`  
**Snapshot date:** `2026-07-25`  
**Policy version:** `2026-07-25.1`  
**Roadmap version:** `2026-07-25.1`  
**Paid-production decision:** **NO-GO**

This is an evidence snapshot of the extracted local archive. It does not change
product behavior and it is not evidence that any test suite, deployment, legal
gate, provider-rights gate, or production-readiness gate passes. The approved
target remains the three bounded systems in the
[fixed product and technical policies](../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md):
Platform modular monolith, independent Trading Engine, and independent Market
Data Service.

The companion
[production roadmap](../codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md) declares
the current repository **NO-GO**. The findings below retain that decision.

## Reproducing the baseline

The inventory and verifier use only Node built-ins; they do not install or add
a dependency.

```text
node scripts/production-baseline.mjs inventory
node scripts/production-baseline.test.mjs
node scripts/production-baseline.mjs verify
```

Equivalent package and Make targets are:

```text
pnpm baseline:inventory
pnpm test:baseline
pnpm baseline:verify

make baseline-inventory
make test-baseline
make baseline-verify
```

`make` was not installed in the local audit environment. The direct Node forms
are the executed evidence. The pnpm targets were attempted, but the sandbox
prevented pnpm from spawning `node.exe` (`EPERM`); the Make targets are
unexecuted convenience aliases.

## Repository inventory

The inventory was read twice consecutively and returned the same result.

| Metric | Verified count |
|---|---:|
| Go files | 375 |
| Vue files | 211 |
| TypeScript/TSX files | 178 |
| SQL files | 202 |
| Up migrations | 98 |
| Down migrations | 99 |
| Go test files | 99 |
| Frontend test/spec files | 10 |

SEC-001 preserves this approved snapshot and begins an explicit current-tree
delta ledger. SEC-001 adds seven Go files (five tests); SEC-002 adds two Go
files (one test) and two TypeScript files; SEC-003 adds twelve Go files (ten
tests) and one TypeScript file; SEC-004 adds five Go files (three tests), two
TypeScript files, two SQL files, and one up migration; SEC-005 adds eight Go
files (four tests) and one TypeScript test; SEC-006 has a final net addition of
two Go test files after retiring seven active provider Go files and adding two
retirement tests. SEC-007 adds four Go files (two tests), two TypeScript files,
two SQL files, and one up migration. P1-REM-001 adds one TypeScript Playwright
mock helper. The current working tree therefore contains 411 Go files, 123 Go
test files, 187 TypeScript/TSX files, 206 SQL files, and 100 up migrations.
Vue remains at the approved 211-file snapshot.
The 99th down migration is
[`0000_baseline.down.sql`](../../packages/db/migrations/0000_baseline.down.sql),
which has no matching up migration. The numbered implementation series has 98
up migrations and 98 corresponding down migrations through
[`0098_fix_migration_audit_issues.up.sql`](../../packages/db/migrations/0098_fix_migration_audit_issues.up.sql).
Classification or reset of migrations belongs to `FND-004`, not this task.

### Application inventory

There are 17 immediate application directories: 14 Go modules, two Node
frontends, and one Nginx gateway directory.

| Application | Current form | In `go.work` |
|---|---|---:|
| [`admin-bff`](../../apps/admin-bff) | Go module, standalone server package | Yes |
| [`admin-frontend`](../../apps/admin-frontend) | Vue/Node package, Dockerfile | n/a |
| [`api-server`](../../apps/api-server) | Go merged wrapper, `main.go`, Dockerfile | Yes |
| [`contest-scheduler`](../../apps/contest-scheduler) | Go module, standalone server package | Yes |
| [`free-contest-generator`](../../apps/free-contest-generator) | Go module, standalone server package | Yes |
| [`gateway`](../../apps/gateway) | Nginx configuration, development and production Dockerfiles | n/a |
| [`leaderboard-worker`](../../apps/leaderboard-worker) | Go module, standalone server package | Yes |
| [`market-ingestor`](../../apps/market-ingestor) | Go module, standalone server package | Yes |
| [`payment-service`](../../apps/payment-service) | Go module, standalone server package | Yes |
| [`settlement-service`](../../apps/settlement-service) | Go module, standalone server package | Yes |
| [`shard-router`](../../apps/shard-router) | Go module with `main.go`; added to the workspace by SEC-001 so its Admin validator is compiled with the shared auth package | Yes |
| [`trade-bff`](../../apps/trade-bff) | Go module, standalone server package | Yes |
| [`trading-core`](../../apps/trading-core) | Go merged wrapper, `main.go`, Dockerfile | Yes |
| [`trading-engine`](../../apps/trading-engine) | Go module, standalone server package | Yes |
| [`user-bff`](../../apps/user-bff) | Go module, standalone server package | Yes |
| [`user-frontend`](../../apps/user-frontend) | Vue/Node package, Dockerfile | n/a |
| [`worker`](../../apps/worker) | Go merged wrapper, `main.go`, Dockerfile | Yes |

### Package inventory

There are 20 immediate package directories. Nineteen contain Go modules:
[`audit`](../../packages/audit), [`auth`](../../packages/auth),
[`config`](../../packages/config), [`contracts`](../../packages/contracts),
[`db`](../../packages/db), [`domain`](../../packages/domain),
[`infra`](../../packages/infra), [`kyc`](../../packages/kyc),
[`notification`](../../packages/notification),
[`observability`](../../packages/observability), [`redis`](../../packages/redis),
[`resilience`](../../packages/resilience), [`scoring`](../../packages/scoring),
[`secrets`](../../packages/secrets), [`sms`](../../packages/sms),
[`storage`](../../packages/storage), [`ticket`](../../packages/ticket),
[`validation`](../../packages/validation), and [`wallet`](../../packages/wallet).
The twentieth is the Node package
[`frontend-shared`](../../packages/frontend-shared). The contracts directory
also contains the nested Node package
[`packages/contracts/ts`](../../packages/contracts/ts).

[`go.work`](../../go.work) includes 33 modules: 14 under `apps` and 19 under
`packages`. SEC-001 added [`apps/shard-router`](../../apps/shard-router) so
its Admin-protected routes use and compile against the explicit Admin trust
context. The Go module
[`scripts/create-admin-users`](../../scripts/create-admin-users) remains outside
the workspace. [`pnpm-workspace.yaml`](../../pnpm-workspace.yaml) includes four
packages: both frontends, TypeScript contracts, and frontend-shared.

## Current runtime and deployment topology

The source contains both standalone services and three merged executable
wrappers. With all profiles enabled,
[`infra/docker/docker-compose.yml`](../../infra/docker/docker-compose.yml)
selects PostgreSQL, Redis, Redpanda, both frontends, gateway, and the three
merged wrappers:

| Merged executable | Embedded runtimes | Shared process resources |
|---|---|---|
| [`api-server`](../../apps/api-server/main.go) | user-bff, admin-bff, payment-service | database pool, Redis client, separate User and Admin authentication contexts |
| [`trading-core`](../../apps/trading-core/main.go) | Market Ingestor, Trading Engine, trade-bff | database pool and Redis client |
| [`worker`](../../apps/worker/main.go) | leaderboard, settlement, scheduler, free contest generator | database pool and partially shared Redis client |

[`infra/k8s/base/kustomization.yaml`](../../infra/k8s/base/kustomization.yaml)
also selects those merged deployments. In contrast, the
[`production overlay`](../../infra/k8s/overlays/production/kustomization.yaml)
patches replicas and image names for standalone workloads that the base does
not create. Therefore the Kubernetes production overlay is internally
inconsistent and is not credible launch evidence.

The approved target topology is not implemented: Platform is not yet the
single modular-monolith codebase with `api`, `realtime`, and `worker` modes,
and Trading Engine and Market Data do not yet have proven independent images,
deployments, credentials, and failure domains.

## Database and migration baseline

- PostgreSQL 16, Redis 7, and Redpanda `v24.1.1` are declared by
  [Compose](../../infra/docker/docker-compose.yml).
- All 197 migration files are under
  [`packages/db/migrations`](../../packages/db/migrations).
- The archive has 98 up migrations and 99 down migrations.
- No fresh-database migration or supported upgrade migration was executed:
  the Docker daemon was unavailable and host `psql` was not installed.
- Schema ownership is not yet separated into `platform`, `engine`, and
  `market_data` roles; that is a later architecture/migration task.

## Test baseline and coverage gaps

The repository has 105 Go test files after SEC-002, but the following modules contain Go code
and no Go test file:

- [`apps/contest-scheduler`](../../apps/contest-scheduler),
  [`apps/free-contest-generator`](../../apps/free-contest-generator),
  [`apps/settlement-service`](../../apps/settlement-service),
  [`apps/shard-router`](../../apps/shard-router),
  [`apps/trading-core`](../../apps/trading-core), and
  [`apps/worker`](../../apps/worker).
- [`packages/audit`](../../packages/audit),
  [`packages/observability`](../../packages/observability), and
  [`packages/ticket`](../../packages/ticket).

The ten frontend test/spec files consist of one Admin Vitest router test and
nine Playwright specifications. There is no User Frontend unit-test file.
Although both frontend manifests define `test`, `lint`, `typecheck`, and
`build`, [CI](../../.github/workflows/ci.yml) runs only frontend lint and build.
It does not run Vitest, Playwright, or the explicit typecheck scripts.

No coverage baseline is available for critical packages. No full-suite pass is
claimed. FND-001 changes no domain behavior, so its only added tests cover the
inventory, evidence-link, finding-row, and toolchain verifiers.

## Supported toolchain baseline

[` .tool-versions`](../../.tool-versions) defines the repeatable local baseline
(the leading display space is not part of the filename):

| Tool | Supported baseline | Other declarations | Local audit environment |
|---|---|---|---|
| Go | `1.24.7` | `go.work` requires `1.24.7`; modules use `go 1.24.0` and mostly `toolchain go1.24.7`; CI selects `1.24` | `1.25.4`; not the exact baseline |
| Node.js | `20.19.0` | root engine permits `^20.19.0`, `^22.13.0`, or `>=24`; CI selects `20` | `22.19.0`; supported, not the baseline |
| pnpm | `8.15.0` | root `packageManager` and engine are exact; CI selects major `8` | `8.15.0`; exact |
| Docker | not yet pinned | Compose parses with Docker Compose | CLI `29.4.3`, Compose `v5.1.3`; daemon unavailable |

CI's major-line declarations are compatible with this baseline but are not
exact patch pins. Pinning CI actions, scanners, Docker/Compose, and all release
build targets is explicitly unresolved production-engineering work.

## P0/P1 findings with repository evidence

Every row contains at least one repository link validated by the FND-001
verifier. Severity is launch severity, not implementation priority within this
task.

| ID | Severity | Finding | Evidence |
|---|---|---|---|
| P0-ARCH-01 | P0 | Market Data, Trading Engine, and trade-bff share the `trading-core` process and failure boundary. | [`apps/trading-core/main.go`](../../apps/trading-core/main.go) |
| P0-ARCH-02 | P0 | User, Admin, and Payment runtimes share the `api-server` process. | [`apps/api-server/main.go`](../../apps/api-server/main.go) |
| P0-ARCH-03 | P0 | Leaderboard, Settlement, Scheduler, and Free Generator share the `worker` process. | [`apps/worker/main.go`](../../apps/worker/main.go) |
| P0-ARCH-04 | P0 | Kubernetes base deploys merged wrappers while the production overlay patches obsolete standalone workload names. | [`base kustomization`](../../infra/k8s/base/kustomization.yaml), [`production overlay`](../../infra/k8s/overlays/production/kustomization.yaml) |
| P0-ARCH-05 | P0 | Merged-process health cannot prove each embedded bounded runtime is independently ready. | [`api-server manifest`](../../infra/k8s/base/api-server.yaml), [`trading-core manifest`](../../infra/k8s/base/trading-core.yaml), [`worker manifest`](../../infra/k8s/base/worker.yaml) |
| P0-SEC-01 | P0 | The merged API wrapper constructs one authentication service and injects it into User, Admin, and Payment runtimes. | [`apps/api-server/main.go`](../../apps/api-server/main.go) |
| P0-SEC-02 | P0 | Authentication middleware accepts a session JWT from the `token` query parameter. | [`packages/auth/middleware.go`](../../packages/auth/middleware.go) |
| P0-SEC-03 | P0 | Missing/disabled SMS configuration selects a mock provider that logs raw OTP codes; production fail-closed behavior is not enforced at selection. | [`user-bff app`](../../apps/user-bff/server/app.go), [`mock SMS provider`](../../packages/sms/mock.go) |
| P0-SEC-04 | P0 | The Admin surface lacks complete sensitive-action password reauthentication; legacy conditional TOTP is not accepted as the planned SEC-007 Super Admin MFA implementation or paid-production evidence. | [`admin auth handlers`](../../apps/admin-bff/server/handlers_helpers.go), [`admin routes/config`](../../apps/admin-bff/server/app.go) |
| P0-FIN-01 | P0 | `platform_fee_bps` and `commission_rate` remain active fee sources. | [`contest join handler`](../../apps/user-bff/server/contest_handlers.go), [`schema definition`](../../packages/db/migrations/0001_init.up.sql) |
| P0-FIN-02 | P0 | Contest join economics still read both fee fields and update Prize Pool in the legacy path. | [`apps/user-bff/server/contest_handlers.go`](../../apps/user-bff/server/contest_handlers.go) |
| P0-FIN-03 | P0 | The scoring distribution implements the legacy Power Law rather than canonical `tralent_v1`. | [`packages/scoring/distribution/distribution.go`](../../packages/scoring/distribution/distribution.go) |
| P0-FIN-04 | P0 | Prize calculation and finalization logic is duplicated across shared scoring, User preview, Leaderboard, and Settlement paths. | [`shared prize code`](../../packages/scoring/prize/distribution.go), [`user preview`](../../apps/user-bff/server/contest_prizes.go), [`leaderboard finalization`](../../apps/leaderboard-worker/server/finalize.go), [`settlement`](../../apps/settlement-service/server/settlement.go) |
| P0-FIN-05 | P0 | Leaderboard and Settlement both retain contest completion/payout authority. | [`leaderboard finalization`](../../apps/leaderboard-worker/server/finalize.go), [`settlement service`](../../apps/settlement-service/server/settlement.go) |
| P0-FIN-06 | P0 | Current state-machine/prize paths do not implement the required immutable economics lock at late-join cutoff. | [`admin state machine`](../../apps/admin-bff/server/handlers_statemachine.go), [`contest prize preview`](../../apps/user-bff/server/contest_prizes.go) |
| P0-CON-01 | P0 | Join authorization requires `registration_open`, so a running contest cannot accept an otherwise valid late entrant. | [`apps/user-bff/server/contest_handlers.go`](../../apps/user-bff/server/contest_handlers.go) |
| P0-CON-02 | P0 | Product-level `max_participants` remains in handlers, templates, schema, and responses. | [`contest handler`](../../apps/user-bff/server/contest_handlers.go), [`template handler`](../../apps/admin-bff/server/handlers_templates.go), [`template migration`](../../packages/db/migrations/0061_tournament_templates_schema.up.sql) |
| P0-CON-03 | P0 | Generic scheduler recurrence and a separate free generator do not implement one deterministic Tehran-time queue policy. | [`scheduler calendar`](../../apps/contest-scheduler/internal/scheduler/calendar.go), [`free generator`](../../apps/free-contest-generator/server/app.go) |
| P1-CON-04 | P1 | Cleanup code hard-deletes archived contest history, participants, symbols, and contests. | [`scheduler cleanup`](../../apps/contest-scheduler/internal/scheduler/cleanup.go) |
| P1-CON-05 | P1 | System-account markers exist, but exclusions are distributed across queries and are not proven for every economics, ranking, and settlement path. | [`system-account migration`](../../packages/db/migrations/0042_system_accounts.up.sql), [`leaderboard payout`](../../apps/leaderboard-worker/server/payout.go) |
| P1-ENG-01 | P1 | `WAL_PERSIST_PATH` defaults to empty, making the Engine WAL memory-only by default. | [`Engine config`](../../apps/trading-engine/server/config.go) |
| P1-ENG-02 | P1 | Engine startup logs a WAL replay warning and continues after replay failure. | [`Engine startup`](../../apps/trading-engine/server/app.go) |
| P1-ENG-03 | P1 | Current Trading Core deployment has no proven durable Engine WAL/snapshot volume. | [`trading-core manifest`](../../infra/k8s/base/trading-core.yaml), [`Compose`](../../infra/docker/docker-compose.yml) |
| P1-ENG-04 | P1 | Execution prices, PnL, and score still use binary `float64`. | [`price book`](../../apps/trading-engine/server/pricebook.go), [`position management`](../../apps/trading-engine/server/position_management.go) |
| P1-ENG-05 | P1 | WAL unit tests exist, but deterministic snapshot/replay, source-epoch, crash-barrier, performance, and soak qualification is not present in CI evidence. | [`WAL tests`](../../apps/trading-engine/server/wal_test.go), [`CI workflow`](../../.github/workflows/ci.yml) |
| P1-MD-01 | P1 | Tick contract v1 uses `float64` and has only symbol, bid, ask, last, timestamp, and volume. | [`tick snapshot v1`](../../packages/contracts/v1/tick_snapshot.go) |
| P1-MD-02 | P1 | Tick contract v1 lacks event ID, provider, sequence, receive/publish timestamps, quality, synthetic marker, and source epoch. | [`tick snapshot v1`](../../packages/contracts/v1/tick_snapshot.go), [`JSON schema`](../../packages/contracts/schemas/tick_snapshot.v1.json) |
| P1-MD-03 | P1 | Existing provider failover is primary/fallback logic, not the approved asset-group consensus and source-epoch switch model. | [`Market Ingestor provider manager`](../../apps/market-ingestor/server/app.go) |
| P1-MD-04 | P1 | Approved-symbol provider coverage and commercial display/redistribution rights have no completed launch-gate evidence. | [`fixed provider policy`](../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md), [`roadmap risk register`](../codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md) |
| P1-FE-01 | P1 | Trading remains inside User Frontend and imports User-root API, store, i18n, utility, and contract modules. | [`trade module`](../../apps/user-frontend/src/modules/trade), [`trade API`](../../apps/user-frontend/src/modules/trade/api/index.ts) |
| P1-FE-02 | P1 | Critical trading files exceed 30 KiB and combine transport, state, or rendering responsibilities. | [`MarketChart.vue`](../../apps/user-frontend/src/modules/trade/components/MarketChart.vue), [`TradingPage.vue`](../../apps/user-frontend/src/modules/trade/views/TradingPage.vue), [`useTradingWebSocket.ts`](../../apps/user-frontend/src/modules/trade/composables/useTradingWebSocket.ts) |
| P1-CI-01 | P1 | Frontend CI lints and builds but does not run existing Vitest, Playwright, or explicit typecheck commands. | [`CI workflow`](../../.github/workflows/ci.yml), [`root package scripts`](../../package.json) |
| P1-CI-02 | P1 | Seven roadmap-critical Go applications have no Go test file. | [`admin-bff`](../../apps/admin-bff), [`api-server`](../../apps/api-server), [`contest-scheduler`](../../apps/contest-scheduler), [`free generator`](../../apps/free-contest-generator), [`settlement`](../../apps/settlement-service), [`trading-core`](../../apps/trading-core), [`worker`](../../apps/worker) |
| P1-CI-03 | P1 | CI installs `golangci-lint` from mutable `HEAD`. | [`CI workflow`](../../.github/workflows/ci.yml) |
| P1-CI-04 | P1 | CI lacks fresh migration, real dependency integration, contract compatibility, image, SBOM, security, restore, rollback, and load gates. | [`CI workflow`](../../.github/workflows/ci.yml), [`Compose dependencies`](../../infra/docker/docker-compose.yml) |

## Known execution limitations

- The archive has no `.git` directory. Per local execution policy, Git history,
  branch, commit, remote, PR, and merge evidence is waived for this task.
- Local Go is `1.25.4`, not the supported `1.24.7` baseline. Go also attempted
  to write telemetry state outside the writable project sandbox.
- `node_modules` is absent. Dependencies were not installed or upgraded.
- Docker CLI and Compose are installed and Compose configuration parses, but
  the Docker Desktop daemon is not running.
- Host `psql`, `redis-cli`, `golangci-lint`, `gitleaks`, and `make` are absent.
- pnpm package-script execution, including root lint, typecheck, and build,
  stopped before the scripts ran because pnpm could not spawn `node.exe`
  (`EPERM`). Direct Node syntax checks and focused tests succeeded.
- Fresh migration, integration, E2E, image, restore, load, and full-suite checks
  were not executed and are not claimed as passing.

## Baseline conclusion

FND-001 establishes a reproducible inventory and evidence map. It does not
remediate the findings. Tragge remains **NO-GO for paid production** until the
roadmap tasks and launch gates are completed with executable evidence.
