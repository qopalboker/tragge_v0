# EXAMPLE-DOC-001 local protocol dry run

**Example status:** Fictional documentation-only task; not a roadmap task

**Dry-run validation status:** `PASS` ? focused protocol validation passed 11/11

**Execution mode:** Local extracted project

**Product/application effect:** None

This file demonstrates the FND-005 protocol. It does not create or complete a
real roadmap task, modify a real task report, create Git evidence, run a Phase
Gate, or authorize a later phase.

## 1. Task selection demonstration

Fictional candidates:

1. `EXAMPLE-DOC-001 ? Verify a documentation navigation example` has no
   dependency and is incomplete.
2. `EXAMPLE-DOC-002 ? Extend the example` depends on EXAMPLE-DOC-001.

The controller selects EXAMPLE-DOC-001 as the first incomplete candidate whose
dependency is satisfied. It does not combine EXAMPLE-DOC-002.

## 2. Dependency checking demonstration

- Declared dependencies: none.
- Authoritative inputs: fixed policy, roadmap, Accepted ADR-0001, canonical
  glossary, and the Codex execution protocol exist.
- Completion evidence is not inferred from a report claim.
- In a real local task, prerequisite artifacts, acceptance evidence, focused
  tests, completed-task regressions, later-task boundary, and changed-file
  inventory would all be checked before editing.

## 3. Goal, non-goals, and scoped-files demonstration

Goal: prove that the protocol can carry one documentation-only example from
selection through a local report structure.

Allowed file for the dry-run evidence:

- `docs/codex/examples/EXAMPLE-DOC-001-local-dry-run.md`

Forbidden: application code, schema, runtime configuration, policy decisions,
other reports, Git metadata, remote operations, EXAMPLE-DOC-002, and every real
roadmap task. An unexpected file would fail the dry run rather than expand it.

## 4. Documentation-only validation demonstration

Appropriate checks are structure, required terminology, local Markdown links,
repository paths, roadmap task IDs, mode rules, prompt stop conditions, and
policy/ADR consistency. Artificial application unit, integration, or E2E tests
are not required because no application behavior changes.

Observed dry-run validation:

- `node --check scripts/codex-execution-protocol.test.mjs`: exit `0`.
- `node scripts/codex-execution-protocol.test.mjs`: exit `0`; 11 passed, 0
  failed, including this fictional scenario.

The FND-005 report preserves the full executed-command evidence. No result was
recorded here until both commands actually completed.

## 5. Example final report structure

### Task ID and dependency verification

EXAMPLE-DOC-001 selected; no dependency. This ID is fictional and does not mark
any roadmap item complete.

### Execution mode

Local. `.git` is not required. Git initialization and remote access are not
authorized.

### Files changed

Only this dry-run evidence file is in the fictional scope. Its actual creation
belongs to FND-005, not to a separate completed task.

### Implementation summary

Demonstrated one-task selection, dependency checking, scoped files,
documentation-only checks, report fields, and stop-after-one behavior.

### Policy and ADR mapping

No product decision changed. The example preserves ADR-0001 boundaries and the
canonical glossary; it implements process evidence only.

### Tests added or updated

The FND-005 focused protocol validator checks this example. No artificial
application test is added.

### Exact commands and results

The real FND-005 report records the exact syntax, focused-test, regression, and
Markdown commands after execution. This example deliberately contains no
fabricated exit status, CI result, commit hash, branch, PR, merge, or deployment.

### Coverage impact

Not applicable: documentation-only process evidence changes no runtime module.

### Acceptance-criteria checklist

- [x] One fictional documentation goal selected.
- [x] Dependency state and authorities demonstrated.
- [x] Scope limited to one example file.
- [x] Appropriate structural validation identified.
- [x] No application or product change represented.
- [x] No real roadmap completion represented.

### Known untested behavior

This dry run does not exercise Git hosting, CI, review, merge, deployment, or
application/runtime behavior. It demonstrates their evidence rules only.

### Remaining risks and unresolved ambiguity

None for the fictional documentation scope. Real tasks must disclose their own
environment and implementation risks.

### Rollback notes

Remove this example only in a scoped documentation change after updating links
and focused validation. Do not discard unrelated files.

### Later-task boundary

EXAMPLE-DOC-002 was not started. No real roadmap task or next phase was started.

### Paid-production status

`NO-GO`; unchanged.

### Git evidence

Not applicable. Local mode created no branch, commit, push, PR, merge, CI, or
deployment evidence.

## 6. Stop-after-one-task demonstration

The fictional controller stops after the report structure above. It does not
select EXAMPLE-DOC-002, run a Phase Gate, or start another phase.
