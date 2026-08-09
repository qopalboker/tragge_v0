# Phase 1 failed-gate remediation plan

**Plan date:** 2026-08-09

**Execution mode:** Git-backed, planning only

**Repository:** `qopalboker/tragge_v0`

**Current phase decision:** `PHASE 1 FAIL`

**Paid-production status:** `NO-GO`

## 1. Failed-gate identity and immutable evidence

The failed Phase 1 gate evaluated main commit
`54d9eaefcd0aa1f954c768ea94a4b048a47937ab`. Remote inspection on the plan date
confirmed that `origin/main` had not advanced beyond that commit.

The immutable failed-gate delivery evidence is:

- branch `codex/phase-1-exit-gate`;
- report-bearing head `4985f90a309a0da073453565969d60e72ffd1915`;
- draft, open, unmerged [pull request #3](https://github.com/qopalboker/tragge_v0/pull/3);
- base `main` at `54d9eaefcd0aa1f954c768ea94a4b048a47937ab`;
- GitHub Actions run `31290269549`, completed `success` for the report-bearing
  head; and
- exact decision `PHASE 1 FAIL`.

The successful report CI proves delivery of the failure evidence; it does not
change the security gate result. Pull request #3 remains draft and unmerged and
must not be rewritten, forced ready, or merged merely to unblock remediation.

This plan follows the [Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md),
the [fixed product policy](../../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md),
the [production roadmap](../PRODUCTION_ROADMAP_AND_CODEX_TASKS.md), the
[Phase 1 controller](../prompts/02_PHASE_1_SECURITY.md), and the
[failed-gate remediation controller](../prompts/13_FAILED_GATE_REMEDIATION.md).
[ADR-0001](../../adr/0001-target-runtime-architecture.md) remains Accepted, and
the [canonical glossary/version catalog](../../product/canonical-domain-glossary-and-version-catalog.md)
remains authoritative. SEC-001 through SEC-007 reports remain the prerequisite
history; current source and executable evidence, not report claims alone, must
prove remediation.

## 2. Blocker A: User authentication Playwright E2E

### Exact evidence and root cause

The gate command was:

```text
pnpm exec playwright test apps/user-frontend/e2e/auth.spec.ts --project=user-chromium
```

It exited `1` before collection and produced zero tests. A planning-only
collection rerun using the same file and project plus `--list` reproduced:

```text
SyntaxError: The requested module './pages' does not provide an export named 'DashboardPage'
SyntaxError: Named export 'TEST_USERS' not found. The requested module '../../../e2e/test-data' is a CommonJS module
Total: 0 tests in 0 files
```

Current repository inspection establishes these distinct defects:

1. `apps/user-frontend/e2e/pages/DashboardPage.ts` exists and exports
   `DashboardPage`, but `apps/user-frontend/e2e/pages/index.ts` exports only
   `LoginPage`. The barrel contract must export the existing page object; no
   replacement page object is needed.
2. `apps/user-frontend/package.json` declares `"type": "module"`, while the
   root `package.json` has no module type. Shared `e2e/test-data.ts` and
   `e2e/fixtures.ts` sit outside the frontend package boundary, so Playwright's
   Node loader classifies that shared subtree through the root CommonJS package
   boundary. The files themselves use TypeScript ES-module exports. A
   setup-only collection command importing only `TEST_USERS` reproduced the
   CommonJS named-export failure, while the self-contained Admin MFA spec
   collected four tests. The error is therefore a scoped module-boundary
   defect, not evidence that `TEST_USERS` should be duplicated or converted to
   an unsafe default-import workaround.
3. `apps/user-frontend/e2e/auth.setup.ts` is an Admin setup copy: it describes
   and executes Admin login at `/admin/login`, uses the Admin fixture, and saves
   the result as the User auth state. That setup cannot provide a valid User
   session on the User frontend origin.
4. `apps/user-frontend/e2e/auth.spec.ts` intercepts the obsolete
   `/api/user/login` path. The current User store calls
   `/api/user/auth/login`. Collection repair alone would therefore expose a
   stale runtime test contract.
5. The `user-chromium` project applies User storage state to the entire selected
   suite while `auth.spec.ts` mixes anonymous login/reset tests and authenticated
   session tests. Remediation must explicitly isolate anonymous and
   authenticated browser contexts rather than relying on ambiguous inherited
   state.

The smallest supported module correction is to keep the one shared test-data
source and define its existing `e2e/` subtree as ESM with a scoped, private
`e2e/package.json` containing `"type": "module"`. The implementation task must
prove this under the actual Playwright loader before retaining it. A
package-wide module-system migration, copied test data, default-import shim, or
weakened Playwright transform is outside scope.

### Browser-journey inventory

| Required journey | Current Playwright evidence | Current status |
|---|---|---|
| Registration | No registration test exists. The current login page contains the registration mode and calls `/api/user/auth/register`. | Missing |
| Email OTP verification | No registration-email verification test exists. Reset OTP tests are not email-ownership evidence. | Missing |
| Login | Seven login/form tests exist in `auth.spec.ts`, but the file does not collect and mocks an obsolete route. | Present but unexecutable/stale |
| Refresh/session persistence | One page-reload persistence test exists; no test proves the current `/api/user/auth/refresh` behavior or session continuation after refresh. | Partial |
| Password-reset request | Identifier submission and transition to OTP exist. | Present but uncollected |
| Reset OTP verification | OTP entry and transition to new-password state exist. | Present but uncollected |
| Password update | New-password completion exists. | Present but uncollected |
| Old-session invalidation after reset | No browser test exists. | Missing |
| Logout/session invalidation | Logout redirect and protected-route denial tests exist. | Present but uncollected |
| Query-JWT rejection | No browser-level case exists. Existing SEC-002 backend/structural evidence must remain green; a browser case may be added only when it exercises real browser routing rather than a fake assertion. | Missing at browser level |

No `skip`, `fixme`, or `only` marker was found in the current User auth spec.
Go or Vitest evidence does not satisfy the missing browser evidence.

### Classification

- **Severity:** P1.
- **Reason:** a mandatory Phase 1 security journey suite collects zero tests,
  leaving registration, identity verification, login/session, and reset
  behavior without required browser evidence. The defect is currently in test
  module/setup contracts rather than proven production behavior, but it can
  conceal user-visible security regressions and blocks the phase gate.
- **Owner:** User frontend E2E and shared Playwright test infrastructure.
- **Likely files:** `apps/user-frontend/e2e/auth.spec.ts`,
  `apps/user-frontend/e2e/auth.setup.ts`,
  `apps/user-frontend/e2e/pages/index.ts`, existing page objects,
  `e2e/test-data.ts`, `e2e/fixtures.ts`, a scoped `e2e/package.json`, and
  `playwright.config.ts` only where context/project isolation requires it.
- **Violated gate criterion:** complete critical Phase 1 browser evidence for
  registration, email OTP verification, login, refresh/session behavior,
  password reset, and old-session invalidation.
- **Violated process/testing policy:** mandatory E2E evidence must execute;
  integration or unit evidence cannot replace a failed browser journey, and a
  confirmed test defect requires regression evidence.
- **Dependencies:** SEC-001 through SEC-007 must remain merged and passing. No
  new package dependency is justified.
- **Remediation risk:** medium. Correcting the loader is small, but the stale
  setup, endpoint, and missing journeys require careful deterministic fixtures
  without changing product authentication behavior.
- **Rollback risk:** low for runtime because the task is E2E/test-infrastructure
  only. Reverting would restore the zero-test failure and must therefore also
  restore the `PHASE 1 FAIL` state.

## 3. Blocker B: canonical Go formatting

### Exact evidence and complete inventory

The reproducible gate scope is:

```text
gofmt -l packages/auth packages/validation packages/sms packages/notification packages/secrets packages/observability packages/resilience packages/audit packages/db apps/admin-bff apps/api-server apps/user-bff apps/trade-bff apps/payment-service
```

It reports exactly 70 tracked files on the evaluated main. The working tree is
clean; no reported file is vendored or generated, and no file is excluded by a
repository formatting policy. The checkout has `core.autocrlf=true` and no
`.gitattributes`, which produces two evidence classes.

#### 39 repository-canonical files reported only because of checkout CRLF

The Git blob for each file is already accepted by `gofmt`; the Windows working
copy differs only by line ending. Last-touch provenance is 13 files in the
SEC-007 squash commit and 26 files in the SEC-006 squash commit.

**Last touched by `54d9eaefcd0aa1f954c768ea94a4b048a47937ab`
(`feat(admin-auth): require super admin mfa`) — 13 files:**

```text
packages/auth/admin_mfa.go
packages/auth/admin_mfa_test.go
packages/auth/auth.go
packages/auth/jwt.go
packages/auth/middleware.go
packages/auth/session.go
apps/admin-bff/server/admin_mfa.go
apps/admin-bff/server/admin_mfa_integration_test.go
apps/admin-bff/server/app.go
apps/admin-bff/server/handlers_admin_auth.go
apps/admin-bff/server/handlers_helpers.go
apps/admin-bff/server/reauthentication.go
apps/payment-service/handlers/webhook_security_test.go
```

**Last touched by `ca53ead8a90c06183f4147b0d2a78bb4c563a28c`
(`feat(security): add abuse controls and retire the former provider`) — 26
files:**

```text
packages/validation/cors.go
packages/validation/cors_test.go
packages/validation/csrf.go
packages/validation/edge_config.go
packages/validation/edge_security_test.go
packages/validation/ip.go
packages/validation/middleware.go
packages/resilience/ratelimit/login_lockout.go
packages/resilience/ratelimit/middleware_test.go
packages/resilience/ratelimit/policy.go
packages/resilience/ratelimit/policy_test.go
apps/user-bff/server/app.go
apps/user-bff/server/auth_handlers.go
apps/trade-bff/server/app.go
apps/trade-bff/server/ws_origin.go
apps/trade-bff/server/ws_origin_test.go
apps/payment-service/handlers/deposit.go
apps/payment-service/handlers/payment_provider_retirement_test.go
apps/payment-service/handlers/webhook.go
apps/payment-service/handlers/webhook_security.go
apps/payment-service/providers/provider.go
apps/payment-service/server/app.go
apps/payment-service/server/circuits.go
apps/payment-service/server/config.go
apps/payment-service/server/inquiry.go
apps/payment-service/server/payment_provider_retirement_test.go
```

#### 31 repository blobs with deterministic canonical `gofmt` diffs

All 31 were last touched by the one-time import commit
`4facb23638c39fdffa482b339e20b8ff4a88d456`
(`chore(repo): import local project through SEC-005`). That import combined
earlier no-Git work, so a more specific originating roadmap task cannot be
reliably assigned from Git history without inventing provenance.

```text
packages/validation/csrf_test.go
packages/validation/sanitize.go
packages/notification/email_test.go
packages/notification/inapp/inapp.go
packages/notification/inapp/inapp_test.go
packages/notification/service_test.go
packages/notification/template_store.go
packages/notification/testhelpers_test.go
packages/resilience/circuitbreaker/breaker_test.go
packages/resilience/ratelimit/middleware.go
packages/resilience/ratelimit/user_limiter.go
packages/resilience/ratelimit/websocket.go
packages/db/credentials_test.go
packages/db/replica.go
apps/user-bff/internal/models/oauth.go
apps/trade-bff/server/alerts.go
apps/trade-bff/server/batcher.go
apps/trade-bff/server/contest_events_consumer.go
apps/trade-bff/server/hub.go
apps/trade-bff/server/hub_contest.go
apps/trade-bff/server/hub_contest_test.go
apps/trade-bff/server/kafka_consumers.go
apps/trade-bff/server/leaderboard_broadcast_test.go
apps/trade-bff/server/metrics.go
apps/trade-bff/server/notification_consumer.go
apps/trade-bff/server/trading_handlers.go
apps/payment-service/handlers/webhook_test.go
apps/payment-service/handlers/withdraw.go
apps/payment-service/handlers/withdraw_test.go
apps/payment-service/providers/jibit.go
apps/payment-service/providers/nowpayments.go
```

Standard `gofmt` parses all 70 files successfully. Its proposed changes are
deterministic layout/whitespace or line-ending normalization; no manual style,
symbol rename, logic reorder, lint fix, dependency change, or semantic refactor
is justified. Because 39 blobs are already canonical, those files may produce
no staged content diff after Git normalization even though the task must run
`gofmt` over and account for the complete 70-file inventory.

The task also needs the smallest direct scope expansion: a root
`.gitattributes` rule `*.go text eol=lf`. Without it, a future Windows checkout
with `core.autocrlf=true` can recreate the same 39 false-positive reports after
the source-format commit. The rule is repository checkout hygiene, not an
application or toolchain change.

### Classification

- **Severity:** P2, while still Phase 1 gate-blocking.
- **Reason:** the finding violates the mandatory formatting/static evidence and
  repository reproducibility standard, but current evidence shows no production
  security behavior defect: 31 files need standard layout and 39 are a Windows
  checkout line-ending artifact. This is lower severity than a zero-test
  authentication E2E suite even though both must be cleared before the gate.
- **Owner:** Go source owners for auth, validation, notification, resilience,
  database, Admin/User/Trade BFFs, and payment service; repository checkout
  hygiene for the LF rule.
- **Likely files:** exactly the 70 inventoried Go files plus `.gitattributes` as
  the evidence-backed, non-behavioral scope expansion.
- **Violated gate criterion:** the Phase 1 formatting/static check must return
  no files for the declared scope.
- **Violated process/testing policy:** touched/relevant Go sources must satisfy
  formatting, vet, lint, test, race, and build checks reproducibly.
- **Dependencies:** SEC-001 through SEC-007 remain the behavioral baseline. It
  does not depend on P1-REM-001 and can run independently.
- **Remediation risk:** low to medium because the mechanical diff is broad.
  Complete diff review and proportional security regressions are required to
  catch accidental non-formatting edits.
- **Rollback risk:** low for behavior. Reverting restores noncanonical source
  and Windows checkout ambiguity and therefore restores the gate blocker.

## 4. P1-REM-001 — Restore complete User authentication Playwright evidence

The following block is copy/paste-ready for a later implementation invocation.

```text
GIT-BACKED FAILED-GATE REMEDIATION — IMPLEMENT P1-REM-001 ONLY

Task: P1-REM-001 — Restore complete User authentication Playwright evidence
Branch: codex/p1-rem-001-user-auth-playwright
Commit: test(e2e): restore user authentication browser coverage
Base: latest authorized main after this remediation plan is merged

Goal:
Restore executable, deterministic, complete Phase 1 User authentication browser
evidence. Fix the confirmed Playwright collection/module contracts and add only
the missing User authentication journeys required by the failed gate.

Dependencies:
- SEC-001 through SEC-007 remain merged and passing on main.
- The planning PR containing this task is merged.
- P1-REM-002 is independent and is not a prerequisite.

Primary scope:
- apps/user-frontend/e2e/**
- e2e/test-data.ts
- e2e/fixtures.ts
- e2e/package.json (scoped ESM boundary, when validated)
- playwright.config.ts only for proven anonymous/authenticated project isolation
- User frontend package/TypeScript configuration only if executed evidence proves
  the scoped e2e module boundary is insufficient
- focused remediation tests and report evidence

Forbidden scope:
- no product authentication policy or backend behavior change;
- no duplicated TEST_USERS or copied test-data source;
- no package-wide module-system migration;
- no default-import shim that hides an unresolved CommonJS/ESM contract;
- no weakened Playwright transform, assertion, test deletion, skip, fixme, only,
  or unit-only substitution;
- no SEC-001 through SEC-007 behavior change, dependency upgrade, or Phase 2 work;
- no production/external provider access and no real credential or user data.

Required implementation:
1. Export the existing DashboardPage from the User page-object barrel and add a
   focused module-contract/collection regression.
2. Preserve the single shared test-data source and make the existing e2e shared
   subtree ESM for the actual Playwright loader, preferably with a scoped private
   e2e/package.json using type=module. Prove named imports for TEST_USERS,
   generateTestEmail, fixtures, and helpers work. Do not duplicate or default-wrap
   the exports.
3. Correct auth.setup.ts so setup constructs a User session on the User origin,
   or split project/context setup when that is the smaller reliable design. It
   must not log in as Admin or save Admin state as User state.
4. Align intercepted endpoints and response contracts with the current User
   frontend, including /api/user/auth/login and /api/user/auth/refresh. Do not
   change application routes merely to match stale tests.
5. Isolate anonymous registration/login/reset tests from authenticated
   persistence/logout tests. No inherited storage state may make an anonymous
   assertion vacuous.
6. Make auth.spec.ts collect a recorded, nonzero expected test count with no
   hidden skip/fixme/only markers.
7. Preserve and execute current login, invalid-login, logout, reset request,
   reset OTP, and password-update cases.
8. Add deterministic browser journeys for registration, registration email OTP
   verification, current refresh/session continuation, and old-session rejection
   after password reset. Mock ARCaptcha and delivery only with local synthetic
   fixtures; never call a real provider.
9. Add a browser query-JWT rejection case only when it can exercise the current
   browser/backend routing honestly. In every case rerun the existing SEC-002
   query-auth regression.
10. Keep browser/test logs free of access tokens, refresh tokens, OTP/reset
    values, passwords, cookies, and generated test credentials.
11. Remove Playwright reports, traces, videos, screenshots, storage-state files,
    and test-results after sanitized evidence is recorded.

Required tests and evidence:
- pnpm exec playwright test apps/user-frontend/e2e/auth.spec.ts
  --project=user-chromium --list
  must exit 0, report the expected nonzero count, import DashboardPage and shared
  helpers, and show no skip/fixme/only marker.
- Run the complete user-chromium authentication suite and prove registration,
  email OTP verification, login success, invalid login, refresh/session
  continuation, logout, password-reset request, reset OTP verification,
  new-password completion, and old-session invalidation.
- Run any separately defined authenticated User setup/project and all other
  User specs affected by shared helpers.
- Run trade-chromium and admin-chromium collection/execution where the shared
  e2e module boundary is consumed.
- Run sec007-admin-mfa and retain the approved 4/4 result or a documented current
  equivalent.
- Run User frontend lint, typecheck, Vitest, and production build.
- Run SEC-001 isolation, SEC-002 query-JWT, SEC-003 OTP/reset, SEC-005 log
  redaction, SEC-006 abuse/security, and SEC-007 Admin MFA regressions.
- Capture browser/test output and scan it for sensitive values.
- Run relevant FND validators, Markdown/path/link checks, changed-file secret
  scan, generated-artifact scan, and git diff --check.

Acceptance criteria:
- The original two module-loader errors are gone and the suite collects tests.
- User setup authenticates User context only.
- Every mandatory User authentication browser journey executes and passes.
- No test is skipped, weakened, deleted, or replaced by unit/integration-only
  evidence.
- SEC-001 through SEC-007 guarantees remain intact.
- No real secret, provider, production system, or real user is used.
- Generated Playwright artifacts are removed.
- No later remediation, ARCH-001, Phase 2 task, or gate rerun starts.
- Paid-production remains NO-GO.

Delivery:
- Review the complete scoped diff and exact command ledger.
- Open one PR targeting main; required final-head CI and review must pass.
- Merge only after every acceptance criterion passes and no unresolved review
  remains.
- Record rollback as reverting the test/config commit; a revert restores the
  failed-gate blocker and does not authorize the gate.
- Stop after P1-REM-001 delivery. Do not run the Phase 1 exit gate.
```

## 5. P1-REM-002 — Canonically format Phase 1 Go sources

The following block is copy/paste-ready for a later implementation invocation.

```text
GIT-BACKED FAILED-GATE REMEDIATION — IMPLEMENT P1-REM-002 ONLY

Task: P1-REM-002 — Canonically format Phase 1 Go sources
Branch: codex/p1-rem-002-gofmt-phase1
Commit: chore(go): format phase 1 security sources
Base: latest authorized main after this remediation plan is merged

Goal:
Make the complete Phase 1 gate Go scope reproducibly canonical under the
repository-supported gofmt without changing behavior.

Dependencies:
- SEC-001 through SEC-007 remain merged and passing on main.
- The planning PR containing this task is merged.
- P1-REM-001 is independent and is not a prerequisite.

Primary scope:
- exactly the 70 Go files inventoried in this plan;
- .gitattributes only for the evidence-backed `*.go text eol=lf` checkout rule;
- focused report/evidence required by this remediation.

Forbidden scope:
- no manual restyling, semantic refactor, symbol rename, logic reorder, lint fix,
  warning cleanup, dependency change, generated/vendor edit, or application
  behavior change;
- no test deletion/weakening and no unrelated source formatting;
- no Playwright remediation, ARCH-001, Phase 2 work, or gate rerun.

Required implementation:
1. Begin with a clean working tree and reproduce the exact 70-file `gofmt -l`
   inventory using the gate scope. Stop on any count/path difference until it is
   explained against latest main.
2. Record each path and its pre-change provenance. Confirm no file is generated,
   vendored, untracked, or excluded by normal Go conventions.
3. Add only the root `.gitattributes` rule `*.go text eol=lf` so a Windows clone
   does not recreate CRLF-only reports.
4. Run standard gofmt on exactly the 70 files. Do not hand-edit formatting.
5. Expect 39 repository-canonical/EOL-only paths to have no staged source
   content change after Git normalization. Expect canonical source diffs for the
   31 imported-baseline blobs; investigate any different staged set.
6. Inspect every diff and reject any change not deterministically produced by
   gofmt or the explicit LF attribute.
7. Run the exact gate-scope gofmt -l again and require zero output, including
   after a clean LF-respecting checkout/worktree verification.

Required tests and evidence:
- Record the pre-change 70-file inventory and final zero-file inventory.
- git check-attr must show eol=lf for representative and all inventoried Go
  paths; verify no unintended attribute applies to non-Go files.
- git diff --check must pass and the complete diff must be reviewed as
  formatting/line-ending only.
- Run Go tests for every touched module/package: auth, validation,
  notification, resilience, db, Admin BFF, User BFF, Trade BFF, and payment
  service as applicable to the actual staged files.
- Run race tests for concurrency-sensitive auth, rate-limit/session, Admin,
  Trade BFF, and webhook packages touched by the staged diff.
- Run Go vet for all touched packages, the pinned repository golangci-lint/CI,
  and builds for touched applications/package entry points.
- Run relevant SEC-001 through SEC-007 structural and behavioral regression
  validators proportionate to the touched packages.
- Run the retired-provider active-reference validator and remaining-provider
  regressions; no removed integration may be restored.
- Run changed-file secret scan, generated coverage/build artifact scan,
  Markdown/path/link checks for the report, and git diff --check.

Acceptance criteria:
- The pre-change inventory reconciles to exactly 70 tracked files.
- Standard gofmt alone produced all Go source changes.
- The final gate-scope gofmt -l output is empty and remains empty in a clean
  LF-respecting checkout.
- No intentional semantic, lint, dependency, or application behavior change is
  present.
- Required tests, race checks, vet, pinned lint/CI, and builds pass on the final
  head.
- No secret or generated output is tracked and no retired provider is restored.
- No later remediation, ARCH-001, Phase 2 task, or gate rerun starts.
- Paid-production remains NO-GO.

Delivery:
- Use the protocol-supported `chore` prefix; `style` is not in the canonical
  Conventional Commit allowlist.
- Open one PR targeting main; required final-head CI and review must pass.
- Merge only after every acceptance criterion passes and no unresolved review
  remains.
- Record rollback as a reviewed revert. Reverting restores the formatting gate
  blocker and does not authorize the gate.
- Stop after P1-REM-002 delivery. Do not run the Phase 1 exit gate.
```

## 6. Dependencies, ordering, and merge rules

P1-REM-001 and P1-REM-002 own disjoint implementation concerns and may run in
parallel from the latest main containing this plan. Neither task depends on the
other. If one merges first, the other must verify its base and update through a
non-destructive normal branch integration before final CI; it must not force
push over reviewed work.

Recommended review order is:

1. P1-REM-001 — restore User authentication browser evidence;
2. P1-REM-002 — format and make Go LF checkout reproducible; and
3. rerun the complete Phase 1 exit gate in a separate invocation only after
   both remediation PRs are merged and verified on main.

Each remediation task uses its own branch, one scoped Conventional Commit by
default, its own PR, exact command evidence, secret scan, final-head CI, and
resolved review. No remediation may be merged while a mandatory test is
failing, skipped unexpectedly, or unexecuted. Merging this planning document
does not satisfy either blocker.

## 7. Separate Phase 1 gate rerun

Only a later gate-only invocation may change `PHASE 1 FAIL` to `PHASE 1 PASS`.
It must evaluate the latest authorized main after both remediation tasks are
merged. It must execute the complete gate again, not merely the formerly
failing Playwright command and `gofmt`.

The rerun must include:

- SEC-001 through SEC-007 complete relevant regressions;
- complete relevant Go tests, concurrency race tests, vet, pinned lint, and
  application builds;
- complete critical Phase 1 Playwright E2E for User auth/reset, sensitive-action
  reauthentication/denial, and Admin MFA;
- real positively identified disposable PostgreSQL and Redis evidence;
- fresh target initialization and supported upgrade migrations;
- coverage for critical auth, Admin, reauthentication, redaction, and abuse
  packages;
- frontend install integrity, lint, typecheck, Vitest, and production builds;
- approved payment-provider retirement and remaining-provider regressions;
- Compose and Nginx validation;
- Markdown/path/link, secret, credential-pattern, active-sensitive-reference,
  and generated-artifact scans;
- complete cleanup evidence;
- unresolved P0/P1 review; and
- report-bearing final-head CI and review evidence.

The rerun must preserve the original failed-gate history: evaluated main SHA,
Playwright command and exit `1`, zero collected tests, missing page-object
export, CommonJS named-import error, original 70-file formatting result, PR #3,
and decision `PHASE 1 FAIL`. A successful later report adds remediation and new
gate evidence; it must never rewrite the original gate as a pass.

## 8. Planning-only decision and stop boundary

This invocation implemented no remediation. It did not change Playwright
imports, shared module configuration, User setup, auth journeys, Go sources,
application behavior, migrations, or runtime configuration. Read-only
Playwright collection diagnostics created temporary report/result directories;
those exact generated directories were removed, and the tree was clean before
the planning branch was created.

Phase 1 remains `PHASE 1 FAIL`. ARCH-001 and Phase 2 remain blocked and were not
started. No production deployment occurred. Paid-production remains `NO-GO`.
