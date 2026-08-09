# Tragge — Production Roadmap and Independent Codex Tasks

**Roadmap version:** `2026-08-09.1`
**Current production decision:** **NO-GO**  
**Execution goal:** fastest safe launch without preserving broken architecture or
creating avoidable rework  
**Companion source of truth:**  
`docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`

---

## 1. Executive Assessment

The repository is recoverable and contains useful engineering work, but it is
not safe for a paid production launch in its current state.

Verified repository scale:

| Item | Count |
|---|---:|
| Go files | 375 |
| Vue files | 211 |
| TypeScript/TSX files | 178 |
| SQL files | 202 |
| Up migrations | 98 |
| Go test files | 99 |

The repository currently mixes three generations of architecture:

- Standalone domain services.
- Merged wrappers: `api-server`, `trading-core`, and `worker`.
- Kubernetes and Docker configurations that still reference different runtime
  topologies.

This is the central cause of operational ambiguity. The roadmap does not
attempt to polish every current service. It converges the repository to the
approved target architecture and deletes transitional runtimes after cutover.

### Test-execution limitation of this audit

The repository requires Go `1.24.7`. The review environment attempted to fetch
that toolchain but had no network access. It also lacked `pnpm` and Docker.
Therefore, this roadmap does **not** claim that the current test suite, images,
or Compose stack pass. Findings are based on static repository inspection and
must be converted into executable CI evidence during Phase 0 and Phase 1.

---

## 2. Confirmed Critical Findings

### P0 — Architecture and deployment

1. `apps/trading-core/main.go` runs Market Data, Trading Engine, and Trade BFF
   together, directly violating the required independent Engine/Market Data
   failure boundaries.
2. `apps/api-server/main.go` merges user, admin, and payment runtimes.
3. `apps/worker/main.go` merges leaderboard, settlement, scheduler, and free
   contest generation.
4. Production Kubernetes manifests and image overrides still reference older
   services while base manifests include merged runtimes. Kubernetes is not a
   credible initial launch path.
5. Health checks for merged processes cannot prove every embedded subsystem is
   alive or ready.

### P0 — Authentication and secrets

6. `apps/api-server/main.go` creates a shared auth service and passes it to user
   and admin paths, bypassing the stronger standalone user/admin key and
   audience separation described by the repository.
7. `packages/auth/middleware.go` accepts session JWTs from a URL query
   parameter.
8. OTP/reset fallback paths can log sensitive codes when providers are missing.
9. Super Admin does not yet have the sensitive-action reauthentication workflow
   or the MFA required before paid-production approval.

### P0 — Financial correctness

10. `platform_fee_bps` and `commission_rate` are both active sources of truth.
11. `apps/user-bff/server/contest_handlers.go` updates Prize Pool contribution
    from `commission_rate`, while preview helpers prefer `platform_fee_bps`.
12. Current prize code in `packages/scoring/distribution` uses a Power Law and
    does not implement the approved `tralent_v1` bucket model.
13. Current preview, state-machine prize lock, leaderboard finalization, and
    settlement code contain multiple formula/finalization paths.
14. Settlement and leaderboard both retain finalization/completion
    responsibilities. One idempotent implementation cannot compensate for two
    owners.
15. Current prize lock occurs at contest start, but the approved product allows
    late entry and requires economics to lock at the late-entry cutoff.

### P0/P1 — Contest lifecycle and scheduler

16. `handleJoinContest` accepts only `registration_open`, so a RUNNING contest
    cannot currently accept a valid late entrant.
17. Product participant capacity is still present in handlers, schema, and UI,
    although the approved model has no contest capacity.
18. The current scheduler is generic recurrence logic and the free generator
    is a separate service; neither represents the complete Tehran-time
    deterministic queue policy.
19. Free-contest generation aligns to older time rules and contains retention
    deletion behavior unsuitable for financial/audit history.
20. The existing practice account is useful, but system participants are not
    consistently excluded from every economics, ranking, and settlement query.

### P1 — Trading Engine

21. `WAL_PERSIST_PATH` defaults to empty.
22. Startup logs a warning and continues when replay fails.
23. Current deployment does not provide a proven durable WAL/snapshot volume.
24. Price and several ranking/settlement paths still use binary floating point.
25. Deterministic snapshot/replay, source epochs, and end-of-contest barrier
    tests are not production-qualified.

### P1 — Market Data

26. `packages/contracts/v1/tick_snapshot.go` contains only symbol, bid, ask,
    last, timestamp, and volume using float64.
27. It lacks event ID, provider, sequence, receive/publish timestamps, quality,
    synthetic marker, and source epoch.
28. Existing failover is useful groundwork, but it is not the approved
    asset-group health/consensus/source-switch model.
29. Provider coverage and commercial display/redistribution rights for the
    approved symbol registry are not yet verified.

### P1 — Frontend and release engineering

30. The trading implementation remains inside `user-frontend` and imports root
    user-panel dependencies.
31. Critical trading files exceed roughly 30 KB and combine transport, state,
    and view responsibilities.
32. Frontend CI currently builds and lints but does not run the existing Vitest
    and Playwright scripts.
33. Critical apps with no Go test files include `admin-bff`, `api-server`,
    `contest-scheduler`, `free-contest-generator`, `settlement-service`,
    `trading-core`, and `worker`.
34. CI installs `golangci-lint` from `HEAD`, which is not reproducible.
35. CI lacks fresh migration, real dependency integration, contract
    compatibility, image, SBOM, security, restore, rollback, and load gates.

---

## 3. Target Repository Shape

```text
apps/
  platform/
    cmd/platform/
    internal/modules/
      identity/
      admin/
      contest/
      scheduler/
      wallet/
      payment/
      kyc/
      settlement/
      leaderboard/
      notification/
      ticket/
    internal/runtime/
      api/
      realtime/
      worker/

  trading-engine/
  market-data/
  user-frontend/
  trade-frontend/
  admin-frontend/
  gateway/

packages/
  contracts/
  money/
  frontend-core/
  trading-contracts/
  design-system/
  observability/
  resilience/
  storage/

docs/
  adr/
  architecture/
  product/
  codex/epics/
  codex/tasks/
  runbooks/
  release/
```

The eventual name `apps/market-data` is preferred over
`apps/market-ingestor`, but renaming is performed only after contract and
deployment cutover to avoid a noisy early change.

---

## 4. Delivery Strategy and Critical Path

### 4.1 Fastest safe execution model

The work should be executed in parallel after the baseline and financial model
are fixed:

```text
Foundation and P0 security
        |
        +--> Platform modular monolith --------+
        |                                      |
        +--> Canonical money/ledger ------------+--> Contest lifecycle/scheduler
        |                                      |             |
        +--> Trading Engine hardening ----------+             +--> Prize/settlement
        |                                      |             |
        +--> Market Data redesign --------------+-------------+
        |
        +--> Trade frontend extraction -----------------------+
        |
        +--> Payments/KYC ------------------------------------+
                                                               |
                                               Production engineering
                                                               |
                                                 Qualification and launch
```

### 4.2 Credible calendar range

Codex reduces mechanical migration time, but it does not replace product,
financial, security, provider, and resilience verification.

| Team | Credible paid-public range |
|---|---:|
| 5–6 experienced engineers, including QA/DevOps | 14–18 weeks |
| 3 experienced engineers | 22–30 weeks |
| 1 engineer plus Codex | 36+ weeks |

The earliest credible path uses:

- One technical lead/architect.
- Two backend engineers, one focused on Platform/finance and one on Engine.
- One frontend engineer.
- One QA/DevOps engineer.
- Optional second backend/Market Data engineer.

A date must not override a failed quality gate.

### 4.3 Launch order

1. Internal simulation.
2. Capped external free practice.
3. Invite-only low-value paid contests.
4. Capped public paid service.
5. Gradual template and limit expansion.

---

## 5. Codex Execution Contract

Every task below is intentionally self-contained and copy/paste ready.

Codex must follow these rules for every task:

1. Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
2. Read only the listed scope and direct dependencies first; avoid a full
   repository reread unless necessary.
3. Create a dedicated branch:
   `codex/<task-id-lowercase>-<short-name>`.
4. Do not mix unrelated cleanup or future tasks.
5. Preserve compatibility only when the task says so.
6. Use migrations and versioned contracts for behavior/data changes.
7. Add or update targeted tests.
8. Run the task's verification commands.
9. Update relevant Markdown documentation.
10. Write a concise implementation summary and unresolved risks.
11. Create one Conventional Commit.
12. Push and open a PR. Merge only after required checks pass and no unresolved
    review remains.
13. If a task cannot be completed safely, commit no partial behavior and report
    the exact blocker and evidence.
14. Minimize new dependencies and explain any addition in the PR.

---

## 6. Phase Exit Gates

| Phase | Required exit |
|---|---|
| 0 | Reproducible baseline, accepted architecture/policy ADRs, migration plan |
| 1 | Auth isolation, no query JWT, no secret logging, sensitive-action reauthentication, abuse controls, Super Admin MFA |
| 2 | Target Platform boundary operational; outbox/inbox and schema ownership |
| 3 | Fixed-point types, one fee model, double-entry ledger, invariants |
| 4 | Late entry, economics lock, start/refund, all scheduler families |
| 5 | `tralent_v1`, one Settlement owner, exact payout/reconstruction |
| 6 | Durable deterministic Engine with recovery/load evidence |
| 7 | Quality-aware Market Data with safe switching and retained evidence |
| 8 | Secure gateways, KYC, deposits, manual withdrawal, reconciliation |
| 9 | Separate trade frontend and critical bilingual E2E flows |
| 10 | Reproducible Compose release, observability, backup/restore, runbooks |
| 11 | All launch gates pass through staged rollout |

---

## 7. Independent Codex Tasks



## Phase 0 — Baseline

### FND-001 — Create the production baseline and repository inventory

```text
You are implementing Tragge task `FND-001`: **Create the production baseline and repository inventory**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fnd-001-create-the-production-baseline-and-reposit`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- None

Primary scope:
- README.md
- docs/**
- .tool-versions or equivalent
- Makefile
- go.work
- package.json

Required implementation:
- Record the verified application/package inventory, current runtime topology, database/migration count, test coverage gaps, toolchain versions, and known execution limitations.
- Create `docs/architecture/current-state-audit.md` with evidence paths and severity labels.
- Define the supported local and CI toolchain; do not change product behavior.

Acceptance criteria:
- The audit is reproducible from documented commands.
- Every P0/P1 finding references an actual repository path.
- The document explicitly says the current repository is NO-GO for paid production.

Verification:
- Run inventory scripts.
- Validate Markdown links.
- Do not claim the full suite passes if required tools are unavailable.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `docs(audit): add reproducible production baseline`.
- Push and open a PR; merge only after all required checks pass.
```

### FND-002 — Record the target architecture ADR

```text
You are implementing Tragge task `FND-002`: **Record the target architecture ADR**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fnd-002-record-the-target-architecture-adr`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FND-001

Primary scope:
- docs/adr/**
- docs/architecture/**

Required implementation:
- Create an ADR that fixes three bounded systems: Platform modular monolith, Trading Engine, Market Data.
- Define Platform runtime modes `api`, `realtime`, and `worker`.
- Define schema/credential ownership, allowed communication, outbox/inbox, and forbidden cross-system SQL.
- Document why `api-server`, `trading-core`, and `worker` wrappers are transitional and must be retired.

Acceptance criteria:
- ADR status is Accepted.
- Architecture diagram and dependency rules are unambiguous.
- The ADR includes migration and rollback principles.

Verification:
- Markdown lint/check.
- Architecture dependency rules reviewed against current imports.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `docs(adr): fix target runtime architecture`.
- Push and open a PR; merge only after all required checks pass.
```

### FND-003 — Create the canonical domain glossary and version catalog

```text
You are implementing Tragge task `FND-003`: **Create the canonical domain glossary and version catalog**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fnd-003-create-the-canonical-domain-glossary-and-v`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FND-002

Primary scope:
- docs/product/**
- packages/contracts/**

Required implementation:
- Define canonical terms including real participant, system participant, base entry fee, late surcharge, Prize Pool, economics lock, Filled Trade, planned winner, actual winner, rank band, reward weight, settlement review, QTY, and asset group.
- Create a version catalog for contest policy, symbol registry, scoring, prize distribution, and event schemas.
- Mark deprecated terms such as product participant capacity and `commission_rate`.

Acceptance criteria:
- The same term has one definition across backend, frontend, SQL, and docs.
- Every versioned rule has a stable identifier format.
- Deprecated terms have explicit migration targets.

Verification:
- Search repository for conflicting terminology and list remediation targets.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `docs(product): add canonical domain glossary`.
- Push and open a PR; merge only after all required checks pass.
```

### FND-004 — Define the disposable-database migration reset strategy

```text
You are implementing Tragge task `FND-004`: **Define the disposable-database migration reset strategy**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fnd-004-define-the-disposable-database-migration-r`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FND-001,FND-003

Primary scope:
- packages/db/migrations/**
- packages/db/init/**
- docs/architecture/**

Required implementation:
- Because the current database can be discarded, define a clean baseline migration strategy instead of preserving broken legacy semantics indefinitely.
- Inventory all 98 up migrations and classify keep, fold, replace, or delete.
- Design a fresh-install baseline plus a controlled legacy import path only if later required.
- Preserve migration traceability in documentation.

Acceptance criteria:
- A blank database can be created from one documented command.
- No duplicate fee, prize, or participant-capacity source remains in the target schema.
- Down/rollback policy is documented.

Verification:
- Fresh install SQL validation.
- Schema diff against declared target model.
- Migration naming/order check.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `docs(db): define clean migration baseline strategy`.
- Push and open a PR; merge only after all required checks pass.
```

### FND-005 — Establish Codex task and branch operating rules

```text
You are implementing Tragge task `FND-005`: **Establish Codex task and branch operating rules**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fnd-005-establish-codex-task-and-branch-operating-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FND-001

Primary scope:
- docs/codex/**
- CONTRIBUTING.md
- .github/**

Required implementation:
- Create the task template, one-task-one-goal rule, scoped-files rule, targeted-test rule, full-suite-at-epic-end rule, dependency approval rule, ADR rule, and Conventional Commit rule.
- Document test-repository and canonical-main-repository flow, branch naming, protected main, merge conditions, and rollback.
- Require implementation summary and unresolved-risk notes for each task.

Acceptance criteria:
- A new Codex session can execute a task without unstated process assumptions.
- Push/merge is allowed only after acceptance criteria pass.
- The template prevents unrelated refactors.

Verification:
- Dry-run the template on one documentation-only task.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `docs(codex): establish execution protocol`.
- Push and open a PR; merge only after all required checks pass.
```


## Phase 1 — Stop the Bleeding

### SEC-001 — Restore cryptographic isolation between user and admin authentication

```text
You are implementing Tragge task `SEC-001`: **Restore cryptographic isolation between user and admin authentication**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sec-001-restore-cryptographic-isolation-between-us`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FND-002

Primary scope:
- apps/api-server/**
- apps/user-bff/**
- apps/admin-bff/**
- packages/auth/**
- infra/docker/**

Required implementation:
- Remove the shared generic auth singleton used by the merged runtime.
- Use distinct signing keys, audiences, refresh-token cookies, session namespaces, token validators, and CSRF contexts for user and admin.
- Add startup validation that production secrets differ and meet strength requirements.
- Keep compatibility only behind an explicit temporary migration flag.

Acceptance criteria:
- A user token cannot authenticate to any admin endpoint and vice versa.
- Cross-audience and cross-key tests fail closed.
- Production startup fails on missing, equal, or weak secrets.

Verification:
- Unit tests for token validation.
- Integration tests for user/admin boundary.
- Cookie and session namespace tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `fix(auth): isolate user and admin security contexts`.
- Push and open a PR; merge only after all required checks pass.
```

### SEC-002 — Remove session JWT support from URL query parameters

```text
You are implementing Tragge task `SEC-002`: **Remove session JWT support from URL query parameters**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sec-002-remove-session-jwt-support-from-url-query-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SEC-001

Primary scope:
- packages/auth/middleware.go
- apps/**/server/**
- apps/*-frontend/src/**

Required implementation:
- Delete acceptance of `?token=` for normal sessions.
- Replace download or WebSocket exceptions with secure cookies, Authorization headers, or short-lived single-use signed tickets.
- Ensure access logs and analytics never receive session credentials.

Acceptance criteria:
- No session middleware reads JWTs from query strings.
- WebSocket and download flows have dedicated bounded credentials.
- Regression tests cover rejected query-token access.

Verification:
- Auth middleware tests.
- WebSocket handshake tests.
- Repository search for query-token construction.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `fix(security): remove jwt query authentication`.
- Push and open a PR; merge only after all required checks pass.
```

### SEC-003 — Make OTP and reset delivery fail closed in production

```text
You are implementing Tragge task `SEC-003`: **Make OTP and reset delivery fail closed in production**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sec-003-make-otp-and-reset-delivery-fail-closed-in`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SEC-001

Primary scope:
- apps/user-bff/**
- packages/sms/**
- packages/notification/**
- packages/secrets/**

Required implementation:
- Remove production fallback that logs OTPs or reset codes.
- Implement provider-required startup/feature validation.
- Route email OTP through Mailerino for Iranian users and Resend for foreign users.
- Use 10-minute OTP TTL, five attempts, 60-second resend cooldown, hashed-at-rest codes, one-time consumption, and rate limits.
- Redact OTPs and reset tokens from logs and errors.

Acceptance criteria:
- No secret code appears in logs under any configuration.
- Missing production provider config fails closed.
- OTP lifecycle and country routing follow policy.

Verification:
- Unit tests with fake provider that never logs payload.
- Integration tests for expiry, attempts, resend, replay, and rate limits.
- Log-capture secret scan.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `fix(identity): harden email otp delivery`.
- Push and open a PR; merge only after all required checks pass.
```

### SEC-004 — Implement sensitive-action password reauthentication and privileged-action enforcement

```text
You are implementing Tragge task `SEC-004`: **Implement sensitive-action password reauthentication and privileged-action enforcement**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sec-004-sensitive-action-password-reauthentication`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SEC-001,SEC-003

Primary scope:
- apps/admin-bff/**
- apps/admin-frontend/**
- packages/auth/**
- packages/db/migrations/**

Required implementation:
- Require fresh Admin-password verification before withdrawal completion, rejected-withdrawal deduction when implemented, destructive Wallet/balance adjustments, security-sensitive overrides, elevated role/permission changes where implemented, and every action explicitly classified as destructive or security-sensitive.
- Issue only short-lived, single-use, Admin-context-specific reauthentication grants bound to actor, active Admin session, action, and resource where applicable.
- Reject grants after expiry, replay, password change, session revocation, or permission change; never put a grant in a URL or log.
- Enforce canonical `USER`, `SUPPORT_ADMIN`, and `SUPER_ADMIN` permissions without a Finance role. Only Super Admin may execute approved destructive financial operations.
- Require a reason where policy or workflow requires one and write immutable success and safe authorization-denial audit events.
- Do not implement, activate, require, or partially roll out Super Admin login MFA in this task; planned `SEC-007` owns that work.

Acceptance criteria:
- Every covered sensitive action rejects missing, stale, replayed, cross-session, cross-actor, cross-action, or wrong-resource reauthentication.
- Password change, session revocation, and permission change invalidate outstanding grants.
- Support Admin cannot execute a Super-Admin-only destructive financial operation, and the denial is safely audited.
- Mandatory reasons and immutable audit records are enforced without logging passwords or grants.
- Canonical roles remain `USER`, `SUPPORT_ADMIN`, and `SUPER_ADMIN`; no Finance role exists.
- SEC-001 through SEC-003 regressions pass and no MFA capability is falsely marked implemented.

Verification:
- Password-verification and reauthentication-grant unit tests.
- Real-database grant lifecycle, replay, expiry, invalidation, and concurrency tests.
- Privileged-action permission matrix and safe denial-audit tests.
- Admin frontend confirmation and sensitive-action E2E tests.
- SEC-001 through SEC-003 regression suites.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(admin-auth): enforce sensitive action reauthentication`.
- Push and open a PR; merge only after all required checks pass.
```
### SEC-005 — Centralize secret redaction and security logging

```text
You are implementing Tragge task `SEC-005`: **Centralize secret redaction and security logging**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sec-005-centralize-secret-redaction-and-security-l`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SEC-003

Primary scope:
- packages/observability/**
- packages/secrets/**
- apps/**

Required implementation:
- Create structured redaction for Authorization, cookies, OTP, reset tokens, payment secrets, webhook signatures, KYC metadata, and private payload fields.
- Add correlation IDs without exposing sensitive values.
- Make unsafe logging a lint/test failure where practical.

Acceptance criteria:
- Known secret fields are redacted in JSON and text logs.
- Panic and error paths use the same redaction.
- A log fixture test proves no seeded secret survives.

Verification:
- Unit tests for redactor.
- Integration log-capture tests.
- Secret-pattern scan on test logs.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `fix(observability): redact credentials and sensitive data`.
- Push and open a PR; merge only after all required checks pass.
```

### SEC-006 — Add edge security, abuse controls, and security regression tests

```text
You are implementing Tragge task `SEC-006`: **Add edge security, abuse controls, and security regression tests**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sec-006-add-edge-security-abuse-controls-and-secur`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SEC-001,SEC-002,SEC-003,SEC-004

Primary scope:
- apps/*-bff/**
- apps/platform/**
- infra/docker/**
- tests/**

Required implementation:
- Standardize request size limits, origin checks, CSRF, CORS, security headers, IP/user rate limits, login lockout, OTP throttling, and payment webhook allow rules.
- Provide separate limits for join, order, cancel, login, OTP, deposit, withdrawal, and admin actions.
- Avoid trusting unverified proxy headers.
- Apply [PAYMENT4-RETIREMENT-2026-08-01](../product/payment4-retirement-policy-amendment.md):
  remove Payment4 from active runtime, configuration, routes, secrets, tests,
  frontends, and operations surfaces without adding a replacement provider.
- Replace obsolete Payment4 end-to-end evidence with executable retirement,
  remaining-provider, fresh-initialization, and regression evidence.

Acceptance criteria:
- Every public endpoint class has an explicit abuse policy.
- Security headers and CORS differ correctly for user/admin origins.
- Rate-limit decisions are observable and deterministic.
- Payment4 has no active implementation, selectable configuration, route,
  webhook, secret, startup dependency, frontend option, or runtime-test gate.
- Remaining provider security controls pass independently without Payment4.

Verification:
- Unit and integration tests for each endpoint class.
- Proxy/header spoof tests.
- Basic OWASP regression suite.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(security): add abuse controls and retire Payment4`.
- Push and open a PR; merge only after all required checks pass.
```

### SEC-007 — Implement Super Admin MFA before paid-production approval

```text
You are implementing Tragge task `SEC-007`: **Implement Super Admin MFA before paid-production approval**.

Status:
- Implemented by the SEC-007 task change set; Git delivery evidence is recorded
  in `docs/codex/reports/SEC-007-git-execution-report.md`.
- Required before paid-production approval can be reconsidered.
- The versioned Admin MFA assurance contract is `super_admin_totp_v1`.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sec-007-super-admin-mfa`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SEC-001,SEC-004,SEC-005,SEC-006

Primary scope:
- apps/admin-bff/**
- apps/admin-frontend/**
- packages/auth/**
- packages/db/migrations/**
- docs/security/**

Required implementation:
- Implement Google-Authenticator-compatible TOTP for Super Admin login without weakening the isolated Admin trust domain.
- Implement secure enrollment, encrypted TOTP-secret storage, verification, replay prevention, recovery codes, and audited reset/recovery procedures.
- Upgrade a Super Admin session only after successful MFA and preserve explicit Support Admin/Super Admin authorization boundaries.
- Add production startup validation for required MFA configuration without exposing secret material.
- Implement the Admin frontend enrollment, login challenge, recovery, and reset flows.
- Preserve the short-lived sensitive-action password reauthentication and privileged-action enforcement introduced by `SEC-004`.

Acceptance criteria:
- Super Admin cannot obtain an MFA-upgraded session without a valid password and valid enrolled TOTP after the approved activation/migration policy applies.
- Enrollment, replay, recovery-code use, reset/recovery, and session upgrade fail closed and are immutably audited.
- TOTP secrets are encrypted at rest, never logged, and never returned after enrollment.
- Production startup rejects missing or unsafe MFA configuration.
- Real database, concurrency, clock-window, recovery, and Admin frontend E2E tests pass.
- Paid-production remains `NO-GO` until this task and every other launch gate pass.

Verification:
- RFC-compatible TOTP vector and clock-window tests.
- Real-database enrollment, encryption, replay, recovery-code, reset, and concurrent-consumption tests.
- Admin login/session-upgrade and authorization integration tests.
- Frontend enrollment, challenge, recovery, and login E2E tests.
- Production configuration, audit, secret-scan, and SEC-001 through SEC-006 regression tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(admin-auth): require super admin mfa`.
- Push and open a PR; merge only after all required checks pass.
```

## Phase 2 — Architecture

### ARCH-001 — Create the Platform modular-monolith skeleton

```text
You are implementing Tragge task `ARCH-001`: **Create the Platform modular-monolith skeleton**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/arch-001-create-the-platform-modular-monolith-skele`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FND-002,FND-004

Primary scope:
- apps/platform/**
- packages/**
- go.work
- infra/docker/**

Required implementation:
- Create `apps/platform` with `cmd/platform` and internal modules for identity, contest, wallet, payment, kyc, settlement, leaderboard, notification, ticket, admin, and scheduler.
- Implement runtime modes api, realtime, worker with independent health/readiness checks.
- Define module interfaces and prohibit direct handler-to-foreign-repository access.

Acceptance criteria:
- All three modes build from one versioned image.
- Each mode exposes accurate liveness/readiness.
- Dependency direction is enforced by package layout and tests.

Verification:
- Go build and unit tests.
- Architecture import-boundary test.
- Mode startup smoke tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(platform): add modular monolith runtime skeleton`.
- Push and open a PR; merge only after all required checks pass.
```

### ARCH-002 — Migrate identity and admin boundaries into Platform

```text
You are implementing Tragge task `ARCH-002`: **Migrate identity and admin boundaries into Platform**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/arch-002-migrate-identity-and-admin-boundaries-into`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-001,SEC-001,SEC-004

Primary scope:
- apps/user-bff/**
- apps/admin-bff/**
- apps/platform/internal/modules/identity/**
- apps/platform/internal/modules/admin/**

Required implementation:
- Move identity use cases and repositories without changing external API behavior unless documented by contract versioning.
- Keep user/admin auth contexts separate inside the monolith.
- Move role and permission enforcement to application services, not only handlers.

Acceptance criteria:
- Identity endpoints run from Platform API mode.
- Standalone BFF compatibility tests pass during migration.
- No admin repository is reachable through user handlers.

Verification:
- Contract tests old vs new.
- Auth boundary integration tests.
- Targeted race tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(platform): migrate identity and admin modules`.
- Push and open a PR; merge only after all required checks pass.
```

### ARCH-003 — Migrate contest, scheduler, leaderboard projection, notification, and ticket modules

```text
You are implementing Tragge task `ARCH-003`: **Migrate contest, scheduler, leaderboard projection, notification, and ticket modules**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/arch-003-migrate-contest-scheduler-leaderboard-proj`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-001,FND-003

Primary scope:
- apps/contest-scheduler/**
- apps/free-contest-generator/**
- apps/leaderboard-worker/**
- packages/ticket/**
- packages/notification/**
- apps/platform/internal/modules/**

Required implementation:
- Move domain use cases into Platform modules while preserving one scheduler implementation and one live leaderboard projection.
- Merge free-contest generation into the canonical scheduler.
- Retain workers as Platform worker-mode jobs, not separate domain services.
- Keep notifications and tickets transactional through outbox events.

Acceptance criteria:
- Old endpoints/events have compatibility tests until cutover.
- Only one scheduler owns contest generation.
- Leaderboard projection has no settlement authority.

Verification:
- Module unit tests.
- Worker-mode integration tests.
- Duplicate scheduler instance test.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(platform): migrate contest support modules`.
- Push and open a PR; merge only after all required checks pass.
```

### ARCH-004 — Migrate wallet, payment, KYC, and withdrawal modules

```text
You are implementing Tragge task `ARCH-004`: **Migrate wallet, payment, KYC, and withdrawal modules**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/arch-004-migrate-wallet-payment-kyc-and-withdrawal-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-001,DATA-003

Primary scope:
- apps/payment-service/**
- packages/wallet/**
- packages/kyc/**
- apps/platform/internal/modules/**

Required implementation:
- Move business orchestration into Platform modules while providers remain adapters.
- Ensure one transaction boundary for ledger postings and local payment state.
- Keep external provider calls outside database transactions and use durable state machines.

Acceptance criteria:
- Payment and withdrawal APIs run from Platform.
- Provider adapters are replaceable and contract-tested.
- No direct balance update bypasses the ledger service.

Verification:
- Payment state-machine tests.
- Ledger integration tests.
- Webhook idempotency tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(platform): migrate financial modules`.
- Push and open a PR; merge only after all required checks pass.
```

### ARCH-005 — Migrate settlement and remove finalization authority from leaderboard

```text
You are implementing Tragge task `ARCH-005`: **Migrate settlement and remove finalization authority from leaderboard**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/arch-005-migrate-settlement-and-remove-finalization`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-001,PRIZE-005

Primary scope:
- apps/settlement-service/**
- apps/leaderboard-worker/server/finalize.go
- apps/platform/internal/modules/settlement/**
- apps/platform/internal/modules/leaderboard/**

Required implementation:
- Move settlement orchestration into Platform worker mode.
- Reduce leaderboard to live/final read-model projection.
- Delete or disable every leaderboard path that completes contests, owns payout retry, or credits wallets.
- Preserve audit history through migration.

Acceptance criteria:
- One code path owns final completion and payout.
- Duplicate event delivery cannot trigger a second finalization owner.
- Leaderboard rebuild does not mutate money.

Verification:
- Settlement ownership integration test.
- Repository search for wallet credits outside wallet/settlement modules.
- Failure/retry tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(settlement): establish single finalization owner`.
- Push and open a PR; merge only after all required checks pass.
```

### ARCH-006 — Implement schema ownership and transactional outbox/inbox

```text
You are implementing Tragge task `ARCH-006`: **Implement schema ownership and transactional outbox/inbox**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/arch-006-implement-schema-ownership-and-transaction`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-001,FND-004

Primary scope:
- packages/db/**
- apps/platform/**
- apps/trading-engine/**
- apps/market-ingestor/**
- packages/contracts/**

Required implementation:
- Create separate schemas/roles for Platform, Engine, and Market Data.
- Implement transactional outbox in each producer and idempotent inbox in each consumer.
- Add retry, dead-letter, ordering key, aggregate version, and observability.
- Remove cross-bounded-system SQL access.

Acceptance criteria:
- Database grants prevent unauthorized schema access.
- Outbox events survive process crash after commit.
- Inbox deduplicates replayed events.

Verification:
- Permission tests with real PostgreSQL.
- Crash-window integration tests.
- Event ordering and duplicate tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(events): add owned schemas and outbox inbox`.
- Push and open a PR; merge only after all required checks pass.
```

### ARCH-007 — Retire merged wrappers and dead standalone runtimes

```text
You are implementing Tragge task `ARCH-007`: **Retire merged wrappers and dead standalone runtimes**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/arch-007-retire-merged-wrappers-and-dead-standalone`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-002,ARCH-003,ARCH-004,ARCH-005,ARCH-006,ENG-001,MD-001

Primary scope:
- apps/api-server/**
- apps/trading-core/**
- apps/worker/**
- apps/free-contest-generator/**
- apps/contest-scheduler/**
- infra/**
- go.work
- README.md

Required implementation:
- Cut traffic to target runtimes, then delete merged wrappers and obsolete standalone binaries after compatibility tests pass.
- Update go.work, Docker, docs, health checks, and CI build targets.
- Keep rollback tags/images for the prior release; do not keep unreachable source indefinitely.

Acceptance criteria:
- Only Platform, Trading Engine, and Market Data backend images are production-deployed.
- No stale ingress/compose target references deleted apps.
- Repository build graph no longer includes wrappers.

Verification:
- Full backend build.
- Compose config validation.
- Smoke tests against all public routes.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(runtime): retire merged legacy applications`.
- Push and open a PR; merge only after all required checks pass.
```


## Phase 3 — Data and Money

### DATA-001 — Introduce canonical fixed-point money, price, rate, and score types

```text
You are implementing Tragge task `DATA-001`: **Introduce canonical fixed-point money, price, rate, and score types**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/data-001-introduce-canonical-fixed-point-money-pric`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FND-003,FND-004

Primary scope:
- packages/money/**
- packages/scoring/**
- packages/contracts/**
- packages/db/migrations/**

Required implementation:
- Create explicit types for money minor units, basis points, price ticks, decimal score, and rational weight.
- Ban float64 at financial and execution boundaries through APIs, contracts, repositories, and static checks.
- Define parsing, formatting, overflow, and rounding modes.

Acceptance criteria:
- Money and price cannot be accidentally mixed.
- All conversions are explicit and tested.
- Overflow and invalid precision fail closed.

Verification:
- Property tests for arithmetic.
- Golden serialization tests.
- Repository scan for prohibited float use in target paths.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(types): add fixed point financial primitives`.
- Push and open a PR; merge only after all required checks pass.
```

### DATA-002 — Replace duplicate fee fields with canonical economics columns

```text
You are implementing Tragge task `DATA-002`: **Replace duplicate fee fields with canonical economics columns**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/data-002-replace-duplicate-fee-fields-with-canonica`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- DATA-001,FND-004

Primary scope:
- packages/db/migrations/**
- apps/user-bff/server/contest_handlers.go
- apps/user-bff/server/contest_prizes.go
- apps/admin-frontend/**
- apps/platform/**

Required implementation:
- Make `platform_fee_bps` the sole base-fee field and default it to 2000.
- Add explicit late-surcharge columns and ledger accounts.
- Remove `commission_rate` reads/writes and UI controls after migration.
- Add constraints for valid basis points and immutable locked economics.

Acceptance criteria:
- Join, preview, admin, and settlement read the same fields.
- No runtime fallback to commission_rate remains.
- Database rejects invalid or mutated locked economics.

Verification:
- Migration fresh-install test.
- Join economics integration tests.
- Repository search for commission_rate.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(economics): canonicalize platform fee fields`.
- Push and open a PR; merge only after all required checks pass.
```

### DATA-003 — Implement the double-entry ledger

```text
You are implementing Tragge task `DATA-003`: **Implement the double-entry ledger**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/data-003-implement-the-double-entry-ledger`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- DATA-001,FND-004

Primary scope:
- packages/wallet/**
- apps/platform/internal/modules/wallet/**
- packages/db/migrations/**

Required implementation:
- Create immutable journal entries and postings with balanced debits/credits.
- Implement user available, user reserved, Prize Pool, platform fee, late surcharge, deposit clearing, withdrawal pending, gateway fee, and adjustment clearing accounts.
- Replace direct balance mutation with ledger transactions and derived/validated balances.

Acceptance criteria:
- Every transaction balances exactly.
- Duplicate idempotency keys return the original result.
- Direct UPDATE of wallet balance is absent from application paths.

Verification:
- Property-based balance tests.
- Concurrent debit tests.
- Crash/retry integration tests.
- Reconciliation query tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(ledger): implement double entry wallet accounting`.
- Push and open a PR; merge only after all required checks pass.
```

### DATA-004 — Add financial invariants, idempotency, and database constraints

```text
You are implementing Tragge task `DATA-004`: **Add financial invariants, idempotency, and database constraints**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/data-004-add-financial-invariants-idempotency-and-d`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- DATA-002,DATA-003

Primary scope:
- packages/db/migrations/**
- apps/platform/internal/modules/wallet/**
- apps/platform/internal/modules/contest/**

Required implementation:
- Add unique operation keys, immutable journal constraints, nonnegative available-balance policy, contest economics checks, payout uniqueness, payment provider-ID uniqueness, and state-transition guards.
- Implement a reconciliation command that exits nonzero on any unexplained difference.

Acceptance criteria:
- Concurrent duplicate requests cannot double debit, credit, refund, or pay.
- Constraint failures are mapped to stable domain errors.
- Reconciliation is deterministic.

Verification:
- Database concurrency tests.
- Retry storm tests.
- Reconciliation fixtures.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(finance): enforce ledger and contest invariants`.
- Push and open a PR; merge only after all required checks pass.
```

### DATA-005 — Formalize system-account semantics

```text
You are implementing Tragge task `DATA-005`: **Formalize system-account semantics**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/data-005-formalize-system-account-semantics`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- DATA-003,FND-003

Primary scope:
- packages/db/migrations/**
- apps/platform/internal/modules/contest/**
- apps/platform/internal/modules/leaderboard/**
- apps/trading-engine/**

Required implementation:
- Define immutable system-account classification and the single practice account.
- Enforce that system accounts cannot join paid contests, affect minimum real participants, economics, winner count, eligibility, or payout.
- Represent leaderboard rank zero explicitly rather than through normal ranking SQL.

Acceptance criteria:
- All economics queries filter system participants by construction.
- System account cannot receive ledger prize postings.
- Free leaderboard consistently shows rank zero.

Verification:
- SQL query tests.
- Paid join authorization test.
- Settlement exclusion test.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(contest): enforce practice system account rules`.
- Push and open a PR; merge only after all required checks pass.
```


## Phase 4 — Contest Lifecycle and Scheduler

### CON-001 — Redesign contest lifecycle for running late entry

```text
You are implementing Tragge task `CON-001`: **Redesign contest lifecycle for running late entry**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/con-001-redesign-contest-lifecycle-for-running-lat`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-003,DATA-002

Primary scope:
- packages/domain/statemachine/**
- apps/platform/internal/modules/contest/**
- packages/db/migrations/**
- packages/contracts/**

Required implementation:
- Replace status-only registration checks with explicit `join_opens_at` and `join_cutoff_at`.
- Allow paid contests in RUNNING state to accept entry before cutoff.
- Add `economics_locked_at`, `engine_session_status`, and guarded transitions.
- Free contests reject entry at start.

Acceptance criteria:
- The state machine has no ambiguous registration status.
- Every transition is idempotent and audited.
- A contest can run and accept a permitted late entrant without reopening status.

Verification:
- Table-driven state tests.
- Boundary tests at exact timestamps.
- Concurrent transition tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(contest): support explicit late join lifecycle`.
- Push and open a PR; merge only after all required checks pass.
```

### CON-002 — Implement atomic on-time and late-entry checkout

```text
You are implementing Tragge task `CON-002`: **Implement atomic on-time and late-entry checkout**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/con-002-implement-atomic-on-time-and-late-entry-ch`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- CON-001,DATA-003

Primary scope:
- apps/platform/internal/modules/contest/**
- apps/user-frontend/**
- packages/contracts/**

Required implementation:
- Calculate base fee split and late surcharge from one canonical quote.
- Show base fee, surcharge, total, and wallet balance before confirmation.
- Atomically debit wallet, add participant, post fee/pool ledger entries, and return idempotent result.
- Reject joins at or after cutoff.

Acceptance criteria:
- A 100-USDT late join posts 80 Prize Pool, 20 base fee, 10 late revenue.
- Repeated request cannot join or charge twice.
- Checkout preview equals posted ledger amounts.

Verification:
- Boundary and concurrency integration tests.
- Frontend contract tests.
- Ledger posting assertions.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(contest): add atomic late entry checkout`.
- Push and open a PR; merge only after all required checks pass.
```

### CON-003 — Lock immutable contest economics at the join cutoff

```text
You are implementing Tragge task `CON-003`: **Lock immutable contest economics at the join cutoff**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/con-003-lock-immutable-contest-economics-at-the-jo`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- CON-001,CON-002,PRIZE-001

Primary scope:
- apps/platform/internal/modules/contest/**
- packages/db/migrations/**
- packages/contracts/**

Required implementation:
- At cutoff, capture real participants, base gross, base fees, late surcharges, Prize Pool, planned winners, and all policy versions.
- Use a serializable/idempotent lock operation.
- Publish an economics-locked event for preview and settlement.
- Prevent later participant or economics mutation.

Acceptance criteria:
- Exactly one immutable snapshot exists per contest.
- Snapshot totals reconcile to ledger postings.
- Settlement can run without recalculating registration economics.

Verification:
- Cutoff race tests.
- Crash/retry tests.
- Snapshot-to-ledger reconciliation tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(contest): lock economics after registration`.
- Push and open a PR; merge only after all required checks pass.
```

### CON-004 — Implement start qualification, full refund, and lazy Engine activation

```text
You are implementing Tragge task `CON-004`: **Implement start qualification, full refund, and lazy Engine activation**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/con-004-implement-start-qualification-full-refund-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- CON-001,DATA-005,ENG-001

Primary scope:
- apps/platform/internal/modules/contest/**
- apps/trading-engine/**
- packages/contracts/**

Required implementation:
- At start count real participants only.
- Paid contests with fewer than two real users cancel and issue full idempotent refunds.
- Qualified contests publish a versioned start command and create an Engine session.
- Free sessions activate only when at least one real user exists; the practice account alone consumes no Engine session.

Acceptance criteria:
- No unqualified paid contest starts.
- Refund totals exactly reverse all entry postings.
- Upcoming contests consume no Engine session.

Verification:
- Start-boundary integration tests.
- Refund idempotency tests.
- Engine session count tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(contest): qualify starts and activate engine lazily`.
- Push and open a PR; merge only after all required checks pass.
```

### CON-005 — Implement versioned scheduler templates and custom contests

```text
You are implementing Tragge task `CON-005`: **Implement versioned scheduler templates and custom contests**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/con-005-implement-versioned-scheduler-templates-an`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-003,CON-001

Primary scope:
- apps/platform/internal/modules/scheduler/**
- apps/platform/internal/modules/admin/**
- packages/db/migrations/**
- packages/contracts/**

Required implementation:
- Create versioned templates for market, duration, cadence, fee set, max QTY, late-entry flag, and enabled state.
- Custom contest input includes start, duration, market, fee, QTY choice, and late-entry flag; calculate end automatically.
- Allow overlap.
- Apply edits only to ungenerated contests; preserve generated contests with participants.

Acceptance criteria:
- Every generated contest references an immutable template version.
- Disabling stops generation and safely archives participant-free upcoming records.
- Admin cannot mutate locked instance fields.

Verification:
- Template version tests.
- Custom validation tests.
- Disable behavior integration tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(scheduler): add versioned templates and custom contests`.
- Push and open a PR; merge only after all required checks pass.
```

### SCH-001 — Build the deterministic Tehran-time scheduler core

```text
You are implementing Tragge task `SCH-001`: **Build the deterministic Tehran-time scheduler core**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sch-001-build-the-deterministic-tehran-time-schedu`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- CON-005

Primary scope:
- apps/platform/internal/modules/scheduler/**
- packages/contracts/**
- packages/db/migrations/**

Required implementation:
- Use Asia/Tehran for slot math and UTC for storage/API.
- Implement distributed lock or unique-insert reconciliation safe under multiple workers.
- Use canonical key template-version + start + fee.
- Generate to target windows rather than one-shot cron assumptions.
- Archive rather than hard-delete historical contests.

Acceptance criteria:
- Two concurrent schedulers produce no duplicates.
- Restart fills only missing slots.
- Timezone conversion is stable across DST database updates.

Verification:
- Property tests across dates.
- Concurrent worker integration test.
- UTC round-trip tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(scheduler): add deterministic slot reconciliation`.
- Push and open a PR; merge only after all required checks pass.
```

### SCH-002 — Implement paid 30-minute schedule

```text
You are implementing Tragge task `SCH-002`: **Implement paid 30-minute schedule**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sch-002-implement-paid-30-minute-schedule`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SCH-001

Primary scope:
- apps/platform/internal/modules/scheduler/**
- apps/platform/internal/modules/contest/**

Required implementation:
- Generate starts every 10 minutes for Crypto and eligible Forex.
- Generate fee variants 5, 10, 20 and max QTY 5.
- Maintain six future start slots and at most 36 records.
- Do not replace missing Forex records when closed.
- Require the full Forex interval to be tradable.

Acceptance criteria:
- The exact upcoming set matches golden fixtures at arbitrary current times.
- Started slots leave the upcoming query and the next slot appears.
- No duplicate or out-of-window record exists.

Verification:
- Clock-controlled golden tests.
- Forex-open/closed tests.
- Scheduler restart test.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(scheduler): generate 30 minute contests`.
- Push and open a PR; merge only after all required checks pass.
```

### SCH-003 — Implement free one-hour queue and practice account registration

```text
You are implementing Tragge task `SCH-003`: **Implement free one-hour queue and practice account registration**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sch-003-implement-free-one-hour-queue-and-practice`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SCH-001,DATA-005

Primary scope:
- apps/platform/internal/modules/scheduler/**
- apps/platform/internal/modules/contest/**

Required implementation:
- Generate starts every hour at minute 30.
- When Forex is open maintain five future starts with Crypto and Forex.
- When Forex is closed maintain eight future Crypto-only starts.
- Auto-register the practice system account idempotently.
- Set free, no-prize, no-official-ranking policy fields.

Acceptance criteria:
- Upcoming free queue exactly matches policy in both market states.
- Practice account is visible but excluded from real counts.
- No old free generator is required.

Verification:
- Golden schedule tests.
- System-account registration tests.
- Transition open-to-closed queue tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(scheduler): generate free practice queue`.
- Push and open a PR; merge only after all required checks pass.
```

### SCH-004 — Implement four-hour and daily schedules

```text
You are implementing Tragge task `SCH-004`: **Implement four-hour and daily schedules**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sch-004-implement-four-hour-and-daily-schedules`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SCH-001,SCH-006

Primary scope:
- apps/platform/internal/modules/scheduler/**

Required implementation:
- Implement all fixed Crypto and Forex four-hour slots and fee sets.
- Implement Crypto daily 01:30-to-01:30 and Forex 01:30-to-00:20 schedules.
- Calculate display horizon through the fourth valid Forex trading day after today, excluding today.
- Include Crypto calendar days through the same endpoint.

Acceptance criteria:
- Golden fixtures match slot, fee, QTY, and horizon rules.
- Invalid Forex intervals are skipped, not moved.
- Crypto remains continuous through weekends.

Verification:
- Date-table tests around weekends.
- Boundary and horizon tests.
- Idempotency tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(scheduler): generate four hour and daily contests`.
- Push and open a PR; merge only after all required checks pass.
```

### SCH-005 — Implement weekly schedules and launch-profile controls

```text
You are implementing Tragge task `SCH-005`: **Implement weekly schedules and launch-profile controls**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sch-005-implement-weekly-schedules-and-launch-prof`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SCH-001,SCH-006

Primary scope:
- apps/platform/internal/modules/scheduler/**
- apps/platform/internal/modules/admin/**

Required implementation:
- Generate daily seven-day Crypto starts at 01:30 with configured fee set.
- Generate Forex only Monday 01:30 through Saturday 00:20.
- Maintain next four valid Forex Monday slots and the corresponding Crypto horizon.
- Add launch-profile enable flags so high-value templates can remain disabled.

Acceptance criteria:
- Weekly fixtures match exact Tehran timestamps.
- A disabled fee/template is not generated.
- Existing participant-bearing contests survive profile changes.

Verification:
- Multi-week golden tests.
- Template toggle tests.
- Year-boundary tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(scheduler): generate weekly contest series`.
- Push and open a PR; merge only after all required checks pass.
```

### SCH-006 — Implement MVP Forex tradability calendar

```text
You are implementing Tragge task `SCH-006`: **Implement MVP Forex tradability calendar**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sch-006-implement-mvp-forex-tradability-calendar`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SCH-001

Primary scope:
- apps/platform/internal/modules/scheduler/**
- apps/market-ingestor/**
- packages/contracts/**

Required implementation:
- Implement the accepted MVP closure Saturday 00:20 through Monday 01:30 in Asia/Tehran.
- Expose `IsTradableInterval` and require whole-interval validity.
- Keep interface extensible for later provider calendar integration.
- Emit an explicit risk/health flag that official holidays are not covered.

Acceptance criteria:
- All interval-overlap edge cases are correct.
- No hardcoded UTC offsets are used.
- Calendar version is captured on generated Forex contests.

Verification:
- Minute-boundary table tests.
- Timezone tests.
- Interface contract tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(calendar): add mvp forex tradability rules`.
- Push and open a PR; merge only after all required checks pass.
```

### SCH-007 — Build Admin scheduler and template management UI

```text
You are implementing Tragge task `SCH-007`: **Build Admin scheduler and template management UI**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sch-007-build-admin-scheduler-and-template-managem`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- CON-005,SCH-002,SCH-003,SCH-004,SCH-005

Primary scope:
- apps/admin-frontend/**
- apps/platform/internal/modules/admin/**
- packages/contracts/ts/**

Required implementation:
- Replace placeholder auto-scheduling UI with versioned template CRUD, enable/disable, preview, launch-profile controls, and custom contest creation.
- Show generated-slot preview in Tehran and browser time.
- Warn that edits affect only future ungenerated contests.

Acceptance criteria:
- Admin preview matches backend generation exactly.
- Permissions restrict changes to Super Admin.
- Audit records contain before/after versions.

Verification:
- Component tests.
- API contract tests.
- Playwright template and custom-contest flows.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(admin): manage versioned contest schedules`.
- Push and open a PR; merge only after all required checks pass.
```

### SCH-008 — Add scheduler property, concurrency, and recovery test suite

```text
You are implementing Tragge task `SCH-008`: **Add scheduler property, concurrency, and recovery test suite**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/sch-008-add-scheduler-property-concurrency-and-rec`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- SCH-002,SCH-003,SCH-004,SCH-005,SCH-006

Primary scope:
- apps/platform/internal/modules/scheduler/**
- tests/integration/**

Required implementation:
- Build a fake clock and generate fixtures for weekends, month/year boundaries, Tehran timezone changes, worker crashes, concurrent workers, disabled templates, and partially pre-existing windows.
- Assert no gaps or duplicates relative to policy.

Acceptance criteria:
- All schedule families have golden fixtures.
- A scheduler can recover from any interrupted generation step.
- Tests are deterministic and run in CI.

Verification:
- Unit property tests and PostgreSQL integration tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `test(scheduler): cover windows concurrency and recovery`.
- Push and open a PR; merge only after all required checks pass.
```


## Phase 5 — Prize and Settlement

### PRIZE-001 — Implement the shared `tralent_v1` rule package

```text
You are implementing Tragge task `PRIZE-001`: **Implement the shared `tralent_v1` rule package**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/prize-001-implement-the-shared-tralent-v1-rule-packa`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- DATA-001,FND-003

Primary scope:
- packages/scoring/**
- packages/contracts/**
- docs/product/**

Required implementation:
- Implement the explicit 2–11 participant prize-share fixtures, 30% half-up winner count, canonical rank bands, and—only for 12 or more participants—0.8 bucket decay, grouped bucket shares, and reward_weight.
- Use rational/fixed-point arithmetic.
- Version configuration and generate fixtures from rules, not a hand-maintained truth table.

Acceptance criteria:
- All explicit 2–11 participant fixtures and the known 18, 38, 45, 49, and 56 participant examples match approved outputs.
- Rule package has no dependency on HTTP, DB, or UI.
- All sums are exact before money allocation.

Verification:
- Golden tests.
- Property tests over participant counts 2..10000.
- Band-boundary tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(prize): implement tralent v1 rules`.
- Push and open a PR; merge only after all required checks pass.
```

### PRIZE-002 — Implement eligibility and actual-winner calculation

```text
You are implementing Tragge task `PRIZE-002`: **Implement eligibility and actual-winner calculation**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/prize-002-implement-eligibility-and-actual-winner-ca`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PRIZE-001,CON-003,DATA-005

Primary scope:
- packages/scoring/**
- apps/platform/internal/modules/settlement/**

Required implementation:
- Use locked real participant count for planned winners.
- Eligible users require at least one Filled Trade.
- Exclude system accounts and no-trade users from prize table.
- If eligible count is lower, trim occupied weights and renormalize to 100%.

Acceptance criteria:
- No-trade and system users never receive a prize row.
- All locked Prize Pool funds remain distributable to eligible winners.
- Eligibility input is immutable and auditable.

Verification:
- Eligibility matrix tests.
- Low-eligible-count fixtures.
- System-account tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(prize): enforce filled trade eligibility`.
- Push and open a PR; merge only after all required checks pass.
```

### PRIZE-003 — Implement exact money allocation, grouped ranks, and ties

```text
You are implementing Tragge task `PRIZE-003`: **Implement exact money allocation, grouped ranks, and ties**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/prize-003-implement-exact-money-allocation-grouped-r`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PRIZE-001,PRIZE-002,DATA-001

Primary scope:
- packages/scoring/**

Required implementation:
- Allocate integer minor units exactly.
- Keep grouped rank-band members equal.
- Pool occupied prize positions for exact ties and divide equally.
- Use competition ranking and stable display ordering.
- Assign residual only where equality is not broken.

Acceptance criteria:
- Sum payouts equals Prize Pool for every generated property case.
- Tie and band equality always holds.
- No float-based comparison or O(n squared) tie loop remains.

Verification:
- Property tests with random pools/ranks.
- Tie fixtures.
- Residual edge cases.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(prize): add exact grouped and tied allocation`.
- Push and open a PR; merge only after all required checks pass.
```

### PRIZE-004 — Integrate one prize package into previews and APIs

```text
You are implementing Tragge task `PRIZE-004`: **Integrate one prize package into previews and APIs**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/prize-004-integrate-one-prize-package-into-previews-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PRIZE-003,CON-003

Primary scope:
- apps/platform/internal/modules/contest/**
- apps/user-frontend/**
- apps/admin-frontend/**
- packages/contracts/**

Required implementation:
- Replace Power Law preview with `tralent_v1`.
- Before economics lock, label preview as estimated and use current real participants.
- After lock, read immutable snapshot.
- Expose bands, per-user amount, percentage, and reward_weight consistently.

Acceptance criteria:
- User, admin, registration preview, and settlement fixture outputs are identical.
- No preview-local formula remains.
- System user and no-trade rules are represented correctly.

Verification:
- Cross-surface contract tests.
- Golden UI fixtures.
- Repository search for old formula.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(prize): unify potential reward previews`.
- Push and open a PR; merge only after all required checks pass.
```

### PRIZE-005 — Make Settlement the sole finalization owner

```text
You are implementing Tragge task `PRIZE-005`: **Make Settlement the sole finalization owner**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/prize-005-make-settlement-the-sole-finalization-owne`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-005,PRIZE-003

Primary scope:
- apps/settlement-service/**
- apps/leaderboard-worker/server/finalize.go
- apps/platform/internal/modules/settlement/**
- apps/platform/internal/modules/leaderboard/**

Required implementation:
- Move final ranking, prize creation, payout state, completion, and retry ownership into Settlement.
- Remove contest completion and financial finalization from leaderboard worker.
- Define one idempotent settlement aggregate and state machine.

Acceptance criteria:
- Only settlement can transition SETTLING to COMPLETED.
- Only wallet/settlement code can create prize ledger postings.
- Duplicate events do not create parallel finalizers.

Verification:
- Ownership integration test.
- Static repository checks.
- Duplicate event and crash tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `fix(settlement): remove duplicate finalization paths`.
- Push and open a PR; merge only after all required checks pass.
```

### PRIZE-006 — Implement Engine freeze, close, and settlement barrier

```text
You are implementing Tragge task `PRIZE-006`: **Implement Engine freeze, close, and settlement barrier**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/prize-006-implement-engine-freeze-close-and-settleme`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PRIZE-005,ENG-001,ENG-007

Primary scope:
- apps/platform/internal/modules/settlement/**
- apps/trading-engine/**
- packages/contracts/**

Required implementation:
- Freeze commands at end time.
- Cancel pending orders and close positions per symbol using valid final snapshots.
- Require explicit shard/session completion acknowledgements.
- Create immutable result snapshot before ranking.
- Route missing-final-quote symbols to review.

Acceptance criteria:
- No settlement starts ranking before every assigned Engine partition reaches terminal state.
- Repeated freeze/close commands are idempotent.
- Snapshot hashes and versions are stored.

Verification:
- Multi-shard barrier tests.
- Partial failure/retry tests.
- Missing quote tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(settlement): add engine completion barrier`.
- Push and open a PR; merge only after all required checks pass.
```

### PRIZE-007 — Post payouts and reconcile Prize Pool exactly

```text
You are implementing Tragge task `PRIZE-007`: **Post payouts and reconcile Prize Pool exactly**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/prize-007-post-payouts-and-reconcile-prize-pool-exac`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PRIZE-005,PRIZE-006,DATA-003

Primary scope:
- apps/platform/internal/modules/settlement/**
- apps/platform/internal/modules/wallet/**

Required implementation:
- Post all winner payouts in one idempotent settlement operation or resumable batches with unique keys.
- Reconcile locked Prize Pool liability to payout total.
- Record failed external notification separately from financial completion.
- Provide retry without double credit.

Acceptance criteria:
- Prize Pool closes to zero liability after successful settlement.
- A crash after any posting resumes safely.
- Duplicate payout count is zero.

Verification:
- Crash-between-postings tests.
- Ledger reconciliation tests.
- High-winner-count test.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(settlement): reconcile and credit prizes`.
- Push and open a PR; merge only after all required checks pass.
```

### PRIZE-008 — Add settlement review and dispute reconstruction tools

```text
You are implementing Tragge task `PRIZE-008`: **Add settlement review and dispute reconstruction tools**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/prize-008-add-settlement-review-and-dispute-reconstr`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PRIZE-006,PRIZE-007,MD-007

Primary scope:
- apps/platform/internal/modules/settlement/**
- apps/platform/internal/modules/admin/**
- apps/admin-frontend/**
- docs/runbooks/**

Required implementation:
- Expose immutable inputs, quote provenance, orders, fills, positions, score, eligibility, bands, rounding, and ledger postings.
- Allow reviewed resolution through explicit compensating actions, never mutation of history.
- Require reason, permission, reauthentication, and audit.

Acceptance criteria:
- An operator can reconstruct any payout from source events.
- Review actions are reversible through compensating entries.
- No admin endpoint directly edits final rank or balance.

Verification:
- Reconstruction fixture test.
- Admin permission tests.
- Audit completeness test.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(admin): add settlement reconstruction workflow`.
- Push and open a PR; merge only after all required checks pass.
```

### PRIZE-009 — Delete Power Law and obsolete prize implementations

```text
You are implementing Tragge task `PRIZE-009`: **Delete Power Law and obsolete prize implementations**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/prize-009-delete-power-law-and-obsolete-prize-implem`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PRIZE-004,PRIZE-005,PRIZE-007

Primary scope:
- packages/scoring/distribution/**
- packages/scoring/prize/**
- apps/**
- packages/domain/statemachine/**

Required implementation:
- Remove old alpha/Power Law production paths, duplicate winner-count functions, start-time prize locks, and formula fallbacks.
- Keep any legacy reader only if needed to render already-versioned historical fixtures.
- Update docs and generated contracts.

Acceptance criteria:
- Repository has one production prize implementation.
- No contest start locks mutable economics before late cutoff.
- Full prize/settlement suite passes.

Verification:
- Repository search for alpha/power law/legacy locks.
- Full scoring tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(prize): remove obsolete distribution paths`.
- Push and open a PR; merge only after all required checks pass.
```


## Phase 6 — Trading Engine

### ENG-001 — Separate Trading Engine runtime and define Platform contracts

```text
You are implementing Tragge task `ENG-001`: **Separate Trading Engine runtime and define Platform contracts**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/eng-001-separate-trading-engine-runtime-and-define`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FND-002,ARCH-006

Primary scope:
- apps/trading-engine/**
- apps/trading-core/**
- packages/contracts/**
- infra/docker/**

Required implementation:
- Run Trading Engine as an independent process/image with its own schema role.
- Define versioned contest configuration, participant activation, order command, freeze, close, snapshot, and result events.
- Remove provider connections and Platform JWT/session responsibilities from Engine.

Acceptance criteria:
- Engine starts without Market Data provider credentials or Platform DB grants.
- Contract compatibility tests exist.
- Compose can restart Engine independently.

Verification:
- Build and startup tests.
- Schema permission tests.
- Contract golden tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(engine): establish independent runtime boundary`.
- Push and open a PR; merge only after all required checks pass.
```

### ENG-002 — Convert execution prices, PnL, and score to fixed point

```text
You are implementing Tragge task `ENG-002`: **Convert execution prices, PnL, and score to fixed point**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/eng-002-convert-execution-prices-pnl-and-score-to-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ENG-001,DATA-001,MD-001

Primary scope:
- apps/trading-engine/**
- packages/contracts/**
- packages/scoring/**

Required implementation:
- Replace float64 at order validation, fill, position, PnL, TP/SL, and score boundaries.
- Define scale per symbol through registry metadata.
- Use explicit rounding and overflow policy.

Acceptance criteria:
- Same event stream yields byte-identical financial state across replays.
- No float comparison decides fill or rank-affecting score.
- Migration fixtures preserve expected outcomes.

Verification:
- Property tests.
- Golden replay tests.
- Overflow/precision tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(engine): use fixed point execution math`.
- Push and open a PR; merge only after all required checks pass.
```

### ENG-003 — Enforce QTY reservation and complete order validation

```text
You are implementing Tragge task `ENG-003`: **Enforce QTY reservation and complete order validation**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/eng-003-enforce-qty-reservation-and-complete-order`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ENG-002

Primary scope:
- apps/trading-engine/**
- packages/contracts/**

Required implementation:
- Enforce integer QTY, minimum 1, and contest max across active plus pending.
- Reserve and release QTY atomically for create, fill, cancel, reject, close, and recovery.
- Validate market, limit, stop, TP, and SL relationships against side and current quote.
- Add request rate limiting independent of QTY.

Acceptance criteria:
- Concurrent commands cannot oversubscribe QTY.
- Every terminal path releases the correct reservation.
- Invalid TP/SL and stale-price orders fail with stable error codes.

Verification:
- Concurrency/race tests.
- Order matrix tests.
- Recovery reservation tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `fix(engine): enforce qty and order invariants`.
- Push and open a PR; merge only after all required checks pass.
```

### ENG-004 — Make WAL durable and fail closed

```text
You are implementing Tragge task `ENG-004`: **Make WAL durable and fail closed**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/eng-004-make-wal-durable-and-fail-closed`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ENG-001,OPS-004

Primary scope:
- apps/trading-engine/server/wal.go
- apps/trading-engine/server/config.go
- apps/trading-engine/server/app.go
- infra/docker/**

Required implementation:
- Require a writable persistent WAL path in production.
- Acknowledge commands only after durable intent and required state commit.
- Implement bounded group commit only if benchmarked and preserving RPO zero semantics.
- Fail startup or enter controlled read-only mode on WAL corruption/write failure.
- Expose WAL lag, size, fsync latency, and failure metrics.

Acceptance criteria:
- Container restart retains pending WAL.
- No production config silently uses in-memory WAL.
- Injected disk error prevents unsafe new commands.

Verification:
- Crash-window tests.
- Corruption tests.
- Persistent-volume compose test.
- Fsync benchmark.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `fix(engine): require durable wal`.
- Push and open a PR; merge only after all required checks pass.
```

### ENG-005 — Implement incremental snapshots and deterministic replay

```text
You are implementing Tragge task `ENG-005`: **Implement incremental snapshots and deterministic replay**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/eng-005-implement-incremental-snapshots-and-determ`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ENG-002,ENG-004

Primary scope:
- apps/trading-engine/**
- packages/storage/**
- packages/contracts/**

Required implementation:
- Snapshot every 60 seconds or 10,000 mutations by default, plus required lifecycle snapshots.
- Use checksummed, versioned, atomic snapshots with external-storage upload hooks.
- Replay snapshot plus WAL deterministically and compact safely.
- Verify state hash after recovery.

Acceptance criteria:
- Restore reproduces orders, fills, positions, QTY, and scores exactly.
- Interrupted snapshot never replaces last good snapshot.
- Compaction cannot discard uncommitted intent.

Verification:
- Golden recovery tests.
- Kill-at-every-step fault tests.
- Snapshot compatibility tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(engine): add deterministic snapshots and replay`.
- Push and open a PR; merge only after all required checks pass.
```

### ENG-006 — Add command/event sequencing and deduplication

```text
You are implementing Tragge task `ENG-006`: **Add command/event sequencing and deduplication**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/eng-006-add-command-event-sequencing-and-deduplica`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ENG-001,ENG-004

Primary scope:
- apps/trading-engine/**
- packages/contracts/**

Required implementation:
- Require idempotency keys for every user command.
- Track participant/contest aggregate versions and market-data sequence/source epoch.
- Reject stale, duplicate, and impossible-order commands deterministically.
- Publish ordered result events with replay-safe IDs.

Acceptance criteria:
- Duplicate create/cancel/close commands return original outcome.
- Out-of-order config or participant events cannot corrupt sessions.
- Sequence gaps are observable.

Verification:
- Duplicate storm tests.
- Out-of-order event tests.
- Partition restart tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(engine): enforce idempotent sequenced commands`.
- Push and open a PR; merge only after all required checks pass.
```

### ENG-007 — Implement paused-symbol, degraded-feed, and final-quote behavior

```text
You are implementing Tragge task `ENG-007`: **Implement paused-symbol, degraded-feed, and final-quote behavior**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/eng-007-implement-paused-symbol-degraded-feed-and-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ENG-006,MD-006

Primary scope:
- apps/trading-engine/**
- packages/contracts/**

Required implementation:
- Stop fills and trigger evaluation for paused symbols while contest time continues.
- Continue unaffected symbols.
- Use generic stable user errors without provider detail.
- At settlement, isolate missing-final-quote trades and emit review impact metadata.

Acceptance criteria:
- Paused symbols never execute pending, TP, or SL events.
- Resuming with a new source epoch cannot replay stale triggers.
- Affected-rank detection is available to Settlement.

Verification:
- Pause/resume tests.
- Source-epoch tests.
- Final-quote impact tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(engine): handle symbol pauses safely`.
- Push and open a PR; merge only after all required checks pass.
```

### ENG-008 — Implement lazy contest sessions and bounded shard ownership

```text
You are implementing Tragge task `ENG-008`: **Implement lazy contest sessions and bounded shard ownership**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/eng-008-implement-lazy-contest-sessions-and-bounde`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- CON-004,ENG-006

Primary scope:
- apps/trading-engine/**
- packages/infra/**
- packages/contracts/**

Required implementation:
- Create sessions only from qualified start commands.
- Assign one authoritative shard/partition per contest with leases or deterministic partitioning.
- Support late participant activation before cutoff without recreating session.
- Prevent split-brain ownership.

Acceptance criteria:
- No upcoming contest consumes session state.
- One contest has one writer at a time.
- Ownership transfer recovers without lost or duplicated command.

Verification:
- Lease/partition tests.
- Late participant tests.
- Node restart and ownership transfer tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(engine): add lazy sessions and shard ownership`.
- Push and open a PR; merge only after all required checks pass.
```

### ENG-009 — Add Engine fault-injection, performance, and soak suite

```text
You are implementing Tragge task `ENG-009`: **Add Engine fault-injection, performance, and soak suite**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/eng-009-add-engine-fault-injection-performance-and`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ENG-003,ENG-005,ENG-006,ENG-007,ENG-008

Primary scope:
- apps/trading-engine/**
- tests/load/**
- tools/**

Required implementation:
- Test crashes between every persistence step, DB latency, broker duplication, Redis loss, market gaps, disk pressure, and process restart.
- Benchmark at two times capped launch load.
- Run deterministic multi-hour soak with state-hash checks.

Acceptance criteria:
- No orphan order/fill/position state remains.
- Replay hashes match.
- Latency and throughput meet documented launch SLO under target load.

Verification:
- Race tests.
- Fault-injection integration suite.
- Load and soak reports committed as artifacts/config, not raw huge data.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `test(engine): add recovery and load qualification`.
- Push and open a PR; merge only after all required checks pass.
```


## Phase 7 — Market Data

### MD-001 — Create versioned fixed-point market-data contract v2

```text
You are implementing Tragge task `MD-001`: **Create versioned fixed-point market-data contract v2**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/md-001-create-versioned-fixed-point-market-data-c`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- DATA-001,FND-003

Primary scope:
- packages/contracts/**
- apps/market-ingestor/**
- apps/trading-engine/**
- apps/*-frontend/**

Required implementation:
- Add event ID, provider, asset group, fixed-point bid/ask/last, provider/receive/publish times, sequence, source epoch, quality, synthetic flag, and normalization version.
- Provide compatibility translation during rollout.
- Define gap, stale, pause, resume, and source-switch events.

Acceptance criteria:
- Schema is validated in Go and TypeScript.
- Engine can reject stale or incompatible data.
- No silent loss is indistinguishable from valid continuity.

Verification:
- Schema golden tests.
- Backward compatibility tests.
- Serialization benchmarks.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(marketdata): add tick contract v2`.
- Push and open a PR; merge only after all required checks pass.
```

### MD-002 — Build provider capability registry and adapter contract

```text
You are implementing Tragge task `MD-002`: **Build provider capability registry and adapter contract**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/md-002-build-provider-capability-registry-and-ada`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- MD-001

Primary scope:
- apps/market-ingestor/**
- docs/architecture/**
- apps/admin-frontend/**

Required implementation:
- Define adapter capabilities for symbols, bid/ask, timestamps, sequence, candles, market status, rate limits, auth, and commercial enablement.
- Create a registry that marks provider/symbol support verified or unverified.
- Prevent production activation without required coverage and entitlement flags.

Acceptance criteria:
- Candidate providers can be compared without provider-specific branching in core selection.
- Unverified provider coverage fails closed.
- Capability matrix is visible to Admin.

Verification:
- Adapter contract tests.
- Registry validation tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(marketdata): add provider capability registry`.
- Push and open a PR; merge only after all required checks pass.
```

### MD-003 — Implement and validate Crypto provider adapters

```text
You are implementing Tragge task `MD-003`: **Implement and validate Crypto provider adapters**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/md-003-implement-and-validate-crypto-provider-ada`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- MD-002

Primary scope:
- apps/market-ingestor/**
- packages/contracts/**

Required implementation:
- Implement or refactor adapters for approved candidates in priority order based on available credentials and verified rights: Nobitex, Wallex, Coinbase, Binance, Deriv, Tiingo.
- Normalize symbols and timestamps.
- Do not fake unsupported bid/ask without `is_synthetic=true`.
- Add REST fallback only where policy and provider limits permit.

Acceptance criteria:
- Each enabled adapter passes the same conformance suite.
- Coverage gaps are explicit.
- Rate limits and reconnect behavior are bounded.

Verification:
- Provider sandbox/fixture tests.
- Recorded-message replay tests.
- Symbol normalization tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(marketdata): add conformant crypto adapters`.
- Push and open a PR; merge only after all required checks pass.
```

### MD-004 — Implement and validate Forex provider adapters

```text
You are implementing Tragge task `MD-004`: **Implement and validate Forex provider adapters**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/md-004-implement-and-validate-forex-provider-adap`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- MD-002

Primary scope:
- apps/market-ingestor/**
- packages/contracts/**

Required implementation:
- Implement or refactor Deriv and Tiingo adapters after coverage/rights verification.
- Normalize all approved Forex symbols, Bid/Ask, timestamp precision, and connection lifecycle.
- Expose missing coverage to Admin and scheduler launch gates.

Acceptance criteria:
- Enabled provider covers required production symbols or launch profile excludes uncovered symbols.
- No provider-specific timestamps leak into canonical contract.
- Reconnect and rate-limit behavior pass conformance tests.

Verification:
- Recorded feed tests.
- Coverage matrix test.
- Reconnect/failover tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(marketdata): add conformant forex adapters`.
- Push and open a PR; merge only after all required checks pass.
```

### MD-005 — Implement asset-group provider health and automatic selection

```text
You are implementing Tragge task `MD-005`: **Implement asset-group provider health and automatic selection**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/md-005-implement-asset-group-provider-health-and-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- MD-003,MD-004

Primary scope:
- apps/market-ingestor/**
- apps/platform/internal/modules/admin/**

Required implementation:
- Score providers per Crypto and Forex using freshness, stability, sequence integrity, latency, error rate, spread quality, consensus deviation, and coverage.
- Add stickiness and cooldown to avoid flapping.
- Support AUTO and FORCE_PROVIDER.
- Review force after one hour and require ten stable minutes before automatic return.

Acceptance criteria:
- Selection is deterministic from the same observations.
- A single remaining provider enters DEGRADED and continues with Admin alert.
- Provider changes are fully audited.

Verification:
- Health-score tests.
- Flapping simulation.
- Single-provider degradation test.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(marketdata): select providers by asset group health`.
- Push and open a PR; merge only after all required checks pass.
```

### MD-006 — Implement consensus validation and safe source switching

```text
You are implementing Tragge task `MD-006`: **Implement consensus validation and safe source switching**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/md-006-implement-consensus-validation-and-safe-so`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- MD-005,ENG-007

Primary scope:
- apps/market-ingestor/**
- packages/contracts/**

Required implementation:
- Use robust consensus such as median/MAD with configurable symbol thresholds.
- On switch pause affected symbols, validate candidate, increment source epoch, publish resume, and prevent stale event mixing.
- Support Admin PAUSE_SYMBOL.
- Never expose provider identity to user clients.

Acceptance criteria:
- An outlier provider cannot silently become active.
- No cross-epoch tick triggers an Engine order.
- Every switch has measurable pause duration and reason.

Verification:
- Outlier simulations.
- Epoch ordering tests.
- Pause/switch integration test with Engine.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(marketdata): validate and switch sources safely`.
- Push and open a PR; merge only after all required checks pass.
```

### MD-007 — Build canonical candles, raw retention, and Admin diagnostics

```text
You are implementing Tragge task `MD-007`: **Build canonical candles, raw retention, and Admin diagnostics**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/md-007-build-canonical-candles-raw-retention-and-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- MD-001,MD-006,OPS-006

Primary scope:
- apps/market-ingestor/**
- apps/admin-frontend/**
- packages/storage/**

Required implementation:
- Aggregate live candles from Bid ticks.
- Use tagged provider candles only for backfill.
- Retain compressed raw canonical events and switch metadata in external object storage for the dispute window.
- Build Admin views for health, coverage, lag, gaps, force, pause, and audit.

Acceptance criteria:
- Candle OHLC can be reproduced from retained ticks.
- Raw retention does not fill the application SSD.
- Admin actions require permissions and audit.

Verification:
- Candle property tests.
- Object-storage integration test.
- Admin Playwright tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(marketdata): add candles retention and diagnostics`.
- Push and open a PR; merge only after all required checks pass.
```

### MD-008 — Add Market Data replay, gap, failover, and load tests

```text
You are implementing Tragge task `MD-008`: **Add Market Data replay, gap, failover, and load tests**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/md-008-add-market-data-replay-gap-failover-and-lo`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- MD-003,MD-004,MD-005,MD-006,MD-007

Primary scope:
- apps/market-ingestor/**
- tests/load/**
- tools/**

Required implementation:
- Replay recorded provider streams with duplicates, gaps, clock skew, stale periods, disconnects, and outliers.
- Verify selection, source epochs, pause behavior, candle equality, and bounded browser publication.
- Benchmark all launch symbols.

Acceptance criteria:
- No silent gap passes as continuous healthy data.
- Failover behavior is deterministic.
- Throughput meets two-times launch target without unbounded memory.

Verification:
- Replay suite.
- Long disconnect tests.
- Load/soak report.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `test(marketdata): qualify feeds and failover`.
- Push and open a PR; merge only after all required checks pass.
```


## Phase 8 — Payments and KYC

### PAY-001 — Create canonical payment provider interfaces and user gateway selection

```text
You are implementing Tragge task `PAY-001`: **Create canonical payment provider interfaces and user gateway selection**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/pay-001-create-canonical-payment-provider-interfac`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-004,DATA-003

Primary scope:
- apps/platform/internal/modules/payment/**
- packages/contracts/**
- apps/user-frontend/**

Required implementation:
- Define Rial and crypto provider interfaces for quote/create/inquiry/webhook/refund capabilities.
- Let user select an enabled gateway at checkout.
- Keep provider-specific payloads behind adapters.
- Persist provider, request ID, quote, and state-machine version.

Acceptance criteria:
- Adding a provider does not change payment use cases.
- Disabled/unhealthy gateways are unavailable to users.
- Provider selection is auditable.

Verification:
- Adapter contract tests.
- Gateway-selection API/UI tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(payment): add canonical gateway adapters`.
- Push and open a PR; merge only after all required checks pass.
```

### PAY-002 — Implement Jibit and Sepal Rial gateways

```text
You are implementing Tragge task `PAY-002`: **Implement Jibit and Sepal Rial gateways**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/pay-002-implement-jibit-and-sepal-rial-gateways`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PAY-001,PAY-004

Primary scope:
- apps/payment-service/providers/**
- apps/platform/internal/modules/payment/**

Required implementation:
- Implement Jibit and Sepal adapters using official sandbox/production configurations.
- Mark Sepal as the selected Rial test path without hardcoding it as production default.
- Validate callbacks, inquiry, amount/currency, and replay windows.

Acceptance criteria:
- Both providers pass common conformance tests.
- Sandbox and production secrets/config are isolated.
- A forged callback cannot credit balance.

Verification:
- Recorded/sandbox tests.
- Signature and replay tests.
- Inquiry reconciliation tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(payment): add jibit and sepal adapters`.
- Push and open a PR; merge only after all required checks pass.
```

### PAY-003 — Implement Plisio and NOWPayments crypto gateways

```text
You are implementing Tragge task `PAY-003`: **Implement Plisio and NOWPayments crypto gateways**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/pay-003-implement-plisio-and-nowpayments-crypto-ga`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PAY-001

Primary scope:
- apps/payment-service/providers/**
- apps/platform/internal/modules/payment/**

Required implementation:
- Implement Plisio and NOWPayments for USDT TRC20 and TRX where verified supported.
- Do not create or store private keys or internal deposit addresses.
- Validate payment amount, asset, network, confirmation state, and webhook signature.
- Credit only final net confirmed amount.

Acceptance criteria:
- Both adapters pass common conformance tests.
- Network/asset mismatch is rejected.
- Duplicate confirmation cannot duplicate credit.

Verification:
- Sandbox/fixture tests.
- Webhook replay tests.
- Under/overpayment state tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(payment): add crypto gateway adapters`.
- Push and open a PR; merge only after all required checks pass.
```

### PAY-004 — Implement immutable Nobitex USDT/IRR payment quotes

```text
You are implementing Tragge task `PAY-004`: **Implement immutable Nobitex USDT/IRR payment quotes**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/pay-004-implement-immutable-nobitex-usdt-irr-payme`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- DATA-001

Primary scope:
- apps/platform/internal/modules/payment/**
- packages/wallet/exchangerate/**
- packages/db/migrations/**

Required implementation:
- Fetch the Nobitex USDTIRT quote at payment-request creation.
- Store raw response hash/payload reference, rial rate, toman display rate, source timestamp, normalized timestamp, and expiry.
- Use a configurable short expiry and fixed-point conversion.
- Never silently refresh an existing request quote.

Acceptance criteria:
- A payment request always references one immutable quote.
- Expired quotes cannot initiate payment.
- Conversion and rounding are deterministic.

Verification:
- Recorded API fixture tests.
- Expiry/boundary tests.
- Precision property tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(payment): lock nobitex fiat quotes`.
- Push and open a PR; merge only after all required checks pass.
```

### PAY-005 — Implement deposit limits, webhook security, and net ledger credit

```text
You are implementing Tragge task `PAY-005`: **Implement deposit limits, webhook security, and net ledger credit**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/pay-005-implement-deposit-limits-webhook-security-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PAY-002,PAY-003,PAY-004,DATA-004

Primary scope:
- apps/platform/internal/modules/payment/**
- apps/platform/internal/modules/wallet/**

Required implementation:
- Enforce 4-USDT-equivalent minimum and 1000-USDT-equivalent maximum.
- Use durable payment state machine and provider payment-ID uniqueness.
- Verify signatures, timestamps, amount, currency, network, and state monotonicity.
- Post exact net confirmed amount from clearing to user available.

Acceptance criteria:
- No pending or unverified payment credits a wallet.
- Duplicate/out-of-order webhook is harmless.
- Deposit history reconciles provider and ledger state.

Verification:
- Provider webhook integration tests.
- Limit tests.
- Crash/retry tests.
- Ledger assertions.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(payment): secure and credit deposits`.
- Push and open a PR; merge only after all required checks pass.
```

### PAY-006 — Implement manual USDT TRC20 withdrawal workflow

```text
You are implementing Tragge task `PAY-006`: **Implement manual USDT TRC20 withdrawal workflow**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/pay-006-implement-manual-usdt-trc20-withdrawal-wor`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- DATA-003,SEC-004,PAY-007

Primary scope:
- apps/platform/internal/modules/payment/**
- apps/admin-frontend/**
- apps/user-frontend/**
- packages/db/migrations/**

Required implementation:
- Require KYC and 10-USDT minimum.
- Reserve requested amount in Withdrawal Pending.
- Allow only Super Admin to complete with transaction hash and password re-entry.
- Support REJECT_AND_RELEASE and Super-Admin-only REJECT_AND_DEDUCT with mandatory reason.
- Record user-paid fee metadata without unsafe balance edits.

Acceptance criteria:
- Every state transition has balanced ledger postings and audit.
- A rejected release returns exact amount.
- A completed withdrawal cannot be reopened or completed twice.

Verification:
- State-machine tests.
- Permission/reauth tests.
- Concurrent action tests.
- Frontend E2E.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(withdrawal): add audited manual trc20 flow`.
- Push and open a PR; merge only after all required checks pass.
```

### PAY-007 — Implement extensible manual KYC review

```text
You are implementing Tragge task `PAY-007`: **Implement extensible manual KYC review**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/pay-007-implement-extensible-manual-kyc-review`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-004,SEC-004,OPS-006

Primary scope:
- packages/kyc/**
- apps/platform/internal/modules/kyc/**
- apps/user-frontend/**
- apps/admin-frontend/**

Required implementation:
- Store extensible identity document metadata and encrypted object-storage references.
- Support submit, needs-review, approved, rejected, resubmit states.
- Allow Support Admin and Super Admin review by permission.
- Never expose raw documents through public URLs; use short-lived signed access.

Acceptance criteria:
- Withdrawal checks authoritative approved status.
- Document access is audited.
- Rejection/resubmission history is immutable.

Verification:
- Permission tests.
- Signed URL expiry tests.
- KYC state-machine E2E.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(kyc): add manual review workflow`.
- Push and open a PR; merge only after all required checks pass.
```

### PAY-008 — Build payment and withdrawal reconciliation operations

```text
You are implementing Tragge task `PAY-008`: **Build payment and withdrawal reconciliation operations**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/pay-008-build-payment-and-withdrawal-reconciliatio`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- PAY-005,PAY-006

Primary scope:
- apps/platform/internal/modules/payment/**
- docs/runbooks/**
- apps/admin-frontend/**

Required implementation:
- Create scheduled reconciliation for gateway payments, clearing accounts, deposits, withdrawals, and orphan provider records.
- Expose actionable mismatches without auto-editing money.
- Provide idempotent repair commands through compensating ledger entries.

Acceptance criteria:
- Daily reconciliation reports zero or explicit explained exceptions.
- Repairs require Super Admin, reason, and audit.
- Provider outages do not lose pending work.

Verification:
- Mismatch fixtures.
- Provider outage/recovery tests.
- Admin reconciliation E2E.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(finance): add gateway reconciliation`.
- Push and open a PR; merge only after all required checks pass.
```


## Phase 9 — Frontends

### FE-001 — Create trade frontend and shared frontend packages

```text
You are implementing Tragge task `FE-001`: **Create trade frontend and shared frontend packages**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fe-001-create-trade-frontend-and-shared-frontend-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FND-002,MD-001,ENG-001

Primary scope:
- apps/trade-frontend/**
- packages/frontend-core/**
- packages/trading-contracts/**
- packages/design-system/**
- pnpm-workspace.yaml

Required implementation:
- Scaffold independent Vue trade application.
- Move only stable auth client, API client, i18n primitives, logging, error handling, and design tokens to shared packages.
- Generate trading REST/WS types from versioned contracts.
- Add mock REST and WebSocket scenarios.

Acceptance criteria:
- Trade frontend builds without importing user-frontend source aliases.
- Shared packages have explicit public APIs.
- Mock scenarios run without backend.

Verification:
- Typecheck, lint, unit build.
- Import-boundary test.
- Mock smoke test.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(frontend): scaffold independent trade panel`.
- Push and open a PR; merge only after all required checks pass.
```

### FE-002 — Extract the existing trading module into trade frontend

```text
You are implementing Tragge task `FE-002`: **Extract the existing trading module into trade frontend**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fe-002-extract-the-existing-trading-module-into-t`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FE-001

Primary scope:
- apps/user-frontend/src/modules/trade/**
- apps/trade-frontend/**
- packages/frontend-core/**

Required implementation:
- Move trading pages/components/composables in small behavior-preserving steps.
- Replace root aliases to user auth, API, notification, i18n, types, and utilities with shared package contracts.
- Delete forwarding stubs after cutover.
- Split oversized files along state and view responsibilities.

Acceptance criteria:
- Trade application runs independently against mocks.
- User frontend no longer compiles trading implementation code.
- No circular workspace dependency exists.

Verification:
- Component tests before/after extraction.
- Build both apps.
- Route smoke tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `refactor(frontend): extract trade application`.
- Push and open a PR; merge only after all required checks pass.
```

### FE-003 — Implement resilient WebSocket state machine and resume

```text
You are implementing Tragge task `FE-003`: **Implement resilient WebSocket state machine and resume**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fe-003-implement-resilient-websocket-state-machin`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FE-001,ENG-006,MD-001

Primary scope:
- apps/trade-frontend/**
- packages/trading-contracts/**

Required implementation:
- Implement connecting, live, reconnecting, resuming, degraded transport, and closed states.
- Resume from sequence numbers.
- Deduplicate and reorder bounded event windows.
- Reconcile authoritative REST snapshot after reconnect.
- Never retry an order by replaying UI intent without idempotency key.

Acceptance criteria:
- Reconnect cannot duplicate an order or fill.
- Out-of-order PnL/position events converge to authoritative state.
- Refresh restores complete contest state.

Verification:
- Mock WS scenario tests.
- Browser offline/online Playwright tests.
- Duplicate/out-of-order tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(trade-ui): add resumable websocket state`.
- Push and open a PR; merge only after all required checks pass.
```

### FE-004 — Rebuild order, position, and QTY interactions

```text
You are implementing Tragge task `FE-004`: **Rebuild order, position, and QTY interactions**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fe-004-rebuild-order-position-and-qty-interaction`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FE-002,FE-003,ENG-003

Primary scope:
- apps/trade-frontend/**

Required implementation:
- Use explicit order-command idempotency keys and pending UI states.
- Display max, available, and reserved QTY.
- Support allowed order types with validation mirrored only for UX; backend remains authoritative.
- Reconcile rejects/fills/cancels and disable affected paused symbols generically.

Acceptance criteria:
- UI cannot submit decimal or excess QTY.
- Optimistic state always resolves from Engine result.
- Double-click and retry cannot create duplicate orders.

Verification:
- Component command tests.
- Playwright order lifecycle.
- Network retry tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(trade-ui): implement authoritative order workflow`.
- Push and open a PR; merge only after all required checks pass.
```

### FE-005 — Rebuild chart and leaderboard for canonical data

```text
You are implementing Tragge task `FE-005`: **Rebuild chart and leaderboard for canonical data**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fe-005-rebuild-chart-and-leaderboard-for-canonica`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FE-002,FE-003,PRIZE-004,MD-007

Primary scope:
- apps/trade-frontend/**

Required implementation:
- Render Bid candles, all enabled contest symbols, positions, and trade markers.
- Use bounded subscriptions and coalesced browser updates.
- Render practice rank zero correctly.
- Render live ranking separately from locked/final prize table and exclude no-trade users from prize view.

Acceptance criteria:
- Chart data provenance matches canonical Bid candles.
- Browser does not receive/process every raw tick unnecessarily.
- Leaderboard handles ties, bands, reconnect, and system rank zero.

Verification:
- Visual/component tests.
- Large-list performance test.
- Playwright reconnect test.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(trade-ui): rebuild chart and leaderboard`.
- Push and open a PR; merge only after all required checks pass.
```

### FE-006 — Update user contest discovery and atomic join checkout

```text
You are implementing Tragge task `FE-006`: **Update user contest discovery and atomic join checkout**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fe-006-update-user-contest-discovery-and-atomic-j`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- CON-002,SCH-007,PRIZE-004

Primary scope:
- apps/user-frontend/**
- packages/contracts/ts/**

Required implementation:
- Show schedule families and browser-local times.
- Before join fetch a signed/expiring quote showing base fee, late surcharge when applicable, total, estimated Prize Pool, and wallet balance.
- Do not show late cutoff timer inside trade panel.
- Represent free practice separately from paid prize contests.

Acceptance criteria:
- Displayed charge equals posted charge.
- Join at cutoff handles server rejection safely.
- Contest lists match scheduler windows and do not invent missing Forex records.

Verification:
- Component tests.
- Playwright on-time/late/free join flows.
- Timezone tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(user-ui): update contest discovery and checkout`.
- Push and open a PR; merge only after all required checks pass.
```

### FE-007 — Build Admin operations for providers, finance, settlement, and KYC

```text
You are implementing Tragge task `FE-007`: **Build Admin operations for providers, finance, settlement, and KYC**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fe-007-build-admin-operations-for-providers-finan`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- MD-007,PAY-006,PAY-007,PAY-008,PRIZE-008

Primary scope:
- apps/admin-frontend/**

Required implementation:
- Implement permission-aware dashboards for provider health/force/pause, deposit/withdrawal, KYC review, reconciliation, settlement reconstruction, and audit.
- Require reauthentication where specified.
- Use paginated/virtualized tables and safe document access.

Acceptance criteria:
- Support Admin cannot perform Super Admin actions.
- Every mutation displays server audit/result ID.
- No sensitive provider secret or full KYC object URL is exposed.

Verification:
- Permission component tests.
- Playwright critical admin flows.
- Accessibility checks.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(admin-ui): add production operations consoles`.
- Push and open a PR; merge only after all required checks pass.
```

### FE-008 — Complete Persian/English RTL/LTR and accessibility

```text
You are implementing Tragge task `FE-008`: **Complete Persian/English RTL/LTR and accessibility**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fe-008-complete-persian-english-rtl-ltr-and-acces`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FE-002,FE-006,FE-007

Primary scope:
- apps/user-frontend/**
- apps/trade-frontend/**
- apps/admin-frontend/**
- packages/design-system/**

Required implementation:
- Centralize translations, remove giant untyped locale files where practical, and add missing-key checks.
- Support RTL/LTR layouts, numeric direction, date/time, money, tables, charts, dialogs, and validation.
- Meet WCAG-oriented keyboard, focus, contrast, and screen-reader basics.

Acceptance criteria:
- All launch routes render in both languages without missing keys.
- Direction changes do not require reload or break charts.
- Critical flows are keyboard usable.

Verification:
- Translation key parity test.
- Visual snapshots RTL/LTR.
- Automated accessibility tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(i18n): complete bilingual rtl ltr support`.
- Push and open a PR; merge only after all required checks pass.
```

### FE-009 — Add frontend contract, E2E, and performance gates

```text
You are implementing Tragge task `FE-009`: **Add frontend contract, E2E, and performance gates**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/fe-009-add-frontend-contract-e2e-and-performance-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FE-003,FE-004,FE-005,FE-006,FE-007,FE-008,SEC-007

Primary scope:
- apps/*-frontend/**
- .github/workflows/**
- tests/**

Required implementation:
- Run Vitest and Playwright in CI.
- Cover registration/OTP, login, contest join, trade reconnect/order, deposit, KYC, withdrawal, Super Admin MFA after `SEC-007`, provider pause, settlement review, and refund.
- Set bundle and runtime performance budgets.
- Use stable test data and mock/provider sandboxes.

Acceptance criteria:
- Critical E2E suite is deterministic.
- Bundle regressions fail CI.
- Mobile and desktop viewports pass.

Verification:
- Vitest, Playwright, typecheck, lint, bundle analysis.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `test(frontend): add production e2e and performance gates`.
- Push and open a PR; merge only after all required checks pass.
```


## Phase 10 — Production Engineering

### OPS-001 — Pin reproducible toolchains and fix CI build targets

```text
You are implementing Tragge task `OPS-001`: **Pin reproducible toolchains and fix CI build targets**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/ops-001-pin-reproducible-toolchains-and-fix-ci-bui`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FND-001,ARCH-007

Primary scope:
- .github/workflows/**
- go.work
- package.json
- pnpm-lock.yaml
- Makefile
- .tool-versions

Required implementation:
- Pin exact supported Go, pnpm, Node, golangci-lint, migration, and security tool versions.
- Stop installing golangci-lint from HEAD.
- Build target runtimes instead of deleted wrappers.
- Cache safely and upload test reports.

Acceptance criteria:
- A clean runner produces repeatable builds.
- CI does not depend on floating tool versions.
- All target applications are built.

Verification:
- Run CI-equivalent scripts locally/containerized where available.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `ci: pin toolchains and target production runtimes`.
- Push and open a PR; merge only after all required checks pass.
```

### OPS-002 — Add PostgreSQL, Redis, broker, migration, and contract integration CI

```text
You are implementing Tragge task `OPS-002`: **Add PostgreSQL, Redis, broker, migration, and contract integration CI**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/ops-002-add-postgresql-redis-broker-migration-and-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-006,FND-004,OPS-001

Primary scope:
- .github/workflows/**
- tests/integration/**
- packages/db/**
- packages/contracts/**

Required implementation:
- Start ephemeral dependencies.
- Test fresh install, migration upgrade fixture, schema permissions, outbox/inbox, ledger, settlement, scheduler, Engine, and contract compatibility.
- Validate generated Go/TS schemas are in sync.

Acceptance criteria:
- Integration failures block merge.
- Fresh database setup is exercised on every schema change.
- Contract breaking changes require explicit version.

Verification:
- CI integration suite and migration checks.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `ci: add backend integration and contract gates`.
- Push and open a PR; merge only after all required checks pass.
```

### OPS-003 — Build minimal hardened production images and Docker Compose

```text
You are implementing Tragge task `OPS-003`: **Build minimal hardened production images and Docker Compose**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/ops-003-build-minimal-hardened-production-images-a`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-007,ENG-004,OPS-001

Primary scope:
- Dockerfile*
- infra/docker/**
- docker-compose*.yml
- apps/gateway/**

Required implementation:
- Create separate images for Platform, Engine, Market Data, user frontend, trade frontend, admin frontend, and gateway.
- Use non-root, read-only root where compatible, health checks, resource limits, persistent volumes, and explicit networks.
- Add durable Engine WAL/snapshot storage and external object-storage config.
- Do not include Kubernetes in launch path.

Acceptance criteria:
- Compose config starts from an empty server with one command.
- Each backend restarts independently.
- Secrets are injected, not baked into images.

Verification:
- Image build.
- Compose config validation.
- Container smoke and restart tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `build: add hardened production compose stack`.
- Push and open a PR; merge only after all required checks pass.
```

### OPS-004 — Implement temporary staging and release pipeline

```text
You are implementing Tragge task `OPS-004`: **Implement temporary staging and release pipeline**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/ops-004-implement-temporary-staging-and-release-pi`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- OPS-002,OPS-003,FE-009

Primary scope:
- .github/workflows/**
- infra/scripts/**
- docs/runbooks/**

Required implementation:
- Deploy a temporary staging stack with separate databases/ports, run migrations, smoke, E2E, and selected load checks, then tear it down.
- Promote immutable image digests to production with manual approval.
- Provide database preflight, backup, deploy, health, canary, and rollback steps.

Acceptance criteria:
- Production never builds from source in place.
- Failed staging or smoke blocks promotion.
- Rollback restores prior images and compatible schema path.

Verification:
- Pipeline dry run.
- Rollback rehearsal.
- Migration failure simulation.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `ci(release): add temporary staging and promotion`.
- Push and open a PR; merge only after all required checks pass.
```

### OPS-005 — Add end-to-end observability and SLO dashboards

```text
You are implementing Tragge task `OPS-005`: **Add end-to-end observability and SLO dashboards**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/ops-005-add-end-to-end-observability-and-slo-dashb`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- ARCH-006,OPS-003

Primary scope:
- packages/observability/**
- apps/**
- infra/observability/**
- docs/runbooks/**

Required implementation:
- Add structured logs, metrics, traces, correlation/causation IDs, and dashboards for Platform, Engine, Market Data, payments, settlement, scheduler, WebSockets, DB, Redis, broker, disk, and traffic.
- Define initial SLOs and actionable alerts.
- Redact sensitive data.

Acceptance criteria:
- An order and settlement can be traced across systems.
- Alerts include owner, severity, and runbook.
- Telemetry overhead stays within budget.

Verification:
- Telemetry integration tests.
- Alert rule tests.
- Redaction tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(observability): add production telemetry and slos`.
- Push and open a PR; merge only after all required checks pass.
```

### OPS-006 — Implement backup, PITR, object storage, and restore drills

```text
You are implementing Tragge task `OPS-006`: **Implement backup, PITR, object storage, and restore drills**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/ops-006-implement-backup-pitr-object-storage-and-r`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- OPS-003,ENG-005,MD-007,PAY-007

Primary scope:
- infra/backup/**
- packages/storage/**
- docs/runbooks/**

Required implementation:
- Enable PostgreSQL PITR, encrypted scheduled backups, object-storage lifecycle, Engine snapshot upload, KYC document storage, and market-data retention.
- Keep local disk retention bounded.
- Automate restore into isolated environment and verify checksums/invariants.

Acceptance criteria:
- A documented restore meets RPO/RTO targets.
- Backup success without restore verification is not accepted.
- Secrets and KYC objects are encrypted and access-controlled.

Verification:
- Automated restore test.
- Point-in-time recovery rehearsal.
- Object lifecycle test.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(ops): add verified backups and restore`.
- Push and open a PR; merge only after all required checks pass.
```

### OPS-007 — Add SAST, dependency, secret, image, and SBOM gates

```text
You are implementing Tragge task `OPS-007`: **Add SAST, dependency, secret, image, and SBOM gates**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/ops-007-add-sast-dependency-secret-image-and-sbom-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- OPS-001,OPS-003

Primary scope:
- .github/workflows/**
- .github/dependabot.yml or equivalent
- docs/security/**

Required implementation:
- Run secret scanning, Go/Node dependency audit, SAST, license review, container scan, and SBOM generation.
- Pin scanner versions and define severity policy.
- Block new critical/high exploitable findings unless documented exception expires.

Acceptance criteria:
- Every released image has an SBOM and scan result.
- Leaked test secrets are prevented.
- Exceptions have owner and expiry.

Verification:
- Seeded vulnerable fixture check.
- CI policy tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `ci(security): add supply chain gates`.
- Push and open a PR; merge only after all required checks pass.
```

### OPS-008 — Optimize WebSocket and traffic usage for the initial server

```text
You are implementing Tragge task `OPS-008`: **Optimize WebSocket and traffic usage for the initial server**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/ops-008-optimize-websocket-and-traffic-usage-for-t`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- FE-003,MD-008,OPS-005

Primary scope:
- apps/platform/internal/modules/realtime/**
- apps/trade-frontend/**
- apps/market-ingestor/**
- infra/**

Required implementation:
- Publish only contest asset-group symbols.
- Prioritize visible/held symbols, coalesce tick updates, compress WS where safe, use sequence deltas, and cap per-client queues.
- Disconnect or resync slow consumers instead of unbounded buffering.
- Measure monthly traffic.

Acceptance criteria:
- Memory is bounded per connection.
- Slow clients cannot delay Engine or Market Data.
- Traffic projection and alerts fit the upgradeable server plan.

Verification:
- WS load test.
- Slow-consumer test.
- Compression/CPU benchmark.
- Traffic report.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `perf(realtime): bound websocket traffic and queues`.
- Push and open a PR; merge only after all required checks pass.
```

### OPS-009 — Create operational runbooks and kill switches

```text
You are implementing Tragge task `OPS-009`: **Create operational runbooks and kill switches**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/ops-009-create-operational-runbooks-and-kill-switc`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- OPS-005,OPS-006

Primary scope:
- docs/runbooks/**
- apps/platform/internal/modules/admin/**
- apps/admin-frontend/**

Required implementation:
- Document and implement kill switches for trading, symbol, contest generation, deposits, and withdrawals.
- Add incident severity, on-call, provider outage, Engine recovery, settlement review, ledger mismatch, disk pressure, traffic exhaustion, backup restore, and key rotation runbooks.
- All switches require audit and safe resume.

Acceptance criteria:
- Operator can contain each critical failure without database editing.
- Runbooks include verification and rollback.
- Switch state is visible and persistent.

Verification:
- Game-day exercises.
- Permission/audit tests.
- Safe resume tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `docs(ops): add incident runbooks and kill switches`.
- Push and open a PR; merge only after all required checks pass.
```

### OPS-010 — Harden single-server production resources and capacity guards

```text
You are implementing Tragge task `OPS-010`: **Harden single-server production resources and capacity guards**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/ops-010-harden-single-server-production-resources-`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- OPS-003,OPS-005,OPS-008

Primary scope:
- infra/docker/**
- apps/**
- docs/architecture/**

Required implementation:
- Define CPU/memory/file-descriptor/disk limits for 8 cores, 16 GB RAM, 100 GB SSD.
- Add global safety limits for active Engine participants, concurrent contests, WebSockets, queues, and storage.
- Alert before traffic/disk exhaustion.
- Keep at least 20 GB operational free-space target.

Acceptance criteria:
- Resource exhaustion degrades safely instead of crashing all runtimes.
- Circuit breakers are configurable and admin-visible.
- Capacity assumptions are documented and load-tested.

Verification:
- Resource-pressure tests.
- Disk-full simulation.
- Connection-limit tests.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `feat(ops): add single server capacity guards`.
- Push and open a PR; merge only after all required checks pass.
```


## Phase 11 — Launch Qualification

### REL-001 — Run full functional and financial regression qualification

```text
You are implementing Tragge task `REL-001`: **Run full functional and financial regression qualification**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/rel-001-run-full-functional-and-financial-regressi`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- All implementation epics

Primary scope:
- tests/**
- docs/release/**

Required implementation:
- Run complete Go race/unit/integration suites, frontend unit/E2E, migration, contracts, scheduler golden fixtures, prize fixtures, payment sandboxes, and reconciliation.
- Record exact image digests and configuration versions.

Acceptance criteria:
- Zero failing required tests.
- Zero unexplained ledger or Prize Pool difference.
- All P0/P1 issues closed.

Verification:
- Full documented release test matrix.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `test(release): complete production regression`.
- Push and open a PR; merge only after all required checks pass.
```

### REL-002 — Run crash, replay, backup, and provider-failover drills

```text
You are implementing Tragge task `REL-002`: **Run crash, replay, backup, and provider-failover drills**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/rel-002-run-crash-replay-backup-and-provider-failo`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- REL-001,ENG-009,MD-008,OPS-006

Primary scope:
- docs/release/**
- docs/runbooks/**

Required implementation:
- Kill Engine and workers at critical points, replay broker events, restore database to point in time, restore Engine snapshots, switch providers, pause symbols, and complete settlement review.
- Capture evidence and remediation.

Acceptance criteria:
- RPO/RTO targets are met.
- No duplicate fill, payout, or deposit occurs.
- All drills have signed results.

Verification:
- Game-day scripts and reports.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `test(resilience): run production recovery drills`.
- Push and open a PR; merge only after all required checks pass.
```

### REL-003 — Run load, soak, and traffic qualification

```text
You are implementing Tragge task `REL-003`: **Run load, soak, and traffic qualification**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/rel-003-run-load-soak-and-traffic-qualification`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- REL-001,OPS-008,OPS-010

Primary scope:
- tests/load/**
- docs/release/**

Required implementation:
- Test at least two times the capped launch target for users, WebSockets, tick rate, orders, contests, settlement, and payment callbacks.
- Run a seven-day soak or an equivalent accelerated plus real-time soak agreed in release plan.
- Measure server traffic, disk, and queue behavior.

Acceptance criteria:
- SLOs hold at target.
- No unbounded growth or unreconciled state.
- Capacity caps and scale triggers are documented.

Verification:
- Load/soak reports with reproducible configs.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `test(performance): qualify launch capacity`.
- Push and open a PR; merge only after all required checks pass.
```

### REL-004 — Complete security, legal, and provider-rights launch gates

```text
You are implementing Tragge task `REL-004`: **Complete security, legal, and provider-rights launch gates**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/rel-004-complete-security-legal-and-provider-right`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- REL-001,OPS-007

Primary scope:
- docs/security/**
- docs/release/**

Required implementation:
- Complete penetration testing and remediation.
- Confirm jurisdiction, contest/payment legality, KYC/AML/sanctions obligations, privacy terms, payment contracts, and market-data display/redistribution rights.
- Record launch-blocking approvals without embedding secrets.

Acceptance criteria:
- No unaccepted critical/high security issue.
- All provider rights and legal approvals are documented by owner.
- Missing approval blocks paid launch.

Verification:
- Security retest and approval checklist.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `docs(release): record security and legal gates`.
- Push and open a PR; merge only after all required checks pass.
```

### REL-005 — Launch internal simulation

```text
You are implementing Tragge task `REL-005`: **Launch internal simulation**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/rel-005-launch-internal-simulation`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- REL-002,REL-003,REL-004

Primary scope:
- docs/release/**
- infra/**

Required implementation:
- Run team-only contests with real feeds and no real deposits/withdrawals.
- Exercise every contest family, late entry, cancellation, free practice, provider switch, restart, and settlement.
- Review daily reconciliation.

Acceptance criteria:
- At least one complete cycle of every enabled contest family passes.
- No unresolved financial or ranking defect.
- Incident process is exercised.

Verification:
- Internal launch checklist and postmortem.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `chore(release): complete internal simulation`.
- Push and open a PR; merge only after all required checks pass.
```

### REL-006 — Launch capped free practice

```text
You are implementing Tragge task `REL-006`: **Launch capped free practice**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/rel-006-launch-capped-free-practice`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- REL-005

Primary scope:
- docs/release/**
- infra/**

Required implementation:
- Open free practice to a limited external cohort.
- Monitor signup/OTP, WebSockets, trading UX, system rank zero, provider stability, support tickets, and resource usage.
- Keep deposits/paid join disabled.

Acceptance criteria:
- Free cohort completes defined soak period without P0/P1.
- Support and monitoring are operational.
- Capacity projections are updated.

Verification:
- Free-launch scorecard.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `chore(release): open capped free practice`.
- Push and open a PR; merge only after all required checks pass.
```

### REL-007 — Launch invite-only paid contests

```text
You are implementing Tragge task `REL-007`: **Launch invite-only paid contests**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/rel-007-launch-invite-only-paid-contests`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- REL-006

Primary scope:
- docs/release/**
- infra/**

Required implementation:
- Enable deposits and selected low-value paid templates for invited users.
- Keep weekly high-fee templates disabled.
- Require daily manual reconciliation review and on-call coverage.
- Cap participants, concurrent contests, deposits, and withdrawals through operational guards.

Acceptance criteria:
- Multiple paid settlement cycles complete with zero unexplained difference.
- Withdrawal workflow and support are proven.
- No unresolved P0/P1.

Verification:
- Invite-paid launch scorecard and go/no-go review.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `chore(release): open invite only paid contests`.
- Push and open a PR; merge only after all required checks pass.
```

### REL-008 — Launch capped public paid service

```text
You are implementing Tragge task `REL-008`: **Launch capped public paid service**.

Repository policy:
- Read `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`.
- Read `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md` only for this task and its dependencies.
- Work on branch `codex/rel-008-launch-capped-public-paid-service`.
- Keep one goal, one scoped change set, and one Conventional Commit.
- Do not implement later roadmap tasks.
- Do not add a dependency without a written rationale and a simpler-alternative review.

Dependencies:
- REL-007

Primary scope:
- docs/release/**
- infra/**

Required implementation:
- Open public registration with conservative launch profile and documented caps.
- Use canary rollout and automatic health rollback.
- Increase templates and limits only after clean operating periods and explicit review.

Acceptance criteria:
- Public launch gates remain green.
- Rollback and kill switches are staffed.
- Expansion criteria are quantitative.

Verification:
- Public launch checklist and operational review.

Delivery:
- Update relevant technical Markdown.
- Add targeted tests and run them.
- Run lint/typecheck/build for touched modules.
- Provide an implementation summary and unresolved risks.
- Commit as: `chore(release): open capped public paid service`.
- Push and open a PR; merge only after all required checks pass.
```

---

## 8. Epic-End Full-Suite Requirements

At the end of every phase/epic, do not rely only on task-level tests.

Required checks include, as applicable:

```text
go test -race -count=1 ./...
frontend lint
frontend typecheck
frontend unit tests
Playwright critical paths
fresh database migration
schema permission tests
integration dependencies
contract compatibility
image build
Compose config validation
security scans
reconciliation command
```

The exact commands must be pinned in repository scripts and CI rather than
copied informally between tasks.

---

## 9. Release Metrics and Initial SLO Targets

Targets are launch hypotheses and must be validated by load tests:

| Metric | Initial target |
|---|---:|
| Platform API availability | 99.9% |
| Trading Engine availability | 99.95% |
| Order processing p95 | < 150 ms |
| Order processing p99 | < 400 ms |
| Canonical tick to Engine p95 | < 150 ms |
| Canonical tick to browser p95 | < 300 ms |
| Money/order RPO | 0 |
| Financial/Engine RTO | <= 30 minutes |
| Unexplained reconciliation difference | 0 |
| Duplicate payout | 0 |
| Duplicate confirmed deposit credit | 0 |

No SLO is considered achieved until measured in staging and launch-profile load
tests.

---

## 10. Risk Register

| Risk | Severity | Required control |
|---|---|---|
| Official Forex holidays not modeled in MVP | High | Document accepted risk, avoid high-risk intervals, add provider calendar post-launch |
| Provider coverage/rights unverified | Critical | Capability and contract gate before enabling symbol/provider |
| Single-server failure domain | High | External backups, PITR, snapshots, restore drills, rapid replacement runbook |
| 100 GB application disk | High | External raw data/KYC/backups, retention, disk alerts, 20 GB free target |
| Traffic growth from WebSockets | High | Coalescing, bounded queues, compression benchmark, upgrade alerts |
| Manual withdrawal error/fraud | Critical | KYC, sensitive-action password reauthentication, Super Admin-only permission, ledger reserve, transaction hash, audit, and MFA before paid launch |
| Prize formula divergence | Critical | One package, golden fixtures, preview/settlement parity |
| Duplicate settlement | Critical | One owner, unique settlement aggregate, idempotent ledger |
| Engine crash inconsistency | Critical | Durable WAL, snapshots, deterministic replay, kill tests |
| Market source discontinuity | Critical | Consensus, source epoch, symbol pause, raw retention |
| Codex scope creep | Medium | One task/branch/commit, scoped paths, epic gates |

---

## 11. Final Paid-Launch Checklist

A paid launch decision is **NO-GO** if any item is false:

- [ ] Target runtime architecture deployed.
- [ ] User/Admin auth isolation proven.
- [ ] Sensitive-action password reauthentication and privileged-action enforcement enabled.
- [ ] Super Admin MFA from `SEC-007` enabled and validated.
- [ ] OTP/token/secret log scan clean.
- [ ] One fee model and one prize model.
- [ ] Economics lock occurs at late-entry cutoff.
- [ ] Settlement is sole finalization owner.
- [ ] Double-entry ledger reconciles exactly.
- [ ] `tralent_v1` golden fixtures pass.
- [ ] No-trade and system users excluded from prizes.
- [ ] Engine WAL/snapshot/replay drills pass.
- [ ] Market Data source-switch drills pass.
- [ ] All scheduler golden fixtures pass.
- [ ] Deposit webhook replay tests pass.
- [ ] Manual withdrawal E2E and audit pass.
- [ ] Backup/PITR restore drill passes.
- [ ] Seven-day soak requirement passes.
- [ ] Two-times launch load passes.
- [ ] Security review and penetration retest pass.
- [ ] Market-data and payment-provider rights approved.
- [ ] Legal/jurisdiction launch approval recorded.
- [ ] On-call, runbooks, and kill switches staffed.
