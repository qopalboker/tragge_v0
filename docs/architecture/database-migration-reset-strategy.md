# Disposable-database migration reset strategy

**Status:** Approved FND-004 strategy; target schema foundation only

**Date:** 2026-07-25

**Paid-production status:** `NO-GO`

## Authority, scope, and present state

This strategy applies the
[fixed product and technical policies](../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md),
the [canonical glossary](../product/canonical-domain-glossary-and-version-catalog.md),
and [ADR-0001](../adr/0001-target-runtime-architecture.md). The
[legacy migration inventory](migration-inventory.md) is the traceability record.
The [production roadmap](../codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md) assigns
all later behavior.

The current pre-launch database is disposable. The top-level legacy chain has
99 up/down pairs (98 at FND-004 plus the SEC-004 canonical-role bridge) and remains evidence of current behavior only. It is **not**
the approved target schema. A fresh target foundation now exists in the
`target` child directory, but it creates only ownership schemas and grants. No
Platform, Trading Engine, or Market Data domain table is claimed implemented by
FND-004. The canonical database schema/version remains **Not assigned
(planned)** until the responsible later tasks implement the declared contract
and its golden schema evidence.

This task changes no application, financial, authentication, Contest, Trading
Engine, Market Data, payment, or frontend behavior. Current applications still
expect the legacy shared `public` schema and are not declared compatible with
the target foundation.

## Evidence from the legacy schema

The following conflicts require a clean replacement rather than indefinite
compatibility:

| Conflict | Repository evidence | Target disposition |
|---|---|---|
| Duplicate Platform Fee sources | [`0001_init`](../../packages/db/migrations/0001_init.up.sql) has `platform_fee_bps`; [`0015_flexible_contest_config`](../../packages/db/migrations/0015_flexible_contest_config.up.sql), [`0046_contest_commission_rate_default`](../../packages/db/migrations/0046_contest_commission_rate_default.up.sql), and [`0047_prize_lock`](../../packages/db/migrations/0047_prize_lock.up.sql) add or persist `commission_rate`. | DATA-002 leaves one canonical `platform_fee_bps = 2000`; no target `commission_rate` for Contest economics. |
| Duplicate/conflicting Prize Pool | [`0018_tournament_calendar`](../../packages/db/migrations/0018_tournament_calendar.up.sql) calculates a live pool; [`0019_settlement_tables`](../../packages/db/migrations/0019_settlement_tables.up.sql) stores gross/net pools; [`0047_prize_lock`](../../packages/db/migrations/0047_prize_lock.up.sql) stores another lock; [`0063_contests_template_fields`](../../packages/db/migrations/0063_contests_template_fields.up.sql) describes a dynamic pool. | CON-003 owns one immutable `contest_economics_snapshot`; PRIZE-007 reconciles exactly to its Prize Pool. |
| Obsolete prize distribution | [`0064_template_prize_distributions`](../../packages/db/migrations/0064_template_prize_distributions.up.sql) and [`0079_template_entry_tiers`](../../packages/db/migrations/0079_template_entry_tiers.up.sql) permit independent percentage tables; score migrations implement old contribution formulas. | PRIZE-001 through PRIZE-004 implement only `tralent_v1`; PRIZE-009 removes Power Law/obsolete paths. |
| Product Participant Capacity | [`0008_free_tournaments`](../../packages/db/migrations/0008_free_tournaments.up.sql) creates `max_participants`; [`0016_contest_state_machine`](../../packages/db/migrations/0016_contest_state_machine.up.sql) maintains `current_participants` for capacity; archive/template migrations copy the fields. | No target product capacity field. CON-003 may snapshot `real_participant_count`; infrastructure circuit breakers remain operational configuration. |
| Ambiguous quantity | [`0001_init`](../../packages/db/migrations/0001_init.up.sql) uses `qty_total` and trading `qty`; later participant counters are separate but capacity comments overload quantity concepts. | People use Real Participant count. Trading Engine owns integer Trading QTY named `qty`; unqualified `quantity` is prohibited. |
| Second Chance | Case-insensitive static search of migration and init SQL found no Second Chance field, table, flag, price, adjustment, or state. | Keep absent. A focused validator rejects active target occurrences; no compatibility column or import mapping is permitted. |
| Duplicate finalization ownership | [`0019_settlement_tables`](../../packages/db/migrations/0019_settlement_tables.up.sql) and [`0030_finalization_tracking`](../../packages/db/migrations/0030_finalization_tracking.up.sql) duplicate state while legacy leaderboard code reads Engine tables. | PRIZE-005 makes Platform Settlement the sole finalization owner; Leaderboard Projection is rebuildable only. |
| Wallet without immutable double-entry backing | [`0004_wallet`](../../packages/db/migrations/0004_wallet.up.sql) stores mutable balance and a single ledger row per movement; later enum/reason migrations extend it. | DATA-003/DATA-004 implement immutable balanced transactions/postings, Available/Reserved accounts, idempotency, and compensating entries. |
| Redis source-of-truth assumptions | SQL does not persist Redis data, but [`0067_contest_schedule_dedup`](../../packages/db/migrations/0067_contest_schedule_dedup.up.sql) describes PostgreSQL as backing a Redis fast-dedup path. | PostgreSQL constraints are authoritative. Redis remains cache, fan-out, rate-limit, or coordination only. |
| Cross-bounded-system ownership | All legacy domain tables share `public`; contests carry `shard_id`, Platform settlement queries trading tables, and symbols/provider config coexist with Platform data. | Platform, Engine, and Market Data Service receive exclusive schemas, roles, and credentials; cross-system data arrives only through versioned events. |
| Shared User/Admin auth storage | Admin roles and legacy TOTP fields are added to the same [`users`](../../packages/db/migrations/0001_init.up.sql) table by later migrations. | SEC-001 and ARCH-002 establish isolated User/Admin credentials and sessions; SEC-004 adds sensitive-action reauthentication, while planned SEC-007 introduces target Super Admin MFA without preserving the shared legacy model. |
| Floating or unversioned financial representation | [`0005_candles`](../../packages/db/migrations/0005_candles.up.sql) uses `DOUBLE PRECISION`; KYC scores also use double; money/price/PnL/score migrations mix `BIGINT`, `DECIMAL`, and `NUMERIC` without one scale contract. | DATA-001, ENG-002, and MD-001 introduce integer/fixed-point primitives with explicit scale/version; binary floating point is forbidden at financial boundaries. |
| Obsolete runtime/service ownership | `shard_id`, provider-specific symbol columns, public outbox-like events, and compatibility views encode the merged wrappers and direct-table era. | ARCH-001/ARCH-006, ENG-001, and MD-001 assign state to exactly one owner; no service/runtime ownership column substitutes for a schema boundary. |

No migration contains active Second Chance schema. That absence is preserved;
it is not evidence that any other legacy semantics are approved.

## Clean target baseline contract

### Schemas, roles, credentials, and grants

One PostgreSQL cluster is permitted initially. The schema and credential
boundary is still mandatory:

| Bounded system | Schema | Non-login owner role | Runtime group role | Authority |
|---|---|---|---|---|
| Platform Modular Monolith | `platform` | `platform_owner` | `platform` | Platform state, local projections, audit, and Platform outbox/inbox only |
| Trading Engine | `engine` | `engine_owner` | `engine` | Orders, fills, positions, sessions, WAL/snapshot metadata, and Engine outbox/inbox only |
| Market Data Service | `market_data` | `market_data_owner` | `market_data` | Registry/provider state, quality/source state, retention indexes, and Market Data outbox/inbox only |

`tragge_migrator` is a non-login group role with membership in the three owner
roles. A deployment supplies a short-lived migration login and grants it only
that group role for the migration window. Each runtime login receives a unique
secret and membership in exactly one runtime group role. Login users and
passwords are never embedded in SQL or shared across bounded systems.

The implemented
[`01-cluster-roles.sql`](../../packages/db/init/target/01-cluster-roles.sql)
creates only non-login group roles. The target
[`0001_schema_ownership.up.sql`](../../packages/db/migrations/target/0001_schema_ownership.up.sql)
creates the three schemas, revokes `PUBLIC`, grants each runtime only its schema,
and configures owner-specific default DML privileges. It creates no domain
table. Cross-schema grants, foreign keys, views, functions, triggers, joins, or
runtime reporting credentials are prohibited. The `public` schema owns no
target domain state.

### Naming, ordering, and migration ownership

- SQL identifiers use lowercase `snake_case`; primary keys end `_id`, UTC
  instants end `_at`, integer minor-unit money ends `_minor`, basis points end
  `_bps`, and scaled integer values end `_units` with an adjacent or contract-
  fixed scale/version.
- Target migration files use `NNNN_<owner>_<imperative>.up.sql` and a matching
  down file when a development down path is safe. The owner token is
  `platform`, `engine`, `market_data`, or `shared` only for the initial role/
  schema foundation. Identifiers are four-digit, unique, monotonically
  increasing, and never reused.
- Cluster role provisioning runs first, target SQL runs in filename order,
  versioned reference seeds run last, and structural/schema-diff validation
  follows. The current target chain contains only migration `0001`; later
  roadmap tasks append, never renumber it.
- Every migration declares one owner. `tragge_migrator` applies reviewed DDL;
  runtime credentials cannot create, alter, or drop schema objects.
- The current database baseline version remains unassigned. ARCH-006 introduces
  per-owner version/checksum recording before target domain migrations are
  accepted. It records identifier, SHA-256 checksum, applied UTC time, actor,
  and execution duration; an applied checksum mismatch is a release failure.

### PostgreSQL conventions

- PostgreSQL 16 is the declared current infrastructure major. The foundation
  requires no extension. A later extension must be pinned, schema-qualified,
  justified, and verified in fresh-install/restore tests. The legacy
  `uuid-ossp` extension is not implicitly carried forward.
- Domain identifiers are opaque UUIDs generated by the owning application or a
  later explicitly approved database primitive. DATA-001 fixes serialization;
  this task does not invent a UUID revision. Sequence/identity values may be
  used only for owner-local append ordering, never as cross-system identity.
- Store instants as `TIMESTAMPTZ` normalized to UTC. Do not store canonical
  instants as timezone-free timestamps. Scheduler calculation uses
  `Asia/Tehran`, then persists UTC.
- Prefer check constraints or owner-local reference tables for changeable state
  sets. PostgreSQL enums are allowed only for truly append-only, owner-local
  sets with an explicit compatibility plan; removing/renaming enum values
  requires a forward replacement migration.
- Canonical money uses signed `BIGINT` integer minor units with one scale from
  DATA-001. Platform Fee uses integer `platform_fee_bps = 2000`. Canonical
  prices, rates, P&L, and T-Score use signed fixed-point integer units plus the
  versioned scale defined by DATA-001/MD-001/ENG-002. `REAL`, `FLOAT`, and
  `DOUBLE PRECISION` are prohibited; arbitrary decimal percentages are not
  canonical financial sources.

### Planned owner data

The following is a contract and task allocation, not a claim of implemented
tables:

| Owner | Planned target records/invariants | Responsible tasks |
|---|---|---|
| Platform | Separate User/Admin authentication records, profile/KYC, roles/permissions, Contest/immutable Scheduler Template Version, joins, economics snapshot, Wallet/ledger, deposits/withdrawals, Settlement, Leaderboard Projection, notifications/support, audit | ARCH-001 through ARCH-005; SEC-001 through SEC-005; DATA-001 through DATA-005; CON-001 through CON-005; PRIZE-001 through PRIZE-008 |
| Trading Engine | Contest sessions, participant activation, orders, fills, positions, integer Trading QTY reservations, fixed-point score/PnL, command dedupe, WAL/snapshot/result metadata | ENG-001 through ENG-008 |
| Market Data Service | Asset Group/Symbol registry, Provider capabilities/configuration, quality/sequence/source epochs, canonical ticks/candles, raw-retention indexes | MD-001 through MD-007 |
| Every owner | Transactional outbox with unique event ID and versioned envelope; transactional inbox with unique consumer/event identity; quarantine/dead-letter evidence | ARCH-006 plus the owning contract task |

There is one Platform Fee source and one Prize Pool source. There is no
`commission_rate` Contest source, `gross_prize` source, product Participant
Capacity field, or Second Chance model. `real_participant_count` is an immutable
economics input, not capacity. Platform may store a Contest's maximum Trading
QTY configuration, while Engine exclusively owns reservation and enforcement.

### Immutability, reconstruction, and constraints

- DATA-003/DATA-004 must create balanced ledger transactions/postings in one
  transaction. Posted rows are immutable; corrections are compensating entries.
  Available and Reserved Balance are ledger-derived, never mutable Redis truth.
- CON-003 creates one immutable `contest_economics_snapshot` after Join Cutoff.
  PRIZE-006 creates the immutable Engine result/final-price evidence. PRIZE-005
  through PRIZE-008 make Settlement idempotent and reconstructable from those
  inputs; Leaderboard Projection never posts prizes or completes a Contest.
- Each owner writes a domain change and outbox row atomically. A consumer writes
  its inbox identity and local side effect atomically. Unique constraints enforce
  idempotency; no cross-owner foreign key or trigger is allowed.
- Audit records are append-only, actor/correlation aware, UTC timestamped, and
  redacted. Financial, security, KYC, provider-switch, migration, and privileged
  Admin actions require immutable evidence.
- Constraints reject negative/overflowed money where the domain forbids it,
  invalid basis points, non-positive Trading QTY, unbalanced ledger postings,
  duplicate Idempotency Keys, duplicate inbox event IDs, and mutation of locked
  snapshots. Exact constraints arrive with their owning task and focused tests.
- Schema reconstruction uses reviewed migrations plus versioned seeds. State
  reconstruction uses owner snapshots/events/WAL as applicable. Backups and
  restore drills remain mandatory; Redis is never reconstruction input.

### Seed and reference-data policy

The FND-004 seed
[`02_reference_data.seed.sql`](../../packages/db/init/target/02_reference_data.seed.sql)
is an explicit no-op marker: it proves ordering without pretending later data
exists. Later tasks add stable, versioned seeds in owner-controlled migrations.
Seeds use immutable identifiers, `INSERT ... ON CONFLICT` only when the expected
row is byte-for-byte compatible, and fail on conflicting content.

DATA-005 creates the one persistent System Participant account and its exclusion
classification. SCH-003 creates that account's registration for each Free
Practice Contest. The account is not a real user, cannot join paid Contests, is
not prize/economics/winner/Official Ranking eligible, and cannot place user-
initiated orders. FND-004 does not seed it early or invent its schema.

## Guarded reset and cutover workflow

The guarded runner is
[`scripts/database-reset.mjs`](../../scripts/database-reset.mjs). Its default is
dry-run. It refuses destructive execution unless all of these hold:

- `--environment`, `TRAGGE_ENV`, and `APP_ENV`, when present, agree on an
  explicitly approved development, local, test, staging, or preproduction
  spelling; any production signal or disagreement is rejected;
- database name matches the approved `tragge_` or `app_` development/test/
  staging pattern;
- the URL explicitly contains PostgreSQL protocol, host, user, and database;
- a non-local host is repeated exactly with `--allow-host`;
- `--confirm-database` exactly equals the URL database;
- `TRAGGE_DATABASE_RESET_CONFIRM` exactly equals
  `I_UNDERSTAND_THIS_DESTROYS_DATA`;
- `--execute` is present.

The URL is read from `TRAGGE_TARGET_DATABASE_URL`; the runner prints only the
user, host, port, and database, never the password. It uses a five-second
connection timeout, 30-second statement timeout, five-second lock timeout, and
`ON_ERROR_STOP`.

### One-command-chain fresh install

In an environment with PostgreSQL client tools and an isolated database, set an
administrative URL through the approved secret mechanism, then run:

```powershell
$env:TRAGGE_ENV = 'test'
$env:TRAGGE_TARGET_DATABASE_URL = '<admin PostgreSQL URL ending in /tragge_fnd004_test>'
$env:TRAGGE_DATABASE_RESET_CONFIRM = 'I_UNDERSTAND_THIS_DESTROYS_DATA'
node scripts/database-reset.mjs --execute --confirm-database tragge_fnd004_test
```

For a non-local preproduction host, additionally pass `--allow-host` with the
exact hostname. This does not permit production and does not relax the database
name or confirmation guards.

The workflow is:

1. **Prerequisites:** Run focused/static validation and prior FND regressions;
   require Node, `psql`, PostgreSQL 16 compatibility, administrative
   `CREATEDB`/`CREATEROLE`, reviewed target URL, and no production credentials.
2. **Backup/export decision:** Default is no export because development data is
   disposable. If an owner explicitly requires evidence, take a dated encrypted
   `pg_dump`, record checksum/retention/owner, and keep it outside the new DB.
3. **Positive target confirmation:** Record environment, sanitized host/port,
   exact database name, approver, and that no production or real-user database
   is targeted. Run the runner once without `--execute` and review its plan.
4. **Drop/recreate:** The execute mode connects to maintenance database
   `postgres`, runs idempotent group-role provisioning, terminates only sessions
   for the exact guarded database, then drops and recreates only that database.
5. **Baseline application:** Apply target up files in deterministic filename
   order with `ON_ERROR_STOP`. Never apply the 99-file top-level legacy chain to
   a target database.
6. **Reference seed:** Apply sorted `*.seed.sql` files. FND-004 is a no-op; later
   owner tasks add versioned reference data and System Participant creation.
7. **Schema validation:** Query schema owners and runtime grants, then compare a
   normalized schema-only dump to the reviewed golden target once it exists.
   The current runner asserts the three foundation owners and proves each
   runtime role has usage only on its owned schema.
8. **Application startup validation:** After ARCH/DATA/CON/ENG/MD migrations are
   implemented, start each runtime with only its credential, run readiness and
   critical integration journeys, and prove cross-schema access fails. Current
   legacy applications are expected to be incompatible; FND-004 does not claim
   this step passed.
9. **Failure recovery:** Stop writers, preserve `psql` output and partial schema
   evidence, discard the failed isolated database, correct only an unapplied
   migration, and rerun from a blank database. For production after launch,
   restore the verified backup/PITR point or deploy a compensating forward
   migration; do not improvise a destructive down.
10. **Evidence:** Store command/result, migration checksums, schema-version rows,
    normalized schema diff, grants query, seed report, startup/readiness output,
    and backup/restore reference in the task/release report. A mismatch is FAIL.

Never run a reset against an unknown URL or any database with real-user data.

## Future migration and rollback policy

1. Production migrations are forward-only by preference. An applied production
   migration is immutable: never silently edit, reorder, rename, or reuse it.
2. A down migration is allowed only for isolated development/test reset or
   during a documented pre-write deployment rollback window when it is proven
   lossless and compatible. Destructive downs are forbidden after new writes.
3. After the rollback window or for any data transformation, use a reviewed
   compensating forward migration. Production recovery ownership belongs to the
   bounded-system owner and release operator; financial repair additionally
   requires ledger/reconciliation ownership.
4. DDL is transactional unless PostgreSQL explicitly forbids it. A non-
   transactional migration declares that fact, checkpoint/retry semantics, and
   cleanup. It cannot mix unrelated changes.
5. Set finite `lock_timeout` and `statement_timeout`; inspect blocking locks and
   estimated duration before deployment. Never wait indefinitely or use an
   unreviewed table lock during active trading/settlement.
6. Data backfills are separate, versioned, idempotent/restartable jobs with
   bounded batches, checkpoints, rate limits, invariant checks, and a final
   reconciliation. Do not hide a large backfill in DDL.
7. Large-table changes use expand/backfill/verify/switch/contract. Prefer new
   nullable columns, concurrent indexes in an explicitly non-transactional
   migration, and delayed constraint validation. Contract only after rollback
   and old-reader windows close.
8. Names follow the target convention above. One migration has one schema owner
   and one concern. All environments apply the same immutable bytes.
9. Applied versions and SHA-256 checksums are recorded per owner. Drift, an
   unknown migration, duplicate ID, missing predecessor, or checksum mismatch
   fails deployment.
10. Fresh-install testing applies every target migration and seed to a blank
    PostgreSQL instance. Upgrade testing begins from each explicitly supported
    release snapshot; no support is implied for arbitrary pre-launch dev data.
11. Every migration change includes static naming/order checks, SQL parse/apply
    evidence, owner/grant checks, normalized schema diff, and relevant
    integration tests. Rollback/restore responsibility and recovery point are
    named before rollout.

## Legacy import policy

Default: **no legacy data import and no compatibility preservation merely for
old development data**.

If an explicit future decision requires import, it is a separate versioned tool
and release step, never a baseline migration or automatic startup hook. It must:

- accept only a declared source schema version/checksum and reject unknown drift;
- read from an isolated, read-only source and write only through target owner
  staging/application interfaces;
- be idempotent or restartable with durable checkpoints and stable source keys;
- map each row to one owner without cross-schema foreign keys or dual truth;
- reject `commission_rate`, capacity, Second Chance, duplicate Prize Pool,
  floating financial values, mutable ledger history, and other invalid target
  semantics rather than weakening constraints;
- reconcile source/accepted/rejected row counts and all financial totals in
  integer units, including ledger balance, Prize Pool, payouts, deposits, and
  Withdrawals;
- generate a signed/versioned import report with source checksum, mapping
  version, timing, rejects, totals, and operator approval;
- run target schema, ledger, Settlement, and reconstruction validation before
  imported data is exposed.

No speculative importer is implemented by FND-004.

## Validation boundaries and remaining work

Focused validation checks the 99-row inventory, deterministic ordering,
classification uniqueness/totals, target file pairing, guard refusal paths,
canonical financial/schema terms, roadmap IDs, repository paths, ADR ownership,
and local Markdown links. The foundation SQL can also be parsed/applied by
PostgreSQL when `psql` is available.

The current environment must not be credited with a real fresh database unless
execute-mode output and schema queries actually ran. Docker availability alone
is not database evidence. Full domain schema diff, current-application startup,
ledger invariants, outbox/inbox behavior, System Participant creation, and
legacy import behavior remain unimplemented and owned by the roadmap tasks
listed above. Paid production remains `NO-GO`.
