# SEC-001 Local Execution Report

## 1. Task and decision

- **Task:** `SEC-001 — Restore cryptographic isolation between user and admin authentication`
- **Mode/date:** local extracted-project execution; 2026-07-26, Asia/Tehran.
- **Decision:** **SEC-001 PASS**.
- **Paid-production status:** **NO-GO**.
- **Stop boundary:** `SEC-002` and every later task were not started.

Active User and Admin runtimes now use separate signing keys, issuers,
audiences, validators, token contexts, refresh contexts, sessions, revocation
namespaces, cookies, and CSRF contexts. Representative cross-context access and
refresh attempts fail closed.

## 2. Dependency and gate verification

The fixed policies, complete SEC-001 roadmap block, canonical execution
protocol, Accepted ADR 0001, canonical glossary, Phase 0 exit report, and Phase
1 controller were read before editing. Repository evidence confirms:

- FND-002 artifacts exist and ADR 0001 is `Accepted`;
- the current Phase 0 decision is `PASS` after PostgreSQL remediation;
- SEC-001 depends on FND-002, whose focused regression passes; and
- the original runtime still built shared/generic auth, so SEC-001 was not
  already implemented.

Authoritative links: [fixed policies](../../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md),
[roadmap](../PRODUCTION_ROADMAP_AND_CODEX_TASKS.md),
[protocol](../CODEX_EXECUTION_PROTOCOL.md), [ADR 0001](../../adr/0001-target-runtime-architecture.md),
[glossary](../../product/canonical-domain-glossary-and-version-catalog.md), and
[Phase 0 report](phase-0-exit-report.md).

## 3. Pre-change shared-auth findings

1. `apps/api-server/main.go` constructed one `auth.Auth` from generic
   `JWT_SECRET`/`JWT_REFRESH_SECRET` and injected it into User, Admin, and
   Payment runtimes.
2. User and Admin standalone paths had different audiences/session prefixes,
   but shared generic refresh-key fallback remained.
3. JWT validation did not require issuer/auth-context identity, allowed a
   broader HMAC family, and did not require exactly one audience.
4. Blacklist and several User/Admin login-state keys were unqualified.
5. Cookie security depended on request transport even in production.
6. User/Admin CSRF shared configuration and had no explicit context identity.
7. Trade BFF and Payment validate User credentials; Settlement and Shard Router
   expose Admin-protected routes. All required explicit contexts.
8. Query-token fallback remains deliberately deferred to SEC-002.
9. Final caller scans find legacy generic secret helpers only in definitions and
   tests, not active User/Admin runtime construction.

## 4. User authentication context after the change

The User context requires its own access/refresh keys, issuer, exact audience,
`auth_context=user`, recognized purpose, `session:user:` and
`jwt_blacklist:user:` namespaces, `refresh_token_user` at `/api/user/auth`,
`tragge_session_hint_user`, `csrf:user`/`USER_FRONTEND_ORIGIN`, and
`auth:user:*` login/OAuth/2FA/reset state. API injects it into User BFF and
Payment; Trade BFF constructs it. Wrong-context injection is rejected.

## 5. Admin authentication context after the change

The Admin context requires its own access/refresh keys, issuer, exact audience,
`auth_context=admin`, recognized purpose, `session:admin:` and
`jwt_blacklist:admin:` namespaces, `refresh_token_admin` at `/api/admin/auth`,
`tragge_session_hint_admin`, `csrf:admin`/`ADMIN_FRONTEND_ORIGIN`, and
`auth:admin:*` state. API injects it only into Admin BFF. Settlement and Shard
Router validate the pair, then construct only the Admin context.

## 6. Cryptographic, refresh, cookie, and startup enforcement

Token parsing is restricted to HS256 and validates signature/key, issuer, one
exact audience, expiry, not-before, purpose, and auth context before role
authorization. Tampered roles, unknown purpose, multiple audiences, malformed
tokens, alternate algorithms, and cross-context credentials fail closed.

Refresh-token cryptographic validation now occurs before session rotation/reuse
handling. A new test exposed that the opposite order could delete the valid
other-context session; the corrected order rejects the foreign token without
mutation.

Cookies are host-only, context-named, and narrowly path-scoped. Production is
always `Secure`; refresh cookies are `HttpOnly`; production is
`SameSite=None`; explicit local/test HTTP is `SameSite=Lax`. Logout clears only
the matching context.

Production rejects missing/equal keys; any equality among all four access and
refresh keys; keys under 32 bytes; placeholder/default/local-only markers; low
variety or entropy; missing/equal issuer/audience; and cookie, session,
revocation, CSRF context, or origin collisions. Error text never includes the
secret. Local/test still requires explicit keys; only non-secret metadata may
default in explicitly non-production environments.

The existing CSRF design is Origin/Referer plus `X-Requested-With`, not a signed
token. SEC-001 uses exact distinct origins and middleware context identifiers.
A full signed CSRF program remains SEC-006.

## 7. Configuration variables and compatibility

Required/canonical variables:

- `JWT_SECRET_USER` / `JWT_SECRET_USER_FILE`
- `JWT_REFRESH_SECRET_USER` / `JWT_REFRESH_SECRET_USER_FILE`
- `JWT_SECRET_ADMIN` / `JWT_SECRET_ADMIN_FILE`
- `JWT_REFRESH_SECRET_ADMIN` / `JWT_REFRESH_SECRET_ADMIN_FILE`
- `JWT_ISSUER_USER`, `JWT_ISSUER_ADMIN`
- `JWT_AUDIENCE_USER`, `JWT_AUDIENCE_ADMIN`
- `USER_FRONTEND_ORIGIN`, `ADMIN_FRONTEND_ORIGIN`

The helper generates each key independently from 48 random bytes. No generated
value was persisted. Generic `JWT_SECRET`/`JWT_REFRESH_SECRET` are deprecated
for User/Admin trust but retained for non-panel legacy/internal paths. No
compatibility flag or dual acceptance was added.

## 8. Scope expansion rationale

- Trade BFF and Payment: direct User-token consumers.
- Settlement and Shard Router: direct Admin-protected routes.
- Worker Compose: embeds Settlement.
- `go.work`: Shard Router was omitted and could not otherwise compile locally.
- FND-001/FND-002 validators and architecture evidence: frozen counts detected
  the exact seven-Go-file/five-test and import deltas; the original Phase 0
  snapshot remains recorded and SEC-001 deltas are explicit.
- Root/env/security documentation and secret helper: required for reproducible
  isolated configuration.

No product, wallet, contest, prize, Settlement outcome, Trading Engine, Market
Data, payment-integration, frontend, migration, or deployment behavior changed.
No dependency/version was added.

## 9. Files changed

The final change set has 41 files, including this report:

- `.env.example`
- `README.md`
- `go.work`
- `infra/docker/docker-compose.yml`
- `infra/docker/secrets/README.md`
- `scripts/secrets/init-secrets.sh`
- `packages/auth/auth.go`
- `packages/auth/blacklist.go`
- `packages/auth/isolation.go` (new)
- `packages/auth/isolation_test.go` (new)
- `packages/auth/jwt.go`
- `packages/auth/middleware_test.go`
- `packages/validation/csrf.go`
- `packages/validation/csrf_isolation_test.go` (new)
- `apps/api-server/auth_contexts.go` (new)
- `apps/api-server/auth_contexts_test.go` (new)
- `apps/api-server/main.go`
- `apps/user-bff/server/app.go`
- `apps/user-bff/server/auth_handlers.go`
- `apps/user-bff/server/auth_isolation_test.go` (new)
- `apps/user-bff/server/forgot_password_handlers.go`
- `apps/user-bff/server/helpers.go`
- `apps/admin-bff/server/app.go`
- `apps/admin-bff/server/auth_isolation_test.go` (new)
- `apps/admin-bff/server/handlers_helpers.go`
- `apps/admin-bff/server/session_namespaces.go`
- `apps/trade-bff/server/app.go`
- `apps/payment-service/server/app.go`
- `apps/payment-service/server/config.go`
- `apps/shard-router/config.go`
- `apps/shard-router/main.go`
- `apps/settlement-service/server/app.go`
- `apps/settlement-service/server/config.go`
- `docs/security/user-admin-authentication-isolation.md` (new)
- `docs/architecture/current-state-audit.md`
- `docs/architecture/target-architecture-import-review.md`
- `scripts/sec001-auth-isolation.test.mjs` (new)
- `scripts/production-baseline.mjs`
- `scripts/production-baseline.test.mjs`
- `scripts/target-architecture.test.mjs`
- `docs/codex/reports/SEC-001-local-execution-report.md` (new)

`apps/shard-router/go.sum` was transiently touched by a failed standalone test
and restored byte-for-byte from `D:/tragge-codex/tragge-main.zip`; its final
SHA-256 matches the archive. It is not a changed file. A transient
`docker-compose.redis-cluster.yml` edit was removed and is not in the final set.

## 10. Tests added or updated

Coverage includes own-context access/refresh acceptance; cross-key, issuer,
audience, context, and refresh rejection; purpose/time/algorithm/malformed and
tampered-role rejection; production configuration failures; the legacy
shared-trust vulnerability fixture; miniredis session/revocation/logout
isolation; representative API endpoints; merged startup; cookies by environment;
and cross-context CSRF rejection. No test was removed, skipped, or weakened.

## 11. Every command and exact result

All commands ran at repository root unless stated. Secret values are omitted.
Read-only inventory used `Get-Content`, `rg`, `rg --files`, `Test-Path`,
`Get-ChildItem`, and `Select-String` over the authorities and auth scope; these
exited `0` unless a result below says otherwise.

### Authority and auth inventory

| Command | Result |
| --- | --- |
| `Get-Content docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md` | Exit `0`; completely read. |
| `Get-Content docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` plus `rg` for SEC-001/SEC-002 | Exit `0`; full block/stop boundary read. |
| `Get-Content docs/codex/CODEX_EXECUTION_PROTOCOL.md` | Exit `0`; local/report/testing rules read. |
| `Get-Content docs/adr/0001-target-runtime-architecture.md` | Exit `0`; `Accepted`. |
| `Get-Content docs/product/canonical-domain-glossary-and-version-catalog.md` | Exit `0`. |
| `Get-Content docs/codex/reports/phase-0-exit-report.md` | Exit `0`; current `PASS`. |
| `Get-Content docs/codex/prompts/02_PHASE_1_SECURITY.md` | Exit `0`; only SEC-001 selected. |
| `Test-Path` over FND-002 and Phase 0 artifacts | Exit `0`; all present. |
| `rg -n 'auth\.DefaultConfig\(|auth\.New\(|GetJWTSecret\(|GetJWTRefreshSecret\(' apps packages --glob '*.go'` | Exit `0`; generic construction inventoried; final app scan has none. |
| `rg -n 'RequireAuth|RequireAdmin|refresh|cookie|csrf|session|JWT_' <auth scope>` | Exit `0`; entry points, envs, middleware, cookies, sessions, and tests inventoried. |
| `rg -n 'GetJWTUserSecret|GetJWTAdminSecret|GetJWTRefreshSecret' apps packages --glob '*.go'` | Exit `0`; helper definitions/tests only, no app caller. |

`apply_patch` failed before every write because the Windows restricted-token
sandbox could not prepare its wrapper. The report-specific call returned exit
`1` with `Failed to write file ...SEC-001-local-execution-report.md`. Therefore
implementation used asserted PowerShell `ReadAllText`/`Contains`/`Replace`/
`WriteAllText` commands: each required its exact pre-change anchor, failed
non-zero when absent, and formatted changed Go files with `gofmt -w`.

Intermediate edit results are not hidden: one multi-file PowerShell command had
a parser error; the next stopped on a CRLF/LF anchor mismatch; corrected
LF-anchor edits exited `0`. Initial inventory-delta and audit replacements also
stopped on line-ending mismatches without writing; corrected per-file commands
exited `0`.

### Go test, race, vet, build, and formatting commands

| Command | Result |
| --- | --- |
| initial `go test ./packages/auth ./packages/validation ./apps/user-bff/server ./apps/admin-bff/server ./apps/api-server` with 1-second timeout | Exit `124`; timed out. |
| same before cache permission | Exit `1`; module/build cache access denied. |
| same after permission with 60-second timeout | Exit `124`; modules downloading. |
| same with 120-second timeout | Exit `0`; baseline focused packages passed. |
| first three post-edit compiles | Exit `1`; respectively found undefined `environment`, undefined User cookie `config`, and removed API `redis` binding; each was corrected. |
| fourth post-edit compile | Exit `0`. |
| `go test ./packages/auth -run 'Test(Isolation|UserAdmin|UserValidator|Modified|TimeAlgorithm|SessionRefresh|LegacyShared)' -count=1` (first) | Exit `1`; exposed cross-refresh session deletion. |
| same targeted command after ordering fix | Exit `0`. |
| `go test ./packages/auth ./packages/validation ./apps/user-bff/server ./apps/admin-bff/server ./apps/api-server -count=1` | Exit `0`. |
| `go test ./apps/trade-bff/server ./apps/payment-service/server ./apps/api-server -count=1` | Exit `0`; Payment compiled with no tests. |
| final suite using host AppData `GOCACHE` | Exit `1`; sandbox denied writes. |
| same with `GOCACHE=C:\tmp\tragge-sec001-go-build` | Exit `1`; sandbox denied directory creation. |
| same with workspace `GOCACHE=...\work\.go-cache-sec001` | Exit `0`. |
| first expanded Settlement/Worker suite | Exit `1`; found a removed import still used by non-auth Settlement config; restored. |
| repeated expanded suite | Exit `0`. |
| `go test ./apps/shard-router -count=1` before workspace update | Exit `1`; pre-existing `go.work` omission. |
| `GOWORK=off go test . -count=1` in Shard Router | Exit `1`; missing checksum/local-replace metadata. |
| `GOWORK=off go test -mod=mod . -count=1` in Shard Router | Exit `1`; attempted unauthorized `git ls-remote`, failed before fetch/auth, and grew `go.sum` 8184→9401 bytes. |
| restore Shard `go.sum` from local zip and SHA-256 compare | Exit `0`; current/archive both `DFE6A7D1748E5BBCD800BC34FA64DBC12DDE4C7D350CF9EE13D873AE2516ED33`. |
| `go test ./apps/shard-router -count=1` after adding it to `go.work` | Exit `0`; compiled, no tests. |
| `go test ./packages/auth ./packages/validation ./apps/user-bff/server ./apps/admin-bff/server ./apps/api-server ./apps/trade-bff/server ./apps/payment-service/server ./apps/settlement-service/server ./apps/worker ./apps/shard-router -count=1` | Exit `0`; all pass/compile. |
| `go test -race ./packages/auth ./apps/api-server -run 'Test(Isolation|UserAdmin|UserValidator|Modified|TimeAlgorithm|SessionRefresh|LegacyShared|NewAuthContexts|Representative)' -count=1` | Exit `0`; auth selected/passed; API regex selected none. |
| `go test -race ./apps/api-server ./apps/user-bff/server ./apps/admin-bff/server -count=1` | Exit `0`; all passed. |
| `gofmt -d` over 27 changed Go files | Exit `0`; no diff. |
| initial and final `go vet` over touched packages, final adding Settlement/Worker/Shard | Exit `0`. |
| initial `go build ./apps/api-server ./apps/user-bff ./apps/admin-bff ./apps/trade-bff ./apps/payment-service` | Exit `1`; no Git VCS metadata and four directories contain no Go files. |
| corrected `go build -buildvcs=false` against actual server package paths | Exit `0`. |
| final corrected build adding Settlement/Worker/Shard | Exit `0`. |
| `go test ./packages/db/migrations_test.go -v; go vet ./packages/db/migrations_test.go` | Exit `0`; 5/5, 98 pairs plus orphan, vet passed. |

Every Go invocation emitted the non-fatal host warning that the telemetry upload
token under AppData could not be created. Passing commands still returned `0`;
no telemetry check is claimed as passed.

### Node, Compose, Markdown, secret, and scope commands

| Command | Result |
| --- | --- |
| `node --check scripts/sec001-auth-isolation.test.mjs` | Exit `0`. |
| SEC structural script before Admin-service expansion | Exit `0`; 9/9. |
| final `node scripts/sec001-auth-isolation.test.mjs` | Exit `0`; 10/10. |
| `docker compose -f infra/docker/docker-compose.yml config --quiet` | Exit `0` before and after final mounts. |
| Compose with `docker-compose.redis-cluster.yml` overlay | Exit `1`; pre-existing `trade-bff` lacks image/build and overlay version is obsolete; no final SEC edit remains there. |
| initial `node scripts/production-baseline.test.mjs` | Exit `1`; 4/5, exact task delta detected `382 !== 375`. |
| initial `node scripts/target-architecture.test.mjs` | Exit `1`; 3/4, exact task delta detected `462 !== 451`. |
| updated baseline test and `node scripts/production-baseline.mjs verify` | Exit `0`; 5/5; reproducible current inventory, 35 findings, 146 links; CI patch-version warnings only. |
| updated architecture test | First exit `1` detected edge `169 !== 168`; evidence corrected to name `packages/validation -> packages/auth`; final exit `0`, 4/4. |
| `node scripts/domain-glossary.test.mjs` | Exit `0`; 8/8. |
| `node scripts/database-migration-reset.test.mjs` | Exit `0`; 10/10. |
| `node scripts/codex-execution-protocol.test.mjs` | Exit `0`; 11/11. |
| `node --test` with all six validator files | Exit `1`; sandbox returned `spawn EPERM` for parallel child processes. |
| same six files sequentially via `node <file>` | Exit `0`; 48/48: FND-001 5, FND-002 4, FND-003 8, FND-004 10, FND-005 11, SEC-001 10. |
| `node --check` over all changed Node scripts | Exit `0`. |
| focused Markdown links in FND/SEC validators | Exit `0`; changed-document links resolve. |
| `Get-Command markdownlint` and `markdownlint-cli2` | Exit `0`; both unavailable, no pass claimed. |
| `bash -n scripts/secrets/init-secrets.sh` | Exit `1`; WSL Bash `E_ACCESSDENIED`, no syntax pass claimed. |
| focused `rg --pcre2` secret-candidate scan over 40 pre-report files | Exit `0`; no private key, provider token, credential URL, or serialized JWT matched. |
| `Test-Path .git` plus structural no-Git check | Exit `0`; `False`. |
| `Get-Date; go version; node --version; docker --version; docker compose version` | Exit `0`; `2026-07-26T03:05:22+03:30`, host Go 1.25.4, Node 22.19.0, Docker 29.4.3, Compose 5.1.3. Repository pins Go 1.24.7/Node 20.19.0. |

## 12. Test-result summary

| Area | Result |
| --- | --- |
| JWT unit/vulnerability regression | PASS |
| Refresh/session/revocation with miniredis | PASS |
| Representative merged-API boundary | PASS |
| Production startup validation | PASS |
| Cookie separation/attributes | PASS |
| CSRF context isolation | PASS |
| All discovered consumer test/compile | PASS |
| Race, vet, build, formatting | PASS |
| Compose base rendering | PASS |
| SEC-001 structural/link/scope | PASS, 10/10 |
| FND-001 through FND-005 | PASS, 38/38 |
| FND-004 Go migration test/vet | PASS, 5/5 plus vet |
| Markdownlint | UNAVAILABLE |
| Bash syntax | UNAVAILABLE due WSL denial |
| Numeric coverage | Not generated; no correctness claim based on a number |
## 13. Policy and ADR mapping

- Authentication remains inside the Platform Modular Monolith in ADR 0001.
- No bounded system, cross-system SQL, product, financial, or role policy changed.
- Roles remain `USER`, `SUPPORT_ADMIN`, and `SUPER_ADMIN`; no Finance role.
- The single new package edge is `packages/validation -> packages/auth` for
  canonical CSRF context names; no app-to-app edge was added.
- No new dependency/version or ADR was introduced.

## 14. Acceptance-criteria checklist

- [x] Shared generic singleton removed from active User/Admin construction.
- [x] Separate cryptographic contexts, four keys, issuers, exact audiences,
  validators, purposes, and auth-context claims enforced.
- [x] Separate refresh contexts, cookies, sessions, revocation, and CSRF contexts.
- [x] Cross-context access/refresh fail closed without other-session mutation.
- [x] User token fails Admin endpoint and Admin token fails User endpoint.
- [x] Cross-key/issuer/audience/context tests pass.
- [x] Missing/equal/weak/repeated/placeholder production secrets fail.
- [x] Metadata, cookie, namespace, and CSRF collisions fail.
- [x] Valid isolated production test configuration succeeds.
- [x] Representative integration and cookie/session/CSRF tests pass.
- [x] Touched modules build, vet, format, and relevant race tests pass.
- [x] Secret scan finds no credential candidate.
- [x] No compatibility mode, new dependency, later task, or weakened test.
- [x] Paid-production status remains `NO-GO`.

## 15. Known untested behavior

- No live-browser E2E login; handler cookies, in-process middleware, token
  services, and miniredis sessions are tested.
- No full process ran against live PostgreSQL/Redis/Kafka or external providers.
- Docker images were not built; Compose rendering passed.
- WSL Bash syntax execution was unavailable; changed generation statements have
  structural checks using the existing OpenSSL workflow.
- Payment, Settlement, Worker, and Shard have no local auth test file; they
  compile and their construction is covered structurally/shared-package tests.
- Numeric coverage and remote CI were not produced.
- No production credential or production startup was used.

## 16. Remaining security risks

- SEC-002 still owns query/WebSocket token removal; behavior remains present.
- SEC-003 through SEC-006 still own OTP, Super Admin TOTP, financial-action
  reauthentication, edge controls, and full signed CSRF work.
- Legacy shared-domain sessions/tokens are invalidated by design; rollout must
  coordinate reauthentication and four-key configuration.
- Generic JWT helpers remain for non-panel/internal legacy paths; future caller
  scans must prevent reintroduction into User/Admin construction.
- The Redis-cluster Compose overlay has an unrelated pre-existing `trade-bff`
  definition error.
- Host Go/Node differ from `.tool-versions`; canonical CI must use pinned tools.

## 17. Rollback notes

Treat auth code, key configuration, cookies, and namespaces as one atomic set.
Restoring shared trust or dual acceptance recreates the vulnerability and is
not an acceptable rollback. On rollout failure, stop affected endpoints,
restore the last isolated build/configuration, explicitly clear only invalid
legacy sessions if approved, and rerun the focused suite before reopening.
Local import must use this manifest/evidence; no Git evidence exists.

## 18. Local/Git and process confirmation

- No `.git` metadata was initialized.
- No branch, commit, push, pull request, merge, or deployment occurred.
- No remote source-control operation completed and no repository data was
  fetched. One `go test -mod=mod` automatically attempted an unauthorized
  `git ls-remote` for missing local module metadata; it failed before
  authentication/fetch, and its checksum side effect was restored exactly.
  This is disclosed instead of claiming no attempt occurred.
- No real secret was printed or persisted.
- `SEC-002` was not started; query-token behavior remains.
- Phase 1 did not advance beyond SEC-001.
- Paid-production status remains **NO-GO**.

## 19. Final result

**SEC-001 PASS**

All required active User/Admin isolation boundaries and security tests pass.
Unavailable checks and unrelated/pre-existing failures are identified above and
are not claimed successful.
## 20. Post-report validation addendum

- Three bounded PowerShell `[IO.File]::WriteAllText`/`AppendAllText` commands
  created sections 1-10, 11-12, and 13-19 after the overlong one-command draft
  failed at process creation with Windows error 206; all three bounded writes
  exited `0`.
- `node --check scripts/sec001-auth-isolation.test.mjs; node
  scripts/sec001-auth-isolation.test.mjs` plus report whitespace inspection
  exited `0`: 10/10 tests and 392 report lines passed, including report links.
- The final secret-candidate scan over this report and the validator exited `0`;
  `.git` remains absent and the SEC-002 query-token fallback remains present.
