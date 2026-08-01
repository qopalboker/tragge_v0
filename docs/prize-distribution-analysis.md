# Prize Distribution Analysis: Current Formula vs Tralent Power Law

## Context

Analysis of the exact mathematical formulas used for prize pool distribution, compared against Tralent's power law model: `prize(rank) = PRIZE_POOL × (1/rank^α) / Σ(1/i^α)` where `α ≈ 1.095`.

## Finding: Two Parallel Distribution Systems

The codebase has two independent prize distribution implementations using different mathematical models.

### Common Prize Pool Calculation

Both systems share the same pool calculation:

```
gross = participants × entry_fee_cents
net = gross × (10000 - platform_fee_bps) / 10000
```

- Default commission: 2000 bps (20%)
- Winners count: `ceil(participants × 0.30)` — top 30%
- Rounding: floor arithmetic + remainder to 1st place for cent-perfect totals

---

## System 1: Exponential Decay

**Files:** `packages/prize/tiers.go`, `packages/prize/distribution.go`

**Formula:**

```
weight(rank) = BasePct × Decay^(rank-1)
percentage(rank) = weight(rank) / Σ(all weights) × 100%
payout(rank) = 1 + floor(distributable × percentage(rank) / 100)
```

Where `distributable = prizePool - winnersCount` (1 cent reserved per winner as floor guarantee).

**Tier parameters (size-stratified):**

| Participants | BasePct | Decay |
|-------------|---------|-------|
| 1–10        | 50.0    | 0.55  |
| 11–50       | 35.0    | 0.70  |
| 51–250      | 25.0    | 0.80  |
| 251–1000    | 18.0    | 0.80  |
| 1001+       | 14.0    | 0.82  |

**Used by:** `packages/prize` package, `user-bff/contest_prizes.go` (prize preview endpoint)

**Tie handling:** Standard competition ranking (1,2,2,4...), tied ranks pool and split prizes equally.

---

## System 2: Hardcoded Percentage Table

**Files:** `apps/leaderboard-worker/payout.go`, `apps/leaderboard-worker/prize_distribution.json`

**Formula:**

```
payout(rank) = floor(prizePoolNet × percentage(rank) / 100)
```

Percentages from JSON config with 8 participant-count brackets. For rank ranges, percentage split equally: `percentagePerRank = rangePercentage / rangeSize`.

**Percentage tables:**

| Participants | Rank 1 | Rank 2 | Rank 3 | Remaining |
|-------------|--------|--------|--------|-----------|
| 1–10        | 50%    | 30%    | 20%    | —         |
| 11–20       | 40%    | 25%    | 15%    | 10%, 6%, 4% |
| 21–50       | 30%    | 20%    | 12%    | 8%, 6%, 14%(6-10), 10%(11-15) |
| 51–100      | 25%    | 15%    | 10%    | 7%, 5%, 15%(6-10), 10%(11-15), 7%(16-20), 6%(21-30) |
| 101–250     | 20%    | 12%    | 8%     | Distributed across 75 winners |
| 251–500     | 15%    | 10%    | 7%     | Distributed across 150 winners |
| 501–1000    | 12%    | 8%     | 5.5%   | Distributed across 300 winners |
| 1001+       | 10%    | 6.5%   | 4.5%   | Distributed across 30% of participants |

**Used by:** `leaderboard-worker` (actual contest finalization and wallet credits)

---

## Comparison with Tralent Power Law

**Tralent's model:** `prize(rank) = POOL × (1/rank^α) / Σ(1/i^α)`, α ≈ 1.095

| Property | Tralent Power Law | System 1 (Exponential Decay) | System 2 (Percentage Table) |
|----------|-------------------|------------------------------|-----------------------------|
| Formula type | Power law: `1/rank^α` | Exponential: `BasePct × Decay^(rank-1)` | Manually tuned lookup table |
| Parameters | Single α (1.095) | BasePct + Decay per tier | Fixed % per rank bracket |
| Size adaptation | Same α for all sizes | Different decay per contest size | Different table per bracket |
| Smoothness | Smooth continuous curve | Smooth continuous curve | Step function (discrete) |
| Rank 1 share (100p) | ~26.5% | ~25% (tier: 25.0, 0.80) | 25% (hardcoded) |
| Tail behavior | Long tail (power law) | Faster dropoff (exponential) | Arbitrary (manually set) |
| Configurability | Single parameter | 2 params per tier | Full manual control |

**Key mathematical difference:** Power law decays as `1/rank^α` (slower, "fatter tail"), while exponential decays as `Decay^rank` (faster dropoff). For large rank values, exponential decay produces significantly smaller prizes than power law — Tralent's model spreads rewards more broadly to lower-ranked winners.

---

## Potential Concern: Inconsistency

The **prize preview** (System 1, exponential decay in `packages/prize`) may differ from **actual payouts** (System 2, percentage table in `leaderboard-worker`). The settlement service also has its own `getPrizeDistribution()` function. These three paths could produce different results for the same contest.

---

## Key Code Locations

| File | Line(s) | Description |
|------|---------|-------------|
| `packages/prize/tiers.go:33-39` | Tier definitions | Exponential decay parameters |
| `packages/prize/distribution.go:99-111` | Core formula | `BasePct × Decay^i` weight calculation |
| `packages/prize/distribution.go:120-143` | Allocation | Floor + remainder distribution |
| `packages/prize/distribution.go:186-329` | Tie handling | `DistributeWithTies()` |
| `apps/leaderboard-worker/payout.go:238-342` | `AllocatePayouts()` | JSON-based percentage allocation |
| `apps/leaderboard-worker/payout.go:168-179` | Fee resolution | `ResolveEffectiveFeeBps()` |
| `apps/leaderboard-worker/prize_distribution.json` | Full config | All 8 bracket percentage tables |
| `apps/settlement-service/settlement.go:648+` | Settlement | `getPrizeDistribution()` (legacy) |
