# Phase 1 Exit Report

**Gate date:** 2026-08-09

**Execution mode:** Git-backed, evidence-only phase gate

**Repository:** `qopalboker/tragge_v0`

**Authenticated/authorized owner:** `qopalboker`

**Evaluated main SHA:** `54d9eaefcd0aa1f954c768ea94a4b048a47937ab`

**Gate branch:** `codex/phase-1-exit-gate`

**Decision:** `PHASE 1 FAIL`
**Paid-production status:** `NO-GO`

## 1. Decision and exact reason

Phase 1 fails its exit gate. The current security implementation passed the
focused, real PostgreSQL/Redis, race, frontend unit, Admin MFA browser, build,
and structural checks executed by this gate. The mandatory critical browser
E2E evidence did not pass, however:

- `pnpm exec playwright test apps/user-frontend/e2e/auth.spec.ts
  --project=user-chromium` exited `1` before collecting any tests;
- `apps/user-frontend/e2e/auth.spec.ts` imports `DashboardPage` from a barrel
  that does not export it; and
- the same suite imports `TEST_USERS` as an ESM named export from the current
  CommonJS `e2e/test-data` module.

The gate also found 70 relevant Go files reported by `gofmt -l`. The gate did
not edit those files because behavioral or source remediation is prohibited in
this invocation. Either missing mandatory item is enough to require the exact
decision `PHASE 1 FAIL`; there is no conditional pass.

## 2. Authorities and versions

The gate read and applied these authorities:

- [Canonical Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md), status
  `Canonical task-lifecycle and evidence protocol`;
- [Fixed Product and Technical Policies](../../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md),
  policy version `2026-08-09.1`;
- [Production Roadmap and Codex Tasks](../PRODUCTION_ROADMAP_AND_CODEX_TASKS.md),
  roadmap version `2026-08-09.1`;
- [Phase 1 controller](../prompts/02_PHASE_1_SECURITY.md);
- [ADR-0001](../../adr/0001-target-runtime-architecture.md), `Accepted`, dated
  2026-07-25;
- [canonical glossary and version catalog](../../product/canonical-domain-glossary-and-version-catalog.md),
  catalog version `2026-08-09.1`;
- [Phase 0 exit report](phase-0-exit-report.md), current decision `PASS` after
  PostgreSQL remediation and paid-production status `NO-GO`;
- FND-001 through FND-005 reports; and
- SEC-001 through SEC-007 reports.

Reports were not accepted as proof by themselves. Current source, tests,
configuration, local runtime behavior, Git history, pull requests, and
observable GitHub Actions evidence were checked.

## 3. Authoritative-main verification

| Check | Result |
|---|---|
| Remote | `https://github.com/qopalboker/tragge_v0.git` exactly |
| Account/repository access | Owner `qopalboker`; connector reports admin, maintain, push, and pull access |
| Working tree before gate | Clean |
| Local/main synchronization | local `main`, `origin/main`, and gate base were `54d9eaefcd0aa1f954c768ea94a4b048a47937ab` |
| Main history | Initial import through SEC-005, SEC-006 squash, SEC-007 squash; no unexpected main commit found |
| SEC reports on main | All seven required reports present |
| Phase 2 / ARCH-001 | Not started |
| Direct main write | None |

The evidence-only branch was created from the verified main SHA. No destructive
reset, force push, direct main push, or deployment occurred.

## 4. Task merge and report evidence

| Task | Current report decision | Delivery evidence on evaluated main | Current executable result |
|---|---|---|---|
| SEC-001 | [`SEC-001 PASS`](SEC-001-local-execution-report.md) | Local no-Git task evidence imported in `4facb23638c39fdffa482b339e20b8ff4a88d456`; no separate historical branch, PR, or CI exists | Auth isolation Go tests and structural tests passed |
| SEC-002 | [`SEC-002 PASS`](SEC-002-local-execution-report.md) | Imported in `4facb23638c39fdffa482b339e20b8ff4a88d456`; no separate historical branch, PR, or CI exists | Query-JWT and WebSocket-ticket regressions passed |
| SEC-003 | [`SEC-003 PASS`](SEC-003-local-execution-report.md) | Imported in `4facb23638c39fdffa482b339e20b8ff4a88d456`; no separate historical branch, PR, or CI exists | Focused tests plus real PostgreSQL/Redis OTP/reset lifecycle passed |
| SEC-004 | [`SEC-004 PASS`](SEC-004-local-execution-report.md) | Imported in `4facb23638c39fdffa482b339e20b8ff4a88d456`; no separate historical branch, PR, or CI exists | Focused tests plus real PostgreSQL/Redis reauthentication/privileged-action lifecycle passed |
| SEC-005 | [`SEC-005 PASS`](SEC-005-local-execution-report.md) | Imported in `4facb23638c39fdffa482b339e20b8ff4a88d456`; no separate historical branch, PR, or CI exists | Redaction, audit, panic/error, Sentry, and frontend log tests passed |
| SEC-006 | [`SEC-006 PASS`](SEC-006-git-execution-report.md) | Branch `codex/sec-006-add-edge-security-abuse-controls-and-secur`; [PR #1](https://github.com/qopalboker/tragge_v0/pull/1); reviewed head `a9738d4b223c6ef810e7f549cca09c30ef3b3d47`; CI run `31281330686` successful; squash `ca53ead8a90c06183f4147b0d2a78bb4c563a28c` | Edge, Redis policy, webhook, and retirement tests passed |
| SEC-007 | [`SEC-007 PASS`](SEC-007-git-execution-report.md) | Branch `codex/sec-007-super-admin-mfa`; [PR #2](https://github.com/qopalboker/tragge_v0/pull/2); reviewed head `7569cd52422bcf8b99604238f854a17e3aa79f4b`; CI run `31288092481` successful; squash `54d9eaefcd0aa1f954c768ea94a4b048a47937ab` | MFA unit, real runtime/concurrency, structural, frontend, and Admin browser tests passed |

PR #2 had no review or unresolved inline thread. GitHub Actions on the final
SEC-006 and SEC-007 heads reported successful change-detection, frontend, and
Go lint/test/build jobs. This report does not invent separate PR or CI evidence
for SEC-001 through SEC-005.

## 5. Policy-to-implementation mapping

| Phase property | Current evidence | Gate result |
|---|---|---|
| User/Admin keys, audiences, refresh/session namespaces, cookies, contexts, and cross-domain rejection | `packages/auth`, User/Admin/API integration tests, and SEC-001 structural validator | PASS |
| No session JWT query authentication; bounded WebSocket ticket path | Auth middleware, Trade BFF, frontend URL-builder tests, and SEC-002 validator | PASS |
| Production OTP/reset fail-closed routing, cooldown, attempts, expiry, no secret logging | SMS/notification/User BFF suites and real SEC-003 PostgreSQL/Redis lifecycle | PASS |
| Sensitive-action password reauthentication and privileged-action enforcement | Auth/Admin BFF suites and real SEC-004 PostgreSQL/Redis lifecycle | PASS |
| Central redaction across structured/text/error/panic/Sentry/audit paths | Observability/audit/frontend tests and SEC-005 validator | PASS |
| Request limits, CSRF, CORS, headers, proxy boundary, endpoint policies, Redis rate limits and login lockout | Validation/resilience suites, real Redis policy test, Nginx/Compose checks | PASS |
| Payment-provider webhook freshness/replay/idempotency | Payment-service tests and real Redis replay test | PASS |
| Retired Payment4 provider remains absent | Focused retirement validator: 14 rationale-allowlisted evidence files after this report, zero active references | PASS |
| NOWPayments and Jibit remain independent; no replacement provider | Provider/payment-service tests and active-source inspection | PASS |
| Super Admin MFA, encrypted storage, replay/recovery/reset/session upgrade, and SEC-004 coexistence | Auth/Admin unit tests, real PostgreSQL/Redis SEC-007 integration, race tests, and Admin browser E2E | PASS |
| Complete critical Phase 1 E2E | Admin MFA browser E2E passed; User auth/reset browser suite failed to load | **FAIL** |
| Relevant Go formatting | `gofmt -l` reported 70 files | **FAIL** |

## 6. Unit and structural validation

- All repository `scripts/*.test.mjs` tests passed outside the Windows
  process-spawn sandbox: 89 tests, 89 passed, 0 failed, 0 skipped.
- The focused suite covers FND-001 through FND-005, SEC-001 through SEC-007,
  the Go workspace parser, migration structure, Markdown links/paths/task IDs,
  terminology/policy, Payment4 retirement, and target architecture imports.
- Complete relevant Phase 1 Go package tests passed under explicit
  `ENVIRONMENT=test` and `APP_ENV=test` for auth, validation, SMS,
  notification, secrets, observability, resilience, audit, database, User BFF,
  Admin BFF, merged API, Trade BFF, and Payment Service.
- Frontend Vitest passed: Admin 10/10, User 12/12. The User run emitted three
  pre-existing Vue lifecycle warnings; no test failed.

An initial combined Node run failed with `spawn EPERM` because the Windows
sandbox prevented worker creation. It was rerun with the approved host process
permission and passed. An initial Go package run used an empty environment,
which production-safe database validation treats as production; the
`packages/db` valid-development fixture therefore failed. The exact suite was
rerun with the explicit test environment and passed.

## 7. Real PostgreSQL, Redis, migration, and concurrency evidence

The gate created only these disposable resources:

- `tragge-phase1-gate-postgres`, `postgres:16.9-alpine`,
  `127.0.0.1:55439`, database `tragge_phase1_gate_test`, user
  `tragge_phase1_gate`;
- `tragge-phase1-gate-redis`, `redis:7.4.5-alpine`,
  `127.0.0.1:56389`; and
- `tragge_phase1_gate_pgdata`.

The password was 32 cryptographically random bytes represented as hex, was
held only in process/container environment, was never printed, and was removed
with the container and volume. No external database, Redis, provider, staging,
production, or real-user resource was contacted.

### Fresh target foundation

- Applied `packages/db/init/target/01-cluster-roles.sql`.
- Applied target migration `0001_schema_ownership.up.sql`.
- Applied `02_reference_data.seed.sql`, which intentionally reports
  `fnd004_no_domain_seed_data`.
- Verified owners `platform:platform_owner`, `engine:engine_owner`, and
  `market_data:market_data_owner`.
- Verified runtime schema grants `1:0:0|0:1:0|0:0:1`.
- Verified zero target-domain tables at the FND-004 foundation stage and zero
  Payment4 database objects.
- Dropped and recreated only `tragge_phase1_gate_test`, reapplied the same
  foundation, and obtained matching sanitized schema hashes:
  `aebafc7e5f020cd9252393660158054620c472bf69b0f22687784c7bd47db6da`.

### Supported upgrade and runtime checks

- SEC-004 PostgreSQL migration test reported `0001 + 0024 + 0099`
  up/down/up success on PostgreSQL 16.
- SEC-007 real integration applied `0100_admin_super_mfa.up.sql` over its
  isolated current-legacy fixture and passed encrypted credential/recovery
  storage, concurrency, replay, reset, audit rollback, and session upgrade.
- SEC-003 real integration passed registration-country persistence, routed OTP,
  attempts/resend, reset, and old-session invalidation.
- Redis OTP lifecycle/binding, reauthentication single-use, endpoint policies,
  login lockout, webhook replay, MFA TOTP replay, and recovery-code concurrency
  all passed.
- Race-enabled auth, SMS, rate-limit, Admin runtime, and webhook suites passed.

The clean FND-004 target remains an ownership-only foundation. Later roadmap
tasks still own final Platform/Engine/Market Data domain tables; this gate does
not falsely claim those planned schemas or application startup against the
foundation are implemented.

## 8. Critical E2E journeys

| Journey | Executed evidence | Result |
|---|---|---|
| Registration and email/phone verification | Real SEC-003 User BFF/PostgreSQL/Redis HTTP-handler lifecycle | PASS |
| User login, refresh, and session isolation | Auth/User/API integration suites and frontend unit tests | PASS at unit/integration level |
| Password reset and old-session invalidation | Real SEC-003 lifecycle | PASS |
| Sensitive-action password reauthentication | Real SEC-004 lifecycle | PASS |
| Unauthorized destructive financial action denial | Real SEC-004 Support Admin/permission denial and audit tests | PASS |
| Super Admin MFA enrollment/login/invalid/replay/recovery/reset | Real SEC-007 lifecycle plus Playwright `sec007-admin-mfa` 4/4 | PASS |
| User authentication/password-reset browser journey | Existing `user-chromium` auth suite | **FAIL: suite did not load; zero tests collected** |

Because the phase controller requires the complete critical E2E set, passing
in-process integration evidence cannot replace the failed browser suite.

## 9. Coverage

| Package/module | Command | Statements | Critical branches exercised |
|---|---|---:|---|
| `packages/auth` | `go test ./packages/auth -coverprofile=<temporary> -count=1` | 62.9% | key/audience/context rejection, refresh/session, reauthentication, MFA secret/challenge/replay/recovery |
| `apps/admin-bff/server` | focused real SEC-004/SEC-007 tests with `-coverprofile` | 6.6% | sensitive-action grant lifecycle, audit rollback, MFA enrollment/login/replay/recovery/reset |
| `packages/observability` | `go test ./packages/observability -coverprofile=<temporary> -count=1` | 35.8% | structured/text/error/panic/Sentry redaction and safe correlation context |
| `packages/resilience/ratelimit` | `go test ./packages/resilience/ratelimit -coverprofile=<temporary> -count=1` | 51.3% | endpoint policy, fail-closed Redis behavior, lockout, concurrency |

Auth coverage is unchanged from the SEC-007 evidence (62.9%). No comparable
prior percentage was recorded for the other three packages. Numeric coverage
does not override invariant tests or the failed E2E gate. Temporary coverage
output was removed.

## 10. Frontend, build, and static results

- `pnpm install --frozen-lockfile`: PASS; lockfile already current.
- `pnpm -r lint`: exit 0 with 9 Admin and 224 User warnings, 0 errors.
- `pnpm -r typecheck`: PASS.
- `pnpm -r test`: PASS, 22 tests.
- `pnpm -r build`: PASS for both frontends; generated `dist` trees removed.
- Admin MFA Playwright: PASS, 4/4.
- User auth Playwright: FAIL before collection for the two module-contract
  errors recorded above.
- Relevant `go vet`: PASS.
- Relevant backend package/application builds: PASS after using actual package
  paths (`server` packages plus merged API).
- `gofmt -l`: FAIL, 70 relevant Go files reported.
- Local `golangci-lint`: unavailable. The pinned `v2.12.2` lint did execute and
  pass on observable final-head SEC-006 and SEC-007 CI; this report does not
  claim a local lint run.
- Docker Compose static rendering: PASS.
- Development and production Nginx 1.25-alpine syntax: PASS.
- Markdownlint: unavailable. Focused Markdown structure/link/path/task-ID tests
  passed before report creation and are not reported as Markdownlint.

## 11. Security, payment retirement, and scans

- Payment4 retirement validator passed before the report with 13 allowlisted
  evidence files and zero active references. The validator allowlist now adds
  this required exit report with a rationale; the final expected historical
  count is 14 and active count remains zero.
- NOWPayments and Jibit provider/payment tests passed. No replacement provider
  was added and no external payment provider was contacted.
- SEC-006 structural validation and real Redis edge/webhook tests passed.
- High-confidence tracked-file scan found zero GitHub-token candidates, zero
  AWS access-key candidates, and zero credential-bearing PostgreSQL URLs.
- One private-key marker exists only in
  `packages/observability/redaction_test.go` as a seeded redaction fixture.
- JWT-shaped text exists only in `packages/auth/session_test.go` as public,
  synthetic parsing fixtures. No complete runtime credential was captured.
- `gitleaks` was unavailable; no gitleaks success is claimed.
- Captured runtime output contained controlled safe error categories but no
  password, grant, JWT, OTP/reset value, MFA secret, recovery code, database
  password, or DSN.

## 12. P0/P1 review

The baseline contains 35 evidenced findings: 18 P0 and 17 P1.

- **Resolved by Phase 1 (5):** `P0-SEC-01` through `P0-SEC-04`, plus
  `P1-CI-03` (the linter is pinned and Go workspace parsing is structured).
- **Assigned to later approved roadmap phases (30):** five architecture, six
  financial, five Contest, five Trading Engine, four Market Data, two frontend,
  and the remaining three CI/operations findings. Their ownership is explicit
  in the roadmap; they continue to block paid production but are not silently
  reclassified as Phase 1 security work.
- **New Phase 1 gate blocker:** the current User authentication Playwright suite
  cannot load, so complete critical E2E evidence is missing.
- **Additional gate blocker:** 70 relevant Go files fail the repository-format
  check.

Phase 1 failure does not change the severities or owners in the baseline audit.

## 13. Command and exact-result ledger

Sensitive generated values are intentionally represented as `<generated>`;
the executed commands did not print them.

| Command | Exact result |
|---|---|
| `git remote get-url origin; git status --short; git rev-parse main; git rev-parse origin/main; git log --oneline --all` | Exit 0; exact origin, clean tree, synchronized main `54d9eae...`, expected history |
| GitHub connector repository, PR #1, PR #2, review/thread, workflow-run, and job reads | Owner/repository exact; PRs merged; PR #2 no review/thread; runs `31281330686` and `31288092481` successful |
| `git switch -c codex/phase-1-exit-gate` | Exit 0; branch created from verified main |
| Authority/report `Get-Content` and `rg` inspections | Exit 0 except one Windows wildcard form for `docs/adr/*.md`; direct ADR read then passed |
| `node --test <all scripts/*.test.mjs>` inside sandbox | Exit 1; 16 files failed to spawn with `EPERM`; no test result inferred |
| Same Node command with approved host process spawning | Exit 0; 89 passed, 0 failed, 0 skipped |
| Relevant Phase 1 `go test` with empty `ENVIRONMENT` | Exit 1; `packages/db` development fixture rejected `sslmode=disable` under fail-closed production interpretation; all other listed packages passed |
| Same relevant Go suite with `ENVIRONMENT=test APP_ENV=test` | Exit 0; all listed packages passed |
| Initial sandbox `docker info` | Exit 1; named-pipe access denied; no daemon result inferred |
| Approved-host `docker info --format ...` | Exit 0; Docker Desktop 29.4.3 |
| First generated-password container command | Exit 1 before Docker mutation; host .NET lacked static `RandomNumberGenerator.GetBytes` |
| Corrected 32-byte generated-password `docker volume create` and two exact-name `docker run` commands | Exit 0; PostgreSQL 16.9 and Redis 7.4.5 ready on loopback only |
| Target SQL copy/application, owner/grant queries, schema dumps, exact DB drop/recreate, reapplication, hash comparison | Exit 0; ownership/grants correct; hashes equal; zero target Payment4 objects |
| SEC-003 PostgreSQL/Redis lifecycle | Exit 0; all eight subtests passed |
| SEC-003 Redis OTP lifecycle/binding matrix | Exit 0; all subtests passed |
| SEC-004 Redis grant single-use | Exit 0 |
| SEC-004 and SEC-007 Admin PostgreSQL/Redis runtime plus upgrade migration | Exit 0; all subtests passed |
| SEC-006 Redis policy/login-lockout and webhook replay tests | Exit 0 |
| `go test -race` for auth, SMS, rate limit, Admin runtime, and webhook packages | Exit 0; five packages passed |
| `pnpm install --frozen-lockfile` | Exit 0; already up to date |
| `pnpm -r lint` | Exit 0; 0 errors, documented warnings |
| `pnpm -r typecheck` | Exit 0 |
| `pnpm -r test` | Exit 0; Admin 10, User 12 |
| `pnpm -r build` | Exit 0; Admin and User production builds completed |
| `pnpm exec playwright test --project=sec007-admin-mfa` with installed Chrome | Exit 0; 4 passed |
| `pnpm exec playwright test apps/user-frontend/e2e/auth.spec.ts --project=user-chromium` | Exit 1; module export errors; no tests found |
| `gofmt -l <Phase 1 relevant paths>` | Exit 1 in gate wrapper; 70 files listed |
| First `go vet; go build ./apps/...` wrapper | Exit 1 at build because four module roots have no root Go files; vet had passed |
| Corrected `go vet` and package-path `go build` | Exit 0; local golangci-lint reported unavailable |
| Four coverage `go test` commands | Exit 0; 62.9%, 0.3%, 35.8%, 51.3%; follow-on `go tool cover` extraction syntax failed and no pass was claimed for extraction |
| Real-runtime Admin focused coverage command | Exit 0; 6.6%; follow-on path extraction found no profile due PowerShell argument construction; package percentage retained from Go output |
| `docker compose ... config --no-interpolate --quiet` | Exit 0 |
| Two read-only Nginx 1.25-alpine `nginx -t` commands | Exit 0; both syntax checks successful |
| Payment retirement, SEC-006, and SEC-007 direct validators | Exit 0; retirement 13 pre-report allowlisted files/zero active, SEC validators passed |
| High-confidence tracked secret-pattern scan | Exit 0; only two intentional test-fixture files; gitleaks and Markdownlint unavailable |
| Exact generated-output/cache `Remove-Item` cleanup after path-within-root checks | Exit 0; gate Go cache, empty coverage dir, two `dist` trees, Playwright report, and test results removed |
| Exact container/volume removal and first verification wrapper | Tool exit 1 after successful absence output because final empty process query inherited a nonzero native status; named containers and volume removed |
| Final `docker ps`, `docker volume ls`, and `netstat` verification | Exit 0; no named object or listener output |
| `git diff --check` before report | Exit 0 |

## 14. Cleanup and scope review

Cleanup verification found:

- both named containers absent;
- the named PostgreSQL volume absent;
- no listeners on 55439 or 56389;
- no gate credential file (none was created);
- generated frontend `dist` directories absent;
- Playwright report/results absent;
- gate Go cache and temporary coverage directory absent; and
- `.tmp` empty.

No application, migration, runtime configuration, or product behavior was
changed by the gate. The only changes are this report and the minimal
retirement-validator allowlist entry required so this report is treated as
rationale-backed historical evidence rather than an active provider reference.

## 15. Known untested behavior and operational impact

- The User authentication Playwright suite is unexecuted because it fails at
  module loading; registration, login, refresh, reset, and old-session browser
  behavior therefore lack complete browser E2E evidence.
- No real Mailerino, Resend, NOWPayments, Jibit, or Payment4 service was
  contacted.
- No production ingress, external observability backend, KMS, backup/restore,
  or production deployment behavior was exercised.
- The FND-004 target foundation is not the final later-phase domain schema.
- Local Markdownlint, gitleaks, and golangci-lint binaries were unavailable.
  Focused validators and observable pinned CI evidence are reported instead,
  without claiming those unavailable local tools passed.

This gate is evidence-only. Its report has no runtime rollback impact; it can
be reverted as documentation, but reverting evidence does not fix either gate
blocker.

## 16. Remediation proposals

Use the [failed-gate remediation controller](../prompts/13_FAILED_GATE_REMEDIATION.md)
in a separate invocation. The smallest proposed remediation items are:

1. Repair only the User authentication Playwright module contracts, then run
   the complete required browser journeys for registration, OTP verification,
   login, refresh, password reset, and old-session invalidation. Add missing
   journey coverage where the current spec has no executable test.
2. Apply and verify canonical `gofmt` output to the 70 reported Phase 1-relevant
   Go files in a dedicated reviewed remediation change, then rerun tests, race,
   vet, builds, and the complete phase E2E gate.

The remediation must not weaken the gate, start ARCH-001/Phase 2, contact
production, or reinterpret passing unit/integration tests as the missing
browser evidence.

## 17. Final confirmations

### Gate-report delivery evidence

- Initial report commit:
  `2e60b8b61da6040d44b3707e9f90a3943f7c232d`.
- Pull request: [#3](https://github.com/qopalboker/tragge_v0/pull/3), targeting
  `main` from `codex/phase-1-exit-gate`.
- Initial report-head CI run: `31290211586`, completed `success`.
- Change detection passed. Frontend and Go jobs were correctly skipped because
  the commit changes only gate documentation and the retirement-evidence
  allowlist; their executed local/SEC task evidence is recorded above.
- The connector's PR mutation permission returned HTTP 403, so the already
  authorized Git Credential Manager session created the draft PR without
  printing its credential.
- The final delivery decision remains `PHASE 1 FAIL`; successful report CI does
  not convert the failed security gate into a pass.

- No behavioral remediation was implemented during this gate.
- No force push or branch-protection bypass occurred.
- No deployment occurred.
- ARCH-001 and Phase 2 were not started.
- Paid-production remains `NO-GO`.

# PHASE 1 FAIL
