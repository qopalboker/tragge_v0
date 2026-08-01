<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import { t } from '@/i18n';
import { api } from '@/api';

interface PrizeRank {
  rank: number;
  amount_cents: number;
  percentage: number;
}

interface PrizePreviewData {
  contest_id: string;
  current_participants: number;
  min_participants: number;
  quorum_met: boolean;
  entry_fee_cents: number;
  commission_rate: number;
  prize_pool_cents: number;
  winners_count: number;
  prizes: PrizeRank[];
  status: string;
  message: string;
}

const props = defineProps<{
  contestId: string;
  status: string;
}>();

// State
const prizeData = ref<PrizePreviewData | null>(null);
const loading = ref(true);
const error = ref<string | null>(null);
const activeTab = ref<'prizes' | 'rules' | 'participants'>('prizes');
const changedFields = ref<Set<string>>(new Set());
let pollTimer: ReturnType<typeof setInterval> | null = null;

// Computed
const formattedPrizePool = computed(() => {
  if (!prizeData.value) return '$0.00';
  return formatCents(prizeData.value.prize_pool_cents);
});

const formattedFirstPrize = computed(() => {
  if (!prizeData.value || !prizeData.value.prizes.length) return '$0.00';
  return formatCents(prizeData.value.prizes[0].amount_cents);
});

const showPrizeTable = computed(() => {
  return prizeData.value && prizeData.value.quorum_met && prizeData.value.prizes.length > 0;
});

const isRegistrationPhase = computed(() => {
  return props.status === 'registration_open' || props.status === 'scheduled';
});

// Format cents to currency
function formatCents(cents: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(cents / 100);
}

// Rank label with ordinal
function getRankLabel(rank: number): string {
  const suffixes = ['th', 'st', 'nd', 'rd'];
  const v = rank % 100;
  return rank + (suffixes[(v - 20) % 10] || suffixes[v] || suffixes[0]);
}

// Rank medal
function getRankMedal(rank: number): string {
  switch (rank) {
    case 1: return '\u{1F947}';
    case 2: return '\u{1F948}';
    case 3: return '\u{1F949}';
    default: return '';
  }
}

// Fetch prize preview data
async function fetchPrizePreview(): Promise<void> {
  try {
    const response = await api.get<PrizePreviewData>(
      `/api/user/contests/${props.contestId}/prize-preview`
    );

    const newData = response.data;

    // Detect changes and trigger animation
    if (prizeData.value) {
      const changed = new Set<string>();

      if (prizeData.value.current_participants !== newData.current_participants) {
        changed.add('participants');
      }
      if (prizeData.value.prize_pool_cents !== newData.prize_pool_cents) {
        changed.add('prize_pool');
      }
      if (prizeData.value.winners_count !== newData.winners_count) {
        changed.add('winners');
      }

      // Check individual prize changes
      for (const prize of newData.prizes) {
        const oldPrize = prizeData.value.prizes.find(p => p.rank === prize.rank);
        if (!oldPrize || oldPrize.amount_cents !== prize.amount_cents) {
          changed.add(`prize_${prize.rank}`);
        }
      }

      if (changed.size > 0) {
        changedFields.value = changed;
        // Clear animation after 1.5s
        setTimeout(() => {
          changedFields.value = new Set();
        }, 1500);
      }
    }

    prizeData.value = newData;
    error.value = null;
  } catch (err) {
    if (!prizeData.value) {
      error.value = t('prizePreview.loadError');
    }
    console.error('Failed to fetch prize preview:', err);
  } finally {
    loading.value = false;
  }
}

// Start polling for updates during registration
function startPolling(): void {
  stopPolling();
  if (isRegistrationPhase.value) {
    pollTimer = setInterval(fetchPrizePreview, 10000);
  }
}

function stopPolling(): void {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

// Watch for status changes to start/stop polling
watch(() => props.status, () => {
  if (isRegistrationPhase.value) {
    startPolling();
  } else {
    stopPolling();
  }
});

// Lifecycle
onMounted(async () => {
  await fetchPrizePreview();
  startPolling();
});

onUnmounted(() => {
  stopPolling();
});
</script>

<template>
  <div class="prize-preview-table">
    <!-- Tab Navigation -->
    <div class="tab-nav">
      <button
        :class="['tab-btn', { active: activeTab === 'prizes' }]"
        @click="activeTab = 'prizes'"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" />
        </svg>
        {{ t('prizePreview.prizeTable') }}
      </button>
      <button
        :class="['tab-btn', { active: activeTab === 'rules' }]"
        @click="activeTab = 'rules'"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
          <polyline points="14 2 14 8 20 8" />
          <line x1="16" y1="13" x2="8" y2="13" />
          <line x1="16" y1="17" x2="8" y2="17" />
          <polyline points="10 9 9 9 8 9" />
        </svg>
        {{ t('prizePreview.rules') }}
      </button>
      <button
        :class="['tab-btn', { active: activeTab === 'participants' }]"
        @click="activeTab = 'participants'"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
          <circle cx="9" cy="7" r="4" />
          <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
          <path d="M16 3.13a4 4 0 0 1 0 7.75" />
        </svg>
        {{ t('prizePreview.participants') }}
        <span v-if="prizeData" class="participant-badge" :class="{ 'pulse-animation': changedFields.has('participants') }">
          {{ prizeData.current_participants }}
        </span>
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="tab-content loading-state">
      <div class="loading-spinner"></div>
      <span>{{ t('common.loading') }}</span>
    </div>

    <!-- Error State -->
    <div v-else-if="error && !prizeData" class="tab-content error-state">
      <p>{{ error }}</p>
      <button class="btn-retry" @click="fetchPrizePreview">{{ t('common.retry') }}</button>
    </div>

    <!-- Prize Table Tab -->
    <div v-else-if="activeTab === 'prizes'" class="tab-content">
      <!-- Summary Row -->
      <div v-if="prizeData" class="prize-summary-row">
        <div class="summary-stat" :class="{ 'pulse-animation': changedFields.has('participants') }">
          <span class="stat-value">{{ prizeData.current_participants }}</span>
          <span class="stat-label">{{ t('prizePreview.traders') }}</span>
        </div>
        <div class="summary-divider"></div>
        <div class="summary-stat" :class="{ 'pulse-animation': changedFields.has('prize_pool') }">
          <span class="stat-value prize-value">{{ formattedPrizePool }}</span>
          <span class="stat-label">{{ t('prizePreview.totalPrize') }}</span>
        </div>
        <div class="summary-divider"></div>
        <div class="summary-stat" :class="{ 'pulse-animation': changedFields.has('prize_pool') }">
          <span class="stat-value first-prize-value">{{ formattedFirstPrize }}</span>
          <span class="stat-label">{{ t('prizePreview.firstPrize') }}</span>
        </div>
        <div class="summary-divider"></div>
        <div class="summary-stat" :class="{ 'pulse-animation': changedFields.has('winners') }">
          <span class="stat-value">{{ prizeData.winners_count }}</span>
          <span class="stat-label">{{ t('prizePreview.winners') }}</span>
        </div>
      </div>

      <!-- Quorum Met: Show Prize Table -->
      <div v-if="showPrizeTable" class="prize-table-container">
        <table class="prize-table">
          <thead>
            <tr>
              <th>{{ t('prizePreview.rank') }}</th>
              <th class="text-right">{{ t('prizePreview.prize') }}</th>
              <th class="text-right">{{ t('prizePreview.share') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="prize in prizeData!.prizes"
              :key="prize.rank"
              :class="{ 'pulse-animation': changedFields.has(`prize_${prize.rank}`), 'top-rank': prize.rank <= 3 }"
            >
              <td class="rank-cell">
                <span v-if="getRankMedal(prize.rank)" class="rank-medal">{{ getRankMedal(prize.rank) }}</span>
                <span class="rank-label">{{ getRankLabel(prize.rank) }}</span>
              </td>
              <td class="amount-cell text-right">{{ formatCents(prize.amount_cents) }}</td>
              <td class="pct-cell text-right">{{ prize.percentage.toFixed(2) }}%</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Quorum Not Met: Warning -->
      <div v-else-if="prizeData && !prizeData.quorum_met" class="quorum-warning">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
          <line x1="12" y1="9" x2="12" y2="13" />
          <line x1="12" y1="17" x2="12.01" y2="17" />
        </svg>
        <h4>{{ t('prizePreview.quorumNotMet') }}</h4>
        <p>{{ t('prizePreview.quorumMessage', { min: prizeData.min_participants, current: prizeData.current_participants }) }}</p>
        <p class="refund-note">{{ t('prizePreview.autoRefund') }}</p>
      </div>

      <!-- Message -->
      <div v-if="prizeData" class="prize-message">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 16v-4M12 8h.01" />
        </svg>
        <span>{{ prizeData.message }}</span>
      </div>
    </div>

    <!-- Rules Tab -->
    <div v-else-if="activeTab === 'rules'" class="tab-content rules-content">
      <div class="rule-item">
        <div class="rule-icon">1</div>
        <div class="rule-text">{{ t('prizePreview.rule1') }}</div>
      </div>
      <div class="rule-item">
        <div class="rule-icon">2</div>
        <div class="rule-text">{{ t('prizePreview.rule2') }}</div>
      </div>
      <div class="rule-item">
        <div class="rule-icon">3</div>
        <div class="rule-text">{{ t('prizePreview.rule3') }}</div>
      </div>
      <div class="rule-item">
        <div class="rule-icon">4</div>
        <div class="rule-text">{{ t('prizePreview.rule4') }}</div>
      </div>
      <div v-if="prizeData" class="rule-item highlight">
        <div class="rule-icon">!</div>
        <div class="rule-text">
          {{ t('prizePreview.ruleCommission', { rate: prizeData.commission_rate || 20 }) }}
        </div>
      </div>
    </div>

    <!-- Participants Tab -->
    <div v-else-if="activeTab === 'participants'" class="tab-content">
      <div v-if="prizeData" class="participants-summary">
        <div class="participant-stat">
          <span class="stat-number" :class="{ 'pulse-animation': changedFields.has('participants') }">
            {{ prizeData.current_participants }}
          </span>
          <span class="stat-desc">{{ t('prizePreview.currentTraders') }}</span>
        </div>
        <div v-if="prizeData.min_participants > 1" class="participant-stat">
          <span class="stat-number">{{ prizeData.min_participants }}</span>
          <span class="stat-desc">{{ t('prizePreview.minimumRequired') }}</span>
        </div>
        <div class="participant-stat">
          <span class="stat-number">{{ prizeData.winners_count }}</span>
          <span class="stat-desc">{{ t('prizePreview.winnersSlots') }}</span>
        </div>
      </div>
      <div v-if="prizeData" class="quorum-status" :class="{ met: prizeData.quorum_met, unmet: !prizeData.quorum_met }">
        <svg v-if="prizeData.quorum_met" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
          <polyline points="22 4 12 14.01 9 11.01" />
        </svg>
        <svg v-else width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 8v4M12 16h.01" />
        </svg>
        <span>{{ prizeData.quorum_met ? t('prizePreview.quorumReached') : t('prizePreview.quorumPending') }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.prize-preview-table {
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

/* Tab Navigation */
.tab-nav {
  display: flex;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg-secondary);
}

.tab-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 12px 8px;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
  white-space: nowrap;
}

.tab-btn:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-tertiary);
}

.tab-btn.active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
}

.tab-btn svg {
  flex-shrink: 0;
}

.participant-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  background: var(--color-primary);
  color: white;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
}

/* Tab Content */
.tab-content {
  padding: var(--spacing-md);
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xl);
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
  to { transform: rotate(360deg); }
}

.error-state {
  text-align: center;
  color: var(--color-text-secondary);
}

.btn-retry {
  margin-top: var(--spacing-sm);
  padding: 6px 16px;
  background: var(--color-primary);
  color: white;
  border: none;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  cursor: pointer;
}

/* Summary Row */
.prize-summary-row {
  display: flex;
  align-items: center;
  justify-content: space-around;
  padding: var(--spacing-sm) 0;
  margin-bottom: var(--spacing-md);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.summary-stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 4px 8px;
  border-radius: var(--radius-sm);
  transition: background-color 0.3s ease;
}

.stat-value {
  font-size: var(--font-size-lg);
  font-weight: 700;
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
}

.prize-value {
  color: #059669;
}

.first-prize-value {
  color: #D97706;
}

.stat-label {
  font-size: 11px;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.summary-divider {
  width: 1px;
  height: 32px;
  background: var(--color-border);
}

/* Prize Table */
.prize-table-container {
  overflow-x: auto;
}

.prize-table {
  width: 100%;
  border-collapse: collapse;
}

.prize-table th {
  padding: 8px 12px;
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-bottom: 1px solid var(--color-border);
}

.prize-table td {
  padding: 10px 12px;
  font-size: var(--font-size-sm);
  border-bottom: 1px solid var(--color-border-light, rgba(0,0,0,0.06));
}

.prize-table tr:last-child td {
  border-bottom: none;
}

.prize-table tr.top-rank {
  background: var(--color-bg-secondary);
}

.text-right {
  text-align: right;
}

.rank-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.rank-medal {
  font-size: 1.1rem;
}

.rank-label {
  font-weight: 500;
  color: var(--color-text-primary);
}

.amount-cell {
  font-weight: 600;
  color: #059669;
  font-variant-numeric: tabular-nums;
}

.pct-cell {
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

/* Quorum Warning */
.quorum-warning {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-lg);
  text-align: center;
  background: #FEF3C7;
  border-radius: var(--radius-md);
}

.quorum-warning svg {
  color: #D97706;
}

.quorum-warning h4 {
  margin: 0;
  font-size: var(--font-size-md);
  font-weight: 600;
  color: #92400E;
}

.quorum-warning p {
  margin: 0;
  font-size: var(--font-size-sm);
  color: #78350F;
}

.refund-note {
  font-style: italic;
  opacity: 0.8;
}

/* Prize Message */
.prize-message {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  margin-top: var(--spacing-md);
  padding: var(--spacing-sm);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.prize-message svg {
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.prize-message span {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

/* Rules Content */
.rules-content {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.rule-item {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.rule-item.highlight {
  background: #FEF3C7;
}

.rule-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  min-width: 24px;
  background: var(--color-primary);
  color: white;
  border-radius: 50%;
  font-size: 12px;
  font-weight: 700;
}

.rule-item.highlight .rule-icon {
  background: #D97706;
}

.rule-text {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  line-height: 1.5;
}

/* Participants Summary */
.participants-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(100px, 1fr));
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-md);
}

.participant-stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: var(--spacing-md);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.stat-number {
  font-size: var(--font-size-2xl, 1.5rem);
  font-weight: 700;
  color: var(--color-text-primary);
}

.stat-desc {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-align: center;
}

.quorum-status {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.quorum-status.met {
  background: #D1FAE5;
  color: #065F46;
}

.quorum-status.unmet {
  background: #FEF3C7;
  color: #92400E;
}

/* Pulse Animation */
.pulse-animation {
  animation: pulse-highlight 1.5s ease-out;
}

@keyframes pulse-highlight {
  0% {
    background-color: rgba(59, 130, 246, 0.2);
  }
  50% {
    background-color: rgba(59, 130, 246, 0.1);
  }
  100% {
    background-color: transparent;
  }
}

/* Dark mode adjustments */
.quorum-warning {
  background: var(--color-warning-bg, #FEF3C7);
}

/* RTL Support */
[dir="rtl"] .tab-nav {
  flex-direction: row-reverse;
}

[dir="rtl"] .tab-btn {
  flex-direction: row-reverse;
}

[dir="rtl"] .rank-cell {
  flex-direction: row-reverse;
}

[dir="rtl"] .rule-item {
  flex-direction: row-reverse;
}

[dir="rtl"] .text-right {
  text-align: left;
}

[dir="rtl"] .prize-message {
  flex-direction: row-reverse;
}

/* Mobile */
@media (max-width: 767px) {
  .prize-summary-row {
    flex-wrap: wrap;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm);
  }

  .summary-divider {
    display: none;
  }

  .summary-stat {
    flex: 1;
    min-width: 60px;
  }

  .stat-value {
    font-size: var(--font-size-md);
  }

  .tab-btn {
    padding: 10px 4px;
    font-size: 12px;
    gap: 4px;
  }

  .tab-btn svg {
    width: 14px;
    height: 14px;
  }

  .prize-table th,
  .prize-table td {
    padding: 8px;
  }

  .participants-summary {
    grid-template-columns: 1fr 1fr;
  }
}
</style>
