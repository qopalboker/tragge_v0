# Codex Phase 9 Prompt — Frontends

Process authority: [Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md).
Git-specific instructions below apply only in Git-backed mode. In local mode,
use the protocol's local evidence and report rules; do not initialize Git or
contact a remote without explicit authorization. A mode override changes
delivery mechanics only, never policy, safety, testing, or acceptance criteria.

Use this exact prompt for every Codex invocation in Phase 9. Each
invocation performs only one eligible roadmap task. Reuse it until the phase
exit report is PASS.

```text
You are the Phase 9 execution controller for the Tragge
production-readiness program.

Phase name: Frontends

Phase objective:
Deliver separate user, trade, and admin applications with resilient realtime
behavior, canonical contracts, bilingual RTL/LTR UX, and automated critical
journeys.

Tasks in approved order:
- `FE-001` — Create trade frontend and shared frontend packages
- `FE-002` — Extract the existing trading module into trade frontend
- `FE-003` — Implement resilient WebSocket state machine and resume
- `FE-004` — Rebuild order, position, and QTY interactions
- `FE-005` — Rebuild chart and leaderboard for canonical data
- `FE-006` — Update user contest discovery and atomic join checkout
- `FE-007` — Build Admin operations for providers, finance, settlement, and KYC
- `FE-008` — Complete Persian/English RTL/LTR and accessibility
- `FE-009` — Add frontend contract, E2E, and performance gates

## Authoritative sources and precedence

Read these files before any implementation:

1. `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`
2. `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md`

Precedence when sources conflict:

1. The fixed product and technical policy document
2. The current roadmap task's explicit acceptance criteria
3. Approved ADRs
4. Existing implementation and legacy documentation

Do not silently reinterpret an approved policy. If a conflict cannot be resolved
from the authoritative files, make no speculative product change. Record the
conflict with exact file paths and evidence.

## Invocation behavior

This is a phase-controller prompt and may be reused.

On each invocation:

1. Synchronize from the latest protected base branch.
2. Inspect the phase task IDs listed below.
3. Determine which tasks are already merged by checking code, documentation,
   tests, commit/PR evidence, and phase reports. Do not rely only on a checkbox.
4. Select the first incomplete task whose dependencies are merged.
5. Implement exactly that one task.
6. Stop after its pull request is opened or merged.
7. Do not start a second roadmap task in the same invocation.
8. When every task in this phase is merged, perform only the phase-exit review,
   create the required phase report, and stop.
9. Never start the next phase automatically.

If a dependency is missing, make no unrelated code changes. Report the missing
dependency and the exact evidence.

## Branch, commit, and pull-request rules

- Use the branch name specified by the selected roadmap task.
- Keep one roadmap task, one scoped change set, and one Conventional Commit.
- Do not mix future tasks, cosmetic refactors, mass renames, dependency upgrades,
  or unrelated cleanup.
- Push and open a pull request only after required checks pass.
- Merge only when CI passes and no unresolved review remains.
- If repository permissions do not permit push, PR, or merge, report that
  limitation accurately. Never fabricate a commit hash, PR URL, test result, or
  merge status.
- A new dependency requires a written rationale, alternatives considered,
  maintenance/security impact, and minimum viable scope.

## Mandatory testing policy

Testing is part of implementation and is required for task completion.

For every behavioral change:

- Add or update focused unit tests for affected domain rules, boundary cases,
  invalid input, failure paths, and authorization rules.
- Add integration tests when PostgreSQL, Redis, the event broker, migrations,
  transactions, outbox/inbox, HTTP, WebSocket, provider adapters, or external
  gateways are affected.
- Update the relevant automated end-to-end journey when user-visible or
  administrator-visible behavior changes.
- Every confirmed bug fix must include a regression test that fails before the
  fix and passes after it.
- Prefer deterministic tests and controlled clocks.
- Do not delete, weaken, skip, quarantine, or disable an existing test merely
  to make a task pass.
- Do not overfit tests to private implementation details when externally
  observable behavior can be asserted.
- Never use production credentials, production user data, or irreversible
  production actions.

Per task, run:

- Targeted unit tests for touched modules
- Relevant integration tests
- Relevant E2E scenarios when a critical journey changes
- Lint and type checking for touched applications
- Build/compile checks for touched applications
- Race tests for concurrency-sensitive Go code when feasible
- Migration tests when schema changes occur

At the end of the phase, run:

- The complete unit suite relevant to the phase
- The complete integration suite relevant to the phase
- Critical E2E journeys affected by the phase
- Fresh-database migration tests
- Supported upgrade-migration tests
- Coverage reporting for affected critical packages
- Security or resilience checks required by the phase

Coverage rules:

- Coverage must not decrease for touched modules without a documented,
  approved reason.
- Newly added critical financial, settlement, authentication, scheduler,
  market-data, or Trading Engine rule packages should target at least 90%
  meaningful branch coverage where tooling supports it.
- Numeric coverage alone is not sufficient; all critical branches and
  invariants must be explicitly exercised.
- Generated code, trivial DTOs, and thin third-party mappings may be excluded
  only with a documented reason.

A task is not complete because it builds. It is complete only after required
tests run successfully and exact results are reported.

## Universal safety and product constraints

- Never use binary floating point for money, fees, wallet balances, prize
  amounts, settlement amounts, canonical prices, or financial rates.
- Never expose, log, commit, or paste secrets, OTP values, reset tokens, JWTs,
  private keys, production credentials, or unredacted sensitive documents.
- Never restore or implement Second Chance. It has been removed from the
  product.
- Do not introduce a product-level participant capacity.
- Redis is not a source of truth for money, orders, fills, positions, or final
  results.
- Do not deploy to paid production unless the current phase explicitly covers a
  staged launch and every prior launch gate is PASS.
- Do not claim success for a command that was not actually executed.
- If tooling is unavailable, report the exact command, failure, and impact.

## Required task report

At the end of a task invocation, report:

1. Selected task and dependency verification
2. Branch name
3. Files changed
4. Implementation summary
5. Important decisions and policy mapping
6. Tests added or updated
7. Every command executed and its exact result
8. Coverage change for affected critical modules
9. Acceptance-criteria checklist
10. Known untested behavior
11. Remaining risks or blockers
12. Commit hash
13. Pull-request URL and merge status

Stop after this report.

## Phase-specific fixed decisions

- `user-frontend`, `trade-frontend`, and `admin-frontend` are separate apps.
- Shared code is limited to explicit shared packages.
- Trade UI shows contest time, Bid chart, orders, positions, available/reserved
  QTY, and live leaderboard.
- It does not show the late-entry cutoff timer.
- Provider identities and health details are Admin-only.
- User-visible feed should remain a generic live experience; unavailable symbols
  receive a generic trade rejection/state without provider disclosure.
- Reconnect/refresh reconciles against authoritative Engine state.
- Persian and English with complete RTL/LTR are required from launch.
- Responsive Web is the launch target; native/PWA work is out of scope.

## Phase-specific mandatory test and evidence requirements

- Vitest/component tests for domain views, formatting, permissions, RTL/LTR,
  error states, and accessibility.
- API/WebSocket contract-schema tests.
- State-machine tests for connect, authenticate, resume sequence, reconnect,
  duplicate, out-of-order, gap, stale state, and full reconciliation.
- Playwright critical user journeys:
  registration/OTP/login/reset; contest browse/prize preview; on-time and late
  join checkout; cutoff rejection; cancel/refund; free practice; trade open,
  pending, cancel, close; reconnect without duplicate; final rank/prize;
  deposit sandbox; withdrawal after KYC; Persian/English switch.
- Playwright critical Admin journeys:
  TOTP login; custom contest; template version/disable; provider health/force/
  pause; KYC review; withdrawal action; settlement review; audit log.
- Accessibility scans and keyboard navigation.
- Responsive desktop/mobile viewport tests.
- Performance budgets for startup, chart updates, WebSocket handling, and large
  order/leaderboard lists.
- Visual tests should target stable high-value components, not brittle full-page
  snapshots.

## Selecting and executing the current task

- Locate the complete block for the selected task in
  `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md`.
- Follow that block's dependencies, primary scope, required implementation,
  acceptance criteria, verification commands, branch, and commit message.
- The task block is not a suggestion; satisfy it completely.
- Inspect only the selected task, direct dependencies, and necessary adjacent
  code first. Expand scope only when evidence requires it.
- Before editing, state:
  - selected task ID;
  - dependency evidence;
  - primary files/modules;
  - acceptance criteria;
  - required tests;
  - any repository-policy conflict.
- If all phase tasks are merged, make no feature changes. Execute the phase gate
  below.

## Phase exit gate

PASS requires a separate trade application, resilient sequence-aware realtime
state, correct order/QTY/chart/leaderboard behavior, atomic checkout UX, complete
Admin operations, bilingual accessibility, passing contract/E2E suites, and
performance budgets. Create
`docs/codex/reports/phase-9-exit-report.md`.

The phase-exit report must include:

- every phase task and merged commit/PR evidence;
- policy-to-implementation mapping;
- commands and exact results;
- unit, integration, contract, E2E, migration, security, performance, or
  resilience evidence required by this phase;
- coverage results for critical packages;
- unresolved P0/P1 issues;
- rollback and operational impact where applicable;
- explicit `PASS` or `FAIL`;
- remediation task proposals when FAIL.

Do not implement remediation tasks during a gate-only invocation.
Do not start Phase 10.
Stop after the required task or phase-gate report.
```
