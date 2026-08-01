<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { useAuthStore } from '@/stores/auth';
import {
  getMarketStatus,
  switchProvider,
  reconnectProvider,
  type MarketStatus,
  type SymbolStatus,
} from '@/api/market';

const toast = useToast();
const auth = useAuthStore();

const status = ref<MarketStatus | null>(null);
const loading = ref(true);
const error = ref<string | null>(null);
const activeTab = ref<string>('all');
const searchQuery = ref('');
const sortField = ref<string>('symbol');
const sortAsc = ref(true);
const switchingProvider = ref(false);
const reconnecting = ref(false);
const selectedProvider = ref('');
const showConfirmDialog = ref(false);
let refreshInterval: ReturnType<typeof setInterval> | null = null;

const canManage = computed(() => auth.hasPermission('market.manage'));

async function fetchStatus(): Promise<void> {
  try {
    status.value = await getMarketStatus();
    if (loading.value) loading.value = false;
    error.value = null;
  } catch {
    if (loading.value) {
      error.value = t('market.loadError');
      loading.value = false;
    }
  }
}

onMounted(() => {
  fetchStatus();
  refreshInterval = setInterval(fetchStatus, 3000);
});

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval);
});

// Category tabs
const categories = computed(() => {
  if (!status.value) return [];
  const counts: Record<string, number> = {};
  for (const sym of status.value.symbols) {
    const cat = sym.category || 'other';
    counts[cat] = (counts[cat] || 0) + 1;
  }
  const tabs = [{ key: 'all', label: t('market.tab.all'), count: status.value.symbols.length }];
  if (counts['forex']) tabs.push({ key: 'forex', label: t('market.tab.forex'), count: counts['forex'] });
  if (counts['crypto']) tabs.push({ key: 'crypto', label: t('market.tab.crypto'), count: counts['crypto'] });
  if (counts['commodity']) tabs.push({ key: 'commodity', label: t('market.tab.commodity'), count: counts['commodity'] });
  return tabs;
});

// Filtered & sorted symbols
const filteredSymbols = computed((): SymbolStatus[] => {
  if (!status.value) return [];
  let syms = status.value.symbols;

  // Filter by category
  if (activeTab.value !== 'all') {
    syms = syms.filter(s => s.category === activeTab.value);
  }

  // Filter by search
  if (searchQuery.value) {
    const q = searchQuery.value.toUpperCase();
    syms = syms.filter(s => s.symbol.toUpperCase().includes(q));
  }

  // Sort
  const field = sortField.value;
  const asc = sortAsc.value;
  syms = [...syms].sort((a, b) => {
    let va: number | string = 0;
    let vb: number | string = 0;
    if (field === 'symbol') {
      va = a.symbol;
      vb = b.symbol;
    } else if (field === 'bid') {
      va = a.bid;
      vb = b.bid;
    } else if (field === 'ask') {
      va = a.ask;
      vb = b.ask;
    } else if (field === 'last') {
      va = a.last;
      vb = b.last;
    } else if (field === 'age') {
      va = a.age_ms;
      vb = b.age_ms;
    } else if (field === 'status') {
      const order: Record<string, number> = { fresh: 0, warning: 1, stale: 2, no_data: 3 };
      va = order[a.status] ?? 4;
      vb = order[b.status] ?? 4;
    }
    if (va < vb) return asc ? -1 : 1;
    if (va > vb) return asc ? 1 : -1;
    return 0;
  });

  return syms;
});

function toggleSort(field: string): void {
  if (sortField.value === field) {
    sortAsc.value = !sortAsc.value;
  } else {
    sortField.value = field;
    sortAsc.value = true;
  }
}

function sortIndicator(field: string): string {
  if (sortField.value !== field) return '';
  return sortAsc.value ? ' \u25B2' : ' \u25BC';
}

function statusIndicator(st: string): string {
  switch (st) {
    case 'fresh': return '\u{1F7E2}';
    case 'warning': return '\u{1F7E1}';
    case 'stale': return '\u{1F534}';
    default: return '\u26AB';
  }
}

function connectionIndicator(st: string): string {
  return st === 'connected' ? '\u{1F7E2}' : '\u{1F534}';
}

function formatPrice(price: number): string {
  if (!price) return '-';
  if (price >= 1000) return price.toFixed(2);
  if (price >= 1) return price.toFixed(5);
  return price.toFixed(6);
}

function formatUptime(symbols: MarketStatus | null): string {
  if (!symbols) return '-';
  // Estimate from symbols' oldest timestamp
  return t('market.connected');
}

function initiateSwitch(provider: string): void {
  selectedProvider.value = provider;
  showConfirmDialog.value = true;
}

async function confirmSwitch(): Promise<void> {
  showConfirmDialog.value = false;
  switchingProvider.value = true;
  try {
    await switchProvider(selectedProvider.value);
    toast.success(t('market.switchSuccess'));
    await fetchStatus();
  } catch {
    toast.error(t('market.switchError'));
  } finally {
    switchingProvider.value = false;
  }
}

async function handleReconnect(): Promise<void> {
  reconnecting.value = true;
  try {
    await reconnectProvider();
    toast.success(t('market.reconnectSuccess'));
    await fetchStatus();
  } catch {
    toast.error(t('market.reconnectError'));
  } finally {
    reconnecting.value = false;
  }
}
</script>

<template>
  <div class="market-data-page">
    <div class="page-header">
      <h1>{{ t('market.title') }}</h1>
      <div class="header-actions">
        <button class="btn btn-icon" @click="fetchStatus" :title="t('market.refresh')">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M1 1v5h5" /><path d="M15 15v-5h-5" />
            <path d="M13.5 6A6 6 0 0 0 3 3.5L1 6" /><path d="M2.5 10A6 6 0 0 0 13 12.5L15 10" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Loading state -->
    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <p>{{ t('market.loading') }}</p>
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="error-state">
      <p>{{ error }}</p>
      <button class="btn btn-primary" @click="fetchStatus">{{ t('common.retry') }}</button>
    </div>

    <!-- Main content -->
    <template v-else-if="status">
      <!-- Provider status card -->
      <div class="status-card">
        <div class="status-row">
          <div class="status-group">
            <span class="status-label">{{ t('market.activeProvider') }}:</span>
            <span class="status-value provider-name">{{ status.active_provider }}</span>
            <template v-if="canManage">
              <select
                v-model="selectedProvider"
                class="provider-select"
                :disabled="switchingProvider"
              >
                <option value="" disabled>{{ t('market.selectProvider') }}</option>
                <option
                  v-for="p in status.available_providers.filter(p => p !== status!.active_provider)"
                  :key="p"
                  :value="p"
                >
                  {{ p }}
                </option>
              </select>
              <button
                class="btn btn-sm btn-primary"
                @click="initiateSwitch(selectedProvider)"
                :disabled="!selectedProvider || switchingProvider"
              >
                {{ switchingProvider ? t('market.switching') : t('market.switch') }}
              </button>
              <button
                class="btn btn-sm btn-warning"
                @click="handleReconnect"
                :disabled="reconnecting"
              >
                {{ reconnecting ? t('market.reconnecting') : t('market.reconnect') }}
              </button>
            </template>
          </div>
        </div>

        <div class="status-details">
          <div class="detail-item">
            <span class="detail-label">Massive:</span>
            <span v-for="(val, key) in status.massive_status" :key="key" class="detail-badge">
              {{ key }}: {{ connectionIndicator(val) }} {{ val }}
            </span>
          </div>
          <div class="detail-item">
            <span class="detail-label">TwelveData:</span>
            <span v-for="(val, key) in status.twelvedata_status" :key="key" class="detail-badge">
              {{ key }}: {{ connectionIndicator(val) }} {{ val }}
            </span>
          </div>
          <div class="detail-item">
            <span class="detail-label">{{ t('market.receiving') }}:</span>
            <span class="detail-value">{{ status.symbols_receiving }}/{{ status.symbols_total }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">{{ t('market.stale') }}:</span>
            <span class="detail-value" :class="{ 'text-danger': status.symbols_stale > 0 }">
              {{ status.symbols_stale }}
            </span>
          </div>
          <div class="detail-item">
            <span class="detail-label">{{ t('market.status') }}:</span>
            <span class="detail-value">{{ formatUptime(status) }}</span>
          </div>
        </div>
      </div>

      <!-- Category tabs -->
      <div class="tabs">
        <button
          v-for="tab in categories"
          :key="tab.key"
          :class="['tab-btn', { active: activeTab === tab.key }]"
          @click="activeTab = tab.key"
        >
          {{ tab.label }} ({{ tab.count }})
        </button>
      </div>

      <!-- Search -->
      <div class="search-bar">
        <input
          v-model="searchQuery"
          type="text"
          :placeholder="t('market.searchSymbol')"
          class="search-input"
        />
      </div>

      <!-- Symbols table -->
      <div class="table-container">
        <table class="symbols-table">
          <thead>
            <tr>
              <th @click="toggleSort('symbol')" class="sortable">
                {{ t('market.symbol') }}{{ sortIndicator('symbol') }}
              </th>
              <th @click="toggleSort('bid')" class="sortable">
                {{ t('market.bid') }}{{ sortIndicator('bid') }}
              </th>
              <th @click="toggleSort('ask')" class="sortable">
                {{ t('market.ask') }}{{ sortIndicator('ask') }}
              </th>
              <th @click="toggleSort('last')" class="sortable">
                {{ t('market.last') }}{{ sortIndicator('last') }}
              </th>
              <th @click="toggleSort('age')" class="sortable">
                {{ t('market.age') }}{{ sortIndicator('age') }}
              </th>
              <th @click="toggleSort('status')" class="sortable">
                {{ t('market.statusCol') }}{{ sortIndicator('status') }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="sym in filteredSymbols" :key="sym.symbol" :class="'row-' + sym.status">
              <td class="symbol-cell">{{ sym.symbol }}</td>
              <td class="price-cell">{{ formatPrice(sym.bid) }}</td>
              <td class="price-cell">{{ formatPrice(sym.ask) }}</td>
              <td class="price-cell">{{ formatPrice(sym.last) }}</td>
              <td class="age-cell">
                <span v-if="sym.status === 'no_data'">-</span>
                <span v-else>{{ Math.round(sym.age_ms / 1000) }}s</span>
              </td>
              <td class="status-cell">{{ statusIndicator(sym.status) }}</td>
            </tr>
            <tr v-if="filteredSymbols.length === 0">
              <td colspan="6" class="empty-row">{{ t('market.noSymbols') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <!-- Confirmation dialog -->
    <div v-if="showConfirmDialog" class="modal-overlay" @click.self="showConfirmDialog = false">
      <div class="modal-dialog">
        <h3>{{ t('market.confirmSwitch') }}</h3>
        <p>{{ t('market.confirmSwitchMessage', { provider: selectedProvider }) }}</p>
        <div class="modal-actions">
          <button class="btn btn-secondary" @click="showConfirmDialog = false">
            {{ t('common.cancel') }}
          </button>
          <button class="btn btn-primary" @click="confirmSwitch">
            {{ t('market.confirmSwitchBtn') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.market-data-page {
  padding: var(--spacing-lg);
  max-width: var(--max-content-width);
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--spacing-lg);
}

.page-header h1 {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0;
}

.header-actions {
  display: flex;
  gap: var(--spacing-sm);
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  cursor: pointer;
  font-size: var(--font-size-sm);
  font-family: inherit;
  transition: all var(--transition-fast);
}

.btn:hover {
  background-color: var(--color-bg-tertiary);
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-icon {
  padding: var(--spacing-sm);
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
}

.btn-primary {
  background-color: var(--color-primary);
  color: white;
  border-color: var(--color-primary);
}

.btn-primary:hover {
  opacity: 0.9;
}

.btn-secondary {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

.btn-warning {
  background-color: var(--color-warning, #d97706);
  color: white;
  border-color: var(--color-warning, #d97706);
}

.btn-warning:hover {
  opacity: 0.9;
}

/* Loading & Error */
.loading-state,
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-2xl);
  gap: var(--spacing-md);
  color: var(--color-text-secondary);
}

.spinner {
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

/* Status card */
.status-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  margin-bottom: var(--spacing-lg);
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-md);
}

.status-group {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.status-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  font-weight: 500;
}

.status-value {
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  font-weight: 600;
}

.provider-name {
  font-size: var(--font-size-md);
  color: var(--color-primary);
  font-weight: 700;
  text-transform: capitalize;
}

.provider-select {
  padding: var(--spacing-xs) var(--spacing-sm);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  font-size: var(--font-size-xs);
  font-family: inherit;
}

.status-details {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-lg);
}

.detail-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.detail-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.detail-badge {
  font-size: var(--font-size-xs);
  padding: 2px var(--spacing-xs);
  border-radius: var(--radius-sm);
  background-color: var(--color-bg-tertiary);
  margin-inline-end: var(--spacing-xs);
}

.detail-value {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
}

.text-danger {
  color: var(--color-error, #dc2626);
}

/* Tabs */
.tabs {
  display: flex;
  gap: var(--spacing-xs);
  margin-bottom: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
  padding-bottom: var(--spacing-xs);
}

.tab-btn {
  padding: var(--spacing-sm) var(--spacing-md);
  border: none;
  border-bottom: 2px solid transparent;
  background: none;
  color: var(--color-text-secondary);
  cursor: pointer;
  font-size: var(--font-size-sm);
  font-family: inherit;
  font-weight: 500;
  transition: all var(--transition-fast);
}

.tab-btn:hover {
  color: var(--color-text-primary);
}

.tab-btn.active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
}

/* Search */
.search-bar {
  margin-bottom: var(--spacing-md);
}

.search-input {
  width: 100%;
  max-width: 400px;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
  font-family: inherit;
}

.search-input:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-light);
}

/* Table */
.table-container {
  overflow-x: auto;
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

.symbols-table {
  width: 100%;
  border-collapse: collapse;
}

.symbols-table th {
  padding: var(--spacing-sm) var(--spacing-md);
  text-align: start;
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  border-bottom: 1px solid var(--color-border);
  background-color: var(--color-bg-secondary);
  white-space: nowrap;
}

.symbols-table th.sortable {
  cursor: pointer;
  user-select: none;
}

.symbols-table th.sortable:hover {
  color: var(--color-text-primary);
}

.symbols-table td {
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: var(--font-size-sm);
  border-bottom: 1px solid var(--color-border);
  white-space: nowrap;
}

.symbol-cell {
  font-weight: 600;
  color: var(--color-text-primary);
}

.price-cell {
  font-family: var(--font-mono, monospace);
  color: var(--color-text-primary);
}

.age-cell {
  color: var(--color-text-secondary);
}

.status-cell {
  text-align: center;
}

.empty-row {
  text-align: center;
  color: var(--color-text-secondary);
  padding: var(--spacing-xl) !important;
}

.row-stale {
  background-color: rgba(220, 38, 38, 0.05);
}

.row-warning {
  background-color: rgba(217, 119, 6, 0.05);
}

.row-no_data {
  background-color: var(--color-bg-secondary);
  opacity: 0.7;
}

/* Modal */
.modal-overlay {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: var(--z-modal, 300);
}

.modal-dialog {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
  max-width: 400px;
  width: 90%;
  box-shadow: var(--shadow-lg);
}

.modal-dialog h3 {
  margin: 0 0 var(--spacing-md);
  font-size: var(--font-size-lg);
  color: var(--color-text-primary);
}

.modal-dialog p {
  margin: 0 0 var(--spacing-lg);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
}
</style>
