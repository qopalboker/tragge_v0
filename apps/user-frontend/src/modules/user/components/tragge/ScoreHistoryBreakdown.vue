<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';
import type { ScoreHistoryEntry } from '@/api';

interface Props {
  entries: ScoreHistoryEntry[];
  loading?: boolean;
  showLimit?: number;
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  showLimit: 10,
});

const emit = defineEmits<{
  viewAll: [];
}>();

interface EnrichedEntry extends ScoreHistoryEntry {
  contribution: number;
  isHighest: boolean;
}

const enrichedEntries = computed<EnrichedEntry[]>(() => {
  if (!props.entries.length) return [];

  // Calculate contribution for each entry based on rank, participants, and score
  const withContributions = props.entries.map(entry => {
    // Simple contribution calculation based on the T-Point formula
    const participantMultiplier = Math.min(1.5, Math.max(0.1, Math.log10(entry.participants) / Math.log10(1000)));
    const rankBonus = Math.min(1.5, Math.max(1.0, 1.0 + (0.5 * (1 - entry.rank / entry.participants))));
    const contribution = entry.score > 0 ? entry.score * participantMultiplier * rankBonus : 0;

    return {
      ...entry,
      contribution: Math.round(contribution * 100) / 100,
      isHighest: false,
    };
  });

  // Mark the highest contribution
  const maxContribution = Math.max(...withContributions.map(e => e.contribution));
  return withContributions.map(entry => ({
    ...entry,
    isHighest: entry.contribution === maxContribution && maxContribution > 0,
  }));
});

const displayedEntries = computed(() => {
  return enrichedEntries.value.slice(0, props.showLimit);
});

const totalContribution = computed(() => {
  return enrichedEntries.value.reduce((sum, e) => sum + e.contribution, 0);
});

const highestEntry = computed(() => {
  return enrichedEntries.value.find(e => e.isHighest);
});

function formatDate(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

function getRankDisplay(rank: number): string {
  if (rank === 1) return '1st';
  if (rank === 2) return '2nd';
  if (rank === 3) return '3rd';
  return `${rank}th`;
}

function getRankIcon(rank: number): string {
  if (rank === 1) return '🥇';
  if (rank === 2) return '🥈';
  if (rank === 3) return '🥉';
  return '';
}
</script>

<template>
  <div class="score-breakdown">
    <div class="breakdown-header">
      <h3 class="breakdown-title">{{ t('tragge.pointBreakdown') }}</h3>
      <span class="total-score">
        {{ t('tragge.totalContribution') }}: <strong>{{ totalContribution.toLocaleString() }}</strong>
      </span>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Empty State -->
    <div v-else-if="!entries.length" class="empty-state">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M12 20V10M18 20V4M6 20v-4" />
      </svg>
      <p>{{ t('tragge.noHistory') }}</p>
    </div>

    <!-- Highest Score Highlight -->
    <div v-else>
      <div v-if="highestEntry" class="highest-score-card">
        <div class="highest-badge">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
          </svg>
          {{ t('tragge.biggestPoint') }}
        </div>
        <div class="highest-content">
          <span class="contest-name">{{ highestEntry.contest_name }}</span>
          <span class="contribution-value">+{{ highestEntry.contribution.toLocaleString() }}</span>
        </div>
        <div class="highest-details">
          {{ getRankDisplay(highestEntry.rank) }} of {{ highestEntry.participants }}
          &bull; {{ formatDate(highestEntry.created_at) }}
        </div>
      </div>

      <!-- History List -->
      <div class="history-list">
        <div
          v-for="entry in displayedEntries"
          :key="entry.contest_id"
          :class="['history-item', { 'is-highlighted': entry.isHighest }]"
        >
          <div class="item-main">
            <div class="rank-badge">
              <span v-if="getRankIcon(entry.rank)" class="rank-icon">{{ getRankIcon(entry.rank) }}</span>
              <span class="rank-number">{{ entry.rank }}</span>
            </div>
            <div class="item-info">
              <span class="contest-name">{{ entry.contest_name }}</span>
              <span class="contest-meta">
                {{ entry.participants }} {{ t('tragge.participants') }} &bull;
                {{ formatDate(entry.created_at) }}
              </span>
            </div>
          </div>
          <div class="item-stats">
            <div class="score-contribution">
              <span class="contribution-label">{{ t('tragge.contribution') }}</span>
              <span class="contribution-value">+{{ entry.contribution.toLocaleString() }}</span>
            </div>
            <div class="original-score">
              <span class="score-label">{{ t('tragge.tournamentPoint') }}</span>
              <span :class="['score-value', { positive: entry.score > 0, negative: entry.score < 0 }]">
                {{ entry.score > 0 ? '+' : '' }}{{ entry.score.toFixed(2) }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- View All Button -->
      <button
        v-if="entries.length > showLimit"
        class="btn btn-secondary view-all-btn"
        @click="emit('viewAll')"
      >
        {{ t('tragge.viewAllHistory', { count: entries.length }) }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.score-breakdown {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
}

.breakdown-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--spacing-lg);
  flex-wrap: wrap;
  gap: var(--spacing-sm);
}

.breakdown-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  margin: 0;
}

.total-score {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.total-score strong {
  color: var(--color-primary);
  font-weight: 600;
}

.loading-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-2xl);
  gap: var(--spacing-md);
  color: var(--color-text-secondary);
}

.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.highest-score-card {
  background: linear-gradient(135deg, var(--color-primary) 0%, #7C3AED 100%);
  border-radius: var(--radius-md);
  padding: var(--spacing-lg);
  margin-bottom: var(--spacing-lg);
  color: white;
}

.highest-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  opacity: 0.9;
  margin-bottom: var(--spacing-sm);
}

.highest-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
}

.highest-content .contest-name {
  font-size: var(--font-size-lg);
  font-weight: 600;
}

.highest-content .contribution-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
}

.highest-details {
  font-size: var(--font-size-sm);
  opacity: 0.85;
  margin-top: var(--spacing-xs);
}

.history-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.history-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  gap: var(--spacing-md);
  transition: background-color var(--transition-fast);
}

.history-item:hover {
  background-color: var(--color-bg-tertiary);
}

.history-item.is-highlighted {
  background-color: rgba(var(--color-primary-rgb, 59, 130, 246), 0.1);
  border: 1px solid var(--color-primary);
}

.item-main {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  flex: 1;
  min-width: 0;
}

.rank-badge {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 40px;
}

.rank-icon {
  font-size: var(--font-size-lg);
}

.rank-number {
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.item-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.contest-name {
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.contest-meta {
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
}

.item-stats {
  display: flex;
  align-items: center;
  gap: var(--spacing-lg);
  flex-shrink: 0;
}

.score-contribution,
.original-score {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}

[dir="rtl"] .score-contribution,
[dir="rtl"] .original-score {
  align-items: flex-start;
}

.contribution-label,
.score-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
}

.contribution-value {
  font-weight: 600;
  color: var(--color-primary);
}

.score-value {
  font-weight: 500;
  font-size: var(--font-size-sm);
}

.score-value.positive {
  color: var(--color-success);
}

.score-value.negative {
  color: var(--color-error);
}

.view-all-btn {
  width: 100%;
  margin-top: var(--spacing-md);
}

@media (max-width: 767px) {
  .history-item {
    flex-direction: column;
    align-items: flex-start;
  }

  .item-main {
    width: 100%;
  }

  .item-stats {
    width: 100%;
    justify-content: space-between;
    padding-top: var(--spacing-sm);
    border-top: 1px solid var(--color-border);
    margin-top: var(--spacing-sm);
  }

  .score-contribution,
  .original-score {
    align-items: flex-start;
  }

  [dir="rtl"] .score-contribution,
  [dir="rtl"] .original-score {
    align-items: flex-end;
  }
}
</style>
