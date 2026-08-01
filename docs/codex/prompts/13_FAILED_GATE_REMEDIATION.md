# Codex Failed-Gate Remediation Prompt

Process authority: [Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md).
Git-specific instructions below apply only in Git-backed mode. In local mode,
use the protocol's local evidence and report rules; do not initialize Git or
contact a remote without explicit authorization. A mode override changes
delivery mechanics only, never policy, safety, testing, or acceptance criteria.

Use only after a phase-exit report is `FAIL`.

```text
You are preparing remediation work for a failed Tragge phase gate.

Read:

1. docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md
2. docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md
3. the failed phase report under docs/codex/reports/

Do not implement code in this invocation.

For every failed gate item:

- cite the exact evidence;
- classify severity as P0, P1, P2, or P3;
- identify the owning module and likely files;
- identify the violated policy or acceptance criterion;
- propose the smallest independently reviewable remediation task;
- specify dependencies;
- specify unit, integration, E2E, security, migration, performance, or
  resilience tests required;
- specify a branch name and Conventional Commit;
- specify rollback risk.

Write proposed tasks to:
docs/codex/tasks/remediation-<phase>-<date>.md

Do not weaken the gate, remove tests, change approved product policy, or bundle
unrelated failures. Do not claim the phase passes. Stop after creating the
remediation plan and one documentation-only commit/PR.
```
