/**
 * Vue composable for prize distribution calculations.
 * Provides reactive prize pool information for contests.
 */

import { computed, type Ref, type ComputedRef } from 'vue';
import type { PrizePoolInfo, UserPrizeStatus } from '@tragge/contracts/v1';
import {
  getPrizePoolInfo,
  getUserPrizeStatus,
  isInPrizeZone,
  getPrizeZoneCutoff,
  getWinnersPercentage,
  formatPrize,
  getTopPrizes,
  getPrizeGroups,
  getUserPrizeSummary,
  getRankMedal,
  type FormattedPrize,
  type PrizeDisplayGroup,
} from '@/utils/prizeDistribution';

export interface UsePrizeDistributionOptions {
  /** Entry fee in cents */
  entryFeeCents: Ref<number> | number;
  /** Number of participants */
  participantCount: Ref<number> | number;
  /** User's current rank (optional) */
  userRank?: Ref<number | null | undefined> | number | null;
  /** User's current score (optional) */
  userScore?: Ref<number | null | undefined> | number | null;
  /** Minimum score to reach prize zone (optional) */
  prizeZoneMinScore?: Ref<number | null | undefined> | number | null;
  /** Locale for formatting */
  locale?: Ref<string> | string;
}

export interface UsePrizeDistributionReturn {
  /** Prize pool information */
  prizePoolInfo: ComputedRef<PrizePoolInfo | null>;
  /** User's prize status */
  userPrizeStatus: ComputedRef<UserPrizeStatus | null>;
  /** Whether user is in prize zone */
  userInPrizeZone: ComputedRef<boolean>;
  /** Prize zone cutoff rank */
  prizeZoneCutoff: ComputedRef<number>;
  /** Winners percentage */
  winnersPercentage: ComputedRef<number>;
  /** Formatted gross prize pool */
  grossPrizePool: ComputedRef<string>;
  /** Formatted net prize pool */
  netPrizePool: ComputedRef<string>;
  /** Number of winners */
  winnersCount: ComputedRef<number>;
  /** Top prizes for display */
  topPrizes: ComputedRef<FormattedPrize[]>;
  /** All prize groups for display */
  prizeGroups: ComputedRef<PrizeDisplayGroup[]>;
  /** User's projected prize (formatted) */
  userProjectedPrize: ComputedRef<string | null>;
  /** User prize summary text */
  userPrizeSummary: ComputedRef<{ text: string; subtext: string; inPrizeZone: boolean } | null>;
  /** Get medal emoji for a rank */
  getRankMedal: (rank: number) => string;
  /** Check if a specific rank is in prize zone */
  isRankInPrizeZone: (rank: number) => boolean;
}

/**
 * Get the value from a ref or primitive.
 */
function unref<T>(value: Ref<T> | T): T {
  return typeof value === 'object' && value !== null && 'value' in value
    ? (value as Ref<T>).value
    : value as T;
}

/**
 * Composable for prize distribution calculations.
 */
export function usePrizeDistribution(options: UsePrizeDistributionOptions): UsePrizeDistributionReturn {
  const {
    entryFeeCents,
    participantCount,
    userRank,
    userScore,
    prizeZoneMinScore,
    locale = 'en-US',
  } = options;

  const winnersPercentage = computed(() => getWinnersPercentage());

  const prizePoolInfo = computed(() => {
    const fee = unref(entryFeeCents);
    const count = unref(participantCount);
    if (fee <= 0 || count <= 0) {
      return null;
    }
    return getPrizePoolInfo(fee, count);
  });

  const prizeZoneCutoff = computed(() => {
    const count = unref(participantCount);
    return getPrizeZoneCutoff(count);
  });

  const userPrizeStatus = computed(() => {
    const rank = unref(userRank);
    const fee = unref(entryFeeCents);
    const count = unref(participantCount);
    const score = unref(userScore);
    const minScore = unref(prizeZoneMinScore);

    if (rank === null || rank === undefined || fee <= 0 || count <= 0) {
      return null;
    }

    return getUserPrizeStatus(
      rank,
      fee,
      count,
      score ?? undefined,
      minScore ?? undefined
    );
  });

  const userInPrizeZone = computed(() => {
    const rank = unref(userRank);
    const count = unref(participantCount);
    if (rank === null || rank === undefined || count <= 0) {
      return false;
    }
    return isInPrizeZone(rank, count);
  });

  const grossPrizePool = computed(() => {
    const info = prizePoolInfo.value;
    const loc = unref(locale);
    if (!info) return '$0.00';
    return formatPrize(info.grossPoolCents, loc);
  });

  const netPrizePool = computed(() => {
    const info = prizePoolInfo.value;
    const loc = unref(locale);
    if (!info) return '$0.00';
    return formatPrize(info.netPoolCents, loc);
  });

  const winnersCount = computed(() => {
    const info = prizePoolInfo.value;
    return info?.winnersCount ?? 0;
  });

  const topPrizes = computed(() => {
    const info = prizePoolInfo.value;
    const loc = unref(locale);
    if (!info) return [];
    return getTopPrizes(info, 10, loc);
  });

  const prizeGroups = computed(() => {
    const info = prizePoolInfo.value;
    const loc = unref(locale);
    if (!info) return [];
    return getPrizeGroups(info, loc);
  });

  const userProjectedPrize = computed(() => {
    const status = userPrizeStatus.value;
    const loc = unref(locale);
    if (!status || !status.inPrizeZone || status.projectedPrizeCents === null) {
      return null;
    }
    return formatPrize(status.projectedPrizeCents, loc);
  });

  const userPrizeSummary = computed(() => {
    const status = userPrizeStatus.value;
    const loc = unref(locale);
    if (!status) return null;
    return getUserPrizeSummary(status, loc);
  });

  const isRankInPrizeZone = (rank: number): boolean => {
    const count = unref(participantCount);
    return isInPrizeZone(rank, count);
  };

  return {
    prizePoolInfo,
    userPrizeStatus,
    userInPrizeZone,
    prizeZoneCutoff,
    winnersPercentage,
    grossPrizePool,
    netPrizePool,
    winnersCount,
    topPrizes,
    prizeGroups,
    userProjectedPrize,
    userPrizeSummary,
    getRankMedal,
    isRankInPrizeZone,
  };
}

// Re-export types and utilities
export type { FormattedPrize, PrizeDisplayGroup };
export { formatPrize, getRankMedal, getWinnersPercentage };
