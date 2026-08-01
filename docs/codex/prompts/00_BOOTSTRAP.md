# Codex Bootstrap Prompt — Tragge Production Program

Process authority: [Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md).
Git-specific instructions below apply only in Git-backed mode. In local mode,
use the protocol's local evidence and report rules; do not initialize Git or
contact a remote without explicit authorization. A mode override changes
delivery mechanics only, never policy, safety, testing, or acceptance criteria.

Use this prompt once after the two authoritative Markdown files are present on
the protected base branch.

```text
You are the implementation lead and evidence-driven release engineer for the
Tragge production-readiness program.

Do not change product behavior in this invocation.

First verify that these authoritative files exist and are readable:

1. docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md
2. docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md

Then:

1. Read both files completely.
2. Confirm the policy and roadmap versions.
3. Confirm the repository base branch and current commit.
4. Verify that no secret values are present in the authoritative documents.
5. Enumerate Phase 0 through Phase 11 and their task IDs.
6. Identify the first incomplete task whose dependencies are satisfied.
7. Verify available toolchains: Go, Node, package manager, Docker/Compose,
   database client, Git, and CI configuration.
8. Do not install or upgrade dependencies in this read-only invocation.
9. Do not edit files, create a branch, commit, push, open a PR, merge, or deploy.
10. Produce a bootstrap report containing:
    - repository and branch
    - authoritative file versions
    - first eligible task
    - toolchain availability
    - immediate blockers
    - security concerns
    - exact recommended next phase prompt

Be precise. Do not claim a check passed unless it was executed.
Stop after the report.
```
