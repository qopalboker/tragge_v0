<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { useAuthStore } from '@/stores/auth';
import { getSymbols, createSymbol, updateSymbol, type Symbol, type CreateSymbolRequest } from '@/api/symbols';
import { getMarketStatus, getProviderConfig, switchCryptoProvider, switchForexProvider, type SymbolStatus, type ProviderConfig } from '@/api/market';

const toast = useToast();
const authStore = useAuthStore();
let isMounted = true;

const symbols = ref<Symbol[]>([]);
const loading = ref(true);
const error = ref<string | null>(null);
const searchQuery = ref('');
const assetTypeFilter = ref('');
const activeFilter = ref('');

const assetTypes = ['stock', 'crypto', 'forex', 'commodity'];

// Tab state
type TabKey = 'crypto' | 'forex' | 'commodity' | 'all';
const activeTab = ref<TabKey>('crypto');

// Stop test price when switching tabs
watch(activeTab, () => stopTestPrice());

// Test price state
const testingSymbol = ref<string | null>(null);
const testPriceData = ref<SymbolStatus | null>(null);
const testPriceLoading = ref(false);
let testPollInterval: ReturnType<typeof setInterval> | null = null;

// Provider config state
const providerConfig = ref<ProviderConfig | null>(null);
const switchingCrypto = ref(false);
const switchingForex = ref(false);
let providerPollInterval: ReturnType<typeof setInterval> | null = null;

// Crypto symbol definitions
const cryptoSymbols = [
  { symbol: 'BTC/USD', display: 'BTC-USDT', name: 'Bitcoin' },
  { symbol: 'ETH/USD', display: 'ETH-USDT', name: 'Ethereum' },
  { symbol: 'SOL/USD', display: 'SOL-USDT', name: 'Solana' },
  { symbol: 'DOGE/USD', display: 'DOGE-USDT', name: 'Dogecoin' },
  { symbol: 'XRP/USD', display: 'XRP-USDT', name: 'Ripple' },
  { symbol: 'ADA/USD', display: 'ADA-USDT', name: 'Cardano' },
  { symbol: 'AVAX/USD', display: 'AVAX-USDT', name: 'Avalanche' },
  { symbol: 'LINK/USD', display: 'LINK-USDT', name: 'Chainlink' },
  { symbol: 'DOT/USD', display: 'DOT-USDT', name: 'Polkadot' },
  { symbol: 'POL/USD', display: 'POL-USDT', name: 'Polygon' },
  { symbol: 'SHIB/USD', display: 'SHIB-USDT', name: 'Shiba Inu' },
  { symbol: 'LTC/USD', display: 'LTC-USDT', name: 'Litecoin' },
  { symbol: 'UNI/USD', display: 'UNI-USDT', name: 'Uniswap' },
  { symbol: 'XLM/USD', display: 'XLM-USDT', name: 'Stellar' },
  { symbol: 'NEAR/USD', display: 'NEAR-USDT', name: 'NEAR Protocol' },
  { symbol: 'AAVE/USD', display: 'AAVE-USDT', name: 'Aave' },
  { symbol: 'SUI/USD', display: 'SUI-USDT', name: 'Sui' },
  { symbol: 'PEPE/USD', display: 'PEPE-USDT', name: 'Pepe' },
  { symbol: 'APT/USD', display: 'APT-USDT', name: 'Aptos' },
  { symbol: 'BCH/USD', display: 'BCH-USDT', name: 'Bitcoin Cash' },
  { symbol: 'CRO/USD', display: 'CRO-USDT', name: 'Cronos' },
  { symbol: 'HBAR/USD', display: 'HBAR-USDT', name: 'Hedera' },
  { symbol: 'ICP/USD', display: 'ICP-USDT', name: 'Internet Computer' },
  { symbol: 'VET/USD', display: 'VET-USDT', name: 'VeChain' },
] as const;

// Forex symbol definitions
const forexSymbols = [
  // Majors
  { symbol: 'EUR/USD', name: 'Euro / US Dollar' },
  { symbol: 'GBP/USD', name: 'British Pound / US Dollar' },
  { symbol: 'USD/JPY', name: 'US Dollar / Japanese Yen' },
  { symbol: 'USD/CHF', name: 'US Dollar / Swiss Franc' },
  { symbol: 'AUD/USD', name: 'Australian Dollar / US Dollar' },
  { symbol: 'USD/CAD', name: 'US Dollar / Canadian Dollar' },
  { symbol: 'NZD/USD', name: 'New Zealand Dollar / US Dollar' },
  // Minors / Crosses
  { symbol: 'EUR/GBP', name: 'Euro / British Pound' },
  { symbol: 'EUR/JPY', name: 'Euro / Japanese Yen' },
  { symbol: 'EUR/CHF', name: 'Euro / Swiss Franc' },
  { symbol: 'EUR/AUD', name: 'Euro / Australian Dollar' },
  { symbol: 'EUR/CAD', name: 'Euro / Canadian Dollar' },
  { symbol: 'EUR/NZD', name: 'Euro / New Zealand Dollar' },
  { symbol: 'GBP/JPY', name: 'British Pound / Japanese Yen' },
  { symbol: 'GBP/CHF', name: 'British Pound / Swiss Franc' },
  { symbol: 'GBP/AUD', name: 'British Pound / Australian Dollar' },
  { symbol: 'GBP/CAD', name: 'British Pound / Canadian Dollar' },
  { symbol: 'GBP/NZD', name: 'British Pound / New Zealand Dollar' },
  { symbol: 'AUD/JPY', name: 'Australian Dollar / Japanese Yen' },
  { symbol: 'AUD/CHF', name: 'Australian Dollar / Swiss Franc' },
  { symbol: 'AUD/CAD', name: 'Australian Dollar / Canadian Dollar' },
  { symbol: 'AUD/NZD', name: 'Australian Dollar / New Zealand Dollar' },
  { symbol: 'CAD/JPY', name: 'Canadian Dollar / Japanese Yen' },
  { symbol: 'CAD/CHF', name: 'Canadian Dollar / Swiss Franc' },
  { symbol: 'CHF/JPY', name: 'Swiss Franc / Japanese Yen' },
  { symbol: 'NZD/JPY', name: 'New Zealand Dollar / Japanese Yen' },
  { symbol: 'NZD/CHF', name: 'New Zealand Dollar / Swiss Franc' },
  { symbol: 'NZD/CAD', name: 'New Zealand Dollar / Canadian Dollar' },
  // Exotics
  { symbol: 'USD/TRY', name: 'US Dollar / Turkish Lira' },
  { symbol: 'USD/MXN', name: 'US Dollar / Mexican Peso' },
  { symbol: 'USD/ZAR', name: 'US Dollar / South African Rand' },
  { symbol: 'USD/SGD', name: 'US Dollar / Singapore Dollar' },
  { symbol: 'USD/HKD', name: 'US Dollar / Hong Kong Dollar' },
  { symbol: 'USD/NOK', name: 'US Dollar / Norwegian Krone' },
  { symbol: 'USD/SEK', name: 'US Dollar / Swedish Krona' },
  { symbol: 'USD/DKK', name: 'US Dollar / Danish Krone' },
  { symbol: 'USD/PLN', name: 'US Dollar / Polish Zloty' },
  { symbol: 'USD/CZK', name: 'US Dollar / Czech Koruna' },
  { symbol: 'USD/HUF', name: 'US Dollar / Hungarian Forint' },
  { symbol: 'EUR/TRY', name: 'Euro / Turkish Lira' },
  { symbol: 'EUR/SEK', name: 'Euro / Swedish Krona' },
  { symbol: 'EUR/NOK', name: 'Euro / Norwegian Krone' },
  { symbol: 'EUR/PLN', name: 'Euro / Polish Zloty' },
  { symbol: 'GBP/TRY', name: 'British Pound / Turkish Lira' },
  // Index
  { symbol: 'US30/USD', name: 'Dow Jones Industrial Average' },
] as const;

// Modal state
const showAddModal = ref(false);
const showEditModal = ref(false);
const editingSymbol = ref<Symbol | null>(null);
const submitting = ref(false);

// Form state
const form = ref<CreateSymbolRequest>({
  symbol: '',
  name: '',
  asset_type: 'stock',
  provider_symbol_twelvedata: '',
  provider_symbol_massive: '',
  provider_symbol_finnhub: '',
  is_active: true,
});

// Check permissions
const canManage = computed(() => authStore.hasPermission('symbols.manage'));

const filteredSymbols = computed(() => {
  let result = symbols.value;

  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase();
    result = result.filter(s =>
      s.symbol.toLowerCase().includes(query) ||
      s.name.toLowerCase().includes(query)
    );
  }

  if (assetTypeFilter.value) {
    result = result.filter(s => s.asset_type === assetTypeFilter.value);
  }

  if (activeFilter.value) {
    const isActive = activeFilter.value === 'true';
    result = result.filter(s => s.is_active === isActive);
  }

  return result;
});

// Get crypto symbol status from the main symbols list
function getCryptoSymbolStatus(canonicalSymbol: string): Symbol | undefined {
  return symbols.value.find(s => s.symbol === canonicalSymbol);
}

function getStatusClass(status: string): string {
  const classes: Record<string, string> = {
    fresh: 'price-status-fresh',
    warning: 'price-status-warning',
    stale: 'price-status-stale',
    no_data: 'price-status-nodata',
  };
  return classes[status] || 'price-status-nodata';
}

function getStatusLabel(status: string): string {
  const labels: Record<string, string> = {
    fresh: t('symbols.statusFresh'),
    warning: t('symbols.statusWarning'),
    stale: t('symbols.statusStale'),
    no_data: t('symbols.statusNoData'),
  };
  return labels[status] || status;
}

function formatAge(ageMs: number): string {
  if (ageMs < 1000) return `${ageMs}ms`;
  if (ageMs < 60000) return `${(ageMs / 1000).toFixed(1)}s`;
  return `${(ageMs / 60000).toFixed(1)}m`;
}

function formatPrice(price: number): string {
  if (price === 0) return '-';
  if (price < 0.01) return price.toFixed(8);
  if (price < 1) return price.toFixed(6);
  if (price < 100) return price.toFixed(4);
  return price.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

async function startTestPrice(canonicalSymbol: string): Promise<void> {
  // Close any existing test
  stopTestPrice();

  testingSymbol.value = canonicalSymbol;
  testPriceLoading.value = true;
  testPriceData.value = null;

  await fetchTestPrice();

  // Poll every 3 seconds
  testPollInterval = setInterval(fetchTestPrice, 3000);
}

async function fetchTestPrice(): Promise<void> {
  if (!testingSymbol.value) return;

  try {
    const status = await getMarketStatus();
    const match = status.symbols.find(s => s.symbol === testingSymbol.value);
    testPriceData.value = match || null;
  } catch {
    testPriceData.value = null;
  } finally {
    testPriceLoading.value = false;
  }
}

function stopTestPrice(): void {
  if (testPollInterval) {
    clearInterval(testPollInterval);
    testPollInterval = null;
  }
  testingSymbol.value = null;
  testPriceData.value = null;
  testPriceLoading.value = false;
}

async function loadProviderConfig(): Promise<void> {
  try {
    providerConfig.value = await getProviderConfig();
  } catch (e) {
    console.warn('Failed to load provider config:', e);
  }
}

async function handleSwitchCrypto(provider: string): Promise<void> {
  if (switchingCrypto.value) return;

  const providerLabel = t(`provider.${provider}`);
  const confirmed = confirm(t('provider.switchConfirm').replace('{provider}', providerLabel));
  if (!confirmed) return;

  switchingCrypto.value = true;
  try {
    await switchCryptoProvider(provider);
    if (!isMounted) return;
    toast.success(t('provider.switchSuccess').replace('{provider}', providerLabel));
    await loadProviderConfig();
  } catch {
    if (!isMounted) return;
    toast.error(t('provider.switchError'));
  } finally {
    switchingCrypto.value = false;
  }
}

async function handleSwitchForex(provider: string): Promise<void> {
  if (switchingForex.value) return;

  const providerLabel = t(`provider.${provider}`);
  const confirmed = confirm(t('provider.switchConfirm').replace('{provider}', providerLabel));
  if (!confirmed) return;

  switchingForex.value = true;
  try {
    await switchForexProvider(provider);
    if (!isMounted) return;
    toast.success(t('provider.switchSuccess').replace('{provider}', providerLabel));
    await loadProviderConfig();
  } catch {
    toast.error(t('provider.switchError'));
  } finally {
    switchingForex.value = false;
  }
}

function formatTickAge(lastTick: number): string {
  if (!lastTick) return '-';
  const ageS = Math.floor(Date.now() / 1000) - lastTick;
  if (ageS < 60) return `${ageS}s`;
  if (ageS < 3600) return `${Math.floor(ageS / 60)}m`;
  return `${Math.floor(ageS / 3600)}h`;
}

async function fetchSymbols(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    const response = await getSymbols({ limit: 200 });
    symbols.value = response.symbols || [];
  } catch {
    error.value = t('common.error');
    symbols.value = [];
  } finally {
    loading.value = false;
  }
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString();
}

function getAssetTypeClass(type: string): string {
  const classes: Record<string, string> = {
    stock: 'type-stock',
    crypto: 'type-crypto',
    forex: 'type-forex',
    commodity: 'type-commodity',
  };
  return classes[type] || 'type-default';
}

function openAddModal(): void {
  form.value = {
    symbol: '',
    name: '',
    asset_type: 'stock',
    provider_symbol_twelvedata: '',
    provider_symbol_massive: '',
    provider_symbol_finnhub: '',
    is_active: true,
  };
  showAddModal.value = true;
}

function closeAddModal(): void {
  showAddModal.value = false;
}

function openEditModal(symbol: Symbol): void {
  editingSymbol.value = symbol;
  form.value = {
    symbol: symbol.symbol,
    name: symbol.name,
    asset_type: symbol.asset_type,
    provider_symbol_twelvedata: symbol.provider_symbol_twelvedata || '',
    provider_symbol_massive: symbol.provider_symbol_massive || '',
    provider_symbol_finnhub: symbol.provider_symbol_finnhub || '',
    is_active: symbol.is_active,
  };
  showEditModal.value = true;
}

function closeEditModal(): void {
  showEditModal.value = false;
  editingSymbol.value = null;
}

async function handleAddSymbol(): Promise<void> {
  if (!form.value.symbol || !form.value.name) {
    toast.error(t('symbols.validationError'));
    return;
  }

  submitting.value = true;
  try {
    const newSymbol = await createSymbol({
      symbol: form.value.symbol.toUpperCase().trim(),
      name: form.value.name.trim(),
      asset_type: form.value.asset_type,
      provider_symbol_twelvedata: form.value.provider_symbol_twelvedata || undefined,
      provider_symbol_massive: form.value.provider_symbol_massive || undefined,
      provider_symbol_finnhub: form.value.provider_symbol_finnhub || undefined,
      is_active: form.value.is_active,
    });
    symbols.value.push(newSymbol);
    symbols.value.sort((a, b) => a.symbol.localeCompare(b.symbol));
    toast.success(t('symbols.createSuccess'));
    closeAddModal();
  } catch {
    toast.error(t('symbols.createError'));
  } finally {
    submitting.value = false;
  }
}

async function handleEditSymbol(): Promise<void> {
  if (!editingSymbol.value || !form.value.name) {
    toast.error(t('symbols.validationError'));
    return;
  }

  submitting.value = true;
  try {
    const updated = await updateSymbol(editingSymbol.value.symbol, {
      name: form.value.name.trim(),
      asset_type: form.value.asset_type,
      provider_symbol_twelvedata: form.value.provider_symbol_twelvedata || undefined,
      provider_symbol_massive: form.value.provider_symbol_massive || undefined,
      provider_symbol_finnhub: form.value.provider_symbol_finnhub || undefined,
      is_active: form.value.is_active,
    });
    const index = symbols.value.findIndex(s => s.symbol === updated.symbol);
    if (index !== -1) {
      symbols.value[index] = updated;
    }
    toast.success(t('symbols.updateSuccess'));
    closeEditModal();
  } catch {
    toast.error(t('symbols.updateError'));
  } finally {
    submitting.value = false;
  }
}

async function toggleActive(symbol: Symbol): Promise<void> {
  try {
    const updated = await updateSymbol(symbol.symbol, { is_active: !symbol.is_active });
    const index = symbols.value.findIndex(s => s.symbol === updated.symbol);
    if (index !== -1) {
      symbols.value[index] = updated;
    }
    toast.success(updated.is_active ? t('symbols.activatedSuccess') : t('symbols.deactivatedSuccess'));
  } catch {
    toast.error(t('symbols.toggleError'));
  }
}

onMounted(() => {
  fetchSymbols();
  loadProviderConfig();
  providerPollInterval = setInterval(loadProviderConfig, 5000);
});

onUnmounted(() => {
  isMounted = false;
  stopTestPrice();
  if (providerPollInterval) {
    clearInterval(providerPollInterval);
    providerPollInterval = null;
  }
});
</script>

<template>
  <div class="symbols-page">
    <div class="page-header">
      <h1 class="page-title">{{ t('symbols.title') }}</h1>
      <button v-if="canManage" class="btn btn-primary" @click="openAddModal">
        + {{ t('symbols.newSymbol') }}
      </button>
    </div>

    <!-- Provider Configuration Section -->
    <div v-if="providerConfig" class="provider-section">
      <h2 class="section-title">{{ t('provider.title') }}</h2>

      <div class="provider-cards">
        <!-- Crypto Provider Card -->
        <div class="provider-card">
          <div class="provider-card-header">
            <h3 class="provider-card-title">{{ t('provider.crypto') }}</h3>
          </div>
          <div class="provider-options">
            <label
              v-for="prov in ['nobitex', 'binance', 'both']"
              :key="prov"
              :class="['provider-option', { 'provider-option-active': providerConfig.crypto.active === prov }]"
            >
              <input
                type="radio"
                name="crypto-provider"
                :value="prov"
                :checked="providerConfig.crypto.active === prov"
                :disabled="switchingCrypto"
                @change="handleSwitchCrypto(prov)"
              />
              <div class="provider-option-content">
                <div class="provider-option-header">
                  <span class="provider-option-name">{{ t(`provider.${prov}`) }}</span>
                  <template v-if="prov !== 'both'">
                    <span
                      v-if="(providerConfig.crypto as Record<string, any>)[prov]?.connected"
                      class="provider-status provider-status-connected"
                    >{{ t('provider.connected') }}</span>
                    <span
                      v-else-if="providerConfig.crypto.active === prov || (providerConfig.crypto.active === 'both')"
                      class="provider-status provider-status-disconnected"
                    >{{ t('provider.disconnected') }}</span>
                    <span v-else class="provider-status provider-status-stopped">{{ t('provider.stopped') }}</span>
                  </template>
                </div>
                <p class="provider-option-desc">{{ t(`provider.description.${prov}`) }}</p>
                <div v-if="prov !== 'both' && (providerConfig.crypto as Record<string, any>)[prov]?.tick_count > 0" class="provider-stats">
                  <span>{{ t('provider.tickCount') }}: {{ ((providerConfig.crypto as Record<string, any>)[prov]?.tick_count || 0).toLocaleString() }}</span>
                  <span>{{ t('provider.errorCount') }}: {{ (providerConfig.crypto as Record<string, any>)[prov]?.error_count || 0 }}</span>
                  <span>{{ t('provider.lastTick') }}: {{ formatTickAge((providerConfig.crypto as Record<string, any>)[prov]?.last_tick || 0) }}</span>
                </div>
              </div>
            </label>
          </div>
        </div>

        <!-- Forex Provider Card (interactive) -->
        <div class="provider-card">
          <div class="provider-card-header">
            <h3 class="provider-card-title">{{ t('provider.forex') }}</h3>
          </div>
          <div class="provider-options">
            <label
              v-for="prov in (providerConfig.forex.available.length ? providerConfig.forex.available : ['massive', 'twelvedata', 'finnhub'])"
              :key="prov"
              :class="['provider-option', { 'provider-option-active': providerConfig.forex.active === prov }]"
            >
              <input
                type="radio"
                name="forex-provider"
                :value="prov"
                :checked="providerConfig.forex.active === prov"
                :disabled="switchingForex"
                @change="handleSwitchForex(prov)"
              />
              <div class="provider-option-content">
                <div class="provider-option-header">
                  <span class="provider-option-name">{{ t(`provider.${prov}`) }}</span>
                  <span
                    v-if="providerConfig.forex.active === prov"
                    :class="['provider-status', providerConfig.forex.using_fallback ? 'provider-status-disconnected' : 'provider-status-connected']"
                  >
                    {{ providerConfig.forex.using_fallback ? t('provider.fallback') : t('provider.connected') }}
                  </span>
                  <span v-else class="provider-status provider-status-stopped">{{ t('provider.stopped') }}</span>
                </div>
                <p class="provider-option-desc">{{ t(`provider.description.${prov}`) }}</p>
              </div>
            </label>
          </div>
        </div>
      </div>
    </div>

    <!-- Tabs -->
    <div class="tabs">
      <button
        :class="['tab-btn', { active: activeTab === 'crypto' }]"
        @click="activeTab = 'crypto'"
      >
        {{ t('symbols.tabCrypto') }}
        <span class="tab-count">{{ cryptoSymbols.length }}</span>
      </button>
      <button
        :class="['tab-btn', { active: activeTab === 'forex' }]"
        @click="activeTab = 'forex'"
      >
        {{ t('symbols.tabForex') }}
        <span class="tab-count">{{ forexSymbols.length }}</span>
      </button>
      <button
        :class="['tab-btn', { active: activeTab === 'commodity' }]"
        @click="activeTab = 'commodity'"
      >
        {{ t('symbols.tabCommodity') }}
        <span v-if="!loading" class="tab-count">{{ symbols.filter(s => s.asset_type === 'commodity').length }}</span>
      </button>
      <button
        :class="['tab-btn', { active: activeTab === 'all' }]"
        @click="activeTab = 'all'"
      >
        {{ t('symbols.tabAll') }}
        <span v-if="!loading" class="tab-count">{{ symbols.length }}</span>
      </button>
    </div>

    <!-- Crypto Tab -->
    <div v-if="activeTab === 'crypto'">
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('symbols.symbol') }}</th>
              <th>{{ t('symbols.name') }}</th>
              <th>{{ t('symbols.status') }}</th>
              <th>{{ t('symbols.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="cs in cryptoSymbols" :key="cs.symbol">
              <td class="symbol-cell">{{ cs.symbol }}</td>
              <td>{{ cs.name }}</td>
              <td>
                <template v-if="getCryptoSymbolStatus(cs.symbol)">
                  <span :class="['status-badge', getCryptoSymbolStatus(cs.symbol)!.is_active ? 'status-active' : 'status-inactive']">
                    {{ getCryptoSymbolStatus(cs.symbol)!.is_active ? t('symbols.active') : t('symbols.inactive') }}
                  </span>
                </template>
                <span v-else class="status-badge status-inactive">-</span>
              </td>
              <td>
                <button
                  :class="['btn', 'btn-ghost', 'btn-sm', { 'btn-active-test': testingSymbol === cs.symbol }]"
                  @click="testingSymbol === cs.symbol ? stopTestPrice() : startTestPrice(cs.symbol)"
                >
                  {{ testingSymbol === cs.symbol ? t('symbols.closeTest') : t('symbols.test') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Live Price Panel -->
      <div v-if="testingSymbol && cryptoSymbols.find(c => c.symbol === testingSymbol)" class="test-price-panel">
        <div class="test-price-header">
          <h3 class="test-price-title">
            {{ testingSymbol }} &mdash;
            {{ cryptoSymbols.find(c => c.symbol === testingSymbol)?.name }}
          </h3>
          <button class="btn btn-ghost btn-sm" @click="stopTestPrice">
            {{ t('symbols.closeTest') }}
          </button>
        </div>

        <div v-if="testPriceLoading" class="test-price-loading">
          {{ t('symbols.priceLoading') }}
        </div>

        <div v-else-if="testPriceData" class="test-price-grid">
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.last') }}</span>
            <span class="test-price-value">${{ formatPrice(testPriceData.last) }}</span>
          </div>
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.priceStatus') }}</span>
            <span :class="['price-status-badge', getStatusClass(testPriceData.status)]">
              {{ getStatusLabel(testPriceData.status) }}
            </span>
          </div>
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.bid') }}</span>
            <span class="test-price-value">${{ formatPrice(testPriceData.bid) }}</span>
          </div>
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.ask') }}</span>
            <span class="test-price-value">${{ formatPrice(testPriceData.ask) }}</span>
          </div>
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.age') }}</span>
            <span class="test-price-value">{{ formatAge(testPriceData.age_ms) }}</span>
          </div>
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.provider') }}</span>
            <span class="test-price-value">{{ testPriceData.provider }}</span>
          </div>
        </div>

        <div v-else class="test-price-no-data">
          {{ t('symbols.noPrice') }}
        </div>
      </div>
    </div>

    <!-- Forex Tab -->
    <div v-if="activeTab === 'forex'">
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('symbols.symbol') }}</th>
              <th>{{ t('symbols.name') }}</th>
              <th>{{ t('symbols.status') }}</th>
              <th>{{ t('symbols.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="fs in forexSymbols" :key="fs.symbol">
              <td class="symbol-cell">{{ fs.symbol }}</td>
              <td>{{ fs.name }}</td>
              <td>
                <template v-if="getCryptoSymbolStatus(fs.symbol)">
                  <span :class="['status-badge', getCryptoSymbolStatus(fs.symbol)!.is_active ? 'status-active' : 'status-inactive']">
                    {{ getCryptoSymbolStatus(fs.symbol)!.is_active ? t('symbols.active') : t('symbols.inactive') }}
                  </span>
                </template>
                <span v-else class="status-badge status-inactive">-</span>
              </td>
              <td>
                <button
                  :class="['btn', 'btn-ghost', 'btn-sm', { 'btn-active-test': testingSymbol === fs.symbol }]"
                  @click="testingSymbol === fs.symbol ? stopTestPrice() : startTestPrice(fs.symbol)"
                >
                  {{ testingSymbol === fs.symbol ? t('symbols.closeTest') : t('symbols.test') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Live Price Panel -->
      <div v-if="testingSymbol && forexSymbols.find(f => f.symbol === testingSymbol)" class="test-price-panel">
        <div class="test-price-header">
          <h3 class="test-price-title">
            {{ testingSymbol }} &mdash;
            {{ forexSymbols.find(f => f.symbol === testingSymbol)?.name }}
          </h3>
          <button class="btn btn-ghost btn-sm" @click="stopTestPrice">
            {{ t('symbols.closeTest') }}
          </button>
        </div>

        <div v-if="testPriceLoading" class="test-price-loading">
          {{ t('symbols.priceLoading') }}
        </div>

        <div v-else-if="testPriceData" class="test-price-grid">
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.last') }}</span>
            <span class="test-price-value">${{ formatPrice(testPriceData.last) }}</span>
          </div>
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.priceStatus') }}</span>
            <span :class="['price-status-badge', getStatusClass(testPriceData.status)]">
              {{ getStatusLabel(testPriceData.status) }}
            </span>
          </div>
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.bid') }}</span>
            <span class="test-price-value">${{ formatPrice(testPriceData.bid) }}</span>
          </div>
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.ask') }}</span>
            <span class="test-price-value">${{ formatPrice(testPriceData.ask) }}</span>
          </div>
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.age') }}</span>
            <span class="test-price-value">{{ formatAge(testPriceData.age_ms) }}</span>
          </div>
          <div class="test-price-item">
            <span class="test-price-label">{{ t('symbols.provider') }}</span>
            <span class="test-price-value">{{ testPriceData.provider }}</span>
          </div>
        </div>

        <div v-else class="test-price-no-data">
          {{ t('symbols.noPrice') }}
        </div>
      </div>
    </div>

    <!-- Commodity Tab -->
    <div v-if="activeTab === 'commodity'">
      <div v-if="loading" class="loading">{{ t('common.loading') }}</div>
      <div v-else class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('symbols.symbol') }}</th>
              <th>{{ t('symbols.name') }}</th>
              <th>{{ t('symbols.status') }}</th>
              <th v-if="canManage">{{ t('symbols.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="symbol in symbols.filter(s => s.asset_type === 'commodity')" :key="symbol.symbol">
              <td class="symbol-cell">{{ symbol.symbol }}</td>
              <td>{{ symbol.name }}</td>
              <td>
                <span :class="['status-badge', symbol.is_active ? 'status-active' : 'status-inactive']">
                  {{ symbol.is_active ? t('symbols.active') : t('symbols.inactive') }}
                </span>
              </td>
              <td v-if="canManage" class="actions-cell">
                <button class="btn btn-ghost btn-sm" @click="openEditModal(symbol)">{{ t('symbols.edit') }}</button>
              </td>
            </tr>
            <tr v-if="symbols.filter(s => s.asset_type === 'commodity').length === 0">
              <td colspan="4" class="no-results">{{ t('symbols.noResults') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- All Symbols Tab -->
    <div v-if="activeTab === 'all'">
      <div class="filters">
        <input
          v-model="searchQuery"
          type="text"
          class="input search-input"
          :placeholder="t('symbols.search')"
        />
        <select v-model="assetTypeFilter" class="input type-select">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="type in assetTypes" :key="type" :value="type">
            {{ t(`symbols.type.${type}`) }}
          </option>
        </select>
        <select v-model="activeFilter" class="input status-select">
          <option value="">{{ t('symbols.allStatuses') }}</option>
          <option value="true">{{ t('symbols.active') }}</option>
          <option value="false">{{ t('symbols.inactive') }}</option>
        </select>
      </div>

      <div v-if="loading" class="loading">
        {{ t('common.loading') }}
      </div>

      <div v-else-if="error" class="error-state">
        <p>{{ error }}</p>
        <button class="btn btn-primary" @click="fetchSymbols">{{ t('common.retry') }}</button>
      </div>

      <div v-else-if="filteredSymbols.length === 0" class="no-results">
        {{ t('symbols.noResults') }}
      </div>

      <div v-else class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('symbols.symbol') }}</th>
              <th>{{ t('symbols.name') }}</th>
              <th>{{ t('symbols.assetType') }}</th>
              <th>{{ t('symbols.status') }}</th>
              <th>{{ t('symbols.addedDate') }}</th>
              <th v-if="canManage">{{ t('symbols.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="symbol in filteredSymbols" :key="symbol.symbol">
              <td class="symbol-cell">{{ symbol.symbol }}</td>
              <td>{{ symbol.name }}</td>
              <td>
                <span :class="['type-badge', getAssetTypeClass(symbol.asset_type)]">
                  {{ t(`symbols.type.${symbol.asset_type}`) }}
                </span>
              </td>
              <td>
                <span :class="['status-badge', symbol.is_active ? 'status-active' : 'status-inactive']">
                  {{ symbol.is_active ? t('symbols.active') : t('symbols.inactive') }}
                </span>
              </td>
              <td>{{ formatDate(symbol.created_at) }}</td>
              <td v-if="canManage" class="actions-cell">
                <button class="btn btn-ghost btn-sm" @click="openEditModal(symbol)">
                  {{ t('symbols.edit') }}
                </button>
                <button
                  :class="['btn', 'btn-ghost', 'btn-sm', symbol.is_active ? 'btn-warning' : 'btn-success']"
                  @click="toggleActive(symbol)"
                >
                  {{ symbol.is_active ? t('symbols.deactivate') : t('symbols.activate') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Add Symbol Modal -->
    <div v-if="showAddModal" class="modal-overlay" @click.self="closeAddModal">
      <div class="modal">
        <div class="modal-header">
          <h2 class="modal-title">{{ t('symbols.addTitle') }}</h2>
          <button class="modal-close" @click="closeAddModal">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label class="form-label">{{ t('symbols.symbol') }} *</label>
            <input
              v-model="form.symbol"
              type="text"
              class="input"
              :placeholder="t('symbols.symbolPlaceholder')"
              maxlength="20"
            />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('symbols.name') }} *</label>
            <input
              v-model="form.name"
              type="text"
              class="input"
              :placeholder="t('symbols.namePlaceholder')"
              maxlength="100"
            />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('symbols.assetType') }}</label>
            <select v-model="form.asset_type" class="input">
              <option v-for="type in assetTypes" :key="type" :value="type">
                {{ t(`symbols.type.${type}`) }}
              </option>
            </select>
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('symbols.providerTwelveData') }}</label>
            <input
              v-model="form.provider_symbol_twelvedata"
              type="text"
              class="input"
              :placeholder="t('symbols.providerPlaceholder')"
            />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('symbols.providerMassive') }}</label>
            <input
              v-model="form.provider_symbol_massive"
              type="text"
              class="input"
              :placeholder="t('symbols.providerPlaceholder')"
            />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('symbols.providerFinnhub') }}</label>
            <input
              v-model="form.provider_symbol_finnhub"
              type="text"
              class="input"
              :placeholder="t('symbols.providerPlaceholder')"
            />
          </div>
          <div class="form-group checkbox-group">
            <label class="checkbox-label">
              <input v-model="form.is_active" type="checkbox" class="checkbox" />
              {{ t('symbols.isActive') }}
            </label>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeAddModal">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="submitting" @click="handleAddSymbol">
            {{ submitting ? t('common.loading') : t('symbols.add') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Edit Symbol Modal -->
    <div v-if="showEditModal" class="modal-overlay" @click.self="closeEditModal">
      <div class="modal">
        <div class="modal-header">
          <h2 class="modal-title">{{ t('symbols.editTitle') }}</h2>
          <button class="modal-close" @click="closeEditModal">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label class="form-label">{{ t('symbols.symbol') }}</label>
            <input
              :value="form.symbol"
              type="text"
              class="input"
              disabled
            />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('symbols.name') }} *</label>
            <input
              v-model="form.name"
              type="text"
              class="input"
              :placeholder="t('symbols.namePlaceholder')"
              maxlength="100"
            />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('symbols.assetType') }}</label>
            <select v-model="form.asset_type" class="input">
              <option v-for="type in assetTypes" :key="type" :value="type">
                {{ t(`symbols.type.${type}`) }}
              </option>
            </select>
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('symbols.providerTwelveData') }}</label>
            <input
              v-model="form.provider_symbol_twelvedata"
              type="text"
              class="input"
              :placeholder="t('symbols.providerPlaceholder')"
            />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('symbols.providerMassive') }}</label>
            <input
              v-model="form.provider_symbol_massive"
              type="text"
              class="input"
              :placeholder="t('symbols.providerPlaceholder')"
            />
          </div>
          <div class="form-group">
            <label class="form-label">{{ t('symbols.providerFinnhub') }}</label>
            <input
              v-model="form.provider_symbol_finnhub"
              type="text"
              class="input"
              :placeholder="t('symbols.providerPlaceholder')"
            />
          </div>
          <div class="form-group checkbox-group">
            <label class="checkbox-label">
              <input v-model="form.is_active" type="checkbox" class="checkbox" />
              {{ t('symbols.isActive') }}
            </label>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeEditModal">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="submitting" @click="handleEditSymbol">
            {{ submitting ? t('common.loading') : t('common.save') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.symbols-page {
  padding: var(--spacing-lg) 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xl);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

/* Tabs */
.tabs {
  display: flex;
  gap: var(--spacing-xs);
  margin-bottom: var(--spacing-lg);
  border-bottom: 2px solid var(--color-border);
}

.tab-btn {
  padding: var(--spacing-sm) var(--spacing-lg);
  border: none;
  background: none;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  transition: color 0.2s, border-color 0.2s;
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.tab-btn:hover {
  color: var(--color-text-primary);
}

.tab-btn.active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
  font-weight: 600;
}

.tab-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 var(--spacing-xs);
  border-radius: var(--radius-full);
  background-color: var(--color-bg-secondary);
  font-size: var(--font-size-xs);
  font-weight: 600;
}

.tab-btn.active .tab-count {
  background-color: var(--color-primary);
  color: white;
}

/* Test Price Panel */
.test-price-panel {
  margin-top: var(--spacing-lg);
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
}

.test-price-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-md);
}

.test-price-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.test-price-loading,
.test-price-no-data {
  text-align: center;
  padding: var(--spacing-lg);
  color: var(--color-text-secondary);
}

.test-price-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-md);
}

.test-price-item {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.test-price-label {
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-secondary);
  text-transform: uppercase;
}

.test-price-value {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  font-family: var(--font-family-mono);
}

/* Price Status Badges */
.price-status-badge {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.price-status-fresh {
  background-color: #DCFCE7;
  color: #16A34A;
}

.price-status-warning {
  background-color: #FEF3C7;
  color: #D97706;
}

.price-status-stale {
  background-color: #FEE2E2;
  color: #DC2626;
}

.price-status-nodata {
  background-color: #F3F4F6;
  color: #6B7280;
}

.btn-active-test {
  color: var(--color-primary);
  font-weight: 600;
}

/* Filters */
.filters {
  display: flex;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.search-input {
  flex: 1;
  max-width: 300px;
}

.type-select,
.status-select {
  width: 150px;
}

.loading,
.no-results,
.error-state {
  text-align: center;
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
}

.error-state p {
  margin: 0;
}

.table-container {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: var(--spacing-md);
  text-align: left;
  border-bottom: 1px solid var(--color-border);
}

[dir="rtl"] .data-table th,
[dir="rtl"] .data-table td {
  text-align: right;
}

.data-table th {
  background-color: var(--color-bg-secondary);
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.data-table tbody tr:hover {
  background-color: var(--color-bg-secondary);
}

.symbol-cell {
  font-family: var(--font-family-mono);
  font-weight: 600;
  color: var(--color-text-primary);
}

.type-badge {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
}

.type-stock {
  background-color: #DBEAFE;
  color: #2563EB;
}

.type-crypto {
  background-color: #FEF3C7;
  color: #D97706;
}

.type-forex {
  background-color: #D1FAE5;
  color: #059669;
}

.type-commodity {
  background-color: #F3E8FF;
  color: #7C3AED;
}

.status-badge {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
}

.status-active {
  background-color: #DCFCE7;
  color: #16A34A;
}

.status-inactive {
  background-color: #F3F4F6;
  color: #6B7280;
}

.actions-cell {
  white-space: nowrap;
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
}

.btn-warning {
  color: #D97706;
}

.btn-warning:hover {
  background-color: #FEF3C7;
}

.btn-success {
  color: #16A34A;
}

.btn-success:hover {
  background-color: #DCFCE7;
}

/* Modal Styles */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 500px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: var(--shadow-lg);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.modal-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.modal-close {
  background: none;
  border: none;
  font-size: var(--font-size-2xl);
  color: var(--color-text-secondary);
  cursor: pointer;
  padding: 0;
  line-height: 1;
}

.modal-close:hover {
  color: var(--color-text-primary);
}

.modal-body {
  padding: var(--spacing-lg);
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

.form-group {
  margin-bottom: var(--spacing-md);
}

.form-label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-xs);
}

.checkbox-group {
  display: flex;
  align-items: center;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  cursor: pointer;
}

.checkbox {
  width: 16px;
  height: 16px;
}

/* Provider Configuration */
.provider-section {
  margin-bottom: var(--spacing-xl);
}

.section-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-md);
}

.provider-cards {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--spacing-lg);
}

.provider-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
}

.provider-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-md);
}

.provider-card-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.provider-options {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.provider-option {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: border-color 0.2s, background-color 0.2s;
}

.provider-option:hover {
  border-color: var(--color-primary);
  background-color: var(--color-bg-secondary);
}

.provider-option-active {
  border-color: var(--color-primary);
  background-color: rgba(59, 130, 246, 0.05);
}

.provider-option input[type="radio"] {
  margin-top: 3px;
  flex-shrink: 0;
}

.provider-option-content {
  flex: 1;
  min-width: 0;
}

.provider-option-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xs);
}

.provider-option-name {
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.provider-option-desc {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  margin: 0 0 var(--spacing-xs) 0;
  line-height: 1.4;
}

.provider-status {
  font-size: var(--font-size-xs);
  font-weight: 600;
  padding: 2px var(--spacing-sm);
  border-radius: var(--radius-full);
}

.provider-status-connected {
  background-color: #DCFCE7;
  color: #16A34A;
}

.provider-status-disconnected {
  background-color: #FEE2E2;
  color: #DC2626;
}

.provider-status-stopped {
  background-color: #F3F4F6;
  color: #6B7280;
}

.provider-stats {
  display: flex;
  gap: var(--spacing-md);
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  font-family: var(--font-family-mono);
}

@media (max-width: 767px) {
  .page-header {
    flex-direction: column;
    gap: var(--spacing-md);
    align-items: stretch;
  }

  .filters {
    flex-direction: column;
  }

  .search-input,
  .type-select,
  .status-select {
    max-width: none;
    width: 100%;
  }

  .table-container {
    overflow-x: auto;
  }

  .data-table {
    min-width: 600px;
  }

  .test-price-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .provider-cards {
    grid-template-columns: 1fr;
  }

  .provider-stats {
    flex-direction: column;
    gap: var(--spacing-xs);
  }

  .modal {
    margin: var(--spacing-md);
    max-width: calc(100% - var(--spacing-lg));
  }
}
</style>
