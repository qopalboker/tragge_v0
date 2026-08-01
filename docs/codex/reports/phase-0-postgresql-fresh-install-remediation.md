# Phase 0 PostgreSQL Fresh Install remediation

**Original gate:** Phase 0 exit `FAIL`

**Remediation date:** 2026-07-26 (Asia/Tehran)

**Execution mode:** Local extracted project; PostgreSQL Fresh Install remediation only

**Remediation decision:** `PASS`

**Paid-production status:** `NO-GO`

This report remediates only the missing live PostgreSQL evidence identified by
the Phase 0 exit gate. It does not start Phase 1, `SEC-001`, another roadmap
task, a product feature, an application refactor, or a deployment.

## 1. Original gate failure

The original [Phase 0 exit report](./phase-0-exit-report.md) recorded one
Phase 0 gate blocker: no real isolated PostgreSQL Fresh Install had run because
host `psql`/migration tooling and the Docker daemon were unavailable. Every
other available Phase 0 check passed. This report preserves that history; it
does not reinterpret the missing check as an earlier success.

## 2. Execution date and local mode

The remediation ran on 2026-07-26 directly in the selected extracted project.
There was no `.git` directory before or after execution. No Git initialization,
branch, commit, remote, push, pull request, merge, CI run, release, production
credential, production connection, or production deployment was used.

The complete FND-004 target for this check is the ownership-only foundation:
seven `NOLOGIN` group roles; the `platform`, `engine`, and `market_data`
schemas; isolated grants/default privileges; one target up migration; and an
intentional no-domain-row seed. Domain tables and a persistent migration-version
table remain planned for later roadmap tasks and were not fabricated here.

## 3. Runtime preflight

| Check | Exact result |
|---|---|
| Docker CLI | Available: `Docker version 29.4.3, build 055a478`; exit `0`. |
| Docker daemon | Unavailable: access to `npipe:////./pipe/dockerDesktopLinuxEngine` was denied; exit `1`. |
| Docker Compose | Available: `Docker Compose version v5.1.3`; exit `0`. |
| Host `psql` | Not found before or after the temporary runtime. |
| Host migration CLI | `migrate` not found. The approved FND-004 runner is `node scripts/database-reset.mjs`, which applies SQL through `psql`. |
| Docker image | None used or pulled because the daemon boundary was unavailable. No container or Compose service was created. |
| Selected runtime | Temporary native PostgreSQL `16.14`, package version `16.14-1`; `postgres --version` and `psql --version` both exited `0`. |
| Native process | Temporary hidden `postgres.exe`; not a production service. |
| Bind/port | `127.0.0.1:55432` only. |
| Database | `tragge_test_phase0`, matching the existing approved `tragge_test_*` guard. |
| Administrative user | `tragge_phase0_admin`, created only inside the disposable cluster. |
| Credential | Random local test-only credential held in a temporary file, rotated once after an early diagnostic leak, never committed, and deleted with the runtime. |
| Environment | Explicit `test`; `APP_ENV`, `TRAGGE_ENV`, and `--environment` production/conflict checks enabled. |
| External target | None. No existing network database or external PostgreSQL service was targeted. |

The smallest temporary runtime was used because the repository contained no
working isolated PostgreSQL service and Docker remained inaccessible. It used
pinned MSYS2 packages downloaded into a temporary work directory:

| Package | SHA-256 |
|---|---|
| `postgresql-16.14-1.pkg.tar.zst` | `af282317d37294a676f0b896d0c7484559410e32e32c4475872a1120a376d492` |
| `icu-78.3-3.pkg.tar.zst` | `b5e805ce81202e48d52bf598345ac5c3ba229f017f03eb23858452b22349c391` |
| `lz4-1.10.0-1.pkg.tar.zst` | `a4c5a3bcd26111554c87591275b8a681bfa4473d1607647e24c22ef6213c055c` |
| `openssl-3.6.3-1.pkg.tar.zst` | `82de7ff886112374ffae9e7b3c843c82342e198543fb024790416ef56434fe9f` |
| `libxml2-2.15.3-1.pkg.tar.zst` | `5da9828356a54938e5402f90b0a104f8b9175ab0f5266e9ed1e6ffd057b754b1` |

The package files, binaries, data directory, logs, and credential were removed
after evidence collection. This temporary runtime is not a production design or
a new repository dependency.

## 4. Sanitized isolated-database identity

Before the first successful initialization, `psql` returned:

```text
16.14|127.0.0.1/32|55432|tragge_phase0_admin|postgres
postgres
```

Only the maintenance database was connectable; `tragge_test_phase0` did not
exist. Before the independent second initialization, the identity check again
returned PostgreSQL `16.14`, loopback `127.0.0.1/32`, port `55432`, user
`tragge_phase0_admin`, and `second_blank_database_count|0`.

No password or credential-bearing URL is stored in this report or the preserved
schema evidence.

## 5. Safety-guard rejection results

All four rejection cases ran before the successful reset. Each exited `1`
before a database command. The URL examples below intentionally omit passwords.

| Case and sanitized command | Exit | Relevant output |
|---|---:|---|
| `APP_ENV=production; node scripts/database-reset.mjs --execute --environment test --confirm-database tragge_test_phase0` | `1` | `RESET_REFUSED_OR_FAILED: APP_ENV "production" is prohibited for database reset` |
| Target URL ending in `/postgres`; execute flags otherwise valid | `1` | Database did not match the approved development/test naming pattern. |
| Valid test target without `TRAGGE_DATABASE_RESET_CONFIRM` | `1` | Required destructive confirmation was absent. |
| URL without an explicit user/complete target | `1` | Target connection must explicitly identify hostname, username, and database. |

Focused tests also cover conflicting environment signals, a non-local host
without exact `--allow-host`, mismatched `--confirm-database`, and redaction of
child-process failures.

## 6. Exact Fresh Install commands

Secrets were loaded from the temporary credential file and are represented by
`<disposable-secret>` below. This is the exact sanitized command chain:

```powershell
initdb.exe -D <temporary-data-dir> -U tragge_phase0_admin `
  -A scram-sha-256 --pwfile=<temporary-credential-file> `
  --encoding=UTF8 --locale=C

postgres.exe -D <temporary-data-dir> -h 127.0.0.1 -p 55432
pg_isready.exe -h 127.0.0.1 -p 55432 -t 2

$env:APP_ENV = 'test'
$env:TRAGGE_TARGET_DATABASE_URL = `
  'postgresql://tragge_phase0_admin:<disposable-secret>@127.0.0.1:55432/tragge_test_phase0?sslmode=disable'
$env:TRAGGE_DATABASE_RESET_CONFIRM = 'I_UNDERSTAND_THIS_DESTROYS_DATA'
node scripts/database-reset.mjs --execute --environment test `
  --confirm-database tragge_test_phase0
```

The first blank run exited `0` with:

```text
DROP DATABASE
CREATE DATABASE
BEGIN
CREATE SCHEMA (three times)
...
COMMIT
fnd004_no_domain_seed_data
DO
RESET_OK: target foundation and structural validation completed
```

`DROP DATABASE IF EXISTS` reported that the absent target was skipped. The
command then created the positively identified test database from blank state.

## 7. Migration and seed results

The deterministic runner plan printed exactly:

```text
role_file=packages/db/init/target/01-cluster-roles.sql
migrations=packages/db/migrations/target/0001_schema_ownership.up.sql
seeds=packages/db/init/target/02_reference_data.seed.sql
```

Static validation returned:

```text
TARGET_UP_COUNT=1
TARGET_IDENTIFIERS=0001_schema_ownership
DUPLICATE_IDENTIFIER_COUNT=0
MISSING_DOWN_COUNT=0
MIGRATION_SHA256=0001_schema_ownership.up.sql:72170eb5ef2190bc243a0158181a38b4ed972a748585586fd29348eb6d51d238
```

The one target up migration ran once in each blank database. The intentional
seed returned `fnd004_no_domain_seed_data`. The 98 legacy migrations were not
applied to the target database.

## 8. Applied migration count and version tracking

- Declared FND-004 target up migrations: **1**.
- Applied to each blank initialization: **1**, exactly once by the sorted runner
  plan.
- Duplicate target identifiers: **0**.
- Missing down pair: **0**.
- Legacy up migrations retained as evidence but not applied: **98**.
- Live migration/version tracking relations: **0**.

The absence of a tracking table is the declared FND-004 baseline, not a hidden
success claim. Per-owner version/checksum recording remains planned for
`ARCH-006`. This remediation validates the one-file process evidence and does
not mark `ARCH-006` implemented.

## 9. Schema and ownership validation

Live SQL returned:

```text
schema|engine|engine_owner|Trading Engine owned state; no cross-system SQL
schema|market_data|market_data_owner|Market Data Service owned state; no cross-system SQL
schema|platform|platform_owner|Platform modular monolith owned state; no cross-system SQL
```

All seven declared group roles were present with `login=false`, `super=false`,
`createdb=false`, `createrole=false`, and `replication=false`. The migrator had
membership in exactly the three owner roles. Runtime search paths were:

```text
engine=engine, pg_catalog
market_data=market_data, pg_catalog
platform=platform, pg_catalog
```

The complete runtime-schema `USAGE` matrix was:

```text
engine:engine=true, engine:market_data=false, engine:platform=false
market_data:engine=false, market_data:market_data=true, market_data:platform=false
platform:engine=false, platform:market_data=false, platform:platform=true
```

`PUBLIC` had neither `USAGE` nor `CREATE` on `public`, `platform`, `engine`, or
`market_data`. Default table privileges were `SELECT`, `INSERT`, `UPDATE`, and
`DELETE` only for each owning runtime; sequence privileges were `USAGE` and
`SELECT` only for the owning runtime.

The exact FND-004 foundation had zero target domain tables, views, and columns.
Therefore:

- banned active identifier count for `commission_rate`, participant capacity,
  `max_participants`, and Second Chance was `0`;
- floating financial-column count was `0`;
- there was no duplicate Platform Fee or Prize Pool source;
- `platform_fee_bps` remains the canonical declared target field, planned for
  `DATA-002`, and was not falsely created by FND-004;
- ledger, outbox/inbox, audit, snapshots, System Participant, and other domain
  foundations remain planned for their roadmap owners and were not falsely
  reported as present.

The persisted sanitized structural evidence is
[`evidence/phase-0-postgresql-target-schema.txt`](./evidence/phase-0-postgresql-target-schema.txt).
Its file SHA-256 is
`44942e15bcd87d22172575fc6834cd3f707779cec87ae17c5aedae533fb72a2e`.

## 10. Determinism comparison

After the first success:

1. `pg_dump --schema-only` exported the target structure.
2. Only PostgreSQL's random `\restrict`/`\unrestrict` client-safety key was
   replaced with `<normalized-random-client-key>`. This token protects dump
   replay parsing and has no schema meaning.
3. The loopback server was stopped; the exact disposable data directory was
   verified and removed.
4. A second cluster was initialized from blank state with the same PostgreSQL
   version, locale, user, bind, port, runner, migration, and seed.
5. The second pre-run database count was zero.
6. The guarded initializer and schema-only export ran again.

Results:

```text
FIRST_NORMALIZED_SHA256=4e9e8df5fe3c741bb1219ec45acf2ee1749650ce7beba4312d94ff7168fc6d81
SECOND_NORMALIZED_SHA256=4e9e8df5fe3c741bb1219ec45acf2ee1749650ce7beba4312d94ff7168fc6d81
STRUCTURAL_EQUIVALENCE=true
```

The two raw hashes differed only because PostgreSQL generated a new random
client-safety key. No object identifiers, timestamps, seed identifiers, or
other schema fields required exclusion.

## 11. Failure and recovery test

A real controlled failure occurred without editing an approved migration:

1. The initial runtime attempt exposed that the runner placed the connection
   URL before `psql` options. `psql` ignored the following file/variable options;
   the target database remained absent, and the command exited non-zero.
2. After correcting argument order, a second attempt created the database,
   applied the migration and seed, but the Node capture path failed at structural
   validation under the Windows sandbox. The runner exited `1`; it did **not**
   print `RESET_OK`.
3. Live queries detected the partial initialization: all three schemas and role
   grants existed even though the overall command failed.
4. The runner was corrected to perform the structural assertion inside one
   server-side `DO` block and to redact all child-process failures.
5. PostgreSQL in this restricted Windows runtime could not signal its
   checkpointer to drop the already-created database. Rather than bypass safety,
   the localhost server was stopped, the positively identified disposable data
   directory was removed, and a new blank cluster was initialized.
6. The corrected guarded run exited `0`, emitted `RESET_OK`, and the final state
   passed all live queries and the independent second rebuild.

A disposable credential URL appeared once in ephemeral diagnostic output before
the error sanitizer was corrected. It was never written to the repository, was
immediately rotated in the temporary cluster, and was deleted at cleanup. The
new focused test proves that failures cannot include a URL or password.

This is a restartable recovery path for the disposable pre-launch database: a
failed initialization is non-success, partial state is detected, and recovery
uses a verified blank cluster rather than editing a migration or forcing a
version record.

## 12. Regression results

| Command | Exact result |
|---|---|
| `node --check scripts/production-baseline.mjs` | Exit `0`. |
| `node --check scripts/production-baseline.test.mjs` | Exit `0`. |
| `node scripts/production-baseline.test.mjs` | Exit `0`; 5 passed, 0 failed. |
| `node --check scripts/target-architecture.test.mjs` | Exit `0`. |
| `node scripts/target-architecture.test.mjs` | Exit `0`; 4 passed, 0 failed. |
| `node --check scripts/domain-glossary.test.mjs` | Exit `0`. |
| `node scripts/domain-glossary.test.mjs` | Exit `0`; 8 passed, 0 failed. |
| `node --check scripts/database-reset.mjs` | Exit `0`. |
| `node --check scripts/database-migration-reset.test.mjs` | Exit `0`. |
| `node scripts/database-migration-reset.test.mjs` | Exit `0`; 10 passed, 0 failed. |
| `go test ./packages/db/migrations_test.go -v` | Exit `0`; 5 passed; 98 legacy pairs and one documented orphan verified. |
| `go vet ./packages/db/migrations_test.go` | Exit `0`; emitted only sandbox-denied Go telemetry token warnings. |
| `node --check scripts/codex-execution-protocol.test.mjs` | Exit `0`. |
| `node scripts/codex-execution-protocol.test.mjs` | Exit `0`; 11 passed, 0 failed. |

The FND-004 suite increased from nine to ten focused tests because this
remediation added the `psql` ordering/redaction case and expanded environment
signal checks. No application unit test was added.

Markdown link/path/task-ID, focused secret, and scope checks are recorded in
Section 16. No standalone markdownlint executable was installed or run, so no
markdownlint pass is claimed.

## 13. Files changed

| File | Change |
|---|---|
| [`../../../scripts/database-reset.mjs`](../../../scripts/database-reset.mjs) | Reject ambient production/conflicting environment signals; place `psql` options before the URL; sanitize failures; validate all three schema owners and the full isolation matrix server-side. |
| [`../../../scripts/database-migration-reset.test.mjs`](../../../scripts/database-migration-reset.test.mjs) | Add focused environment, ordering, and credential-redaction coverage; 10 tests now pass. |
| [`../../architecture/database-migration-reset-strategy.md`](../../architecture/database-migration-reset-strategy.md) | Align documented environment and full runtime-isolation guards with the executable runner. |
| [`../../../packages/db/README.md`](../../../packages/db/README.md) | Link the real remediation evidence and describe conflicting-environment refusal. |
| [`evidence/phase-0-postgresql-target-schema.txt`](./evidence/phase-0-postgresql-target-schema.txt) | Preserve the sanitized normalized target schema-only export. |
| [`phase-0-postgresql-fresh-install-remediation.md`](./phase-0-postgresql-fresh-install-remediation.md) | This remediation report. |
| [`phase-0-exit-report.md`](./phase-0-exit-report.md) | Preserve the original failure and add the dated current Phase 0 `PASS` decision. |

No application, legacy migration, target migration, seed, runtime
configuration, product behavior, infrastructure behavior, or frontend file was
changed.

## 14. Known untested behavior

- Docker-based PostgreSQL was not run because the Docker daemon remained
  inaccessible. The native PostgreSQL evidence is real; no Docker success is
  claimed.
- Dropping an already-populated target inside this restricted Windows sandbox
  was blocked by PostgreSQL's inability to signal its checkpointer. Blank
  initialization and whole-disposable-cluster recovery were both executed and
  passed. The ordinary existing-database drop path should be rerun in CI once a
  normal PostgreSQL service is available.
- Graceful `pg_ctl stop` was blocked by the same Windows restricted-token
  boundary. The positively identified local process was stopped directly, and
  port/process checks passed before deletion.
- FND-004 intentionally has no domain tables or migration-version table.
  Runtime behavior for later `ARCH-*`, `DATA-*`, `CON-*`, `ENG-*`, `MD-*`, and
  other roadmap migrations remains untested because it is not implemented.
- Application startup against the future domain schema remains a later-task and
  phase-gate responsibility.

These limitations do not erase the previously missing Fresh Install evidence:
two real blank PostgreSQL 16.14 installations, migration/seed execution, live
ownership validation, failure recovery, and deterministic schema comparison
all completed.

## 15. Cleanup evidence

After evidence collection:

```text
CLEANED|<temporary-postgresql-runtime>|previously_existed=true|exists_after=false
CLEANED|<temporary-go-cache>|previously_existed=true|exists_after=false
DISPOSABLE_CREDENTIAL_REMOVED_WITH_RUNTIME=true
RUNTIME_EXISTS_AFTER=False
GO_CACHE_EXISTS_AFTER=False
LOCAL_PORT_55432_LISTENER_COUNT=0
VISIBLE_POSTGRES_PROCESS_COUNT=0
```

No Docker container was created. A final `docker ps` confirmation could not
query the inaccessible daemon and exited `1`; this is not reported as a Docker
cleanup pass. The absence of a local container follows from the recorded fact
that no Docker create/run/compose-up command succeeded or was used.

## 16. Command and validation ledger

Commands that loaded the disposable secret are shown as exact sanitized
equivalents. The secret value and full credential-bearing URL are deliberately
omitted.

| # | Command or command block | Exact result |
|---:|---|---|
| 1 | Workspace, `AGENTS.md`, artifact, and `.git` enumeration with `Get-ChildItem`, `rg --files`, and `Test-Path` | Exit `0`; repository located; no `.git`; authoritative files present. |
| 2 | Full reads and SHA-256/line counts for fixed policy, roadmap, reset strategy, inventories, FND-004/exit reports, protocol, SQL, and scripts | Exit `0`; actual filenames verified. |
| 3 | Initial Docker/`psql`/migration preflight | Wrapper exit `1`; Docker CLI/Compose present, daemon denied, host `psql`/`migrate` absent. |
| 4 | Docker Desktop start/poll probe | Wrapper exit `0`; an initial blank-pipeline expression produced a false-positive boolean and was discarded. |
| 5 | Explicit `docker info` and `docker ps` probes | Wrapper exit `0`; both inner commands exit `1` with named-pipe permission denied. |
| 6 | Docker group/service/WSL inspection | Wrapper exit `0`; account is in `docker-users`, service was stopped/manual, start unavailable, WSL access denied. |
| 7 | Network permission request and Docker/WSL retry | Permission granted; Docker and WSL still exit `1` at OS boundary. |
| 8 | Installed PostgreSQL/package-manager/build-tool search | Exit `0`; no PostgreSQL; `winget`/Chocolatey observed; no suitable existing runtime. |
| 9 | EDB download probes (`Invoke-WebRequest`, `curl`, Node HTTPS) | PowerShell/curl failed TLS; Node reached server but received geo-block `403`; no package installed. |
| 10 | MSYS2 package discovery/download and SHA-256 verification | Exit `0`; PostgreSQL 16.14-1 and four required libraries matched the hashes in Section 3. |
| 11 | Initial binary probes before library extraction | `psql` exit `0`; `postgres`/`pg_dump` failed missing DLL. |
| 12 | Dependency extraction and binary probes | Exit `0`; `postgres`, `initdb`, `pg_ctl`, `psql`, `pg_dump`, and `pg_isready` reported 16.14. |
| 13 | Initial `apply_patch` operations | Failed at the Windows sandbox wrapper before file mutation. |
| 14 | No-index unified-diff fallback patches | Exit `0` for the applied scoped changes; two malformed/incorrect-context attempts exited non-zero without mutation. |
| 15 | First reset-test syntax run after insertion | Exit `1`; misplaced nested test caused syntax/structure correction; no database command. |
| 16 | Corrected reset syntax/focused tests | Exit `0`; initially 9 top-level/10 total, then corrected to 10 independent passing tests. |
| 17 | Four required guard CLI invocations | Each runner exit `1` as detailed in Section 5; no database command. |
| 18 | First `initdb`/`pg_ctl` attempts | Argument correction required; `pg_ctl start` exited `1` at Windows restricted-token boundary. |
| 19 | Hidden direct `postgres.exe` start on `127.0.0.1:55432` | Tool wrapper was interrupted, but the server started; readiness/identity checks later exited `0`. |
| 20 | Cleanup attempt while socket was live | Refused without deletion; the runtime and data remained intact. |
| 21 | `psql` readiness/identity/blank-database query | Exit `0`; PostgreSQL 16.14, loopback, test admin, only `postgres` database. |
| 22 | First guarded runner execution | Exit `1`; exposed URL-before-options defect; target remained absent. |
| 23 | Local disposable credential rotation and authentication | Exit `0`; new credential authenticated; old value invalidated. |
| 24 | Runner ordering/redaction patch and tests | Exit `0`; ordering/redaction assertions pass. |
| 25 | Second guarded runner execution | Exit `1` after migration/seed; structural capture failed, no `RESET_OK`. |
| 26 | Partial-state schema/role/isolation queries | Exit `0`; three schemas and isolation matrix detected. |
| 27 | Manual URL-last `psql` diagnostic | Exit `0`; confirmed PostgreSQL/SQL were sound. |
| 28 | Server-side structural-validation runner patch | Exit `0`; approved migration remained unchanged. |
| 29 | Independent syntax/focused rerun | Exit `0`; 10 independent tests pass. |
| 30 | Guarded recovery attempt against partial database | First invocation rejected unknown `--confirmation` argument; corrected invocation reached PostgreSQL but exited `1` on checkpoint signal. |
| 31 | `pg_ctl -m fast stop` | Exit `1`; Windows operation not permitted; readiness remained `0` (accepting). |
| 32 | Positively identified `Stop-Process` and readiness probe | Stop exit `0`; `pg_isready` exit `2` (no response). |
| 33 | Exact temporary data-directory verification/removal | Exit `0`; only the expected disposable `data` directory removed. |
| 34 | Blank `initdb` with both PostgreSQL and DLL directories on `PATH` | Cluster creation succeeded; post-success restricted-token messages recorded. |
| 35 | Hidden native server start and `pg_isready` | Server live on loopback; readiness exit `0`. |
| 36 | Blank identity/database query | Exit `0`; target absent. |
| 37 | First successful guarded initialization | Exit `0`; migration, seed, server-side validation, and `RESET_OK`. |
| 38 | Deep schema/role/access/banned-field/tracking query | Exit `0`; results in Sections 8-9. |
| 39 | First default-ACL query | Exit `1` due ambiguous PostgreSQL `"char"` concatenation; no state change. |
| 40 | Corrected default-ACL/search-path query | Exit `0`; owner-scoped table/sequence grants verified. |
| 41 | Target migration static validator and runner dry run | Both exit `0`; one unique paired target migration. |
| 42 | First `pg_dump -X` attempt | Exit `1`; `-X` is not a `pg_dump` option; no evidence claimed. |
| 43 | Corrected first `pg_dump --schema-only` and normalization | Exit `0`; normalized hash `4e9e8df5...6d81`; two random client keys normalized. |
| 44 | Stop first server, verify port closed, remove first data directory | Exit `0`; only the disposable cluster removed. |
| 45 | Second `initdb`, start, readiness, and blank identity | Exit `0`; target database count `0`. |
| 46 | Second guarded initialization and final live SQL | Exit `0`; `RESET_OK`, owners/access/domain/banned/tracking results pass. |
| 47 | Second schema dump, normalization, and comparison | Exit `0`; hashes equal; `STRUCTURAL_EQUIVALENCE=true`. |
| 48 | Graceful stop, direct fallback, final readiness | `pg_ctl` exit `1`; direct stop exit `0`; `pg_isready` exit `2`. |
| 49 | Phase 0 Node syntax and focused/regression block | Exit `0`; 5 + 4 + 8 + 10 + 11 tests passed. |
| 50 | `go version` | Exit `0`; `go1.25.4 windows/amd64`. |
| 51 | `go test ./packages/db/migrations_test.go -v` | Exit `0`; 5 passed. |
| 52 | `go vet ./packages/db/migrations_test.go` | Exit `0`; sandbox telemetry token warnings only. |
| 53 | Current Docker/native runtime/package/port preflight replay | Wrapper exit `0`; Docker daemon inner exit `1`; temporary PostgreSQL 16.14 tools exit `0`; port probe exit `2` after stop. |
| 54 | Persist normalized structural evidence and verify hashes | Exit `0`; only terminal-newline formatting differs from the runtime dump; schema content preserved. |
| 55 | Exact runtime/cache cleanup and local port/process checks | Exit `0`; runtime/cache/credential absent; zero listener and visible PostgreSQL processes. |
| 56 | Post-cleanup Docker test-container query | Exit `1` because daemon is inaccessible; no Docker cleanup pass claimed and no container was ever created. |
| 57 | Scoped documentation/report/evidence patches | Built-in patching was rejected before mutation by the Windows sandbox; no-index unified diffs exited `0`. |
| 58 | Post-report FND-001 through FND-005 Node regression block | Exit `0`; 5 + 4 + 8 + 10 + 11 tests passed. |
| 59 | First combined link/task/secret/scope validator | Exit `1`; it misclassified historical README examples, `EXAMPLE-DOC-001`, and `SHA-256`; no product finding or mutation. |
| 60 | Corrected link/task/secret/scope validator | Exit `0`; 4 Markdown files, 42 local links, 33 roadmap IDs, 0 broken/style/unknown/secret findings, and exactly 7 allowed changed files. |
| 61 | Markdownlint availability probe | Exit `0`; `markdownlint` and `markdownlint-cli2` unavailable; no markdownlint pass claimed. |
| 62 | Final `.git`, Phase 1, `SEC-001`, runtime, port, process, and decision checks | Exit `0`; no prohibited state or work; current `PASS` and paid-production `NO-GO` verified. |

## 17. Explicit remediation decision

# `PASS`

The previously missing real Fresh Install evidence now exists: two positively
identified isolated PostgreSQL 16.14 blank clusters were initialized; the one
approved target migration and seed ran; live roles, owners, grants, isolation,
and absent forbidden domain sources were validated; a real non-zero partial
failure was detected and recovered; and normalized schema exports were
structurally identical.

Final confirmations:

- Phase 1 was not started.
- `SEC-001` was not started.
- No other roadmap task was started.
- No Git metadata was created.
- No remote or production operation occurred.
- No application or infrastructure behavior changed.
- Paid-production status remains `NO-GO`.
