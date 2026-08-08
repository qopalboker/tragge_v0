<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useRoute } from 'vue-router';
import { t } from '@/i18n';
import { useI18nStore } from '@/stores/i18n';
import { api } from '@/api';
import { getAccessToken } from '@/api/client';
import type { AxiosError } from 'axios';

const route = useRoute();
const i18nStore = useI18nStore();

type PageState = 'pending' | 'success' | 'failed' | 'auth_error';

const state = ref<PageState>('pending');
const pollingTimedOut = ref(false);
const retryTimer = ref<ReturnType<typeof setInterval> | null>(null);
const elapsedSeconds = ref(0);

// Crypto payment metadata (populated from polling response)
const paymentNetwork = ref('');
const cryptoAmount = ref('');
const cryptoCurrency = ref('');
const txHash = ref('');

const direction = computed(() => i18nStore.direction);

const loginUrl = computed(() => {
  const currentPath = window.location.pathname + window.location.search;
  return `/user/login?redirect=${encodeURIComponent(currentPath)}`;
});

const MAX_POLL_SECONDS = 30;
const POLL_INTERVAL_MS = 3000;

onMounted(() => {
  document.documentElement.dir = direction.value;

	const status = route.query.status as string | undefined;
	const purchaseId = route.query.purchase_id as string | undefined;
	const effectiveId = purchaseId;

  if (status === '1' || status === 'PAID' || status === 'CONFIRMED') {
    state.value = 'success';
    // If we have an ID, poll once to get metadata
    if (effectiveId) {
      fetchMetadata(effectiveId);
    }
  } else if (status === '0' || status === 'FAILED' || status === 'CANCELLED' || status === 'EXPIRED') {
    state.value = 'failed';
  } else if (effectiveId) {
    // Unknown status — poll for result (requires authentication)
    if (!getAccessToken()) {
      state.value = 'auth_error';
      return;
    }
    state.value = 'pending';
    startPolling(effectiveId);
  } else {
    // No useful params at all
    state.value = 'failed';
  }
});

onUnmounted(() => {
  stopPolling();
});

interface StatusResponse {
  status?: string;
  provider?: string;
  network?: string;
  crypto_amount?: number;
  crypto_currency?: string;
  tx_hash?: string;
}

function extractMetadata(data: StatusResponse): void {
  if (data.network) paymentNetwork.value = data.network;
  if (data.crypto_amount) cryptoAmount.value = data.crypto_amount.toString();
  if (data.crypto_currency) cryptoCurrency.value = data.crypto_currency;
  if (data.tx_hash) txHash.value = data.tx_hash;
}

function startPolling(purchaseId: string): void {
  retryTimer.value = setInterval(async () => {
    elapsedSeconds.value += POLL_INTERVAL_MS / 1000;

    try {
      const res = await api.get(`/api/payments/status/${encodeURIComponent(purchaseId)}`);
      const data = res.data as StatusResponse;

      extractMetadata(data);

      if (data.status === 'succeeded') {
        state.value = 'success';
        stopPolling();
      } else if (data.status === 'failed' || data.status === 'expired' || data.status === 'refunded') {
        state.value = 'failed';
        stopPolling();
      }
    } catch (err) {
      const axiosErr = err as AxiosError;
      if (axiosErr.response?.status === 401 || axiosErr.response?.status === 403) {
        state.value = 'auth_error';
        stopPolling();
        return;
      }
      // Ignore other transient errors during polling — keep trying
    }

    if (elapsedSeconds.value >= MAX_POLL_SECONDS) {
      // Timeout — payment may still be processing, don't show failure
      state.value = 'pending';
      pollingTimedOut.value = true;
      stopPolling();
    }
  }, POLL_INTERVAL_MS);
}

async function fetchMetadata(id: string): Promise<void> {
  if (!getAccessToken()) return;
  try {
    const res = await api.get(`/api/payments/status/${encodeURIComponent(id)}`);
    extractMetadata(res.data as StatusResponse);
  } catch {
    // Non-critical — metadata display is optional
  }
}

function stopPolling(): void {
  if (retryTimer.value) {
    clearInterval(retryTimer.value);
    retryTimer.value = null;
  }
}

function getExplorerUrl(hash: string, network: string): string {
  const n = (network || '').toLowerCase();
  if (n.includes('trc20') || n.includes('tron')) {
    return `https://tronscan.org/#/transaction/${hash}`;
  }
  if (n.includes('erc20') || n.includes('ethereum') || n.includes('eth')) {
    return `https://etherscan.io/tx/${hash}`;
  }
  if (n.includes('bsc') || n.includes('bep20') || n.includes('bnb')) {
    return `https://bscscan.com/tx/${hash}`;
  }
  return '#';
}

const hasCryptoMeta = computed(() => !!(paymentNetwork.value || cryptoAmount.value || txHash.value));

function toggleLanguage(): void {
  i18nStore.toggleLocale();
}
</script>

<template>
  <div :class="['payment-result-page', { 'rtl': direction === 'rtl' }]">
    <div class="payment-result-container">
      <!-- Logo -->
      <div class="logo">
        <svg width="48" height="48" viewBox="0 0 32 32" fill="none">
          <rect width="32" height="32" rx="4" fill="var(--color-primary)" />
          <path d="M8 22L12 14L18 18L24 10" stroke="white" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        <span class="logo-text">Tragge</span>
      </div>

      <!-- Pending / Verifying State -->
      <div v-if="state === 'pending'" class="status-card">
        <template v-if="pollingTimedOut">
          <div class="status-icon warning">
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 9v4m0 4h.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
            </svg>
          </div>
          <h1 class="status-title">{{ t('payment.processingTitle') }}</h1>
          <p class="status-message">{{ t('payment.processingDescription') }}</p>
          <a href="/user/wallet" class="btn btn-primary">
            {{ t('payment.goToWallet') }}
          </a>
        </template>
        <template v-else>
          <div class="loading-icon">
            <div class="spinner-large"></div>
          </div>
          <h1 class="status-title">{{ t('payment.verifying') }}</h1>
          <p class="status-message">{{ t('payment.pending') }}</p>
        </template>
      </div>

      <!-- Success State -->
      <div v-else-if="state === 'success'" class="status-card">
        <div class="status-icon success">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
            <polyline points="22,4 12,14.01 9,11.01" />
          </svg>
        </div>
        <h1 class="status-title">{{ t('payment.success') }}</h1>
        <p class="status-message">{{ t('payment.successDescription') }}</p>

        <!-- Crypto Payment Details -->
        <div v-if="hasCryptoMeta" class="payment-details">
          <div v-if="paymentNetwork" class="detail-row">
            <span class="detail-label">{{ t('payment.network') }}</span>
            <span class="detail-value">{{ paymentNetwork }}</span>
          </div>
          <div v-if="cryptoAmount && cryptoCurrency" class="detail-row">
            <span class="detail-label">{{ t('payment.cryptoAmount') }}</span>
            <span class="detail-value">{{ cryptoAmount }} {{ cryptoCurrency }}</span>
          </div>
          <div v-if="txHash" class="detail-row">
            <span class="detail-label">{{ t('payment.txHash') }}</span>
            <a :href="getExplorerUrl(txHash, paymentNetwork)" target="_blank" rel="noopener" class="detail-value tx-hash-link">
              {{ txHash.slice(0, 8) }}...{{ txHash.slice(-8) }}
            </a>
          </div>
        </div>

        <a href="/user/wallet" class="btn btn-primary">
          {{ t('payment.goToWallet') }}
        </a>
      </div>

      <!-- Failed State -->
      <div v-else-if="state === 'failed'" class="status-card">
        <div class="status-icon error">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="15" y1="9" x2="9" y2="15" />
            <line x1="9" y1="9" x2="15" y2="15" />
          </svg>
        </div>
        <h1 class="status-title">{{ t('payment.failed') }}</h1>
        <p class="status-message">{{ t('payment.failedDescription') }}</p>
        <a href="/user/wallet" class="btn btn-primary">
          {{ t('payment.tryAgain') }}
        </a>
      </div>

      <!-- Session Expired State -->
      <div v-else-if="state === 'auth_error'" class="status-card">
        <div class="status-icon warning">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 9v4m0 4h.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
          </svg>
        </div>
        <h1 class="status-title">{{ t('payment.sessionExpired') }}</h1>
        <p class="status-message">{{ t('payment.sessionExpiredDescription') }}</p>
        <a :href="loginUrl" class="btn btn-primary">
          {{ t('payment.login') }}
        </a>
      </div>

      <!-- Language Toggle -->
      <button class="lang-toggle" @click="toggleLanguage">
        {{ i18nStore.locale === 'en' ? 'فارسی' : 'English' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.payment-result-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, var(--color-bg-secondary) 0%, var(--color-bg-tertiary) 100%);
  padding: var(--spacing-lg);
}

.payment-result-page.rtl {
  direction: rtl;
}

.payment-result-container {
  width: 100%;
  max-width: 400px;
}

.logo {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-2xl);
}

.logo-text {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.status-card {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-xl);
  padding: var(--spacing-2xl);
  box-shadow: var(--shadow-lg);
  text-align: center;
}

.loading-icon {
  width: 80px;
  height: 80px;
  margin: 0 auto var(--spacing-lg);
  display: flex;
  align-items: center;
  justify-content: center;
}

.spinner-large {
  width: 48px;
  height: 48px;
  border: 4px solid var(--color-bg-tertiary);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

.status-icon {
  width: 80px;
  height: 80px;
  margin: 0 auto var(--spacing-lg);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.status-icon.success {
  background-color: #D1FAE5;
  color: #059669;
}

.status-icon.error {
  background-color: #FEE2E2;
  color: #DC2626;
}

.status-icon.warning {
  background-color: #FEF3C7;
  color: #D97706;
}

.status-title {
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-sm);
}

.status-message {
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-xl);
}

/* Crypto Payment Details */
.payment-details {
  background-color: var(--color-bg-secondary, #f5f5f5);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
  text-align: start;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  padding: var(--spacing-xs) 0;
  font-size: var(--font-size-sm);
}

.detail-row + .detail-row {
  border-top: 1px solid var(--color-border, #e5e7eb);
}

.detail-label {
  color: var(--color-text-secondary);
}

.detail-value {
  color: var(--color-text-primary);
  font-weight: 500;
}

.tx-hash-link {
  color: var(--color-primary);
  text-decoration: none;
}

.tx-hash-link:hover {
  text-decoration: underline;
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-lg);
  border-radius: var(--radius-md);
  font-size: var(--font-size-md);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
  border: none;
  text-decoration: none;
}

.btn-primary {
  background-color: var(--color-primary);
  color: white;
}

.btn-primary:hover {
  background-color: var(--color-primary-dark);
}

/* Language Toggle */
.lang-toggle {
  display: block;
  margin: var(--spacing-lg) auto 0;
  padding: var(--spacing-sm) var(--spacing-md);
  background: none;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.lang-toggle:hover {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
