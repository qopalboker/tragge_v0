# Legacy migration inventory and classification

**Status:** FND-004 approved reset evidence

**Inventory date:** 2026-07-25; updated 2026-08-09 by SEC-007

**Scope:** Top-level `*.up.sql` files in
[`packages/db/migrations`](../../packages/db/migrations), before the clean target
baseline cutover

## Reproducible count and order

From the repository root, PowerShell reproduces the inventory order:

```powershell
Get-ChildItem packages/db/migrations -File -Filter '*.up.sql' |
  Sort-Object Name |
  Select-Object -ExpandProperty Name
```

The initial FND-004 audit found **98 up migrations**, numbered continuously from
`0001` through `0098`. SEC-004 appended the paired
`0099_admin_canonical_roles` migration. SEC-007 appended the paired
`0100_admin_super_mfa` migration. The current reproducible inventory is
therefore **100 up migrations** numbered continuously through `0100`, with no
duplicate identifier or missing matching down file. It now has 101 down files:
`0000_baseline.down.sql` remains the one documented orphan. The legacy directory
contains **100 up/down pairs plus one known orphan down**. The target foundation
is isolated in the `target` child directory and is not part of this legacy count.

## Classification rules and totals

- `KEEP`: retain the legacy migration unchanged in the target chain.
- `FOLD_INTO_BASELINE`: preserve its approved intent while expressing it once in
  the clean owner-schema baseline.
- `REPLACE`: preserve only valid requirements; a named roadmap task must provide
  target semantics, ownership, types, and constraints.
- `DELETE_AFTER_CUTOVER`: omit obsolete, superseded, removed, or non-launch
  behavior after traceability and cutover evidence are retained.

No legacy migration is `KEEP`: every current migration writes the shared
`public` model or depends on it, so retaining any one unchanged would violate
ADR-0001 ownership. Totals are:

| KEEP | FOLD_INTO_BASELINE | REPLACE | DELETE_AFTER_CUTOVER | Total |
|---:|---:|---:|---:|---:|
| 0 | 25 | 57 | 18 | 100 |

## Complete manifest

?Down? means a matching legacy down file exists after FND-004. ?Dependency?
records the important earlier object dependency, in addition to ordered
application of the preceding chain. Operations name whether the migration
creates, alters, backfills, renames, or removes data/schema.

| Order | Migration | Down | Operation and primary domain/tables | Important earlier dependency | Known duplicate or target conflict | Class | Rationale, intended owner, and roadmap replacement |
|---:|---|:---:|---|---|---|---|---|
| 1 | `0001_init.up.sql` | yes | Creates users, roles, contests, participants, orders, fills, positions, leaderboard snapshots, audit log | none | Shared public ownership; mixes all three systems; financial/trading types are not canonical | REPLACE | Split Platform, Engine, and Market Data ownership through ARCH-001, ARCH-006, DATA-001, and ENG-001. |
| 2 | `0002_shard_config.up.sql` | yes | Creates shard config/log and alters contests with shard assignment | 0001 contests | Platform Contest references Engine shard internals | REPLACE | Engine session/shard ownership belongs to ENG-008; Platform receives events only. |
| 3 | `0003_partition_tables.up.sql` | yes | Creates and backfills partitioned orders, fills, positions | 0001 trading tables; 0002 shards | Duplicates unpartitioned trading tables and keeps shared ownership | REPLACE | ENG-001 and ENG-008 choose Engine-owned storage and any partitioning after benchmarks. |
| 4 | `0004_wallet.up.sql` | yes | Creates wallets, wallet ledger, payment intents, payouts | 0001 users/contests | Mutable wallet balance plus single-entry ledger is not immutable double-entry accounting | REPLACE | Platform DATA-003/DATA-004 and ARCH-004 own ledger, payment, and Withdrawal tables. |
| 5 | `0005_candles.up.sql` | yes | Creates candle table with floating OHLCV | 0001 extension | Binary floating canonical prices and shared public ownership | REPLACE | Market Data Service MD-007 owns fixed-point canonical candles and provenance. |
| 6 | `0006_user_stats.up.sql` | yes | Creates stats/history, functions, triggers, and score backfill logic | 0001 users/participants | Leaderboard, global score, PnL, and prize-like weighting are conflated | REPLACE | Platform projection via ARCH-003; Engine T-Score via DATA-001/ENG-002. |
| 7 | `0007_leaderboard_indexes.up.sql` | yes | Creates indexes on positions, orders, participants | 0001 trading/contest tables | Leaderboard queries Engine tables directly | REPLACE | Local Platform projections replace cross-owner indexes under ARCH-003/ARCH-006. |
| 8 | `0008_free_tournaments.up.sql` | yes | Alters contests; creates tournament templates and seed data | 0001 contests | Adds `max_participants` product capacity and legacy free-contest semantics | REPLACE | Remove capacity; CON-005, DATA-005, and SCH-003 define target templates/practice. |
| 9 | `0009_contest_duration_types.up.sql` | yes | Creates duration enum/config, alters/backfills contests, seeds configs | 0001 contests | Duplicates later template/schedule representations | REPLACE | Platform CON-005 and SCH-001 through SCH-006 own versioned scheduling. |
| 10 | `0010_kyc_system.up.sql` | yes | Creates KYC verification, documents, audit | 0001 users | Provider-oriented fields need target manual-review normalization | FOLD_INTO_BASELINE | Preserve KYC evidence intent in Platform ARCH-004 with SEC policy constraints. |
| 11 | `0011_affiliate_program.up.sql` | yes | Creates referral/affiliate commission tables, view, triggers | 0001 users/wallet | Affiliate program is not approved launch scope; commission naming risks fee collision | DELETE_AFTER_CUTOVER | Omit from clean baseline unless separately approved by a future roadmap decision. |
| 12 | `0012_user_status.up.sql` | yes | Creates user-status enum; alters users; adds indexes | 0001 users | Role/auth isolation still absent | FOLD_INTO_BASELINE | Preserve account status in Platform identity under ARCH-002 and SEC-001. |
| 13 | `0013_order_history_index.up.sql` | yes | Creates order history index | 0001 orders | Index is on shared legacy Engine data | REPLACE | ENG-001 owns Engine schema; indexes follow measured Engine queries. |
| 14 | `0014_tralent_score.up.sql` | yes | Renames score, alters history, creates formula/functions, backfills | 0006 stats/history | Obsolete score formula confuses T-Score and Reward Weight | DELETE_AFTER_CUTOVER | DATA-001/ENG-002 define T-Score; PRIZE-001 defines separate Reward Weight. |
| 15 | `0015_flexible_contest_config.up.sql` | yes | Creates asset enum; alters/backfills contests/templates; seeds rows | 0008 templates; 0009 duration | `commission_rate`, capacity/min fields, mixed/commodity assets, duplicate config | REPLACE | CON-001/CON-005 and DATA-002 provide canonical Platform model. |
| 16 | `0016_contest_state_machine.up.sql` | yes | Alters enum/contests; creates history; triggers/backfills participant counter | 0001 contests; 0008 capacity | Capacity counter and status-only registration; lifecycle/finalization overlap | REPLACE | CON-001/CON-003/CON-004 define Platform lifecycle and locked real count. |
| 17 | `0017_auto_generated_contests.up.sql` | yes | Alters contests and adds indexes | 0008 templates; 0016 contest fields | Legacy generator/service ownership and mutable template pointer | REPLACE | Platform CON-005/SCH-001 use immutable Scheduler Template Version. |
| 18 | `0018_tournament_calendar.up.sql` | yes | Alters templates/contests; creates view/indexes; calculates Prize Pool | 0008/0015 templates and fee | Independent Prize Pool formula uses `commission_rate`; duplicate scheduler model | REPLACE | CON-005/SCH tasks schedule; CON-003/DATA-002 own one economics snapshot. |
| 19 | `0019_settlement_tables.up.sql` | yes | Creates settlement, prize, ranking, event, snapshot tables | 0001 contests/users | Multiple Prize Pool fields and settlement/ranking ownership mixed in public | REPLACE | PRIZE-005/PRIZE-006/PRIZE-007 and ARCH-005 define Platform Settlement and Engine result boundary. |
| 20 | `0020_affiliate_activation.up.sql` | yes | Alters/backfills referrals; replaces affiliate view/function | 0011 affiliate tables | Non-launch affiliate behavior | DELETE_AFTER_CUTOVER | Omit with 0011 unless later explicitly approved. |
| 21 | `0021_password_reset_tokens.up.sql` | yes | Creates link-token table and indexes | 0001 users | Superseded by code/OTP migrations and target fail-closed delivery | DELETE_AFTER_CUTOVER | SEC-003 defines canonical reset verification; do not preserve link tokens for dev data. |
| 22 | `0022_email_verification.up.sql` | yes | Alters/backfills users; creates email token table | 0001 users | Marks all legacy users verified and duplicates later verification-code stores | REPLACE | SEC-003 supplies one fail-closed OTP/reset model without automatic legacy trust. |
| 23 | `0023_user_profile.up.sql` | yes | Alters users; creates indexes/updated trigger | 0001 users | Shared public table only | FOLD_INTO_BASELINE | Preserve approved profile/country fields in Platform ARCH-002. |
| 24 | `0024_admin_roles.up.sql` | yes | Seeds roles/permissions; creates permission tables | 0001 roles/users | Legacy viewer/moderator roles conflict with USER, SUPPORT_ADMIN, SUPER_ADMIN | REPLACE | ARCH-002 and SEC-001/SEC-004 implement isolated Admin identity/permissions. |
| 25 | `0025_withdrawal_management.up.sql` | yes | Alters payout enum/table; adds review fields/indexes | 0004 payouts | Payout table conflates Withdrawal and prize payout; target actions are incomplete | REPLACE | ARCH-004 and DATA-003 model Withdrawal Pending and audited release/deduct postings. |
| 26 | `0026_symbols_master.up.sql` | yes | Creates symbol registry, seed rows, permission grants | 0001 roles/contest symbols | Platform/public ownership; stock/commodity outside launch; provider columns embedded | REPLACE | Market Data Service MD-002 owns approved versioned Asset Group/Symbol registry. |
| 27 | `0027_contest_reminder_tracking.up.sql` | yes | Alters contests; creates notifications and indexes | 0001 contests/users | Reminder sent marker is coupled to Contest row | FOLD_INTO_BASELINE | Preserve notification intent in Platform ARCH-003 with owned delivery state. |
| 28 | `0028_email_templates.up.sql` | yes | Creates and seeds email templates | none beyond Platform DB | No product-rule conflict; mutable singleton is superseded later | FOLD_INTO_BASELINE | Preserve template intent in Platform ARCH-003, folded with 0036 versions. |
| 29 | `0029_wallet_idempotency.up.sql` | yes | Alters wallet ledger; adds idempotency indexes | 0004 wallet ledger | Adds safety to a non-double-entry ledger | REPLACE | DATA-003/DATA-004 preserve idempotency on immutable transactions/postings. |
| 30 | `0030_finalization_tracking.up.sql` | yes | Creates contest finalization state and trigger | 0001 contests | Duplicates 0019 settlement state and legacy finalization owner | REPLACE | PRIZE-005 makes Settlement sole finalization owner. |
| 31 | `0031_contest_pause_timing.up.sql` | yes | Alters contests with pause timing and index | 0001 contests | Generic pause semantics are not the target lifecycle or symbol pause | REPLACE | CON-001 owns Contest lifecycle; MD-006/ENG-007 own Paused Symbol behavior. |
| 32 | `0032_decimal_scores.up.sql` | yes | Alters score/PnL columns and rebuilds view | 0001, 0006, 0019 | Decimal columns do not establish canonical scale/version and span owners | REPLACE | DATA-001 and ENG-002 introduce explicit integer fixed-point representation. |
| 33 | `0033_calendar_entries.up.sql` | yes | Creates calendar/schedule tables and constraints | 0008 templates | Duplicate scheduler source alongside 0009/0018/0062 | DELETE_AFTER_CUTOVER | CON-005/SCH-001 replace with one versioned template/schedule source. |
| 34 | `0034_oauth_accounts.up.sql` | yes | Creates OAuth table; alters password nullability | 0001 users | Google sign-in is post-launch; weakens initial email/password invariant | DELETE_AFTER_CUTOVER | Omit from launch baseline; any future OAuth needs an approved task. |
| 35 | `0035_recreate_positions_compat.up.sql` | yes | Creates compatibility view | 0032; 0001 positions | Compatibility view hides legacy type/table split and crosses future ownership | DELETE_AFTER_CUTOVER | No compatibility preservation for disposable dev data; Engine target contracts replace it. |
| 36 | `0036_email_template_versions.up.sql` | yes | Creates version table/function/trigger | 0028 templates | Arbitrary five-version trigger is implementation-specific | FOLD_INTO_BASELINE | Preserve immutable version intent in Platform ARCH-003; redesign constraints there. |
| 37 | `0037_multi_interval_reminders.up.sql` | yes | Creates reminder-delivery table/index | 0001 contests/users | Shared public ownership only | FOLD_INTO_BASELINE | Platform ARCH-003 owns idempotent notification delivery. |
| 38 | `0038_contest_unjoin_config.up.sql` | yes | Adds unjoin columns | 0001 contests | Feature was removed by 0059 and is absent from approved policy | DELETE_AFTER_CUTOVER | Do not include obsolete unjoin configuration. |
| 39 | `0039_contest_started_email_template.up.sql` | yes | Seeds email template | 0028 templates | Seed belongs in versioned reference data | FOLD_INTO_BASELINE | Platform ARCH-003 reference-data seed. |
| 40 | `0040_contest_ending_reminders.up.sql` | yes | Seeds email template | 0028 templates | Seed belongs in versioned reference data | FOLD_INTO_BASELINE | Platform ARCH-003 reference-data seed. |
| 41 | `0041_jibit_kyc_fields.up.sql` | yes | Alters KYC with provider scores/status | 0010 KYC | Automated Jibit KYC flow conflicts with approved manual review; double scores | REPLACE | ARCH-004 preserves evidence needed for manual KYC; provider automation is not baseline. |
| 42 | `0042_system_accounts.up.sql` | yes | Alters/seeds users and participants; backfills flags | 0001 users/participants | Bot satisfies legacy minimum and auto-join rationale; target System Participant is excluded | REPLACE | DATA-005 creates classification; SCH-003 registers it only in Free Practice. |
| 43 | `0043_rename_finnhub_to_massive.up.sql` | yes | Renames provider columns | 0001 contest symbols; 0026 symbols | Intermediate provider-name churn | DELETE_AFTER_CUTOVER | MD-002 introduces adapter mappings without retaining rename history in baseline. |
| 44 | `0044_update_massive_provider_symbols.up.sql` | yes | Backfills provider mappings | 0043 renamed columns | Superseded provider mapping data including non-launch assets | DELETE_AFTER_CUTOVER | MD-002/MD-003/MD-004 seed verified capabilities separately. |
| 45 | `0045_massive_provider_symbols.up.sql` | yes | Inserts and updates symbol/provider mappings | 0026/0043 symbols | Registry contents exceed/contradict approved launch set and mix ownership | REPLACE | MD-002 owns versioned registry and coverage evidence. |
| 46 | `0046_contest_commission_rate_default.up.sql` | yes | Alters defaults and backfills `commission_rate` | 0015 fee columns | Deprecated duplicate fee source remains even at 20 percent | DELETE_AFTER_CUTOVER | DATA-002 uses only `platform_fee_bps = 2000`. |
| 47 | `0047_prize_lock.up.sql` | yes | Creates prize lock; alters contests with pool fields | 0015 fee; 0019 settlement | Locks at start, duplicates Prize Pool, persists `commission_rate` | REPLACE | CON-003 locks one economics snapshot after Join Cutoff; PRIZE tasks consume it. |
| 48 | `0048_wallet_reason_code.up.sql` | yes | Alters wallet ledger and adds indexes | 0004 wallet ledger | Reason codes sit on non-double-entry mutable model | REPLACE | DATA-003/DATA-004 preserve qualified reasons in immutable ledger transactions. |
| 49 | `0049_password_changed_at.up.sql` | yes | Alters users with session invalidation timestamp | 0001 users | User/Admin isolation still absent | FOLD_INTO_BASELINE | Preserve invalidation evidence within separately owned auth stores via SEC-001. |
| 50 | `0050_two_factor_auth.up.sql` | yes | Alters shared users with TOTP/backups | 0001 users | User and Admin credentials share storage; encryption/reauth invariants incomplete | REPLACE | SEC-001 isolates Admin trust; SEC-004 adds sensitive-action reauthentication; planned SEC-007 replaces legacy TOTP with target Super Admin MFA. |
| 51 | `0051_nobitex_provider.up.sql` | yes | Alters/backfills symbol provider mappings | 0026 symbols | Provider mapping stored in public Platform-style registry | REPLACE | Market Data MD-002/MD-003 owns verified adapters and mappings. |
| 52 | `0052_withdrawal_limits.up.sql` | yes | Creates per-user limits and payout index | 0004 payouts | Initial policy has no daily limit; payout/Withdrawal conflation remains | REPLACE | ARCH-004 models approved Withdrawal flow; future limits need policy change. |
| 53 | `0053_order_type_directional.up.sql` | yes | Alters order enum | 0001 order type | Shared PostgreSQL enum and public Engine table | REPLACE | ENG-001/ENG-003 define versioned commands and Engine-owned validation. |
| 54 | `0054_free_contest_unique_constraint.up.sql` | yes | Alters order constraints; adds free-contest uniqueness index | 0053 order enum; 0001 contests | Combines Engine constraints with Platform scheduler dedup | REPLACE | ENG-003 owns order constraints; SCH-001/CON-005 own canonical dedup key. |
| 55 | `0055_settlement_batch_indexes.up.sql` | yes | Creates filled-order settlement index | 0001 orders | Platform Settlement reads Engine order table directly | REPLACE | PRIZE-006 consumes immutable Engine result; no cross-schema query. |
| 56 | `0056_tournament_template_updated_at.up.sql` | yes | Alters templates; creates trigger | 0008 templates | Mutable row is not an immutable Scheduler Template Version | FOLD_INTO_BASELINE | Preserve audit timestamp intent in CON-005 versioned templates. |
| 57 | `0057_withdraw_fee_and_expired_status.up.sql` | yes | Alters ledger/payment enums | 0004 wallet/payment enums | Ledger enum is coupled to non-double-entry model | REPLACE | DATA-003/ARCH-004 express ledger accounts and payment states canonically. |
| 58 | `0058_withdrawal_refund_ledger_types.up.sql` | yes | Alters ledger enum | 0004/0057 ledger enum | Refund types substitute for compensating double-entry postings | REPLACE | DATA-003/DATA-004 use immutable compensating transactions. |
| 59 | `0059_drop_contest_unjoin_columns.up.sql` | yes | Removes obsolete unjoin columns | 0038 | Pure cleanup of feature absent from target | DELETE_AFTER_CUTOVER | Clean baseline omits both add and drop migrations. |
| 60 | `0060_terms_accepted_at.up.sql` | yes | Alters users with acceptance timestamp | 0001 users | Shared public ownership only | FOLD_INTO_BASELINE | Preserve consent timestamp in Platform identity/profile under ARCH-002. |
| 61 | `0061_tournament_templates_schema.up.sql` | yes | Creates enums; alters/backfills templates | 0008/0015 templates | Rial entry fee and decimal `commission_rate`; duplicate duration/market models | REPLACE | CON-005 uses USDT minor units, Asset Group, Trading QTY, and immutable versions. |
| 62 | `0062_tournament_schedules.up.sql` | yes | Creates schedules, validation function, trigger | 0061 templates | Cron/weekend model does not implement fixed Tehran schedule policy | REPLACE | SCH-001 through SCH-006 define deterministic target schedules. |
| 63 | `0063_contests_template_fields.up.sql` | yes | Alters contests; adds fee amount/market close; removes capacity constraint | 0061/0062; 0015/0047 | Keeps capacity fields and dynamic Prize Pool/commission duplicates | REPLACE | DATA-002/CON-003 remove duplicates; CON-005 links immutable template version. |
| 64 | `0064_template_prize_distributions.up.sql` | yes | Creates arbitrary rank percentage table | 0008 templates | Competes with canonical `tralent_v1`; percentage storage lacks versioned rules | REPLACE | PRIZE-001/PRIZE-003 own one versioned distribution and exact allocation. |
| 65 | `0065_seed_tournament_templates.up.sql` | yes | Seeds 23 legacy templates | 0061 templates | Rial fees, decimal commission, free contests with real prizes, obsolete assets | DELETE_AFTER_CUTOVER | CON-005/SCH tasks provide approved versioned USDT templates later. |
| 66 | `0066_seed_tournament_schedules.up.sql` | yes | Seeds legacy schedules | 0062 schedules; 0065 templates | Schedule set does not prove approved Tehran/Forex rules | DELETE_AFTER_CUTOVER | SCH-001 through SCH-006 seed validated schedules. |
| 67 | `0067_contest_schedule_dedup.up.sql` | yes | Creates schedule/start unique index | 0062 schedules; 0001 contests | Key omits Scheduler Template Version and Base Entry Fee | REPLACE | CON-005/SCH-001 use the canonical three-part unique key. |
| 68 | `0068_tournament_state_enhancements.up.sql` | yes | Alters contests; creates archive table copying Contest columns | 0001/0015 contests | Archive duplicates mutable state, fee/capacity fields, and public ownership | REPLACE | Platform keeps auditable history/snapshots without a denormalized conflicting copy. |
| 69 | `0069_ban_expires_at.up.sql` | yes | Alters users and adds sweeper index | 0012 user status | Shared public ownership only | FOLD_INTO_BASELINE | Preserve temporary suspension evidence in Platform identity under ARCH-002. |
| 70 | `0070_final_rankings_win_rate.up.sql` | yes | Alters final rankings | 0019 final rankings | Leaderboard/final result ownership and decimal score representation conflict | REPLACE | PRIZE-005/PRIZE-006 and DATA-001 define immutable result and projection fields. |
| 71 | `0071_contest_template_dedup.up.sql` | yes | Creates template/start unique index | 0017 template pointer | Key omits immutable version and fee; duplicates 0067 approach | REPLACE | CON-005/SCH-001 use the canonical unique key only. |
| 72 | `0072_positions_tp_sl.up.sql` | yes | Alters positions with TP/SL prices | 0001 positions | Public Engine state and unversioned numeric price scale | REPLACE | ENG-001/ENG-002/ENG-003 own fixed-point TP/SL fields and constraints. |
| 73 | `0073_fills_realized_pnl.up.sql` | yes | Alters fills and partitioned fills with PnL | 0001/0003 fills | Duplicated tables and unversioned numeric PnL | REPLACE | ENG-002 owns one fixed-point Engine fill/PnL model. |
| 74 | `0074_rename_tralent_to_tragge.up.sql` | yes | Renames score; replaces functions; backfills behavior | 0014 score | Intermediate obsolete naming/formula | DELETE_AFTER_CUTOVER | Target T-Score is newly defined by DATA-001/ENG-002. |
| 75 | `0075_rename_tragge_score_to_tragge_point.up.sql` | yes | Renames score/contribution; replaces functions | 0074 | Another intermediate name; still confuses performance and prize contribution | DELETE_AFTER_CUTOVER | Do not preserve rename history in clean target schema. |
| 76 | `0076_update_crypto_symbols.up.sql` | yes | Inserts/updates/soft-removes crypto symbols | 0026/0051 symbols | Contents differ from approved registry and provider verification is absent | REPLACE | MD-002/MD-003 supply versioned coverage-gated reference data. |
| 77 | `0077_binance_provider.up.sql` | yes | Alters symbols; creates/seeds provider config | 0026/0051 symbols | Provider selection and symbol registry are shared public state | REPLACE | Market Data MD-002/MD-005 owns Provider capability and Asset Group selection. |
| 78 | `0078_symbol_sort_order.up.sql` | yes | Alters/backfills symbol display order | 0026 symbols | Includes Commodity and unversioned UI ordering in shared registry | REPLACE | MD-002 owns registry data; Platform projection may store display metadata by event. |
| 79 | `0079_template_entry_tiers.up.sql` | yes | Creates tiers/prize percentages; alters/seeds contests/templates | 0061/0064 templates | Fee override, arbitrary prize percentages, duplicate template/fee sources | REPLACE | CON-005, DATA-002, and PRIZE-001 replace with immutable fee sets/rules. |
| 80 | `0080_predefined_avatars.up.sql` | yes | Creates and seeds avatar reference table | 0023 profile | No fixed-policy conflict | FOLD_INTO_BASELINE | Preserve ancillary Platform profile reference data under ARCH-002. |
| 81 | `0081_manual_kyc_iranian.up.sql` | yes | Alters KYC/documents and enum; adds index | 0010 KYC | Earlier provider-specific KYC fields still require normalization | FOLD_INTO_BASELINE | Preserve approved manual KYC evidence in Platform ARCH-004. |
| 82 | `0082_chart_drawings.up.sql` | yes | Creates per-user chart drawing table | 0001 users | Shared public ownership; not authoritative trading state | FOLD_INTO_BASELINE | Preserve as Platform-owned user preference only, never Engine truth. |
| 83 | `0083_security_audit_log.up.sql` | yes | Creates append-oriented security audit table/indexes | 0001 users | Separate from legacy generic/KYC audit tables | FOLD_INTO_BASELINE | Consolidate immutable Platform audit ownership under ARCH-002/SEC-005. |
| 84 | `0084_notification_preferences.up.sql` | yes | Creates user notification preferences | 0001 users | Shared public ownership only | FOLD_INTO_BASELINE | Platform ARCH-003 owns notification preferences. |
| 85 | `0085_rename_system_bot.up.sql` | yes | Updates seeded system-account display data | 0042 system account | Intermediate branding data on conflicting bot semantics | DELETE_AFTER_CUTOVER | DATA-005 supplies one canonical System Participant identity. |
| 86 | `0086_phone_unique_index.up.sql` | yes | Creates partial unique phone index | 0023 users | Later 0092 also adds unique phone constraint | FOLD_INTO_BASELINE | Preserve one normalized Platform identity uniqueness invariant via ARCH-002. |
| 87 | `0087_support_tickets.up.sql` | yes | Creates support tickets/messages/attachments and indexes | 0001 users | Shared public ownership only | FOLD_INTO_BASELINE | Platform ARCH-003 owns support and audited Admin assignment. |
| 88 | `0088_finnhub_provider.up.sql` | yes | Alters/backfills symbols with provider mapping | 0026 symbols | Reintroduces a provider column renamed in 0043; includes non-launch assets | REPLACE | MD-002/MD-004 define verified provider adapters and mappings. |
| 89 | `0089_ticket_updated_index.up.sql` | yes | Creates concurrent support-ticket index | 0087 support tickets | `CONCURRENTLY` execution mode conflicts with transactional assumptions | FOLD_INTO_BASELINE | Create the needed index directly in clean Platform baseline; no online build needed for empty DB. |
| 90 | `0090_provider_payment_id_unique.up.sql` | yes | Replaces index with unique provider/payment index | 0004 payment intents | Legacy table still lacks target ledger/payment boundary | FOLD_INTO_BASELINE | Preserve idempotency invariant in Platform ARCH-004/DATA-004 target payment table. |
| 91 | `0091_email_verification_otp.up.sql` | yes | Alters email token table with attempt counter | 0022 token table | Extends a duplicated legacy store instead of one canonical fail-closed code model | REPLACE | SEC-003 defines OTP expiry, attempts, cooldown, provider failure, and redaction. |
| 92 | `0092_add_phone_auth.up.sql` | yes | Alters users; creates OTP logs; makes email optional | 0023/0086 users | Phone-only registration contradicts approved initial email registration | REPLACE | ARCH-002/SEC-003 retain approved email ownership verification; phone changes need policy. |
| 93 | `0093_verification_codes.up.sql` | yes | Creates generalized verification codes and indexes | 0001 users | Duplicates 0022/0091; delivery/provider semantics incomplete | REPLACE | SEC-003 provides one versioned, fail-closed verification model. |
| 94 | `0094_password_reset_codes.up.sql` | yes | Creates password reset codes/indexes | 0001 users; 0093 pattern | Duplicates 0021 link-token path; auth isolation/redaction incomplete | REPLACE | SEC-001/SEC-003 own isolated reset flow and session invalidation. |
| 95 | `0095_verification_codes_cleanup_index.up.sql` | yes | Creates concurrent cleanup index | 0093 codes | Index is tied to a store that must be replaced | REPLACE | SEC-003 defines canonical retention/cleanup and its indexes. |
| 96 | `0096_full_forex_symbols.up.sql` | yes | Inserts/updates Forex and Dow Jones symbols | 0026/0088 symbols | Includes non-approved Dow Jones/commodity semantics and unverified coverage | REPLACE | MD-002/MD-004 seed only approved, coverage-gated launch symbols. |
| 97 | `0097_users_preferred_lang.up.sql` | yes | Alters users with language preference | 0001 users | Shared public ownership only | FOLD_INTO_BASELINE | Preserve Persian/English preference in Platform identity/profile. |
| 98 | `0098_fix_migration_audit_issues.up.sql` | yes | Adds indexes; drops/recreates triggers; alters scores/phone/drawing constraints | 0032/0056/0082/0086/0089/0095 | Corrective patch refers to unstable legacy score names and transactional index failures | DELETE_AFTER_CUTOVER | Fold valid end-state constraints into owning replacements; omit the corrective migration itself. |
| 99 | `0099_admin_canonical_roles.up.sql` | yes | Creates the canonical Support Admin role, limits it to KYC permissions, and migrates legacy Admin assignments | 0001 roles/user roles; 0024 permissions | Transitional shared-public role data is not the clean target Platform auth schema | FOLD_INTO_BASELINE | Preserve canonical USER/SUPPORT_ADMIN/SUPER_ADMIN semantics in the future Platform-owned auth baseline; omit this legacy bridge after cutover. |
| 100 | `0100_admin_super_mfa.up.sql` | yes | Creates Admin-only MFA credentials and hashed single-use recovery-code tables | 0001 users; 0099 canonical Admin roles | Transitional shared-public placement precedes Platform identity ownership cutover | FOLD_INTO_BASELINE | Preserve the `super_admin_totp_v1` invariants in the Platform-owned Admin identity schema under ARCH-002; never fold the legacy shared User TOTP columns into this credential. |

## Traceability conclusion

The 100 files remain in the legacy top-level directory only to reproduce current
behavior and supported local evidence until cutover. The clean target command
uses only the isolated target child directory. After target implementation,
fresh-install, restore, and explicitly supported upgrade evidence pass, all 100
legacy pairs are removed together under a reviewed cutover; individual files
are never silently edited or mixed into the target version history.
