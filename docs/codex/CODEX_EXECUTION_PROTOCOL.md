# Codex execution protocol

**Status:** Canonical task-lifecycle and evidence protocol

**Effective date:** 2026-07-25

**Applies to:** Local extracted projects, test/development repositories, and
the canonical main repository

## Purpose and process authority

This document is the single process authority for executing Tragge roadmap
tasks. Phase prompts select work and task blocks define task-specific scope;
they do not replace this lifecycle. A new Codex session must read this protocol,
the selected task block, and the substantive authorities before editing.

For product, architecture, and implementation decisions, precedence is:

1. [Fixed Product and Technical Policies](../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md).
2. The selected task's explicit scope and acceptance criteria in the
   [production roadmap](PRODUCTION_ROADMAP_AND_CODEX_TASKS.md).
3. Accepted, non-superseded [ADRs](../adr/0001-target-runtime-architecture.md).
4. The [canonical domain glossary and version catalog](../product/canonical-domain-glossary-and-version-catalog.md).
5. Existing implementation and legacy documentation as current-state evidence.

For execution process, precedence is:

1. An explicit invocation instruction selecting local or Git-backed delivery,
   but only for delivery mechanics and only within the substantive authorities.
2. This protocol.
3. Compatible task-specific delivery instructions in the roadmap.
4. The reusable [phase controller prompts](prompts/README.md).
5. Contributor and repository templates.

Never silently resolve a substantive conflict. Stop before the conflicting
change, cite both sources, record the blocker, and request the decision owner.
An invocation may waive Git delivery; it cannot waive product policy, safety,
testing, evidence, acceptance criteria, or production restrictions.

## Invariant: one task, one goal, one scoped change set

Each invocation implements exactly one eligible roadmap task and one goal.
It uses one scoped change set and stops after that task's report. Do not bundle a
second roadmap task, implement a future task early, or start the next phase.

Only files needed by the task's primary scope, allowed-file list, tests,
documentation, and evidence may change. Unrelated cleanup, cosmetic mass edits,
renames, dependency upgrades, and broad refactors are prohibited. Preserve
pre-existing unrelated changes. If an unexpected file changes, restore only
the task's accidental change when safe; otherwise stop and report the overlap.

Never change product policy, financial formulas, canonical representations, or
architecture boundaries implicitly. Never weaken, skip, delete, quarantine, or
rewrite a valid test merely to make a check pass. Never report an unexecuted or
unavailable check as successful.

## Execution mode decision

Determine and state the execution mode before changing files.

- **Local mode** applies when an explicit invocation selects a local extracted
  project or Git delivery is explicitly waived.
- **Git-backed mode** applies only when the selected directory is an authorized
  repository and branch/remote operations are requested or authorized.
- The presence of `.git` alone does not authorize branch, remote, push, merge,
  release, or deployment operations.
- If mode is ambiguous and the difference would cause a remote or destructive
  action, stop and request explicit authority. Read-only inspection may continue.

## Task selection and dependency verification

Use this deterministic sequence:

1. Read the fixed policy, this protocol, roadmap task block, applicable accepted
   ADRs, canonical glossary, phase prompt, and direct dependency reports.
2. If the invocation names a task, confirm it belongs to the current phase and
   its dependencies are complete. Otherwise select the first incomplete task in
   phase order whose dependencies are satisfied.
3. Prove completion from repository evidence; a report's claim is necessary but
   never sufficient by itself.
4. Record the selected task, dependency evidence, primary files/modules,
   acceptance criteria, required tests, execution mode, and any conflict before
   editing.
5. If a dependency is incomplete, change nothing unrelated. Report the exact
   missing evidence and stop.

For a locally completed dependency, verify at minimum:

- every required artifact and report exists;
- acceptance criteria have repository evidence, not only checked boxes;
- the dependency's focused tests pass now;
- relevant completed-task regression checks pass;
- the dependency report discloses unavailable checks and unresolved risks;
- no later task was started as part of that dependency; and
- comparison against the declared baseline or another deterministic inventory
  finds no unexpected changed file.

For a Git-backed dependency, also verify its change is merged into the selected
base, required CI passed, and review/merge evidence is real. Local completion is
not merge evidence.

## Task-start checklist

Before editing, record or verify all of the following:

- [ ] Task ID, title, goal, non-goals, and stop condition.
- [ ] Execution mode and selected project/repository identity.
- [ ] Authoritative documents and applicable ADR status.
- [ ] Direct dependencies and their current evidence.
- [ ] Primary scope, allowed paths, forbidden scope, and pre-existing changes.
- [ ] Product, security, financial, contract, migration, and observability impact.
- [ ] Required unit, integration, E2E, regression, lint, typecheck, build,
      migration, and structural checks, with not-applicable reasons.
- [ ] Rollback or recovery path and destructive-action guards.
- [ ] Dependency additions or approvals required.
- [ ] Known unavailable tooling that may limit acceptance evidence.

Use the [roadmap task template](templates/ROADMAP_TASK_TEMPLATE.md) when authoring
or normalizing a task prompt.

## Implementation rules

Implement the smallest complete change that satisfies the selected task. Keep
domain ownership consistent with ADR-0001: Platform Modular Monolith, Trading
Engine, and Market Data Service remain the three bounded systems. Use canonical
glossary names. The removed Second Chance capability must not be introduced as
active behavior, and product-level Participant Capacity must not be created.

Update documentation in the same change whenever behavior, contracts, schema,
operations, configuration, support procedures, or user/Admin workflows change.
Documentation-only tasks must distinguish approved targets from current legacy
behavior and must not imply that planned implementation exists.

### Dependency approval

A new dependency is allowed only when all conditions are documented:

1. The current platform, repository dependency set, or standard library is
   insufficient.
2. A simpler implementation and existing-library alternative were considered.
3. Maintenance ownership, release cadence, security history, transitive impact,
   and supply-chain risk are acceptable.
4. License use is acceptable for the repository and deployment.
5. The version is pinned according to repository policy.
6. Removal, rollback, and data/contract compatibility implications are known.
7. The dependency is limited to the smallest package and runtime scope.

If human approval is required and absent, stop and report a blocker. Do not add,
upgrade, or substitute the dependency speculatively.

### ADR creation

Create an ADR when a task introduces or changes any of these decisions:

- system or module boundaries;
- database ownership;
- public API or WebSocket contracts;
- cross-system command/event envelopes;
- canonical data representations;
- authentication, authorization, cryptographic, or other security boundaries;
- consistency or concurrency model;
- persistence, replay, backup, or recovery model;
- provider-selection strategy;
- deployment architecture; or
- an irreversible or expensive-to-reverse choice.

An ADR must contain context, decision, alternatives considered, consequences,
migration impact, rollback/reversal cost, affected roadmap tasks, status,
date/version, and references to any superseded ADRs. Do not create an ADR for a
trivial implementation detail that does not change one of these decisions.
Until an ADR is Accepted, implementation must not treat its proposal as policy.

## Testing and evidence rules

Testing is mandatory and proportional to impact.

For every behavior-changing task:

- add focused unit tests for important domain logic, boundaries, invalid input,
  failure paths, authorization, idempotency, and deterministic edge cases;
- add integration tests when databases, migrations, transactions, events,
  service boundaries, providers, Redis, brokers, or external adapters change;
- update critical user/Admin E2E journeys when their observable flow changes;
- add a regression test for every confirmed bug;
- run lint, typecheck, and build/compile for every touched module; and
- report exact commands, exit status, pass/fail counts, and material warnings.

For a documentation-only task, do not add artificial application unit tests.
Add focused structural, local-link, terminology, repository-path, task-ID,
policy, template, or evidence validation. Re-run completed prerequisite-task
regressions where relevant. Run Markdown lint when available; otherwise report
its absence and run the focused Markdown structure/style/link checks.

At the end of every Epic or Phase, the gate invocation must run the complete
relevant unit suite, complete relevant integration suite, critical E2E
journeys, fresh/upgrade migration tests, coverage evidence, and the additional
security/performance/resilience checks required by that gate. It must produce a
phase report with explicit `PASS` or `FAIL`. Numeric coverage is evidence, not
proof of correctness; critical branches and invariants require direct tests.

Unavailable tooling remains an explicit limitation. Record the intended
command, observed failure or absence, impact, and whether an alternative static
check ran. Never convert a static check into a runtime-pass claim.

## Local execution mode

Local mode works directly in the selected local project directory. Git metadata
is optional, and absence of `.git` must not block implementation. Codex must not
initialize Git unless explicitly authorized and must not connect to GitHub or
any other remote service unless explicitly authorized.

Local mode requires no branch, commit, push, pull request, merge, commit hash,
or CI evidence. Every task creates:

```text
docs/codex/reports/<TASK-ID>-local-execution-report.md
```

The report records every changed file and every executed command with its exact
result. Completion is determined from repository artifacts, tests, reports, and
acceptance evidence. Local completion is not equivalent to merge, CI, review,
release, or deployment evidence, and the report must say so.

A later session proves a local task complete by applying the dependency checks
above: verify required artifacts, acceptance evidence, focused tests, completed
prerequisite regressions, the one-task stop boundary, and a deterministic
changed-file inventory. It must preserve unavailable-check disclosures. A later
Git import must preserve task reports and validation evidence, review every
listed file, and rerun required checks in the target repository.

## Future Git-backed repository flow

The workflow has three distinct locations:

1. A **local working copy** is where authorized edits and local checks occur.
2. A **test/development repository** is the integration location for task
   branches, pull requests, CI, staging evidence, and review.
3. The **canonical main repository** is the protected source for releases. Its
   protected `main` accepts only reviewed, check-passing changes through the
   approved promotion flow.

Do not assume both repositories share an account, platform, remote name, or
credential. GitHub-compatible pull requests are expected, but equivalent
protected-branch behavior on another Git platform is acceptable.

### Branch and base rules

- Start from the task block's named branch when it is valid. Otherwise use
  `codex/<task-id-lower>-<short-kebab-slug>`.
- Verify the intended remote/repository, protected base, and exact base commit
  before branching.
- The working tree must be clean before task work; preserve and resolve any
  pre-existing user change before proceeding.
- Use one roadmap task per branch and one scoped change set per task.
- Never work directly on protected `main` and never bypass its protections.
- All dependencies must already be merged into the selected base.

### Commit rules

Use Conventional Commits with one of:

- `feat` for a user- or operator-visible capability;
- `fix` for defect correction;
- `refactor` for behavior-preserving restructuring;
- `test` for test-only changes;
- `docs` for documentation-only changes;
- `chore` for maintenance not covered elsewhere;
- `build` for build/dependency mechanics;
- `ci` for CI configuration;
- `perf` for measured performance improvement; or
- `revert` for an explicit prior-change reversal.

Format: `<type>(<optional-scope>): <imperative summary>`. Include the task ID in
the body or footer when it is not evident from branch/PR metadata. One
Conventional Commit is the default. Multiple commits are permitted only when
they provide necessary reviewable stages, separate generated artifacts, or
address review feedback; every commit must be coherent and pass required
pre-push checks. Squash before merge when intermediate commits add no lasting
review value.

### Push, pull request, review, and merge

Push is permitted only after dependencies are merged, local acceptance criteria
and required tests pass, no unresolved P0/P1 issue was introduced, a secret
scan finds no secret, documentation is current, rollback impact is understood,
and all review comments that already exist are resolved. A failing or unavailable
required local check blocks push unless the designated human owner explicitly
accepts the limitation in repository evidence.

Every pull request must use the repository template and identify task ID,
dependencies, scope, acceptance evidence, exact commands/results, coverage,
migration/security/financial/contract impacts, rollback, documentation, and
unresolved risks. CI must run the proportional lint, typecheck, build, unit,
integration, E2E, migration, contract, security, and evidence checks selected by
the task. Required human/code-owner/security/financial review follows ownership
rules; the author cannot self-declare an unavailable approval.

Merge is permitted only when:

- all dependencies are merged;
- every acceptance criterion and required test passes;
- all required CI checks pass on the final commit;
- no unresolved P0/P1 issue was introduced;
- secret scanning is clear;
- documentation is updated;
- rollback impact and recovery steps are understood;
- all review comments are resolved; and
- required approvals are present.

Squash merge is the default for one-task branches. A reviewed merge commit may
be used for approved release branches or when preserving a necessary commit
sequence. Rebase merge is allowed only when it preserves required provenance
and policy. Direct production changes and direct pushes to protected `main` are
prohibited.

Release branches or signed tags may be created only by an approved release task
after the relevant phase/launch gates pass. A test-repository merge is not
automatic authorization to promote or deploy from the canonical repository.

### Git rollback and recovery

Understand rollback before push. Prefer a new reviewed `revert` commit for a
merged change; do not rewrite protected history. For stateful changes, follow
the migration/contract recovery plan and use compensating forward changes when
down migration would lose data or violate compatibility. A failed deployment
rolls back to a known compatible artifact/configuration only when its data and
contracts remain compatible. Record incident, restore point, replay/reconcile
requirements, and verification evidence.

## Task-completion checklist

Before marking a task complete:

- [ ] Only the selected task was implemented; later tasks were not started.
- [ ] Every changed file is expected and within scope.
- [ ] Implementation summary and policy/ADR mapping are complete.
- [ ] Required tests/checks ran, with exact results and limitations recorded.
- [ ] Acceptance criteria have explicit PASS/FAIL evidence.
- [ ] Documentation, contracts, migrations, runbooks, and ADRs are updated when
      their subject changed.
- [ ] Known untested behavior and unresolved risks are explicit.
- [ ] Rollback/recovery notes are actionable and proportional.
- [ ] No secret, unauthorized dependency, policy contradiction, or unrelated
      refactor is present.
- [ ] Delivery evidence matches the active mode; no Git/CI/PR/deployment claim
      is fabricated.
- [ ] Paid-production status is stated.
- [ ] The required task report is written, then the invocation stops.

## Required task report

Every implementation report contains these sections:

1. Task ID and dependency verification.
2. Execution mode: local or Git-backed.
3. Files changed.
4. Implementation summary.
5. Policy and ADR mapping.
6. Tests added or updated.
7. Every exact command and result, including exit status and warnings.
8. Coverage impact where applicable.
9. Acceptance-criteria checklist.
10. Known untested behavior.
11. Remaining risks and unresolved process/technical ambiguity.
12. Rollback notes.
13. Confirmation that later tasks were not started.
14. Paid-production status.
15. Git evidence only when Git-backed mode was actually active.

Reports must never fabricate commit hashes, branches, PR URLs, review or merge
status, CI results, test results, coverage, release tags, deployment results, or
approvals. A required implementation summary and unresolved-risk section may
state ?none identified? only after the corresponding review was performed.

## Phase-controller and phase-gate behavior

Phase prompts are reusable controllers, not one-time task prompts. On each
invocation the controller must:

1. inspect completed task artifacts, reports, tests, and mode-appropriate
   delivery evidence;
2. identify the first incomplete task whose dependencies are satisfied;
3. implement exactly that task;
4. write its report and stop;
5. run the Phase Gate only in a later gate-only invocation after every phase
   task is complete in the active evidence mode; and
6. never start the next phase automatically.

A task invocation never performs the Phase Gate unless the invocation explicitly
selects gate-only work. The gate runs full phase evidence and emits explicit
`PASS` or `FAIL`; it never treats an unavailable check as a pass.

After `FAIL`, use the
[failed-gate remediation prompt](prompts/13_FAILED_GATE_REMEDIATION.md). It
creates the smallest independently reviewable remediation tasks from exact
failed evidence. It must not implement remediation during the planning
invocation, weaken the gate, delete tests, change policy, or claim the phase
passes. After remediation tasks complete, rerun the gate in a separate
invocation. A passing gate authorizes consideration of the next phase; it does
not start it.

## Protected production and security behavior

Codex must not independently:

- approve legal or jurisdiction requirements;
- approve Market Data licensing or redistribution rights;
- approve financial launch risk;
- deploy paid production without an explicitly authorized launch task and all
  prior gates passing;
- print, store in reports, or expose production secrets, credentials, tokens,
  OTPs, private keys, or unredacted sensitive records;
- bypass human approval for staged launch;
- change Wallet balances or Settlement outcomes outside approved, audited
  tooling; or
- weaken security, reconciliation, restore, migration, or launch gates to meet
  a schedule.

Security-sensitive findings must not be placed in a public issue when disclosure
could create risk. Stop, avoid secret or exploit reproduction disclosure, and
use the repository owner's approved private security-reporting channel. If no
private channel is configured, record only that coordination is required and
request it from the owner.

Paid production remains `NO-GO` until the roadmap's launch qualification and
human approvals explicitly change that status.
