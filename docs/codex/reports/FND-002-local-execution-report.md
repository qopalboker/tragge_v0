# FND-002 local execution report

**Task:** `FND-002` - Record the target architecture ADR

**Execution mode:** Extracted local archive; Git delivery requirements waived

**Execution date:** 2026-07-25

**Task result:** **COMPLETE LOCALLY WITH RECORDED LIMITATIONS**

**Paid-production result:** **NO-GO (unchanged)**

This report is completion evidence for FND-002 only. It does not implement or
start `FND-003`.

## 1. Selected task and dependency verification

The selected task is `FND-002`. Its dependency is `FND-001`.

Dependency evidence:

- The [FND-001 local execution report](FND-001-local-execution-report.md) marks
  the dependency complete under the same local-execution override.
- All eight files listed by that report exist.
- `node scripts/production-baseline.test.mjs` passed 5 of 5 tests both before
  and after the FND-002 implementation.
- `node scripts/production-baseline.mjs verify` passed before and after the
  change: inventory was reproducible, all 35 P0/P1 findings retained resolving
  evidence, 147 checked FND-001 links resolved, and toolchain declarations
  remained compatible. Its three existing CI patch-pin warnings remain.
- The archive comparison found all eight expected FND-001 changes, all expected
  FND-002 changes, no missing original file, and no unexpected change.

Authoritative sources were read before implementation:

- [Fixed Product and Technical Policies](../../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md),
  version `2026-07-25.1`, SHA-256
  `71242471394A18452BA4F3F01EFF6373631881A9F3BAA29DA39F2E5FF05FDC75`.
- [Production Roadmap and Codex Tasks](../PRODUCTION_ROADMAP_AND_CODEX_TASKS.md),
  version `2026-07-25.1`, SHA-256
  `DE59B33A618C44D2681A72CB5178DA22F62B2DCE43C4D62B51476DDBB5E5004F`.
- [Phase 0 controller](../prompts/01_PHASE_0_BASELINE.md), including the exact
  FND-002 block.

## 2. Branch name

Not applicable under the local-execution override. No `.git` directory exists,
no repository was initialized, and no branch was created.

The roadmap's nominal branch is
`codex/fnd-002-record-the-target-architecture-adr`; it was recorded but not
used.

## 3. Files changed

| File | Change |
|---|---|
| [`docs/adr/0001-target-runtime-architecture.md`](../../adr/0001-target-runtime-architecture.md) | Added the Accepted target architecture decision, diagram, dependency and ownership rules, wrapper retirement, rejected alternatives, migration, and rollback principles. |
| [`docs/architecture/target-architecture-import-review.md`](../../architecture/target-architecture-import-review.md) | Added reproducible current-import evidence and rule-by-rule target conformance classification. |
| [`scripts/target-architecture.test.mjs`](../../../scripts/target-architecture.test.mjs) | Added four dependency-free focused architecture-document and import-boundary tests. |
| [`docs/codex/reports/FND-002-local-execution-report.md`](FND-002-local-execution-report.md) | Added this local task report. |

No application source, package manifest, dependency, lockfile, schema,
migration, runtime configuration, deployment manifest, API contract, or
product behavior was changed.

## 4. Implementation summary

- Accepted exactly three backend bounded systems: Platform modular monolith,
  Trading Engine, and Market Data Service.
- Fixed Platform as one codebase, image, and release version with `api`,
  `realtime`, and `worker` modes.
- Added an unambiguous topology diagram distinguishing clients/infrastructure
  from the three backend systems.
- Fixed `platform`, `engine`, and `market_data` schema/role ownership and
  explicitly prohibited all cross-system SQL and shared runtime credentials.
- Limited cross-system interaction to versioned commands/events with the fixed
  metadata envelope and transactional outbox/inbox/idempotency rules.
- Defined inward source dependencies, Platform in-process interfaces, shared
  package constraints, deployment/failure boundaries, and Redis's
  non-authoritative role.
- Recorded why `api-server`, `trading-core`, and `worker` are transitional and
  how each must be retired.
- Added staged migration and rollback principles, seven rejected alternatives,
  positive consequences, and unresolved costs.
- Reviewed 451 current internal Go import lines and 168 distinct module pairs.
  Ten cross-module app edges are present, all in the three wrappers; no shared
  package imports an application.

## 5. Important decisions and policy mapping

| Decision | Policy/task mapping |
|---|---|
| Exactly three bounded systems | Fixed policy section 2.1 and the FND-002 required implementation. |
| Platform modes share one image/version | Fixed policy section 2.2. |
| Platform modules use in-process interfaces | Fixed policy section 2.3; direct intra-Platform HTTP is forbidden. |
| Cross-system commands/events use fixed envelope metadata and outbox/inbox | Fixed policy section 2.3 and FND-002. |
| Runtime roles own only `platform`, `engine`, or `market_data` schemas | Fixed policy section 2.4 and FND-002. |
| Cross-system SQL and shared application credentials are forbidden | Fixed policy section 2.4; the ADR makes enforcement implications explicit. |
| Redis remains non-authoritative | Fixed policy sections 2.3 and universal Phase 0 safety rules. |
| Engine and Market Data remain separate processes/images/deployments/failure domains | Fixed policy section 2.1. |
| Initial topology remains Docker Compose on one server | Fixed policy section 17.3; Kubernetes is a rejected initial-launch alternative. |
| Migrations maintain one source of truth and rollback-compatible contracts | FND-002 acceptance criteria; no FND-004 reset strategy was implemented. |
| No dependency was added | Repository delivery policy. Tests use Node built-ins and the existing baseline helper. |
| No Git delivery action was taken | User's local-execution override supersedes only branch/commit/push/PR/merge mechanics. |

## 6. Tests added or updated

Added four focused tests in
[`scripts/target-architecture.test.mjs`](../../../scripts/target-architecture.test.mjs):

1. ADR status is Accepted, exactly three bounded systems are in the decision
   table, and all three Platform modes are present.
2. Required schema ownership, envelope, outbox/inbox, diagram, wrapper,
   migration, rollback, and rejected-alternative content exists.
3. The FND-002 Markdown files have no tabs/trailing whitespace and all local
   links resolve.
4. The live Go import graph matches the documented ten cross-module wrapper
   edges, 451 internal import lines, 168 distinct pairs, and zero package-to-app
   edges.

Final direct result: **4 passed, 0 failed**.

The existing FND-001 suite also passed: **5 passed, 0 failed**. The first
FND-002 test run had **3 passed, 1 failed** because the section extractor
assumed LF-only input while the files use CRLF. The test was corrected to
accept `CRLF` and `LF`, then rerun successfully; no production file was
involved.

## 7. Every command executed and exact result

Commands ran from the extracted project root unless noted. Long inline
PowerShell scripts and unified file contents are named here by operation rather
than duplicated in full.

| # | Command or operation | Exact result |
|---:|---|---|
| 1 | `Get-Content -Raw` for the Phase 0 prompt and both authoritative files; extract the exact FND-002 block; `Get-FileHash -Algorithm SHA256`. | Exit `0`; prompt/task read, policy length 32,781 characters and roadmap length 179,139; hashes match section 1. |
| 2 | First FND-001 artifact/hash evidence PowerShell pipeline. | PowerShell parser failed with `An empty pipe element is not allowed`; no repository change. |
| 3 | Corrected FND-001 artifact/hash evidence, report search, documentation listing, and ADR-reference search. | Exit `0`; all eight dependency files exist; no prior `docs/adr` directory or target ADR existed. |
| 4 | List policy/audit headings and read the complete FND-001 report. | Exit `0`; dependency report and relevant navigation evidence read. |
| 5 | Read target-architecture and infrastructure policy sections, current topology, app modules, wrapper file lists, and run an initial `rg` import expression. | Exit `1` because the final `rg` expression had an unclosed character class; preceding reads succeeded and the invalid import output was discarded. |
| 6 | Read all three wrapper entrypoints and run an initial PowerShell import classifier using `System.IO.Path.GetRelativePath`. | PowerShell returned exit `0` but emitted repeated `MethodNotFound`/null errors because that runtime lacks `GetRelativePath`; classifier output was invalid and discarded. Wrapper reads succeeded. |
| 7 | First corrected import-classifier attempt. | PowerShell parser failed with `An empty pipe element is not allowed`; no change. |
| 8 | Final import classifier using a validated root-prefix substring. | Exit `0`; 451 internal lines, 168 distinct pairs, 13 app-to-app pairs including three self-pairs, ten cross-module wrapper edges, zero package-to-app pairs. |
| 9 | `node scripts/production-baseline.test.mjs`; `node scripts/production-baseline.mjs verify`; `.git`/prior-report/ADR checks. | Exit `0`; 5 tests passed; baseline verifier passed with three pre-existing CI patch-pin warnings; `.git`, FND-002 report, and ADR were absent. |
| 10 | Inspect baseline helper exports, its focused test, and root package scripts. | Exit `0`; existing Markdown validator was suitable and no new dependency was needed. |
| 11 | Built-in `apply_patch` attempt to add ADR, import review, and test. | Failed to write the ADR; no file changed. |
| 12 | `New-Item -ItemType Directory -Path docs/adr -Force`. | Exit `0`; only the missing directory was created; no ADR file yet. |
| 13 | Built-in `apply_patch` retry for the ADR. | Failed to write; no file changed. |
| 14 | First `git apply --no-index --whitespace=error-all -` ADR fallback. | Exit `128`; rejected the final patch line as trailing whitespace; no file changed. |
| 15 | `git apply --no-index --whitespace=nowarn -` for the ADR. | Exit `0`; 353-line ADR created; `.git` remained absent. The command was used only as a unified-diff writer. |
| 16 | Same no-index diff operation for the import review. | Exit `0`; 95-line review created; `.git` remained absent. |
| 17 | Same no-index diff operation for the targeted test. | Exit `0`; 169-line test created; `.git` remained absent. |
| 18 | `node --check scripts/target-architecture.test.mjs`; first `node scripts/target-architecture.test.mjs`. | Syntax check exit `0`; test command exit `1` with 3 passed and 1 CRLF-sensitive section-extraction failure. |
| 19 | Inspect the failing lines and count CRLF/LF endings in all three files. | Exit `0`; all three files consistently used CRLF and ended with LF. |
| 20 | First no-index correction patch without whitespace-ignore mode. | Exit `1`; patch did not apply; test file remained unchanged. |
| 21 | No-index correction with `--ignore-whitespace`; rerun syntax and targeted tests. | Exit `0`; correction applied, syntax passed, 4 tests passed and 0 failed. |
| 22 | Probe Markdown linter; run target syntax/tests, FND-001 tests/verifier, FND-002 link validation, and `.git` guard. | Exit `0`; no standalone `markdownlint`/`markdownlint-cli2` is installed; focused style/link check passed; 4/4 target tests and 5/5 baseline tests passed; 19 FND-002 links resolved; `.git` absent. Baseline verifier retained three CI patch-pin warnings. |
| 23 | Open the original ZIP and list root samples/current root entries. | Exit `0`; archive root is `tragge-main/`; local project root and original ZIP were suitable for comparison. |
| 24 | SHA-256 compare all current files against `D:\tragge-codex\tragge-main.zip`, classifying the eight FND-001 and three then-created FND-002 files. | Exit `0`; 3 modified, 8 added, 0 original missing, all 8 FND-001 and 3 FND-002 files present, 0 unexpected, 0 expected missing, `.git` absent. |
| 25 | No-index unified-diff operation creating this report. | Exit `0`; report created; no Git metadata initialized. |
| 26 | Final syntax, targeted/baseline tests, all FND-002 Markdown-link validation, archive-scope comparison including this report, and `.git` guard. | Exit `0`; 4/4 target and 5/5 baseline tests passed; baseline verification passed with the three recorded warnings; all FND-002 links resolved; scope was 3 modified plus 9 added files since the ZIP (8 FND-001 and 4 FND-002), with 0 missing/unexpected files; `.git` absent. |

## 8. Coverage change for affected critical modules

No application or critical domain module changed. Numeric code coverage is
therefore not applicable and no coverage increase/decrease is claimed. The new
document/import contract has four focused functional tests, but no numeric
coverage claim is made for the test script.

## 9. Acceptance-criteria checklist

- [x] ADR status is Accepted.
- [x] Exactly three backend bounded systems are fixed.
- [x] Platform `api`, `realtime`, and `worker` modes share one image/version.
- [x] Architecture diagram is explicit and unambiguous.
- [x] Source dependency rules are explicit and reviewed against current imports.
- [x] `platform`, `engine`, and `market_data` schema/credential ownership is fixed.
- [x] Allowed in-process and cross-system communication is fixed.
- [x] Event metadata and transactional outbox/inbox/idempotency are fixed.
- [x] Cross-system SQL and authoritative Redis use are forbidden.
- [x] `api-server`, `trading-core`, and `worker` are documented as transitional
  with required retirement dispositions.
- [x] Migration and rollback principles are present.
- [x] Rejected alternatives and migration consequences are present.
- [x] Focused Markdown/import tests pass.
- [x] Local Markdown links and referenced repository paths resolve.
- [x] No application behavior or dependency changed.
- [x] Git delivery requirements were waived and no Git repository/action was
  created or performed.

## 10. Known untested behavior

- The Mermaid diagram was source-checked but not rendered because no Mermaid
  renderer is installed.
- No standalone Markdown linter is installed. The focused test checks tabs,
  trailing whitespace, required structure, and local link resolution; it does
  not claim a full markdownlint ruleset pass.
- Runtime schema grants, credentials, outbox/inbox semantics, message-version
  handling, process isolation, Compose topology, and rollback behavior are
  target decisions only; they are not implemented or integration-tested by
  this documentation task.
- No Go/frontend unit, integration, E2E, race, migration, lint, typecheck, or
  application build was run for FND-002 because no application module, schema,
  dependency, or runtime configuration changed. No such pass is claimed.

## 11. Remaining risks or blockers

- The current runtime still does not implement the Accepted architecture.
- Ten current cross-module app imports remain in the three wrappers by design;
  the wrappers still need staged retirement.
- Shared packages still require ownership classification.
- Schema roles/privileges and cross-system SQL denial are not enforced.
- Versioned envelopes and transactional outbox/inbox paths are not proven.
- Independent Engine and Market Data images, deployments, credentials,
  recovery paths, and failure tests remain future implementation work.
- Existing FND-001 P0/P1 findings and CI patch-pin warnings remain unresolved.
- Paid production remains **NO-GO**.

## 12. Commit hash

Not applicable. The local-execution override prohibited Git initialization and
commit creation. No commit exists. The nominal Conventional Commit message
would be `docs(adr): fix target runtime architecture`, but it was not executed.

## 13. Pull-request URL and merge status

Not applicable. No remote service was contacted, and no push, pull request, or
merge was attempted. Completion is tracked by the four files in section 3 and
this report's local evidence.

## Final outcome

FND-002 is complete under the local-execution override. The target architecture
is now an Accepted, test-backed decision, while implementation conformance
remains explicitly outstanding. `FND-003` was not started.
