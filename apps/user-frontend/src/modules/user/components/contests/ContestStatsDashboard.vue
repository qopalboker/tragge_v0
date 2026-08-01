<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';

interface UserContestStats {
  total_contests: number;
  total_wins: number;
  total_prizes_cents: number;
  win_rate: number;
  best_rank: number;
  average_rank: number;
  total_pnl: number;
  favorite_market?: string;
}

const props = defineProps<{
  stats: UserContestStats;
}>();

// Formatted values
const formattedPrizes = computed(() => {
  if (!props.stats.total_prizes_cents) return '$0';
  const amount = props.stats.total_prizes_cents / 100;
  if (amount >= 1000) {
    return `$${(amount / 1000).toFixed(1)}K`;
  }
  return `$${amount.toFixed(2)}`;
});

const formattedWinRate = computed(() => {
  return `${(props.stats.win_rate * 100).toFixed(1)}%`;
});

const formattedAvgRank = computed(() => {
  return props.stats.average_rank.toFixed(1);
});

const formattedPnl = computed(() => {
  const sign = props.stats.total_pnl >= 0 ? '+' : '';
  return `${sign}${props.stats.total_pnl.toFixed(2)}%`;
});

const pnlClass = computed(() => {
  if (props.stats.total_pnl > 0) return 'positive';
  if (props.stats.total_pnl < 0) return 'negative';
  return 'neutral';
});

function getMarketIcon(market: string | undefined): string {
  switch (market) {
    case 'crypto': return 'M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm1 17.93V19a1 1 0 0 1-2 0v-.93A7.44 7.44 0 0 1 5.07 13H6a1 1 0 0 1 0 2H5.07A7.44 7.44 0 0 1 11 20.93V20a1 1 0 0 1 2 0v.93A7.44 7.44 0 0 1 18.93 15H18a1 1 0 0 1 0-2h.93A7.44 7.44 0 0 1 13 5.07V5a1 1 0 0 1 2 0v.07a7.38 7.38 0 0 1 1.24.28 1 1 0 1 1-.58 1.91A5.43 5.43 0 0 0 12 6.5a5.5 5.5 0 1 0 5.5 5.5 5.4 5.4 0 0 0-.76-2.76 1 1 0 1 1 1.72-1.01A7.4 7.4 0 0 1 19.5 12 7.5 7.5 0 0 1 12 19.5a7.38 7.38 0 0 1-1-.07z';
    case 'forex': return 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z';
    case 'stocks': return 'M3 13h2v7H3zm5-4h2v11H8zm5-4h2v15h-2zm5 7h2v8h-2z';
    default: return 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93s3.05-7.44 7-7.93V4c-4.39.49-7.81 4.23-7.81 8.77 0 4.54 3.42 8.28 7.81 8.77v-.61zm1-17.86v.07c4.39.49 7.81 4.23 7.81 8.77 0 4.54-3.42 8.28-7.81 8.77V17a7.008 7.008 0 0 0 6-6.92c0-3.87-3.13-7-7-7v-.01z';
  }
}
</script>

<template>
  <div class="stats-dashboard">
    <div class="stats-grid">
      <!-- Total Contests -->
      <div class="stat-card">
        <div class="stat-icon contests">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
            <line x1="16" y1="2" x2="16" y2="6" />
            <line x1="8" y1="2" x2="8" y2="6" />
            <line x1="3" y1="10" x2="21" y2="10" />
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ stats.total_contests }}</span>
          <span class="stat-label">{{ t('contestStats.totalContests') }}</span>
        </div>
      </div>

      <!-- Total Wins -->
      <div class="stat-card">
        <div class="stat-icon wins">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6M18 9h1.5a2.5 2.5 0 0 0 0-5H18M4 22h16M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22M18 2H6v7a6 6 0 0 0 12 0V2Z" />
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ stats.total_wins }}</span>
          <span class="stat-label">{{ t('contestStats.totalWins') }}</span>
        </div>
      </div>

      <!-- Total Prizes -->
      <div class="stat-card highlight">
        <div class="stat-icon prizes">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="12" y1="1" x2="12" y2="23" />
            <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value prize">{{ formattedPrizes }}</span>
          <span class="stat-label">{{ t('contestStats.totalPrizes') }}</span>
        </div>
      </div>

      <!-- Win Rate -->
      <div class="stat-card">
        <div class="stat-icon rate">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <path d="M12 6v6l4 2" />
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ formattedWinRate }}</span>
          <span class="stat-label">{{ t('contestStats.winRate') }}</span>
        </div>
      </div>

      <!-- Best Rank -->
      <div class="stat-card">
        <div class="stat-icon best">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value">#{{ stats.best_rank || '-' }}</span>
          <span class="stat-label">{{ t('contestStats.bestRank') }}</span>
        </div>
      </div>

      <!-- Average Rank -->
      <div class="stat-card">
        <div class="stat-icon avg">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="18" y1="20" x2="18" y2="10" />
            <line x1="12" y1="20" x2="12" y2="4" />
            <line x1="6" y1="20" x2="6" y2="14" />
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value">#{{ formattedAvgRank }}</span>
          <span class="stat-label">{{ t('contestStats.avgRank') }}</span>
        </div>
      </div>

      <!-- Total P&L -->
      <div class="stat-card">
        <div class="stat-icon pnl">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="22 7 13.5 15.5 8.5 10.5 2 17" />
            <polyline points="16 7 22 7 22 13" />
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value" :class="pnlClass">{{ formattedPnl }}</span>
          <span class="stat-label">{{ t('contestStats.totalPnl') }}</span>
        </div>
      </div>

      <!-- Favorite Market -->
      <div v-if="stats.favorite_market" class="stat-card">
        <div class="stat-icon market">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
            <path :d="getMarketIcon(stats.favorite_market)" />
          </svg>
        </div>
        <div class="stat-content">
          <span class="stat-value capitalize">{{ stats.favorite_market }}</span>
          <span class="stat-label">{{ t('contestStats.favoriteMarket') }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.stats-dashboard {
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--spacing-md);
}

.stat-card {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  transition: all var(--transition-fast);
}

.stat-card:hover {
  background: var(--color-bg-tertiary);
}

.stat-card.highlight {
  background: linear-gradient(135deg, rgba(255, 215, 0, 0.1) 0%, rgba(255, 165, 0, 0.1) 100%);
  border: 1px solid rgba(255, 215, 0, 0.3);
}

.stat-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: var(--radius-md);
  flex-shrink: 0;
}

.stat-icon.contests {
  background: rgba(99, 102, 241, 0.1);
  color: #6366F1;
}

.stat-icon.wins {
  background: rgba(16, 185, 129, 0.1);
  color: #10B981;
}

.stat-icon.prizes {
  background: rgba(255, 215, 0, 0.2);
  color: #FFD700;
}

.stat-icon.rate {
  background: rgba(59, 130, 246, 0.1);
  color: #3B82F6;
}

.stat-icon.best {
  background: rgba(245, 158, 11, 0.1);
  color: #F59E0B;
}

.stat-icon.avg {
  background: rgba(168, 85, 247, 0.1);
  color: #A855F7;
}

.stat-icon.pnl {
  background: rgba(16, 185, 129, 0.1);
  color: #10B981;
}

.stat-icon.market {
  background: rgba(107, 114, 128, 0.1);
  color: #6B7280;
}

.stat-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.stat-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
  line-height: 1.2;
}

.stat-value.prize {
  color: #FFD700;
}

.stat-value.positive {
  color: #10B981;
}

.stat-value.negative {
  color: #EF4444;
}

.stat-value.neutral {
  color: var(--color-text-secondary);
}

.stat-value.capitalize {
  text-transform: capitalize;
}

.stat-label {
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

/* Mobile */
@media (max-width: 767px) {
  .stats-dashboard {
    padding: var(--spacing-md);
  }

  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
    gap: var(--spacing-sm);
  }

  .stat-card {
    padding: var(--spacing-sm);
    gap: var(--spacing-sm);
  }

  .stat-icon {
    width: 40px;
    height: 40px;
  }

  .stat-icon svg {
    width: 20px;
    height: 20px;
  }

  .stat-value {
    font-size: var(--font-size-lg);
  }

  .stat-label {
    font-size: 10px;
  }
}
</style>
