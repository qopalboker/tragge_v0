<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useWalletStore } from '@/stores/wallet';
import type { WalletTransactionType, WalletTransaction } from '@/api';

const router = useRouter();
const walletStore = useWalletStore();

// Filter state
const activeFilter = ref<WalletTransactionType | 'all'>('all');
const currentPage = ref(1);
const perPage = 20;

// Computed
const filteredTransactions = computed(() => {
  if (activeFilter.value === 'all') {
    return walletStore.transactions;
  }
  return walletStore.transactions.filter(tx => tx.type === activeFilter.value);
});

const paginatedTransactions = computed(() => {
  return filteredTransactions.value;
});

// Filter options
const filters: { value: WalletTransactionType | 'all'; labelKey: string }[] = [
  { value: 'all', labelKey: 'wallet.history.filterAll' },
  { value: 'deposit', labelKey: 'wallet.history.filterDeposits' },
  { value: 'withdrawal', labelKey: 'wallet.history.filterWithdrawals' },
  { value: 'prize_credit', labelKey: 'wallet.history.filterPrizes' },
  { value: 'contest_entry', labelKey: 'wallet.history.filterEntries' },
  { value: 'contest_refund', labelKey: 'wallet.history.filterRefunds' },
];

// Methods
function getTransactionColor(tx: WalletTransaction): string {
  if (tx.amount_cents > 0) return 'credit';
  return 'debit';
}

function getTransactionTypeClass(tx: WalletTransaction): string {
  switch (tx.type) {
    case 'deposit': return 'type-deposit';
    case 'withdrawal': return 'type-withdrawal';
    case 'prize_credit': return 'type-prize';
    case 'contest_entry': return 'type-entry';
    case 'contest_refund': return 'type-refund';
    default: return 'type-default';
  }
}

function getReasonCodeLabel(tx: WalletTransaction): string {
  if (tx.reason_code) {
    const key = `wallet.reasonCodes.${tx.reason_code}`;
    const translated = t(key);
    if (translated !== key) return translated;
  }
  if (tx.description) return tx.description;
  return t(`wallet.types.${tx.type}`);
}

function formatAmount(cents: number): string {
  const amount = Math.abs(cents) / 100;
  const formatted = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
  }).format(amount);
  return cents > 0 ? `+${formatted}` : `-${formatted}`;
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function navigateToContest(tx: WalletTransaction): void {
  if (tx.ref_type === 'contest' && tx.ref_id) {
    router.push(`/user/contests/${tx.ref_id}`);
  }
}

function isContestRelated(tx: WalletTransaction): boolean {
  return tx.ref_type === 'contest' && !!tx.ref_id;
}

async function applyFilter(filter: WalletTransactionType | 'all'): Promise<void> {
  activeFilter.value = filter;
  currentPage.value = 1;
  await walletStore.fetchHistory({
    limit: perPage,
    offset: 0,
    type: filter === 'all' ? undefined : filter,
  });
}

async function loadMore(): Promise<void> {
  await walletStore.loadMoreTransactions({
    limit: perPage,
    type: activeFilter.value === 'all' ? undefined : activeFilter.value,
  });
}

onMounted(async () => {
  await walletStore.fetchHistory({ limit: perPage, offset: 0 });
});
</script>

<template>
  <div class="wallet-history">
    <!-- Balance Header -->
    <div class="balance-header">
      <div class="balance-amount">
        <span class="balance-label">{{ t('wallet.availableBalance') }}</span>
        <span class="balance-value">{{ walletStore.formattedBalance }}</span>
      </div>
    </div>

    <!-- Filter Tabs -->
    <div class="filter-tabs">
      <button
        v-for="filter in filters"
        :key="filter.value"
        :class="['filter-tab', { active: activeFilter === filter.value }]"
        @click="applyFilter(filter.value)"
      >
        {{ t(filter.labelKey) }}
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="walletStore.transactionsLoading && paginatedTransactions.length === 0" class="loading-state">
      <div class="spinner"></div>
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Empty State -->
    <div v-else-if="paginatedTransactions.length === 0" class="empty-state">
      <p>{{ t('wallet.noTransactions') }}</p>
    </div>

    <!-- Transaction List -->
    <div v-else class="transaction-list">
      <div
        v-for="tx in paginatedTransactions"
        :key="tx.id"
        :class="['transaction-item', { clickable: isContestRelated(tx) }]"
        @click="isContestRelated(tx) && navigateToContest(tx)"
      >
        <!-- Icon -->
        <div :class="['tx-icon', getTransactionTypeClass(tx)]">
          <span v-if="tx.type === 'deposit'" class="icon-char">&#8595;</span>
          <span v-else-if="tx.type === 'withdrawal'" class="icon-char">&#8593;</span>
          <span v-else-if="tx.type === 'prize_credit'" class="icon-char">&#9733;</span>
          <span v-else-if="tx.type === 'contest_entry'" class="icon-char">&#9654;</span>
          <span v-else-if="tx.type === 'contest_refund'" class="icon-char">&#8634;</span>
          <span v-else class="icon-char">&#8226;</span>
        </div>

        <!-- Details -->
        <div class="tx-details">
          <div class="tx-reason">{{ getReasonCodeLabel(tx) }}</div>
          <div class="tx-date">{{ formatDate(tx.created_at) }}</div>
        </div>

        <!-- Amount -->
        <div :class="['tx-amount', getTransactionColor(tx)]">
          {{ formatAmount(tx.amount_cents) }}
        </div>
      </div>
    </div>

    <!-- Load More / Pagination -->
    <div v-if="walletStore.hasMoreTransactions" class="load-more">
      <button
        class="load-more-btn"
        :disabled="walletStore.transactionsLoading"
        @click="loadMore"
      >
        <span v-if="walletStore.transactionsLoading" class="spinner-small"></span>
        <span v-else>{{ t('common.loadMore') }}</span>
      </button>
    </div>

    <!-- Page Info -->
    <div v-if="walletStore.totalTransactions > 0" class="page-info">
      {{ t('wallet.history.showing', {
        count: paginatedTransactions.length,
        total: walletStore.totalTransactions
      }) }}
    </div>
  </div>
</template>

<style scoped>
.wallet-history {
  max-width: 700px;
  margin: 0 auto;
}

.balance-header {
  background: linear-gradient(135deg, var(--color-primary, #6366f1), #8b5cf6);
  border-radius: 12px;
  padding: 24px;
  margin-bottom: 20px;
  color: #fff;
}

.balance-label {
  display: block;
  font-size: 0.875rem;
  opacity: 0.85;
  margin-bottom: 4px;
}

.balance-value {
  display: block;
  font-size: 2rem;
  font-weight: 700;
}

.filter-tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
  overflow-x: auto;
  padding-bottom: 4px;
}

.filter-tab {
  padding: 6px 14px;
  border-radius: 20px;
  border: 1px solid var(--color-border, #e5e7eb);
  background: var(--color-surface, #fff);
  color: var(--color-text-secondary, #6b7280);
  font-size: 0.8125rem;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}

.filter-tab:hover {
  border-color: var(--color-primary, #6366f1);
  color: var(--color-primary, #6366f1);
}

.filter-tab.active {
  background: var(--color-primary, #6366f1);
  color: #fff;
  border-color: var(--color-primary, #6366f1);
}

.loading-state,
.empty-state {
  text-align: center;
  padding: 48px 16px;
  color: var(--color-text-secondary, #6b7280);
}

.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--color-border, #e5e7eb);
  border-top-color: var(--color-primary, #6366f1);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin: 0 auto 12px;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.transaction-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.transaction-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  background: var(--color-surface, #fff);
  border-radius: 8px;
  transition: background 0.15s;
}

.transaction-item.clickable {
  cursor: pointer;
}

.transaction-item.clickable:hover {
  background: var(--color-surface-hover, #f9fafb);
}

.tx-icon {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  font-size: 1.125rem;
}

.tx-icon.type-deposit {
  background: rgba(37, 99, 235, 0.1);
  color: #2563eb;
}

.tx-icon.type-withdrawal {
  background: rgba(220, 38, 38, 0.1);
  color: #dc2626;
}

.tx-icon.type-prize {
  background: rgba(234, 179, 8, 0.1);
  color: #ca8a04;
}

.tx-icon.type-entry {
  background: rgba(217, 119, 6, 0.1);
  color: #d97706;
}

.tx-icon.type-refund {
  background: rgba(79, 70, 229, 0.1);
  color: #4f46e5;
}

.tx-icon.type-default {
  background: rgba(107, 114, 128, 0.1);
  color: #6b7280;
}

.icon-char {
  font-size: 1.125rem;
  line-height: 1;
}

.tx-details {
  flex: 1;
  min-width: 0;
}

.tx-reason {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--color-text-primary, #111827);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tx-date {
  font-size: 0.75rem;
  color: var(--color-text-secondary, #6b7280);
  margin-top: 2px;
}

.tx-amount {
  font-size: 0.9375rem;
  font-weight: 600;
  white-space: nowrap;
}

.tx-amount.credit {
  color: #059669;
}

.tx-amount.debit {
  color: #dc2626;
}

.load-more {
  text-align: center;
  padding: 16px;
}

.load-more-btn {
  padding: 10px 24px;
  border-radius: 8px;
  border: 1px solid var(--color-border, #e5e7eb);
  background: var(--color-surface, #fff);
  color: var(--color-text-primary, #111827);
  cursor: pointer;
  font-size: 0.875rem;
  transition: all 0.2s;
}

.load-more-btn:hover:not(:disabled) {
  border-color: var(--color-primary, #6366f1);
  color: var(--color-primary, #6366f1);
}

.load-more-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.spinner-small {
  display: inline-block;
  width: 16px;
  height: 16px;
  border: 2px solid var(--color-border, #e5e7eb);
  border-top-color: var(--color-primary, #6366f1);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.page-info {
  text-align: center;
  padding: 12px;
  font-size: 0.75rem;
  color: var(--color-text-secondary, #6b7280);
}

/* RTL Support */
[dir="rtl"] .transaction-item {
  flex-direction: row-reverse;
}

[dir="rtl"] .tx-details {
  text-align: right;
}

[dir="rtl"] .tx-amount {
  text-align: left;
}

/* Dark mode */
@media (prefers-color-scheme: dark) {
  .transaction-item {
    background: var(--color-surface-dark, #1f2937);
  }

  .transaction-item.clickable:hover {
    background: var(--color-surface-hover-dark, #374151);
  }
}
</style>
