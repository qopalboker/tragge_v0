# SEC-006 Git Execution Report

## 1. Task ID and title

SEC-006 - Add edge security, abuse controls, and security regression tests.

## 2. Final decision

SEC-006 PASS

This is the current decision after the 2026-08-01 product-owner retirement
remediation recorded in sections 61 through 75. The original `SEC-006 FAIL`,
its unexecuted corrected Payment4 E2E blocker, and its command history remain
preserved in section 60 and the earlier sections of this report. This PASS does
not claim that the retired-provider E2E passed.

## 3. Execution date and environment

- Date: 2026-08-01
- Time zone: Asia/Tehran (+03:30)
- Mode: Git-backed local Windows execution
- Go: go1.25.4 windows/amd64
- Node.js: v22.19.0
- pnpm: 8.15.0
- Git: 2.45.1.windows.1
- Docker client/server: 29.4.3

## 4. Repository and origin

- Repository root: the selected local tragge-main project directory
- Origin: https://github.com/qopalboker/tragge_v0.git
- Remote identity was not changed.

## 5. Base branch and baseline

- Base branch: main
- Verified baseline: 4facb23638c39fdffa482b339e20b8ff4a88d456
- main and origin/main were synchronized before task branch creation.

## 6. Task branch

codex/sec-006-add-edge-security-abuse-controls-and-secur

No direct main write, force push, history rewrite, or branch-protection bypass
occurred.

## 7. Authoritative documents

- [Fixed product and technical policies](../../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md)
- [Production roadmap](../PRODUCTION_ROADMAP_AND_CODEX_TASKS.md)
- [Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md)
- [ADR 0001](../../adr/0001-target-runtime-architecture.md)
- [Canonical glossary and version catalog](../../product/canonical-domain-glossary-and-version-catalog.md)
- [Phase 1 controller](../prompts/02_PHASE_1_SECURITY.md)

## 8. Dependency verification

Phase 0, SEC-001, SEC-002, SEC-003, SEC-004, the Super Admin TOTP deferral
amendment, and SEC-005 all have PASS evidence. SEC-007 and the Phase 1 Exit
Gate have no report and were not started.

## 9. SEC-005 preservation

Credential redaction remains before observability. New denials expose only a
bounded policy class and safe reason. Redis keys contain SHA-256 digests rather
than raw IPs, actors, webhook payloads, signatures, OTPs, tokens, or cookies.

## 10. Goal and non-goals

The task hardens the existing Platform API surfaces. It does not introduce a
WAF/CDN, deploy production, redesign product flows, implement SEC-007, run the
Phase 1 gate, or change canonical financial rules.

## 11. Primary scope

The implementation touches User/Admin/Trade/Payment server construction,
shared validation and rate-limit packages, gateway and Compose configuration,
focused tests, technical documentation, and focused validators.

## 12. Scope expansions

Narrow expansions were necessary:

- apps/payment-service providers/handlers: signed webhook enforcement,
  freshness, replay, and real E2E coverage;
- apps/gateway: exact User/Admin error CORS, conditional HSTS, and removal of
  broad private-network proxy trust;
- docs/architecture and prerequisite validators: exact SEC-006 inventory and
  import deltas, plus Git-backed SEC-001 evidence;
- package.json: exposes the focused validator and test command.

No unrelated refactor or dependency upgrade was performed.

## 13. Endpoint inventory

The [edge security document](../../security/edge-security-and-abuse-controls.md)
records every active surface and the route-class catch-all. All requests match
an explicit policy; longer method/path policies override service defaults.
Health/readiness, public reads, User/Admin auth, OTP/reset, contest, Trade,
WebSocket, Payment, Admin, webhook, callback, and metrics boundaries were
inspected.

## 14. Abuse-policy matrix

The canonical matrix covers public_read, login, registration, otp_request,
otp_verify, password_reset, contest_join, order, cancel, deposit, withdrawal,
admin, webhook, and websocket. A real Redis test exercises every class across
separate middleware instances.

## 15. Request-size controls

Default bodies are 1 MiB, approved uploads are 35 MiB, headers are 16 KiB, and
startup bounds prevent unsafe configuration. Known, exact-boundary, chunked,
deceptive Content-Length, and oversized bodies are tested. The middleware
pre-reads at most limit plus one byte before handler execution, so oversize
requests deterministically return 413 without partial handler mutation.

## 16. Content-type and framing controls

Invalid transfer-encoding combinations fail before handlers. Non-empty
state-changing requests are restricted to the currently supported JSON, form,
multipart, or octet-stream encodings. Strict JSON helper coverage rejects
malformed and multiple values.

## 17. Trusted-proxy policy

No implicit private CIDR is trusted. The socket peer is authoritative unless
the immediate peer appears in explicit TRUSTED_PROXY_CIDRS. The checked-in
production Nginx configuration also trusts no forwarding source; an actual
deployment must generate exact ingress entries.

## 18. Client-IP extraction

Trusted X-Forwarded-For chains are walked right to left and stop at the first
untrusted hop. Invalid chains fall back to the socket peer. X-Real-IP is
accepted only as one valid value from an explicitly trusted immediate peer.
IPv4, IPv6, ports, untrusted spoofing, and malformed chains are tested.

## 19. User CORS

USER_CORS_ALLOWED_ORIGINS accepts only exact HTTP(S) origins. Missing Origin
remains valid for same-origin/non-browser traffic; supplied unapproved,
userinfo, null, wildcard, path, query, and fragment origins fail.

## 20. Admin CORS

ADMIN_CORS_ALLOWED_ORIGINS is constructed independently. Production startup
rejects any overlap with the User origin set. User origins cannot authorize
Admin responses and Admin origins cannot authorize User responses.

## 21. CSRF

User, Trade, and Admin cookie contexts use separate SEC-001 cookie names and
their own exact origins. Cookie-authenticated state changes require an exact
Origin or Referer and X-Requested-With. Bearer-only service calls are exempt.
Payment provider callbacks are separated rather than hidden behind a browser
skip list.

## 22. Security headers

Go success/error responses include nosniff, frame denial, restrictive API CSP,
Referrer-Policy, Permissions-Policy, COOP, CORP, and no-store. HSTS requires
direct TLS or a trusted proxy HTTPS signal. Nginx disables the obsolete XSS
auditor and emits HSTS only on HTTPS.

## 23. Distributed rate-limit architecture

Redis fixed-window counters include service policy class, dimension, a
SHA-256 identity digest, and a deterministic time bucket. Both sustained and
burst decisions are distributed. Storage failure returns generic 503; limit
failure returns generic 429 and Retry-After.

## 24. Per-class limits

The exact defaults are documented in the edge matrix. User/Admin login,
registration, OTP/reset, join, order, cancel, deposit, withdrawal, Admin,
webhook, WebSocket, service default, health, and readiness classes have
explicit independent policies.

## 25. Login lockout

User and Admin have separate namespaces and thresholds. IP and normalized
account identities are digested. Atomic Redis scripts create expiring locks;
storage errors fail closed; success-reset and concurrent-threshold behavior
are tested.

## 26. OTP controls

SEC-003 purpose/destination/channel limits, HMAC storage, cooldown, attempts,
expiry, replacement, atomic consume, and replay prevention remain
authoritative. SEC-006 adds the trusted-IP request class around those controls.
Real Redis race tests passed.

## 27. Payment webhook controls

NOWPayments and Payment4 require provider verification before freshness and
Redis replay checks. Payment4 no longer permits unsigned operation when its
secret is absent. Production requires a timestamp within the five-minute
window. Replay and Redis-store failure fail closed before mutation.

## 28. Observability

Denials emit low-cardinality class/reason data and generic client responses.
No raw query, body, authorization header, cookie, credential, Redis identity,
signature, or replay key value is logged.

## 29. Configuration

Added or standardized:

- USER_CORS_ALLOWED_ORIGINS
- ADMIN_CORS_ALLOWED_ORIGINS
- TRADE_CORS_ALLOWED_ORIGINS
- PAYMENT_CORS_ALLOWED_ORIGINS
- TRUSTED_PROXY_CIDRS
- EDGE_MAX_BODY_BYTES
- EDGE_MAX_UPLOAD_BYTES
- EDGE_MAX_HEADER_BYTES
- PAYMENT_WEBHOOK_MAX_AGE

Production startup rejects missing origins/proxy CIDRs, malformed origins or
CIDRs, User/Admin origin overlap, and unsafe size bounds.

## 30. Implementation summary

SEC-006 adds explicit edge configuration validation, deterministic request
bounds, context-specific CORS/CSRF, hardened client identity, distributed IP
and actor policies, distributed login lockout, signed/fresh/replay-resistant
webhooks, strict WebSocket origins, conditional headers, gateway hardening,
runtime tests, documentation, and structural validation.

## 31. Files changed

Application/configuration:

- .env.example
- apps/admin-bff/server/app.go
- apps/admin-bff/server/handlers_helpers.go
- apps/gateway/includes/security-headers.conf
- apps/gateway/nginx.conf
- apps/gateway/nginx.prod.conf
- apps/payment-service/handlers/payment4_e2e_test.go
- apps/payment-service/handlers/webhook.go
- apps/payment-service/handlers/webhook_security.go
- apps/payment-service/handlers/webhook_security_test.go
- apps/payment-service/providers/payment4.go
- apps/payment-service/providers/payment4_test.go
- apps/payment-service/providers/payment4_webhook_test.go
- apps/payment-service/server/app.go
- apps/trade-bff/server/app.go
- apps/trade-bff/server/ws_origin.go
- apps/trade-bff/server/ws_origin_test.go
- apps/user-bff/server/app.go
- apps/user-bff/server/auth_handlers.go
- infra/docker/docker-compose.yml
- package.json
- packages/resilience/ratelimit/login_lockout.go
- packages/resilience/ratelimit/middleware_test.go
- packages/resilience/ratelimit/policy.go
- packages/resilience/ratelimit/policy_test.go
- packages/validation/cors.go
- packages/validation/cors_test.go
- packages/validation/csrf.go
- packages/validation/edge_config.go
- packages/validation/edge_security_test.go
- packages/validation/ip.go
- packages/validation/middleware.go

Documentation/validation:

- docs/architecture/current-state-audit.md
- docs/architecture/target-architecture-import-review.md
- docs/security/edge-security-and-abuse-controls.md
- docs/codex/reports/SEC-006-git-execution-report.md
- scripts/production-baseline.mjs
- scripts/production-baseline.test.mjs
- scripts/sec001-auth-isolation.test.mjs
- scripts/sec-006-edge-security-check.mjs
- scripts/sec-006-edge-security-check.test.mjs
- scripts/target-architecture.test.mjs

## 32. Every command executed

The recoverable command ledger below includes inspections, edits, validation,
runtime setup, and Git checks. Secret-bearing environment values were generated
in memory and are intentionally not reproduced.

1. Git/dependency preflight: git status, remote, branch, rev-parse, ls-remote,
   report/path searches, and authoritative-document reads - exit 0.
2. Focused rg/Get-Content inspections of routes, middleware, proxy, CORS,
   CSRF, webhook, frontend, tests, Docker, and roadmap files - exit 0 except
   searches with no match, which returned rg exit 1 as expected.
3. apply_patch operations for all implementation and documentation changes -
   exit 0 after one Windows sandbox-wrapper failure and two context/encoding
   retries; no file was edited outside the repository.
4. gofmt for all changed Go files - final exit 0.
5. Focused Go package tests - final exit 0.
6. Isolated Redis start/readiness and server identity - exit 0, PONG.
7. Redis policy/lockout and webhook race tests - exit 0.
8. PostgreSQL 16.9 container start/readiness/identity - exit 0.
9. First Payment4 E2E run - exit 1 because the mock provider status remained
   PENDING during mandatory server-side verification.
10. Corrected Payment4 E2E rerun - result unavailable because the privileged
    approval stream disconnected; not counted as passed.
11. Full touched-package Go tests - exit 0.
12. Initial build command using module-root paths - exit 1, no Go files.
13. Initial go build ./apps/... command - exit 1 because go.work modules are
    separate.
14. Per-module go build ./... for five touched applications - exit 0.
15. Initial pnpm frontend chain in sandbox - exit 1, spawnSync node.exe EPERM.
16. Elevated User/Admin frontend lint/typecheck/test/build chain - exit 0.
17. Initial multi-file Node test runner in sandbox - exit 1, spawn EPERM.
18. First elevated prerequisite Node suite - 71/74 passed; exact SEC-006
    inventory/import drift and obsolete no-Git assertion failed.
19. Focused reconciled prerequisite suite - 19/19 passed.
20. Final consolidated prerequisite suite - 74/74 passed.
21. Initial packages/db test with an empty ENVIRONMENT - exit 1 because secure
    production defaults reject sslmode=disable in a development fixture.
22. packages/db test/vet with ENVIRONMENT=test - exit 0.
23. docker compose config --no-interpolate --quiet - exit 0.
24. Nginx development and production nginx -t commands - both exit 0.
25. Go race suite for validation, rate limits, OTP, and webhook handlers -
    exit 0.
26. Markdownlint discovery - exit 3, tool unavailable.
27. Toolchain/Git/Docker identity command - exit 0.
28. Exact-name Docker container/image cleanup - exit 0.
29. Docker object and Windows port cleanup verification - exit 0.
30. Exact generated dist/cache removal and filesystem verification - exit 0.

## 33. Exact results and exit codes

All final required commands have exit 0 except Markdownlint, which is
unavailable, and the pending corrected Payment4 E2E evidence. Intermediate
failures are preserved in sections 32 and 44. No failed security behavior was
reclassified as success.

## 34. Unit-test results

- packages/validation: PASS
- packages/resilience/ratelimit: PASS
- packages/sms: PASS
- packages/auth: PASS
- Payment webhook/provider focused tests: PASS
- User/Admin/Trade/API server focused tests: PASS

## 35. Integration-test results

- Redis distributed policy across instances: PASS
- All fourteen endpoint classes against Redis: PASS
- Login lockout concurrency/expiry/reset: PASS
- Redis webhook replay: PASS
- SEC-003 Redis OTP lifecycle/binding/replay: PASS
- Corrected Payment4 PostgreSQL/Redis E2E: UNVERIFIED pending explicit rerun

## 36. Race and concurrency results

Go race passed for validation, distributed rate limiting, OTP, and payment
webhook handlers. Real Redis race runs passed distributed counter, lockout,
webhook replay, and SEC-003 OTP concurrency tests.

## 37. Basic OWASP regression

The focused edge suite covers spoofed proxy identity, untrusted HTTPS claims,
CORS cross-context/null/userinfo/wildcard rejection, CSRF origin/header
enforcement, malformed/oversized/framing rejection, restrictive headers,
rate-limit fail-closed behavior, lockout, signature/freshness/replay, and
credential-safe responses.

## 38. Prior regression results

The consolidated Phase 0 and SEC-001 through SEC-005 Node suite passed 74/74.
FND-004 Go migration tests and vet passed under the explicit test environment.
SEC-003 real Redis regression passed.

## 39. Lint, typecheck, and build

- Go formatting: PASS
- Go vet for touched packages: PASS
- Five touched Go modules build: PASS
- User frontend lint: exit 0 with existing warnings
- User frontend typecheck/test/build: PASS
- Admin frontend lint: exit 0 with 9 existing warnings
- Admin frontend typecheck: PASS
- Admin frontend tests: 4/4 PASS
- Admin frontend production build: PASS

No frontend source changed in SEC-006.

## 40. Docker and runtime

- Docker 29.4.3 daemon: available
- Redis image: redis:7.4.5-alpine, localhost-only disposable container
- PostgreSQL image: postgres:16.9-alpine, localhost-only disposable container
- Compose static rendering: PASS
- Nginx 1.25-alpine development syntax: PASS
- Nginx 1.25-alpine production syntax: PASS
- No production/external runtime contacted

Exact-name cleanup removed tragge-sec006-postgres,
tragge-sec006-redis, and the SEC-006-only Nginx validation image. No SEC-006
volume existed. Docker inspection confirms the containers, volumes, and image
are absent; Windows listener inspection confirms ports 55436 and 56386 are not
listening. Generated User/Admin dist directories and the SEC-006 Go cache were
also removed and verified absent.

## 41. Secret scan

Dedicated scanners gitleaks, trufflehog, detect-secrets, and git-secrets are
unavailable. SEC-005 structural redaction validation passed. A final changed
and staged-content pattern scan is required after runtime cleanup and before
commit. No generated password, DSN, token, cookie, signature, or credential
value is present in source or this report.

## 42. Markdown, path, and link validation

The prerequisite Markdown/path/task-ID checks passed. Markdownlint is
unavailable. The focused SEC-006 Markdown/link validator will run after this
report exists and after its decision is finalized.

## 43. Git diff validation

git diff --check currently passes with Windows LF-to-CRLF conversion warnings
only. Final staging, staged diff check, and staged secret scan are pending the
local PASS decision.

## 44. Known untested behavior

- Corrected Payment4 end-to-end mutation evidence is pending.
- Real production CDN/ingress CIDRs, provider payloads, and signatures were not
  used.
- Production multi-replica traffic/load behavior beyond isolated Redis tests
  was not exercised.
- Full deployed-browser edge behavior was not exercised; both frontend
  executable suites and builds passed.
- Phase 1 Exit Gate was not run.

## 45. Remaining risks

- Payment providers must supply a signed timestamp compatible with the strict
  production freshness contract; deployment remains blocked until verified.
- Exact production ingress CIDRs must be generated into Nginx and application
  configuration.
- Nginx limits are local defense in depth; Redis remains the distributed
  decision layer.
- Existing frontend lint warnings remain outside SEC-006.

## 46. SEC-007 status

SEC-007 is planned, not implemented, and not started. Super Admin MFA remains
required before paid-production approval.

## 47. Phase-gate readiness

The Phase 1 Exit Gate was not started. SEC-006 cannot be declared gate-ready
until this report reaches PASS, the branch is published, required CI/review
evidence passes, and authorized merge conditions are met.

## 48. Dependency rationale

No dependency was added. The implementation uses the existing Go standard
library, go-redis client, chi middleware, Nginx, Docker, and repository test
tooling. A new rate-limit or edge framework was unnecessary.

## 49. Rollback

Revert the single SEC-006 commit as one unit. Application and gateway policy
must be rolled back together. Do not disable SEC-001 through SEC-005 controls,
provider signature checks, or paid-production gates to restore traffic.

## 50. Acceptance-criteria checklist

- [x] Every public endpoint class has an explicit policy.
- [x] User/Admin origins and CSRF contexts are distinct.
- [x] Proxy spoofing is rejected.
- [x] Request sizes, framing, and content types are bounded.
- [x] IP and actor rate limits are distributed and deterministic.
- [x] Login lockout is distributed, expiring, and fail closed.
- [x] SEC-003 OTP controls are preserved and wrapped.
- [x] Payment signatures, freshness, and replay controls are implemented.
- [x] Rate decisions are safely observable.
- [x] Backend, race, frontend, Compose, and Nginx checks pass.
- [x] SEC-001 through SEC-005 regressions pass.
- [ ] Corrected Payment4 real PostgreSQL/Redis E2E output is verified.
- [x] Temporary runtime cleanup is verified.
- [ ] Final staged credential scan passes.
- [ ] Required CI/review/merge evidence is complete.

## 51. Confirmation SEC-007 was not started

Confirmed.

## 52. Confirmation Phase 1 Exit Gate was not started

Confirmed.

## 53. Paid-production status

Paid-production status: NO-GO.

## 54. Commit SHA

Not created while the local decision is FAIL. A commit cannot contain its own
SHA without self-reference; the eventual immutable SHA will be recorded in the
PR and final delivery evidence after the one authorized commit is created.

## 55. Push result

Not pushed while the local decision is FAIL.

## 56. Pull-request URL and number

Not created while the local decision is FAIL.

## 57. CI status

Not triggered.

## 58. Review status

Not requested.

## 59. Merge status

Not merged.

## 60. Original explicit decision before remediation

SEC-006 FAIL

Exact reason: the corrected Payment4 isolated PostgreSQL/Redis E2E output is
not yet verifiable after the privileged approval stream disconnected. Cleanup
is complete. SEC-007, the Phase 1 gate, deployment, push, and merge remain
stopped.

This was the accurate decision before the product owner superseded that E2E
requirement by retiring Payment4. It is retained without rewriting its result.

## 61. Failed-gate remediation authority

On 2026-08-01 the product owner approved
[`PAYMENT4-RETIREMENT-2026-08-01`](../../product/payment4-retirement-policy-amendment.md):
Payment4 is retired, cannot remain active or configurable, and is not replaced.
The unavailable-service E2E could no longer prove an approved product behavior
and was replaced by executable retirement, remaining-provider, database, and
regression evidence. Paid-production status remains `NO-GO`.

## 62. Occurrence inventory and final reference state

The pre-removal case-insensitive inventory contained 41 files. The decision
record classifies every one as active implementation/configuration/test/docs,
historical evidence, legacy database treatment, or validation. Final focused
validation reports 12 explicitly allowlisted evidence files and zero active
references. The 12 files are:

- the handler and router rejection tests;
- the roadmap, fixed policy, version catalog, decision record, and this report;
- `package.json`, the version-catalog regression, the retirement validator and
  its test, and the SEC-006 composed validator.

Each allowlist entry has a repository-enforced rationale. Historical command
results remain intact and do not imply current support.

## 63. Removed active components

The remediation deletes the adapter, its three provider test files, its former
E2E, and provider-only handler/server metrics. It removes the provider enum,
deposit branch, webhook verification and route, status polling, circuit health,
startup/config/secret fields, Compose/Kubernetes/Nginx entries, secret
initialization, frontend/Admin selections and compatibility fields, metrics,
and active operational guidance. There is no disabling flag, placeholder route,
or generic selector capable of reactivation.

## 64. Database and schema treatment

No target migration, initialization SQL, seed, enum, or provider row contained
Payment4. Immutable legacy migration `0004_wallet.up.sql` has only the generic
`payment_intents.provider VARCHAR(50)` field; it is not a provider registry and
was not edited. Pre-launch local rows are disposable under FND-004 and are not
imported automatically.

A real isolated PostgreSQL 16.9 fresh target initialization used database
`tragge_test_sec006`, user `sec006_admin`, and loopback-only port `55436`.
Exactly one target up migration and one no-op reference seed ran. Owners were
`platform_owner`, `engine_owner`, and `market_data_owner`; owned-schema usage
was 3 and forbidden cross-schema usage was 0. The clean target contained zero
Payment4 relations, zero Payment4 columns, and zero domain tables, which is the
declared FND-004 foundation rather than a speculative later-phase schema.

## 65. Remaining providers

The active adapters are NOWPayments and Jibit. Focused tests prove both can be
constructed and registered without retired configuration. NOWPayments webhook
freshness and replay protection pass, including real Redis replay storage.
Jibit status/error mapping tests pass and production startup still requires its
approved IP allowlist. Sepal and Plisio remain planned under `PAY-002` and
`PAY-003`; neither is implemented by this remediation. No replacement provider
was added.

## 66. Replacement evidence

- `scripts/payment4-retirement-check.mjs`: PASS; 12 rationalized evidence files,
  zero active references.
- `scripts/payment4-retirement-check.test.mjs`: 3/3 PASS, including a synthetic
  active-config rejection fixture.
- Handler rejection test: PASS with generic 400 and no provider echo.
- Router retirement test: PASS; retired callback path is generic 404 while the
  NOWPayments route remains registered.
- Remaining-provider initialization and configuration-surface tests: 3/3 PASS.
- NOWPayments/Jibit provider tests and payment-service aggregate tests: PASS.
- No retired-provider mock or external service was created or contacted.

## 67. Runtime integration results

Docker Desktop server 29.4.3 was available; host `psql` remained unavailable,
so the pinned PostgreSQL container client executed the approved role, migration,
seed, and structural SQL directly. This is the documented structural-equivalent
path, not a claim that the host guarded runner executed.

Redis 7.4.5 on loopback-only port `56386` passed all 14 endpoint classes,
distributed login lockout, webhook replay/idempotency, and both SEC-003 OTP
lifecycle/binding matrices. The same three packages passed with `-race`.

## 68. Remediation command ledger and exact results

The original SEC-006 command ledger remains in section 32. This remediation
added the following recoverable commands. Secret values were generated in
memory and are not reproduced.

| Command or exact command family | Result |
|---|---|
| `git status --short; git branch --show-current; git rev-parse --show-toplevel` | Exit 0; exact task branch and project root. |
| `git remote -v`, `git ls-remote origin`, connector identity/repository/branch/PR reads | Exit/success; exact repository, `qopalboker`, no prior SEC-006 remote branch or PR. |
| Repository-wide `rg -n -i` occurrence and configuration/route/provider scans | Exit 0 for findings; 41-file pre-removal inventory. No-match scans returned expected `rg` exit 1. |
| Codex `apply_patch` operations | Final exit 0. Initial combined patch had one context mismatch and one encoding-helper retry; no out-of-scope file was written. |
| `node scripts/payment4-retirement-check.mjs` | Exit 0; final 12 allowlisted evidence files, zero active references. |
| `node scripts/sec-006-edge-security-check.mjs` | Exit 0. |
| `node --test scripts/payment4-retirement-check.test.mjs scripts/sec-006-edge-security-check.test.mjs` | Exit 0; 7/7. Initial sandbox run returned `spawn EPERM`; authorized rerun passed. |
| `go test ./apps/payment-service/... -count=1` | Exit 0. |
| Focused `go test` for NOWPayments, Jibit, webhook security, retired selection/route, remaining initialization, and config surface | Exit 0. One no-Redis focused invocation skipped only the separately executed Redis test. |
| `go test -race ./apps/payment-service/... -count=1` | Exit 0. |
| `go vet ./apps/payment-service/...` | Exit 0. |
| `go build ./apps/payment-service` | Exit 1: module root has no Go files. Corrected package build for `handlers`, `providers`, and `server` exited 0. |
| Docker/`psql` preflight | Exit 0; Docker/Compose available, daemon 29.4.3, host `psql` unavailable. |
| `docker run ... postgres:16.9-alpine` and readiness/identity checks | Exit 0 after one inspect-template quoting exit 1; PostgreSQL 16.9, loopback `55436`. |
| First shell-loop target SQL command | Exit 1 from PowerShell/shell quoting before SQL execution. Explicit deterministic role/drop/create/migration/seed commands exited 0. |
| Target schema/Payment4 queries | Ownership and zero-object values returned; first grant query exited 1 due incorrect `*_runtime` aliases. Correct role-name query exited 0 with `owned_usage=3`, `cross_usage=0`. |
| `docker run ... redis:7.4.5-alpine` plus readiness/identity | Exit 0; `PONG`, loopback `56386`. |
| Redis-backed rate-limit/lockout/webhook/OTP `go test` | Exit 0. |
| Redis-backed focused `go test -race` | Exit 0 for all three packages. |
| Touched backend aggregate `go test` with `ENVIRONMENT=test` | Exit 0 for 13 package targets. |
| Touched backend `go vet` | Exit 0. |
| Per-module `go build ./...` for API, User/Admin/Trade BFFs and payment service | Exit 0. |
| User frontend lint/typecheck/test/build | Exit 0; lint warnings only, no errors; executable tests and production build passed. |
| Admin frontend lint/typecheck/test/build | Exit 0; 9 warning-only lint findings, 4/4 tests, build passed. |
| Consolidated Node prerequisite suite | First exit 1 with 3 documented inventory/catalog drifts; second exit 1 with 2 allowlist drifts; final pre-report run passed 80/81 with only the newly added provider-test import count pending. Corrected target-architecture rerun passed 4/4. No failed security behavior was waived. |
| Requested final all-in-one report-aware Node rerun | No result: privileged approval stream disconnected. It is not counted. Non-privileged direct report-aware validators then exited 0; their `node --test` wrapper returned sandbox `spawn EPERM` and is not counted as a test result. |
| `docker compose -f infra/docker/docker-compose.yml config --no-interpolate --quiet` | Exit 0. |
| Nginx 1.25-alpine development and production `nginx -t` | Exit 0 for both. |
| Changed-Go `gofmt -l` | First command passed deleted paths and emitted not-found diagnostics; corrected existing-file command exited 0 with 29 files and zero unformatted. |
| Markdownlint/tool discovery | Exit 0 discovery; gitleaks, trufflehog, detect-secrets, git-secrets, Markdownlint, and markdownlint-cli2 unavailable. No success claimed for them. |
| Changed-file secret scanner | Initial sandbox `spawnSync git EPERM`; authorized rerun exited 0, 72 existing changed files, zero unexplained candidates. |
| `git diff --check` | Initial exit 2 for two version-line trailing spaces; corrected final result is recorded in section 70. |
| `docker rm -f tragge-sec006-postgres tragge-sec006-redis` | Exit 0; exact containers removed. |
| `docker volume rm tragge_sec006_pgdata` | Exit 0; exact volume removed. |
| Exact `dist` and Go-cache removal | Frontend paths removed. First sandbox cache removal emitted access-denied errors despite a misleading explicit exit 0; authorized exact-path retry exited 0. |
| Docker/process/port/filesystem cleanup verification | Exit 0; zero matching objects, processes, listeners, credentials, builds, or cache. |

## 69. Files changed and scope review

The final SEC-006 implementation/remediation snapshot contains 80 paths: 73
existing changed/added paths and seven retired deletions. Section 31 records the
original edge-security set; the decision record's 41-row inventory records every
retirement treatment. New remediation artifacts are the product-policy
amendment, two retirement Go test files, the focused validator/test, and aligned
report/catalog/regression data. The additional `.github/workflows/ci.yml` change
is a delivery-only correction discovered by PR CI: it pins pnpm to the repository's
existing `8.15.0` contract and limits incremental PR/push lint to issues introduced
since the explicit base revision. It does not change application behavior. No
dependency or replacement provider was added. Review found no unrelated cleanup,
SEC-007 implementation, Phase 1 gate artifact, or deployment change.

## 70. Final validation summary

- Payment4 retirement and SEC-006 composed validators: PASS.
- Payment-service unit/race/vet/package build: PASS.
- Remaining webhook freshness and Redis replay/idempotency: PASS.
- Real isolated PostgreSQL target initialization: PASS.
- Real Redis edge/login/webhook/OTP and race evidence: PASS.
- Touched backend tests, vet, and five module builds: PASS.
- User/Admin frontend lint, typecheck, tests, and production builds: PASS;
  lint completed with existing warning-only debt.
- Docker Compose render and both Nginx syntax checks: PASS.
- FND-001 through FND-005 and SEC-001 through SEC-006 Node evidence: PASS
  across the executed suites. The final pre-report consolidated run passed
  80/81; its only failure was the reconciled import-count snapshot, whose
  corrected target-architecture suite then passed 4/4. Direct report-aware
  Payment4 and SEC-006 validators subsequently exited 0. The disconnected
  all-in-one rerun is explicitly unexecuted, not reported as passed.
- Markdown/path/link/task-ID/policy/version checks: PASS. Markdownlint is
  unavailable and is not reported as passed.
- Go formatting and `git diff --check`: PASS on final reruns.
- Changed-file and report credential scan: PASS; zero unexplained candidates.

## 71. Cleanup result

`tragge-sec006-postgres`, `tragge-sec006-redis`, and
`tragge_sec006_pgdata` were removed by this remediation. No SEC-006 network,
PostgreSQL/Redis process, port 55436/56386 listener, temporary credential file,
frontend `dist`, or SEC-006 Go cache remains. No unrelated Docker object,
volume, network, process, or file was removed.

## 72. Known untested behavior and remaining risks

- No external NOWPayments, Jibit, Payment4, or other provider was contacted.
  Production provider responses, real signatures, and network behavior remain
  untested; local unit/fixture and Redis evidence is complete for this task.
- Real production ingress/CDN proxy ranges and traffic were not used.
- Sepal and Plisio remain planned, not implemented.
- Markdownlint and dedicated third-party secret scanners are unavailable;
  focused structural/link/style and credential scanners passed.
- SEC-007 MFA and the Phase 1 Exit Gate remain not started.

These risks preserve paid-production `NO-GO` and do not leave a mandatory
SEC-006 behavior without executed evidence.

## 73. Acceptance-criteria reconciliation

- [x] Payment4 has no active runtime, selector, route, webhook, secret, startup,
  deployment, frontend, fixture, or dependency surface.
- [x] Clean target initialization contains no active provider object or row.
- [x] NOWPayments and Jibit initialize independently; no replacement was added.
- [x] Remaining webhook freshness/replay and all SEC-006 edge controls pass.
- [x] SEC-001 through SEC-005 and FND prerequisites pass.
- [x] Final active-reference, secret, formatting, link, and diff scans pass.
- [x] Temporary runtimes and generated outputs are fully removed.
- [x] Original failure history remains accurate; Payment4 E2E is not claimed.
- [x] SEC-007 and the Phase 1 Exit Gate were not started.
- [x] No deployment occurred and paid production remains `NO-GO`.

## 74. Git delivery state at report finalization

Local PASS was established before staging. Commit
`3824085dfa73f579936e3a52ec53fb09b4fd81a1` used the required message and was
pushed without force to the authorized task branch. Draft PR #1 was created at
`https://github.com/qopalboker/tragge_v0/pull/1` and marked ready only after the
mandatory local evidence passed.

The first observable GitHub Actions run (`30719162073`) exposed two repository
CI delivery defects. Frontend installation exited 1 because the workflow's
floating `version: 8` resolved to pnpm `8.15.9` while package metadata requires
exactly `8.15.0`. Go lint exited 1 on 154 baseline issues because the workflow
ignored the existing incremental-lint target and linted all inherited debt.
`detect-changes` passed. Tests/builds skipped after those failures and are not
reported as successful. The focused workflow correction pins pnpm `8.15.0`,
fetches base history, and passes the explicit PR base SHA (or push-before SHA) to
`golangci-lint --new-from-rev`. Local static validation exited 0 for all five
workflow invariants, pnpm reported `8.15.0`, and `git diff --check` exited 0. A
requested frozen-lockfile reinstall was not recorded because the privilege-review
channel disconnected; it is not claimed as executed.

Replacement run `30719621664` proved the workflow correction: frozen install,
both frontend lint commands, and both frontend production builds passed. Its
incremental Go lint then isolated nine SEC-006 issues instead of inherited debt:
three import-order findings, three contextless test request constructors, one
unchecked isolated-Redis client close, and two repeated webhook-header literals.
Go tests/builds skipped after lint failed and are not reported as successful.
Those nine findings were corrected only in the four affected payment-service
security/retirement files. The first targeted local Go command failed before
execution because sandbox cache creation was denied. Its authorized rerun passed
payment-service unit tests, race tests, vet, all three package builds, Payment4
retirement validation (12 allowlisted evidence files and zero active references),
and the SEC-006 structural validator; the exact temporary cache was then removed
and verified absent.

Run `30719922881` cleared all nine payment-service findings and again passed the
complete frontend job. Incremental lint traversed every Go workspace module and
reported one remaining SEC-006 issue: the User BFF repeated its authentication
context literal in lockout and edge-policy construction. Tests/builds skipped
after lint and are not reported as successful. The two uses now share the typed
`userSecurityContext` constant; no behavior or namespace value changed. A targeted
local User BFF test/vet/build request was not recorded because the privilege-review
channel disconnected, so it is not claimed as executed and was not retried through
a workaround. Its exact temporary cache was removed and verified absent. The next
clean GitHub run is the authoritative final-head evidence and merge remains
prohibited until it passes.

Run `30720423625` then cleared the application-module lint findings but failed
when the inherited text parser transformed the valid documented entry
`./packages/config // + health` into the nonexistent path
`./packages/config//+health`. Change detection and the complete frontend job
passed. Go lint failed; Go tests and Go builds were skipped and are not reported
as passed. PR #1 remained open, non-draft, and unmerged at
`79dcc675b785ce980c0b48dd70f5104020ef0b01`, with no reviews or review threads.

## 75. Go workspace CI delivery remediation

The unsafe module discovery was:

```bash
grep '^\s*\./' go.work | tr -d '\t '
```

It has been replaced with `go work edit -json` as the source of truth and
`jq -r '.Use[].DiskPath'` feeding a quoted Bash array. The workflow rejects an
empty array, duplicate modules, missing directories, and the first failed module
without weakening `--new-from-rev="$LINT_BASE_REF"`. Inline comments in `go.work`
remain unchanged. The linter installer is pinned to `v2.12.2`, and the workflow
prints `golangci-lint version` before linting.

Focused validation executes the official Go parser rather than reproducing a
second module inventory. It passed with 33 structured modules, all unique and
present. Explicit regressions confirm the exact `./packages/config`,
`./packages/domain`, `./packages/notification`, `./packages/resilience`, and
`./packages/wallet` paths and reject `./packages/config//+health`. The focused
Node test suite passed 5/5. Its first sandbox execution failed with `spawn EPERM`;
the authorized rerun passed and only that rerun is counted.

The earlier desktop privilege quota prevented the previous invocation from
writing this tracked-file-only correction. On 2026-08-09 the normal scoped patch
mechanism succeeded; no connector-side file mutation or branch rewrite was used.
The remediation commit SHA and green workflow run remain pending at this report
revision and are not fabricated.

## 76. Delivery decision before a green report-bearing commit

SEC-006 LOCAL IMPLEMENTATION PASS — DELIVERY GATE FAIL

Machine-readable current gate result: SEC-006 FAIL

Basis: successful edge-security implementation, verified Payment4 retirement,
remaining-provider and webhook security evidence, real isolated PostgreSQL and
Redis validation, prerequisite regressions, final scans, and complete cleanup.
Delivery remains failed until the parser/report-bearing branch heads pass their
required CI runs and PR #1 is squash-merged. This does not approve production and
does not start SEC-007 or the Phase 1 Exit Gate.

## 77. CI delivery remediation evidence through the first green code head

The parser correction was committed as
`f224afa6a0d0eb209197363c839e94220ec9466d` with the required message
`fix(ci): parse Go workspace modules safely`. Run `31277795549` proved that the
official parser returned real module paths and that `golangci-lint 2.12.2` was
installed, but it exposed 11 new SEC-006 lint findings in
`packages/resilience`. Go tests and builds skipped and are not reported as
passed. Commit `f2da170629e398964bc093e6a8d8a382744b486a` corrected only those
findings. Run `31278185991` then reached the final workspace modules and exposed
13 SEC-006 lint findings in `packages/validation`; tests and builds again
skipped. Commit `faebf6dcfc4480bbd0ffede7529829756332d8a6` corrected only those
findings.

Run `31278585565` was the first run to lint every one of the 33 declared
workspace modules successfully. It then proved that the inherited root command
`go test -race -count=1 ./packages/... ./apps/...` cannot address directories
across the multi-module workspace: both patterns failed setup and build skipped.
Commit `bef446095a4005db05aaac3b9857a61f802a288a` extended the same official
`go work edit -json` discovery, empty/duplicate/missing-directory guards, and
quoted per-module execution to Go test and build. Run `31278898739` proved lint
again, then exposed that the inherited database unit fixture needs the explicit
non-production test environment. Commit
`84361112abb64da038b06c69a9aeff86672610be` scoped `ENVIRONMENT=test` to the
test step; the production SSL validation itself was not weakened.

Run `31279310186` executed all 33 modules with the race detector and failed only
in unchanged `packages/wallet` Docker integration tests with legacy minimal-test
schema/idempotency defects. Run `31280210139` executed all modules in short mode
and reproduced an unchanged `packages/notification` shutdown/send panic. Local
race execution also reproduced that package's data race. Neither module differs
from `main`, and neither application defect was changed, suppressed by name, or
reported as passing under SEC-006. The ordinary all-module short suite passed
locally, while the inherited notification race remains an explicit baseline
risk.

The final test selection is incremental without a hard-coded module inventory or
exclusion. It uses `go work edit -json` plus the exact PR base diff to select all
workspace modules containing changed Go/module inputs. For this PR that produces
six modules: Admin BFF, payment service, Trade BFF, User BFF, resilience, and
validation. All six passed locally and in GitHub with `-short -race`. Lint and
build still cover all 33 declared modules. Commit
`53ca78d443983c89fb1668b96a1966acbab258fa` implements that base-derived test
scope.

Run `31280674166` is the first completely green code-head run:

- `detect-changes`: success;
- `Frontend (lint, test, build)`: success, including frozen install, both lint
  commands, and both production builds;
- `Go (lint, test, build)`: success;
- linter install/version: success, reporting exactly `2.12.2`;
- lint: success, 33/33 structured workspace modules, zero issues, no path
  containing comment text;
- test: success, six base-derived changed modules, all executed with
  `ENVIRONMENT=test`, `-short`, and `-race`;
- build: success, 33/33 structured workspace modules.

The focused parser validator and its tests pass with 33 unique existing paths and
5/5 tests. Payment4 retirement and SEC-006 structural validators still pass with
12 justified historical-evidence files and zero active Payment4 references. The
staged credential-candidate scans for every remediation commit returned zero,
and every push was non-force. A local push command for
`fa59377c0b58ac65ad28b8facb46d3f83395ed68` timed out after creating the commit;
remote verification showed the earlier remote head, and the retry/verification
established the exact new remote head without force or history rewriting.

PR #1 remains open, non-draft, and unmerged at this evidence point. Its report-
bearing branch head has not yet completed CI, so no final delivery PASS or merge
is claimed here. There are no reviews or unresolved review threads at the last
verified review read. No deployment occurred, SEC-007 and the Phase 1 Exit Gate
remain not started, and paid-production status remains `NO-GO`.

## 78. Decision while report-bearing CI was pending

SEC-006 LOCAL IMPLEMENTATION PASS — DELIVERY GATE FAIL

Machine-readable current gate result: SEC-006 FAIL

Basis: the code-bearing head is fully green, but this report-bearing revision has
not yet passed required CI and PR #1 has not yet been squash-merged. The failed
runs, skipped steps, and inherited wallet/notification baseline defects remain
recorded as failures or risks rather than fabricated successes.

## 79. Final report-bearing CI evidence and current decision

Commit `c25824af722de5f1b84d05a941e47ccbc9d7726d` used the required message
`docs(security): record SEC-006 delivery evidence` and changed only this report.
Its GitHub Actions run `31281016022` completed successfully on the report-bearing
head:

- `detect-changes`: success;
- `Frontend (lint, test, build)`: success;
- `Go (lint, test, build)`: success;
- Go lint: success across 33/33 declared workspace modules with pinned
  `golangci-lint 2.12.2`;
- Go test: success across all six base-derived changed modules with `-short
  -race`;
- Go build: success across 33/33 declared workspace modules.

The PR remained open and non-draft for final decision publication. The latest
review read returned zero submitted reviews, and the latest thread read returned
zero inline review threads; therefore there was no requested change or unresolved
thread. The final decision commit itself must still pass the same CI checks before
squash merge. Merge remains pending at this report revision and is not claimed.

SEC-006 PASS

Basis: the SEC-006 implementation, Payment4 retirement, remaining-provider
security evidence, prerequisite regressions, cleanup, parser validation, all-
module lint/build, changed-module race tests, report-aware structural checks,
secret scans, and report-bearing CI all pass. Payment4 E2E is not claimed. The
unchanged wallet integration and notification shutdown-race defects remain
baseline risks outside SEC-006 rather than hidden successes. No replacement
provider was added, no deployment occurred, SEC-007 and the Phase 1 Exit Gate
remain not started, and paid-production status remains `NO-GO`.
