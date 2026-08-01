# SQL Migration Review Report

**Date**: 2026-04-06
**Scope**: All 97 migration pairs in `packages/db/migrations/`

---

## Critical Issues (HIGH)

### 1. `DOUBLE PRECISION` for OHLCV prices — `0005_candles.up.sql:12-16`
- `open`, `high`, `low`, `close`, `volume` use `DOUBLE PRECISION` (IEEE 754 float)
- For candle data used in trading comparisons, floating-point imprecision can cause incorrect threshold evaluations
- **Recommendation**: Use `NUMERIC(20,8)` consistent with `orders.limit_price` and `positions.entry_price`

### 2. Down migration reverts to wrong precision — `0032_decimal_scores.down.sql:39-58`
- Up changes `user_stats.total_score` from `NUMERIC(20,4)` to `NUMERIC(20,8)`
- Down reverts to `DECIMAL(20,2)` instead of original `NUMERIC(20,4)` — **data loss on rollback**
- Same issue for `tragge_score`, `score`, `score_contribution` columns

### 3. Blanket UPDATE loses explicit values — `0046_contest_commission_rate_default.up.sql:9-10`
- `UPDATE contests SET commission_rate = 20.00 WHERE commission_rate = 17.00`
- No way to distinguish between "default 17" and "explicitly set 17"
- Rollback (`20→17`) corrupts contests that were explicitly set to 20.00 before migration

### 4. Duplicate triggers on `tournament_templates` — `0056` + `0061`
- `0056` creates `trg_tournament_templates_updated_at` → calls `update_tournament_templates_updated_at()`
- `0061` creates `set_tournament_templates_updated_at` → calls `trigger_set_updated_at()`
- Both fire on every UPDATE, doing the same work

### 5. OAuth tokens stored as plaintext — `0034_oauth_accounts.up.sql:31`
- `access_token TEXT` and `refresh_token TEXT` stored unencrypted
- Comments note "encrypted at rest recommended" but no enforcement

### 6. TOTP secret stored as plaintext — `0050_two_factor_auth.up.sql:4`
- `totp_secret TEXT` stored unencrypted
- `TOTP_ENCRYPTION_KEY` exists in secrets but no DB-level enforcement

### 7. `CREATE INDEX CONCURRENTLY` in transactions — `0086`, `0089`, `0095`
- `CREATE INDEX CONCURRENTLY` cannot run inside a transaction block
- golang-migrate wraps statements in transactions by default → **migrations will fail**
- Files: `0086_phone_unique_index.up.sql:2`, `0089_ticket_updated_index.up.sql:1`, `0095_verification_codes_cleanup_index.up.sql:2`

### 8. Destructive down migration for OAuth users — `0034_oauth_accounts.down.sql:35`
- `DELETE FROM users WHERE password_hash IS NULL` permanently deletes all OAuth-only users
- Not safely reversible in production

### 9. Down migration deletes pre-existing symbols — `0096_full_forex_symbols.down.sql:5-7`
- Deletes `EUR/GBP`, `EUR/CHF`, `EUR/AUD`, etc. which existed before this migration (from 0078, 0088)
- Rolling back this migration causes data loss of unrelated records

### 10. Down migration fails if phone-only users exist — `0092_add_phone_auth.down.sql:1`
- `ALTER COLUMN email SET NOT NULL` fails if any users registered with phone only (NULL email)
- Makes rollback impossible in production

### 11. Duplicate unique constraint on `phone` — `0092_add_phone_auth.up.sql:2`
- Adds inline `UNIQUE` on `phone` column
- `0086` already created partial unique index `idx_users_phone WHERE phone IS NOT NULL`
- Inline `UNIQUE` prevents multiple NULLs (only 1 NULL allowed), contradicting the partial index approach

### 12. Partitioned tables lose FK constraints — `0003_partition_tables.up.sql:32-222`
- When `orders`, `fills`, `positions` are converted to partitioned tables, FKs to `contests(id)` and `users(id)` are dropped
- Only FK to `shard_config(shard_id)` remains — referential integrity to contests/users no longer enforced at DB level

### 13. `template_id` FK silently never applied — `0015` + `0017`
- `0015` adds `contests.template_id UUID` (no FK constraint)
- `0017` tries `ADD COLUMN IF NOT EXISTS template_id UUID REFERENCES tournament_templates(id)`
- Since column already exists, `IF NOT EXISTS` silently skips → FK is never created

### 14. `REFRESH MATERIALIZED VIEW CONCURRENTLY` without UNIQUE index — `0018_tournament_calendar.up.sql:152`
- `refresh_calendar_contests_mv()` uses `REFRESH CONCURRENTLY` which requires a unique index on the materialized view
- No unique index is defined → **will fail at runtime**

---

## Medium Issues

### Missing Indexes on Foreign Keys

| File | Table.Column | Line |
|------|-------------|------|
| `0019` | `prize_distributions.user_id` | 87 (idx exists at 112, OK) |
| `0028` | `email_templates.updated_by` | 15 |
| `0033` | `calendar_entries.created_by` | 53 |
| `0036` | `email_template_versions.created_by`, `updated_by` | 16-17 |
| `0063` | `contests.template_id` | 11 (only partial dedup index in 0071) |
| `0077` | `provider_config.updated_by` | 14 (also no FK constraint) |
| `0082` | `chart_drawings.contest_id` | — (no index, no FK constraint either) |

### Missing CHECK Constraints

| File | Table.Column | Issue | Line |
|------|-------------|-------|------|
| `0001` | `contest_participants.final_prize_cents` | No `CHECK (>= 0)` on prize amount | 106 |
| `0005` | `candles.high/low/open/close` | No `CHECK (high >= low)` or positivity constraints | 12-16 |
| `0006` | `user_stats.win_rate` | No `CHECK (win_rate >= 0 AND win_rate <= 100)` | 16 |
| `0019` | `contest_settlements.prize_pool_*_cents` | No `CHECK (>= 0)` on monetary BIGINT columns | 54-57 |
| `0038` | `contests.unjoin_deadline_minutes` | No `CHECK (>= 0)` | 3 |
| `0047` | `prize_locks.*_cents` | No `CHECK (>= 0)` on monetary columns | 9-14 |
| `0063` | `contests.commission_amount` | No `CHECK (>= 0)` on financial BIGINT | 23 |
| `0070` | `final_rankings.win_rate` | No `CHECK (win_rate >= 0 AND win_rate <= 100)` | 1 |
| `0072` | `positions.take_profit`, `stop_loss` | No `CHECK (> 0)` when not NULL | 4-5 |
| `0079` | `template_entry_tiers.commission_rate_override` | No range CHECK (0-100) | 33 |
| `0079` | `template_entry_tiers.qty_total_override` | No `CHECK (>= 0)` | 27 |
| `0084` | `notification_preferences.category`, `channel` | No CHECK or ENUM for valid values | 5-6 |
| `0093` | `verification_codes.attempts`, `max_attempts` | Missing NOT NULL | 9-10 |
| `0097` | `users.preferred_lang` | No `CHECK (preferred_lang IN ('fa','en'))` | 1 |

### Missing FK Constraints

| File | Table.Column | Issue | Line |
|------|-------------|-------|------|
| `0082` | `chart_drawings.user_id`, `contest_id` | No `REFERENCES` — orphan risk if user/contest deleted | 2-3 |

### Missing UNIQUE Constraints

| File | Table.Column | Issue | Line |
|------|-------------|-------|------|
| `0021` | `password_reset_tokens.token_hash` | Should be UNIQUE to prevent hash collisions and serve as lookup index | 5 |
| `0022` | `email_verification_tokens.token_hash` | Same as above | 9 |

### Redundant Indexes

| File | Index | Redundant With | Line |
|------|-------|----------------|------|
| `0001` | `idx_users_email` | Already covered by `UNIQUE` constraint on `email` | 18 |
| `0024` | `idx_permissions_name` | Already covered by `UNIQUE` constraint on `name` | 22 |
| `0029` | `idx_wallet_ledger_idempotency_lookup` | Duplicate of unique partial index `idx_wallet_ledger_idempotency_key` | 30 |
| `0034` | `idx_oauth_accounts_provider_lookup` | Duplicate of unique constraint `uq_oauth_provider_user_id` | 59 |
| `0037` | `idx_contest_reminders_sent_contest` on `(contest_id)` | Already leading column of PK `(contest_id, reminder_type)` | 21 |

### Data Type Concerns

| File | Table.Column | Issue | Line |
|------|-------------|-------|------|
| `0041` | `kyc_verifications.face_match_score`, `liveness_score` | `DOUBLE PRECISION` for score thresholds — floating-point imprecision | 10-11 |
| `0068` | `tournaments_archive.entry_fee_cents` | Uses `INT` but source table uses `BIGINT` — potential truncation | 25 |
| `0001` | `contest_participants.final_prize_cents` | `INT` limits to ~21M — Rials may exceed this | 106 |

---

## Low Issues

### Down Migration Inconsistencies

| File | Issue |
|------|-------|
| `0045` down | Doesn't revert UPDATE statements in Part 3 (relies on 0044 rollback) |
| `0075` down | Renames to `tralent_score_contribution` (pre-0074 name) — naming mismatch if partially rolled back |
| `0090` down | Restores index on `(provider, provider_payment_id)` but original was on `provider_payment_id` alone |

### Other

| File | Issue | Line |
|------|-------|------|
| `0036` | Race condition in `check_max_template_versions` trigger under concurrent inserts | 35 |
| `0041` | No format CHECK on `national_code` (should be `^\d{10}$`) | 6 |
| `0042` | Hardcoded fake password_hash `'SYSTEM_ACCOUNT_NO_LOGIN'` — not a valid Argon2id hash | 36 |
| `0056` | Creates new function instead of reusing `trigger_set_updated_at()` (inconsistency) | 5 |
| `0065` | Seed data sets `entry_fee_cents=0` while `entry_fee` has actual value — inconsistency | 14-107 |
| `0078` | No index on `sort_order` (its purpose is ordering) | — |
| `0080` | `bg_color VARCHAR(7)` doesn't validate hex format | 7 |
| `0081` | `national_code_manual VARCHAR(10)` no format CHECK | 9 |
| `0087` | `ticket_messages.sender_id ON DELETE CASCADE` destroys conversation history | 41 |
| `0093` | `created_at` allows NULL (missing `NOT NULL`) | 11 |
| `0011` | Race condition in `generate_referral_code()` — concurrent inserts can collide | 127-139 |
| `0014` down | Does not recalculate scores after formula change revert — data correctness issue | — |
| `0016` | `contest_status_history.from_status` missing index for reverse lookups | 48 |
| `0025` | `payouts.reviewed_by` FK defaults to RESTRICT; should be `ON DELETE SET NULL` | 28 |

---

## Summary

| Severity | Count |
|----------|-------|
| HIGH (data loss, security, runtime failure) | 14 |
| MEDIUM (missing constraints/indexes) | 28 |
| LOW (inconsistencies, style) | 17 |
| **Total** | **59** |

### Top Priority Fixes

1. **Fix `CREATE INDEX CONCURRENTLY` migrations** (0086, 0089, 0095) — these will fail at runtime
2. **Fix `REFRESH CONCURRENTLY` without unique index** (0018) — will fail at runtime
3. **Fix duplicate triggers on `tournament_templates`** (0056 + 0061) — causes double execution
4. **Fix 0032 down migration** — reverts to wrong precision, causes data loss
5. **Add FK constraint on `contests.template_id`** (0015/0017) — silently never applied
6. **Add FK constraints to `chart_drawings`** (0082) — orphan data risk
7. **Fix 0092 duplicate unique constraint on `phone`** — contradicts 0086's partial index
8. **Restore FK integrity after partitioning** (0003) — contests/users FKs dropped
