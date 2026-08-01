<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { t } from '@/i18n';
import { useWalletStore } from '@/stores/wallet';
import type { WalletTransactionType, WalletTransaction } from '@/api';

const walletStore = useWalletStore();

// Tab state
type TabName = 'overview' | 'history';
const activeTab = ref<TabName>('overview');

// History filter state
const historyFilter = ref<WalletTransactionType | 'all'>('all');
const currentPage = ref(1);
const perPage = 10;

// Computed values
const formattedBalance = computed(() => {
  return walletStore.formattedBalance;
});

const recentTransactions = computed(() => {
  return walletStore.transactions.slice(0, 5);
});

const totalPages = computed(() => {
  return Math.ceil(walletStore.totalTransactions / perPage);
});

// Functions
function formatCurrency(amount: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: walletStore.currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amount);
}

function formatDate(dateString: string): string {
  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(dateString));
}

function getTransactionColor(amountCents: number): string {
  if (amountCents > 0) return 'positive';
  if (amountCents < 0) return 'negative';
  return '';
}

// Map V2 transaction types to icon/CSS categories
function getIconCategory(type: string): string {
  switch (type) {
    case 'deposit': return 'deposit';
    case 'withdrawal': return 'withdrawal';
    case 'prize_credit': return 'prize';
    case 'contest_entry': return 'entry';
    case 'contest_refund': return 'refund';
    case 'adjustment': return 'refund';
    default: return type;
  }
}

// Get CSS class for withdrawal status badge
function getWithdrawalStatusClass(tx: { type: string; status?: string }): string {
  if (tx.type !== 'withdrawal' || !tx.status) {
    return tx.status || 'pending';
  }
  switch (tx.status) {
    case 'pending':
      return 'pending';
    case 'processing':
      return 'processing';
    case 'succeeded':
    case 'completed':
      return 'completed';
    case 'rejected':
      return 'rejected';
    case 'failed':
      return 'failed';
    case 'cancelled':
      return 'cancelled';
    default:
      return tx.status;
  }
}

// Get display text for withdrawal status
function getWithdrawalStatusText(tx: { type: string; status?: string }): string {
  if (tx.type !== 'withdrawal' || !tx.status) {
    return t(`wallet.status.${tx.status || 'pending'}`);
  }
  switch (tx.status) {
    case 'pending':
      return t('wallet.status.pendingReview');
    case 'processing':
      return t('wallet.status.processing');
    case 'succeeded':
      return t('wallet.status.completed');
    case 'rejected':
      return t('wallet.status.rejected');
    case 'failed':
      return t('wallet.status.failed');
    case 'cancelled':
      return t('wallet.status.cancelled');
    default:
      return t(`wallet.status.${tx.status}`);
  }
}

// Get localized description for a transaction
function getTransactionDescription(tx: WalletTransaction): string {
  return tx.description || t(`wallet.types.${tx.type}`);
}

// Get display-friendly status text for any transaction type
function getStatusText(tx: { type: string; status?: string }): string {
  if (tx.type === 'withdrawal') {
    return getWithdrawalStatusText(tx);
  }
  const status = tx.status === 'succeeded' ? 'completed' : (tx.status || 'pending');
  return t(`wallet.status.${status}`);
}

// Get CSS class for any transaction status
function getStatusClass(tx: { type: string; status?: string }): string {
  if (tx.type === 'withdrawal') {
    return getWithdrawalStatusClass(tx);
  }
  const status = tx.status || 'pending';
  if (status === 'succeeded' || status === 'completed') return 'completed';
  return status;
}

async function loadTransactions(): Promise<void> {
  const type = historyFilter.value === 'all' ? undefined : historyFilter.value;
  const offset = (currentPage.value - 1) * perPage;
  await walletStore.fetchHistory({
    type,
    limit: perPage,
    offset,
  });
}

function goToPage(page: number): void {
  if (page >= 1 && page <= totalPages.value) {
    currentPage.value = page;
    loadTransactions();
  }
}

// Watch for filter changes
watch(historyFilter, () => {
  currentPage.value = 1;
  loadTransactions();
});

// Initialize
onMounted(async () => {
  await walletStore.initialize();
});
</script>

<template>
  <div class="wallet-page">
    <!-- Balance Card -->
    <div class="balance-card">
      <div class="balance-content">
        <div class="balance-main">
          <span class="balance-label">{{ t('wallet.availableBalance') }}</span>
          <h1 class="balance-amount">{{ formattedBalance }}</h1>
        </div>
        <div class="balance-actions">
          <button class="btn btn-primary" @click="walletStore.openDepositModal()">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="12" y1="5" x2="12" y2="19" />
              <polyline points="19 12 12 19 5 12" />
            </svg>
            {{ t('wallet.deposit') }}
          </button>
          <button class="btn btn-secondary" @click="walletStore.openWithdrawModal()">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="12" y1="19" x2="12" y2="5" />
              <polyline points="5 12 12 5 19 12" />
            </svg>
            {{ t('wallet.withdraw') }}
          </button>
        </div>
      </div>
      <div class="balance-decoration">
        <svg width="120" height="120" viewBox="0 0 120 120" fill="none">
          <circle cx="60" cy="60" r="50" stroke="currentColor" stroke-width="2" opacity="0.2" />
          <circle cx="60" cy="60" r="35" stroke="currentColor" stroke-width="2" opacity="0.15" />
          <circle cx="60" cy="60" r="20" stroke="currentColor" stroke-width="2" opacity="0.1" />
        </svg>
      </div>
    </div>

    <!-- Tab Navigation -->
    <div class="tabs">
      <button
        :class="['tab', { 'tab-active': activeTab === 'overview' }]"
        @click="activeTab = 'overview'"
      >
        {{ t('wallet.overview') }}
      </button>
      <button
        class="tab"
        @click="walletStore.openDepositModal()"
      >
        {{ t('wallet.deposit') }}
      </button>
      <button
        class="tab"
        @click="walletStore.openWithdrawModal()"
      >
        {{ t('wallet.withdraw') }}
      </button>
      <button
        :class="['tab', { 'tab-active': activeTab === 'history' }]"
        @click="activeTab = 'history'; loadTransactions()"
      >
        {{ t('wallet.historyTab') }}
      </button>
    </div>

    <!-- Tab Content -->
    <div class="tab-content">
      <!-- Overview Tab -->
      <div v-if="activeTab === 'overview'" class="overview-tab">
        <!-- Recent Transactions -->
        <div class="section">
          <div class="section-header">
            <h3 class="section-title">{{ t('wallet.recentTransactions') }}</h3>
            <button class="btn btn-ghost" @click="activeTab = 'history'; loadTransactions()">
              {{ t('wallet.viewAll') }}
            </button>
          </div>
          <div v-if="walletStore.transactionsLoading" class="loading-state">
            <div class="spinner"></div>
          </div>
          <div v-else-if="recentTransactions.length === 0" class="empty-state">
            <p>{{ t('wallet.noTransactions') }}</p>
          </div>
          <div v-else class="transaction-list">
            <div
              v-for="tx in recentTransactions"
              :key="tx.id"
              class="transaction-item"
            >
              <div class="transaction-icon" :class="getIconCategory(tx.type)">
                <svg v-if="getIconCategory(tx.type) === 'deposit'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="12" y1="5" x2="12" y2="19" />
                  <polyline points="19 12 12 19 5 12" />
                </svg>
                <svg v-else-if="getIconCategory(tx.type) === 'withdrawal'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="12" y1="19" x2="12" y2="5" />
                  <polyline points="5 12 12 5 19 12" />
                </svg>
                <svg v-else-if="getIconCategory(tx.type) === 'prize'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6" />
                  <path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18" />
                  <path d="M4 22h16" />
                  <path d="M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20 7 22h10c0-2-1-3-2-3.5-.5-.23-1-.66-1-1.21v-2.63" />
                </svg>
                <svg v-else-if="getIconCategory(tx.type) === 'entry'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M2 12h6a2 2 0 0 1 2 2v6a2 2 0 0 1-2 2H2" />
                  <path d="M22 12h-6a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h6" />
                  <path d="M22 2H2v10h20V2z" />
                </svg>
                <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="12" cy="12" r="10" />
                </svg>
              </div>
              <div class="transaction-info">
                <span class="transaction-type">{{ t(`wallet.types.${tx.type}`) }}</span>
                <span class="transaction-desc">{{ getTransactionDescription(tx) }}</span>
              </div>
              <div class="transaction-meta">
                <span :class="['transaction-amount', getTransactionColor(tx.amount_cents)]">
                  {{ tx.amount_cents > 0 ? '+' : '' }}{{ formatCurrency(tx.amount_cents / 100) }}
                </span>
                <span class="transaction-date">{{ formatDate(tx.created_at) }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Quick Actions -->
        <div class="quick-actions">
          <button class="quick-action-btn" @click="walletStore.openDepositModal()">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="18" height="18" rx="2" />
              <line x1="12" y1="8" x2="12" y2="16" />
              <line x1="8" y1="12" x2="16" y2="12" />
            </svg>
            <span>{{ t('wallet.addFunds') }}</span>
          </button>
          <button class="quick-action-btn" @click="activeTab = 'history'">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <polyline points="12 6 12 12 16 14" />
            </svg>
            <span>{{ t('wallet.viewHistory') }}</span>
          </button>
        </div>
      </div>

      <!-- History Tab -->
      <div v-if="activeTab === 'history'" class="history-tab">
        <!-- Filters -->
        <div class="history-filters">
          <button
            :class="['filter-btn', { 'active': historyFilter === 'all' }]"
            @click="historyFilter = 'all'"
          >
            {{ t('wallet.allTransactions') }}
          </button>
          <button
            :class="['filter-btn', { 'active': historyFilter === 'deposit' }]"
            @click="historyFilter = 'deposit'"
          >
            {{ t('wallet.deposits') }}
          </button>
          <button
            :class="['filter-btn', { 'active': historyFilter === 'withdrawal' }]"
            @click="historyFilter = 'withdrawal'"
          >
            {{ t('wallet.withdrawals') }}
          </button>
          <button
            :class="['filter-btn', { 'active': historyFilter === 'prize_credit' }]"
            @click="historyFilter = 'prize_credit'"
          >
            {{ t('wallet.prizes') }}
          </button>
          <button
            :class="['filter-btn', { 'active': historyFilter === 'contest_entry' }]"
            @click="historyFilter = 'contest_entry'"
          >
            {{ t('wallet.entries') }}
          </button>
        </div>

        <!-- Transaction List -->
        <div v-if="walletStore.transactionsLoading" class="loading-state">
          <div class="spinner"></div>
          <p>{{ t('common.loading') }}</p>
        </div>
        <div v-else-if="walletStore.transactions.length === 0" class="empty-state">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
            <polyline points="14 2 14 8 20 8" />
            <line x1="16" y1="13" x2="8" y2="13" />
            <line x1="16" y1="17" x2="8" y2="17" />
            <polyline points="10 9 9 9 8 9" />
          </svg>
          <p>{{ t('wallet.noTransactionsFound') }}</p>
        </div>
        <div v-else class="transaction-list full-list">
          <div
            v-for="tx in walletStore.transactions"
            :key="tx.id"
            class="transaction-wrapper"
          >
            <div class="transaction-item">
              <div class="transaction-icon" :class="getIconCategory(tx.type)">
                <svg v-if="getIconCategory(tx.type) === 'deposit'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="12" y1="5" x2="12" y2="19" />
                  <polyline points="19 12 12 19 5 12" />
                </svg>
                <svg v-else-if="getIconCategory(tx.type) === 'withdrawal'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="12" y1="19" x2="12" y2="5" />
                  <polyline points="5 12 12 5 19 12" />
                </svg>
                <svg v-else-if="getIconCategory(tx.type) === 'prize'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6" />
                  <path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18" />
                  <path d="M4 22h16" />
                  <path d="M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20 7 22h10c0-2-1-3-2-3.5-.5-.23-1-.66-1-1.21v-2.63" />
                </svg>
                <svg v-else-if="getIconCategory(tx.type) === 'entry'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M2 12h6a2 2 0 0 1 2 2v6a2 2 0 0 1-2 2H2" />
                  <path d="M22 12h-6a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h6" />
                  <path d="M22 2H2v10h20V2z" />
                </svg>
                <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <polyline points="23 4 23 10 17 10" />
                  <polyline points="1 20 1 14 7 14" />
                  <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
                </svg>
              </div>
              <div class="transaction-info">
                <span class="transaction-type">{{ t(`wallet.types.${tx.type}`) }}</span>
                <span class="transaction-desc">{{ getTransactionDescription(tx) }}</span>
              </div>
              <div class="transaction-meta">
                <span :class="['transaction-amount', getTransactionColor(tx.amount_cents)]">
                  {{ tx.amount_cents > 0 ? '+' : '' }}{{ formatCurrency(tx.amount_cents / 100) }}
                </span>
                <span class="transaction-date">{{ formatDate(tx.created_at) }}</span>
                <span :class="['transaction-status', getStatusClass(tx)]">
                  {{ getStatusText(tx) }}
                </span>
              </div>
            </div>
            <!-- Estimated review time for pending withdrawals -->
            <div v-if="tx.type === 'withdrawal' && tx.status === 'pending'" class="withdrawal-pending-notice">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <polyline points="12 6 12 12 16 14" />
              </svg>
              <span>{{ t('wallet.withdrawalPendingReview') }}</span>
            </div>
            <!-- Admin comment for rejected/failed withdrawals -->
            <div v-if="tx.type === 'withdrawal' && (tx.status === 'rejected' || tx.status === 'failed') && tx.admin_comment" class="withdrawal-reason">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <line x1="12" y1="16" x2="12" y2="12" />
                <line x1="12" y1="8" x2="12.01" y2="8" />
              </svg>
              <span>{{ t('wallet.withdrawalReason') }}: {{ tx.admin_comment }}</span>
            </div>
          </div>
        </div>

        <!-- Pagination -->
        <div v-if="totalPages > 1" class="pagination">
          <button
            class="pagination-btn"
            :disabled="currentPage === 1"
            @click="goToPage(currentPage - 1)"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="15 18 9 12 15 6" />
            </svg>
          </button>
          <span class="pagination-info">
            {{ currentPage }} / {{ totalPages }}
          </span>
          <button
            class="pagination-btn"
            :disabled="currentPage === totalPages"
            @click="goToPage(currentPage + 1)"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="9 18 15 12 9 6" />
            </svg>
          </button>
        </div>
      </div>
    </div>

  </div>
</template>

<style scoped>
.wallet-page {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

/* Balance Card */
.balance-card {
  background: linear-gradient(135deg, var(--color-primary) 0%, #1D4ED8 100%);
  border-radius: var(--radius-xl);
  padding: var(--spacing-xl);
  color: white;
  position: relative;
  overflow: hidden;
}

.balance-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
  position: relative;
  z-index: 1;
  flex-wrap: wrap;
  gap: var(--spacing-lg);
}

.balance-main {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.balance-label {
  font-size: var(--font-size-sm);
  opacity: 0.9;
}

.balance-amount {
  font-size: var(--font-size-4xl);
  font-weight: 700;
  margin: 0;
}

.balance-actions {
  display: flex;
  gap: var(--spacing-md);
}

.balance-actions .btn {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.balance-actions .btn-primary {
  background-color: white;
  color: var(--color-primary);
}

.balance-actions .btn-primary:hover {
  background-color: rgba(255, 255, 255, 0.9);
}

.balance-actions .btn-secondary {
  background-color: rgba(255, 255, 255, 0.2);
  color: white;
  border: 1px solid rgba(255, 255, 255, 0.3);
}

.balance-actions .btn-secondary:hover {
  background-color: rgba(255, 255, 255, 0.3);
}

.balance-decoration {
  position: absolute;
  right: -20px;
  top: 50%;
  transform: translateY(-50%);
  opacity: 0.5;
  color: white;
}

[dir="rtl"] .balance-decoration {
  right: auto;
  left: -20px;
}

/* Tabs */
.tabs {
  display: flex;
  gap: var(--spacing-xs);
  background-color: var(--color-bg-secondary);
  padding: var(--spacing-xs);
  border-radius: var(--radius-lg);
  overflow-x: auto;
}

.tab {
  flex: 1;
  padding: var(--spacing-sm) var(--spacing-md);
  border: none;
  background: none;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
  white-space: nowrap;
}

.tab:hover {
  color: var(--color-text-primary);
}

.tab-active {
  background-color: var(--color-bg-primary);
  color: var(--color-primary);
  box-shadow: var(--shadow-sm);
}

.tab-content {
  min-height: 400px;
}

/* Section */
.section {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-md);
}

.section-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  margin: 0;
}

/* Transaction List */
.transaction-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.transaction-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.transaction-icon {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-full);
  flex-shrink: 0;
}

.transaction-icon.deposit {
  background-color: #DBEAFE;
  color: #2563EB;
}

.transaction-icon.withdrawal {
  background-color: #FEE2E2;
  color: #DC2626;
}

.transaction-icon.prize {
  background-color: #D1FAE5;
  color: #059669;
}

.transaction-icon.entry {
  background-color: #FEF3C7;
  color: #D97706;
}

.transaction-icon.refund {
  background-color: #E0E7FF;
  color: #4F46E5;
}

.transaction-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  min-width: 0;
}

.transaction-type {
  font-weight: 500;
  color: var(--color-text-primary);
}

.transaction-desc {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.transaction-meta {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--spacing-xs);
}

[dir="rtl"] .transaction-meta {
  align-items: flex-start;
}

.transaction-amount {
  font-weight: 600;
}

.transaction-amount.positive {
  color: var(--color-success);
}

.transaction-amount.negative {
  color: var(--color-danger);
}

.transaction-date {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.transaction-status {
  font-size: var(--font-size-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
}

.transaction-status.pending {
  background-color: #FEF3C7;
  color: #D97706;
}

.transaction-status.completed {
  background-color: #D1FAE5;
  color: #059669;
}

.transaction-status.failed {
  background-color: #FEE2E2;
  color: #DC2626;
}

.transaction-status.cancelled {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-muted);
}

.transaction-status.processing {
  background-color: #DBEAFE;
  color: #2563EB;
}

.transaction-status.rejected {
  background-color: #FEE2E2;
  color: #DC2626;
}

/* Transaction wrapper for withdrawals with notices */
.transaction-wrapper {
  display: flex;
  flex-direction: column;
  gap: 0;
}

/* Withdrawal pending review notice */
.withdrawal-pending-notice {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  padding-left: calc(40px + var(--spacing-md) + var(--spacing-md));
  background-color: #FEF9E7;
  border-radius: 0 0 var(--radius-md) var(--radius-md);
  margin-top: -1px;
  font-size: var(--font-size-xs);
  color: #92400E;
}

.withdrawal-pending-notice svg {
  flex-shrink: 0;
  color: #D97706;
}

[dir="rtl"] .withdrawal-pending-notice {
  padding-left: var(--spacing-md);
  padding-right: calc(40px + var(--spacing-md) + var(--spacing-md));
}

/* Withdrawal rejection/failure reason */
.withdrawal-reason {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  padding-left: calc(40px + var(--spacing-md) + var(--spacing-md));
  background-color: #FEF2F2;
  border-radius: 0 0 var(--radius-md) var(--radius-md);
  margin-top: -1px;
  font-size: var(--font-size-xs);
  color: #991B1B;
}

.withdrawal-reason svg {
  flex-shrink: 0;
  margin-top: 1px;
  color: #DC2626;
}

[dir="rtl"] .withdrawal-reason {
  padding-left: var(--spacing-md);
  padding-right: calc(40px + var(--spacing-md) + var(--spacing-md));
}

/* Dark mode support for withdrawal notices */
:root[data-theme="dark"] .withdrawal-pending-notice {
  background-color: rgba(217, 119, 6, 0.15);
  color: #FCD34D;
}

:root[data-theme="dark"] .withdrawal-pending-notice svg {
  color: #FBBF24;
}

:root[data-theme="dark"] .withdrawal-reason {
  background-color: rgba(220, 38, 38, 0.15);
  color: #FCA5A5;
}

:root[data-theme="dark"] .withdrawal-reason svg {
  color: #F87171;
}

:root[data-theme="dark"] .transaction-status.processing {
  background-color: rgba(37, 99, 235, 0.2);
  color: #93C5FD;
}

:root[data-theme="dark"] .transaction-status.pending {
  background-color: rgba(217, 119, 6, 0.2);
  color: #FCD34D;
}

:root[data-theme="dark"] .transaction-status.rejected {
  background-color: rgba(220, 38, 38, 0.2);
  color: #FCA5A5;
}

:root[data-theme="dark"] .transaction-status.failed {
  background-color: rgba(220, 38, 38, 0.2);
  color: #FCA5A5;
}

:root[data-theme="dark"] .transaction-status.completed {
  background-color: rgba(5, 150, 105, 0.2);
  color: #6EE7B7;
}

/* Quick Actions */
.quick-actions {
  display: flex;
  gap: var(--spacing-md);
  margin-top: var(--spacing-lg);
}

.quick-action-btn {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-lg);
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  cursor: pointer;
  transition: all var(--transition-fast);
  color: var(--color-text-secondary);
}

.quick-action-btn:hover {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

/* History Filters */
.history-filters {
  display: flex;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-lg);
  flex-wrap: wrap;
}

.filter-btn {
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-full);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.filter-btn:hover {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.filter-btn.active {
  background-color: var(--color-primary);
  border-color: var(--color-primary);
  color: white;
}

.full-list {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-md);
}

/* Pagination */
.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: var(--spacing-md);
  margin-top: var(--spacing-lg);
}

.pagination-btn {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.pagination-btn:hover:not(:disabled) {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.pagination-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.pagination-info {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

/* Loading & Empty States */
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
  width: 40px;
  height: 40px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

.btn-spinner {
  width: 20px;
  height: 20px;
  border: 2px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  display: inline-block;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Button Large */
.btn-lg {
  width: 100%;
  padding: var(--spacing-md) var(--spacing-lg);
  font-size: var(--font-size-md);
}

/* Responsive */
@media (max-width: 767px) {
  .balance-card {
    padding: var(--spacing-lg);
  }

  .balance-amount {
    font-size: var(--font-size-2xl);
  }

  .balance-content {
    flex-direction: column;
    align-items: flex-start;
  }

  .balance-actions {
    width: 100%;
    flex-direction: column;
  }

  .balance-actions .btn {
    width: 100%;
    justify-content: center;
  }

  .balance-decoration {
    display: none;
  }

  .tabs {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }

  .tab {
    flex: 0 0 auto;
    padding: var(--spacing-sm) var(--spacing-lg);
  }

  .quick-actions {
    flex-direction: column;
  }

  .history-filters {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    flex-wrap: nowrap;
    padding-bottom: var(--spacing-sm);
  }

  .filter-btn {
    flex-shrink: 0;
  }

  .transaction-item {
    flex-wrap: wrap;
  }

  .transaction-info {
    flex: 0 0 calc(100% - 56px);
  }

  .transaction-meta {
    flex: 0 0 100%;
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
    margin-top: var(--spacing-sm);
    padding-top: var(--spacing-sm);
    border-top: 1px solid var(--color-border);
  }

  [dir="rtl"] .transaction-meta {
    flex-direction: row-reverse;
  }
}
</style>
