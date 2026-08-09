# SEC-007 Git Execution Report

## 1. Task ID and title

`SEC-007 — Implement Super Admin MFA before paid-production approval`

## 2. Current decision

`SEC-007 PASS`

This decision covers the implemented repository state and the mandatory local,
isolated-runtime, frontend, regression, and cleanup evidence in this report.
GitHub delivery evidence is recorded separately after the report-bearing branch
head is tested. This decision does not approve paid production.

## 3. Execution date and mode

- Date: 2026-08-09
- Time zone: Asia/Tehran (+03:30)
- Mode: Git-backed local Windows execution
- Repository: `qopalboker/tragge_v0`
- Origin: `https://github.com/qopalboker/tragge_v0.git`
- Base: `main` at `ca53ead8a90c06183f4147b0d2a78bb4c563a28c`
- Task branch: `codex/sec-007-super-admin-mfa`

No direct write to `main`, force push, history rewrite, branch-protection
bypass, production contact, or deployment occurred.

## 4. Authoritative sources

- [Fixed product and technical policies](../../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md)
- [Production roadmap](../PRODUCTION_ROADMAP_AND_CODEX_TASKS.md)
- [Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md)
- [ADR 0001](../../adr/0001-target-runtime-architecture.md)
- [Canonical glossary and version catalog](../../product/canonical-domain-glossary-and-version-catalog.md)
- [Phase 1 security controller](../prompts/02_PHASE_1_SECURITY.md)
- [Super Admin MFA design](../../security/super-admin-mfa.md)

## 5. Dependency and branch verification

- Phase 0 exit: PASS.
- SEC-001: PASS; User and Admin cryptographic trust domains remain isolated.
- SEC-002: PASS; reusable session JWT query authentication remains rejected.
- SEC-003: PASS; OTP/reset delivery remains fail closed.
- SEC-004: PASS; sensitive-action password reauthentication remains enforced.
- SEC-005: PASS; credential redaction remains in force.
- SEC-006: PASS and merged through PR #1.
- `origin/main`, local `HEAD`, and their merge base were all
  `ca53ead8a90c06183f4147b0d2a78bb4c563a28c` before implementation.
- The SEC-007 remote branch and pull request did not exist before delivery.
- The Phase 1 Exit Gate and Phase 2 were not started.

## 6. Pre-change legacy inventory

- `packages/auth/totp.go` supplied generic RFC 6238-compatible SHA-1 TOTP
  calculation with six digits and a plus/minus one-step window, but did not
  provide Super Admin enrollment, replay-counter persistence, recovery codes,
  or login assurance.
- Historical migration `0050` contains shared-user TOTP and backup-code
  columns. It remains immutable migration history and is not the active target
  credential model.
- A dormant legacy Admin two-factor handler existed but was not registered in
  the approved runtime. It used neither the new encrypted target storage nor
  replay-safe verification. It now fails closed with ordinary not-found
  behavior and cannot decrypt or accept legacy plaintext material.
- Super Admin password authentication previously created a full Admin session
  immediately. Support Admin and Super Admin shared the isolated Admin trust
  domain, but there was no MFA assurance distinction.
- Canonical roles were and remain `USER`, `SUPPORT_ADMIN`, and `SUPER_ADMIN`.
  No Finance role exists.

## 7. Final authentication boundary

Support Admin password authentication continues to create an isolated Admin
session. A valid Super Admin password creates only a short-lived, opaque,
Admin-context MFA challenge. A Super Admin session is created only after a
valid TOTP or unused recovery code is atomically verified. Central Admin
authorization requires the `super_admin_totp_v1` assurance for Super Admin
sessions; role claims alone cannot bypass this boundary.

Access and refresh tokens carry the explicit assurance value. The server also
checks the authoritative Redis session assurance, so a modified token or stale
session cannot manufacture MFA state. Refresh preserves and validates the
assurance rather than upgrading it.

## 8. Enrollment and credential storage

Enrollment requires a valid password-stage challenge and returns a single
Google-Authenticator-compatible provisioning URI plus ten one-time recovery
codes. The server stores:

- an AES-256-GCM encrypted TOTP secret with the versioned prefix
  `enc:admin-mfa:v1:`;
- the last accepted TOTP counter for replay rejection;
- peppered HMAC-SHA-256 recovery-code digests;
- enrollment and reset audit evidence.

Plaintext TOTP secrets and recovery codes are never persisted. Encryption
rejects malformed, tampered, or plaintext values. Recovery codes are disclosed
only during enrollment, are not recoverable afterward, and are consumed
atomically.

## 9. Configuration and startup validation

The Admin runtime requires independent MFA encryption-key and recovery-pepper
configuration. Validation rejects missing, malformed, placeholder, repeated,
or equal secret material. The encryption key must decode to 32 bytes. The
production issuer is `Tragge Admin`, and challenge lifetime cannot exceed five
minutes. Example and Compose configuration declare file-backed secrets without
containing a real credential.

## 10. Challenge, replay, and abuse controls

MFA challenges are opaque random values. Redis stores only their SHA-256 keys,
applies a bounded TTL, consumes them through an atomic Lua operation, and
retains a replay tombstone. Challenges are bound to the Admin identity,
password/session security fingerprint, and purpose.

TOTP acceptance atomically advances the last accepted counter in PostgreSQL;
concurrent submission of the same code permits exactly one success. Recovery
code consumption is similarly single use. Invalid MFA attempts increment the
existing Admin login-lockout controls. A correct password does not clear MFA
failure counters; only successful MFA completion clears them.

Password change, Admin session revocation, role change, permission change, or
credential reset changes the security fingerprint or revokes outstanding
state, causing stale challenges and sessions to fail closed.

## 11. Recovery and privileged reset

A Super Admin can sign in with one unused recovery code. Recovery use is
audited and cannot be replayed. Credential reset is a security-sensitive
operation requiring:

- an authenticated MFA-assured acting Super Admin;
- `users.edit` permission;
- a SEC-004 fresh password-reauthentication grant bound to
  `admin.mfa.reset` and the exact target user;
- an immutable mandatory audit record.

Support Admin is denied. Reset revokes target sessions, password
reauthentication grants, and MFA challenges. Transaction failure or mandatory
audit failure rolls the reset back.

## 12. Frontend behavior

The Admin login screen implements password stage, TOTP enrollment, recovery
code display, TOTP/recovery verification, safe generic failures, and English
and Persian RTL content. Challenge, provisioning secret, and recovery codes
exist only in the in-memory store. They are never placed in a URL,
`localStorage`, or `sessionStorage`.

The user-detail Admin flow can reset Super Admin MFA only after the existing
SEC-004 password-reauthentication prompt. A route-guard defect was corrected so
the canonical `SUPPORT_ADMIN` and `SUPER_ADMIN` roles satisfy the Admin route
guard; the legacy `admin` capability check does not introduce another role.

## 13. Database and migration

Migration `0100_admin_super_mfa` creates the clean target credential and
recovery-code tables with encrypted-secret constraints, unique ownership, and
replay-counter storage. The deterministic up-migration count is now 100.
Applying the down migration removed both SEC-007 tables; reapplying the up
migration recreated both. No already-applied migration was edited.

## 14. Audit and safe diagnostics

Enrollment, verification success/failure, recovery use, reset success/denial,
challenge invalidation, and mandatory-audit failures use bounded security
events. A session created immediately before mandatory audit failure is
deleted. Passwords, TOTP values, recovery codes, challenges, JWTs, cookies,
encryption keys, peppers, and complete session identifiers are excluded from
logs and audit payloads.

## 15. Files changed

### Configuration and runtime construction

- `.env.example`
- `infra/docker/docker-compose.yml`
- `infra/docker/secrets/README.md`
- `scripts/secrets/init-secrets.sh`

### Admin backend

- `apps/admin-bff/server/app.go`
- `apps/admin-bff/server/admin_mfa.go` (new)
- `apps/admin-bff/server/admin_mfa_integration_test.go` (new)
- `apps/admin-bff/server/handlers_admin_auth.go`
- `apps/admin-bff/server/handlers_helpers.go`
- `apps/admin-bff/server/reauthentication.go`

### Shared authentication

- `packages/auth/admin_mfa.go` (new)
- `packages/auth/admin_mfa_test.go` (new)
- `packages/auth/auth.go`
- `packages/auth/jwt.go`
- `packages/auth/middleware.go`
- `packages/auth/session.go`

### Database

- `packages/db/migrations/0100_admin_super_mfa.up.sql` (new)
- `packages/db/migrations/0100_admin_super_mfa.down.sql` (new)

### Admin frontend

- `apps/admin-frontend/src/api/reauthentication.ts`
- `apps/admin-frontend/src/api/users.ts`
- `apps/admin-frontend/src/i18n/locales/en.ts`
- `apps/admin-frontend/src/i18n/locales/fa.ts`
- `apps/admin-frontend/src/modules/admin/views/LoginPage.vue`
- `apps/admin-frontend/src/modules/admin/views/UserDetailPage.vue`
- `apps/admin-frontend/src/stores/auth.ts`
- `apps/admin-frontend/src/stores/admin_mfa.test.ts` (new)
- `apps/admin-frontend/e2e/admin_mfa.spec.ts` (new)
- `playwright.config.ts`

### Documentation and authoritative alignment

- `docs/architecture/migration-inventory.md`
- `docs/architecture/target-architecture-import-review.md`
- `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md`
- `docs/codex/prompts/02_PHASE_1_SECURITY.md`
- `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`
- `docs/product/canonical-domain-glossary-and-version-catalog.md`
- `docs/security/edge-security-and-abuse-controls.md`
- `docs/security/sensitive-action-password-reauthentication.md`
- `docs/security/super-admin-mfa.md` (new)
- `docs/codex/reports/SEC-007-git-execution-report.md` (new)

### Focused and prerequisite validation

- `package.json`
- `scripts/sec-007-super-admin-mfa-check.mjs` (new)
- `scripts/sec-007-super-admin-mfa-check.test.mjs` (new)
- `scripts/database-migration-reset.test.mjs`
- `scripts/domain-glossary.test.mjs`
- `scripts/production-baseline.mjs`
- `scripts/production-baseline.test.mjs`
- `scripts/sec-004-sensitive-action-check.mjs`
- `scripts/sec-006-edge-security-check.mjs`
- `scripts/payment4-retirement-check.mjs`
- `scripts/super-admin-totp-deferral-policy.test.mjs`
- `scripts/target-architecture.test.mjs`

### Directly justified scope expansion

- `apps/payment-service/handlers/webhook_security_test.go`: the SEC-006
  prerequisite race regression reused one fixed webhook identity across test
  invocations. The fixture now derives a unique synthetic replay body per test;
  production behavior is unchanged.

No dependency was added or upgraded.

## 16. Tests added or updated

- Table-driven MFA configuration, encryption, TOTP, recovery-code, challenge,
  refresh-assurance, and middleware tests.
- Real PostgreSQL/Redis integration covering enrollment, encryption at rest,
  TOTP and recovery concurrency, password/role/permission invalidation,
  lockout preservation, mandatory-audit rollback, Support Admin denial, and
  reset/session revocation.
- Admin frontend unit tests for in-memory state and canonical role guards.
- Four browser E2E journeys covering enrollment, normal TOTP login, recovery
  login, invalid/expired handling, RTL/LTR rendering, and authorized reset.
- Focused structural and policy validator plus all prerequisite validator
  snapshots affected by migration 0100 and SEC-007's implemented status.

## 17. Command and exact-result ledger

All outputs below are sanitized. Disposable secret values and DSNs were never
printed or retained. Failed attempts are included and are not counted as
passing evidence.

### Repository and dependency inspection

1. `git remote get-url origin; git branch --show-current; git rev-parse HEAD; git rev-parse origin/main; git merge-base HEAD origin/main; git status --short`
   — exit 0; exact origin and task branch verified; all three SHAs were
   `ca53ead8a90c06183f4147b0d2a78bb4c563a28c`; only SEC-007 files were changed.
2. Repository searches with `rg` over auth routes, roles, TOTP, MFA, recovery,
   session construction, migrations, frontend storage/URLs, policies, and task
   reports — exit 0 when matches existed and exit 1 for documented no-match
   searches; produced the inventory in sections 6 and 15.
3. `git ls-remote --heads origin main codex/sec-007-super-admin-mfa`
   — exit 0; `main` existed at the verified baseline and no SEC-007 branch was
   present before delivery.

### Focused Go tests and real runtime

4. `go test ./packages/auth -run 'Test(AdminMFA|MFA|RefreshPreservesMFA|RequireSuperAdminMFA)' -count=1 -v`
   — exit 0; focused shared-auth tests passed.
5. `go test ./apps/admin-bff -run TestSEC007 -count=1 -v`
   — exit 1; the workspace root contains no Go files. This command was
   corrected to target `./apps/admin-bff/server`.
6. `go test ./apps/admin-bff/server -run TestSEC007 -count=1 -v`
   — exit 0; focused Admin handler tests passed.
7. A localhost-only Docker command created
   `tragge-sec007-postgres` (`postgres:16-alpine`, `127.0.0.1:55437`) and
   `tragge-sec007-redis` (`redis:7.4-alpine`, `127.0.0.1:56387`) using a
   generated disposable credential, waited for `pg_isready` and Redis `PING`,
   and ran the integration suite — exit 0 after an initial empty-password
   startup attempt was rejected and its container was removed.
8. `docker exec tragge-sec007-postgres psql ... -c 'select version()'`
   — exit 0; PostgreSQL `16.14` observed.
9. `docker exec tragge-sec007-redis redis-server --version`
   — exit 0; Redis `7.4.10` observed.
10. `go test -race ./packages/auth/... ./apps/admin-bff/server -count=1`
    against the positively identified isolated runtimes — exit 0;
    `packages/auth` passed in 3.830s and Admin BFF passed in 4.995s.
11. Migration 0100 apply/query/down/query/up/query commands through the isolated
    PostgreSQL container — exit 0; credential and recovery tables existed,
    both disappeared after down, and both reappeared after up; the final table
    had 12 columns and the ciphertext constraint was present.
12. A query executed after the integration harness cleanup returned no test
    rows — exit 1 for that evidence expectation; this was correctly identified
    as harness cleanup, not reported as a persistence pass, and schema evidence
    was obtained independently.
13. `docker rm -f tragge-sec007-postgres tragge-sec007-redis` and
    `docker volume rm tragge_sec007_pgdata` — exit 0 for objects that existed.

### Backend regression, race, vet, build, and lint

14. `go test ./packages/auth ./packages/validation ./packages/sms ./packages/notification ./packages/secrets ./packages/observability ./packages/resilience ./apps/admin-bff/server ./apps/api-server ./apps/user-bff/server ./apps/trade-bff/server ./apps/payment-service/... -count=1`
    — exit 0; all targeted SEC-001 through SEC-007 backend regressions passed.
15. `go test -race ./packages/auth ./apps/admin-bff/server ./apps/payment-service/... -count=1`
    — exit 0; concurrency-sensitive auth, MFA, Admin, and payment regressions passed.
16. `go vet ./packages/auth ./apps/admin-bff/server ./apps/api-server ./apps/payment-service/...`
    — exit 0.
17. `go build ./packages/auth ./apps/admin-bff ./apps/api-server ./apps/payment-service`
    — exit 0.
18. `golangci-lint version`
    — exit 127 before the pinned tool was installed in the temporary tool directory.
19. `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`
    with isolated Go tool/module/build caches — exit 0.
20. Pinned `golangci-lint v2.12.2` initial runs over `packages/auth` and
    `apps/admin-bff` — non-zero; reported 11 and 33 new findings respectively.
    Findings were corrected without disabling rules.
21. Pinned `golangci-lint v2.12.2 run --timeout 5m --new-from-rev=origin/main ./...`
    in each touched Go module (`packages/auth`, `apps/admin-bff`,
    `apps/api-server`, `apps/payment-service`) — exit 0 for all four; zero new findings.
22. `gofmt -l` over every changed Go file — exit 0 with empty output.

### Coverage

23. `go test ./packages/auth -coverprofile=<temporary>` and
    `go tool cover -func=<temporary>` — exit 0; package total 62.9%; MFA
    functions ranged from 66.7% to 100% and TOTP verification reached 90.9%.
24. `go test ./apps/admin-bff/server -run TestSEC007 -coverprofile=<temporary>`
    and `go tool cover -func=<temporary>` — exit 0; whole large package 3.6%;
    directly touched MFA functions ranged from 51.5% to 100%.
25. Earlier coverage extraction attempts used an incorrect path/output parser
    and exited non-zero; they were not reported as evidence. The corrected
    commands above produced the recorded figures.

### Frontend

26. `pnpm --filter @tragge/admin-frontend lint`
    — an intermediate run failed on one new `no-useless-escape` finding; the
    expression was corrected. Final run exited 0 with nine documented
    pre-existing warnings and no errors.
27. `pnpm --filter @tragge/admin-frontend typecheck`
    — exit 0.
28. `pnpm --filter @tragge/admin-frontend test`
    — exit 0; 10 tests passed.
29. `pnpm --filter @tragge/admin-frontend build`
    — exit 0; production build completed.
30. `pnpm exec playwright test apps/admin-frontend/e2e/admin_mfa.spec.ts --project=admin-chromium`
    — first execution could not find bundled Chromium. `playwright install
    chromium` was attempted and failed with provider HTTP 403; neither is
    reported as passed.
31. Browser E2E with installed system Chrome initially ran 3/4; the fourth test
    exposed the canonical-role route-guard defect. A separate intermediate run
    also failed because video capture expected unavailable `ffmpeg`. The route
    guard was corrected and video was disabled for this focused local run.
32. `pnpm exec playwright test apps/admin-frontend/e2e/admin_mfa.spec.ts --project=admin-chromium`
    with the repository's system-Chrome fallback — exit 0; 4/4 tests passed.

### Structural, prerequisite, configuration, and documentation validation

33. `node scripts/sec-007-super-admin-mfa-check.mjs`
    — exit 0; SEC-007 structural/policy validation passed.
34. `node --test scripts/sec-007-super-admin-mfa-check.test.mjs`
    — exit 0; focused validator tests passed.
35. `node --test scripts/*.test.mjs`
    — one intermediate run failed because a prerequisite snapshot still
    expected the deferred-task wording. Snapshots were aligned to the approved
    implemented state. Final run exited 0; 89/89 tests passed.
36. `docker compose -f infra/docker/docker-compose.yml config --no-interpolate --quiet`
    — exit 0.
37. Focused Markdown path/link checks performed by the Node validators — exit 0;
    all local links and referenced paths resolved.
38. `markdownlint --version` and `markdownlint-cli2 --version`
    — unavailable; focused structure/style/link checks ran instead. Markdownlint
    is not claimed as passed.

### Secret, artifact, diff, and cleanup checks

39. Changed-file scanner for private keys, credential-bearing URLs, JWTs,
    database/Redis passwords, OTP values, recovery codes, and secret assignments
    — exit 0; zero candidates after fixture allowlisting.
40. `gitleaks version`, `trufflehog --version`, `detect-secrets --version`, and
    `git-secrets --version` — unavailable; none is claimed as passed.
41. `git diff --check` — exit 0.
42. Filesystem scans for `dist`, coverage, Playwright reports/results, built Go
    executables, temporary env files, and runtime logs — exit 0 with no retained
    generated artifact.
43. Docker exact-name inspection and TCP listener inspection for ports 55437 and
    56387 — exit 0; no SEC-007 container, volume, process, or listener remained.
44. Initial ordinary deletion of one temporary Go module-cache tree failed on
    Windows long/read-only paths. File attributes were cleared and the exact
    SEC-007 cache path was removed; the final existence check returned false.
45. A sandboxed parallel `node --test scripts/*.test.mjs` invocation exited 1
    because the sandbox denied all child-process spawns with `EPERM`. The suite
    was rerun with the already approved Node test execution permission.
46. The first report-aware elevated `node --test scripts/*.test.mjs` run exited
    1 with 87/89 passing: the Payment4 retirement validator correctly rejected
    this newly created report as an unclassified historical reference. The
    report was added to the explicit evidence allowlist with a rationale; no
    runtime/configuration occurrence was allowed.
47. `node scripts/sec-007-super-admin-mfa-check.mjs; node --test
    scripts/sec-007-super-admin-mfa-check.test.mjs; node --test
    scripts/*.test.mjs` after the allowlist correction — exit 0; focused
    validation passed 3/3 and the consolidated suite passed 89/89.
48. A high-confidence changed-file credential scan examined 51 files — exit 0;
    one credential-URL pattern was the existing synthetic
    `tragge_test_phase0` reset-guard fixture; zero real-secret candidates
    remained after that explicit classification.
49. The first generated-artifact name scan traversed dependency directories and
    reported 2,007 dependency-internal `dist` directories plus the empty root
    `.tmp`; a corrected scan excluding `.git` and `node_modules` found only the
    empty `.tmp` directory and no coverage, build, or Playwright output.
50. The empty root `.tmp` path was resolved beneath the project, verified to
    contain zero entries, removed with exact-name `Remove-Item`, and verified
    absent — exit 0.
51. `git add --all; git status --short; git diff --cached --stat; git diff
    --cached --check` staged 51 files and reported three intentional Markdown
    hard-break trailing spaces in `docs/security/super-admin-mfa.md` — exit 2.
    The metadata was changed to blank-line-separated paragraphs before the
    final staged check; no whitespace rule was weakened.
52. Final `git add --all; git diff --cached --check` plus the staged-content
    credential/artifact scanner — exit 0; 51 files staged, no whitespace error,
    no generated/private path, and only the explicitly synthetic
    `tragge_test_phase0` credential-URL test fixture matched.

## 18. Runtime results

- PostgreSQL: PASS against real disposable PostgreSQL 16.14.
- Redis: PASS against real disposable Redis 7.4.10.
- Migration up/down/up: PASS.
- TOTP same-counter concurrency: exactly one success.
- Recovery-code concurrency: exactly one success.
- Mandatory-audit failure: session/reset state rolled back or removed.
- Cleanup: PASS; containers, volume, credentials, caches, logs, and listeners absent.

No external identity, payment, email, SMS, production, staging, or real-user
system was contacted.

## 19. Frontend results

- Lint: PASS with nine pre-existing warnings and no errors.
- Typecheck: PASS.
- Unit tests: PASS, 10/10.
- Production build: PASS.
- Browser E2E: PASS, 4/4 using installed system Chrome after the bundled-browser
  download was unavailable.

## 20. Prerequisite regression results

SEC-001 authentication isolation, SEC-002 query-token prohibition, SEC-003 OTP
fail-closed behavior, SEC-004 password reauthentication, SEC-005 redaction,
SEC-006 edge/payment-provider security, and FND-001 through FND-005 focused
validators all passed. The consolidated Node suite passed 89/89.

Payment4 remains retired with zero active runtime/configuration references.
NOWPayments and Jibit remain the active providers. No provider was added.

## 21. Structural and credential scan

The focused SEC-007 validator confirms:

- no plaintext MFA credential field is part of the active target schema;
- no MFA challenge, secret, code, recovery code, password, JWT, or cookie is
  persisted in frontend browser storage or placed in a URL;
- no Finance role is accepted;
- legacy login 2FA routes are not registered;
- no task later than SEC-007 or Phase 1 gate artifact was introduced.

The changed-file credential scan found zero unsafe candidates. Generic secret
scanners were unavailable and are not claimed as passed.

## 22. Known untested behavior

- Real Google Authenticator interoperability was validated through the RFC
  contract and provisioning URI shape, not by contacting a third-party app.
- External production observability, backup/restore, key-management, and
  multi-region failure behavior were not executed.
- GitHub Actions and PR delivery are recorded only after observable completion;
  local PASS is not represented as CI evidence.
- The repository-wide frontend lint baseline still emits nine pre-existing
  warnings; SEC-007 introduced no lint error.
- Markdownlint was unavailable; focused Markdown validation passed.

## 23. Remaining risks

- Paid production remains blocked pending the explicit Phase 1 Exit Gate and
  all later launch, operational, legal, provider, recovery, and observability
  approvals.
- Production MFA encryption and recovery-pepper values must be generated,
  stored, rotated, and recovered through approved secret-management operations.
- Operator recovery is intentionally not a bypass: a lost credential requires
  the audited Super Admin reset path and operational governance.

## 24. Rollback notes

Before production launch, rollback is a forward repository revert plus the
documented clean disposable-database reset. In an applied environment, do not
silently edit migration 0100: use the explicit down migration only for a
positively identified disposable environment, or a reviewed compensating
forward migration. Rolling back MFA reopens the password-only Super Admin risk
and therefore cannot be a paid-production operating mode.

## 25. Acceptance-criteria checklist

- [x] Super Admin password login does not create a full session before MFA.
- [x] Support Admin login remains functional without gaining Super Admin authority.
- [x] TOTP is Google-Authenticator-compatible and uses a bounded time window.
- [x] TOTP secrets are encrypted with independent validated key material.
- [x] Recovery codes are one-time and stored only as peppered digests.
- [x] TOTP and recovery replay fail atomically under concurrency.
- [x] Challenges are opaque, short lived, purpose bound, and stored hashed.
- [x] Password, session, role, permission, and MFA reset invalidate stale state.
- [x] Super Admin access and refresh sessions carry verified MFA assurance.
- [x] Modified role/assurance claims cannot bypass signature and session checks.
- [x] Brute-force controls cover the MFA stage and password success does not clear MFA failures.
- [x] Reset requires an MFA-assured Super Admin, permission, exact SEC-004 grant, and audit.
- [x] Mandatory audit failure prevents or rolls back security state.
- [x] Frontend enrollment, login, recovery, reset, English, and Persian RTL flows pass.
- [x] Frontend stores no MFA secret or challenge persistently and places none in URLs.
- [x] Migration 0100 apply/down/up and target-schema validation pass.
- [x] Real isolated PostgreSQL and Redis integration and race tests pass.
- [x] Backend tests, race tests, vet, build, formatting, and pinned lint pass.
- [x] Frontend lint, typecheck, unit tests, production build, and E2E pass.
- [x] SEC-001 through SEC-006 and relevant FND regressions pass.
- [x] Focused secret/log scans find no credential.
- [x] No dependency was added.
- [x] No later task, Phase 1 Exit Gate, Phase 2 work, or deployment was started.
- [x] Paid-production status remains `NO-GO`.

## 26. Delivery evidence

### Implementation commit and push

- Commit: `9c70846d849fc043f038200f16be671da778f05e`
- Message: `feat(admin-auth): require super admin mfa`
- Files: 51
- Push: succeeded to `origin/codex/sec-007-super-admin-mfa` without force.

### Pull request

- PR: [#2](https://github.com/qopalboker/tragge_v0/pull/2)
- Base: `main`
- Head: `codex/sec-007-super-admin-mfa`
- Initial state: draft
- Initial head: `9c70846d849fc043f038200f16be671da778f05e`

The GitHub connector's create-PR operation returned `403 Resource not
accessible by integration`. The in-app browser fallback was not authenticated,
and no connected Chrome browser was available. The PR was therefore created
through the GitHub REST API using the already authorized Git Credential Manager
credential strictly in memory. The credential was not printed, written to
disk, passed in a command argument, or included in output.

### First GitHub Actions run

- Workflow: `CI`
- Run ID: `31287841381`
- Head: `9c70846d849fc043f038200f16be671da778f05e`
- Change detection: executed, PASS.
- Frontend install: executed, PASS.
- User frontend lint/build: executed, PASS.
- Admin frontend lint/build: executed, PASS.
- Pinned Go lint: executed, PASS.
- Go tests: executed, PASS.
- Go builds: executed, PASS.
- Overall conclusion: PASS.

This documentation update is committed separately so the final report-bearing
branch head can receive its own observable CI result. The PR remains draft and
unmerged until that final result and the review/thread checks pass.

### Delivery command results

1. `git commit -m "feat(admin-auth): require super admin mfa"` — exit 0;
   created `9c70846d849fc043f038200f16be671da778f05e`, 51 files, 3,195 insertions,
   and 215 deletions.
2. `git push --set-upstream origin codex/sec-007-super-admin-mfa` — exit 0;
   new remote branch created and upstream configured; no force option used.
3. GitHub connector create-PR request — failed with HTTP 403; no PR was created
   by that attempt.
4. GitHub compare-page fallback — read-only page loaded while signed out; no
   form submission or credential entry occurred.
5. Git Credential Manager-backed GitHub REST create-PR request — exit 0; created
   draft PR #2 at the exact implementation head without exposing the credential.
6. GitHub Actions run `31287841381` inspection — all three jobs completed with
   `success`; Go `Test` and `Build` were observed as executed, not skipped.

## 27. Final confirmations

- Current task decision: `SEC-007 PASS`.
- Phase 1 Exit Gate: not started.
- Phase 2: not started.
- Production deployment: not performed.
- Force push: not performed.
- Branch-protection bypass: not performed.
- Paid-production status: `NO-GO`.
