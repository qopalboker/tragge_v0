<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import { t } from '@/i18n';
import { useContestsStore } from '@/stores/contests';
import { useAuthStore } from '@/stores/auth';
import { useToast } from '@/composables/useToast';
import { usePrizeDistribution } from '@/composables/usePrizeDistribution';
import PrizeInfoPanel from '@/components/prize/PrizeInfoPanel.vue';
import PrizeZoneIndicator from '@/components/prize/PrizeZoneIndicator.vue';

const contestsStore = useContestsStore();
const authStore = useAuthStore();
const toast = useToast();

// Auto-refresh state
const AUTO_REFRESH_INTERVAL = 5000;
const isRefreshing = ref(false);
const lastUpdatedAt = ref<Date | null>(null);
const secondsAgo = ref(0);
let refreshIntervalId: ReturnType<typeof setInterval> | null = null;
let secondsIntervalId: ReturnType<typeof setInterval> | null = null;

const selectedContestId = ref<string>('');
const showPrizePanel = ref(true);

// Computed
const currentUserId = computed(() => authStore.user?.id || '');

// Selected contest
const selectedContest = computed(() => {
  if (!selectedContestId.value) return null;
  return contestsStore.contests.find(c => c.id === selectedContestId.value) || null;
});

// Prize distribution
const entryFeeCents = computed(() => selectedContest.value?.entry_fee_cents || 0);
const participantCount = computed(() => contestsStore.leaderboard.length || 0);

const {
  prizeZoneCutoff,
  isRankInPrizeZone,
} = usePrizeDistribution({
  entryFeeCents,
  participantCount,
});

const userRank = computed(() => {
  if (!currentUserId.value) return null;
  const entry = contestsStore.leaderboard.find(e => e.user_id === currentUserId.value);
  return entry?.rank || null;
});

const userScore = computed(() => {
  if (!currentUserId.value) return null;
  const entry = contestsStore.leaderboard.find(e => e.user_id === currentUserId.value);
  return entry?.total_score || null;
});

const isUserInTop100 = computed(() => userRank.value !== null && userRank.value <= 100);

// Get user's history entry for the current contest to show their rank if not in top 100
const userHistoryEntry = computed(() => {
  if (!selectedContestId.value) return null;
  return contestsStore.userHistory.find(h => h.contest_id === selectedContestId.value);
});

function formatScore(score: number): string {
  if (score >= 0) {
    return `+$${score.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
  }
  return `-$${Math.abs(score).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function formatUserId(userId: string): string {
  // Show abbreviated user ID for privacy
  if (userId.length <= 8) return userId;
  return `${userId.slice(0, 4)}...${userId.slice(-4)}`;
}

function isCurrentUser(userId: string): boolean {
  return userId === currentUserId.value;
}

function getRankClass(rank: number): string {
  if (rank === 1) return 'rank-gold';
  if (rank === 2) return 'rank-silver';
  if (rank === 3) return 'rank-bronze';
  return '';
}

function isPrizeZoneCutoff(rank: number): boolean {
  return rank === prizeZoneCutoff.value && participantCount.value > 0;
}

function getEntryPrizeClass(rank: number): string {
  if (participantCount.value === 0) return '';
  if (isRankInPrizeZone(rank)) return 'in-prize-zone';
  return 'out-prize-zone';
}

async function loadData(): Promise<void> {
  try {
    // Load contests first
    if (contestsStore.contests.length === 0) {
      await contestsStore.fetchContests();
    }

    // Load user history
    await contestsStore.fetchUserHistory();

    // Select the first contest if none selected
    if (!selectedContestId.value && contestsStore.contests.length > 0) {
      // Prefer running contests, then registration_open
      const runningContest = contestsStore.contests.find(c => c.status === 'running');
      const openContest = contestsStore.contests.find(c => c.status === 'registration_open');
      selectedContestId.value = runningContest?.id || openContest?.id || contestsStore.contests[0].id;
    }
  } catch {
    toast.error(t('leaderboard.loadError'));
  }
}

async function loadLeaderboard(): Promise<void> {
  if (!selectedContestId.value) return;

  try {
    await contestsStore.fetchLeaderboard(selectedContestId.value);
    lastUpdatedAt.value = new Date();
    secondsAgo.value = 0;
  } catch {
    toast.error(t('leaderboard.loadError'));
  }
}

// Auto-refresh logic
const isContestLive = computed(() => {
  return selectedContest.value?.status === 'running';
});

async function refreshLeaderboard(): Promise<void> {
  if (!selectedContestId.value || isRefreshing.value) return;

  isRefreshing.value = true;
  try {
    await contestsStore.fetchLeaderboard(selectedContestId.value, { silent: true });
    lastUpdatedAt.value = new Date();
    secondsAgo.value = 0;
  } catch {
    // Silent fail for auto-refresh; user can manually retry
  } finally {
    isRefreshing.value = false;
  }
}

function startAutoRefresh(): void {
  stopAutoRefresh();
  refreshIntervalId = setInterval(() => {
    if (!document.hidden) {
      refreshLeaderboard();
    }
  }, AUTO_REFRESH_INTERVAL);
}

function stopAutoRefresh(): void {
  if (refreshIntervalId !== null) {
    clearInterval(refreshIntervalId);
    refreshIntervalId = null;
  }
}

function startSecondsCounter(): void {
  stopSecondsCounter();
  secondsIntervalId = setInterval(() => {
    if (lastUpdatedAt.value) {
      secondsAgo.value = Math.floor((Date.now() - lastUpdatedAt.value.getTime()) / 1000);
    }
  }, 1000);
}

function stopSecondsCounter(): void {
  if (secondsIntervalId !== null) {
    clearInterval(secondsIntervalId);
    secondsIntervalId = null;
  }
}

function handleVisibilityChange(): void {
  if (document.hidden) {
    stopAutoRefresh();
  } else if (isContestLive.value) {
    // Refresh immediately when tab becomes visible again, then restart interval
    refreshLeaderboard();
    startAutoRefresh();
  }
}

// Watch for contest selection changes
watch(selectedContestId, (newId) => {
  if (newId) {
    loadLeaderboard();
  }
});

// Watch live status to start/stop auto-refresh
watch(isContestLive, (live) => {
  if (live) {
    startAutoRefresh();
  } else {
    stopAutoRefresh();
  }
});

onMounted(() => {
  loadData();
  startSecondsCounter();
  document.addEventListener('visibilitychange', handleVisibilityChange);
});

onUnmounted(() => {
  stopAutoRefresh();
  stopSecondsCounter();
  document.removeEventListener('visibilitychange', handleVisibilityChange);
});
</script>

<template>
  <div class="leaderboard-page">
    <!-- Header -->
    <div class="page-header">
      <h1 class="page-title">{{ t('leaderboard.title') }}</h1>

      <!-- Contest Selector -->
      <div class="contest-selector">
        <label class="selector-label">{{ t('leaderboard.selectContest') }}</label>
        <select
          v-model="selectedContestId"
          class="contest-select"
          :disabled="contestsStore.loading"
        >
          <option value="" disabled>{{ t('leaderboard.chooseContest') }}</option>
          <option
            v-for="contest in contestsStore.contests"
            :key="contest.id"
            :value="contest.id"
          >
            {{ contest.name }} ({{ contest.status }})
          </option>
        </select>
      </div>
    </div>

    <!-- Auto-refresh Status Bar -->
    <div v-if="selectedContestId && !contestsStore.loading && !contestsStore.leaderboardLoading" class="refresh-bar">
      <div class="refresh-bar-left">
        <span v-if="isContestLive" class="live-indicator">
          {{ t('leaderboard.liveIndicator') }}
        </span>
        <span v-if="lastUpdatedAt" class="last-updated">
          {{ t('leaderboard.lastUpdated', { seconds: secondsAgo }) }}
        </span>
      </div>
      <button
        class="refresh-btn"
        :disabled="isRefreshing"
        @click="refreshLeaderboard"
        :title="t('leaderboard.refresh')"
      >
        <svg
          :class="['refresh-icon', { 'refresh-spinning': isRefreshing }]"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
        >
          <path d="M21 2v6h-6" />
          <path d="M3 12a9 9 0 0 1 15-6.7L21 8" />
          <path d="M3 22v-6h6" />
          <path d="M21 12a9 9 0 0 1-15 6.7L3 16" />
        </svg>
        <span>{{ t('leaderboard.refresh') }}</span>
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="contestsStore.loading || contestsStore.leaderboardLoading" class="loading-state">
      <div class="loading-spinner"></div>
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Error State -->
    <div v-else-if="contestsStore.leaderboardError" class="error-state card">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="12" cy="12" r="10" />
        <path d="M12 8v4" />
        <path d="M12 16h.01" />
      </svg>
      <p>{{ contestsStore.leaderboardError }}</p>
      <button class="btn btn-primary" @click="loadLeaderboard">
        {{ t('common.retry') }}
      </button>
    </div>

    <!-- No Contest Selected -->
    <div v-else-if="!selectedContestId" class="empty-state card">
      <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6" />
        <path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18" />
        <path d="M4 22h16" />
        <path d="M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22" />
        <path d="M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22" />
        <path d="M18 2H6v7a6 6 0 0 0 12 0V2Z" />
      </svg>
      <h2>{{ t('leaderboard.selectToView') }}</h2>
      <p>{{ t('leaderboard.selectContestPrompt') }}</p>
    </div>

    <!-- Leaderboard Content -->
    <div v-else class="leaderboard-content">
      <!-- Prize Info Panel -->
      <div v-if="showPrizePanel && entryFeeCents > 0 && participantCount > 0" class="prize-panel-container">
        <PrizeInfoPanel
          :entry-fee-cents="entryFeeCents"
          :participant-count="participantCount"
          :user-rank="userRank"
          :user-score="userScore"
          :show-user-status="!!currentUserId"
          :show-breakdown="true"
          :compact="false"
        />
      </div>

      <!-- Current User Rank Card (if not in top 100) -->
      <div v-if="currentUserId && !isUserInTop100 && userHistoryEntry" class="user-rank-card card">
        <div class="your-rank-label">{{ t('leaderboard.yourRank') }}</div>
        <div class="your-rank-row">
          <span class="your-rank-position">{{ userHistoryEntry.final_rank || '-' }}</span>
          <span class="your-rank-user">{{ t('leaderboard.you') }}</span>
          <span :class="['your-rank-score', { 'score-positive': userHistoryEntry.total_score >= 0, 'score-negative': userHistoryEntry.total_score < 0 }]">
            {{ formatScore(userHistoryEntry.total_score) }}
          </span>
        </div>
      </div>

      <!-- Top 3 Podium -->
      <div v-if="contestsStore.leaderboard.length >= 3" class="podium">
        <div class="podium-item podium-second">
          <div class="podium-avatar">
            <span class="podium-rank">2</span>
          </div>
          <div class="podium-user">{{ formatUserId(contestsStore.leaderboard[1].user_id) }}</div>
          <div :class="['podium-score', { 'score-positive': contestsStore.leaderboard[1].total_score >= 0 }]">
            {{ formatScore(contestsStore.leaderboard[1].total_score) }}
          </div>
          <div class="podium-stand podium-stand-silver"></div>
        </div>
        <div class="podium-item podium-first">
          <div class="podium-crown">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 2L15 8L22 9L17 14L18 21L12 18L6 21L7 14L2 9L9 8L12 2Z" />
            </svg>
          </div>
          <div class="podium-avatar podium-avatar-gold">
            <span class="podium-rank">1</span>
          </div>
          <div class="podium-user">{{ formatUserId(contestsStore.leaderboard[0].user_id) }}</div>
          <div :class="['podium-score', { 'score-positive': contestsStore.leaderboard[0].total_score >= 0 }]">
            {{ formatScore(contestsStore.leaderboard[0].total_score) }}
          </div>
          <div class="podium-stand podium-stand-gold"></div>
        </div>
        <div class="podium-item podium-third">
          <div class="podium-avatar">
            <span class="podium-rank">3</span>
          </div>
          <div class="podium-user">{{ formatUserId(contestsStore.leaderboard[2].user_id) }}</div>
          <div :class="['podium-score', { 'score-positive': contestsStore.leaderboard[2].total_score >= 0 }]">
            {{ formatScore(contestsStore.leaderboard[2].total_score) }}
          </div>
          <div class="podium-stand podium-stand-bronze"></div>
        </div>
      </div>

      <!-- Leaderboard Table -->
      <div class="leaderboard-table-container card">
        <table class="leaderboard-table">
          <thead>
            <tr>
              <th class="col-rank">{{ t('leaderboard.rank') }}</th>
              <th class="col-player">{{ t('leaderboard.player') }}</th>
              <th class="col-score">{{ t('leaderboard.score') }}</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="entry in contestsStore.leaderboard" :key="entry.user_id">
              <!-- Prize Zone Cutoff Indicator -->
              <tr v-if="isPrizeZoneCutoff(entry.rank)" class="prize-zone-row">
                <td colspan="3">
                  <PrizeZoneIndicator
                    :entry-fee-cents="entryFeeCents"
                    :participant-count="participantCount"
                    :current-view-rank="entry.rank"
                    variant="line"
                  />
                </td>
              </tr>
              <!-- Leaderboard Entry -->
              <tr
                :class="['leaderboard-row', { 'current-user': isCurrentUser(entry.user_id) }, getEntryPrizeClass(entry.rank)]"
              >
                <td class="col-rank">
                  <span :class="['rank-badge', getRankClass(entry.rank)]">
                    {{ entry.rank }}
                  </span>
                  <PrizeZoneIndicator
                    v-if="entryFeeCents > 0 && participantCount > 0"
                    :entry-fee-cents="entryFeeCents"
                    :participant-count="participantCount"
                    :current-view-rank="entry.rank"
                    variant="badge"
                    :show-label="false"
                  />
                </td>
                <td class="col-player">
                  <span class="player-name">
                    {{ formatUserId(entry.user_id) }}
                    <span v-if="isCurrentUser(entry.user_id)" class="you-badge">{{ t('leaderboard.you') }}</span>
                  </span>
                </td>
                <td class="col-score">
                  <span :class="['score', { 'score-positive': entry.total_score >= 0, 'score-negative': entry.total_score < 0 }]">
                    {{ formatScore(entry.total_score) }}
                  </span>
                </td>
              </tr>
            </template>
          </tbody>
        </table>

        <!-- Empty table state -->
        <div v-if="contestsStore.leaderboard.length === 0" class="table-empty">
          <p>{{ t('leaderboard.noEntries') }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.leaderboard-page {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
  max-width: 800px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

.contest-selector {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.selector-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
}

.contest-select {
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: var(--font-size-sm);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-primary);
  cursor: pointer;
  max-width: 400px;
}

.contest-select:focus {
  outline: none;
  border-color: var(--color-primary);
}

/* Refresh Bar */
.refresh-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.refresh-bar-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.live-indicator {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: #DC2626;
  white-space: nowrap;
}

.last-updated {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  white-space: nowrap;
}

.refresh-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
  white-space: nowrap;
}

.refresh-btn:hover:not(:disabled) {
  color: var(--color-primary);
  border-color: var(--color-primary);
}

.refresh-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.refresh-icon {
  flex-shrink: 0;
}

.refresh-spinning {
  animation: spin 0.8s linear infinite;
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.loading-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.error-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  text-align: center;
}

.error-state svg {
  color: var(--color-danger);
}

.empty-state svg {
  color: var(--color-text-muted);
}

.empty-state h2 {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
}

.empty-state p,
.error-state p {
  color: var(--color-text-secondary);
}

.leaderboard-content {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

/* User Rank Card */
.user-rank-card {
  padding: var(--spacing-md);
  background: linear-gradient(135deg, var(--color-primary), var(--color-secondary));
  color: white;
}

.your-rank-label {
  font-size: var(--font-size-sm);
  opacity: 0.9;
  margin-bottom: var(--spacing-sm);
}

.your-rank-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.your-rank-position {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  min-width: 48px;
}

.your-rank-user {
  flex: 1;
  font-weight: 500;
}

.your-rank-score {
  font-size: var(--font-size-lg);
  font-weight: 600;
}

/* Podium */
.podium {
  display: flex;
  align-items: flex-end;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  padding-top: var(--spacing-2xl);
}

.podium-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
  position: relative;
}

.podium-crown {
  position: absolute;
  top: -24px;
  color: #F59E0B;
}

.podium-avatar {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-full);
  font-weight: 600;
  font-size: var(--font-size-lg);
}

.podium-avatar-gold {
  background-color: #F59E0B;
  color: white;
  width: 56px;
  height: 56px;
}

.podium-user {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.podium-score {
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.podium-stand {
  width: 80px;
  border-radius: var(--radius-md) var(--radius-md) 0 0;
  margin-top: var(--spacing-sm);
}

.podium-stand-gold {
  height: 80px;
  background: linear-gradient(180deg, #F59E0B 0%, #D97706 100%);
}

.podium-stand-silver {
  height: 60px;
  background: linear-gradient(180deg, #9CA3AF 0%, #6B7280 100%);
}

.podium-stand-bronze {
  height: 40px;
  background: linear-gradient(180deg, #D97706 0%, #B45309 100%);
}

.podium-first {
  order: 2;
}

.podium-second {
  order: 1;
}

.podium-third {
  order: 3;
}

/* Leaderboard Table */
.leaderboard-table-container {
  padding: 0;
  overflow: hidden;
}

.leaderboard-table {
  width: 100%;
  border-collapse: collapse;
}

.leaderboard-table th {
  padding: var(--spacing-md);
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-align: left;
  background-color: var(--color-bg-secondary);
  border-bottom: 1px solid var(--color-border);
}

[dir="rtl"] .leaderboard-table th {
  text-align: right;
}

.leaderboard-table td {
  padding: var(--spacing-md);
  font-size: var(--font-size-sm);
  border-bottom: 1px solid var(--color-border);
}

.leaderboard-row:last-child td {
  border-bottom: none;
}

.leaderboard-row:hover {
  background-color: var(--color-bg-secondary);
}

.leaderboard-row.current-user {
  background-color: #EFF6FF;
}

.leaderboard-row.current-user:hover {
  background-color: #DBEAFE;
}

.col-rank {
  width: 80px;
}

.col-score {
  width: 140px;
  text-align: right;
}

[dir="rtl"] .col-score {
  text-align: left;
}

.rank-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  height: 32px;
  padding: 0 var(--spacing-xs);
  font-weight: 600;
  border-radius: var(--radius-md);
  background-color: var(--color-bg-tertiary);
}

.rank-gold {
  background-color: #FEF3C7;
  color: #D97706;
}

.rank-silver {
  background-color: #F3F4F6;
  color: #6B7280;
}

.rank-bronze {
  background-color: #FED7AA;
  color: #C2410C;
}

.player-name {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.you-badge {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 600;
  background-color: var(--color-primary);
  color: white;
  border-radius: var(--radius-md);
}

.score {
  font-weight: 600;
  font-family: var(--font-mono, monospace);
}

.score-positive {
  color: #059669;
}

.score-negative {
  color: #DC2626;
}

.table-empty {
  padding: var(--spacing-2xl);
  text-align: center;
  color: var(--color-text-secondary);
}

/* Prize Panel Container */
.prize-panel-container {
  margin-bottom: var(--spacing-md);
}

/* Prize Zone Row */
.prize-zone-row td {
  padding: 0;
  background: transparent;
  border: none;
}

/* Prize Zone Styles */
.leaderboard-row.in-prize-zone {
  background-color: rgba(16, 185, 129, 0.03);
}

.leaderboard-row.in-prize-zone:hover {
  background-color: rgba(16, 185, 129, 0.08);
}

.leaderboard-row.out-prize-zone {
  opacity: 0.7;
}

.leaderboard-row.out-prize-zone:hover {
  opacity: 1;
}

/* Rank column with badge */
.col-rank {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

@media (max-width: 767px) {
  .page-header {
    gap: var(--spacing-sm);
  }

  .page-title {
    font-size: var(--font-size-xl);
  }

  .contest-select {
    max-width: none;
    width: 100%;
  }

  .podium {
    padding: var(--spacing-md);
    gap: var(--spacing-sm);
  }

  .podium-stand {
    width: 60px;
  }

  .podium-stand-gold {
    height: 60px;
  }

  .podium-stand-silver {
    height: 45px;
  }

  .podium-stand-bronze {
    height: 30px;
  }

  .podium-avatar {
    width: 40px;
    height: 40px;
    font-size: var(--font-size-md);
  }

  .podium-avatar-gold {
    width: 48px;
    height: 48px;
  }

  .leaderboard-table th,
  .leaderboard-table td {
    padding: var(--spacing-sm);
  }

  .col-rank {
    width: 60px;
  }

  .col-score {
    width: 100px;
  }
}
</style>
