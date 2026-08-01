<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, inject } from 'vue';
import { useRoute } from 'vue-router';
import { t } from '@/i18n';
import { useI18nStore } from '@/stores/i18n';
import { useTradingStore } from '@/stores/trading';
import { useAuthStore } from '@/stores/auth';
import { api } from '@/api';
import { leaderboardLogger } from '@/utils/logger';
import { formatScore, getPnLColorClass } from '@/utils/formatters';
import type { WebSocketMessage, ConnectionStatus } from '@/composables/useWebSocket';
import { getPrizePoolInfo, isInPrizeZone, getPrizeZoneCutoff, formatPrize, type RankPrize } from '@/utils/prizeDistribution';

// Props
const props = defineProps<{
  contestId?: string;
  compact?: boolean;
  entryFeeCents?: number;
}>();

// Emits
const emit = defineEmits<{
  (e: 'expand'): void;
}>();

const route = useRoute();
const tradingStore = useTradingStore();
const authStore = useAuthStore();
const i18nStore = useI18nStore();

// Inject WebSocket state from TradingPage
const wsLastMessage = inject<import('vue').Ref<WebSocketMessage | null>>('wsLastMessage', ref(null));
const wsStatus = inject<import('vue').Ref<ConnectionStatus>>('wsStatus', ref('disconnected' as ConnectionStatus));

// Contest ID from props or route
const contestId = computed(() => props.contestId || (route.params.contestId as string));

// State
const isLoading = ref(true);
const isFetching = ref(false);
const isLoadingMore = ref(false);
const searchQuery = ref('');
const page = ref(1);
const pageSize = 50;
const totalParticipants = ref(0);
const lastUpdateTime = ref<Date | null>(null);

// Animation states for rank changes
const animatingRanks = ref<Set<string>>(new Set());
const previousRanks = ref<Map<string, number>>(new Map());

// WebSocket connection state
const isWsConnected = computed(() => wsStatus.value === 'connected');

let pollInterval: number | null = null;

interface LeaderboardEntry {
  rank: number;
  user_id: string;
  username?: string;
  total_score: number;
  rank_change?: number;
}

interface LeaderboardResponse {
  contest_id: string;
  entries: LeaderboardEntry[];
  total?: number;
}

// Local entries with rank change tracking
const localEntries = ref<LeaderboardEntry[]>([]);

// Filtered entries based on search
const filteredEntries = computed(() => {
  if (!searchQuery.value.trim()) {
    return localEntries.value;
  }
  const query = searchQuery.value.toLowerCase();
  return localEntries.value.filter(entry => {
    const displayName = entry.username || entry.user_id.substring(0, 8);
    return displayName.toLowerCase().includes(query);
  });
});

// Paginated entries
const displayedEntries = computed(() => {
  if (props.compact) {
    return filteredEntries.value.slice(0, 5);
  }
  return filteredEntries.value.slice(0, page.value * pageSize);
});

// Check if there are more entries to load
const hasMore = computed(() => {
  return displayedEntries.value.length < filteredEntries.value.length;
});

// Current user's entry
const currentUserEntry = computed(() => {
  if (!authStore.user?.id) return null;
  return localEntries.value.find(entry => entry.user_id === authStore.user?.id);
});

// Check if current user is visible in displayed entries
const isUserVisible = computed(() => {
  if (!currentUserEntry.value) return false;
  return displayedEntries.value.some(entry => entry.user_id === authStore.user?.id);
});

// Locale for formatting
const locale = computed(() => i18nStore.locale === 'fa' ? 'fa-IR' : 'en-US');

// Prize zone calculations
const prizeZoneCutoff = computed(() => {
  if (!props.entryFeeCents || totalParticipants.value <= 0) return 0;
  return getPrizeZoneCutoff(totalParticipants.value);
});

const prizePoolInfo = computed(() => {
  if (!props.entryFeeCents || totalParticipants.value <= 0) return null;
  return getPrizePoolInfo(props.entryFeeCents, totalParticipants.value);
});

const netPrizePool = computed(() => {
  if (!prizePoolInfo.value) return null;
  return formatPrize(prizePoolInfo.value.netPoolCents, locale.value);
});

const userProjectedPrize = computed(() => {
  if (!currentUserEntry.value || !prizePoolInfo.value) return null;
  const rank = currentUserEntry.value.rank;
  if (rank > prizePoolInfo.value.winnersCount) return null;
  const prize = prizePoolInfo.value.prizes.find((p: RankPrize) => p.rank === rank);
  return prize ? formatPrize(prize.prizeCents, locale.value) : null;
});

function checkInPrizeZone(rank: number): boolean {
  if (!props.entryFeeCents || totalParticipants.value <= 0) return false;
  return isInPrizeZone(rank, totalParticipants.value);
}

function isPrizeZoneCutoffRank(rank: number): boolean {
  return rank === prizeZoneCutoff.value && prizeZoneCutoff.value > 0;
}

async function fetchLeaderboard(showLoading = true) {
  if (showLoading && page.value === 1) {
    isLoading.value = true;
  }
  if (!showLoading) {
    isFetching.value = true;
  }

  try {
    const response = await api.get<LeaderboardResponse>(
      `/api/user/leaderboard?contest_id=${contestId.value}`
    );

    if (response.data && response.data.entries) {
      const newEntries = response.data.entries;

      // Calculate rank changes
      newEntries.forEach(entry => {
        const prevRank = previousRanks.value.get(entry.user_id);
        if (prevRank !== undefined && prevRank !== entry.rank) {
          entry.rank_change = prevRank - entry.rank;
          // Trigger animation (2s duration)
          animatingRanks.value.add(entry.user_id);
          setTimeout(() => {
            animatingRanks.value.delete(entry.user_id);
          }, 2000);
        } else {
          entry.rank_change = 0;
        }
        previousRanks.value.set(entry.user_id, entry.rank);
      });

      localEntries.value = newEntries;
      totalParticipants.value = response.data.total || newEntries.length;
      lastUpdateTime.value = new Date();

      // Update store
      tradingStore.updateLeaderboard(
        newEntries,
        authStore.user?.id
      );
    }
  } catch (err) {
    leaderboardLogger.error('Failed to fetch leaderboard:', err);
  } finally {
    isLoading.value = false;
    isFetching.value = false;
    isLoadingMore.value = false;
  }
}

function loadMore() {
  if (!hasMore.value || isLoadingMore.value) return;
  isLoadingMore.value = true;
  page.value += 1;
  // Entries are already loaded, just display more
  setTimeout(() => {
    isLoadingMore.value = false;
  }, 300);
}

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
    return { icon: '', class: '', text: '' };
  }
  if (change > 0) {
    return { icon: '▲', class: 'rank-change-up', text: `▲ +${change}` };
  }
  return { icon: '▼', class: 'rank-change-down', text: `▼ -${Math.abs(change)}` };
}

function getDisplayName(entry: LeaderboardEntry): string {
  if (entry.user_id === authStore.user?.id) {
    return 'YOU';
  }
  return entry.username || entry.user_id.substring(0, 8) + '...';
}

function isCurrentUser(entry: LeaderboardEntry): boolean {
  return entry.user_id === authStore.user?.id;
}

function scrollToUser() {
  const userElement = document.querySelector('.leaderboard-current-user');
  if (userElement) {
    userElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
}

// Watch for search changes to reset pagination
watch(searchQuery, () => {
  page.value = 1;
});

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
  <div :class="['leaderboard-panel', { 'compact': compact }]">
    <!-- Header -->
    <div class="leaderboard-header">
      <div class="flex items-center gap-2">
        <div class="header-icon">
          <svg class="w-4 h-4 text-amber-400" fill="currentColor" viewBox="0 0 24 24">
            <path d="M5 3h14a2 2 0 012 2v2h-2V5H5v2H3V5a2 2 0 012-2zm7 4l4 5h-3v6h-2v-6H8l4-5zm-9 9v3a2 2 0 002 2h14a2 2 0 002-2v-3h-2v3H5v-3H3z"/>
          </svg>
        </div>
        <h3 class="header-title">{{ t('trade.leaderboard') }}</h3>
        <!-- Prize Pool Badge -->
        <span v-if="netPrizePool" class="prize-pool-badge">
          <span class="prize-icon">&#x1F4B0;</span>
          {{ netPrizePool }}
        </span>
      </div>

      <div class="flex items-center gap-3">
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

        <!-- Expand Button (compact mode only) -->
        <button
          v-if="compact"
          class="expand-btn"
          @click="emit('expand')"
          :title="t('common.viewAll')"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4"/>
          </svg>
        </button>
      </div>
    </div>

    <!-- Search Bar (non-compact mode) -->
    <div v-if="!compact" class="search-bar">
      <svg class="search-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
      </svg>
      <input
        v-model="searchQuery"
        type="text"
        :placeholder="t('common.search')"
        class="search-input"
      />
    </div>

    <!-- User's Position Card (if not visible in list) -->
    <div
      v-if="currentUserEntry && !isUserVisible && !compact"
      :class="['user-position-card', { 'in-prize-zone': checkInPrizeZone(currentUserEntry.rank) }]"
      @click="scrollToUser"
    >
      <div class="flex items-center gap-2">
        <span class="user-star">⭐</span>
        <span class="user-rank-label">{{ t('trade.yourRank') }}:</span>
      </div>
      <div class="flex items-center gap-3">
        <span class="user-rank-value">#{{ currentUserEntry.rank }}</span>
        <span :class="['user-score', getPnLColorClass(currentUserEntry.total_score)]">
          {{ formatScore(currentUserEntry.total_score, { locale, showSign: true, currency: true }) }}
        </span>
        <!-- Prize indicator -->
        <span v-if="userProjectedPrize" class="user-prize-badge">
          <span class="prize-icon">&#x1F4B5;</span>
          {{ userProjectedPrize }}
        </span>
        <span v-else-if="prizeZoneCutoff > 0" class="ranks-away-badge">
          {{ currentUserEntry.rank - prizeZoneCutoff }} {{ t('prize.ranksAway') }}
        </span>
        <span
          v-if="currentUserEntry.rank_change !== undefined && currentUserEntry.rank_change !== 0"
          :class="['rank-change-badge', getRankChangeDisplay(currentUserEntry.rank_change).class]"
        >
          {{ getRankChangeDisplay(currentUserEntry.rank_change).text }}
        </span>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="isLoading" class="loading-state">
      <div class="spinner"></div>
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Leaderboard Table -->
    <div v-else-if="displayedEntries.length > 0" class="leaderboard-content">
      <!-- Table Header -->
      <div class="table-header">
        <div class="col-rank">#</div>
        <div v-if="!compact" class="col-rank-change">+/-</div>
        <div class="col-trader">{{ t('trade.player') }}</div>
        <div class="col-score">{{ t('trade.score') }}</div>
      </div>

      <!-- Table Body with Virtual Scrolling Ready Container -->
      <div class="table-body scrollbar-thin">
        <!-- Prize Zone Cutoff Indicator -->
        <div
          v-if="prizeZoneCutoff > 0 && !compact && displayedEntries.some(e => e.rank === prizeZoneCutoff)"
          class="prize-zone-cutoff-line"
          :style="{ '--cutoff-position': displayedEntries.findIndex(e => e.rank === prizeZoneCutoff) }"
        >
          <span class="cutoff-label">
            <span class="cutoff-icon">&#x1F3C6;</span>
            {{ t('prize.prizeZone') }}
          </span>
        </div>
        <div
          v-for="entry in displayedEntries"
          :key="entry.user_id"
          :class="[
            'table-row',
            {
              'current-user': isCurrentUser(entry),
              'leaderboard-current-user': isCurrentUser(entry),
              'rank-animated': animatingRanks.has(entry.user_id),
              'rank-up': entry.rank_change && entry.rank_change > 0,
              'rank-down': entry.rank_change && entry.rank_change < 0,
              'in-prize-zone': checkInPrizeZone(entry.rank),
              'out-prize-zone': prizeZoneCutoff > 0 && !checkInPrizeZone(entry.rank),
              'prize-zone-cutoff': isPrizeZoneCutoffRank(entry.rank)
            }
          ]"
        >
          <!-- Rank -->
          <div class="col-rank">
            <span v-if="getRankDisplay(entry.rank).icon" class="rank-medal">
              {{ getRankDisplay(entry.rank).icon }}
            </span>
            <span v-else :class="['rank-number', getRankDisplay(entry.rank).class]">
              {{ entry.rank }}
            </span>
          </div>

          <!-- Rank Change (between Rank and Username) -->
          <div v-if="!compact" class="col-rank-change">
            <span
              v-if="entry.rank_change && entry.rank_change !== 0"
              :class="[
                'change-indicator',
                getRankChangeDisplay(entry.rank_change).class
              ]"
            >
              {{ getRankChangeDisplay(entry.rank_change).text }}
            </span>
          </div>

          <!-- Trader Name -->
          <div class="col-trader">
            <span v-if="isCurrentUser(entry)" class="user-indicator">⭐</span>
            <span :class="['trader-name', { 'current-user-name': isCurrentUser(entry) }]">
              {{ getDisplayName(entry) }}
            </span>
          </div>

          <!-- Score -->
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
      </div>

      <!-- Footer -->
      <div v-if="!compact" class="leaderboard-footer">
        <div class="footer-stats">
          {{ t('leaderboard.showing') }} {{ displayedEntries.length }} {{ t('leaderboard.of') }} {{ totalParticipants }} {{ t('leaderboard.participants') }}
        </div>

        <button
          v-if="hasMore"
          class="load-more-btn"
          :disabled="isLoadingMore"
          @click="loadMore"
        >
          <span v-if="isLoadingMore" class="spinner-small"></span>
          <span v-else>{{ t('history.loadMore') }}</span>
        </button>
      </div>

      <!-- Compact Footer -->
      <div v-else class="compact-footer">
        <span class="participant-count">{{ totalParticipants }} {{ t('leaderboard.participants') }}</span>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else class="empty-state">
      <svg class="empty-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/>
      </svg>
      <p>{{ t('trade.noLeaderboardData') }}</p>
    </div>
  </div>
</template>

<style scoped>
.leaderboard-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: linear-gradient(135deg, rgba(20, 25, 38, 0.95), rgba(15, 20, 32, 0.98));
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 1rem;
  overflow: hidden;
}

.leaderboard-panel.compact {
  max-height: 400px;
}

/* Header */
.leaderboard-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.875rem 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  background: linear-gradient(135deg, rgba(251, 191, 36, 0.05), transparent);
}

.header-icon {
  width: 1.75rem;
  height: 1.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, rgba(251, 191, 36, 0.2), rgba(245, 158, 11, 0.1));
  border-radius: 0.5rem;
}

.header-title {
  font-size: 0.875rem;
  font-weight: 600;
  color: white;
}

/* Connection Status Indicators */
.live-indicator {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.25rem 0.625rem;
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
  width: 0.5rem;
  height: 0.5rem;
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
    box-shadow: 0 0 0 4px rgba(16, 185, 129, 0);
  }
}

.live-text {
  font-size: 0.625rem;
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
  gap: 0.375rem;
  padding: 0.25rem 0.625rem;
  background: rgba(6, 182, 212, 0.15);
  border: 1px solid rgba(6, 182, 212, 0.3);
  border-radius: 9999px;
}

.updating-spinner {
  width: 0.5rem;
  height: 0.5rem;
  border: 1.5px solid rgba(6, 182, 212, 0.3);
  border-top-color: #06b6d4;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.updating-text {
  font-size: 0.625rem;
  font-weight: 600;
  color: #06b6d4;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.expand-btn {
  padding: 0.375rem;
  border-radius: 0.375rem;
  color: var(--color-text-muted);
  transition: all 0.2s;
}

.expand-btn:hover {
  background: rgba(255, 255, 255, 0.1);
  color: white;
}

/* Search Bar */
.search-bar {
  position: relative;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.search-icon {
  position: absolute;
  inset-inline-start: 1.5rem;
  top: 50%;
  transform: translateY(-50%);
  width: 1rem;
  height: 1rem;
  color: var(--color-text-muted);
}

.search-input {
  width: 100%;
  padding-block: 0.5rem;
  padding-inline-start: 2.25rem;
  padding-inline-end: 0.75rem;
  background: rgba(15, 20, 32, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 0.5rem;
  color: white;
  font-size: 0.8125rem;
  transition: all 0.2s;
}

.search-input:focus {
  outline: none;
  border-color: rgba(16, 185, 129, 0.5);
  background: rgba(15, 20, 32, 0.8);
}

.search-input::placeholder {
  color: var(--color-text-muted);
}

/* User Position Card */
.user-position-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 0.75rem;
  padding: 0.75rem 1rem;
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.15), rgba(6, 182, 212, 0.1));
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: 0.75rem;
  cursor: pointer;
  transition: all 0.2s;
}

.user-position-card:hover {
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.2), rgba(6, 182, 212, 0.15));
  transform: translateY(-1px);
}

.user-star {
  font-size: 1rem;
}

.user-rank-label {
  font-size: 0.75rem;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.user-rank-value {
  font-size: 1.125rem;
  font-weight: 700;
  font-family: var(--font-family-mono);
  color: white;
}

.user-score {
  font-size: 0.875rem;
  font-weight: 600;
  font-family: var(--font-family-mono);
}

.rank-change-badge {
  font-size: 0.75rem;
  font-weight: 600;
  padding: 0.125rem 0.5rem;
  background: rgba(0, 0, 0, 0.3);
  border-radius: 9999px;
}

/* Table */
.leaderboard-content {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.table-header {
  display: grid;
  grid-template-columns: 60px 60px 1fr 100px;
  gap: 0.5rem;
  padding: 0.625rem 1rem;
  background: rgba(15, 20, 32, 0.5);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  font-size: 0.6875rem;
  font-weight: 600;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.compact .table-header {
  grid-template-columns: 50px 1fr 90px;
}

.table-body {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}

.table-row {
  display: grid;
  grid-template-columns: 60px 60px 1fr 100px;
  gap: 0.5rem;
  padding: 0.625rem 1rem;
  align-items: center;
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
  background-color: transparent;
  transition: background-color 2s ease-out;
}

.compact .table-row {
  grid-template-columns: 50px 1fr 90px;
  padding: 0.5rem 0.75rem;
}

.table-row:hover {
  background-color: rgba(255, 255, 255, 0.03);
}

/* Current user: subtle blue border */
.table-row.current-user {
  background: linear-gradient(90deg, rgba(59, 130, 246, 0.1), transparent);
  border-inline-start: 3px solid rgba(59, 130, 246, 0.7);
}

[dir="rtl"] .table-row.current-user {
  background: linear-gradient(270deg, rgba(59, 130, 246, 0.1), transparent);
}

/* Rank animated: flash then fade via transition */
.table-row.rank-animated.rank-up {
  background-color: rgba(16, 185, 129, 0.25);
  transition: none;
}

.table-row.rank-animated.rank-down {
  background-color: rgba(239, 68, 68, 0.2);
  transition: none;
}

/* Once animatingRanks removes the user_id, rank-animated is removed and
   the background-color transitions back to transparent over 2s */
.table-row:not(.rank-animated).rank-up {
  background-color: transparent;
  transition: background-color 2s ease-out;
}

.table-row:not(.rank-animated).rank-down {
  background-color: transparent;
  transition: background-color 2s ease-out;
}

/* Columns */
.col-rank {
  text-align: center;
}

.col-rank-change {
  text-align: center;
}

.col-score {
  text-align: end;
  font-family: var(--font-family-mono);
}

.rank-medal {
  font-size: 1.25rem;
  line-height: 1;
}

.rank-number {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.75rem;
  height: 1.75rem;
  background: rgba(255, 255, 255, 0.05);
  border-radius: 0.375rem;
  font-size: 0.75rem;
  font-weight: 600;
  font-family: var(--font-family-mono);
  color: var(--color-text-secondary);
}

.rank-number.rank-gold {
  background: rgba(255, 215, 0, 0.2);
  color: #FFD700;
  border: 1px solid rgba(255, 215, 0, 0.4);
}

.rank-number.rank-silver {
  background: rgba(192, 192, 192, 0.2);
  color: #C0C0C0;
  border: 1px solid rgba(192, 192, 192, 0.4);
}

.rank-number.rank-bronze {
  background: rgba(205, 127, 50, 0.2);
  color: #CD7F32;
  border: 1px solid rgba(205, 127, 50, 0.4);
}

.col-trader {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  overflow: hidden;
}

.user-indicator {
  font-size: 0.875rem;
  flex-shrink: 0;
}

.trader-name {
  font-size: 0.8125rem;
  font-family: var(--font-family-mono);
  color: var(--color-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trader-name.current-user-name {
  color: var(--color-accent-green);
  font-weight: 600;
}

.score-value {
  font-size: 0.8125rem;
  font-weight: 600;
  transition: all 0.3s;
}

.score-value.score-animated {
  animation: score-bounce 0.5s ease-out;
}

@keyframes score-bounce {
  0%, 100% { transform: scale(1); }
  50% { transform: scale(1.1); }
}

.change-indicator {
  font-size: 0.6875rem;
  font-weight: 600;
  font-family: var(--font-family-mono);
  white-space: nowrap;
}

.rank-change-up {
  color: #10b981;
}

.rank-change-down {
  color: #ef4444;
}

/* Footer */
.leaderboard-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(15, 20, 32, 0.3);
}

.footer-stats {
  font-size: 0.6875rem;
  color: var(--color-text-muted);
}

.load-more-btn {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.375rem 0.875rem;
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.2), rgba(6, 182, 212, 0.15));
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: 0.5rem;
  color: var(--color-accent-green);
  font-size: 0.75rem;
  font-weight: 500;
  transition: all 0.2s;
}

.load-more-btn:hover:not(:disabled) {
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.3), rgba(6, 182, 212, 0.25));
  transform: translateY(-1px);
}

.load-more-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.compact-footer {
  padding: 0.5rem 0.75rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  text-align: center;
}

.participant-count {
  font-size: 0.625rem;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

/* Loading & Empty States */
.loading-state,
.empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  padding: 2rem;
  color: var(--color-text-muted);
}

.spinner {
  width: 2rem;
  height: 2rem;
  border: 2px solid rgba(255, 255, 255, 0.1);
  border-top-color: var(--color-accent-green);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

.spinner-small {
  width: 1rem;
  height: 1rem;
  border: 2px solid rgba(255, 255, 255, 0.1);
  border-top-color: var(--color-accent-green);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.empty-icon {
  width: 3rem;
  height: 3rem;
  opacity: 0.5;
}

.loading-state p,
.empty-state p {
  font-size: 0.8125rem;
  text-align: center;
}

/* Prize Zone Styles */
.prize-pool-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem 0.5rem;
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.2), rgba(6, 182, 212, 0.15));
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: 9999px;
  font-size: 0.6875rem;
  font-weight: 600;
  color: var(--color-accent-green);
}

.prize-icon {
  font-size: 0.75rem;
}

.user-position-card.in-prize-zone {
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.2), rgba(6, 182, 212, 0.15));
  border: 1px solid rgba(16, 185, 129, 0.4);
}

.user-prize-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.125rem 0.375rem;
  background: rgba(16, 185, 129, 0.2);
  border-radius: 0.25rem;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--color-accent-green);
}

.ranks-away-badge {
  font-size: 0.625rem;
  color: var(--color-text-muted);
  font-style: italic;
}

.prize-zone-cutoff-line {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0.25rem 0;
  position: relative;
}

.prize-zone-cutoff-line::before,
.prize-zone-cutoff-line::after {
  content: '';
  flex: 1;
  height: 2px;
  background: linear-gradient(90deg, transparent, #10B981 50%, transparent);
}

.prize-zone-cutoff-line::before {
  background: linear-gradient(90deg, transparent, #10B981);
}

.prize-zone-cutoff-line::after {
  background: linear-gradient(90deg, #10B981, transparent);
}

.cutoff-label {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.125rem 0.5rem;
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.2), rgba(6, 182, 212, 0.15));
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: 9999px;
  font-size: 0.5625rem;
  font-weight: 600;
  color: #10B981;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  white-space: nowrap;
}

.cutoff-icon {
  font-size: 0.625rem;
}

.table-row.in-prize-zone {
  background: rgba(16, 185, 129, 0.03);
}

.table-row.in-prize-zone:hover {
  background: rgba(16, 185, 129, 0.08);
}

.table-row.out-prize-zone {
  opacity: 0.65;
}

.table-row.out-prize-zone:hover {
  opacity: 0.9;
}

.table-row.prize-zone-cutoff {
  border-bottom: 2px solid rgba(16, 185, 129, 0.3);
}

/* Mobile Responsive */
@media (max-width: 640px) {
  .table-header,
  .table-row {
    grid-template-columns: 50px 1fr 80px;
  }

  .col-rank-change {
    display: none;
  }

  .leaderboard-footer {
    flex-direction: column;
    gap: 0.5rem;
  }

  .prize-pool-badge {
    display: none;
  }
}
</style>
