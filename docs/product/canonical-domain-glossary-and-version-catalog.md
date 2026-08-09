# Canonical domain glossary and version catalog

**Status:** Approved terminology and version baseline

**Catalog version:** `2026-08-09.1`

**Date:** 2026-08-09

**Scope:** Backend, frontend, SQL, contracts, tests, and technical documentation

## Authority and use

This document applies the
[Fixed Product and Technical Policies](FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md)
and [ADR-0001](../adr/0001-target-runtime-architecture.md). The
[Production Roadmap and Independent Codex Tasks](../codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md)
assign implementation work. Those sources retain their documented precedence;
this catalog does not add product rules, financial formulas, or bounded systems.

New identifiers, APIs, database fields, UI labels, events, tests, and technical
documentation use the canonical terms below. Existing names that conflict are
legacy evidence, not alternative definitions. They remain visible in the
[repository baseline](../architecture/current-state-audit.md) and the
remediation register until their owning roadmap tasks migrate them.

Status words in the version catalog mean:

- **current**: the named document or decision exists and is the approved source;
  it does not imply that legacy implementation already conforms.
- **planned**: the target is approved but its canonical implementation artifact
  or version has not been introduced.
- **legacy / noncanonical**: the artifact exists for compatibility or evidence
  but must not be selected as the approved target.
- **deprecated**: new uses are prohibited and existing uses have an explicit
  migration target.

## Stable identifier conventions

Stable identifiers are assigned only by an approved source or the roadmap task
that introduces the versioned artifact:

1. Policy and roadmap documents use `YYYY-MM-DD.revision`, as in
   `2026-07-25.1`.
2. Architecture decisions use `ADR-NNNN`, as in `ADR-0001`.
3. Named rule/configuration families use lowercase ASCII snake case followed by
   `_vN`, where `N` is a positive major version, as in `tralent_v1`. A behavior
   change requires a new identifier; an old locked contest keeps its recorded
   identifier.
4. Wire schemas use `vN`. JSON schema filenames use
   `{message_name}.vN.json`, code packages use `/vN`, and the serialized
   envelope carries the matching schema version. Breaking compatibility
   requires a new major version.
5. Versioned database templates and registries expose an immutable
   `{entity}_version_id`. A generated contest records the immutable version ID
   and all rule identifiers that governed it. The implementing task chooses the
   storage representation; this document does not fabricate one.
6. Immutable snapshot formats must carry an assigned schema version plus their
   stable record identity or content hash. No schema number is assigned before
   its owning task defines and tests the format.
7. Migration filenames currently use `NNNN_name.up.sql` and
   `NNNN_name.down.sql`. The canonical clean-baseline identifier remains
   unassigned until `FND-004` decides it.

`Not assigned (planned)` is intentional. It is not version `0`, `v1`, or a
permission to infer a number from a legacy artifact.

## Canonical glossary

### Architecture and runtime terms

| Term | Canonical meaning |
|---|---|
| **Platform Modular Monolith** | One backend bounded system and one codebase, image, and release version owning identity, contest/scheduler, wallet/ledger, payments/KYC/withdrawals, settlement orchestration, leaderboard projection, notifications/support, and admin/audit. Its runtime modes are deployment units, not domain microservices. |
| **Platform API runtime** | The `platform --mode=api` deployment of the Platform image. It serves public and administrative HTTP APIs and invokes Platform application interfaces in process. |
| **Platform Realtime runtime** | The `platform --mode=realtime` deployment of the same Platform image. It owns authenticated realtime connections and delivers Platform projections/versioned events without querying Engine or Market Data tables. |
| **Platform Worker runtime** | The `platform --mode=worker` deployment of the same Platform image. It runs Platform schedulers, settlement orchestration, projections, notifications, and asynchronous jobs through in-process Platform modules. |
| **Trading Engine** | The independent bounded system owning orders, fills, positions, Trading QTY reservation, trading score, pending/TP/SL execution, contest sessions, WAL, snapshots, and deterministic replay. It is not part of Market Data or Platform. |
| **Market Data Service** | The independent bounded system owning provider adapters, symbol normalization, provider health/selection, price quality/sequence/gap/stale detection, source switching, tick/candle publication, and raw dispute-retention data. It is not part of Trading Engine. |

### Contest, scheduling, participation, and QTY terms

| Term | Canonical meaning |
|---|---|
| **Contest** | One generated trading-tournament instance with one Asset Group, start/end times, duration, Base Entry Fee, maximum Trading QTY, late-entry policy, and recorded policy/version identifiers. Instance fields fixed by policy become immutable when the first Real Participant joins. |
| **Contest Template** | The logical Admin-managed scheduler definition whose edits create new immutable Scheduler Template Versions. A template is not a Contest instance and edits do not mutate already generated contests. |
| **Scheduler Template Version** | One immutable revision of a Contest Template, referenced by every Contest it generates through `schedule_template_version_id`. It fixes market, duration, cadence, fee set, maximum Trading QTY, late-entry flag, and enabled/launch-profile configuration for that revision. |
| **Custom Contest** | A Super Admin-created Contest whose start, duration, Asset Group, Base Entry Fee, Trading QTY choice, and late-entry flag are supplied explicitly. It may overlap scheduled contests and uses the same fixed Platform Fee and lifecycle/economics rules. |
| **Upcoming** | A generated Contest whose scheduled start has not occurred. It exists only in Platform storage and consumes no Trading Engine session. It is a query/display category, not proof that joining is authorized. |
| **Registration Open** | A derived join-authorization condition, not the sole Contest lifecycle state. An eligible user may join only when the applicable join window is open; a paid Running Contest may still be Registration Open until Join Cutoff, while Free Practice closes registration at start. |
| **Running** | The active trading lifecycle condition after a qualified Contest starts and before its end/freeze. Running does not imply registration is closed: a paid Contest can accept permitted Late Entry before Join Cutoff. |
| **Join Cutoff** | The stored `join_cutoff_at` timestamp after which no new participant may join. Paid late entry uses the policy formula; Free Practice uses start time. The Economics Lock occurs immediately after the join window closes. |
| **Late Entry** | A permitted paid-contest join after start and before Join Cutoff when the Contest enables it. It charges the Base Entry Fee plus the Late-Entry Surcharge; it is not available for Free Practice. |
| **Real Participant** | A real user registered in a Contest. Real Participant count controls paid start qualification, economics, Planned Winners, and Engine activation. System Participants are excluded. |
| **Free Practice Contest** | A one-hour, zero-entry-fee Crypto or Forex Contest with no Prize Pool, prize table, or Official Ranking impact; maximum Trading QTY is 10 and entry after start is disabled. |
| **System Participant** | The persistent practice system account's registration in a Free Practice Contest. It is not a real user, displays rank `0`, cannot join paid contests or place user-initiated orders, and is never prize/economics/winner/Official Ranking eligible. |
| **Participant Capacity** | Product-level Participant Capacity does not exist. Fields such as `max_participants`, `participant_capacity`, or a UI capacity label are deprecated and must be removed. Operational circuit breakers are infrastructure safety limits and must be named as such, not exposed as Contest capacity. |
| **Quantity (unqualified)** | An ambiguous term prohibited in domain fields and UI labels. Use Trading QTY for order/position reservation and Real Participant count for people. Never use `quantity` to mean both concepts. |
| **Trading QTY** | The integer-only order/position resource owned by Trading Engine, conventionally represented as `qty`. Pending orders and active positions reserve it; total reserved QTY cannot exceed the Contest's maximum Trading QTY. It never means participant count. |

### Money, economics, prize, and scoring terms

| Term | Canonical meaning |
|---|---|
| **Base Entry Fee** | The USDT-denominated Contest entry amount before any Late-Entry Surcharge. For a regular paid entry, 20% is Platform Fee and 80% contributes to Prize Pool. |
| **Late-Entry Surcharge** | The additional amount charged for Late Entry, equal to 10% of Base Entry Fee. It is entirely Platform revenue and contributes nothing to Prize Pool. |
| **Platform Fee** | The Platform's 20% share of Base Entry Fee. The sole canonical base-fee field is `platform_fee_bps = 2000`; `commission_rate` is not a source of truth. |
| **Prize Pool** | The locked, USDT-denominated amount fully distributable to eligible winners. Regular paid entries contribute 80% of Base Entry Fee; Late-Entry Surcharge contributes zero. Final prize payouts must equal it exactly in minor units. |
| **Gross Prize** | A deprecated ambiguous label with no separate canonical financial amount. Use Prize Pool for distributable winner funds and `gross_base_entry_total` for the sum of Base Entry Fees before fee split. Do not create a `gross_prize` source of truth. |
| **Economics Lock** | The idempotent operation immediately after Join Cutoff that freezes Real Participant count, gross base entry total, Base Platform Fee, Late-Entry Surcharge revenue, net Prize Pool, Planned Winners, and all governing rule versions in `contest_economics_snapshot`. |
| **Filled Trade** | A trade for which Trading Engine recorded an execution fill. An accepted, pending, canceled, or otherwise unfilled order is not a Filled Trade. At least one Filled Trade is required for prize eligibility. |
| **Planned Winners** | The winner-slot count computed from all locked Real Participants under `tralent_v1`, before Filled Trade eligibility is applied. It is recorded in the economics snapshot. |
| **Eligible Users** | Real Participants with at least one Filled Trade in the immutable settlement input. System Participants and no-trade users are excluded from the prize table. |
| **Actual Winners** | `min(planned_winners, eligible_ranked_users)`. If fewer Eligible Users exist, occupied Reward Weights are preserved and renormalized so the entire Prize Pool is distributed. |
| **Rank Band** | A versioned consecutive-rank grouping in `tralent_v1`. Ranks 1 through 10 are individual bands; later canonical ranges are grouped buckets whose share is divided equally among occupied ranks. |
| **Reward Weight** | The prize-distribution weighting value named `reward_weight`, defined by policy as Real Participant count multiplied by individual prize share. It is part of `tralent_v1` allocation terminology and is not T-Score. |
| **T-Score** | The canonical cumulative Contest performance/ranking score generated by Trading Engine from simulated trading results. T-Score must not name Reward Weight, prize share, participant count, or Platform revenue. |
| **Official Ranking** | The ranking output that may affect recognized competitive results. Free Practice and System Participants have no Official Ranking impact. Rank `0` is a System Participant display convention, not an official competitive rank. |
| **Commission Rate** | Deprecated for Base Entry Fee economics. Existing `commission_rate` fields/read fallbacks migrate to Platform Fee in integer basis points, specifically `platform_fee_bps`. Unrelated provider/affiliate rates must be explicitly qualified and cannot substitute for Platform Fee. |

### Wallet, settlement, projection, and control terms

| Term | Canonical meaning |
|---|---|
| **Wallet** | Platform's internal USDT-denominated accounting view backed by the Double-Entry Ledger. It does not hold private keys or generate/manage blockchain deposit addresses. |
| **Available Balance** | Confirmed Wallet value currently spendable by the user. Paid Contest join requires sufficient Available Balance. It is derived from authoritative ledger postings, not Redis. |
| **Reserved Balance** | Wallet value moved out of Available Balance for a pending obligation, such as a withdrawal or other explicitly modeled hold. It is represented by ledger accounts/postings and is not independently mutable cache state. |
| **Double-Entry Ledger** | Platform's immutable accounting record in which every transaction has balancing postings. Corrections use compensating entries; existing rows are never edited. |
| **Settlement** | The sole owner of final Contest completion and payout: freeze, close/barrier, immutable Engine result, final eligible ranking, `tralent_v1` allocation, ledger posting, Prize Pool reconciliation, completion, and final events. |
| **Settlement Review** | The non-completed condition/workflow used when final quote or immutable evidence is insufficient or a rank-affecting issue exists. Resolution uses audited, authorized compensating action and never mutation of history. |
| **Leaderboard Projection** | A rebuildable read model of live/final rank and score. It does not own Settlement, complete Contests, calculate an independent final prize table, credit Wallets, or own payout retry state. |
| **Reconciliation** | A deterministic comparison of authoritative records that proves expected totals/events equal actual ledger, payment, settlement, scheduler, or state outputs and reports every unexplained difference. It never auto-edits history to force equality. |

### Market Data terms

| Term | Canonical meaning |
|---|---|
| **Provider** | An external market-data source behind a Market Data Service adapter. Production enablement requires verified coverage, rights, timestamps, sequence/rate limits, and symbol mapping; provider identity is admin-only. |
| **Asset Group** | Exactly one of `crypto` or `forex` for launch. A Contest selects one Asset Group, Market Data selects an active Provider independently per Asset Group, and `commodity` is not a launch group. |
| **Symbol** | A normalized tradable instrument in the versioned symbol registry and exactly one Asset Group. Provider-native names map to the canonical Symbol; enabled registry Symbols are available to that Contest group. |
| **Price Quality** | The canonical assessment carried with market data and based on freshness, connection stability, sequence integrity, latency, errors, spread, consensus deviation, and required Symbol coverage. Lowest latency alone is insufficient. |
| **Source Epoch** | The monotonically incremented source-generation value changed on a validated Provider switch. It lets consumers distinguish ordered data before and after a source transition. |
| **Stale Price** | A quote that fails the applicable freshness/continuity rule. It cannot trigger new fills, pending orders, or TP/SL execution; affected final settlement may require Settlement Review. |
| **Paused Symbol** | A Symbol whose trading triggers are stopped during an Admin pause, source switch, or quality failure. New fills and pending/TP/SL triggers remain stopped until validated resume. |
| **Degraded Feed** | Asset Group state when only one healthy Provider remains. Trading continues under policy, Admin is alerted, and an audit event is recorded; provider details are not exposed to users. |

### Identity, authorization, and Admin security terms

| Term | Canonical meaning |
|---|---|
| **Support Admin** | The Admin role `SUPPORT_ADMIN`. It may perform only explicitly approved support and KYC operations. It cannot execute Super-Admin-only destructive financial operations and is not a substitute for Super Admin. |
| **Super Admin** | The privileged Admin role `SUPER_ADMIN`. It requires the `super_admin_totp_v1` login/session assurance and alone may execute approved destructive financial operations after explicit authorization and fresh password reauthentication. |
| **Sensitive-Action Password Reauthentication** | Fresh verification of the active Admin actor's password immediately before an action classified as destructive or security-sensitive. `SEC-004` implements this current local control; it is distinct from login MFA and does not prove paid-production readiness. |
| **Reauthentication Grant** | A short-lived, single-use Admin-context credential produced after Sensitive-Action Password Reauthentication and bound to actor, active session, action, and resource where applicable. It is invalid after replay, expiry, password change, session revocation, or permission change and never appears in a URL or log. |
| **Super Admin MFA** | The implemented `SEC-007` Admin-only `super_admin_totp_v1` assurance: password-first Google-Authenticator-compatible TOTP enrollment/login, encrypted Admin credential storage, explicit counter replay prevention, single-use recovery codes, audited reset, fail-closed startup validation, and session upgrade only after MFA. |
### Messaging, durability, identity, and payment terms

| Term | Canonical meaning |
|---|---|
| **Outbox** | An owner-schema table/record written in the same transaction as a domain change. A relay publishes its versioned command/event so a crash after commit does not lose delivery. |
| **Inbox** | A consumer-owned durable deduplication record written in the same transaction as its local side effect. Unique event IDs make replayed delivery idempotent. |
| **Idempotency Key** | A stable caller-supplied command identity. Repeating the same command with the same key returns/replays the original outcome rather than creating another financial or trading effect; it is distinct from an event ID. |
| **Immutable Snapshot** | A versioned, non-overwritten record of all inputs/state needed for later settlement, audit, restore, or dispute reconstruction. Changes create new records or compensating history, never edits to the snapshot. |
| **Replay** | Deterministic reprocessing of retained events, Engine WAL, or snapshots to reconstruct state. Inbox/event/command identities prevent replay from duplicating external effects. |
| **KYC** | Manual identity verification reviewed by Support Admin or Super Admin, with extensible document evidence, status, reviewer, reason, and immutable audit timestamps. Completed KYC is mandatory for Withdrawal. |
| **Deposit** | A gateway-confirmed addition to Wallet through Rial, USDT TRC20, or TRX methods. Platform credits the exact net confirmed amount through idempotent ledger posting after gateway verification. |
| **Withdrawal** | A KYC-gated manual external USDT TRC20 transfer. Amount moves from Available Balance to Withdrawal Pending, Super Admin executes externally, records the transaction hash, and completes or rejects through audited ledger actions. |
| **Second Chance** | Removed product capability. It is not active, must not be implemented, restored, displayed, versioned, or used to alter late entry, fees, prizes, T-Score, Official Ranking, or Settlement. |

## Collision rules

These statements are normative shorthand for code review and contract design:

1. `quantity` without a qualifier is rejected. People use Real Participant
   count; trading resource uses Trading QTY/`qty`.
2. Product-level Participant Capacity does not exist. Operational safety limits
   never become Contest fields or user-facing capacity.
3. T-Score is trading performance/ranking score. Reward Weight is prize
   distribution weighting. They are never aliases.
4. `commission_rate` is deprecated for Contest economics. Platform Fee uses
   `platform_fee_bps`; rates for other domains must be qualified.
5. Leaderboard Projection is a read model. Settlement alone owns finalization
   and Wallet prize postings.
6. A Free Practice System Participant is not a Real Participant and is never
   prize, economics, winner-count, or Official Ranking eligible.
7. Second Chance is removed and prohibited, not a dormant or planned feature.
8. Sensitive-Action Password Reauthentication is the `SEC-004` control for
   privileged actions; Super Admin MFA is the separate implemented `SEC-007`
   login/session-upgrade control. Both remain required before paid production.

## Version catalog

The target column names an identifier only when an authoritative source has
already assigned it. `Not assigned (planned)` prevents a legacy version from
being mistaken for an approved target.

| Versioned item | Canonical current or planned identifier | Status | Source and responsible roadmap task |
|---|---|---|---|
| Fixed product-policy document | `2026-08-09.1` | current | Approved [fixed policy](FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md), including implemented SEC-007 Super Admin MFA and the [Payment4 retirement](payment4-retirement-policy-amendment.md) decision. |
| Production roadmap | `2026-08-09.1` | current | Current [roadmap](../codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md), including completed `SEC-006` and implemented `SEC-007`. |
| Target architecture ADR | `ADR-0001` | current | Accepted [target runtime architecture](../adr/0001-target-runtime-architecture.md). |
| Contest policy ruleset | `2026-07-29.1` policy sections 4-7 and 10-12 | current policy; target implementation incomplete | `CON-001` through `CON-005`, `PRIZE-001` through `PRIZE-008`, and `DATA-005` implement the approved rules without inventing a parallel policy ID. |
| Scheduler Template Version | Not assigned (planned); identity field `schedule_template_version_id` | planned | `CON-005` introduces immutable versions and stores their IDs on generated Contests. |
| Symbol registry | Not assigned (planned); future family follows `{family}_vN` and immutable version ID rules | planned | The approved launch contents are in policy section 8; `MD-002` owns the registry/capability evidence and `CON-003` records the selected version in locked economics. |
| Scoring / T-Score rules | Not assigned (planned) | planned | `DATA-001` defines fixed-point score types and `ENG-002` implements deterministic Engine scoring. No prize Reward Weight version may be reused as scoring version. |
| Prize distribution | `tralent_v1` | approved target; implementation planned | Fixed policy section 11; `PRIZE-001` introduces the canonical package. The existing [`tralent_like_v1.json`](../../packages/contracts/prize_distribution/tralent_like_v1.json) is legacy/noncanonical. |
| Money and rate representation | Not assigned (planned) | planned | Fixed policy section 4 requires integer minor units, integer basis points, and fixed-point/rational rates; `DATA-001` introduces the versioned implementation and serialization. |
| Contest economics snapshot | Canonical record name `contest_economics_snapshot`; schema/version not assigned (planned) | planned | Fixed policy section 4.4 and `CON-003`; the task must assign/test a schema version and capture every governing rule ID. |
| Legacy shared event schemas | `v1` namespace and `*.v1.json` | legacy / noncanonical | Existing [`packages/contracts`](../../packages/contracts/README.md) compatibility artifacts use incomplete metadata and float representations. `DATA-001`, `MD-001`, `ENG-001`, and `ARCH-006` replace target boundaries by explicit versions. |
| Market Data event contract | `v2` | planned | Explicitly assigned by `MD-001`; it adds fixed-point prices, full tick metadata, gap/stale/pause/resume/source-switch events, and compatibility translation. Existing [`tick_snapshot.v1.json`](../../packages/contracts/schemas/tick_snapshot.v1.json) remains legacy evidence. |
| Trading Engine command/event contracts | Not assigned (planned); each family must use `vN` | planned | `ENG-001` defines contest configuration, participant activation, order, freeze, close, snapshot, and result contracts; `ENG-006` adds command idempotency, ordering, and deduplication. Legacy `/v1` does not preassign the target major. |
| Settlement result snapshot | Not assigned (planned) | planned | `PRIZE-006` defines the immutable result format, hash/version, partition completion barrier, and final-quote evidence before ranking. Existing [`settlement.go`](../../packages/contracts/v1/settlement.go) is not accepted as the target snapshot version. |
| Outbox/inbox event envelope | Not assigned (planned); envelope schema must use `vN` | planned | ADR-0001 fixes required metadata; `ARCH-006` implements and versions the transactional envelope, ordering, retry, dead-letter, and deduplication behavior. |
| User/Admin authentication isolation | No public contract version assigned; implemented boundary recorded by `SEC-001` | current implementation | `SEC-001` established separate User/Admin cryptographic, session, cookie, refresh, revocation, and CSRF contexts; later tasks must preserve them. |
| Sensitive-action reauthentication contract | Not assigned (current local implementation; no public contract version) | current local implementation | `SEC-004` defines and tests password verification and a short-lived actor/session/action/resource-bound opaque grant without implementing login MFA. |
| Payment-provider retirement decision | `PAYMENT4-RETIREMENT-2026-08-01` | current product decision | Payment4 is retired and has no active contract. `SEC-006` removes the legacy adapter and proves NOWPayments and Jibit remain independent; no replacement provider is approved. |
| Super Admin MFA contract | `super_admin_totp_v1` | current implementation | `SEC-007` and the [security contract](../security/super-admin-mfa.md) define/test TOTP enrollment, pre-session challenges, recovery, signed/session assurance, audit, production configuration, migration `0100`, and frontend flows. |
| REST API contracts | Not assigned (planned) | planned | Current endpoints are legacy/unversioned. `ARCH-002` through `ARCH-005` preserve or explicitly version Platform APIs, while `FE-001` generates trading REST types from versioned contracts. |
| WebSocket contracts | Not assigned (planned) | planned | Current streams are legacy. `FE-001` generates types from versioned contracts and `FE-003` defines sequence/resume/deduplication/reconciliation behavior after `ENG-006` and `MD-001`. |
| Database schema/migration baseline | Not assigned (planned); legacy sequence uses `NNNN_name.up/down.sql` | planned | The current 100-up-migration chain includes transitional current-runtime migrations and is not a canonical clean schema version. `FND-004` owns the clean baseline identifier, fresh-install command, legacy classification, and rollback policy. |

## Repository terminology remediation register

This FND-003 task records conflicts but does not change application behavior.
The listed owner tasks must migrate the target atomically and update this
catalog when they assign versions.

| Conflict | Current representative evidence | Canonical migration target | Owner task(s) |
|---|---|---|---|
| `max_participants` and participant-capacity semantics | [`contest_config.go`](../../packages/contracts/v1/contest_config.go), [`contest-config.ts`](../../packages/contracts/ts/v1/contest-config.ts), [`0008_free_tournaments.up.sql`](../../packages/db/migrations/0008_free_tournaments.up.sql), and [`contracts.ts`](../../apps/user-frontend/src/types/contracts.ts) | Remove product capacity fields. Use Real Participant count for people and separately named operational circuit breakers. | `FND-004`, `CON-001`, `CON-005`, `FE-006` |
| `commission_rate` as Contest fee source/fallback | [`contest_config.go`](../../packages/contracts/v1/contest_config.go), [`contest_prizes.go`](../../apps/user-bff/server/contest_prizes.go), [`finalize.go`](../../apps/leaderboard-worker/server/finalize.go), and [`0015_flexible_contest_config.up.sql`](../../packages/db/migrations/0015_flexible_contest_config.up.sql) | `platform_fee_bps = 2000` plus explicit Late-Entry Surcharge fields/accounts; no runtime fallback. | `FND-004`, `DATA-002` |
| Legacy `tralent_score`/`tragge_score` naming and possible T-Score/Reward Weight confusion | [`0014_tralent_score.up.sql`](../../packages/db/migrations/0014_tralent_score.up.sql), [`0074_rename_tralent_to_tragge.up.sql`](../../packages/db/migrations/0074_rename_tralent_to_tragge.up.sql), and [`0019_settlement_tables.up.sql`](../../packages/db/migrations/0019_settlement_tables.up.sql) | T-Score names Engine performance score only; `reward_weight` names prize weighting only. | `FND-004`, `DATA-001`, `ENG-002`, `PRIZE-001` |
| Status-only `registration_open` authorization | [`contest_handlers.go`](../../apps/user-bff/server/contest_handlers.go), [`calendar.go`](../../apps/contest-scheduler/internal/scheduler/calendar.go), and [`ContestDetailsPage.vue`](../../apps/user-frontend/src/modules/user/views/ContestDetailsPage.vue) | Evaluate explicit join-open/cutoff timestamps and policy; Running paid Contests can permit Late Entry before cutoff. | `CON-001`, `FE-006` |
| Leaderboard finalization/prize ownership | [`finalize.go`](../../apps/leaderboard-worker/server/finalize.go), [`effects.go`](../../packages/domain/statemachine/effects.go), and [`apps/contest-scheduler/README.md`](../../apps/contest-scheduler/README.md) | Leaderboard Projection only; Settlement owns ranking finalization, payout, retries, and completion. | `ARCH-005`, `PRIZE-005` |
| System account represented but exclusion not uniformly proven | [`0042_system_accounts.up.sql`](../../packages/db/migrations/0042_system_accounts.up.sql), [`free-contest-generator/server/app.go`](../../apps/free-contest-generator/server/app.go), and [`contest-scheduler/calendar.go`](../../apps/contest-scheduler/internal/scheduler/calendar.go) | One immutable System Participant classification, rank `0`, and exclusion by construction from paid join/economics/eligibility/payout. | `DATA-005`, `PRIZE-002`, `SCH-003` |
| Noncanonical prize fixtures and Power Law implementations | [`tralent_like_v1.json`](../../packages/contracts/prize_distribution/tralent_like_v1.json), [`distribution.go`](../../packages/scoring/distribution/distribution.go), and [`settlement.go`](../../apps/settlement-service/server/settlement.go) | One rational/fixed-point `tralent_v1` package and fixture set used by preview and Settlement. | `PRIZE-001`, `PRIZE-003`, `PRIZE-004`, `PRIZE-009` |
| Legacy float/incomplete `v1` contracts | [`packages/contracts/README.md`](../../packages/contracts/README.md), [`tick_snapshot.v1.json`](../../packages/contracts/schemas/tick_snapshot.v1.json), and [`position_closed_event.go`](../../packages/contracts/v1/position_closed_event.go) | Explicit fixed-point, envelope-complete, independently versioned Market Data and Engine contracts. | `DATA-001`, `MD-001`, `ENG-001`, `ARCH-006` |
| Gross prize terminology | No current repository field was found in the targeted search. | Keep it absent; use Prize Pool or `gross_base_entry_total` according to meaning. | `CON-003`, `PRIZE-004` |
| Second Chance | Repository search found only phase-prompt prohibitions and no `second_chance` code identifier. | Keep removed; any active implementation or UI appearance is a release-blocking regression. | All tasks; enforced by phase prompts and focused glossary validation. |

## Current versus target interpretation

- Existing files remain evidence of **current legacy behavior** until their
  owner tasks migrate them. A filename containing `v1` does not prove policy
  approval.
- The definitions and versions in this document describe the **approved target**
  even where status is planned.
- Future tasks must not rewrite historical snapshots under a new definition or
  version. Locked/completed Contests retain the identifiers and immutable inputs
  under which they were created.
- Assigning any currently unassigned identifier requires the responsible task to
  document compatibility, migration, rollout, rollback, and golden/contract
  evidence. This catalog must then be updated in the same change.
