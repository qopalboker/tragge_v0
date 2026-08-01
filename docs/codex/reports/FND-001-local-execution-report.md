# FND-001 local execution report

**Task:** `FND-001` ? Create the production baseline and repository inventory  
**Execution mode:** extracted local archive; Git delivery requirements waived  
**Execution date:** `2026-07-25`  
**Task result:** **COMPLETE LOCALLY WITH RECORDED ENVIRONMENT LIMITATIONS**  
**Paid-production result:** **NO-GO**

This report is completion evidence for FND-001 only. It does not start or
implement `FND-002`.

## 1. Selected task and dependency verification

The selected task is `FND-001`. Its roadmap dependency is `None`. Before
implementation, the required
[`current-state-audit.md`](../../architecture/current-state-audit.md) and a
local toolchain declaration did not exist. The task was therefore the first
incomplete eligible Phase 0 task.

Authoritative sources were read completely as UTF-8 before implementation:

- [Fixed Product and Technical Policies](../../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md),
  version `2026-07-25.1`, SHA-256
  `71242471394A18452BA4F3F01EFF6373631881A9F3BAA29DA39F2E5FF05FDC75`.
- [Production Roadmap and Codex Tasks](../PRODUCTION_ROADMAP_AND_CODEX_TASKS.md),
  version `2026-07-25.1`, SHA-256
  `DE59B33A618C44D2681A72CB5178DA22F62B2DCE43C4D62B51476DDBB5E5004F`.
- [Phase 0 prompt](../prompts/01_PHASE_0_BASELINE.md).

## 2. Branch name

Not applicable by the local execution override. No `.git` directory exists,
and no repository was initialized. No branch was created.

The roadmap's nominal branch
`codex/fnd-001-create-the-production-baseline-and-reposit` was recorded but not
used.

## 3. Files changed

| File | Change |
|---|---|
| [`.tool-versions`](../../../.tool-versions) | Added exact local Go, Node, and pnpm baseline versions. |
| [`README.md`](../../../README.md) | Added the NO-GO status, audit link, corrected prerequisites, and reproducibility commands. |
| [`Makefile`](../../../Makefile) | Added convenience inventory, verification, and focused-test targets. |
| [`package.json`](../../../package.json) | Added baseline scripts and aligned Node/pnpm engine declarations with the resolved lockfile requirements. |
| [`scripts/production-baseline.mjs`](../../../scripts/production-baseline.mjs) | Added dependency-free inventory, evidence-link, finding-row, and toolchain verification. |
| [`scripts/production-baseline.test.mjs`](../../../scripts/production-baseline.test.mjs) | Added five focused tests for the baseline utility. |
| [`docs/architecture/current-state-audit.md`](../../architecture/current-state-audit.md) | Added the required inventory, topology, migration/test gaps, toolchains, limitations, and 35 evidenced P0/P1 findings. |
| [`docs/codex/reports/FND-001-local-execution-report.md`](FND-001-local-execution-report.md) | Added this local task report. |

No product implementation, schema, migration, API contract, runtime behavior,
dependency version, lockfile, deployment, or later-roadmap artifact was
changed.

## 4. Implementation summary

- Reproduced the roadmap counts: 375 Go, 211 Vue, 178 TypeScript/TSX, 202 SQL,
  98 up migrations, and 99 Go test files.
- Recorded all 17 application directories, all 20 immediate package
  directories, the 32-module Go workspace, two Go modules outside `go.work`,
  and four pnpm workspace packages.
- Recorded 99 down migrations, including the unmatched
  `0000_baseline.down.sql`.
- Mapped the current merged-wrapper and standalone runtime generations and the
  conflicting Compose/Kubernetes deployment shapes.
- Recorded eleven Go code modules with no Go tests and the missing frontend CI
  unit/E2E/typecheck execution.
- Added 35 P0/P1 rows; every row has severity and one or more resolving
  repository evidence links.
- Defined the local baseline as Go `1.24.7`, Node `20.19.0`, and pnpm `8.15.0`.
- Added a repeatable, dependency-free verifier and focused tests.

## 5. Important decisions and policy mapping

| Decision | Policy/task mapping |
|---|---|
| Keep paid production at NO-GO. | Phase 0 fixed decision and FND-001 acceptance criterion. |
| Record, do not remediate, the architecture/security/financial findings. | FND-001 forbids product behavior changes and later-task implementation. |
| Preserve exactly three approved target bounded systems in the audit. | Policy section 2: Platform, Trading Engine, and Market Data Service. |
| Add no dependency. | Roadmap dependency policy; the verifier uses Node built-ins. |
| Pin a repeatable local baseline but leave CI major-line pin remediation visible. | FND-001 defines supported toolchains; exact CI/release pin hardening remains later production-engineering work. |
| Do not classify or reset migrations. | FND-004 owns migration reset strategy. |
| Use local files and this report rather than Git evidence. | User's local execution override. |

The root Node engine was narrowed because the resolved lockfile includes tools
that require Node `^20.19.0`, `^22.13.0`, or `>=24`; Node 18 is not a truthful
supported baseline for the current dependency graph. No dependency or lockfile
was changed.

## 6. Tests added or updated

Added five focused Node tests:

1. Reproduce all six approved core repository counts.
2. Parse local, external, and angle-bracket Markdown links.
3. Detect missing local Markdown targets.
4. Require exactly 35 P0/P1 finding rows with severity and evidence.
5. Verify compatibility between `.tool-versions`, `go.work`, root package
   declarations, and CI major lines.

Result: **5 passed, 0 failed** when executed directly with
`node scripts/production-baseline.test.mjs`.

## 7. Commands executed and exact results

Commands were run from the extracted project root unless noted.

| Command or command group | Exact result |
|---|---|
| Read Phase 0 prompt, extract FND-001 block, search `AGENTS.md`/`.agents`/`.codex`. | Prompt and task read successfully; no repository instruction file found. Group exit `1` only because the final `rg` search had no match. |
| Read both authoritative documents with `ReadAllBytes`, UTF-8 roundtrip, and `Get-FileHash SHA256`. | Exit `0`; both complete reads passed and hashes match section 1. |
| `rg --files` with extension/migration/test filters, executed during discovery and later by the verifier. | Exit `0`; counts reproduced as 375/211/178/202/98/99. |
| Enumerate `apps`, `packages`, package manifests, Go modules, and `go work edit -json`. | Inventory succeeded; 17 apps, 20 packages, 32 workspace modules. Go emitted a telemetry-token permission warning while still returning exit `0`. |
| Enumerate migration pairs. | Exit `0`; 98 up, 99 down, only `0000_baseline` lacks an up pair. |
| Count Go/test files by module and frontend test/spec files. | Exit `0`; eleven Go modules have zero Go test files; ten frontend test/spec files exist. |
| Inspect runtime entry points, Compose services, Kubernetes resources, CI, auth, finance, contest, Engine, Market Data, and frontend evidence with `Get-Content` and `rg`. | Evidence reads completed. Two exploratory groups ended `1`: one used an invalid Windows wildcard passed to `rg`; another had no matching CI test/gate keywords. No pass was inferred from those exits. |
| `docker compose -f infra/docker/docker-compose.yml config --quiet`. | Exit `0`; static Compose configuration parses. |
| `docker compose -f infra/docker/docker-compose.yml --profile '*' config --services`. | Exit `0`; services: Redpanda, Kafka init, Redis, User Frontend, PostgreSQL, Worker, Trading Core, Admin Frontend, API Server, Gateway. |
| Local toolchain probes (`go version`, `node --version`, `pnpm --version`, Docker/Compose, Git, `psql`, `redis-cli`, `golangci-lint`, `gitleaks`, `make`). | Go `1.25.4`; Node `22.19.0`; pnpm `8.15.0`; Docker CLI `29.4.3`; Compose `v5.1.3`; Git `2.45.1`. Docker daemon unavailable. Other named tools absent. Go emitted a telemetry write warning. |
| Initial `go test ./packages/auth/...` with downloads disabled. | Exit `1` before compilation: sandbox denied Go build-cache and telemetry-state writes outside the project. No test result claimed. |
| Built-in `apply_patch` attempts and shell `apply_patch` probe. | Failed before edits because the Windows sandbox wrapper could not write/execute the helper. Exact project write permission did not change the failure. |
| `git apply --no-index` temporary add/delete probe. | Add exit `0`, delete exit `0`, probe removed; no `.git` metadata created. It was then used only as a unified-diff patch engine. |
| Initial `node --test scripts/production-baseline.test.mjs`. | Exit `1`; Node's isolated test runner could not spawn a child process (`EPERM`). The command was changed to direct in-process execution. |
| Initial `node scripts/production-baseline.mjs inventory`. | Exit `0`; inventory printed successfully. |
| Initial `node scripts/production-baseline.mjs verify`. | Exit `1`; correctly detected one non-existent evidence link. The citation was corrected to `0001_init.up.sql`. |
| `node scripts/production-baseline.test.mjs` after correction. | Exit `0`; 5 tests passed, 0 failed. |
| `node scripts/production-baseline.mjs verify` after correction. | Exit `0`; counts, 35 finding rows, 135 links, and toolchain compatibility passed; three CI patch-pin warnings remained. |
| `node --check scripts/production-baseline.mjs` and `node --check scripts/production-baseline.test.mjs`. | Both exit `0`. |
| `pnpm test:baseline`, `pnpm baseline:inventory`, and `pnpm baseline:verify`. | Each exit `1` before script execution because pnpm could not spawn `C:\Program Files\nodejs\node.exe --version` (`EPERM`). Direct Node equivalents passed. |
| `pnpm lint`, `pnpm typecheck`, and `pnpm build`. | Each exit `1` before underlying workspace scripts ran for the same pnpm-to-Node spawn `EPERM`. No lint/typecheck/build pass claimed. |

The final verifier and changed-file comparison are recorded below after this
report became part of the Markdown-link set.

## 8. Coverage change

No application or critical domain module changed, and no existing coverage
baseline was available. Coverage was therefore neither increased nor
decreased. The new baseline verifier has five focused functional tests, but no
numeric coverage claim is made.

## 9. Acceptance-criteria checklist

- [x] Audit is reproducible from documented commands.
- [x] Application/package inventory, current topology, migration count, test
  gaps, toolchain versions, and execution limitations are recorded.
- [x] `docs/architecture/current-state-audit.md` exists.
- [x] All 35 P0/P1 findings reference resolving repository paths.
- [x] Audit explicitly says the repository is NO-GO for paid production.
- [x] Supported local/CI-compatible toolchain baseline is defined.
- [x] No product behavior changed.
- [x] No dependency was added or upgraded.
- [x] Focused tests and direct syntax checks pass.
- [x] Local Git requirements were waived; Git was not initialized or used for
  branch/commit/remote operations.

## 10. Known untested behavior

- Make aliases were not run because `make` is absent.
- pnpm aliases and existing root lint/typecheck/build scripts could not start
  because the sandbox denied pnpm's child-process spawn.
- No Go suite, frontend Vitest suite, Playwright journey, integration test,
  migration execution, database permission test, image build, or deployment
  test was completed.
- Docker runtime behavior was not tested because the daemon is unavailable.
- The full repository Markdown corpus was not normalized or repaired; the
  verifier covers README, the new audit/report, and every evidence link used by
  the 35 findings.

## 11. Remaining risks and blockers

- All 35 P0/P1 findings in the audit remain unresolved by design.
- CI uses compatible major lines rather than exact patch pins and installs
  `golangci-lint` from mutable `HEAD`.
- Exact Go `1.24.7`, Docker daemon, database clients, scanners, Make, and a
  dependency-installed frontend workspace are needed for broader evidence.
- The archive cannot provide historical or review provenance. Future import
  into Git must review the eight changed files listed above.
- Paid production remains **NO-GO**.

## 12. Commit hash

Not applicable. No Git repository was initialized and no commit was created,
as required by local execution mode.

## 13. Pull request URL and merge status

Not applicable. No remote service was contacted; no push, pull request, or
merge was attempted.

## Final local evidence

- `node scripts/production-baseline.test.mjs`: exit `0`; 5 passed, 0 failed.
- `node scripts/production-baseline.mjs inventory`: exit `0`; 375 Go, 211 Vue,
  178 TypeScript/TSX, 202 SQL, 98 up migrations, 99 down migrations, 99 Go test
  files, and 10 frontend test/spec files.
- `node scripts/production-baseline.mjs verify`: exit `0`; consecutive
  inventories match, 35 findings are evidenced, 147 local Markdown links
  resolve, and local toolchain declarations are compatible. It reports three
  non-failing warnings because CI uses major rather than patch pins.
- Both `node --check` commands: exit `0`.
- `package.json` JSON parse: exit `0`.
- `docker compose -f infra/docker/docker-compose.yml config --quiet`: exit `0`.
- The first ZIP hash-comparison attempt was invalid because this PowerShell
  runtime does not implement `Convert.ToHexString`; it emitted
  `MethodNotFound` and its apparent change list was discarded.
- Corrected ZIP comparison using `BitConverter`: exit `0`; exactly three
  original files changed and five files were added, matching section 3; zero
  original files are missing.
- Secret-value signature scan of all eight changed/added files: exit `0`; zero
  candidates found.
- `.git` presence check: `False`.

The task is not represented as a Git merge. This report and the audit are its
local completion evidence. FND-001 is complete under the local execution
override; paid-production readiness remains **NO-GO**.
