<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue';
import { t } from '@/i18n';
import { userStatsApi, type GlobalLeaderboardEntry, type UserStats, type ScoreHistoryEntry } from '@/api';
import TraggePointBadge from '@/components/tragge/TraggePointBadge.vue';
import ScoreHistoryBreakdown from '@/components/tragge/ScoreHistoryBreakdown.vue';

// State
const entries = ref<GlobalLeaderboardEntry[]>([]);
const userStats = ref<UserStats | null>(null);
const userRank = ref<number | null>(null);
const userScore = ref<number | null>(null);
const scoreHistory = ref<ScoreHistoryEntry[]>([]);
const loading = ref(true);
const historyLoading = ref(true);
const error = ref<string | null>(null);
const currentPage = ref(0);
const pageSize = 50;
const activeTab = ref<'leaderboard' | 'breakdown'>('leaderboard');

// Computed
const hasNextPage = computed(() => entries.value.length === pageSize);
const hasPrevPage = computed(() => currentPage.value > 0);

const isUserInCurrentPage = computed(() => {
  if (!userRank.value) return false;
  const startRank = currentPage.value * pageSize + 1;
  const endRank = startRank + pageSize - 1;
  return userRank.value >= startRank && userRank.value <= endRank;
});

const bestFinish = computed(() => {
  if (!scoreHistory.value.length) return null;
  return Math.min(...scoreHistory.value.map(e => e.rank));
});

// Methods
async function loadLeaderboard() {
  loading.value = true;
  error.value = null;

  try {
    const response = await userStatsApi.getGlobalLeaderboard({
      limit: pageSize,
      offset: currentPage.value * pageSize,
    });
    entries.value = response.entries;
    userRank.value = response.user_rank || null;
    userScore.value = response.user_score || null;
  } catch (err: any) {
    error.value = err.response?.data?.error || t('tragge.loadError');
  } finally {
    loading.value = false;
  }
}

async function loadUserStats() {
  try {
    userStats.value = await userStatsApi.getMyStats();
  } catch {
    // Silently fail - user stats are supplementary
  }
}

async function loadScoreHistory() {
  historyLoading.value = true;
  try {
    const response = await userStatsApi.getMyScoreHistory({ limit: 50 });
    scoreHistory.value = response.entries;
  } catch {
    // Silently fail
  } finally {
    historyLoading.value = false;
  }
}

function nextPage() {
  if (hasNextPage.value) {
    currentPage.value++;
    loadLeaderboard();
  }
}

function prevPage() {
  if (hasPrevPage.value) {
    currentPage.value--;
    loadLeaderboard();
  }
}

function jumpToMyRank() {
  if (!userRank.value) return;
  const targetPage = Math.floor((userRank.value - 1) / pageSize);
  if (targetPage !== currentPage.value) {
    currentPage.value = targetPage;
    loadLeaderboard();
  }
}

function formatScore(score: number): string {
  return score.toLocaleString(undefined, { maximumFractionDigits: 0 });
}

function getRankDisplay(rank: number): { icon: string; class: string } {
  if (rank === 1) return { icon: '👑', class: 'rank-gold' };
  if (rank === 2) return { icon: '🥈', class: 'rank-silver' };
  if (rank === 3) return { icon: '🥉', class: 'rank-bronze' };
  return { icon: '', class: '' };
}

// Watch tab changes
watch(activeTab, (tab) => {
  if (tab === 'breakdown' && !scoreHistory.value.length && !historyLoading.value) {
    loadScoreHistory();
  }
});

onMounted(() => {
  loadLeaderboard();
  loadUserStats();
  loadScoreHistory();
});
</script>

<template>
  <div class="global-leaderboard-page">
    <!-- Page Header -->
    <div class="page-header">
      <div class="header-icon">🌍</div>
      <div class="header-content">
        <h1 class="page-title">{{ t('tragge.globalLeaderboard') }}</h1>
        <p class="page-description">{{ t('tragge.globalDescription') }}</p>
      </div>
    </div>

    <!-- User Stats Card -->
    <div v-if="userStats" class="user-stats-card card">
      <div class="user-stats-grid">
        <div class="stat-item stat-score">
          <span class="stat-label">{{ t('tragge.traggePoint') }}</span>
          <span class="stat-value primary">{{ formatScore(userStats.tragge_point) }}</span>
          <TraggePointBadge :score="userStats.tragge_point" size="sm" :show-label="false" />
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('tragge.globalRank') }}</span>
          <span class="stat-value">
            <template v-if="userRank">#{{ userRank.toLocaleString() }}</template>
            <template v-else>-</template>
          </span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('tragge.contestsParticipated') }}</span>
          <span class="stat-value">{{ userStats.total_contests }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('tragge.wins') }}</span>
          <span class="stat-value">{{ userStats.total_wins }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('tragge.bestFinish') }}</span>
          <span class="stat-value">
            <template v-if="bestFinish">#{{ bestFinish }}</template>
            <template v-else>-</template>
          </span>
        </div>
      </div>
    </div>

    <!-- Tab Navigation -->
    <div class="tabs">
      <button
        :class="['tab', { active: activeTab === 'leaderboard' }]"
        @click="activeTab = 'leaderboard'"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 20V10M18 20V4M6 20v-4" />
        </svg>
        {{ t('tragge.leaderboard') }}
      </button>
      <button
        :class="['tab', { active: activeTab === 'breakdown' }]"
        @click="activeTab = 'breakdown'"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 6v6l4 2" />
        </svg>
        {{ t('tragge.pointBreakdownTab') }}
      </button>
    </div>

    <!-- Leaderboard Tab -->
    <div v-if="activeTab === 'leaderboard'" class="card leaderboard-card">
      <!-- Loading State -->
      <div v-if="loading" class="loading-state">
        <div class="spinner"></div>
        <p>{{ t('common.loading') }}</p>
      </div>

      <!-- Error State -->
      <div v-else-if="error" class="error-state">
        <p>{{ error }}</p>
        <button class="btn btn-primary" @click="loadLeaderboard">{{ t('common.retry') }}</button>
      </div>

      <!-- Leaderboard Content -->
      <div v-else-if="entries.length > 0">
        <!-- Table -->
        <div class="table-container">
          <table class="leaderboard-table">
            <thead>
              <tr>
                <th class="col-rank">#</th>
                <th class="col-trader">{{ t('tragge.trader') }}</th>
                <th class="col-score">{{ t('tragge.point') }}</th>
                <th class="col-contests">{{ t('tragge.contests') }}</th>
                <th class="col-wins">{{ t('tragge.winsColumn') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="entry in entries"
                :key="entry.user_id"
                :class="[
                  'leaderboard-row',
                  getRankDisplay(entry.rank).class,
                  { 'is-current-user': entry.user_id === userStats?.user_id }
                ]"
              >
                <td class="col-rank">
                  <div class="rank-cell">
                    <span v-if="getRankDisplay(entry.rank).icon" class="rank-icon">
                      {{ getRankDisplay(entry.rank).icon }}
                    </span>
                    <span class="rank-number">{{ entry.rank }}</span>
                  </div>
                </td>
                <td class="col-trader">
                  <div class="trader-cell">
                    <span class="trader-name">
                      {{ entry.username || entry.user_id.slice(0, 8) + '...' }}
                    </span>
                    <TraggePointBadge
                      :score="entry.tragge_point"
                      size="sm"
                      :show-label="false"
                    />
                  </div>
                </td>
                <td class="col-score">
                  <span class="score-value">{{ formatScore(entry.tragge_point) }}</span>
                </td>
                <td class="col-contests">{{ entry.total_contests }}</td>
                <td class="col-wins">{{ entry.total_wins }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- User's Rank (if not on current page) -->
        <div v-if="userRank && !isUserInCurrentPage && userStats" class="your-rank-section">
          <div class="your-rank-divider">
            <span>...</span>
          </div>
          <table class="leaderboard-table your-rank-table">
            <tbody>
              <tr class="leaderboard-row is-current-user">
                <td class="col-rank">
                  <div class="rank-cell">
                    <span class="rank-icon">⭐</span>
                    <span class="rank-number">{{ userRank }}</span>
                  </div>
                </td>
                <td class="col-trader">
                  <div class="trader-cell">
                    <span class="trader-name you-label">{{ t('tragge.you') }}</span>
                    <TraggePointBadge
                      :score="userStats.tragge_point"
                      size="sm"
                      :show-label="false"
                    />
                  </div>
                </td>
                <td class="col-score">
                  <span class="score-value">{{ formatScore(userStats.tragge_point) }}</span>
                </td>
                <td class="col-contests">{{ userStats.total_contests }}</td>
                <td class="col-wins">{{ userStats.total_wins }}</td>
              </tr>
            </tbody>
          </table>
          <button class="btn btn-secondary btn-sm jump-btn" @click="jumpToMyRank">
            {{ t('tragge.jumpToMyRank') }}
          </button>
        </div>

        <!-- Pagination -->
        <div class="pagination">
          <button
            class="btn btn-secondary"
            :disabled="!hasPrevPage"
            @click="prevPage"
          >
            {{ t('common.previous') }}
          </button>
          <span class="page-info">
            {{ t('tragge.pageInfo', { page: currentPage + 1 }) }}
          </span>
          <button
            class="btn btn-secondary"
            :disabled="!hasNextPage"
            @click="nextPage"
          >
            {{ t('common.next') }}
          </button>
        </div>
      </div>

      <!-- Empty State -->
      <div v-else class="empty-state">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M12 20V10M18 20V4M6 20v-4" />
        </svg>
        <p>{{ t('tragge.noData') }}</p>
      </div>
    </div>

    <!-- Score Breakdown Tab -->
    <div v-else class="breakdown-tab">
      <ScoreHistoryBreakdown
        :entries="scoreHistory"
        :loading="historyLoading"
        :show-limit="15"
      />
    </div>

    <!-- Badge Legend -->
    <div class="badge-legend card">
      <h3 class="legend-title">{{ t('tragge.badgeLegend') }}</h3>
      <div class="legend-grid">
        <div class="legend-item">
          <span class="legend-icon">🥉</span>
          <div class="legend-info">
            <span class="legend-name">{{ t('tragge.badge.bronze') }}</span>
            <span class="legend-requirement">1,000+ {{ t('tragge.point') }}</span>
          </div>
        </div>
        <div class="legend-item">
          <span class="legend-icon">🥈</span>
          <div class="legend-info">
            <span class="legend-name">{{ t('tragge.badge.silver') }}</span>
            <span class="legend-requirement">5,000+ {{ t('tragge.point') }}</span>
          </div>
        </div>
        <div class="legend-item">
          <span class="legend-icon">🥇</span>
          <div class="legend-info">
            <span class="legend-name">{{ t('tragge.badge.gold') }}</span>
            <span class="legend-requirement">10,000+ {{ t('tragge.point') }}</span>
          </div>
        </div>
        <div class="legend-item">
          <span class="legend-icon">💎</span>
          <div class="legend-info">
            <span class="legend-name">{{ t('tragge.badge.diamond') }}</span>
            <span class="legend-requirement">50,000+ {{ t('tragge.point') }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.global-leaderboard-page {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

/* Page Header */
.page-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-lg);
}

.header-icon {
  font-size: 48px;
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  margin-bottom: var(--spacing-xs);
}

.page-description {
  font-size: var(--font-size-md);
  color: var(--color-text-secondary);
  margin: 0;
}

/* User Stats Card */
.user-stats-card {
  background: linear-gradient(135deg, var(--color-primary) 0%, #7C3AED 100%);
  color: white;
}

.user-stats-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: var(--spacing-lg);
}

.stat-item {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.stat-item.stat-score {
  position: relative;
}

.stat-label {
  font-size: var(--font-size-xs);
  opacity: 0.85;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.stat-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
}

.stat-value.primary {
  font-size: var(--font-size-2xl);
}

/* Tabs */
.tabs {
  display: flex;
  gap: var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
  padding-bottom: var(--spacing-sm);
}

.tab {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border: none;
  background: none;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  border-radius: var(--radius-md);
  transition: all var(--transition-fast);
}

.tab:hover {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

.tab.active {
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

/* Leaderboard Card */
.leaderboard-card {
  overflow: hidden;
}

.loading-state,
.error-state,
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
  width: 40px;
  height: 40px;
  border: 4px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Table */
.table-container {
  overflow-x: auto;
}

.leaderboard-table {
  width: 100%;
  border-collapse: collapse;
}

.leaderboard-table thead {
  background-color: var(--color-bg-secondary);
}

.leaderboard-table th {
  padding: var(--spacing-md);
  text-align: left;
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  border-bottom: 2px solid var(--color-border);
}

[dir="rtl"] .leaderboard-table th {
  text-align: right;
}

.leaderboard-table td {
  padding: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
}

.leaderboard-row:hover {
  background-color: var(--color-bg-secondary);
}

.leaderboard-row.rank-gold {
  background-color: rgba(251, 191, 36, 0.1);
}

.leaderboard-row.rank-silver {
  background-color: rgba(156, 163, 175, 0.1);
}

.leaderboard-row.rank-bronze {
  background-color: rgba(180, 83, 9, 0.1);
}

.leaderboard-row.is-current-user {
  background-color: rgba(59, 130, 246, 0.15);
  border-left: 3px solid var(--color-primary);
}

[dir="rtl"] .leaderboard-row.is-current-user {
  border-left: none;
  border-right: 3px solid var(--color-primary);
}

/* Column widths */
.col-rank { width: 80px; }
.col-trader { min-width: 200px; }
.col-score { width: 120px; }
.col-contests, .col-wins { width: 100px; text-align: center; }

/* Rank Cell */
.rank-cell {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.rank-icon {
  font-size: var(--font-size-lg);
}

.rank-number {
  font-weight: 600;
  color: var(--color-text-secondary);
}

/* Trader Cell */
.trader-cell {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.trader-name {
  font-weight: 500;
}

.you-label {
  color: var(--color-primary);
  font-weight: 600;
}

/* Score Value */
.score-value {
  font-weight: 600;
  color: var(--color-primary);
  font-family: var(--font-mono, monospace);
}

/* Your Rank Section */
.your-rank-section {
  border-top: 1px solid var(--color-border);
  padding-top: var(--spacing-md);
}

.your-rank-divider {
  text-align: center;
  color: var(--color-text-tertiary);
  margin-bottom: var(--spacing-sm);
}

.your-rank-table {
  margin-bottom: var(--spacing-md);
}

.jump-btn {
  display: block;
  margin: 0 auto;
}

/* Pagination */
.pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

.page-info {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Badge Legend */
.badge-legend {
  background-color: var(--color-bg-secondary);
}

.legend-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  margin-bottom: var(--spacing-md);
}

.legend-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-md);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm);
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-md);
}

.legend-icon {
  font-size: var(--font-size-xl);
}

.legend-info {
  display: flex;
  flex-direction: column;
}

.legend-name {
  font-weight: 600;
  font-size: var(--font-size-sm);
}

.legend-requirement {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

/* Mobile Responsive */
@media (max-width: 1023px) {
  .user-stats-grid {
    grid-template-columns: repeat(3, 1fr);
  }

  .legend-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 767px) {
  .page-header {
    flex-direction: column;
    text-align: center;
  }

  .page-title {
    font-size: var(--font-size-xl);
  }

  .user-stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .tabs {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }

  .leaderboard-table {
    font-size: var(--font-size-sm);
  }

  .leaderboard-table th,
  .leaderboard-table td {
    padding: var(--spacing-sm);
  }

  .col-contests, .col-wins {
    display: none;
  }

  .legend-grid {
    grid-template-columns: 1fr;
  }

  .rank-icon {
    font-size: var(--font-size-md);
  }
}
</style>
