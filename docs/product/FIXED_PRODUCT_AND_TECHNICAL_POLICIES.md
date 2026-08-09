# Tragge — Fixed Product and Technical Policies

**Status:** Approved baseline for production implementation  
**Policy version:** `2026-08-09.1`
**Canonical timezone for scheduling:** `Asia/Tehran`  
**Canonical storage timezone:** UTC  
**Technical language:** English  
**User interface languages:** Persian and English, with full RTL/LTR support

> This document is the product and engineering source of truth. Code, database
> constraints, API contracts, previews, settlement, admin tools, tests, and
> documentation must conform to it. Any future change requires a versioned
> decision record and must never mutate already-started contests.

---

## 1. Product Scope

Tragge is a simulated trading-tournament platform with:

- Paid Forex and Crypto contests.
- Free one-hour practice contests.
- A proprietary responsive web trading panel.
- A user panel and a separate admin panel.
- An internal USDT-denominated balance ledger.
- Fiat and crypto deposits through external gateways.
- Manual USDT TRC20 withdrawals after KYC and Super Admin approval.
- No custody of blockchain private keys or deposit addresses inside Tragge.

The first production release is a responsive web product. Native mobile
applications and PWA-specific features are outside the launch scope.

---

## 2. Target Architecture

### 2.1 Runtime boundaries

The production backend has exactly three bounded systems:

1. **Platform Modular Monolith**
   - Identity and authentication
   - User profile and KYC
   - Contest catalog and lifecycle
   - Scheduler and template management
   - Wallet and double-entry ledger
   - Payments and withdrawals
   - Prize preview and settlement orchestration
   - Leaderboard projection
   - Notifications
   - Tickets and support
   - Admin, permissions, and audit

2. **Trading Engine**
   - Orders, fills, positions, QTY reservation
   - Realized and unrealized trading score
   - Pending-order and TP/SL execution
   - Contest trading sessions
   - Deterministic replay, WAL, and snapshots

3. **Market Data Service**
   - Provider adapters
   - Symbol registry and normalization
   - Provider health and selection
   - Price quality, sequence, gap, and stale detection
   - Provider switching
   - Tick and candle publication
   - Raw event retention required for dispute reconstruction

The Trading Engine and Market Data Service are separate processes, images,
deployments, and failure domains. They must never be merged into a single
runtime.

### 2.2 Platform runtime modes

The Platform remains one modular-monolith codebase and one release version,
but may run the same image in separate operational modes:

- `platform --mode=api`
- `platform --mode=realtime`
- `platform --mode=worker`

These modes are deployment units, not independent domain microservices.

### 2.3 Internal communication rules

- Platform modules communicate through in-process application interfaces.
- Platform modules must not call each other over HTTP.
- Cross-bounded-system communication uses versioned commands/events and an
  outbox/inbox pattern.
- Every command/event has an event ID, correlation ID, causation ID, schema
  version, aggregate version, and occurred-at timestamp.
- Consumers are idempotent and reject or quarantine incompatible versions.
- Redis is never the source of truth for money, orders, fills, positions, or
  final contest results.

### 2.4 Data ownership

The initial production server may use one PostgreSQL cluster, but ownership is
separated by schema and credentials:

- `platform` schema and role
- `engine` schema and role
- `market_data` schema and role

No bounded system may query another system's tables directly. Cross-system
read models are built from events.

---

## 3. Repository and Delivery Policy

- The product remains a monorepo.
- User, trade, and admin frontends are separate applications:
  - `apps/user-frontend`
  - `apps/trade-frontend`
  - `apps/admin-frontend`
- Shared frontend code is limited to explicit packages:
  - `packages/frontend-core`
  - `packages/trading-contracts`
  - `packages/design-system`
- A separate GitHub account/repository is used as the Codex and staging
  integration repository.
- The canonical production repository is kept in a separate account with
  protected `main`.
- A successful task may be pushed and merged only after all task acceptance
  criteria pass.
- Commits use Conventional Commits.
- New dependencies require a written need, alternatives considered, security
  review, maintenance assessment, and minimal scope.

---

## 4. Canonical Money Model

### 4.1 Currency and precision

- The internal wallet and contest economics are denominated in USDT.
- Money is stored as integer minor units using one canonical precision defined
  by the money package and database schema.
- Floating-point types are prohibited for wallet balances, fees, prize amounts,
  payment amounts, and settlement.
- Percentages are stored as integer basis points.
- Rates and weights use fixed-point decimal or rational arithmetic.
- Preview and settlement must produce identical results from the same immutable
  input snapshot.

### 4.2 Entry fee split

For every regular paid entry:

- `20%` of the base entry fee is Platform Fee.
- `80%` of the base entry fee is contributed to the Prize Pool.
- Canonical field: `platform_fee_bps = 2000`.
- `commission_rate` is not a source of truth and must be removed after
  migration.

Example for a base entry fee of `100 USDT`:

- Platform Fee: `20 USDT`
- Prize Pool contribution: `80 USDT`

### 4.3 Late-entry surcharge

A user joining after contest start pays an additional surcharge equal to
`10%` of the base entry fee.

- The surcharge is entirely Platform Revenue.
- The surcharge does not enter the Prize Pool.
- The base entry fee still follows the 20/80 split.
- A late entrant therefore contributes the same base Prize Pool amount as an
  on-time entrant.
- Checkout must show the base entry fee, late surcharge, and final total before
  confirmation.

Example for a `100 USDT` base entry fee:

- User pays: `110 USDT`
- Base Platform Fee: `20 USDT`
- Late-entry Platform Revenue: `10 USDT`
- Prize Pool contribution: `80 USDT`

### 4.4 Economics lock

Contest economics are immutable immediately after the late-entry window closes:

- Final real-participant count
- Gross base entry total
- Base Platform Fee
- Late-entry surcharge revenue
- Net Prize Pool
- Planned winner count
- Prize distribution version
- Rank-band version
- Weight-decay version

No participant, fee, winner count, or prize amount may change after this lock.
The immutable record is named `contest_economics_snapshot`.

---

## 5. Contest Model

### 5.1 Market

Every contest has exactly one asset group:

- `crypto`
- `forex`

All enabled symbols in the selected asset group are available in the trading
panel for that contest.

There is no launch-time `commodity` contest group.

### 5.2 Participant capacity

Product-level participant capacity does not exist.

Operational circuit breakers may impose emergency infrastructure limits such
as maximum concurrent engine participants, contests, and WebSocket
connections. These are safety controls, not contest capacity fields, and must
not be shown as contest capacity.

### 5.3 Paid contest start condition

At `start_time`:

- Two or more real users joined: create the Trading Engine session and start.
- Fewer than two real users joined: cancel immediately and issue a full,
  idempotent refund.

System users never satisfy the two-real-user condition.

### 5.4 Engine session activation

Upcoming contests exist only in the Platform database. A Trading Engine session
is created lazily at contest start only when the contest satisfies its start
condition. This prevents unused upcoming contests from consuming engine
resources.

### 5.5 Trading durations and maximum QTY

| Contest type | Duration | Maximum trading QTY |
|---|---:|---:|
| Paid short | 30 minutes | 5 |
| Free practice | 1 hour | 10 |
| Paid medium | 4 hours | 10 |
| Paid daily | Defined below | 20 |
| Paid weekly | Defined below | 20 |
| Custom | Admin-selected | 5, 10, or 20 |

Rules:

- QTY is integer-only.
- Minimum order QTY is `1`.
- Pending orders reserve QTY.
- Active positions reserve QTY.
- Total reserved QTY across active positions and pending orders cannot exceed
  contest maximum QTY.
- Request rate limits still apply independently of QTY.
- Positions may remain open for multiple days when the contest duration permits.
- Every remaining position is force-closed at contest end under the settlement
  final-price policy.

### 5.6 Join windows

Free practice contests do not allow entry after start.

Paid contests may allow entry after start until:

```text
late_join_cutoff_at =
  start_time + min(10% of scheduled duration, 30 minutes)
```

Examples:

| Duration | Late-entry window |
|---:|---:|
| 30 minutes | 3 minutes |
| 4 hours | 24 minutes |
| 1 day | 30 minutes |
| 1 week | 30 minutes |

A custom contest can disable late entry. When enabled, the same formula applies.

Join authorization depends on `join_cutoff_at`, not only contest status.
A running contest can accept a late entrant before the cutoff.

### 5.7 Contest immutability

Once the first real user joins, the following contest-instance fields are
immutable:

- Market
- Start and end times
- Duration
- Base entry fee
- Maximum trading QTY
- Platform fee
- Late-entry policy
- Prize distribution version
- Symbol-registry version

Template changes only affect contests not yet generated.

---

## 6. Free Practice Contests

Free practice contests have:

- Duration: 1 hour
- Entry fee: 0
- Prize Pool: none
- Official prize table: none
- Official global ranking impact: none
- Market: Crypto or Forex
- Maximum trading QTY: 10
- Entry after start: disabled

### 6.1 Practice system user

One persistent system account is automatically registered in every free
contest.

- It is visible in the live leaderboard.
- Its displayed rank is always `0`.
- It is excluded from all prize, economics, eligibility, winner-count, and
  official-ranking queries.
- It cannot join paid contests.
- It cannot place user-initiated orders.
- Its presence alone does not allocate an Engine session.
- A free Engine session is activated when at least one real user is present.

This approach is preferred over synthetic participant counters because it is
auditable, simple, and consistent with the existing system-account model.

---

## 7. Scheduler Policy

### 7.1 Time rules

- Slot calculations use `Asia/Tehran`.
- `start_time`, `end_time`, and cutoff timestamps are stored in UTC.
- APIs return ISO-8601 UTC timestamps.
- Frontends display time in the browser timezone.
- Slot generation must be deterministic, idempotent, and safe under concurrent
  scheduler instances.
- Canonical unique key:
  `schedule_template_version_id + start_time + entry_fee_minor`.

### 7.2 Forex tradability in the first release

For the initial release, Forex is considered closed in Tehran time from:

```text
Saturday 00:20 inclusive
to Monday 01:30 exclusive
```

A Forex contest is generated only when its entire interval is tradable under
this rule.

Official holidays, early closes, and late opens are an explicitly accepted MVP
risk and are not hardcoded. A provider-backed market calendar is a post-launch
hardening item.

Crypto is tradable 24/7.

### 7.3 Paid 30-minute contests

- Start every 10 minutes at `:00, :10, :20, :30, :40, :50`.
- Markets: Crypto and Forex.
- Entry fees per market: `5`, `10`, and `20 USDT`.
- Maximum trading QTY: `5`.
- Six future start slots are maintained.
- Maximum upcoming records:
  `6 slots × 2 markets × 3 fees = 36`.
- When Forex is closed, no replacement Forex contest is created; only Crypto
  records are shown.
- A Forex record is generated only if the full 30-minute interval is tradable.

### 7.4 Free one-hour contests

- Start every 60 minutes on minute `30`.
- Examples: `23:30`, `00:30`, `01:30`.
- At each eligible start, create one Crypto and one Forex practice contest.
- When Forex is open, display the next **5 start slots**, each containing Crypto
  and Forex.
- When Forex is closed, display the next **8 start slots**, Crypto only.
- No artificial Forex replacement is created.

### 7.5 Paid four-hour contests

Crypto daily start slots:

- `01:30`
- `05:30`
- `09:30`
- `13:30`
- `17:30`
- `21:30`

Crypto settings:

- Entry fees: `5`, `20 USDT`
- Maximum trading QTY: `10`

Forex daily start slots:

- `05:30`
- `09:30`
- `13:30`
- `17:30`

Forex settings:

- Entry fees: `5`, `20 USDT`
- Maximum trading QTY: `10`
- Full interval must be tradable.

Display horizon:

```text
display_end_date = end of the fourth valid Forex trading day after today
```

Today is not counted. Crypto is displayed for every calendar day through the
same endpoint. Forex is displayed only on valid trading days.

### 7.6 Paid daily contests

Crypto:

- Start: `01:30`
- End: next day `01:30`
- Entry fees: `20`, `50`, `100 USDT`
- Maximum trading QTY: `20`

Forex:

- Start: `01:30`
- End: next day `00:20`
- Duration: 22 hours 50 minutes
- Entry fees: `20`, `50`, `100 USDT`
- Maximum trading QTY: `20`
- Full interval must be tradable.
- Invalid days are skipped and not moved.

The display horizon is the same as four-hour contests.

### 7.7 Paid weekly contests

Crypto:

- A new seven-day contest series starts every calendar day at `01:30`.
- End: the same weekday seven days later at `01:30`.
- Entry fees: `250`, `500`, `1000 USDT`.
- Maximum trading QTY: `20`.
- Display all daily Crypto weekly starts through the fourth valid future Forex
  Monday horizon.

Forex:

- Start only Monday at `01:30`.
- End Saturday at `00:20`.
- Entry fees: `500`, `1000`, `5000 USDT`.
- Maximum trading QTY: `20`.
- Display the next four valid Monday start slots.
- If the full interval is not tradable, skip that week without moving it.

All entry-fee sets are editable through versioned templates. High-value
templates may remain disabled in the initial launch profile.

### 7.8 Custom contests

Super Admin can define:

- Start date and time
- Duration; end time is calculated automatically
- Market
- Entry fee
- Maximum trading QTY from `5`, `10`, `20`
- Late-entry enabled/disabled

Custom contests may overlap scheduler contests. Platform fee remains fixed at
20% for the initial release.

### 7.9 Template lifecycle

Templates are database-backed, versioned, and editable in Admin.

- A template update creates a new version.
- Generated contests never change because of a template edit.
- Disabling a template stops future generation.
- Upcoming generated contests with no real participants may be cancelled and
  archived.
- Upcoming generated contests with real participants remain unchanged.
- Hard deletion is prohibited for auditable contest history.

---

## 8. Symbols and Market Registry

The launch registry is editable and versioned. The currently approved initial
set contains 34 Forex symbols and 22 Crypto assets.

### 8.1 Forex

```text
AUD/CAD
AUD/CHF
AUD/JPY
AUD/NZD
AUD/USD
CAD/CHF
CAD/JPY
CAD/SGD
CHF/DKK
CHF/HUF
CHF/JPY
EUR/AUD
EUR/CAD
EUR/CHF
EUR/GBP
EUR/JPY
EUR/NOK
EUR/NZD
EUR/SGD
EUR/TRY
EUR/USD
GBP/AUD
GBP/CAD
GBP/CHF
GBP/JPY
GBP/NZD
GBP/USD
NZD/CAD
NZD/CHF
NZD/JPY
NZD/USD
USD/CAD
USD/CHF
USD/JPY
```

### 8.2 Crypto

```text
AAVE
APT
AVAX
BTC
BCH
ADA
LINK
CRO
DOGE
ETH
HBAR
ICP
LTC
NEAR
PEPE
DOT
SHIB
SOL
XLM
SUI
UNI
XRP
```

The desired future count of 37 Forex and 24 Crypto symbols is not a launch
invariant. Additions or removals use a versioned registry and provider coverage
gate.

---

## 9. Market Data Policy

### 9.1 Candidate providers

Product-owner supplied candidates:

Crypto:

- Nobitex
- Wallex
- Coinbase
- Binance
- Deriv
- Tiingo

Forex:

- Deriv
- Tiingo

Provider coverage, commercial rights, redistribution rights, timestamps,
sequence support, rate limits, and symbol mapping must be verified before a
provider is production-enabled.

### 9.2 Selection granularity

The first release selects the active provider independently by asset group:

- Active Crypto provider
- Active Forex provider

The health model still evaluates symbol coverage and symbol quality. A
symbol-specific pause or override is allowed, but default provider selection is
not independently flapped for every symbol.

### 9.3 Provider quality score

Selection considers:

- Freshness
- Connection stability
- Sequence integrity
- Latency
- Error rate
- Spread quality
- Deviation from healthy-provider consensus
- Required symbol coverage

Lowest latency alone is not sufficient.

### 9.4 Provider control

Admin supports:

- `AUTO`
- `FORCE_PROVIDER`
- `PAUSE_SYMBOL`

A manual provider force is reviewed after one hour. Automatic control may
resume only when the preferred automatic provider has remained stable for at
least ten minutes.

### 9.5 Degraded operation

If only one healthy provider remains:

- Trading continues.
- Asset-group status becomes `DEGRADED`.
- Admin receives an alert.
- An audit event is recorded.
- Provider identity and technical health are not exposed to users.

### 9.6 Provider switch

During a provider switch:

1. Pause affected symbols.
2. Stop new fills and pending/TP/SL triggering for paused symbols.
3. Validate the new source against quality and consensus rules.
4. Increment `source_epoch`.
5. Resume the symbol.
6. Audit the full transition.

Users receive only a generic rejection when an affected symbol cannot trade;
provider names and internal state remain hidden.

### 9.7 Tick contract

The canonical tick event contains at least:

- `schema_version`
- `event_id`
- `symbol`
- `asset_group`
- fixed-point `bid`
- fixed-point `ask`
- fixed-point `last`, when available
- `provider`
- `provider_timestamp`
- `received_at`
- `published_at`
- `sequence`
- `source_epoch`
- `quality`
- `is_synthetic`
- `normalization_version`

Synthetic bid/ask values must be explicitly marked. Silent queue drops are
prohibited; any drop or gap must be measurable and detectable.

### 9.8 Candles

- Canonical live candles are aggregated internally from Bid ticks.
- Provider candles may be used for chart backfill only after normalization and
  provenance tagging.
- Chart and audit prices therefore use the same canonical Bid source.

---

## 10. Trading and Scoring Policy

- Trading is simulated.
- Execution uses real Bid/Ask from the canonical feed.
- Spread is floating and derived from the feed.
- Trading commission is zero.
- Swap is zero.
- Slippage is limited to real quote movement between accepted command and
  deterministic execution.
- A buy opens at Ask and closes at Bid.
- A sell opens at Bid and closes at Ask.
- Chart display uses Bid.
- Order, fill, position, price, and score calculations use fixed-point decimal,
  never binary floating point at financial boundaries.
- The ranking score is the canonical cumulative contest performance score
  generated by the Trading Engine.
- All open positions are closed at contest end using the immutable settlement
  price snapshot.
- If a valid final quote is unavailable for a symbol, only that symbol's
  affected trades are paused for review. If resolving those trades can change
  any prize rank, the entire contest remains in `SETTLEMENT_REVIEW`.

---

## 11. Prize Distribution — `tralent_v1`

### 11.1 Distribution version

Canonical configuration:

```yaml
distribution_version: tralent_v1
winner_ratio: 0.30
rounding: half_up
decay_factor: 0.80
eligibility: at_least_one_filled_trade
```

### 11.2 Planned winner count

Small-contest rules:

| Real registered participants | Planned winners |
|---:|---:|
| 2–3 | 1 |
| 4–6 | 2 |
| 7–11 | 3 |

For 12 or more real registered participants:

```text
raw_winners = floor(participants × 0.30 + 0.5)
planned_winners = upper boundary of the rank band containing raw_winners
```

Canonical rank bands:

```text
1, 2, 3, 4, 5, 6, 7, 8, 9, 10
11–15
16–20
21–25
26–30
31–35
36–40
41–50
51–60
61–75
76–100
101–125
126–150
151–175
176–200
201–225
226–250
251–275
276–300
301–375
376–450
451–525
526–600
601–750
751–900
901–1050
1051–1200
1201–1500
1501–1800
1801–2100
2101–2400
2401–3000
```

### 11.3 Eligibility

- Planned winner count uses all real registered participants captured in the
  economics snapshot.
- Prize eligibility requires at least one Filled Trade.
- Users with no Filled Trade do not appear in the prize table.
- The practice system user is excluded.
- `actual_winners = min(planned_winners, eligible_ranked_users)`.
- If eligible users are fewer than planned winners, preserve the weights of the
  remaining occupied ranks and renormalize them to 100%.
- No unallocated prize amount becomes Platform Revenue.
- No prize amount remains undistributed.

### 11.4 Small-contest prize shares

For fewer than 12 real registered participants, the prize shares are explicit
policy fixtures and do not use geometric normalization:

| Real participants | Prize shares by rank |
|---:|---|
| 2–3 | `100%` |
| 4 | `80%, 20%` |
| 5 | `75%, 25%` |
| 6 | `70%, 30%` |
| 7 | `65%, 25%, 10%` |
| 8 | `60%, 25%, 15%` |
| 9 | `55%, 30%, 15%` |
| 10–11 | `50%, 30%, 20%` |

These fixtures are versioned as part of `tralent_v1`.

### 11.5 Bucket weights for 12 or more participants

For active prize buckets:

```text
weight[1] = 1
weight[2] = 0.8
weight[3] = 0.8²
...
bucket_share[i] = weight[i] / sum(active bucket weights)
```

Individual ranks 1–10 each form one bucket. A grouped rank band receives one
bucket share, then that share is divided equally among every occupied rank in
the group.

Backend terminology uses `reward_weight`, not `t_score`:

```text
reward_weight = real_registered_participants × individual_prize_share
```

### 11.6 Exact ties

An exact score tie uses a pooled-position rule:

- The tied users occupy consecutive prize positions.
- The prize amounts for those occupied positions are pooled.
- The pooled amount is divided equally among tied eligible users.
- The next rank follows competition ranking.
- Tie equality must never be broken by rounding.
- A stable user ID is used only for deterministic display ordering, not payout.

### 11.7 Rounding

- Prize calculations use rational or fixed-point arithmetic.
- Final money allocation is exact in integer minor units.
- Sum of all prize payouts equals the locked Prize Pool exactly.
- Members of a grouped rank band receive exactly equal amounts.
- Members of a tie group receive exactly equal amounts.
- Residual minor units are assigned only to the highest individual non-tied
  ranks and never break group equality.
- Prize preview and final settlement share one package and one fixture set.

---

## 12. Settlement Policy

Settlement is the sole owner of final contest completion and payout.

Canonical sequence:

1. Freeze contest commands.
2. Stop new orders.
3. Cancel pending orders.
4. Close open positions using valid final-price snapshots.
5. Wait for an explicit completion barrier from every assigned Engine shard.
6. Create an immutable engine-result snapshot.
7. Build final eligible rankings once.
8. Calculate prizes once using `tralent_v1`.
9. Write double-entry ledger transactions.
10. Reconcile Prize Pool liability to payouts exactly.
11. Mark settlement completed.
12. Mark contest completed.
13. Publish final notifications and read-model events.

The live Leaderboard is a projection only. It must not:

- Complete contests
- Credit wallets
- Calculate an independent final prize table
- Own settlement retry state

Every settlement operation is idempotent and reconstructable from immutable
inputs.

---

## 13. Wallet, Deposits, and Withdrawals

### 13.1 Internal wallet

- The Platform maintains an internal USDT-denominated double-entry ledger.
- It does not store private keys.
- It does not generate or manage blockchain deposit addresses.
- A user can join paid contests only with confirmed available balance.

Required ledger accounts include:

- User available
- User reserved
- Contest Prize Pool
- Platform fee revenue
- Late-entry surcharge revenue
- Deposit clearing
- Withdrawal pending
- Gateway fee expense or clearing
- Manual adjustment clearing

Admin corrections use compensating entries. Existing ledger rows are immutable.

### 13.2 Deposits

User-selectable Rial gateways:

- Jibit
- Sepal

Sepal is also the selected Rial test gateway.

User-selectable crypto gateways:

- Plisio
- NOWPayments

Payment4 is retired effective 2026-08-01 under
[product decision PAYMENT4-RETIREMENT-2026-08-01](payment4-retirement-policy-amendment.md).
It is not selectable, configurable, callable, or production-supported. Its
routes, credentials, provider adapter, and startup requirements must remain
absent. This decision adds no replacement provider.

Supported deposit methods:

- Rial
- USDT TRC20
- TRX

Rules:

- Minimum deposit: `4 USDT` equivalent.
- Maximum deposit: `1000 USDT` equivalent.
- Wallet credits the exact net confirmed amount reported by the gateway.
- Gateway fees are not re-calculated by Tragge when the gateway reports the net
  amount.
- Every webhook and inquiry operation is idempotent.
- Webhook signatures, timestamps, replay windows, and provider payment IDs are
  verified.

### 13.3 Rial quote

The IRR/USDT reference quote is captured from the Nobitex
`USDTIRT` order book when the payment request is created.

- Store raw response, normalized rate, quote timestamp, source, and expiry.
- The quote is immutable for that payment request.
- A short configurable expiry is required.
- Payment confirmation records the exact gateway-settled amount and rate.

### 13.4 Withdrawals

- Asset: USDT TRC20 only.
- Minimum: `10 USDT`.
- No initial daily limit.
- User pays the withdrawal fee.
- Completed KYC is mandatory.
- Withdrawal execution is external and manual.
- Only Super Admin can complete or deduct a rejected withdrawal.

Flow:

```text
User submits amount and TRC20 address
→ amount moves from Available to Withdrawal Pending
→ Super Admin reviews
→ Super Admin transfers externally
→ Super Admin enters transaction hash
→ Super Admin marks Completed
```

On rejection, Admin must choose:

- `REJECT_AND_RELEASE`: return reserved amount to available balance.
- `REJECT_AND_DEDUCT`: remove amount through a compensating ledger entry.

`REJECT_AND_DEDUCT` requires Super Admin permission, password re-entry, a
mandatory reason, and immutable audit logging.

---

## 14. KYC and Roles

### 14.1 Registration

Initial registration collects:

- Email
- Password
- First name
- Last name
- Country

Email ownership is verified by OTP before the account becomes fully active.
Google sign-in is post-launch.

### 14.2 Email OTP

- Iranian users: Mailerino.
- Foreign users: Resend.
- OTP validity default: 10 minutes.
- Maximum verification attempts: 5.
- Resend cooldown default: 60 seconds.
- OTP values, reset tokens, and verification secrets are never logged.
- In production, missing provider configuration is a startup or feature
  hard-failure, never a mock-provider fallback.

### 14.3 Roles

Initial roles:

- `USER`
- `SUPPORT_ADMIN`
- `SUPER_ADMIN`

Support Admin may review KYC and support tickets under explicit permissions.
Super Admin owns withdrawals, security-sensitive overrides, and permission
management.

### 14.4 KYC

KYC is reviewed manually by Support Admin or Super Admin.

Initial extensible record supports:

- Identity document type
- Identity document number
- Front image
- Back image when applicable
- Selfie
- Country
- Review status
- Reviewer
- Review reason
- Audit timestamps

The exact accepted document catalog can be expanded later without changing the
state machine.

### 14.5 Current Admin authentication

Super Admin login requires password verification followed by the Admin-only MFA
contract `super_admin_totp_v1` inside the isolated Admin cryptographic and
session trust domain established by `SEC-001`.

- A valid Admin password and every current Admin authorization, session,
  revocation, cookie, and CSRF control remain required.
- Password verification alone must not issue a Super Admin session, access
  token, or refresh token. `SEC-007` owns the required enrollment, TOTP/recovery
  challenge, and MFA session assurance.
- Password-only Super Admin authentication is prohibited and is not evidence
  for paid-production readiness.
- Password reset invalidates all existing sessions.
- User and Admin JWT signing keys, audiences, refresh tokens, cookies, and Redis
  namespaces are cryptographically and operationally separate.
- Session JWTs are never accepted from URL query parameters.

### 14.6 Sensitive-action password reauthentication

Fresh password reauthentication remains mandatory for:

- withdrawal completion;
- rejected-withdrawal deduction when that workflow is implemented;
- destructive Wallet or balance adjustment;
- a security-sensitive override;
- elevated role or permission changes where currently implemented; and
- another action explicitly classified as destructive or security-sensitive.

The resulting reauthentication grant must be short lived, single use,
Admin-context-specific, actor-bound, session-bound, action-bound, and
resource-bound where applicable. It must be rejected after password change,
session revocation, or permission change. It must never appear in a URL or log.

Roles remain `USER`, `SUPPORT_ADMIN`, and `SUPER_ADMIN`; a Finance role is not
part of the product. Only Super Admin may execute approved destructive
financial operations. Support Admin remains limited to explicitly approved
support and KYC permissions.

### 14.7 Super Admin MFA

`SEC-007` implements the versioned `super_admin_totp_v1` contract and remains a
required paid-production prerequisite. The contract includes:

- Google-Authenticator-compatible TOTP;
- secure enrollment;
- encrypted TOTP-secret storage;
- replay prevention;
- recovery codes;
- reset and recovery procedures;
- session upgrade only after MFA;
- immutable audit events;
- production startup requirements;
- frontend enrollment and login flows; and
- real database and concurrency tests.

Implementation of this control does not independently approve paid production;
all other launch gates and the Phase 1 exit gate remain required.
Paid-production status remains `NO-GO`.

---

## 15. Notifications and Support

Initial notification events:

- Deposit confirmed
- Withdrawal status changed
- Contest reminder
- Contest started
- Late-entry warning
- Prize won
- Contest settlement under review
- Contest cancelled and refunded

Channels are user-configurable where applicable. Financial and security
notifications cannot be silently dropped; delivery failures are observable and
retryable.

A ticket/support module is part of the production scope.

---

## 16. Frontend Policy

- Persian and English are available from day one.
- Every user and admin screen supports RTL and LTR.
- Source code, identifiers, commits, and technical documentation are English.
- The trade panel is a separate application and release artifact.
- The trade panel displays:
  - Contest remaining time
  - Live Bid chart
  - Orders and positions
  - Available and reserved QTY
  - Live leaderboard
- It does not display a late-entry cutoff timer.
- Provider names and provider-health details are admin-only.
- The UI reconciles with authoritative Engine state after reconnect and refresh.
- WebSocket events have sequence numbers and resume support.
- Duplicate and out-of-order events cannot create duplicate orders or corrupt
  displayed P&L.

---

## 17. Reliability, Backup, and Recovery

### 17.1 Trading Engine durability

- A command is acknowledged only after its durable intent and required database
  state are committed.
- WAL persistence is mandatory in production.
- WAL errors fail closed or place the Engine in a controlled read-only mode.
- Engine replay is deterministic.
- Default incremental snapshot threshold:
  - every 60 seconds, or
  - every 10,000 state mutations,
  - whichever occurs first.
- Mandatory snapshots are taken at contest start, freeze, settlement handoff,
  and clean shutdown.
- Snapshot frequency remains configurable and must be benchmarked.

### 17.2 Backups

- PostgreSQL point-in-time recovery is enabled.
- Encrypted backups and engine snapshots are copied to external object storage.
- Raw market-data retention needed for disputes is externalized from the
  application SSD.
- Local disk is not the only backup location.
- Restore drills are mandatory before paid launch and periodically afterward.
- A backup is not considered valid until a restore test succeeds.

### 17.3 Initial infrastructure

Initial deployment:

- One dedicated server
- 8 CPU cores
- 16 GB RAM
- 100 GB SSD
- Docker Compose
- Upgradeable network traffic
- External object storage
- Provider snapshots where available
- Basic DDoS protection

Permanent Production and Staging do not run together. Staging is started
temporarily for a release, validated, and then stopped.

Kubernetes is outside the initial launch path.

---

## 18. Observability and Operations

Required production telemetry:

- Structured logs with correlation IDs
- Metrics for API, Engine, Market Data, payments, settlement, scheduler, and
  WebSockets
- Distributed traces across bounded systems
- Provider health and source-switch audit
- Ledger and settlement reconciliation dashboards
- Dead-letter and inbox/outbox lag dashboards
- Disk, traffic, CPU, memory, database, Redis, and broker alerts
- User-impacting incident severity model
- On-call runbooks
- Trading kill switch
- Payment/deposit kill switch
- Withdrawal kill switch
- Contest-generation kill switch
- Per-symbol pause control

Secrets, OTPs, access tokens, refresh tokens, KYC documents, and sensitive
payment payloads are redacted from telemetry.

---

## 19. Production Quality Gates

Paid public launch is prohibited until all conditions pass:

- Zero open P0 and P1 issues.
- One canonical fee model.
- One canonical prize package.
- Settlement is the only finalization owner.
- User/Admin authentication isolation is tested.
- Super Admin MFA from `SEC-007` is enforced and validated.
- Sensitive-action password reauthentication from `SEC-004` is enforced.
- No OTP, token, or secret appears in logs.
- Fresh database install and upgrade tests pass.
- Double-entry ledger balances exactly.
- Prize Pool reconciliation has zero unexplained difference.
- Engine crash/replay tests pass.
- Provider switch and stale-price drills pass.
- Backup restore succeeds.
- Seven-day soak test passes.
- Load test reaches at least twice the capped launch target.
- Security review and penetration-test findings are remediated.
- Gateway webhook replay tests pass.
- A real contest dispute can be reconstructed from immutable records.
- Legal, payment-provider, market-data-use, and jurisdiction reviews are
  approved.

---

## 20. Change-Control Rule

Any future change to money, prize distribution, contest timing, ranking,
eligibility, late entry, settlement, or provider execution policy requires:

1. A versioned ADR or product decision.
2. A new configuration/version identifier.
3. Backward-compatible reconstruction of old contests.
4. Golden fixture updates.
5. Preview/settlement parity tests.
6. Explicit rollout and rollback instructions.

Already-locked or completed contests are never recalculated under a newer rule.
