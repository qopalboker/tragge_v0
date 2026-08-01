/**
 * Prize distribution types for Tragge trading tournaments.
 * These types mirror the prize distribution configuration used by the leaderboard-worker.
 */

/**
 * Single rank payout definition.
 * Used for top individual positions (1st, 2nd, 3rd, etc.)
 */
export interface SingleRankPayout {
  rank: number;
  percentage: number;
}

/**
 * Range-based payout definition.
 * Used for groups of ranks that share the same payout pool.
 */
export interface RangeRankPayout {
  rank_from: number;
  rank_to: number;
  percentage: number;
}

/**
 * Union type for payout definitions.
 */
export type PayoutDefinition = SingleRankPayout | RangeRankPayout;

/**
 * Type guard to check if a payout is a single rank payout.
 */
export function isSingleRankPayout(payout: PayoutDefinition): payout is SingleRankPayout {
  return 'rank' in payout && !('rank_from' in payout);
}

/**
 * Type guard to check if a payout is a range rank payout.
 */
export function isRangeRankPayout(payout: PayoutDefinition): payout is RangeRankPayout {
  return 'rank_from' in payout && 'rank_to' in payout;
}

/**
 * Distribution tier based on participant count.
 */
export interface DistributionTier {
  min_participants: number;
  max_participants: number | null;
  payouts: PayoutDefinition[];
}

/**
 * Complete prize distribution configuration.
 */
export interface PrizeDistributionConfig {
  version: string;
  name: string;
  description: string;
  winners_percentage: number;
  distributions: DistributionTier[];
}

/**
 * Calculated prize for a specific rank.
 */
export interface RankPrize {
  rank: number;
  prizeCents: number;
  percentage: number;
  isRange: boolean;
  rangeStart?: number;
  rangeEnd?: number;
}

/**
 * Prize pool information for a contest.
 */
export interface PrizePoolInfo {
  /** Gross prize pool before platform fee (entry_fee * participants) */
  grossPoolCents: number;
  /** Net prize pool after platform fee */
  netPoolCents: number;
  /** Platform fee in basis points (e.g., 2000 = 20%) */
  platformFeeBps: number;
  /** Total number of participants */
  participantCount: number;
  /** Number of winners (based on winners_percentage) */
  winnersCount: number;
  /** Winners percentage (e.g., 30 for top 30%) */
  winnersPercentage: number;
  /** List of prizes by rank */
  prizes: RankPrize[];
  /** Entry fee in cents */
  entryFeeCents: number;
}

/**
 * User's prize status in a contest.
 */
export interface UserPrizeStatus {
  /** User's current rank */
  rank: number;
  /** Whether user is in prize zone */
  inPrizeZone: boolean;
  /** Current projected prize (if in prize zone) */
  projectedPrizeCents: number | null;
  /** Score needed to reach prize zone (if not in prize zone) */
  scoreToReachPrizeZone: number | null;
  /** Rank of last prize winner */
  prizeZoneCutoff: number;
}

/**
 * Default platform fee in basis points (20%).
 */
export const DEFAULT_PLATFORM_FEE_BPS = 2000;

/**
 * Calculate the gross prize pool.
 */
export function calculateGrossPrizePool(entryFeeCents: number, participantCount: number): number {
  return entryFeeCents * participantCount;
}

/**
 * Calculate the net prize pool after platform fee.
 */
export function calculateNetPrizePool(grossPoolCents: number, platformFeeBps: number = DEFAULT_PLATFORM_FEE_BPS): number {
  return Math.floor(grossPoolCents * (10000 - platformFeeBps) / 10000);
}

/**
 * Calculate the number of winners based on participant count and winners percentage.
 */
export function calculateWinnersCount(participantCount: number, winnersPercentage: number): number {
  return Math.ceil(participantCount * winnersPercentage / 100);
}

/**
 * Get the appropriate distribution tier for a given participant count.
 */
export function getDistributionTier(
  distributions: DistributionTier[],
  participantCount: number
): DistributionTier | null {
  for (const tier of distributions) {
    const minOk = participantCount >= tier.min_participants;
    const maxOk = tier.max_participants === null || participantCount <= tier.max_participants;
    if (minOk && maxOk) {
      return tier;
    }
  }
  return null;
}

/**
 * Calculate prizes for all winning positions.
 */
export function calculatePrizes(
  netPoolCents: number,
  tier: DistributionTier,
  winnersCount: number
): RankPrize[] {
  const prizes: RankPrize[] = [];

  for (const payout of tier.payouts) {
    if (isSingleRankPayout(payout)) {
      if (payout.rank <= winnersCount) {
        prizes.push({
          rank: payout.rank,
          prizeCents: Math.floor(netPoolCents * payout.percentage / 100),
          percentage: payout.percentage,
          isRange: false,
        });
      }
    } else if (isRangeRankPayout(payout)) {
      // Calculate how many ranks in this range are winners
      const rangeStart = payout.rank_from;
      const rangeEnd = Math.min(payout.rank_to, winnersCount);

      if (rangeStart <= winnersCount) {
        const ranksInRange = rangeEnd - rangeStart + 1;
        const poolForRange = Math.floor(netPoolCents * payout.percentage / 100);
        const prizePerRank = Math.floor(poolForRange / ranksInRange);

        for (let rank = rangeStart; rank <= rangeEnd; rank++) {
          prizes.push({
            rank,
            prizeCents: prizePerRank,
            percentage: payout.percentage / ranksInRange,
            isRange: true,
            rangeStart: payout.rank_from,
            rangeEnd: payout.rank_to,
          });
        }
      }
    }
  }

  return prizes.sort((a, b) => a.rank - b.rank);
}

/**
 * Get prize for a specific rank.
 */
export function getPrizeForRank(prizes: RankPrize[], rank: number): RankPrize | null {
  return prizes.find(p => p.rank === rank) || null;
}

/**
 * Calculate complete prize pool info for a contest.
 */
export function calculatePrizePoolInfo(
  entryFeeCents: number,
  participantCount: number,
  config: PrizeDistributionConfig,
  platformFeeBps: number = DEFAULT_PLATFORM_FEE_BPS
): PrizePoolInfo | null {
  if (participantCount === 0) {
    return null;
  }

  const grossPoolCents = calculateGrossPrizePool(entryFeeCents, participantCount);
  const netPoolCents = calculateNetPrizePool(grossPoolCents, platformFeeBps);
  const winnersCount = calculateWinnersCount(participantCount, config.winners_percentage);
  const tier = getDistributionTier(config.distributions, participantCount);

  if (!tier) {
    return null;
  }

  const prizes = calculatePrizes(netPoolCents, tier, winnersCount);

  return {
    grossPoolCents,
    netPoolCents,
    platformFeeBps,
    participantCount,
    winnersCount,
    winnersPercentage: config.winners_percentage,
    prizes,
    entryFeeCents,
  };
}

/**
 * Calculate user's prize status.
 */
export function calculateUserPrizeStatus(
  userRank: number,
  prizePoolInfo: PrizePoolInfo,
  userScore?: number,
  prizeZoneMinScore?: number
): UserPrizeStatus {
  const inPrizeZone = userRank <= prizePoolInfo.winnersCount;
  const prize = getPrizeForRank(prizePoolInfo.prizes, userRank);

  return {
    rank: userRank,
    inPrizeZone,
    projectedPrizeCents: inPrizeZone && prize ? prize.prizeCents : null,
    scoreToReachPrizeZone: !inPrizeZone && userScore !== undefined && prizeZoneMinScore !== undefined
      ? prizeZoneMinScore - userScore
      : null,
    prizeZoneCutoff: prizePoolInfo.winnersCount,
  };
}

/**
 * Format cents to currency string.
 */
export function formatPrizeCents(cents: number, locale: string = 'en-US'): string {
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(cents / 100);
}
