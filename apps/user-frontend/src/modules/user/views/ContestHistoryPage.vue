<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { api } from '@/api';
import ContestStatsDashboard from '@/components/contests/ContestStatsDashboard.vue';

// Types
interface ContestHistoryEntry {
  contest_id: string;
  contest_name: string;
  status: 'completed' | 'cancelled' | 'running';
  starts_at: string;
  ends_at: string;
  joined_at: string;
  total_score: number;
  pnl_percent?: number;
  final_rank?: number;
  total_participants: number;
  final_prize_cents?: number;
  trade_count?: number;
  duration_type?: string;
  market_type?: string;
}

interface ContestHistoryResponse {
  contests: ContestHistoryEntry[];
  total: number;
  page: number;
  per_page: number;
}

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

type FilterType = 'all' | 'wins' | 'prize_zone' | 'no_prize';

const router = useRouter();

// State
const history = ref<ContestHistoryEntry[]>([]);
const stats = ref<UserContestStats | null>(null);
const loading = ref(true);
const loadingMore = ref(false);
const error = ref<string | null>(null);
const activeFilter = ref<FilterType>('all');
const page = ref(1);
const hasMore = ref(true);
const perPage = 10;

// Computed
const filteredHistory = computed(() => {
  switch (activeFilter.value) {
    case 'wins':
      return history.value.filter(c => c.final_rank === 1);
    case 'prize_zone':
      return history.value.filter(c => (c.final_prize_cents ?? 0) > 0);
    case 'no_prize':
      return history.value.filter(c => (c.final_prize_cents ?? 0) === 0 && c.status === 'completed');
    default:
      return history.value;
  }
});

const filterCounts = computed(() => {
  return {
    all: history.value.length,
    wins: history.value.filter(c => c.final_rank === 1).length,
    prize_zone: history.value.filter(c => (c.final_prize_cents ?? 0) > 0).length,
    no_prize: history.value.filter(c => (c.final_prize_cents ?? 0) === 0 && c.status === 'completed').length,
  };
});

// Fetch contest history
async function fetchHistory(loadMore = false): Promise<void> {
  if (loadMore) {
    loadingMore.value = true;
  } else {
    loading.value = true;
    page.value = 1;
    history.value = [];
  }
  error.value = null;

  try {
    const response = await api.get<ContestHistoryResponse>('/api/user/me/contest-history', {
      params: {
        page: page.value,
        per_page: perPage,
      },
    });

    if (loadMore) {
      history.value = [...history.value, ...response.data.contests];
    } else {
      history.value = response.data.contests;
    }

    hasMore.value = response.data.contests.length === perPage;
  } catch (err) {
    console.error('Failed to fetch contest history:', err);
    error.value = t('contestHistory.loadError');
  } finally {
    loading.value = false;
    loadingMore.value = false;
  }
}

// Fetch user stats
async function fetchStats(): Promise<void> {
  try {
    const response = await api.get<UserContestStats>('/api/user/me/contest-stats');
    stats.value = response.data;
  } catch (err) {
    console.error('Failed to fetch contest stats:', err);
    // Non-critical, don't show error
  }
}

// Load more
async function loadMore(): Promise<void> {
  if (loadingMore.value || !hasMore.value) return;
  page.value++;
  await fetchHistory(true);
}

// Filter change
function setFilter(filter: FilterType): void {
  activeFilter.value = filter;
}

// Navigation
function viewResults(contestId: string): void {
  router.push(`/user/contests/${contestId}/results`);
}

function viewDetails(contestId: string): void {
  router.push(`/user/contests/${contestId}`);
}

// Formatting helpers
function formatDate(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

function formatPnl(pnlPercent: number | undefined): string {
  if (pnlPercent === undefined || pnlPercent === null) return '+0.00%';
  const sign = pnlPercent >= 0 ? '+' : '';
  return `${sign}${pnlPercent.toFixed(2)}%`;
}

function getPnlClass(pnlPercent: number | undefined): string {
  if (!pnlPercent) return 'neutral';
  if (pnlPercent > 0) return 'positive';
  if (pnlPercent < 0) return 'negative';
  return 'neutral';
}

function formatPrize(cents: number | undefined): string {
  if (!cents || cents === 0) return '$0';
  const amount = cents / 100;
  return `$${amount.toFixed(2)}`;
}

function getRankBadgeClass(rank: number | undefined): string {
  if (!rank) return '';
  if (rank === 1) return 'rank-gold';
  if (rank === 2) return 'rank-silver';
  if (rank === 3) return 'rank-bronze';
  return '';
}

function getStatusClass(status: string): string {
  switch (status) {
    case 'completed': return 'status-completed';
    case 'running': return 'status-live';
    case 'cancelled': return 'status-cancelled';
    default: return '';
  }
}

function getStatusLabel(status: string): string {
  switch (status) {
    case 'completed': return t('contestHistory.statusCompleted');
    case 'running': return t('contestHistory.statusLive');
    case 'cancelled': return t('contestHistory.statusCancelledRefunded');
    default: return status;
  }
}

// Lifecycle
onMounted(async () => {
  await Promise.all([fetchHistory(), fetchStats()]);
});

// Reset page when filter changes
watch(activeFilter, () => {
  // Filtered view is computed, no need to refetch
});
</script>

<template>
  <div class="contest-history-page">
    <div class="page-header">
      <h1 class="page-title">{{ t('contestHistory.title') }}</h1>
      <p class="page-subtitle">{{ t('contestHistory.subtitle') }}</p>
    </div>

    <!-- Stats Dashboard -->
    <ContestStatsDashboard v-if="stats" :stats="stats" class="stats-section" />

    <!-- Filter Tabs -->
    <div class="filter-tabs">
      <button
        v-for="filter in (['all', 'wins', 'prize_zone', 'no_prize'] as FilterType[])"
        :key="filter"
        class="filter-tab"
        :class="{ active: activeFilter === filter }"
        @click="setFilter(filter)"
      >
        {{ t(`contestHistory.filter.${filter}`) }}
        <span class="filter-count">{{ filterCounts[filter] }}</span>
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="loading-container">
      <div class="loading-spinner"></div>
      <span>{{ t('common.loading') }}</span>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="error-container">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="12" cy="12" r="10" />
        <line x1="15" y1="9" x2="9" y2="15" />
        <line x1="9" y1="9" x2="15" y2="15" />
      </svg>
      <h2>{{ t('contestHistory.errorTitle') }}</h2>
      <p>{{ error }}</p>
      <button class="btn btn-primary" @click="fetchHistory()">
        {{ t('common.retry') }}
      </button>
    </div>

    <!-- Empty State -->
    <div v-else-if="filteredHistory.length === 0" class="empty-container">
      <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
        <line x1="16" y1="2" x2="16" y2="6" />
        <line x1="8" y1="2" x2="8" y2="6" />
        <line x1="3" y1="10" x2="21" y2="10" />
      </svg>
      <h2>{{ t('contestHistory.noContests') }}</h2>
      <p>{{ t('contestHistory.noContestsDesc') }}</p>
      <button class="btn btn-primary" @click="router.push('/user/contests')">
        {{ t('contestHistory.browseContests') }}
      </button>
    </div>

    <!-- Contest List -->
    <div v-else class="contest-list">
      <div
        v-for="contest in filteredHistory"
        :key="contest.contest_id"
        class="contest-card"
        @click="contest.status === 'completed' ? viewResults(contest.contest_id) : viewDetails(contest.contest_id)"
      >
        <div class="card-header">
          <div class="contest-info">
            <div class="contest-name-row">
              <span v-if="contest.final_rank === 1" class="trophy-icon">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6M18 9h1.5a2.5 2.5 0 0 0 0-5H18M4 22h16M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22M18 2H6v7a6 6 0 0 0 12 0V2Z" />
                </svg>
              </span>
              <h3 class="contest-name">{{ contest.contest_name }}</h3>
            </div>
            <span class="contest-date">{{ formatDate(contest.ends_at) }}</span>
          </div>
          <span class="status-badge" :class="getStatusClass(contest.status)">
            {{ getStatusLabel(contest.status) }}
          </span>
        </div>

        <div class="card-body">
          <div class="stat-item">
            <span class="stat-label">{{ t('contestHistory.rank') }}</span>
            <span class="stat-value">
              <span v-if="contest.final_rank" class="rank-badge" :class="getRankBadgeClass(contest.final_rank)">
                #{{ contest.final_rank }}
              </span>
              <span v-else class="rank-pending">-</span>
              <span class="rank-of">{{ t('contestHistory.of') }} {{ contest.total_participants }}</span>
            </span>
          </div>

          <div class="stat-item">
            <span class="stat-label">{{ t('contestHistory.score') }}</span>
            <span class="stat-value" :class="getPnlClass(contest.pnl_percent)">
              {{ formatPnl(contest.pnl_percent) }}
            </span>
          </div>

          <div class="stat-item">
            <span class="stat-label">{{ t('contestHistory.prize') }}</span>
            <span class="stat-value prize" :class="{ 'has-prize': (contest.final_prize_cents ?? 0) > 0 }">
              {{ formatPrize(contest.final_prize_cents) }}
            </span>
          </div>

          <div v-if="contest.trade_count !== undefined" class="stat-item">
            <span class="stat-label">{{ t('contestHistory.trades') }}</span>
            <span class="stat-value">{{ contest.trade_count }}</span>
          </div>
        </div>

        <div class="card-footer">
          <button class="view-btn" @click.stop="viewResults(contest.contest_id)">
            {{ t('contestHistory.viewDetails') }}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M9 18l6-6-6-6" />
            </svg>
          </button>
        </div>
      </div>
    </div>

    <!-- Load More -->
    <div v-if="hasMore && filteredHistory.length > 0 && !loading" class="load-more-container">
      <button class="load-more-btn" :disabled="loadingMore" @click="loadMore">
        <span v-if="loadingMore" class="loading-spinner small"></span>
        <span v-else>{{ t('contestHistory.loadMore') }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.contest-history-page {
  padding: var(--spacing-lg);
  max-width: 900px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: var(--spacing-xl);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-xs) 0;
}

.page-subtitle {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
}

.stats-section {
  margin-bottom: var(--spacing-xl);
}

/* Filter Tabs */
.filter-tabs {
  display: flex;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-lg);
  padding-bottom: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}

.filter-tab {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  background: transparent;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
  white-space: nowrap;
}

.filter-tab:hover {
  background: var(--color-bg-secondary);
  border-color: var(--color-primary);
  color: var(--color-text-primary);
}

.filter-tab.active {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: white;
}

.filter-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 10px;
  font-size: var(--font-size-xs);
  font-weight: 600;
}

.filter-tab.active .filter-count {
  background: rgba(255, 255, 255, 0.2);
}

/* Loading & Error States */
.loading-container,
.error-container,
.empty-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  text-align: center;
}

.loading-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.loading-spinner.small {
  width: 16px;
  height: 16px;
  border-width: 2px;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.error-container svg,
.empty-container svg {
  color: var(--color-text-muted);
}

.error-container h2,
.empty-container h2 {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.error-container p,
.empty-container p {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
}

.btn {
  padding: var(--spacing-sm) var(--spacing-lg);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 600;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.btn-primary {
  background: var(--color-primary);
  border: none;
  color: white;
}

.btn-primary:hover {
  background: var(--color-secondary);
}

/* Contest List */
.contest-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.contest-card {
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  overflow: hidden;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.contest-card:hover {
  border-color: var(--color-primary);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: var(--spacing-md) var(--spacing-lg);
  background: var(--color-bg-secondary);
  border-bottom: 1px solid var(--color-border);
}

.contest-info {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.contest-name-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.trophy-icon {
  color: #FFD700;
}

.contest-name {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.contest-date {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.status-badge {
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
}

.status-completed {
  background: rgba(16, 185, 129, 0.1);
  color: #10B981;
}

.status-live {
  background: rgba(239, 68, 68, 0.1);
  color: #EF4444;
}

.status-cancelled {
  background: rgba(107, 114, 128, 0.1);
  color: #6B7280;
}

.card-body {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
}

.stat-item {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.stat-label {
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.stat-value {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
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

.stat-value.prize.has-prize {
  color: #FFD700;
}

.rank-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  font-weight: 700;
  background: var(--color-bg-tertiary);
}

.rank-badge.rank-gold {
  background: linear-gradient(135deg, #FFD700 0%, #FFA500 100%);
  color: #0f172a;
}

.rank-badge.rank-silver {
  background: linear-gradient(135deg, #E8E8E8 0%, #C0C0C0 100%);
  color: #0f172a;
}

.rank-badge.rank-bronze {
  background: linear-gradient(135deg, #CD7F32 0%, #B87333 100%);
  color: white;
}

.rank-pending {
  color: var(--color-text-muted);
}

.rank-of {
  font-size: var(--font-size-xs);
  font-weight: 400;
  color: var(--color-text-secondary);
}

.card-footer {
  display: flex;
  justify-content: flex-end;
  padding: var(--spacing-sm) var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

.view-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: transparent;
  border: none;
  color: var(--color-primary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.view-btn:hover {
  color: var(--color-secondary);
}

/* Load More */
.load-more-container {
  display: flex;
  justify-content: center;
  padding: var(--spacing-lg);
}

.load-more-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-xl);
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.load-more-btn:hover:not(:disabled) {
  background: var(--color-bg-tertiary);
  border-color: var(--color-primary);
}

.load-more-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

/* RTL Support */
[dir="rtl"] .contest-name-row {
  flex-direction: row-reverse;
}

[dir="rtl"] .view-btn {
  flex-direction: row-reverse;
}

[dir="rtl"] .view-btn svg {
  transform: rotate(180deg);
}

/* Mobile */
@media (max-width: 767px) {
  .contest-history-page {
    padding: var(--spacing-md);
  }

  .page-title {
    font-size: var(--font-size-xl);
  }

  .filter-tabs {
    gap: var(--spacing-xs);
  }

  .filter-tab {
    padding: var(--spacing-xs) var(--spacing-sm);
    font-size: var(--font-size-xs);
  }

  .card-header {
    padding: var(--spacing-sm) var(--spacing-md);
    flex-direction: column;
    gap: var(--spacing-sm);
  }

  .card-body {
    grid-template-columns: repeat(2, 1fr);
    padding: var(--spacing-md);
    gap: var(--spacing-sm);
  }

  .card-footer {
    padding: var(--spacing-sm) var(--spacing-md);
  }
}
</style>
