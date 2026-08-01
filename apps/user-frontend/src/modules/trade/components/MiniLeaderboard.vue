<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, inject } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useI18nStore } from '@/stores/i18n';
import { useTradingStore } from '@/stores/trading';
import { useAuthStore } from '@/stores/auth';
import { api } from '@/api';
import { leaderboardLogger } from '@/utils/logger';
import { formatScore, getPnLColorClass } from '@/utils/formatters';
import type { WebSocketMessage, ConnectionStatus } from '@/composables/useWebSocket';

const route = useRoute();
const router = useRouter();
const tradingStore = useTradingStore();
const authStore = useAuthStore();
const i18nStore = useI18nStore();

// Inject WebSocket state from TradingPage
const wsLastMessage = inject<import('vue').Ref<WebSocketMessage | null>>('wsLastMessage', ref(null));
const wsStatus = inject<import('vue').Ref<ConnectionStatus>>('wsStatus', ref('disconnected' as ConnectionStatus));

const contestId = computed(() => route.params.contestId as string);
const isLoading = ref(true);
const isFetching = ref(false);

// WebSocket connection state
const isWsConnected = computed(() => wsStatus.value === 'connected');

let pollInterval: number | null = null;

interface LeaderboardResponse {
  contest_id: string;
  entries: Array<{
    rank: number;
    user_id: string;
    username?: string;
    total_score: number;
  }>;
  total?: number;
}

// Animation states for rank changes
const animatingRanks = ref<Set<string>>(new Set());

async function fetchLeaderboard(showLoading = true) {
  if (!showLoading) {
    isFetching.value = true;
  }
  try {
    const response = await api.get<LeaderboardResponse>(
      `/api/user/leaderboard?contest_id=${contestId.value}`
    );

    if (response.data && response.data.entries) {
      // Detect rank changes for animations
      const currentEntries = tradingStore.leaderboardEntries;
      response.data.entries.forEach(entry => {
        const existing = currentEntries.find(e => e.user_id === entry.user_id);
        if (existing && existing.rank !== entry.rank) {
          animatingRanks.value.add(entry.user_id);
          setTimeout(() => {
            animatingRanks.value.delete(entry.user_id);
          }, 1000);
        }
      });

      tradingStore.updateLeaderboard(
        response.data.entries,
        authStore.user?.id
      );
    }
  } catch (err) {
    leaderboardLogger.error('Failed to fetch leaderboard:', err);
  } finally {
    isLoading.value = false;
    isFetching.value = false;
  }
}

// Locale for formatting
const locale = computed(() => i18nStore.locale === 'fa' ? 'fa-IR' : 'en-US');

function getRankDisplay(rank: number): { icon: string; class: string } {
  switch (rank) {
    case 1:
      return { icon: '🥇', class: 'rank-gold' };
    case 2:
      return { icon: '🥈', class: 'rank-silver' };
    case 3:
      return { icon: '🥉', class: 'rank-bronze' };
    default:
      return { icon: '', class: '' };
  }
}

function getRankChangeDisplay(change: number | undefined): { icon: string; class: string; text: string } {
  if (change === undefined || change === 0) {
    return { icon: '-', class: 'text-slate-500', text: '-' };
  }
  if (change > 0) {
    return { icon: '↑', class: 'text-emerald-400', text: `↑${change}` };
  }
  return { icon: '↓', class: 'text-rose-400', text: `↓${Math.abs(change)}` };
}

function getDisplayName(entry: { user_id: string; username?: string }): string {
  if (entry.user_id === authStore.user?.id) {
    return t('trade.you');
  }
  return entry.username || entry.user_id.substring(0, 8) + '...';
}

function isCurrentUser(entry: { user_id: string }): boolean {
  return entry.user_id === authStore.user?.id;
}

function navigateToFullLeaderboard() {
  router.push({ name: 'leaderboard', params: { contestId: contestId.value } });
}

// Show top 5 entries for mini view
const topEntries = computed(() =>
  tradingStore.leaderboardEntries.slice(0, 5)
);

// Current user entry (for showing at bottom if not in top 5)
const currentUserEntry = computed(() => {
  if (!authStore.user?.id) return null;
  return tradingStore.leaderboardEntries.find(entry => entry.user_id === authStore.user?.id);
});

// Check if current user is in top 5
const isUserInTop5 = computed(() => {
  if (!currentUserEntry.value) return false;
  return currentUserEntry.value.rank <= 5;
});

const userRank = computed(() => tradingStore.userRank);
const userRankChange = computed(() => tradingStore.userRankChange);
const hasData = computed(() => topEntries.value.length > 0);
const totalParticipants = computed(() => tradingStore.leaderboardEntries.length);

// Watch for WebSocket leaderboard_updated messages
watch(wsLastMessage, (msg) => {
  if (msg && msg.type === 'leaderboard_updated') {
    fetchLeaderboard(false);
  }
});

onMounted(() => {
  fetchLeaderboard();

  // Fallback polling every 15 seconds in case WebSocket messages are missed
  pollInterval = window.setInterval(() => {
    fetchLeaderboard(false);
  }, 15000);
});

onUnmounted(() => {
  if (pollInterval !== null) {
    clearInterval(pollInterval);
    pollInterval = null;
  }
});
</script>

<template>
  <div class="mini-leaderboard">
    <!-- Header -->
    <div class="header">
      <div class="flex items-center gap-2">
        <div class="header-icon">
          <svg class="w-3.5 h-3.5 text-amber-400" fill="currentColor" viewBox="0 0 24 24">
            <path d="M5 3h14a2 2 0 012 2v2h-2V5H5v2H3V5a2 2 0 012-2zm7 4l4 5h-3v6h-2v-6H8l4-5zm-9 9v3a2 2 0 002 2h14a2 2 0 002-2v-3h-2v3H5v-3H3z"/>
          </svg>
        </div>
        <span class="header-title">{{ t('trade.leaderboard') }}</span>
      </div>

      <div class="flex items-center gap-2">
        <!-- Connection Status Indicator -->
        <div v-if="isFetching" class="updating-indicator">
          <span class="updating-spinner"></span>
          <span class="updating-text">{{ t('status.updating') }}</span>
        </div>
        <div v-else-if="isWsConnected" class="live-indicator live-indicator--connected">
          <span class="live-dot live-dot--connected"></span>
          <span class="live-text live-text--connected">{{ t('status.live') }}</span>
        </div>
        <div v-else class="live-indicator live-indicator--disconnected">
          <span class="live-dot live-dot--disconnected"></span>
          <span class="live-text live-text--disconnected">{{ t('status.polling') }}</span>
        </div>

        <!-- Expand Button -->
        <button
          class="expand-btn"
          @click="navigateToFullLeaderboard"
          :title="t('common.viewAll')"
        >
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4"/>
          </svg>
        </button>
      </div>
    </div>

    <!-- User Rank Badge (if in top 100) -->
    <div v-if="userRank !== null && !isLoading" class="user-rank-badge">
      <div class="flex items-center gap-1.5">
        <span class="user-star">⭐</span>
        <span class="rank-text">{{ t('trade.yourRank') }}:</span>
      </div>
      <div class="flex items-center gap-2">
        <span :class="['rank-number', getRankDisplay(userRank).class]">
          #{{ userRank }}
        </span>
        <span
          v-if="userRankChange !== 0"
          :class="['rank-change', getRankChangeDisplay(userRankChange).class]"
        >
          {{ getRankChangeDisplay(userRankChange).text }}
        </span>
      </div>
    </div>

    <!-- Leaderboard Table -->
    <div v-if="!isLoading && hasData" class="leaderboard-table">
      <div class="table-header">
        <div class="col-rank">#</div>
        <div class="col-player">{{ t('trade.player') }}</div>
        <div class="col-score">{{ t('trade.score') }}</div>
      </div>
      <div class="table-body">
        <div
          v-for="entry in topEntries"
          :key="entry.user_id"
          :class="[
            'table-row',
            {
              'current-user': isCurrentUser(entry),
              'rank-animated': animatingRanks.has(entry.user_id)
            }
          ]"
        >
          <div class="col-rank">
            <span v-if="getRankDisplay(entry.rank).icon" class="rank-medal">
              {{ getRankDisplay(entry.rank).icon }}
            </span>
            <span v-else :class="['rank-badge', getRankDisplay(entry.rank).class]">
              {{ entry.rank }}
            </span>
          </div>
          <div class="col-player">
            <span v-if="isCurrentUser(entry)" class="user-indicator">⭐</span>
            <span :class="['player-id', { 'current-user-name': isCurrentUser(entry) }]">
              {{ getDisplayName(entry) }}
            </span>
          </div>
          <div class="col-score">
            <span
              :class="[
                'score-value',
                getPnLColorClass(entry.total_score),
                { 'score-animated': animatingRanks.has(entry.user_id) }
              ]"
            >
              {{ formatScore(entry.total_score, { locale, showSign: true, currency: true }) }}
            </span>
          </div>
        </div>

        <!-- Show user's position if not in top 5 -->
        <template v-if="currentUserEntry && !isUserInTop5">
          <div class="separator">
            <span class="separator-dots">• • •</span>
          </div>
          <div class="table-row current-user user-row-highlight">
            <div class="col-rank">
              <span class="rank-badge">{{ currentUserEntry.rank }}</span>
            </div>
            <div class="col-player">
              <span class="user-indicator">⭐</span>
              <span class="player-id current-user-name">{{ t('trade.you') }}</span>
            </div>
            <div class="col-score">
              <span :class="['score-value', getPnLColorClass(currentUserEntry.total_score)]">
                {{ formatScore(currentUserEntry.total_score, { locale, showSign: true, currency: true }) }}
              </span>
            </div>
          </div>
        </template>
      </div>

      <!-- Footer -->
      <div class="footer">
        <span class="participant-count">{{ totalParticipants }} {{ t('leaderboard.participants') }}</span>
        <button class="view-all-btn" @click="navigateToFullLeaderboard">
          {{ t('common.viewAll') }}
        </button>
      </div>
    </div>

    <!-- Loading State -->
    <div v-else-if="isLoading" class="loading-state">
      <div class="spinner"></div>
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Empty State -->
    <div v-else class="empty-state">
      <svg
        width="24"
        height="24"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.5"
      >
        <path d="M9 11l3 3L22 4" />
        <path d="M21 12v7a2 2 0 01-2 2H5a2 2 0 01-2-2V5a2 2 0 012-2h11" />
      </svg>
      <p>{{ t('trade.noLeaderboardData') }}</p>
    </div>
  </div>
</template>

<style scoped>
.mini-leaderboard {
  display: flex;
  flex-direction: column;
  height: 100%;
  font-size: var(--font-size-sm);
}

/* Header */
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 0 var(--spacing-sm) 0;
  margin-bottom: var(--spacing-sm);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.header-icon {
  width: 1.5rem;
  height: 1.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, rgba(251, 191, 36, 0.2), rgba(245, 158, 11, 0.1));
  border-radius: 0.375rem;
}

.header-title {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: white;
}

/* Connection Status Indicators */
.live-indicator {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.125rem 0.375rem;
  border-radius: 9999px;
}

.live-indicator--connected {
  background: rgba(16, 185, 129, 0.15);
  border: 1px solid rgba(16, 185, 129, 0.3);
}

.live-indicator--disconnected {
  background: rgba(251, 191, 36, 0.15);
  border: 1px solid rgba(251, 191, 36, 0.3);
}

.live-dot {
  width: 0.375rem;
  height: 0.375rem;
  border-radius: 50%;
}

.live-dot--connected {
  background: #10b981;
  animation: pulse-live-green 1.5s ease-in-out infinite;
}

.live-dot--disconnected {
  background: #fbbf24;
  animation: none;
}

@keyframes pulse-live-green {
  0%, 100% {
    opacity: 1;
    box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.7);
  }
  50% {
    opacity: 0.7;
    box-shadow: 0 0 0 3px rgba(16, 185, 129, 0);
  }
}

.live-text {
  font-size: 0.5rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.live-text--connected {
  color: #10b981;
}

.live-text--disconnected {
  color: #fbbf24;
}

.updating-indicator {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.125rem 0.375rem;
  background: rgba(6, 182, 212, 0.15);
  border: 1px solid rgba(6, 182, 212, 0.3);
  border-radius: 9999px;
}

.updating-spinner {
  width: 0.375rem;
  height: 0.375rem;
  border: 1.5px solid rgba(6, 182, 212, 0.3);
  border-top-color: #06b6d4;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.updating-text {
  font-size: 0.5rem;
  font-weight: 600;
  color: #06b6d4;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.expand-btn {
  padding: 0.25rem;
  border-radius: 0.25rem;
  color: var(--color-text-muted);
  transition: all 0.2s;
}

.expand-btn:hover {
  background: rgba(255, 255, 255, 0.1);
  color: white;
}

/* User Rank Badge */
.user-rank-badge {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm) var(--spacing-md);
  background: linear-gradient(135deg, var(--color-bg-tertiary) 0%, rgba(16, 185, 129, 0.1) 100%);
  border-radius: var(--radius-sm);
  margin-bottom: var(--spacing-sm);
}

.user-star {
  font-size: 0.75rem;
}

.rank-text {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.rank-number {
  font-size: var(--font-size-lg);
  font-weight: 700;
  font-family: var(--font-family-mono);
}

.rank-number.rank-gold {
  color: #FFD700;
}

.rank-number.rank-silver {
  color: #C0C0C0;
}

.rank-number.rank-bronze {
  color: #CD7F32;
}

.rank-change {
  font-size: var(--font-size-xs);
  font-weight: 600;
  font-family: var(--font-family-mono);
}

/* Table */
.leaderboard-table {
  display: flex;
  flex-direction: column;
  flex: 1;
  overflow: hidden;
}

.table-header,
.table-row {
  display: grid;
  grid-template-columns: 40px 1fr 70px;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  align-items: center;
}

.table-header {
  font-weight: 600;
  color: var(--color-text-muted);
  text-transform: uppercase;
  font-size: 0.5625rem;
  letter-spacing: 0.05em;
  border-bottom: 1px solid var(--color-border);
  padding-bottom: var(--spacing-sm);
}

.table-body {
  overflow-y: auto;
  flex: 1;
}

.table-row {
  border-bottom: 1px solid var(--color-border);
  transition: all 0.2s;
  font-size: var(--font-size-xs);
}

.table-row:last-child {
  border-bottom: none;
}

.table-row:hover {
  background-color: var(--color-bg-tertiary);
}

.table-row.current-user {
  background-color: rgba(16, 185, 129, 0.1);
  border-left: 2px solid var(--color-buy);
}

.table-row.rank-animated {
  animation: rank-flash 0.5s ease-out;
}

@keyframes rank-flash {
  0%, 100% { background: transparent; }
  50% { background: rgba(251, 191, 36, 0.15); }
}

.user-row-highlight {
  background: linear-gradient(90deg, rgba(16, 185, 129, 0.15), transparent) !important;
}

.col-rank {
  text-align: center;
}

.col-score {
  text-align: end;
  font-family: var(--font-family-mono);
}

.rank-medal {
  font-size: 1rem;
  line-height: 1;
}

.rank-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.5rem;
  height: 1.5rem;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  font-weight: 700;
  font-size: var(--font-size-xs);
}

.rank-badge.rank-gold {
  background-color: rgba(255, 215, 0, 0.2);
  color: #FFD700;
  border: 1px solid #FFD700;
}

.rank-badge.rank-silver {
  background-color: rgba(192, 192, 192, 0.2);
  color: #C0C0C0;
  border: 1px solid #C0C0C0;
}

.rank-badge.rank-bronze {
  background-color: rgba(205, 127, 50, 0.2);
  color: #CD7F32;
  border: 1px solid #CD7F32;
}

.col-player {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  overflow: hidden;
}

.user-indicator {
  font-size: 0.625rem;
  flex-shrink: 0;
}

.player-id {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.player-id.current-user-name {
  color: var(--color-accent-green);
  font-weight: 600;
}

.score-value {
  font-weight: 600;
  font-size: var(--font-size-xs);
  transition: all 0.3s;
}

.score-value.score-animated {
  animation: score-bounce 0.5s ease-out;
}

@keyframes score-bounce {
  0%, 100% { transform: scale(1); }
  50% { transform: scale(1.1); }
}

/* Separator */
.separator {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0.25rem 0;
}

.separator-dots {
  font-size: 0.5rem;
  color: var(--color-text-muted);
  letter-spacing: 0.25em;
}

/* Footer */
.footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm) var(--spacing-xs);
  border-top: 1px solid var(--color-border);
  margin-top: auto;
}

.participant-count {
  font-size: 0.5625rem;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.view-all-btn {
  font-size: 0.5625rem;
  color: var(--color-accent-green);
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  transition: all 0.2s;
}

.view-all-btn:hover {
  color: var(--color-accent-cyan);
}

/* Loading & Empty States */
.loading-state,
.empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  color: var(--color-text-muted);
  padding: var(--spacing-lg);
}

.spinner {
  width: 24px;
  height: 24px;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-buy);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.empty-state p,
.loading-state p {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}
</style>
