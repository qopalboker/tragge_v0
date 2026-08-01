<script setup lang="ts">
import { computed, ref } from 'vue';
import { t } from '@/i18n';

interface LeaderboardEntry {
  rank: number;
  user_id: string;
  username?: string;
  total_score: number;
  pnl_percent?: number;
  reward_cents?: number;
  trade_count?: number;
}

const props = withDefaults(defineProps<{
  entries: LeaderboardEntry[];
  loading: boolean;
  currentUserId?: string;
  showRewards?: boolean;
  showTradeCount?: boolean;
  prizeWinnersPercentage?: number;
  totalParticipants?: number;
  pageSize?: number;
}>(), {
  showRewards: true,
  showTradeCount: false,
  prizeWinnersPercentage: 30,
  pageSize: 20,
});

const emit = defineEmits<{
  (e: 'view-trades', userId: string, contestId?: string): void;
}>();

// Pagination state
const currentPage = ref(1);

// Computed
const hasEntries = computed(() => props.entries.length > 0);

// Calculate prize zone cutoff rank
const prizeCutoffRank = computed(() => {
  const total = props.totalParticipants || props.entries.length;
  if (total === 0) return 0;
  return Math.ceil(total * (props.prizeWinnersPercentage / 100));
});

// Total pages
const totalPages = computed(() => {
  return Math.ceil(props.entries.length / props.pageSize);
});

// Paginated entries
const paginatedEntries = computed(() => {
  const start = (currentPage.value - 1) * props.pageSize;
  const end = start + props.pageSize;
  return props.entries.slice(start, end);
});

// Page numbers to display
const pageNumbers = computed(() => {
  const pages: (number | string)[] = [];
  const total = totalPages.value;
  const current = currentPage.value;

  if (total <= 7) {
    for (let i = 1; i <= total; i++) {
      pages.push(i);
    }
  } else {
    pages.push(1);
    if (current > 3) {
      pages.push('...');
    }
    for (let i = Math.max(2, current - 1); i <= Math.min(total - 1, current + 1); i++) {
      pages.push(i);
    }
    if (current < total - 2) {
      pages.push('...');
    }
    pages.push(total);
  }

  return pages;
});

// Find current user's page
const currentUserPage = computed(() => {
  if (!props.currentUserId) return null;
  const index = props.entries.findIndex(e => e.user_id === props.currentUserId);
  if (index === -1) return null;
  return Math.ceil((index + 1) / props.pageSize);
});

// Helper functions
function formatPnl(pnlPercent: number | undefined): string {
  if (pnlPercent === undefined || pnlPercent === null) return '0.00%';
  const sign = pnlPercent >= 0 ? '+' : '';
  return `${sign}${pnlPercent.toFixed(2)}%`;
}

function getPnlClass(pnlPercent: number | undefined): string {
  if (!pnlPercent) return 'neutral';
  if (pnlPercent > 0) return 'positive';
  if (pnlPercent < 0) return 'negative';
  return 'neutral';
}

function formatReward(rewardCents: number | undefined): string {
  if (!rewardCents || rewardCents === 0) return '-';
  const amount = rewardCents / 100;
  return `$${amount.toFixed(2)}`;
}

function isCurrentUser(userId: string): boolean {
  return props.currentUserId === userId;
}

function getDisplayName(entry: LeaderboardEntry): string {
  if (entry.username) return entry.username;
  return entry.user_id.substring(0, 8).toUpperCase();
}

function getRankClass(rank: number): string {
  if (rank === 1) return 'rank-gold';
  if (rank === 2) return 'rank-silver';
  if (rank === 3) return 'rank-bronze';
  return '';
}

function isInPrizeZone(rank: number): boolean {
  return rank <= prizeCutoffRank.value;
}

function isPrizeCutoffRow(rank: number, index: number): boolean {
  // Show cutoff line after the last prize-winning position on this page
  if (index >= paginatedEntries.value.length - 1) return false;
  const nextEntry = paginatedEntries.value[index + 1];
  return rank === prizeCutoffRank.value && nextEntry && nextEntry.rank > prizeCutoffRank.value;
}

function goToPage(page: number | string): void {
  if (typeof page === 'number' && page >= 1 && page <= totalPages.value) {
    currentPage.value = page;
  }
}

function goToMyRank(): void {
  if (currentUserPage.value) {
    currentPage.value = currentUserPage.value;
  }
}

function handleViewTrades(userId: string): void {
  emit('view-trades', userId);
}
</script>

<template>
  <div class="ranking-table">
    <!-- Loading State -->
    <div v-if="loading" class="loading-state">
      <div class="loading-spinner"></div>
      <span>{{ t('common.loading') }}</span>
    </div>

    <!-- Empty State -->
    <div v-else-if="!hasEntries" class="empty-state">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M12 20v-6M6 20V10M18 20v-4" />
      </svg>
      <p>{{ t('contestResults.noRankings') }}</p>
    </div>

    <!-- Table -->
    <template v-else>
      <!-- Jump to My Rank button -->
      <div v-if="currentUserPage && currentUserPage !== currentPage" class="jump-to-rank">
        <button class="jump-btn" @click="goToMyRank">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 19V5M5 12l7-7 7 7" />
          </svg>
          {{ t('contestResults.jumpToMyRank') }}
        </button>
      </div>

      <div class="table-container">
        <table>
          <thead>
            <tr>
              <th class="col-position">{{ t('contestResults.position') }}</th>
              <th class="col-username">{{ t('contestResults.username') }}</th>
              <th class="col-pnl">{{ t('contestResults.pnlColumn') }}</th>
              <th v-if="showTradeCount" class="col-trades">{{ t('contestResults.tradesColumn') }}</th>
              <th v-if="showRewards" class="col-reward">{{ t('contestResults.rewardColumn') }}</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="(entry, index) in paginatedEntries" :key="entry.user_id">
              <tr
                :class="{
                  'current-user': isCurrentUser(entry.user_id),
                  'in-prize-zone': isInPrizeZone(entry.rank),
                  'out-prize-zone': !isInPrizeZone(entry.rank)
                }"
              >
                <td class="col-position">
                  <span class="rank-badge" :class="getRankClass(entry.rank)">
                    {{ entry.rank }}
                  </span>
                </td>
                <td class="col-username">
                  <span class="username">{{ getDisplayName(entry) }}</span>
                  <span v-if="isCurrentUser(entry.user_id)" class="you-badge">
                    {{ t('contestResults.you') }}
                  </span>
                </td>
                <td class="col-pnl" :class="getPnlClass(entry.pnl_percent)">
                  {{ formatPnl(entry.pnl_percent) }}
                </td>
                <td v-if="showTradeCount" class="col-trades">
                  <button
                    v-if="entry.trade_count !== undefined"
                    class="trades-link"
                    @click="handleViewTrades(entry.user_id)"
                  >
                    {{ entry.trade_count }}
                  </button>
                  <span v-else>-</span>
                </td>
                <td v-if="showRewards" class="col-reward" :class="{ 'has-prize': isInPrizeZone(entry.rank) }">
                  {{ formatReward(entry.reward_cents) }}
                </td>
              </tr>
              <!-- Prize zone cutoff line -->
              <tr v-if="isPrizeCutoffRow(entry.rank, index)" class="prize-cutoff-row">
                <td :colspan="showRewards ? (showTradeCount ? 5 : 4) : (showTradeCount ? 4 : 3)">
                  <div class="prize-cutoff-line">
                    <span class="cutoff-label">{{ t('contestResults.prizeCutoff', { percentage: prizeWinnersPercentage }) }}</span>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>

      <!-- Pagination -->
      <div v-if="totalPages > 1" class="pagination">
        <button
          class="page-btn"
          :disabled="currentPage === 1"
          @click="goToPage(currentPage - 1)"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M15 18l-6-6 6-6" />
          </svg>
        </button>

        <button
          v-for="page in pageNumbers"
          :key="page"
          class="page-btn"
          :class="{ 'active': page === currentPage, 'ellipsis': page === '...' }"
          :disabled="page === '...'"
          @click="goToPage(page)"
        >
          {{ page }}
        </button>

        <button
          class="page-btn"
          :disabled="currentPage === totalPages"
          @click="goToPage(currentPage + 1)"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M9 18l6-6-6-6" />
          </svg>
        </button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ranking-table {
  width: 100%;
}

.loading-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.loading-spinner {
  width: 24px;
  height: 24px;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.empty-state svg {
  color: var(--color-text-muted);
}

.empty-state p {
  font-size: var(--font-size-sm);
  margin: 0;
}

.jump-to-rank {
  display: flex;
  justify-content: flex-end;
  padding: var(--spacing-sm) var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
}

.jump-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: var(--color-primary);
  color: white;
  border: none;
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: 600;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.jump-btn:hover {
  background: var(--color-secondary);
}

.table-container {
  overflow-x: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
}

thead {
  background: var(--color-bg-secondary);
}

th {
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-text-secondary);
  text-align: left;
  border-bottom: 1px solid var(--color-border);
}

td {
  padding: var(--spacing-md);
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  border-bottom: 1px solid var(--color-border);
}

tbody tr:hover {
  background: var(--color-bg-secondary);
}

tbody tr.current-user {
  background: rgba(99, 102, 241, 0.1);
}

tbody tr.current-user:hover {
  background: rgba(99, 102, 241, 0.15);
}

tbody tr.out-prize-zone {
  opacity: 0.7;
}

tbody tr.out-prize-zone:hover {
  opacity: 1;
}

/* Prize cutoff row */
.prize-cutoff-row {
  background: transparent !important;
}

.prize-cutoff-row td {
  padding: 0;
  border: none;
}

.prize-cutoff-line {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-xs) var(--spacing-md);
  background: linear-gradient(90deg, transparent 0%, rgba(239, 68, 68, 0.1) 50%, transparent 100%);
  border-top: 2px dashed var(--color-danger);
  border-bottom: 2px dashed var(--color-danger);
}

.cutoff-label {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-danger);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

/* Column widths */
.col-position {
  width: 80px;
  text-align: center;
}

.col-username {
  min-width: 150px;
}

.col-pnl {
  width: 100px;
  text-align: right;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.col-trades {
  width: 80px;
  text-align: center;
}

.col-reward {
  width: 100px;
  text-align: right;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.col-reward.has-prize {
  color: #10B981;
}

/* Rank badges */
.rank-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  height: 28px;
  padding: 0 var(--spacing-xs);
  border-radius: var(--radius-md);
  font-weight: 600;
  font-size: var(--font-size-sm);
  background: var(--color-bg-tertiary);
  color: var(--color-text-primary);
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

/* Username styling */
.username {
  font-weight: 500;
}

.you-badge {
  display: inline-flex;
  align-items: center;
  margin-left: var(--spacing-xs);
  padding: 2px 6px;
  background: var(--color-primary);
  color: white;
  font-size: 10px;
  font-weight: 600;
  border-radius: var(--radius-sm);
  text-transform: uppercase;
}

/* Trades link */
.trades-link {
  background: transparent;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  padding: 2px 8px;
  font-size: var(--font-size-sm);
  color: var(--color-primary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.trades-link:hover {
  background: var(--color-primary);
  color: white;
  border-color: var(--color-primary);
}

/* PnL colors */
.col-pnl.positive {
  color: #10B981;
}

.col-pnl.negative {
  color: #EF4444;
}

.col-pnl.neutral {
  color: var(--color-text-secondary);
}

/* Pagination */
.pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-md);
  border-top: 1px solid var(--color-border);
}

.page-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  height: 32px;
  padding: 0 var(--spacing-sm);
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.page-btn:hover:not(:disabled) {
  background: var(--color-bg-tertiary);
  border-color: var(--color-primary);
}

.page-btn.active {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: white;
}

.page-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.page-btn.ellipsis {
  border: none;
  background: transparent;
}

/* RTL Support */
[dir="rtl"] th,
[dir="rtl"] td {
  text-align: right;
}

[dir="rtl"] .col-position {
  text-align: center;
}

[dir="rtl"] .col-pnl,
[dir="rtl"] .col-reward {
  text-align: left;
}

[dir="rtl"] .you-badge {
  margin-left: 0;
  margin-right: var(--spacing-xs);
}

[dir="rtl"] .page-btn svg {
  transform: rotate(180deg);
}

/* Mobile */
@media (max-width: 767px) {
  th,
  td {
    padding: var(--spacing-sm);
  }

  .col-username {
    min-width: 100px;
  }

  .username {
    font-size: var(--font-size-xs);
  }

  .you-badge {
    display: none;
  }

  .col-pnl,
  .col-reward {
    font-size: var(--font-size-xs);
  }

  .col-trades {
    display: none;
  }

  .pagination {
    gap: 2px;
    padding: var(--spacing-sm);
  }

  .page-btn {
    min-width: 28px;
    height: 28px;
    font-size: var(--font-size-xs);
  }
}
</style>
