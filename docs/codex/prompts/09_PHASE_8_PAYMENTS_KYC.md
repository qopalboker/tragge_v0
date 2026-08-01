# Codex Phase 8 Prompt — Payments and KYC

Process authority: [Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md).
Git-specific instructions below apply only in Git-backed mode. In local mode,
use the protocol's local evidence and report rules; do not initialize Git or
contact a remote without explicit authorization. A mode override changes
delivery mechanics only, never policy, safety, testing, or acceptance criteria.

Use this exact prompt for every Codex invocation in Phase 8. Each
invocation performs only one eligible roadmap task. Reuse it until the phase
exit report is PASS.

```text
You are the Phase 8 execution controller for the Tragge
production-readiness program.

Phase name: Payments and KYC

Phase objective:
Implement secure, idempotent external deposits, exact ledger credit, manual KYC,
and Super-Admin-controlled USDT TRC20 withdrawals without storing private keys.

Tasks in approved order:
- `PAY-001` — Create canonical payment provider interfaces and user gateway selection
- `PAY-002` — Implement Jibit and Sepal Rial gateways
- `PAY-003` — Implement Plisio and NOWPayments crypto gateways
- `PAY-004` — Implement immutable Nobitex USDT/IRR payment quotes
- `PAY-005` — Implement deposit limits, webhook security, and net ledger credit
- `PAY-006` — Implement manual USDT TRC20 withdrawal workflow
- `PAY-007` — Implement extensible manual KYC review
- `PAY-008` — Build payment and withdrawal reconciliation operations

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

- User chooses among Jibit/Sepal for Rial and Plisio/NOWPayments for crypto.
- Sepal supports Rial sandbox testing.
- Deposits support Rial, USDT TRC20, and TRX.
- Minimum deposit is 4 USDT equivalent; maximum is 1000 USDT equivalent.
- Nobitex USDTIRT quote is captured and frozen when the payment request is
  created.
- Wallet credits the exact net confirmed amount reported by the gateway.
- Tragge does not store blockchain private keys or manage deposit addresses.
- Withdrawals are USDT TRC20 only, minimum 10 USDT, user pays fee, no initial
  daily limit, completed KYC required.
- Execution is external/manual; Super Admin enters transaction hash and status.
- Reject choices are RELEASE or DEDUCT; DEDUCT requires Super Admin,
  reauthentication, reason, compensating entry, and audit.
- KYC is reviewed manually by Support Admin or Super Admin.

## Phase-specific mandatory test and evidence requirements

- Provider-interface contract tests and user gateway-selection tests.
- Sandbox/fixture tests for create, inquire, callback/webhook, expiry, and
  provider error mapping.
- Signature, timestamp, replay-window, duplicate provider-ID, duplicate webhook,
  and out-of-order callback tests.
- Immutable quote, raw response, expiry, normalization, and concurrent
  confirmation tests.
- Deposit min/max boundary and exact net-ledger-credit tests.
- Withdrawal reservation, completion, duplicate completion, release, deduct,
  reauthentication, permission, audit, and transaction-hash tests.
- KYC state machine, document authorization, reviewer permissions, and audit
  tests.
- Reconciliation tests for gateway clearing, unmatched payment, duplicate,
  partial/failure, and manual resolution.
- E2E with sandbox/mocks: deposit, duplicate webhook safety, KYC submit/review,
  withdrawal submit, Super Admin approve/complete/reject.

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

PASS requires user-selectable tested gateways, immutable rates, secure
idempotent net credit, exact deposit limits, manual KYC, audited manual
withdrawals, and reconciliation operations with no unexplained ledger gap.
Create `docs/codex/reports/phase-8-exit-report.md`.

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
Do not start Phase 9.
Stop after the required task or phase-gate report.
```
