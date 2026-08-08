<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import {
  getFinancialSummary,
  getDeposits,
  getTransactions,
  formatAmount,
  formatCompactAmount,
  getDepositStatusColor,
  type FinancialSummaryResponse,
  type Deposit,
  type Transaction,
} from '@/api/financial';

const toast = useToast();

// State
const loading = ref(true);
const error = ref<string | null>(null);
const summary = ref<FinancialSummaryResponse | null>(null);
const recentDeposits = ref<Deposit[]>([]);
const recentWithdrawals = ref<Transaction[]>([]);

// Date range state
const dateRange = ref<'7d' | '30d' | '90d' | 'custom'>('30d');
const customFrom = ref('');
const customTo = ref('');

// Computed date values
const dateParams = computed(() => {
  const now = new Date();
  let from: Date;
  let to = now;

  switch (dateRange.value) {
    case '7d':
      from = new Date(now);
      from.setDate(from.getDate() - 7);
      break;
    case '30d':
      from = new Date(now);
      from.setDate(from.getDate() - 30);
      break;
    case '90d':
      from = new Date(now);
      from.setDate(from.getDate() - 90);
      break;
    case 'custom':
      if (customFrom.value && customTo.value) {
        return {
          from: customFrom.value,
          to: customTo.value,
        };
      }
      from = new Date(now);
      from.setDate(from.getDate() - 30);
      break;
    default:
      from = new Date(now);
      from.setDate(from.getDate() - 30);
  }

  return {
    from: from.toISOString().split('T')[0],
    to: to.toISOString().split('T')[0],
  };
});

// Chart data
const chartData = computed(() => {
  if (!summary.value || !summary.value.net_revenue.length) {
    return { points: '', maxValue: 0, minValue: 0 };
  }

  const data = summary.value.net_revenue;
  const values = data.map(d => d.amount_cents);
  const maxValue = Math.max(...values, 0);
  const minValue = Math.min(...values, 0);
  const range = maxValue - minValue || 1;

  const width = 100;
  const height = 40;
  const padding = 2;

  const points = data.map((d, i) => {
    const x = padding + (i / (data.length - 1 || 1)) * (width - 2 * padding);
    const y = height - padding - ((d.amount_cents - minValue) / range) * (height - 2 * padding);
    return `${x},${y}`;
  }).join(' ');

  return { points, maxValue, minValue };
});

// Fetch all data
async function fetchData(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    const [summaryData, depositsData, withdrawalsData] = await Promise.all([
      getFinancialSummary({
        from: dateParams.value.from,
        to: dateParams.value.to,
        granularity: dateRange.value === '90d' ? 'week' : 'day',
      }),
      getDeposits({ limit: 5, status: 'succeeded' }),
      getTransactions({ limit: 5, type: 'withdrawal' }),
    ]);

    summary.value = summaryData;
    recentDeposits.value = depositsData.deposits || [];
    recentWithdrawals.value = withdrawalsData.transactions || [];
  } catch {
    error.value = t('financial.loadError');
    toast.error(t('financial.loadError'));
  } finally {
    loading.value = false;
  }
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function formatShortDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
  });
}

// Lifecycle
onMounted(() => {
  fetchData();
});

// Watchers
watch([dateRange, customFrom, customTo], () => {
  if (dateRange.value !== 'custom' || (customFrom.value && customTo.value)) {
    fetchData();
  }
});
</script>

<template>
  <div class="financial-page">
    <!-- Header -->
    <div class="page-header">
      <div class="header-content">
        <h1 class="page-title">{{ t('financial.title') }}</h1>
      </div>

      <!-- Date Range Selector -->
      <div class="date-range-selector">
        <div class="range-buttons">
          <button
            :class="['range-btn', { active: dateRange === '7d' }]"
            @click="dateRange = '7d'"
          >
            {{ t('financial.last7Days') }}
          </button>
          <button
            :class="['range-btn', { active: dateRange === '30d' }]"
            @click="dateRange = '30d'"
          >
            {{ t('financial.last30Days') }}
          </button>
          <button
            :class="['range-btn', { active: dateRange === '90d' }]"
            @click="dateRange = '90d'"
          >
            {{ t('financial.last90Days') }}
          </button>
          <button
            :class="['range-btn', { active: dateRange === 'custom' }]"
            @click="dateRange = 'custom'"
          >
            {{ t('financial.custom') }}
          </button>
        </div>

        <div v-if="dateRange === 'custom'" class="custom-range">
          <input
            v-model="customFrom"
            type="date"
            class="date-input"
            :placeholder="t('financial.from')"
          />
          <span class="date-separator">-</span>
          <input
            v-model="customTo"
            type="date"
            class="date-input"
            :placeholder="t('financial.to')"
          />
        </div>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="error-state">
      <p>{{ error }}</p>
      <button class="btn btn-primary" @click="fetchData">{{ t('common.retry') }}</button>
    </div>

    <!-- Content -->
    <template v-else-if="summary">
      <!-- Summary Cards -->
      <div class="summary-cards">
        <div class="summary-card deposits">
          <div class="card-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 2v20M17 7l-5-5-5 5" />
            </svg>
          </div>
          <div class="card-content">
            <span class="card-label">{{ t('financial.totalDeposits') }}</span>
            <span class="card-value positive">{{ formatAmount(summary.totals.total_deposits_cents) }}</span>
          </div>
        </div>

        <div class="summary-card withdrawals">
          <div class="card-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 22V2M7 17l5 5 5-5" />
            </svg>
          </div>
          <div class="card-content">
            <span class="card-label">{{ t('financial.totalWithdrawals') }}</span>
            <span class="card-value negative">{{ formatAmount(summary.totals.total_withdrawals_cents) }}</span>
          </div>
        </div>

        <div class="summary-card entry-fees">
          <div class="card-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
              <rect x="8" y="2" width="8" height="4" rx="1" ry="1" />
            </svg>
          </div>
          <div class="card-content">
            <span class="card-label">{{ t('financial.totalEntryFees') }}</span>
            <span class="card-value">{{ formatAmount(summary.totals.total_entry_fees_cents) }}</span>
          </div>
        </div>

        <div class="summary-card prizes">
          <div class="card-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6M18 9h1.5a2.5 2.5 0 0 0 0-5H18M4 22h16M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22M18 2H6v7a6 6 0 0 0 12 0V2Z" />
            </svg>
          </div>
          <div class="card-content">
            <span class="card-label">{{ t('financial.totalPrizes') }}</span>
            <span class="card-value negative">{{ formatAmount(summary.totals.total_prizes_paid_cents) }}</span>
          </div>
        </div>

        <div class="summary-card net-revenue">
          <div class="card-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="12" y1="1" x2="12" y2="23" />
              <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
            </svg>
          </div>
          <div class="card-content">
            <span class="card-label">{{ t('financial.netRevenue') }}</span>
            <span :class="['card-value', summary.totals.net_revenue_cents >= 0 ? 'positive' : 'negative']">
              {{ formatAmount(summary.totals.net_revenue_cents) }}
            </span>
          </div>
        </div>
      </div>

      <!-- Revenue Chart -->
      <div class="chart-section">
        <h2 class="section-title">{{ t('financial.revenueOverTime') }}</h2>
        <div class="chart-container">
          <svg viewBox="0 0 100 50" class="chart" preserveAspectRatio="none">
            <!-- Grid lines -->
            <line x1="0" y1="25" x2="100" y2="25" class="grid-line" />

            <!-- Chart line -->
            <polyline
              v-if="chartData.points"
              :points="chartData.points"
              class="chart-line"
              fill="none"
              stroke-width="0.5"
            />

            <!-- Zero line -->
            <line x1="0" y1="25" x2="100" y2="25" class="zero-line" />
          </svg>

          <div class="chart-labels">
            <span class="label-max">{{ formatCompactAmount(chartData.maxValue) }}</span>
            <span class="label-zero">$0</span>
            <span class="label-min">{{ formatCompactAmount(chartData.minValue) }}</span>
          </div>

          <div class="chart-x-labels">
            <span v-for="(point, idx) in summary.net_revenue" :key="idx" class="x-label">
              <template v-if="idx === 0 || idx === summary.net_revenue.length - 1 || idx === Math.floor(summary.net_revenue.length / 2)">
                {{ formatShortDate(point.date) }}
              </template>
            </span>
          </div>
        </div>
      </div>

      <!-- Tables Section -->
      <div class="tables-section">
        <!-- Recent Deposits -->
        <div class="table-card">
          <div class="table-header">
            <h3>{{ t('financial.recentDeposits') }}</h3>
          </div>

          <div v-if="recentDeposits.length === 0" class="empty-state">
            <p>{{ t('financial.noDeposits') }}</p>
          </div>

          <table v-else class="data-table">
            <thead>
              <tr>
                <th>{{ t('financial.user') }}</th>
                <th>{{ t('financial.amount') }}</th>
                <th>{{ t('financial.provider') }}</th>
                <th>{{ t('financial.status') }}</th>
                <th>{{ t('financial.date') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="deposit in recentDeposits" :key="deposit.id">
                <td class="user-cell">
                  <span class="user-email">{{ deposit.user.email }}</span>
                </td>
                <td class="amount-cell positive">
                  {{ formatAmount(deposit.amount_cents, deposit.currency) }}
                </td>
                <td>
                  <span :class="['provider-badge', `provider-${deposit.provider}`]">
                    {{ t(`financial.providers.${deposit.provider}`) || deposit.provider || '-' }}
                  </span>
                </td>
                <td>
                  <span :class="['status-badge', getDepositStatusColor(deposit.status)]">
                    {{ t(`financial.depositStatus.${deposit.status}`) }}
                  </span>
                </td>
                <td class="date-cell">{{ formatDate(deposit.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Recent Withdrawals -->
        <div class="table-card">
          <div class="table-header">
            <h3>{{ t('financial.recentWithdrawals') }}</h3>
          </div>

          <div v-if="recentWithdrawals.length === 0" class="empty-state">
            <p>{{ t('financial.noWithdrawals') }}</p>
          </div>

          <table v-else class="data-table">
            <thead>
              <tr>
                <th>{{ t('financial.user') }}</th>
                <th>{{ t('financial.amount') }}</th>
                <th>{{ t('financial.description') }}</th>
                <th>{{ t('financial.date') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="withdrawal in recentWithdrawals" :key="withdrawal.id">
                <td class="user-cell">
                  <span class="user-email">{{ withdrawal.user.email }}</span>
                </td>
                <td class="amount-cell negative">
                  {{ formatAmount(Math.abs(withdrawal.amount_cents)) }}
                </td>
                <td>{{ withdrawal.description || '-' }}</td>
                <td class="date-cell">{{ formatDate(withdrawal.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.financial-page {
  padding: var(--spacing-lg);
  max-width: 1400px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: var(--spacing-xl);
  gap: var(--spacing-lg);
  flex-wrap: wrap;
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0;
}

/* Date Range Selector */
.date-range-selector {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.range-buttons {
  display: flex;
  gap: var(--spacing-xs);
  background-color: var(--color-bg-secondary);
  padding: var(--spacing-xs);
  border-radius: var(--radius-md);
}

.range-btn {
  padding: var(--spacing-xs) var(--spacing-md);
  background: none;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.range-btn:hover {
  color: var(--color-text-primary);
}

.range-btn.active {
  background-color: var(--color-bg-primary);
  color: var(--color-primary);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.custom-range {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.date-input {
  padding: var(--spacing-xs) var(--spacing-sm);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
}

.date-separator {
  color: var(--color-text-secondary);
}

/* Loading/Error States */
.loading-state,
.error-state {
  text-align: center;
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto var(--spacing-md);
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Summary Cards */
.summary-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-xl);
}

.summary-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-md);
}

.card-icon {
  width: 48px;
  height: 48px;
  border-radius: var(--radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.summary-card.deposits .card-icon {
  background-color: var(--color-success-light, #dcfce7);
  color: var(--color-success, #16a34a);
}

.summary-card.withdrawals .card-icon {
  background-color: var(--color-error-light, #fee2e2);
  color: var(--color-error, #dc2626);
}

.summary-card.entry-fees .card-icon {
  background-color: var(--color-warning-light, #fef3c7);
  color: var(--color-warning, #d97706);
}

.summary-card.prizes .card-icon {
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

.summary-card.net-revenue .card-icon {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.card-content {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.card-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.card-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.card-value.positive {
  color: var(--color-success, #16a34a);
}

.card-value.negative {
  color: var(--color-error, #dc2626);
}

/* Chart Section */
.chart-section {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  margin-bottom: var(--spacing-xl);
}

.section-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-lg) 0;
}

.chart-container {
  position: relative;
  height: 200px;
  padding-left: 60px;
  padding-bottom: 30px;
}

.chart {
  width: 100%;
  height: 100%;
}

.grid-line {
  stroke: var(--color-border);
  stroke-width: 0.2;
  stroke-dasharray: 2, 2;
}

.zero-line {
  stroke: var(--color-text-secondary);
  stroke-width: 0.1;
}

.chart-line {
  stroke: var(--color-primary);
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}

.chart-labels {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 30px;
  width: 55px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  text-align: right;
  padding-right: var(--spacing-sm);
}

.chart-x-labels {
  position: absolute;
  bottom: 0;
  left: 60px;
  right: 0;
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

/* Tables Section */
.tables-section {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
  gap: var(--spacing-lg);
}

.table-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

.table-header {
  padding: var(--spacing-md) var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.table-header h3 {
  margin: 0;
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
}

.empty-state {
  padding: var(--spacing-xl);
  text-align: center;
  color: var(--color-text-secondary);
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th {
  text-align: left;
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-bg-secondary);
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
}

.data-table td {
  padding: var(--spacing-sm) var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
  font-size: var(--font-size-sm);
}

.data-table tr:last-child td {
  border-bottom: none;
}

.user-cell {
  max-width: 150px;
}

.user-email {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-primary);
  font-weight: 500;
}

.amount-cell {
  font-weight: 600;
}

.amount-cell.positive {
  color: var(--color-success, #16a34a);
}

.amount-cell.negative {
  color: var(--color-error, #dc2626);
}

.date-cell {
  color: var(--color-text-secondary);
  white-space: nowrap;
}

/* Status Badge */
.status-badge {
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: capitalize;
}

.status-badge.success {
  background-color: var(--color-success-light, #dcfce7);
  color: var(--color-success, #16a34a);
}

.status-badge.warning {
  background-color: var(--color-warning-light, #fef3c7);
  color: var(--color-warning, #d97706);
}

.status-badge.info {
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

.status-badge.error {
  background-color: var(--color-error-light, #fee2e2);
  color: var(--color-error, #dc2626);
}

.status-badge.secondary {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

/* Provider Badges */
.provider-badge {
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
}

.provider-jibit {
  background-color: #FFF3E0;
  color: #E65100;
}

.provider-nowpayments {
  background-color: #E8F5E9;
  color: #2E7D32;
}

/* Buttons */
.btn {
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all var(--transition-fast);
}

.btn-primary {
  background-color: var(--color-primary);
  color: white;
}

.btn-primary:hover {
  background-color: var(--color-primary-dark, #1d4ed8);
}

/* Responsive */
@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
  }

  .tables-section {
    grid-template-columns: 1fr;
  }

  .summary-cards {
    grid-template-columns: repeat(2, 1fr);
  }

  .chart-container {
    height: 150px;
  }
}

@media (max-width: 480px) {
  .summary-cards {
    grid-template-columns: 1fr;
  }

  .range-buttons {
    flex-wrap: wrap;
  }
}
</style>
