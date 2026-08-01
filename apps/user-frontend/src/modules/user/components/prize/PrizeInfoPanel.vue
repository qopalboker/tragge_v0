<script setup lang="ts">
import { computed, toRef } from 'vue';
import { t } from '@/i18n';
import { usePrizeDistribution } from '@/composables/usePrizeDistribution';

interface Props {
  entryFeeCents: number;
  participantCount: number;
  userRank?: number | null;
  userScore?: number | null;
  prizeZoneMinScore?: number | null;
  locale?: string;
  showUserStatus?: boolean;
  showBreakdown?: boolean;
  compact?: boolean;
}

const props = withDefaults(defineProps<Props>(), {
  userRank: null,
  userScore: null,
  prizeZoneMinScore: null,
  locale: 'en-US',
  showUserStatus: true,
  showBreakdown: true,
  compact: false,
});

const {
  netPrizePool,
  winnersCount,
  prizeGroups,
  userPrizeStatus,
  userInPrizeZone,
  userPrizeSummary,
  getRankMedal,
} = usePrizeDistribution({
  entryFeeCents: toRef(() => props.entryFeeCents),
  participantCount: toRef(() => props.participantCount),
  userRank: toRef(() => props.userRank),
  userScore: toRef(() => props.userScore),
  prizeZoneMinScore: toRef(() => props.prizeZoneMinScore),
  locale: toRef(() => props.locale),
});

// Display groups (limited in compact mode)
const displayGroups = computed(() => {
  const groups = prizeGroups.value;
  if (props.compact) {
    // Show top 5 in compact mode + last prize
    if (groups.length <= 6) return groups;
    return [...groups.slice(0, 5), groups[groups.length - 1]];
  }
  return groups;
});

// Show "..." row in compact mode
const showEllipsis = computed(() => {
  return props.compact && prizeGroups.value.length > 6;
});
</script>

<template>
  <div :class="['prize-info-panel', { compact }]">
    <!-- Header with Prize Pool -->
    <div class="prize-header">
      <div class="prize-pool-icon">
        <span class="prize-emoji">&#x1F4B0;</span>
      </div>
      <div class="prize-pool-info">
        <span class="prize-pool-label">{{ t('prize.prizePool') }}</span>
        <span class="prize-pool-amount">{{ netPrizePool }}</span>
      </div>
    </div>

    <!-- Participants & Winners Info -->
    <div class="prize-stats">
      <div class="stat-item">
        <span class="stat-value">{{ participantCount }}</span>
        <span class="stat-label">{{ t('prize.participants') }}</span>
      </div>
      <div class="stat-divider"></div>
      <div class="stat-item">
        <span class="stat-value">{{ winnersCount }}</span>
        <span class="stat-label">{{ t('prize.topWinPrizes', { count: winnersCount }) }}</span>
      </div>
    </div>

    <!-- User Prize Status (if applicable) -->
    <div
      v-if="showUserStatus && userPrizeStatus && userRank"
      :class="['user-prize-status', { 'in-zone': userInPrizeZone, 'out-zone': !userInPrizeZone }]"
    >
      <div class="status-header">
        <span class="status-icon">{{ userInPrizeZone ? '&#x1F4B5;' : '&#x1F3AF;' }}</span>
        <span class="status-title">{{ t('prize.yourStatus') }}</span>
      </div>
      <div class="status-content">
        <div class="rank-info">
          <span class="rank-label">{{ t('prize.currentRank') }}:</span>
          <span class="rank-value">#{{ userRank }}</span>
        </div>
        <div v-if="userPrizeSummary" class="prize-summary">
          <span class="summary-text">{{ userPrizeSummary.text }}</span>
          <span class="summary-subtext">{{ userPrizeSummary.subtext }}</span>
        </div>
      </div>
    </div>

    <!-- Prize Breakdown -->
    <div v-if="showBreakdown && displayGroups.length > 0" class="prize-breakdown">
      <div class="breakdown-header">
        <span class="breakdown-title">{{ t('prize.prizeBreakdown') }}</span>
      </div>
      <div class="breakdown-list">
        <div
          v-for="(group, index) in displayGroups"
          :key="index"
          :class="['prize-row', { 'current-rank': userRank && userRank >= group.rankStart && userRank <= group.rankEnd }]"
        >
          <div class="prize-rank">
            <span v-if="group.rankStart <= 3" class="rank-medal">{{ getRankMedal(group.rankStart) }}</span>
            <span class="rank-text">{{ group.label }}</span>
          </div>
          <div class="prize-amount">
            <span class="amount-value">{{ group.prizePerRank }}</span>
            <span v-if="group.rankCount > 1" class="amount-note">({{ t('prize.perRank') }})</span>
          </div>
        </div>
        <!-- Ellipsis row for compact mode -->
        <div v-if="showEllipsis" class="prize-row ellipsis-row">
          <div class="prize-rank">
            <span class="rank-text">...</span>
          </div>
          <div class="prize-amount">
            <span class="amount-value">...</span>
          </div>
        </div>
      </div>
      <!-- Last prize indicator -->
      <div v-if="prizeGroups.length > 0" class="last-prize-note">
        <span class="note-text">
          {{ prizeGroups[prizeGroups.length - 1].label }}: {{ prizeGroups[prizeGroups.length - 1].prizePerRank }}
          ({{ t('prize.minimum') }})
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.prize-info-panel {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

.prize-info-panel.compact {
  padding: var(--spacing-md);
  gap: var(--spacing-sm);
}

/* Header */
.prize-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding-bottom: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
}

.prize-pool-icon {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #FEF3C7 0%, #FDE68A 100%);
  border-radius: var(--radius-md);
}

.compact .prize-pool-icon {
  width: 40px;
  height: 40px;
}

.prize-emoji {
  font-size: 1.5rem;
}

.compact .prize-emoji {
  font-size: 1.25rem;
}

.prize-pool-info {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.prize-pool-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.prize-pool-amount {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.compact .prize-pool-amount {
  font-size: var(--font-size-xl);
}

/* Stats */
.prize-stats {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-lg);
  padding: var(--spacing-sm) 0;
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
}

.stat-value {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
}

.stat-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-align: center;
}

.stat-divider {
  width: 1px;
  height: 32px;
  background-color: var(--color-border);
}

/* User Prize Status */
.user-prize-status {
  padding: var(--spacing-md);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-secondary);
}

.user-prize-status.in-zone {
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.1) 0%, rgba(6, 182, 212, 0.05) 100%);
  border: 1px solid rgba(16, 185, 129, 0.3);
}

.user-prize-status.out-zone {
  background: linear-gradient(135deg, rgba(251, 191, 36, 0.1) 0%, rgba(245, 158, 11, 0.05) 100%);
  border: 1px solid rgba(251, 191, 36, 0.3);
}

.status-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-sm);
}

.status-icon {
  font-size: 1.25rem;
}

.status-title {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
}

.status-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
}

.rank-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.rank-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.rank-value {
  font-size: var(--font-size-lg);
  font-weight: 700;
  color: var(--color-text-primary);
}

.prize-summary {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}

.summary-text {
  font-size: var(--font-size-md);
  font-weight: 600;
}

.in-zone .summary-text {
  color: #059669;
}

.out-zone .summary-text {
  color: #D97706;
}

.summary-subtext {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

/* Prize Breakdown */
.prize-breakdown {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.breakdown-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.breakdown-title {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.breakdown-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.prize-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-sm);
  background-color: var(--color-bg-secondary);
  transition: background-color 0.2s;
}

.prize-row:hover {
  background-color: var(--color-bg-tertiary);
}

.prize-row.current-rank {
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.1) 0%, rgba(99, 102, 241, 0.05) 100%);
  border: 1px solid rgba(59, 130, 246, 0.3);
}

.prize-row.ellipsis-row {
  background-color: transparent;
  justify-content: center;
  padding: var(--spacing-xs);
}

.ellipsis-row .prize-rank,
.ellipsis-row .prize-amount {
  color: var(--color-text-muted);
}

.prize-rank {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.rank-medal {
  font-size: 1.125rem;
}

.rank-text {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.prize-amount {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.amount-value {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: #059669;
  font-family: var(--font-mono, monospace);
}

.amount-note {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

/* Last Prize Note */
.last-prize-note {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-xs);
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
}

.note-text {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

/* Mobile Responsive */
@media (max-width: 640px) {
  .prize-info-panel {
    padding: var(--spacing-md);
  }

  .prize-header {
    flex-direction: column;
    text-align: center;
  }

  .status-content {
    flex-direction: column;
    align-items: stretch;
    gap: var(--spacing-sm);
  }

  .prize-summary {
    align-items: flex-start;
  }
}
</style>
