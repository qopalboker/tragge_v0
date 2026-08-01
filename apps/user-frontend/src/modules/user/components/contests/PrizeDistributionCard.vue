<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';

const props = defineProps<{
  prizePoolCents: number;
  participantCount: number;
  prizeWinnersPercentage?: number;
}>();

// Prize distribution tiers (top 30% by default)
const winnersPercentage = computed(() => props.prizeWinnersPercentage ?? 30);

const winnersCount = computed(() => {
  return Math.max(1, Math.ceil(props.participantCount * (winnersPercentage.value / 100)));
});

// Prize distribution (simplified version)
// Based on standard distribution: 1st gets ~25%, 2nd ~15%, 3rd ~10%, rest shared
const prizeDistribution = computed(() => {
  const total = props.prizePoolCents;
  const count = winnersCount.value;

  if (count === 0 || total === 0) return [];

  const distribution: Array<{
    rank: string;
    percentage: number;
    amount: number;
  }> = [];

  // First place always gets 25%
  distribution.push({
    rank: '1st',
    percentage: 25,
    amount: Math.round(total * 0.25),
  });

  if (count >= 2) {
    distribution.push({
      rank: '2nd',
      percentage: 15,
      amount: Math.round(total * 0.15),
    });
  }

  if (count >= 3) {
    distribution.push({
      rank: '3rd',
      percentage: 10,
      amount: Math.round(total * 0.10),
    });
  }

  // Remaining is split among 4th to winnersCount
  if (count > 3) {
    const remaining = total - distribution.reduce((sum, d) => sum + d.amount, 0);
    const remainingCount = count - 3;
    const perPerson = Math.floor(remaining / remainingCount);
    const remainingPercentage = 50 / remainingCount;

    if (remainingCount <= 7) {
      // Show individual ranks
      for (let i = 4; i <= count; i++) {
        distribution.push({
          rank: `${i}th`,
          percentage: Math.round(remainingPercentage),
          amount: perPerson,
        });
      }
    } else {
      // Show as range
      distribution.push({
        rank: `4th-${count}th`,
        percentage: 50,
        amount: perPerson,
      });
    }
  }

  return distribution;
});

// Formatted prize pool
const formattedPrizePool = computed(() => {
  const amount = props.prizePoolCents / 100;
  if (amount >= 1000) {
    return `$${(amount / 1000).toFixed(1)}K`;
  }
  return `$${amount.toFixed(2)}`;
});

// Format amount
function formatAmount(cents: number): string {
  const amount = cents / 100;
  return `$${amount.toFixed(2)}`;
}

// Get rank medal
function getRankMedal(rank: string): string {
  if (rank === '1st') return '🥇';
  if (rank === '2nd') return '🥈';
  if (rank === '3rd') return '🥉';
  return '';
}
</script>

<template>
  <div class="prize-distribution-card">
    <div class="card-header">
      <div class="header-icon">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" />
        </svg>
      </div>
      <h3 class="header-title">{{ t('contestResults.prizeDistribution') }}</h3>
    </div>

    <div class="prize-summary">
      <div class="summary-item">
        <span class="summary-label">{{ t('contestResults.totalPrizePool') }}</span>
        <span class="summary-value">{{ formattedPrizePool }}</span>
      </div>
      <div class="summary-divider"></div>
      <div class="summary-item">
        <span class="summary-label">{{ t('contestResults.winnersCount') }}</span>
        <span class="summary-value">{{ winnersCount }}</span>
      </div>
    </div>

    <div class="distribution-list">
      <div
        v-for="item in prizeDistribution"
        :key="item.rank"
        class="distribution-item"
      >
        <div class="rank-info">
          <span v-if="getRankMedal(item.rank)" class="rank-medal">
            {{ getRankMedal(item.rank) }}
          </span>
          <span class="rank-label">{{ item.rank }}</span>
        </div>
        <div class="prize-info">
          <span class="prize-amount">{{ formatAmount(item.amount) }}</span>
          <span class="prize-percentage">({{ item.percentage }}%)</span>
        </div>
      </div>
    </div>

    <div class="distribution-note">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="10" />
        <path d="M12 16v-4M12 8h.01" />
      </svg>
      <span>{{ t('contestResults.prizeDistributionNote', { percentage: winnersPercentage }) }}</span>
    </div>
  </div>
</template>

<style scoped>
.prize-distribution-card {
  padding: var(--spacing-lg);
}

.card-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-lg);
}

.header-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: linear-gradient(135deg, #FEF3C7 0%, #FDE68A 100%);
  border-radius: var(--radius-md);
}

.header-icon svg {
  color: #D97706;
}

.header-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.prize-summary {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xl);
  padding: var(--spacing-md);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  margin-bottom: var(--spacing-lg);
}

.summary-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
}

.summary-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.summary-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.summary-divider {
  width: 1px;
  height: 40px;
  background: var(--color-border);
}

.distribution-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.distribution-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  transition: background-color var(--transition-fast);
}

.distribution-item:hover {
  background: var(--color-bg-tertiary);
}

.rank-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.rank-medal {
  font-size: 1.25rem;
}

.rank-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.prize-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.prize-amount {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: #059669;
  font-variant-numeric: tabular-nums;
}

.prize-percentage {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.distribution-note {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
  margin-top: var(--spacing-md);
  padding: var(--spacing-sm);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.distribution-note svg {
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.distribution-note span {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

/* RTL Support */
[dir="rtl"] .card-header,
[dir="rtl"] .rank-info,
[dir="rtl"] .prize-info {
  flex-direction: row-reverse;
}

[dir="rtl"] .distribution-item {
  flex-direction: row-reverse;
}

/* Mobile */
@media (max-width: 767px) {
  .prize-distribution-card {
    padding: var(--spacing-md);
  }

  .prize-summary {
    gap: var(--spacing-lg);
    padding: var(--spacing-sm);
  }

  .summary-value {
    font-size: var(--font-size-lg);
  }

  .distribution-item {
    padding: var(--spacing-xs) var(--spacing-sm);
  }
}
</style>
