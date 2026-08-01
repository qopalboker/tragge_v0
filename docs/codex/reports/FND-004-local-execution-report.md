# FND-004 local execution report

**Task:** `FND-004 ? Define the disposable-database migration reset strategy`

**Execution mode:** Local extracted archive; no Git operations

**Date:** 2026-07-25

**Result:** `PASS`

**Paid-production status:** `NO-GO`

## 1. Selection and dependency verification

FND-004 was the only task selected. FND-005 was not started.

The required FND-001/FND-003 dependencies and requested FND-002 architecture
dependency were verified from repository evidence before editing:

- `docs/architecture/current-state-audit.md`
- `docs/adr/0001-target-runtime-architecture.md`
- `docs/architecture/target-architecture-import-review.md`
- `docs/product/canonical-domain-glossary-and-version-catalog.md`
- `packages/contracts/README.md`
- `docs/codex/reports/FND-001-local-execution-report.md`
- `docs/codex/reports/FND-002-local-execution-report.md`
- `docs/codex/reports/FND-003-local-execution-report.md`
- all four prior focused validation scripts

The three reports record local completion, and the final regressions passed:
FND-001 5/5, FND-002 4/4, and FND-003 8/8. ADR-0001 remains Accepted.

The phase prompt requires Git delivery, but the user?s local-execution override
explicitly disables branch, commit, push, pull request, and merge work. No
`.git` directory was created and no remote service was contacted.

## 2. Migration inventory result

The initial and final legacy inventory is deterministic:

- Top-level legacy up migrations: **98**
- Ordered identifiers: **0001 through 0098**, continuous
- Duplicate up identifiers: **0**
- Up migrations without matching down: **0**
- Top-level legacy down files: **99**
- Known down without up: **1**, `0000_baseline.down.sql`
- Target foundation migrations: **1 up/down pair**, isolated in the `target`
  child directory and excluded from legacy counts

The orphan `0000` down drops/recreates `public`. It remains byte-for-byte equal
to the original archive only for FND-001 traceability, is never read by the
target runner, and is deleted with the legacy chain after cutover.

Every one of the 98 up migrations is classified exactly once:

| Classification | Count |
|---|---:|
| `KEEP` | 0 |
| `FOLD_INTO_BASELINE` | 23 |
| `REPLACE` | 57 |
| `DELETE_AFTER_CUTOVER` | 18 |
| **Total** | **98** |

Zero `KEEP` is intentional: every legacy migration writes or depends on the
shared `public` model, so none can remain unchanged in the ADR-owned target.

## 3. Files changed by FND-004

1. `docs/architecture/database-migration-reset-strategy.md` ? added the target
   schema contract, conflict audit, reset/cutover workflow, migration/rollback
   policy, and isolated optional import policy.
2. `docs/architecture/migration-inventory.md` ? added the complete ordered
   98-row inventory and classification.
3. `docs/codex/reports/FND-004-local-execution-report.md` ? added this report.
4. `packages/db/README.md` ? distinguishes the legacy chain from the target
   foundation and replaces the unguarded volume-reset instructions.
5. `packages/db/init/target/01-cluster-roles.sql` ? added non-login migration,
   owner, and runtime group-role provisioning; no credentials.
6. `packages/db/init/target/02_reference_data.seed.sql` ? added an explicit
   no-domain-data seed marker with DATA-005/SCH-003 ownership.
7. `packages/db/migrations/target/0001_schema_ownership.up.sql` ? added only the
   three schemas, owners, grants, and default privileges.
8. `packages/db/migrations/target/0001_schema_ownership.down.sql` ? added a
   development/test-only foundation down migration.
9. `packages/db/migrations_test.go` ? asserts 98 valid pairs and exactly one
   documented legacy orphan rather than treating unknown orphans as valid.
10. `scripts/database-reset.mjs` ? added guarded dry-run/execute reset runner.
11. `scripts/database-migration-reset.test.mjs` ? added nine focused tests.
12. `scripts/production-baseline.mjs` ? keeps FND-001 SQL counts scoped to its
    immutable legacy snapshot while FND-004 validates isolated target SQL.
13. `scripts/squash-migrations.sh` ? fails closed before its historical unsafe
    `pg_dump`/force body and directs operators to the guarded strategy.

`packages/db/migrations/0000_baseline.down.sql` was temporarily removed during
editing, then restored byte-for-byte from `D:\tragge-codex\tragge-main.zip`.
Its final SHA-256 is
`5c89bdf2a05ed1f805395f44c2e66216d9878747b5856151951ce699b977dafd`,
matching the archive, so it is not a final changed file.

No application source, financial calculation, authentication behavior, Contest
behavior, Trading Engine behavior, Market Data behavior, payment integration,
frontend, dependency manifest, lockfile, or production deployment file changed.

## 4. Implementation and policy mapping

### Ownership foundation

The target uses exactly the ADR-0001 systems and schemas:

| System | Schema | Owner role | Runtime group role |
|---|---|---|---|
| Platform Modular Monolith | `platform` | `platform_owner` | `platform` |
| Trading Engine | `engine` | `engine_owner` | `engine` |
| Market Data Service | `market_data` | `market_data_owner` | `market_data` |

`tragge_migrator` is a non-login group with owner-role memberships. Runtime
roles receive only their own schema usage and default DML grants; SQL embeds no
login or password. The execute-mode validation checks the complete 3?3 schema
privilege matrix. Domain tables remain planned; the target foundation contains
no `CREATE TABLE`.

### Canonical schema rules

The target declaration provides:

- one Platform Fee field, `platform_fee_bps = 2000`;
- one Prize Pool source in `contest_economics_snapshot`;
- no canonical Contest `commission_rate`;
- no product Participant Capacity field;
- no Second Chance field/table/state;
- integer minor-unit money and integer basis points;
- integer fixed-point price, rate, P&L, and T-Score representations with scales
  assigned by their owning later tasks;
- integer Trading QTY separated from Real Participant count;
- immutable double-entry ledger, snapshot, outbox/inbox, audit, idempotency, and
  reconstruction requirements;
- UTC `TIMESTAMPTZ`, opaque UUID identity, conservative enum rules, deterministic
  migration ordering, and explicit migration ownership.

The implemented foundation intentionally assigns no canonical complete database
schema version and no domain-table version. The glossary?s `Not assigned
(planned)` status remains correct until later tasks implement and verify those
artifacts.

### Reset safety

`scripts/database-reset.mjs` defaults to dry-run. Destructive execution refuses
unless the environment is explicitly non-production, the database matches the
approved dev/test/staging name pattern, connection identity is complete, a
remote host is repeated exactly, the database confirmation matches exactly,
the destructive confirmation value is present, and `--execute` is specified.
It prints no password. Production is rejected before `psql` can run.

The exact fresh-install command chain is documented. It provisions group roles,
drops/recreates only the guarded database, applies the isolated target chain,
runs sorted seeds, and checks schema ownership/grants. The old squash script
exits `1` before `pg_dump`; the old volume-removal reset is no longer approved.

### Migration and import policy

The strategy documents forward-only production preference, tightly limited
development/pre-write downs, compensating forward migrations, transaction and
lock/timeout expectations, restartable backfills, expand/contract large-table
changes, immutable checksums, schema-version recording, fresh/upgrade testing,
and named restore responsibility. Applied production migrations may never be
silently edited.

Default legacy import is none. Any later exception is a separate, versioned,
non-automatic, restartable tool that validates source version, preserves target
invariants, reconciles row/financial totals, and emits an import report. No
legacy importer was implemented.

## 5. PostgreSQL and Docker execution status

- `psql`: unavailable
- `pg_dump`: unavailable
- `golang-migrate`: unavailable
- `sqlfluff`: unavailable
- `pgsanity`: unavailable
- Docker CLI: installed, client `29.4.3`
- Docker daemon: unavailable; named-pipe connection failed
- Real PostgreSQL fresh install: **not executed**
- Real destructive reset: **not executed**
- Docker PostgreSQL validation: **not executed**
- Normalized live schema dump/diff: **not executed**

The dry-run and structural/static checks ran. No real database result is
claimed. The report preserves the exact unavailable-runtime evidence below.

## 6. Tests and validation

Final successful evidence:

- `node --check scripts/database-reset.mjs`: exit `0`.
- `node --check scripts/database-migration-reset.test.mjs`: exit `0`.
- `node scripts/database-migration-reset.test.mjs`: exit `0`; **9 passed, 0
  failed**.
- Guarded dry-run: exit `0`; exact target plan printed; no database command.
- Production execute probe: expected exit `1`; refused before `psql`.
- `go test ./packages/db/migrations_test.go -v`: exit `0`; **5 passed** and
  reports 98 pairs plus one documented orphan.
- `go vet ./packages/db/migrations_test.go`: exit `0`.
- FND-001: **5/5** tests and baseline verification passed; the same three CI
  patch-pin warnings remain.
- FND-002: **4/4** passed.
- FND-003: **8/8** passed.
- Local Markdown links for the strategy, inventory, and DB README resolve; no
  tabs or trailing whitespace.
- Final archive comparison: expected 21 additions and 7 modifications, no
  original file missing, no unexpected path, `.git` absent, FND-005 report
  absent.

Go emitted a non-failing telemetry warning because it cannot write
`C:\Users\parsa\AppData\Roaming\go\telemetry\local\upload.token`. The full DB
package test was attempted but could not resolve an uncached Sarama module
because outbound dependency access was unavailable. The focused standalone
migration test and vet did not require that dependency and passed.

Coverage change: not applicable. No application behavior or critical runtime
package changed, and no artificial application unit test was added.

## 7. Every command and exact result

Commands are listed chronologically. Long read/patch bodies are identified by
the exact files or ranges they operated on; patch content is the delivered file
content. No successful result is inferred from a failed command.

| # | Command | Exact result |
|---:|---|---|
| 1 | Resolve `work/tragge-main`; list root; `rg --files` for repository instruction files | Exit `1`; project root listed; no AGENTS/.agents/.codex instruction file matched. |
| 2 | Read `docs/codex/prompts/01_PHASE_0_BASELINE.md`; locate FND-003/004/005 headings | Exit `0`; prompt read and FND-004 found at line 442. |
| 3 | Read roadmap lines 442?487 and the complete fixed policy | Exit `0`; complete FND-004 block and policy read. |
| 4 | Read ADR-0001 and the canonical glossary together | Exit `0`; 589 output lines; combined output truncated inside glossary, so commands 5?6 completed it. |
| 5 | Read glossary lines 1?120 and count lines | Exit `0`; 234 total lines. |
| 6 | Read glossary lines 121?234 | Exit `0`; glossary read through EOF. |
| 7 | Verify prerequisite paths; attempt report grep; list DB/scripts/root command files | Exit `0`; all 12 artifacts present; Windows wildcard passed literally to `rg` caused one recorded report-grep error; DB/root listing completed. |
| 8 | Read status/result evidence from FND-001/002/003 reports | Exit `0`; all three local-completion records found. |
| 9 | Read DB README, Go migration test, squash script, and all init scripts | Exit `0`; current migration/init behavior inspected. |
| 10 | Summarize comments and SQL operations for migrations 0001?0049 | Exit `0`; all 49 printed in deterministic order. |
| 11 | Summarize comments and SQL operations for migrations 0050?0098 | Exit `0`; all 49 printed in deterministic order. |
| 12 | Broad conflict searches for fee/prize/capacity/settlement/numeric/ownership terms | Exit `0`; evidence printed; output was large/truncated and one Windows `*.up.sql` wildcard produced an OS error. Focused commands followed. |
| 13 | List roadmap task headings and compute migration pairs with an erroneous `Join-Path ... + '.sql'` expression | Exit `0` with non-terminating errors; headings printed, preliminary `UP=98 DOWN=99`, then repeated `Join-Path` parameter errors. No pairing result claimed. |
| 14 | Corrected deterministic up/down/base comparison | Exit `0`; `UP=98`, `DOWN=99`, duplicate IDs `0`, missing down `0`, one orphan `0000_baseline`, no 0000 up. |
| 15 | Read orphan down; inspect Make/migration commands and tool availability | Exit `0`; unsafe public drop found; Node/Go/Docker present; psql/pg_dump/createdb/dropdb/migrate unavailable. |
| 16 | Focused table/numeric/Second Chance/Redis searches | Exit `0`; fixed/float conflicts printed; Second Chance `NO_MATCHES`; two Redis comments; initial Windows wildcard table search errored and was replaced by command 17. |
| 17 | Extract every `CREATE TABLE` target per up migration | Exit `0`; table inventory printed for all creating migrations. |
| 18 | Create exact target init/migration child directories | Exit `0`; two directories created. |
| 19 | Unified-diff add four SQL files and delete orphan in one command | Exit `1`; four additions succeeded; delete patch did not apply. |
| 20 | Inspect added files/orphan and `.git` | Exit `0`; four SQL files present, runner not yet present, orphan present, `.git=False`. |
| 21 | Hex-inspect orphan | Exit `0`; content readable; no BOM. |
| 22 | Resolve/guard path then remove orphan | Exit `0`; exact in-project file removed temporarily. |
| 23 | Unified-diff add `scripts/database-reset.mjs` | Exit `0`; 257 lines added; `.git=False`. |
| 24 | Unified-diff add migration inventory | Exit `0`; 161 lines added. |
| 25 | Unified-diff add reset strategy | Exit `0`; 341 lines added. |
| 26 | Read established Node validation test style | Exit `0`; relevant test headers read. |
| 27 | Unified-diff add focused FND-004 test | Exit `0`; 286 lines added. |
| 28 | First test-import correction patch | Exit `1`; corrupt patch; no change. |
| 29 | Inspect first 32 test lines | Exit `0`; erroneous import located. |
| 30 | Second import correction patch | Exit `1`; patch did not apply; no change. |
| 31 | Zero-context import correction | Exit `1`; patch did not apply; no change. |
| 32 | Hex-inspect test file | Exit `0`; UTF-8/CRLF identified. |
| 33 | Import correction with whitespace-tolerant unified diff | Exit `0`; invalid import removed. |
| 34 | Combined DB README/squash patch | Exit `1`; corrupt patch; no change. |
| 35 | Zero-context insert target/legacy section in DB README | Exit `0`; 26 lines inserted. |
| 36 | Inspect README tail and squash script with line numbers | Exit `0`; exact unsafe sections identified. |
| 37 | Insert squash fail-closed guard and replace README reset section | Exit `0`; guard 9 lines; unsafe reset instructions retired. |
| 38 | Node syntax checks and first focused test run | Exit `1`; syntax passed; 7 tests passed, 2 failed: sandbox blocked child Node spawn and ADR capitalization assertion was too strict. |
| 39 | Inspect failing test sections | Exit `0`; exact lines read. |
| 40 | First in-process import patch | Exit `1`; corrupt patch; no change. |
| 41 | Corrected import patch | Exit `0`; test now imports `main` and no child-process helper. |
| 42 | Replace child-spawn dry-run test with in-process captured output | Exit `0`; 19 old lines replaced by 21. |
| 43 | First ADR case-comparison patch | Exit `1`; corrupt patch; no change. |
| 44 | Inspect architecture assertion lines | Exit `0`; exact current line numbers found. |
| 45 | Zero-context case-insensitive authority comparison | Exit `0`; assertion corrected without weakening terms. |
| 46 | Re-run syntax and focused tests | Exit `0`; 9/9 passed. |
| 47 | Execute dry-run and production-labeled execute refusal | Exit `0` overall; dry-run plan printed; production request exited `1` as expected before psql. |
| 48 | `docker version`, local Postgres image list, psql/migrate availability | Exit `0` wrapper; both Docker operations internally exited `1` because daemon pipe absent; psql/migrate false. |
| 49 | `go test ./packages/db/...` with isolated cache | Exit `1`; telemetry write warning plus uncached Sarama module attempted blocked proxy; package did not compile. |
| 50 | `go test ./packages/db/migrations_test.go -v` while orphan was temporarily absent | Exit `0`; 5 tests passed; telemetry warning. |
| 51 | Inspect FND-001 migration-count references | Exit `0`; audit records 98 up/99 down and orphan link. |
| 52 | Unified-diff restore orphan | Exit `0`; five lines restored, later byte-normalized from archive. |
| 53 | Locate stale ?removed orphan? wording | Exit `0`; affected inventory lines found. |
| 54 | First inventory wording correction | Exit `1`; patch did not apply; no change. |
| 55 | Inspect inventory lines 19?28 | Exit `0`; exact text found. |
| 56 | Zero-context inventory correction | Exit `0`; orphan retained as traceability until cutover. |
| 57 | Inspect focused migration pairing assertions | Exit `0`; exact lines found. |
| 58 | First known-orphan test patch | Exit `1`; patch did not apply; no change. |
| 59 | Zero-context known-orphan assertions | Exit `0`; 98 ups/99 downs and exact orphan required. |
| 60 | Inspect Go migration test pair/count sections | Exit `0`; exact lines found. |
| 61 | First Go orphan-validation patch | Exit `1`; corrupt patch; no change. |
| 62 | Zero-context Go constant/pair validation patches | Exit `0`; exact orphan asserted; unknown orphans still fail. |
| 63 | Inspect Go count lines | Exit `0`; exact lines found. |
| 64 | Update Go count/output for 98 pairs plus one known orphan | Exit `0`. |
| 65 | First FND-001 regression after target SQL addition | Exit `1`; 4/5 tests; SQL count 206 vs 202 and up count 99 vs 98; baseline verify failed those two metrics. |
| 66 | Inspect baseline inventory implementation | Exit `0`; recursive target SQL inclusion identified. |
| 67 | Inspect baseline test and expected snapshot metrics | Exit `0`; immutable expected metrics confirmed. |
| 68 | First baseline scoping patch | Exit `1`; corrupt patch; no change. |
| 69 | Zero-context baseline SQL-scope correction | Exit `0`; FND-001 excludes only FND-004 isolated target SQL. |
| 70 | Inspect corrected baseline and Go test | Exit `0`; intended code present. |
| 71 | `gofmt`; Node checks; focused test; standalone Go migration test | Exit `0`; 9/9 Node and 5/5 Go passed; telemetry warning only. |
| 72 | FND-001/002/003 full focused regression command | Exit `0`; 5/5, baseline verify PASS with 3 warnings, 4/4, and 8/8. |
| 73 | Include DB README in focused Markdown validation | Exit `0`. |
| 74 | WSL `bash -n`/runtime guard attempt | Exit `1`; Windows Bash service access denied; no shell check/result claimed. |
| 75 | Probe alternate `sh` | Exit `0`; `SH_UNAVAILABLE`; no alternate shell test claimed. |
| 76 | Locate privilege query and squash guard text | Exit `0`; exact lines found. |
| 77 | Expand execute-mode privilege query to full 3?3 matrix | Exit `0`. |
| 78 | First squash static-test patch | Exit `1`; corrupt patch; no change. |
| 79 | Zero-context squash static assertions | Exit `0`; test proves `exit 1` precedes `pg_dump`. |
| 80 | Re-run Node syntax and focused test | Exit `0`; 9/9 passed. |
| 81 | First SHA-256 archive comparison | Exit `0`; 28 changed/added, 0 missing, `.git=False`; orphan line-ending difference identified. |
| 82 | First archive byte-restore command using invalid `Select-Object -Single` | Exit `0` with a non-terminating parameter error; hash was not accepted as proof. |
| 83 | Correct archive byte restore with unique-entry count | Exit `0`; exact orphan restored; SHA-256 `5c89...dafd`. |
| 84 | Standalone Go vet and parser/runtime availability probe | Exit `0`; vet passed; psql, pg_dump, sqlfluff, pgsanity, migrate unavailable; telemetry warning. |
| 85 | Attempt to replace old squash header | Exit `1`; patch did not apply; no change. |
| 86 | Insert explicit retirement notice above historical squash header | Exit `0`. |
| 87 | Create this FND-004 report with a generated unified diff | Exit `0`; report added. |
| 88 | Final syntax, focused, Go, prior-regression, dry-run/refusal, and Markdown validation command | Exit `0`; 9/9, 5/5, vet 0, FND 5/5 + 4/4 + 8/8, guards and links pass; recorded warnings only. |
| 89 | Initial final archive/scope/count guard | Exit `1`; file scope was correct at 21 added, 7 modified, 0 missing/unexpected, but an over-broad classification regex also counted the summary table and returned 100 instead of 98. |
| 90 | Update this report command log and narrow the final classification parser to numbered manifest rows | Exit `0`; report corrected; no product or schema file changed. |
| 91 | Corrected final archive/scope/count guard including this report | Exit `0`; 21 added, 7 modified, 0 missing/unexpected, `.git` absent, FND-005 report absent, 98 classified with totals 0/23/57/18. |

Non-shell editing/tool actions:

| Action | Exact result |
|---|---|
| Built-in `apply_patch` with the first multi-file patch | Failed before changes: Windows split writable-root sandbox could not prepare. |
| Request exact read/write permission for the project | Granted for the session. |
| Retry built-in `apply_patch` | Same sandbox-preparation failure; no changes. |
| Discover orchestration-layer `apply_patch` | Tool was exposed. |
| Orchestration patch before target directories existed | Failed to write; no change. |
| Orchestration patch after directory creation | Failed to write; no change. |
| Fallback editor | `git apply --no-index` applied unified diffs only; no repository metadata, branch, commit, remote, or Git history was created. |

## 8. Acceptance-criteria checklist

- [x] Every current top-level up migration is inventoried and classified once.
- [x] Migration count/order is reproducible and duplicate identifiers fail.
- [x] One guarded fresh-install command chain is documented and implemented.
- [x] Real PostgreSQL/Docker execution is accurately marked unexecuted.
- [x] Target declaration has one Platform Fee source and no canonical
  `commission_rate`.
- [x] Target declaration has one Prize Pool source.
- [x] Target declaration has no product Participant Capacity.
- [x] Target declaration and SQL have no active Second Chance model.
- [x] Target financial values use integer/fixed-point representation.
- [x] Platform, Trading Engine, and Market Data ownership is explicit and
  privilege-separated.
- [x] Down/rollback policy is explicit.
- [x] Optional legacy import is isolated, non-automatic, and not implemented.
- [x] Later roadmap features are marked planned, not implemented.
- [x] Migration naming/order, inventory coverage, paths, task IDs, ownership,
  links, dry-run, and refusal guards have focused automated validation.
- [x] FND-001, FND-002, and FND-003 focused regressions pass.
- [x] No application behavior changed and no artificial application tests were
  added.
- [x] Paid-production status remains `NO-GO`.

## 9. Unresolved schema ambiguities and later owners

These are intentionally unresolved because their implementation belongs to
later tasks:

- Canonical complete database baseline/version and per-owner checksum tables:
  ARCH-006 plus the first owning domain migrations.
- Money/price/rate/P&L/T-Score scale and UUID serialization: DATA-001, ENG-002,
  MD-001.
- Canonical Platform Fee columns: DATA-002.
- Ledger tables/accounts/posting constraints: DATA-003/DATA-004.
- System Participant identity and Free Practice registrations: DATA-005 and
  SCH-003.
- Contest lifecycle, template, economics snapshot, and start/refund schema:
  CON-001 through CON-005.
- Settlement/result/Prize Pool schema: PRIZE-001 through PRIZE-008.
- Trading Engine and Market Data domain tables/contracts: ENG-001 through
  ENG-008 and MD-001 through MD-007.
- Runtime login/secret provisioning and production database automation:
  ARCH-006 and OPS-003.
- Real backup/PITR/restore evidence: OPS-006.

## 10. Known untested behavior and remaining risk

- The role/foundation SQL was not parsed or applied by PostgreSQL here.
- No blank PostgreSQL database, schema-only dump, or normalized live schema diff
  was produced.
- No Docker container was started because the daemon is unavailable.
- Current legacy applications were not started against the target foundation;
  they are expected to be incompatible until later architecture/data tasks.
- Full `packages/db` tests did not compile because a dependency was uncached and
  external dependency access was unavailable; no pass is claimed.
- WSL Bash was access denied and no alternate `sh` existed, so the squash script
  was validated statically, not by a shell runtime.
- Target domain constraints, ledger balancing, outbox/inbox, snapshots,
  Settlement, seeding, and optional import remain planned and untested.
- Existing FND-001 P0/P1 findings and CI patch-pin warnings remain unresolved.

These limitations do not change FND-004?s documentation/foundation acceptance,
but they remain paid-launch blockers.

## 11. Delivery status

- Branch: not created under local-execution override.
- Commit: not created.
- Push/remote: not used.
- Pull request: not created.
- Merge: not performed.
- FND-004: **PASS** for local documentation, structural foundation, guard, and
  focused static validation.
- FND-005: **not started**.
- Paid production: **NO-GO**.
