<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { t } from '@/i18n';
import { api } from '@/api';

interface Trade {
  trade_id: string;
  symbol: string;
  side: 'buy' | 'sell';
  qty: number;
  entry_price: number;
  exit_price?: number;
  pnl?: number;
  pnl_percent?: number;
  opened_at: string;
  closed_at?: string;
  status: 'open' | 'closed';
}

interface TradeHistoryResponse {
  trades: Trade[];
  total: number;
  summary: {
    total_trades: number;
    winning_trades: number;
    losing_trades: number;
    total_pnl: number;
    avg_win: number;
    avg_loss: number;
  };
}

const props = defineProps<{
  contestId: string;
  userId?: string;
  show: boolean;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
}>();

// State
const trades = ref<Trade[]>([]);
const summary = ref<TradeHistoryResponse['summary'] | null>(null);
const loading = ref(true);
const error = ref<string | null>(null);

// TODO: Backend endpoints for trade history are not yet implemented:
//   - GET /api/user/contests/{contestId}/my-trades — current user's trades in a contest
//   - GET /api/user/contests/{contestId}/trades/{userId} — another user's trades in a contest
// These handlers need to be added to user-bff. They should query the fills/positions tables
// joined with orders for the given contest_id and user_id, and return TradeHistoryResponse
// with individual trades and a summary (total_trades, winning_trades, losing_trades,
// total_pnl, avg_win, avg_loss).
async function fetchTrades(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    const endpoint = props.userId
      ? `/api/user/contests/${props.contestId}/trades/${props.userId}`
      : `/api/user/contests/${props.contestId}/my-trades`;

    const response = await api.get<TradeHistoryResponse>(endpoint);
    trades.value = response.data.trades;
    summary.value = response.data.summary;
  } catch (err: any) {
    console.error('Failed to fetch trade history:', err);
    if (err?.response?.status === 403) {
      error.value = t('tradeHistory.availableAfterContest');
    } else {
      error.value = t('tradeHistory.loadError');
    }
  } finally {
    loading.value = false;
  }
}

// Computed
const hasTrades = computed(() => trades.value.length > 0);

const winRate = computed(() => {
  if (!summary.value || summary.value.total_trades === 0) return 0;
  return (summary.value.winning_trades / summary.value.total_trades) * 100;
});

// Formatting helpers
function formatPrice(price: number): string {
  if (price >= 1000) {
    return price.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  }
  return price.toFixed(price < 1 ? 6 : 2);
}

function formatPnl(pnl: number | undefined): string {
  if (pnl === undefined) return '-';
  const sign = pnl >= 0 ? '+' : '';
  return `${sign}$${Math.abs(pnl).toFixed(2)}`;
}

function formatPnlPercent(pnlPercent: number | undefined): string {
  if (pnlPercent === undefined) return '-';
  const sign = pnlPercent >= 0 ? '+' : '';
  return `${sign}${pnlPercent.toFixed(2)}%`;
}

function formatDateTime(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function getPnlClass(pnl: number | undefined): string {
  if (pnl === undefined || pnl === 0) return 'neutral';
  return pnl > 0 ? 'positive' : 'negative';
}

function getSideClass(side: string): string {
  return side === 'buy' ? 'side-buy' : 'side-sell';
}

// Close modal
function handleClose(): void {
  emit('close');
}

// Handle backdrop click
function handleBackdropClick(event: MouseEvent): void {
  if (event.target === event.currentTarget) {
    handleClose();
  }
}

// Watch for show changes
watch(() => props.show, (newShow) => {
  if (newShow) {
    fetchTrades();
  }
});

// Initial load
onMounted(() => {
  if (props.show) {
    fetchTrades();
  }
});
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="show" class="modal-backdrop" @click="handleBackdropClick">
        <div class="modal-container">
          <div class="modal-header">
            <h3 class="modal-title">{{ t('tradeHistory.title') }}</h3>
            <button class="close-btn" @click="handleClose">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>

          <div class="modal-body">
            <!-- Loading State -->
            <div v-if="loading" class="loading-container">
              <div class="loading-spinner"></div>
              <span>{{ t('common.loading') }}</span>
            </div>

            <!-- Error State -->
            <div v-else-if="error" class="error-container">
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <circle cx="12" cy="12" r="10" />
                <line x1="15" y1="9" x2="9" y2="15" />
                <line x1="9" y1="9" x2="15" y2="15" />
              </svg>
              <p>{{ error }}</p>
              <button class="btn btn-primary" @click="fetchTrades">
                {{ t('common.retry') }}
              </button>
            </div>

            <!-- Empty State -->
            <div v-else-if="!hasTrades" class="empty-container">
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M12 20v-6M6 20V10M18 20v-4" />
              </svg>
              <p>{{ t('tradeHistory.noTrades') }}</p>
            </div>

            <!-- Content -->
            <template v-else>
              <!-- Summary Stats -->
              <div v-if="summary" class="summary-section">
                <div class="summary-grid">
                  <div class="summary-item">
                    <span class="summary-value">{{ summary.total_trades }}</span>
                    <span class="summary-label">{{ t('tradeHistory.totalTrades') }}</span>
                  </div>
                  <div class="summary-item">
                    <span class="summary-value positive">{{ summary.winning_trades }}</span>
                    <span class="summary-label">{{ t('tradeHistory.wins') }}</span>
                  </div>
                  <div class="summary-item">
                    <span class="summary-value negative">{{ summary.losing_trades }}</span>
                    <span class="summary-label">{{ t('tradeHistory.losses') }}</span>
                  </div>
                  <div class="summary-item">
                    <span class="summary-value">{{ winRate.toFixed(1) }}%</span>
                    <span class="summary-label">{{ t('tradeHistory.winRate') }}</span>
                  </div>
                  <div class="summary-item highlight">
                    <span class="summary-value" :class="getPnlClass(summary.total_pnl)">
                      {{ formatPnl(summary.total_pnl) }}
                    </span>
                    <span class="summary-label">{{ t('tradeHistory.totalPnl') }}</span>
                  </div>
                </div>
              </div>

              <!-- Trades Table -->
              <div class="trades-table-container">
                <table class="trades-table">
                  <thead>
                    <tr>
                      <th>{{ t('tradeHistory.symbol') }}</th>
                      <th>{{ t('tradeHistory.side') }}</th>
                      <th>{{ t('tradeHistory.qty') }}</th>
                      <th>{{ t('tradeHistory.entry') }}</th>
                      <th>{{ t('tradeHistory.exit') }}</th>
                      <th>{{ t('tradeHistory.pnl') }}</th>
                      <th>{{ t('tradeHistory.time') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="trade in trades" :key="trade.trade_id" :class="{ 'trade-open': trade.status === 'open' }">
                      <td class="col-symbol">
                        <span class="symbol">{{ trade.symbol }}</span>
                      </td>
                      <td class="col-side">
                        <span class="side-badge" :class="getSideClass(trade.side)">
                          {{ trade.side.toUpperCase() }}
                        </span>
                      </td>
                      <td class="col-qty">{{ trade.qty.toLocaleString() }}</td>
                      <td class="col-price">{{ formatPrice(trade.entry_price) }}</td>
                      <td class="col-price">
                        <span v-if="trade.exit_price">{{ formatPrice(trade.exit_price) }}</span>
                        <span v-else class="status-open">{{ t('tradeHistory.open') }}</span>
                      </td>
                      <td class="col-pnl" :class="getPnlClass(trade.pnl)">
                        <div class="pnl-cell">
                          <span class="pnl-amount">{{ formatPnl(trade.pnl) }}</span>
                          <span class="pnl-percent">{{ formatPnlPercent(trade.pnl_percent) }}</span>
                        </div>
                      </td>
                      <td class="col-time">
                        <div class="time-cell">
                          <span>{{ formatDateTime(trade.opened_at) }}</span>
                          <span v-if="trade.closed_at" class="time-closed">
                            {{ formatDateTime(trade.closed_at) }}
                          </span>
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </template>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-backdrop {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: var(--spacing-md);
}

.modal-container {
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 900px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.modal-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.close-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.close-btn:hover {
  background: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

.modal-body {
  padding: var(--spacing-lg);
  overflow-y: auto;
  flex: 1;
}

/* Loading & Error States */
.loading-container,
.error-container,
.empty-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  text-align: center;
  color: var(--color-text-secondary);
}

.loading-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.error-container svg,
.empty-container svg {
  color: var(--color-text-muted);
}

.btn {
  padding: var(--spacing-sm) var(--spacing-lg);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 600;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.btn-primary {
  background: var(--color-primary);
  border: none;
  color: white;
}

.btn-primary:hover {
  background: var(--color-secondary);
}

/* Summary Section */
.summary-section {
  margin-bottom: var(--spacing-lg);
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(100px, 1fr));
  gap: var(--spacing-sm);
}

.summary-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: var(--spacing-md);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.summary-item.highlight {
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.1) 0%, rgba(168, 85, 247, 0.1) 100%);
  border: 1px solid rgba(99, 102, 241, 0.2);
}

.summary-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.summary-value.positive {
  color: #10B981;
}

.summary-value.negative {
  color: #EF4444;
}

.summary-value.neutral {
  color: var(--color-text-secondary);
}

.summary-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

/* Trades Table */
.trades-table-container {
  overflow-x: auto;
}

.trades-table {
  width: 100%;
  border-collapse: collapse;
}

.trades-table th {
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-text-secondary);
  text-align: left;
  background: var(--color-bg-secondary);
  border-bottom: 1px solid var(--color-border);
}

.trades-table td {
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  border-bottom: 1px solid var(--color-border);
}

.trades-table tr:hover {
  background: var(--color-bg-secondary);
}

.trades-table tr.trade-open {
  background: rgba(59, 130, 246, 0.05);
}

.col-symbol .symbol {
  font-weight: 600;
}

.col-side {
  width: 80px;
}

.side-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-weight: 600;
}

.side-badge.side-buy {
  background: rgba(16, 185, 129, 0.1);
  color: #10B981;
}

.side-badge.side-sell {
  background: rgba(239, 68, 68, 0.1);
  color: #EF4444;
}

.col-qty {
  font-variant-numeric: tabular-nums;
}

.col-price {
  font-variant-numeric: tabular-nums;
}

.status-open {
  color: var(--color-primary);
  font-weight: 500;
}

.col-pnl {
  text-align: right;
}

.col-pnl.positive {
  color: #10B981;
}

.col-pnl.negative {
  color: #EF4444;
}

.col-pnl.neutral {
  color: var(--color-text-secondary);
}

.pnl-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}

.pnl-amount {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.pnl-percent {
  font-size: var(--font-size-xs);
  opacity: 0.8;
}

.col-time {
  white-space: nowrap;
}

.time-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: var(--font-size-xs);
}

.time-closed {
  color: var(--color-text-secondary);
}

/* Transitions */
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active .modal-container,
.modal-leave-active .modal-container {
  transition: transform 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .modal-container,
.modal-leave-to .modal-container {
  transform: scale(0.95);
}

/* RTL Support */
[dir="rtl"] .trades-table th,
[dir="rtl"] .trades-table td {
  text-align: right;
}

[dir="rtl"] .col-pnl {
  text-align: left;
}

[dir="rtl"] .pnl-cell {
  align-items: flex-start;
}

/* Mobile */
@media (max-width: 767px) {
  .modal-container {
    max-height: 100vh;
    border-radius: var(--radius-lg) var(--radius-lg) 0 0;
    margin-top: auto;
  }

  .modal-body {
    padding: var(--spacing-md);
  }

  .summary-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .trades-table th,
  .trades-table td {
    padding: var(--spacing-xs) var(--spacing-sm);
    font-size: var(--font-size-xs);
  }

  .col-time {
    display: none;
  }
}
</style>
