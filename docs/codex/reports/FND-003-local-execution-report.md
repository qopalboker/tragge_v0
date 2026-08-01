# FND-003 local execution report

**Task:** `FND-003` - Create the canonical domain glossary and version catalog

**Execution mode:** Extracted local archive; Git delivery requirements waived

**Execution date:** 2026-07-25

**Task result:** **COMPLETE LOCALLY WITH RECORDED LIMITATIONS**

**Paid-production result:** **NO-GO (unchanged)**

This report is completion evidence for FND-003 only. It does not implement or
start `FND-004`.

## 1. Selected task and dependency verification

The selected task is `FND-003`. Its roadmap dependency is `FND-002`.

Verified local dependency evidence:

- The [FND-001 report](FND-001-local-execution-report.md) and
  [FND-002 report](FND-002-local-execution-report.md) both mark their task
  complete under the local-execution override.
- Required baseline, ADR, import-review, report, and validation files all exist.
- [ADR-0001](../../adr/0001-target-runtime-architecture.md) is Accepted.
- Before and after FND-003, the FND-001 focused suite passed 5 of 5 and the
  FND-002 focused suite passed 4 of 4.
- The FND-001 verifier still passes reproducible inventory, evidence links, and
  toolchain compatibility with its three existing CI patch-pin warnings.

Authoritative sources were read before implementation:

- [Fixed Product and Technical Policies](../../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md),
  version `2026-07-25.1`, SHA-256
  `71242471394A18452BA4F3F01EFF6373631881A9F3BAA29DA39F2E5FF05FDC75`.
- [Production Roadmap and Independent Codex Tasks](../PRODUCTION_ROADMAP_AND_CODEX_TASKS.md),
  version `2026-07-25.1`, SHA-256
  `DE59B33A618C44D2681A72CB5178DA22F62B2DCE43C4D62B51476DDBB5E5004F`.
- [Phase 0 controller](../prompts/01_PHASE_0_BASELINE.md).
- [ADR-0001](../../adr/0001-target-runtime-architecture.md).

## 2. Branch name

Not applicable under the local-execution override. No `.git` directory exists,
no repository was initialized, and no branch was created.

The nominal roadmap branch
`codex/fnd-003-create-the-canonical-domain-glossary-and-v` was recorded but not
used.

## 3. Files changed

| File | Change |
|---|---|
| [`docs/product/canonical-domain-glossary-and-version-catalog.md`](../../product/canonical-domain-glossary-and-version-catalog.md) | Added 61 canonical/deprecated term definitions, collision rules, stable identifier conventions, an 18-item current/planned/legacy version catalog, and a repository remediation register. |
| [`packages/contracts/README.md`](../../../packages/contracts/README.md) | Marked existing `v1` as legacy compatibility rather than target approval and corrected target terminology for fixed-point prices, Trading QTY, Platform Fee, and absent Participant Capacity. |
| [`scripts/domain-glossary.test.mjs`](../../../scripts/domain-glossary.test.mjs) | Added eight dependency-free focused glossary, version, path, task-ID, policy, ADR, Second Chance, and Markdown tests. |
| [`docs/codex/reports/FND-003-local-execution-report.md`](FND-003-local-execution-report.md) | Added this report. |

No application source, executable contract/schema, package manifest, dependency,
lockfile, migration, runtime configuration, deployment file, infrastructure, API
behavior, product rule, or financial formula changed.

## 4. Implementation summary

- Defined 61 canonical terms exactly once across architecture, contest lifecycle,
  participation/QTY, economics/prize/scoring, wallet/settlement, market data,
  messaging/durability, KYC, deposits, and withdrawals.
- Included every term requested by the invocation and the roadmap-required Real
  Participant, Economics Lock, and Settlement Review terms.
- Explicitly separated Trading QTY from people and prohibited unqualified
  `quantity` at domain/UI boundaries.
- Marked product Participant Capacity, Gross Prize, and Contest-economics
  `commission_rate` as deprecated/noncanonical with explicit migration targets.
- Fixed T-Score as Engine performance/ranking score and Reward Weight as a
  separate `tralent_v1` prize-distribution value.
- Fixed Leaderboard Projection as a read model and Settlement as the only final
  completion/payout owner.
- Defined System Participant as a non-real, rank-zero, non-prize/economics/
  Official Ranking participant in Free Practice only.
- Kept Second Chance removed and added a test that scans `apps/` and `packages/`
  for active references.
- Added stable identifier conventions without fabricating unassigned versions.
- Cataloged 18 versioned items as current, planned, or legacy/noncanonical.
  Only identifiers already assigned by authority are used: `2026-07-25.1`,
  `ADR-0001`, `tralent_v1`, and planned Market Data `v2`.
- Linked every unassigned planned artifact to the roadmap task responsible for
  assigning/implementing it.
- Recorded representative current conflicts for `max_participants`,
  `commission_rate`, status-only registration, legacy scoring names,
  leaderboard finalization, system-account exclusion, Power Law/noncanonical
  prize fixtures, and float/incomplete `v1` contracts.

## 5. Important decisions and policy mapping

| Decision | Source mapping |
|---|---|
| Exactly three architecture terms and three Platform modes | Fixed policy section 2 and Accepted ADR-0001. |
| Participant count and Trading QTY are different domains | Fixed policy sections 5.2 and 5.5; product capacity does not exist. |
| Base Fee/Platform Fee/Surcharge/Prize Pool definitions remain 20/10/80 policy | Fixed policy section 4; no new formula introduced. |
| Platform Fee uses `platform_fee_bps = 2000` | Fixed policy section 4.2; `commission_rate` is deprecated for this domain. |
| Planned/actual winners, eligibility, rank bands, Reward Weight, and exact payout use `tralent_v1` terminology | Fixed policy section 11. |
| T-Score is not Reward Weight | Fixed policy section 10 plus section 11.5 naming rule and the invocation's collision requirement. |
| Settlement owns finalization; Leaderboard is a projection | Fixed policy section 12 and ADR-0001. |
| System Participant is not real or prize/official-ranking eligible | Fixed policy section 6.1. |
| Market Data terminology and planned `v2` | Fixed policy section 9; `MD-001` explicitly assigns the planned `v2`. |
| Unknown target versions remain `Not assigned (planned)` | FND-003 invocation prohibits fabricated schema/contract numbers. |
| Existing contract `v1` remains legacy evidence | Current package inspection; it lacks required metadata and uses legacy floats. No executable contract was changed. |
| Clean database baseline remains unassigned | `FND-004` owns that decision and was not started. |
| No new dependency | FND-003 validation uses only Node built-ins and the existing baseline helper. |

## 6. Tests added or updated

Added eight focused tests in
[`scripts/domain-glossary.test.mjs`](../../../scripts/domain-glossary.test.mjs):

1. All 61 required/canonical/deprecated terms exist exactly once with substantive
   definitions.
2. Collision rules and fixed financial meanings remain explicit.
3. Second Chance appears only as removed/prohibited and has no active
   `apps/`/`packages/` reference.
4. All 18 catalog entries have the expected current/planned/legacy state and
   only authoritative assigned IDs.
5. Every referenced roadmap task ID and local Markdown path exists.
6. Architecture names/modes agree with Accepted ADR-0001.
7. Financial terminology agrees with fixed policy and the contracts README no
   longer blesses legacy float prices as target.
8. FND-003 Markdown has no tabs or trailing whitespace.

Final result: **8 passed, 0 failed**.

Existing regression results:

- FND-001 focused suite: **5 passed, 0 failed**.
- FND-001 verifier: **PASS**, with the same three non-failing CI patch-pin
  warnings for Go, Node, and pnpm.
- FND-002 focused suite: **4 passed, 0 failed**.
- Existing Go contracts module: `go test ./...` passed; `go vet ./...` exited
  `0` with two sandbox-denied Go telemetry upload-token warnings.

No artificial application unit test was added.

## 7. Every command executed and exact result

Commands ran from the extracted project root unless a subdirectory is stated.
Long inline PowerShell classifiers and unified file contents are named by
operation rather than reproduced in full.

| # | Command or operation | Exact result |
|---:|---|---|
| 1 | Read the full Phase 0 prompt, fixed policy, roadmap, extract exact FND-003 block, and SHA-256 hash sources. | Exit `0`; full reads succeeded; policy/roadmap hashes match section 1; FND-003 block located. |
| 2 | Read full ADR-0001; verify both prior reports/artifacts; inventory `docs/product` and `packages/contracts`. | Exit `0`; reports/artifacts exist, ADR is Accepted, and current contract files were listed. |
| 3 | Run FND-001 tests/verifier and FND-002 syntax/tests; check `.git` and FND-003/FND-004 report absence. | Exit `0`; 5/5 and 4/4 passed; baseline verifier passed with three existing warnings; `.git` and later reports absent. |
| 4 | List roadmap task headings, search version/contract responsibility, and read Contest/Scheduler policy sections. | Exit `0`; all task IDs and lifecycle/template rules inspected. |
| 5 | Read contract README, schema metadata, prize fixture, and version/deprecated-field declarations. | Exit `0`; found legacy `v1`, float guidance, `tralent_like_v1`, `commission_rate`, and `max_participants` evidence. |
| 6 | First policy-term/collision-count PowerShell group. | Parser failed with `An empty pipe element is not allowed`; no change. |
| 7 | Corrected term/collision search. | Exit `0`; term locations and representative file counts returned. |
| 8 | Search Second Chance, fee/capacity/T-Score, leaderboard finalization, and system-account evidence. | Exit `0`; Second Chance references were prohibitions only; current remediation targets were found. |
| 9 | Extract complete roadmap blocks for FND-004, ARCH-006, DATA/CON/PRIZE/ENG/MD/FE/OPS owner tasks and search API contract assignments. | Exit `0`; responsible tasks and the explicit planned Market Data `v2` were verified. |
| 10 | Read REST/API task context and Platform/FE task summaries. | Exit `0`; `FE-001` owns generated trading REST/WS types and ARCH tasks own Platform API migration/versioning. |
| 11 | Search symbol-registry/version identifiers and read roadmap header. | Exit `0`; no symbol-registry version is assigned; policy/roadmap are both `2026-07-25.1`. |
| 12 | Extract DATA-005, ENG-002, ARCH-001..005, and FE-001 blocks. | Exit `0`; glossary and catalog task ownership verified. |
| 13 | Search exact T-Score and capacity locations and hash representative contract files. | Overall exit `0`; the middle `rg apps/*-frontend/src` Windows wildcard was invalid and its output discarded; exact T-Score and hash commands succeeded. |
| 14 | Search lifecycle and prize legacy targets. | Exit `0`; status-only `registration_open`, Power Law, and noncanonical fixture targets found. |
| 15 | Built-in `apply_patch` add-file probe for the glossary. | Failed to write; no file changed. |
| 16 | `git apply --no-index --whitespace=nowarn -` unified-diff fallback for the glossary. | Exit `0`; 234-line file created; `.git` remained absent. Used only as a diff writer. |
| 17 | Read numbered contract README lines. | Exit `0`; exact documentation edit context inspected. |
| 18 | First no-index README update patch. | Exit `128` with `corrupt patch at line 19`; no change. |
| 19 | Corrected no-index README patch with validated hunk counts. | Exit `0`; legacy/target clarification applied; `.git` remained absent. |
| 20 | No-index unified-diff operation adding `scripts/domain-glossary.test.mjs`. | Exit `0`; 322-line focused test created; `.git` remained absent. |
| 21 | `node --check scripts/domain-glossary.test.mjs`; `node scripts/domain-glossary.test.mjs`. | Exit `0`; syntax passed and 8 tests passed, 0 failed. |
| 22 | First contract manifest/tool/file-summary PowerShell group. | Parser failed with `An empty pipe element is not allowed`; no change. |
| 23 | Corrected manifest/tool/file-summary group. | Exit `0`; Go/Node/pnpm present, standalone Markdown linters absent, touched hashes/line counts captured. |
| 24 | Read contract tests and run a Go import `rg`. | Overall exit `1` because final `rg` found no matching quoted-import lines; contract test read succeeded. |
| 25 | First contract `go test ./...`/`go vet ./...` group using `C:\tmp` cache. | Exit `1` before compile: sandbox denied the cache directory and Go telemetry token; no Go test result claimed. |
| 26 | Retry from `packages/contracts` with a writable sibling-work cache, `GOTOOLCHAIN=local`, and telemetry disabled. | Exit `0`; `go test ./...` passed (`contracts/v1`, 0.479s); `go vet ./...` exited `0` and emitted two telemetry upload-token access warnings. |
| 27 | Read `packages/contracts/ts/package.json` and `pnpm-workspace.yaml`. | Exit `0`; TS package has no lint/typecheck/build scripts. |
| 28 | Probe Markdown linter; run FND-003 syntax/tests, FND-001 tests/verifier, FND-002 tests, FND-003 link check, and local guard. | Exit `0`; no standalone linter; 8/8, 5/5, and 4/4 passed; verifier passed with three warnings; 39 links resolved; `.git` absent and no FND-004 report. |
| 29 | SHA-256 compare project against original ZIP with FND-001/002/003 classification. | Exit `0`; 4 modified, 11 added, 0 original missing; expected 8/4/3 task files present; 0 unexpected/missing; `.git` absent. |
| 30 | No-index unified-diff operation creating this report. | Exit `0`; report created; no Git metadata initialized. |
| 31 | First combined final-verification and recursive sibling-cache-cleanup request. | Rejected before execution because destructive-operation approval is unavailable; no subcommand in the group ran. |
| 32 | Inspect report lines 187-241 after the blocked request. | Exit `0`; acceptance/limitations text read, but command-table row 31 was outside that range. |
| 33 | Locate command rows 28-31 and scratch-cache text with `Select-String`. | Exit `0`; exact stale row 31 located at line 182. |
| 34 | First no-index command-log correction attempt. | Exit `128` with `corrupt patch at line 14`; no file changed. |
| 35 | Second no-index command-log correction attempt. | Exit `128` with `corrupt patch at line 15`; no file changed. |
| 36 | Third no-index command-log correction attempt. | Exit `128` with `corrupt patch at line 16`; no file changed. |
| 37 | Fourth no-index command-log correction attempt. | Exit `128` with `corrupt patch at line 17`; no file changed. |
| 38 | Zero-context no-index command-log correction attempt. | Exit `1`; patch did not apply at report line 182; no file changed. |
| 39 | First generated full-file no-index diff attempt. | Exit `128` with `patch with only garbage at line 3` because header strings were malformed; no file changed. |
| 40 | Corrected generated full-file no-index diff updating this command log and cache limitation. | Exit `0`; rows 31-41 and the scratch-cache limitation corrected; `.git` remained absent. |
| 41 | Final FND-003/prior regression/contract/Markdown validation, archive scope including report, and local guard, without deletion. | Exit `0`; 8/8, 5/5, and 4/4 passed; contract test/vet passed with recorded telemetry warnings; all FND-003 links resolved; scope was 4 modified plus 12 added since ZIP (8 FND-001, 4 FND-002, 4 FND-003), 0 missing/unexpected; `.git` absent and FND-004 absent. |

## 8. Coverage change for affected critical modules

No application or critical domain implementation changed, so numeric code
coverage is not applicable and no increase/decrease is claimed. The new
document validator has eight focused tests. The existing contracts Go tests were
run for regression confidence, not because executable contracts changed.

## 9. Acceptance-criteria checklist

- [x] One canonical definition exists for every required term.
- [x] Real Participant, System Participant, economics, eligibility, prize,
  Settlement Review, Trading QTY, and Asset Group terms satisfy the FND-003 block.
- [x] `quantity` cannot ambiguously mean participants and Trading QTY.
- [x] Product-level Participant Capacity is explicitly absent/deprecated.
- [x] T-Score and Reward Weight are explicitly different.
- [x] Platform Fee uses `platform_fee_bps`; `commission_rate` has a migration target.
- [x] Leaderboard Projection has no Settlement authority.
- [x] System Participant is not real or prize/Official Ranking eligible.
- [x] Second Chance appears only as removed/prohibited and no active code was found.
- [x] Version catalog covers all invocation-required documents, rules, snapshots,
  event envelopes, REST/WS contracts, and database baseline.
- [x] Every unimplemented version is marked planned and references owner task IDs.
- [x] No undocumented schema/contract number was fabricated.
- [x] Current legacy behavior and approved target behavior are distinct.
- [x] Stable identifier formats are defined.
- [x] Deprecated terms have explicit migration targets.
- [x] Repository terminology conflicts and remediation paths/tasks are listed.
- [x] Every referenced local path and roadmap task ID resolves.
- [x] Financial terminology agrees with fixed policy.
- [x] Architecture terminology agrees with ADR-0001.
- [x] Focused and prior regression checks pass.
- [x] No application behavior, infrastructure, executable contract, or dependency changed.
- [x] FND-004 was not started.

## 10. Known untested behavior

- No standalone `markdownlint`/`markdownlint-cli2` is installed. The focused
  validator checks required structure, consistency rules, tabs/trailing
  whitespace, task IDs, and local links; no full markdownlint ruleset pass is
  claimed.
- Planned versions are documentation targets, not implemented contracts. Their
  serialization, compatibility, migration, and golden tests remain with their
  listed roadmap owners.
- No frontend lint/typecheck/build script exists in `packages/contracts/ts`.
  No TS code changed and no such pass is claimed.
- No application unit/integration/E2E, migration, database, broker, Redis,
  runtime, or infrastructure test was needed or run for this documentation-only
  task. No full-suite pass is claimed.
- The rebuildable Go cache remains outside the project at
  `work/fnd003-gocache`; its recursive cleanup request was blocked before
  execution by sandbox approval policy.

## 11. Remaining risks or blockers

- Legacy `commission_rate`, `max_participants`, float contracts, status-only
  registration, Power Law prize paths, and leaderboard finalization code remain.
- Most target schema/contract IDs remain intentionally unassigned until their
  roadmap tasks define executable compatibility rules.
- The existing `v1` contract namespace may be mistaken for approved target
  behavior unless consumers follow the new legacy warning/catalog.
- FND-004 must still define the clean database baseline; no migration strategy
  was implemented here.
- Existing FND-001 P0/P1 issues and CI patch-pin warnings remain unresolved.
- Paid production remains **NO-GO**.

## 12. Commit hash

Not applicable. No Git repository was initialized and no commit was created.
The nominal Conventional Commit would be
`docs(product): add canonical domain glossary`, but it was not executed.

## 13. Pull-request URL and merge status

Not applicable. No remote service was contacted; no push, pull request, or
merge was attempted. Completion is represented by the four files in section 3
and this report's local evidence.

## Final outcome

FND-003 is complete under the local-execution override. Canonical terminology,
version status, and remediation ownership are fixed without changing product
behavior. `FND-004` was not started.
