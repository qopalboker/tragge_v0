/**
 * Prize distribution utility for the user frontend.
 * Loads the distribution config and provides calculation functions.
 */

import type {
  PrizeDistributionConfig,
  PrizePoolInfo,
  RankPrize,
  UserPrizeStatus,
} from '@tragge/contracts/v1';

import {
  calculatePrizePoolInfo,
  calculateUserPrizeStatus,
  getPrizeForRank,
  formatPrizeCents,
  DEFAULT_PLATFORM_FEE_BPS,
} from '@tragge/contracts/v1';

// Import the prize distribution config JSON
import distributionConfig from '../../../../packages/contracts/prize_distribution/tralent_like_v1.json';

/**
 * The loaded prize distribution configuration.
 */
export const prizeDistributionConfig = distributionConfig as PrizeDistributionConfig;

/**
 * Get prize pool information for a contest.
 */
export function getPrizePoolInfo(
  entryFeeCents: number,
  participantCount: number,
  platformFeeBps: number = DEFAULT_PLATFORM_FEE_BPS
): PrizePoolInfo | null {
  return calculatePrizePoolInfo(entryFeeCents, participantCount, prizeDistributionConfig, platformFeeBps);
}

/**
 * Get user's prize status in a contest.
 */
export function getUserPrizeStatus(
  userRank: number,
  entryFeeCents: number,
  participantCount: number,
  userScore?: number,
  prizeZoneMinScore?: number
): UserPrizeStatus | null {
  const prizePoolInfo = getPrizePoolInfo(entryFeeCents, participantCount);
  if (!prizePoolInfo) {
    return null;
  }
  return calculateUserPrizeStatus(userRank, prizePoolInfo, userScore, prizeZoneMinScore);
}

/**
 * Check if a rank is in the prize zone.
 */
export function isInPrizeZone(rank: number, participantCount: number): boolean {
  const winnersCount = Math.ceil(participantCount * prizeDistributionConfig.winners_percentage / 100);
  return rank <= winnersCount;
}

/**
 * Get the prize zone cutoff rank.
 */
export function getPrizeZoneCutoff(participantCount: number): number {
  return Math.ceil(participantCount * prizeDistributionConfig.winners_percentage / 100);
}

/**
 * Get the winners percentage from config.
 */
export function getWinnersPercentage(): number {
  return prizeDistributionConfig.winners_percentage;
}

/**
 * Format prize amount for display.
 */
export function formatPrize(cents: number, locale: string = 'en-US'): string {
  return formatPrizeCents(cents, locale);
}

/**
 * Get top N prizes with formatting.
 */
export interface FormattedPrize {
  rank: number;
  amount: string;
  amountCents: number;
  label: string;
  isRange: boolean;
}

export function getTopPrizes(
  prizePoolInfo: PrizePoolInfo,
  count: number = 10,
  locale: string = 'en-US'
): FormattedPrize[] {
  const prizes = prizePoolInfo.prizes.slice(0, count);

  return prizes.map((prize: RankPrize) => ({
    rank: prize.rank,
    amount: formatPrizeCents(prize.prizeCents, locale),
    amountCents: prize.prizeCents,
    label: getRankLabel(prize.rank),
    isRange: prize.isRange,
  }));
}

/**
 * Get all prizes grouped by range for display.
 */
export interface PrizeDisplayGroup {
  label: string;
  rankStart: number;
  rankEnd: number;
  prizePerRank: string;
  prizePerRankCents: number;
  totalPrize: string;
  totalPrizeCents: number;
  rankCount: number;
}

export function getPrizeGroups(prizePoolInfo: PrizePoolInfo, locale: string = 'en-US'): PrizeDisplayGroup[] {
  const groups: PrizeDisplayGroup[] = [];
  const prizes = prizePoolInfo.prizes;

  let i = 0;
  while (i < prizes.length) {
    const prize = prizes[i];

    if (prize.isRange && prize.rangeStart !== undefined && prize.rangeEnd !== undefined) {
      // Find all prizes in this range
      const rangeStart = prize.rangeStart;
      const rangeEnd = Math.min(prize.rangeEnd, prizePoolInfo.winnersCount);
      const rankCount = rangeEnd - rangeStart + 1;
      const totalPrizeCents = prize.prizeCents * rankCount;

      groups.push({
        label: rankCount === 1 ? getRankLabel(rangeStart) : `${rangeStart}-${rangeEnd}`,
        rankStart: rangeStart,
        rankEnd: rangeEnd,
        prizePerRank: formatPrizeCents(prize.prizeCents, locale),
        prizePerRankCents: prize.prizeCents,
        totalPrize: formatPrizeCents(totalPrizeCents, locale),
        totalPrizeCents,
        rankCount,
      });

      // Skip all prizes in this range
      i += rankCount;
    } else {
      // Single rank prize
      groups.push({
        label: getRankLabel(prize.rank),
        rankStart: prize.rank,
        rankEnd: prize.rank,
        prizePerRank: formatPrizeCents(prize.prizeCents, locale),
        prizePerRankCents: prize.prizeCents,
        totalPrize: formatPrizeCents(prize.prizeCents, locale),
        totalPrizeCents: prize.prizeCents,
        rankCount: 1,
      });
      i++;
    }
  }

  return groups;
}

/**
 * Get rank label with ordinal suffix.
 */
export function getRankLabel(rank: number): string {
  const suffixes = ['th', 'st', 'nd', 'rd'];
  const v = rank % 100;
  return rank + (suffixes[(v - 20) % 10] || suffixes[v] || suffixes[0]);
}

/**
 * Get medal emoji for rank.
 */
export function getRankMedal(rank: number): string {
  switch (rank) {
    case 1: return '🥇';
    case 2: return '🥈';
    case 3: return '🥉';
    default: return '';
  }
}

/**
 * Get prize summary text for a user.
 */
export function getUserPrizeSummary(
  userPrizeStatus: UserPrizeStatus,
  locale: string = 'en-US'
): { text: string; subtext: string; inPrizeZone: boolean } {
  if (userPrizeStatus.inPrizeZone && userPrizeStatus.projectedPrizeCents !== null) {
    return {
      text: formatPrizeCents(userPrizeStatus.projectedPrizeCents, locale),
      subtext: `Projected prize at rank #${userPrizeStatus.rank}`,
      inPrizeZone: true,
    };
  }

  const ranksAway = userPrizeStatus.rank - userPrizeStatus.prizeZoneCutoff;
  return {
    text: `${ranksAway} ${ranksAway === 1 ? 'rank' : 'ranks'} away`,
    subtext: `from prize zone (top ${userPrizeStatus.prizeZoneCutoff})`,
    inPrizeZone: false,
  };
}

// Re-export types and utilities for convenience
export {
  PrizePoolInfo,
  RankPrize,
  UserPrizeStatus,
  DEFAULT_PLATFORM_FEE_BPS,
  getPrizeForRank,
};
