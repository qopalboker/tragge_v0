# Roadmap task execution template

Use this template with the
[canonical Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md). Replace
all angle-bracket placeholders. Do not remove a field; use `None` or `Not
applicable` with a reason when appropriate.

## Task ID and title

`<TASK-ID> ? <imperative title>`

## Goal

<One independently verifiable outcome. This is the invocation's only goal.>

## Non-goals

- <Explicitly excluded adjacent outcome.>
- <Later roadmap task that must not be implemented early.>

## Dependencies

| Dependency | Required evidence | Verified result |
|---|---|---|
| `<TASK-ID or None>` | <Artifacts, report, focused tests, merge evidence when applicable> | <Exact evidence or blocker> |

A report claim alone is not completion evidence. Verify artifacts, acceptance
criteria, focused regressions, later-task boundary, and changed-file scope.

## Authoritative policies

1. [Fixed Product and Technical Policies](../../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md)
2. [Production roadmap](../PRODUCTION_ROADMAP_AND_CODEX_TASKS.md), complete selected task block
3. <Applicable Accepted ADRs, or `None`>
4. [Canonical glossary and version catalog](../../product/canonical-domain-glossary-and-version-catalog.md)
5. [Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md)

<Identify any conflict and the controlling source. Do not silently reinterpret
policy or architecture.>

## Execution mode

`<Local | Git-backed>`

<Identify selected project/repository. State whether Git/remote operations are
authorized.>

## Primary scope

- `<Directory/module>`

## Allowed files/modules

- `<Exact path or bounded glob>`
- Tests, documentation, and evidence required by this task only

## Forbidden scope

- Unrelated cleanup, formatting, mass rename, or broad refactor
- Any second or future roadmap task
- Dependency addition or upgrade without documented approval
- Product-policy or financial-rule change not explicitly authorized
- Silent architecture-boundary change
- Weakening, skipping, deleting, or quarantining tests to pass
- Claiming an unavailable or unexecuted check passed
- Files outside the allowed scope unless exact evidence proves they are required

## Required implementation

1. <Requirement and observable evidence.>
2. <Requirement and observable evidence.>

## Impact assessment

### Data/schema impact

<Schema, data lifecycle, backfill, consistency, ownership, or `None` with reason.>

### Security impact

<Authentication, authorization, secrets, privacy, abuse, or `None` with reason.>

### Financial impact

<Money, fees, ledger, settlement, reconciliation, or `None` with reason.>

### Contract/API impact

<REST, WebSocket, command/event, compatibility, versioning, or `None` with reason.>

### Migration impact

<Database/runtime/data migration and upgrade/rollback path, or `None` with reason.>

### Observability impact

<Logs, metrics, traces, audit, alerts, dashboards, or `None` with reason.>

## ADR decision

`<Required ? proposed ADR path and trigger | Not required ? reason>`

If required, include context, decision, alternatives, consequences, migration,
rollback/reversal cost, affected task IDs, status, date/version, and superseded
references.

## Dependency decision

`<No new dependency | Approval required>`

If approval is required, document insufficiency of existing tools, simpler
alternatives, maintenance/security/license impact, pinned version, smallest
scope, and removal/rollback implications. Stop if approval is absent.

## Rollback strategy

<Revert/compensating-forward/configuration/data-restore steps and verification.
For documentation-only work, identify how to restore the prior authoritative
links/process text without discarding unrelated changes.>

## Unit tests

- `<Command and expected evidence | Not applicable ? reason>`

## Integration tests

- `<Command and expected evidence | Not applicable ? reason>`

## E2E tests

- `<Command and expected evidence | Not applicable ? reason>`

## Regression tests

- `<Completed dependency and bug regressions>`

## Lint, typecheck, and build

- `<Touched-module command | Not applicable ? reason>`

## Verification commands

Run and report each exact command and result:

```text
<command>
```

Do not replace exit status, pass/fail counts, warnings, unavailable tools, or
runtime limitations with a summary claim.

## Acceptance criteria

- [ ] <Criterion with direct evidence.>
- [ ] Only this roadmap task was implemented.
- [ ] Every changed file is expected and within scope.
- [ ] Required checks passed or the task is explicitly reported `FAIL`.
- [ ] Known untested behavior and unresolved risks are disclosed.

## Documentation updates

- `<Canonical documentation, runbook, contract, ADR, or report path>`

## Known limitations

- <Unavailable environment/tooling and impact.>
- <Approved target behavior still planned rather than implemented.>

## Required final report

Use the protocol's 15-part report standard. In local mode, write:

```text
docs/codex/reports/<TASK-ID>-local-execution-report.md
```

The report must include dependency verification, execution mode, every changed
file, implementation summary, policy/ADR mapping, tests, exact commands and
results, coverage impact, acceptance checklist, known untested behavior,
remaining risks, rollback notes, later-task boundary, paid-production status,
and Git evidence only when Git-backed mode was actually active.

## Stop condition

Stop immediately after the selected task's report. Do not implement another
roadmap task, run a Phase Gate during a task invocation, or begin the next
phase. A failure produces honest `FAIL` evidence or a blocker; it never expands
scope or weakens acceptance criteria.
