# Codex Phase 5 Prompt — Prize and Settlement

Process authority: [Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md).
Git-specific instructions below apply only in Git-backed mode. In local mode,
use the protocol's local evidence and report rules; do not initialize Git or
contact a remote without explicit authorization. A mode override changes
delivery mechanics only, never policy, safety, testing, or acceptance criteria.

Use this exact prompt for every Codex invocation in Phase 5. Each
invocation performs only one eligible roadmap task. Reuse it until the phase
exit report is PASS.

```text
You are the Phase 5 execution controller for the Tragge
production-readiness program.

Phase name: Prize and Settlement

Phase objective:
Replace every competing prize/finalization path with one versioned,
cent-perfect, reconstructable settlement pipeline.

Tasks in approved order:
- `PRIZE-001` — Implement the shared `tralent_v1` rule package
- `PRIZE-002` — Implement eligibility and actual-winner calculation
- `PRIZE-003` — Implement exact money allocation, grouped ranks, and ties
- `PRIZE-004` — Integrate one prize package into previews and APIs
- `PRIZE-005` — Make Settlement the sole finalization owner
- `PRIZE-006` — Implement Engine freeze, close, and settlement barrier
- `PRIZE-007` — Post payouts and reconcile Prize Pool exactly
- `PRIZE-008` — Add settlement review and dispute reconstruction tools
- `PRIZE-009` — Delete Power Law and obsolete prize implementations

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

- Distribution version is `tralent_v1`.
- Planned winners use approved small-contest fixtures and 30% half-up rank-band
  rules for 12+ real participants.
- Eligibility requires at least one Filled Trade.
- Users without a Filled Trade do not appear in the prize table.
- The free-practice system participant is excluded.
- If eligible users are fewer than planned winners, occupied weights are
  renormalized to 100%; no money remains or becomes Platform Revenue.
- Exact ties pool occupied positions and split equally.
- Grouped rank members receive exactly equal payouts.
- Preview and final settlement use the same package and immutable inputs.
- Settlement is the sole finalization and payout owner.
- Leaderboard is projection-only.
- Power Law and obsolete prize paths must be deleted after cutover.

## Phase-specific mandatory test and evidence requirements

- Golden fixtures for every participant count/band boundary and all explicit
  2–11 participant distributions.
- Property tests for winner count, normalization, exact payout sum, grouped-rank
  equality, tie equality, deterministic residual allocation, and no float use.
- Eligibility tests with no-trade users, system users, fewer eligible users, and
  empty eligible sets.
- Preview-versus-settlement byte/amount equivalence tests.
- Settlement state-machine integration tests: freeze, stop orders, cancel
  pending, close positions, shard barrier, immutable result snapshot, payout,
  reconciliation, completion, and notification.
- Retry/crash tests at every settlement step proving no duplicate credit.
- Dispute reconstruction tests from immutable snapshots/events.
- E2E: contest completion, final rank, exact wallet prize, no-trade exclusion,
  settlement review, and successful retry.

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

PASS requires `tralent_v1` as the only prize implementation, one Settlement
owner, exact Prize Pool reconciliation, idempotent payouts, immutable
reconstruction evidence, and deletion of Power Law/obsolete finalizers. Create
`docs/codex/reports/phase-5-exit-report.md`.

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
Do not start Phase 6.
Stop after the required task or phase-gate report.
```
