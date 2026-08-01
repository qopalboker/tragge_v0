<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { getDashboardMetrics, type DashboardMetrics } from '@/api/dashboard';

const router = useRouter();
const auth = useAuthStore();

const metrics = ref<DashboardMetrics | null>(null);
const loading = ref(true);
const error = ref(false);
let refreshInterval: ReturnType<typeof setInterval> | null = null;

// Permission checks for navigation
const canViewUsers = computed(() => auth.hasPermission('users.view'));
const canViewContests = computed(() => auth.hasPermission('contests.view'));
const canViewWithdrawals = computed(() => auth.hasPermission('withdrawals.view'));
const canViewKYC = computed(() => auth.hasPermission('kyc.view'));

// Format currency from cents to dollars
function formatCurrency(cents: number): string {
  const dollars = cents / 100;
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(dollars);
}

// Format large numbers with commas
function formatNumber(num: number): string {
  return new Intl.NumberFormat('en-US').format(num);
}

async function fetchMetrics(): Promise<void> {
  try {
    metrics.value = await getDashboardMetrics();
    error.value = false;
  } catch (e) {
    if (!metrics.value) {
      error.value = true;
    }
    console.error('Failed to fetch dashboard metrics:', e);
  } finally {
    loading.value = false;
  }
}

function navigateTo(path: string): void {
  router.push(path);
}

onMounted(() => {
  fetchMetrics();
  // Refresh metrics every 60 seconds
  refreshInterval = setInterval(fetchMetrics, 60000);
});

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval);
  }
});
</script>

<template>
  <div class="dashboard-page">
    <div class="page-header">
      <h1 class="page-title">{{ t('dashboard.title') }}</h1>
      <button class="refresh-btn" @click="fetchMetrics" :disabled="loading">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8" />
          <path d="M21 3v5h-5" />
        </svg>
        {{ t('dashboard.refresh') }}
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="loading && !metrics" class="loading-state">
      <div class="spinner"></div>
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Error State -->
    <div v-else-if="error && !metrics" class="error-state">
      <p>{{ t('dashboard.loadError') }}</p>
      <button @click="fetchMetrics">{{ t('common.retry') }}</button>
    </div>

    <!-- Metrics Grid -->
    <div v-else-if="metrics" class="metrics-grid">
      <!-- Users Section -->
      <section class="metrics-section">
        <h2 class="section-title">{{ t('dashboard.sections.users') }}</h2>
        <div class="cards-grid">
          <div
            class="metric-card clickable"
            @click="canViewUsers && navigateTo('/admin/users')"
            :class="{ disabled: !canViewUsers }"
          >
            <div class="metric-icon users">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.users.total) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.totalUsers') }}</span>
            </div>
          </div>

          <div class="metric-card">
            <div class="metric-icon new-users">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <line x1="19" y1="8" x2="19" y2="14" />
                <line x1="22" y1="11" x2="16" y2="11" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.users.new_today) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.newUsersToday') }}</span>
            </div>
          </div>

          <div class="metric-card">
            <div class="metric-icon verified">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
                <polyline points="22 4 12 14.01 9 11.01" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.users.verified_count) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.verifiedUsers') }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- Contests Section -->
      <section class="metrics-section">
        <h2 class="section-title">{{ t('dashboard.sections.contests') }}</h2>
        <div class="cards-grid">
          <div
            class="metric-card clickable"
            @click="canViewContests && navigateTo('/admin/contests')"
            :class="{ disabled: !canViewContests }"
          >
            <div class="metric-icon contests">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6" />
                <path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18" />
                <path d="M4 22h16" />
                <path d="M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22" />
                <path d="M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22" />
                <path d="M18 2H6v7a6 6 0 0 0 12 0V2Z" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.contests.total) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.totalContests') }}</span>
            </div>
          </div>

          <div class="metric-card highlight-green">
            <div class="metric-icon active">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <polygon points="10 8 16 12 10 16 10 8" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.contests.active_now) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.activeContests') }}</span>
            </div>
          </div>

          <div class="metric-card">
            <div class="metric-icon scheduled">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
                <line x1="16" y1="2" x2="16" y2="6" />
                <line x1="8" y1="2" x2="8" y2="6" />
                <line x1="3" y1="10" x2="21" y2="10" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.contests.scheduled) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.scheduledContests') }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- Financial Section -->
      <section class="metrics-section">
        <h2 class="section-title">{{ t('dashboard.sections.financial') }}</h2>
        <div class="cards-grid">
          <div class="metric-card">
            <div class="metric-icon deposits">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="12" y1="1" x2="12" y2="23" />
                <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatCurrency(metrics.financial.total_deposits_today_cents) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.depositsToday') }}</span>
            </div>
          </div>

          <div
            class="metric-card clickable"
            :class="{
              'highlight-orange': metrics.financial.pending_withdrawals_count > 0,
              disabled: !canViewWithdrawals
            }"
            @click="canViewWithdrawals && navigateTo('/admin/withdrawals')"
          >
            <div class="metric-icon withdrawals">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="1" y="4" width="22" height="16" rx="2" ry="2" />
                <line x1="1" y1="10" x2="23" y2="10" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.financial.pending_withdrawals_count) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.pendingWithdrawals') }}</span>
              <span v-if="metrics.financial.pending_withdrawals_count > 0" class="badge warning">
                {{ t('dashboard.actionRequired') }}
              </span>
            </div>
          </div>

          <div class="metric-card">
            <div class="metric-icon revenue">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="12" y1="1" x2="12" y2="23" />
                <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatCurrency(metrics.financial.total_revenue_cents) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.totalRevenue') }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- Trading Section -->
      <section class="metrics-section">
        <h2 class="section-title">{{ t('dashboard.sections.trading') }}</h2>
        <div class="cards-grid">
          <div class="metric-card">
            <div class="metric-icon traders">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.trading.active_traders_now) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.activeTraders') }}</span>
            </div>
          </div>

          <div class="metric-card">
            <div class="metric-icon orders">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                <polyline points="14 2 14 8 20 8" />
                <line x1="16" y1="13" x2="8" y2="13" />
                <line x1="16" y1="17" x2="8" y2="17" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.trading.orders_today) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.ordersToday') }}</span>
            </div>
          </div>

          <div class="metric-card">
            <div class="metric-icon trades">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="17 1 21 5 17 9" />
                <path d="M3 11V9a4 4 0 0 1 4-4h14" />
                <polyline points="7 23 3 19 7 15" />
                <path d="M21 13v2a4 4 0 0 1-4 4H3" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.trading.trades_today) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.tradesToday') }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- Pending Actions Section -->
      <section class="metrics-section">
        <h2 class="section-title">{{ t('dashboard.sections.pendingActions') }}</h2>
        <div class="cards-grid">
          <div
            class="metric-card clickable"
            :class="{
              'highlight-red': metrics.kyc.pending_count > 0,
              disabled: !canViewKYC
            }"
            @click="canViewKYC && navigateTo('/admin/kyc-review')"
          >
            <div class="metric-icon kyc">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M20 8v6" />
                <path d="M23 11h-6" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.kyc.pending_count) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.pendingKYC') }}</span>
              <span v-if="metrics.kyc.pending_count > 0" class="badge danger">
                {{ t('dashboard.reviewRequired') }}
              </span>
            </div>
          </div>

          <div class="metric-card" :class="{ 'highlight-orange': metrics.affiliate.pending_activation_count > 0 }">
            <div class="metric-icon affiliate">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
              </svg>
            </div>
            <div class="metric-content">
              <span class="metric-value">{{ formatNumber(metrics.affiliate.pending_activation_count) }}</span>
              <span class="metric-label">{{ t('dashboard.metrics.pendingAffiliates') }}</span>
              <span v-if="metrics.affiliate.pending_activation_count > 0" class="badge warning">
                {{ t('dashboard.actionRequired') }}
              </span>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.dashboard-page {
  padding: var(--spacing-lg);
  max-width: 1400px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xl);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0;
}

.refresh-btn {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.refresh-btn:hover:not(:disabled) {
  background: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.refresh-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.loading-state,
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 400px;
  gap: var(--spacing-md);
  color: var(--color-text-secondary);
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.metrics-grid {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xl);
}

.metrics-section {
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
}

.section-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-lg) 0;
}

.cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: var(--spacing-md);
}

.metric-card {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  transition: all var(--transition-fast);
}

.metric-card.clickable {
  cursor: pointer;
}

.metric-card.clickable:hover:not(.disabled) {
  border-color: var(--color-primary);
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.metric-card.disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.metric-card.highlight-green {
  border-color: var(--color-success, #10b981);
  background: rgba(16, 185, 129, 0.05);
}

.metric-card.highlight-orange {
  border-color: var(--color-warning, #f59e0b);
  background: rgba(245, 158, 11, 0.05);
}

.metric-card.highlight-red {
  border-color: var(--color-error, #ef4444);
  background: rgba(239, 68, 68, 0.05);
}

.metric-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: var(--radius-md);
  flex-shrink: 0;
}

.metric-icon.users { background: rgba(99, 102, 241, 0.1); color: #6366f1; }
.metric-icon.new-users { background: rgba(16, 185, 129, 0.1); color: #10b981; }
.metric-icon.verified { background: rgba(34, 197, 94, 0.1); color: #22c55e; }
.metric-icon.contests { background: rgba(168, 85, 247, 0.1); color: #a855f7; }
.metric-icon.active { background: rgba(16, 185, 129, 0.1); color: #10b981; }
.metric-icon.scheduled { background: rgba(59, 130, 246, 0.1); color: #3b82f6; }
.metric-icon.deposits { background: rgba(34, 197, 94, 0.1); color: #22c55e; }
.metric-icon.withdrawals { background: rgba(245, 158, 11, 0.1); color: #f59e0b; }
.metric-icon.revenue { background: rgba(99, 102, 241, 0.1); color: #6366f1; }
.metric-icon.traders { background: rgba(236, 72, 153, 0.1); color: #ec4899; }
.metric-icon.orders { background: rgba(59, 130, 246, 0.1); color: #3b82f6; }
.metric-icon.trades { background: rgba(168, 85, 247, 0.1); color: #a855f7; }
.metric-icon.kyc { background: rgba(239, 68, 68, 0.1); color: #ef4444; }
.metric-icon.affiliate { background: rgba(245, 158, 11, 0.1); color: #f59e0b; }

.metric-content {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  min-width: 0;
}

.metric-value {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  line-height: 1.2;
}

.metric-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  margin-top: var(--spacing-xs);
  width: fit-content;
}

.badge.warning {
  background: rgba(245, 158, 11, 0.15);
  color: var(--color-warning, #f59e0b);
}

.badge.danger {
  background: rgba(239, 68, 68, 0.15);
  color: var(--color-error, #ef4444);
}

/* Dark mode adjustments */
:root[data-theme="dark"] .metric-card {
  background: var(--color-bg-tertiary);
}

:root[data-theme="dark"] .metric-card.highlight-green {
  background: rgba(16, 185, 129, 0.1);
}

:root[data-theme="dark"] .metric-card.highlight-orange {
  background: rgba(245, 158, 11, 0.1);
}

:root[data-theme="dark"] .metric-card.highlight-red {
  background: rgba(239, 68, 68, 0.1);
}

/* RTL support */
[dir="rtl"] .metric-card {
  flex-direction: row-reverse;
}

[dir="rtl"] .metric-content {
  text-align: right;
}

/* Responsive */
@media (max-width: 768px) {
  .dashboard-page {
    padding: var(--spacing-md);
  }

  .page-header {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--spacing-md);
  }

  .cards-grid {
    grid-template-columns: 1fr;
  }
}
</style>
