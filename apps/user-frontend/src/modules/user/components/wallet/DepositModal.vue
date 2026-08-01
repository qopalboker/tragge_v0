<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue';
import { t } from '@/i18n';
import { useWalletStore } from '@/stores/wallet';
import { useI18nStore } from '@/stores/i18n';
import { useToast } from '@/composables/useToast';
import { walletApi } from '@/api';

const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
}>();

const walletStore = useWalletStore();
const i18nStore = useI18nStore();
const toast = useToast();

const locale = computed(() => i18nStore.locale);

// Steps
type Step = 'method' | 'form';
const step = ref<Step>('method');

// Payment method
type PaymentMethod = 'crypto' | 'fiat';
const paymentMethod = ref<PaymentMethod>('crypto');

// Amount
const amount = ref<number | null>(null);
const depositLoading = ref(false);
const MIN_DEPOSIT = 5;
const MAX_DEPOSIT = 10000;
const PRESET_AMOUNTS = [10, 20, 50];

// Nobitex USDT price (for fiat)
const usdtPriceToman = ref<number | null>(null);
const usdtPriceLoading = ref(false);
let nobitexInterval: ReturnType<typeof setInterval> | null = null;

const showFiatOption = computed(() => locale.value === 'fa');

const equivalentToman = computed(() => {
  if (!amount.value || !usdtPriceToman.value) return null;
  return Math.round(amount.value * usdtPriceToman.value);
});

const isAmountValid = computed(() => {
  return amount.value !== null && amount.value >= MIN_DEPOSIT && amount.value <= MAX_DEPOSIT;
});

// Fetch USD/Toman exchange rate via backend
async function fetchNobitexPrice(): Promise<void> {
  usdtPriceLoading.value = true;
  try {
    const rate = await walletApi.getExchangeRate();
    usdtPriceToman.value = Math.round(rate.usd_to_irt);
  } catch {
    // silently fail, user can still deposit without rate
  } finally {
    usdtPriceLoading.value = false;
  }
}

function startNobitexPolling(): void {
  stopNobitexPolling();
  fetchNobitexPrice();
  nobitexInterval = setInterval(fetchNobitexPrice, 60_000);
}

function stopNobitexPolling(): void {
  if (nobitexInterval) {
    clearInterval(nobitexInterval);
    nobitexInterval = null;
  }
}

// Watch for fiat selection to start polling
watch(() => paymentMethod.value, (method) => {
  if (method === 'fiat' && props.show) {
    startNobitexPolling();
  } else {
    stopNobitexPolling();
  }
});

// Watch modal visibility
watch(() => props.show, (visible) => {
  if (visible) {
    step.value = 'method';
    paymentMethod.value = 'crypto';
    amount.value = null;
    depositLoading.value = false;
  } else {
    stopNobitexPolling();
  }
});

onBeforeUnmount(() => {
  stopNobitexPolling();
});

function closeModal(): void {
  emit('update:show', false);
}

function selectMethod(method: PaymentMethod): void {
  paymentMethod.value = method;
  step.value = 'form';
}

function setPresetAmount(val: number): void {
  amount.value = val;
}

function goBack(): void {
  if (showFiatOption.value) {
    step.value = 'method';
  }
}

async function handleDeposit(): Promise<void> {
  if (!isAmountValid.value || !amount.value) return;

  depositLoading.value = true;

  try {
    const amountCents = Math.round(amount.value * 100);

    if (paymentMethod.value === 'fiat') {
      const result = await walletStore.createFiatDeposit(amountCents);
      if (result?.payment_url) {
        window.location.href = result.payment_url;
      } else if (walletStore.error) {
        toast.error(walletStore.error);
      }
    } else {
      const result = await walletStore.createCryptoDeposit(amountCents);
      if (result?.payment_url) {
        window.open(result.payment_url, '_blank');
        closeModal();
      } else if (walletStore.error) {
        toast.error(walletStore.error);
      }
    }
  } finally {
    depositLoading.value = false;
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
    <div v-if="show" class="modal-overlay" @click.self="closeModal">
      <div class="modal-content">
        <!-- Header -->
        <div class="modal-header">
          <button v-if="step === 'form' && showFiatOption" class="modal-back" @click="goBack">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="15 18 9 12 15 6" />
            </svg>
          </button>
          <h3 class="modal-title">{{ t('wallet.depositModal.title') }}</h3>
          <button class="modal-close" @click="closeModal">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        <!-- Step 1: Method Selection -->
        <div v-if="step === 'method' && showFiatOption" class="modal-body">
          <p class="step-label">{{ t('wallet.depositModal.selectMethod') }}</p>
          <div class="method-options">
            <button class="method-card" @click="selectMethod('crypto')">
              <div class="method-icon crypto-icon">
                <svg width="28" height="28" viewBox="0 0 24 24" fill="none">
                  <circle cx="12" cy="12" r="10" fill="#26A17B" />
                  <path d="M13.5 11.5v-1h2v-2h-7v2h2v1c-2.5.2-4.5.8-4.5 1.5s2 1.3 4.5 1.5v3h3v-3c2.5-.2 4.5-.8 4.5-1.5s-2-1.3-4.5-1.5z" fill="white" />
                </svg>
              </div>
              <span class="method-name">{{ t('wallet.depositModal.cryptoMethod') }}</span>
              <span class="method-desc">{{ t('wallet.depositModal.cryptoDesc') }}</span>
            </button>
            <button class="method-card" @click="selectMethod('fiat')">
              <div class="method-icon fiat-icon">
                <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M3 21h18" />
                  <path d="M3 10h18" />
                  <path d="M5 6l7-3 7 3" />
                  <path d="M4 10v11" />
                  <path d="M20 10v11" />
                  <path d="M8 14v3" />
                  <path d="M12 14v3" />
                  <path d="M16 14v3" />
                </svg>
              </div>
              <span class="method-name">{{ t('wallet.depositModal.fiatMethod') }}</span>
              <span class="method-desc">{{ t('wallet.depositModal.fiatDesc') }}</span>
            </button>
          </div>
        </div>

        <!-- Step 2: Deposit Form (or directly if no fiat option) -->
        <div v-if="step === 'form' || !showFiatOption" class="modal-body">
          <!-- Preset Amounts -->
          <div class="preset-amounts">
            <button
              v-for="preset in PRESET_AMOUNTS"
              :key="preset"
              :class="['preset-btn', { active: amount === preset }]"
              @click="setPresetAmount(preset)"
            >
              ${{ preset }}
            </button>
          </div>

          <!-- Amount Input -->
          <div class="form-group">
            <label class="form-label">{{ t('wallet.depositModal.amountLabel') }}</label>
            <div class="amount-input-wrapper">
              <span class="currency-symbol">$</span>
              <input
                v-model.number="amount"
                type="number"
                class="amount-input"
                :min="MIN_DEPOSIT"
                :max="MAX_DEPOSIT"
                placeholder="0.00"
                step="0.01"
              />
              <span class="input-suffix">USD</span>
            </div>
            <span class="form-hint">
              {{ t('wallet.depositModal.minDeposit', { amount: `$${MIN_DEPOSIT}` }) }} · {{ t('wallet.depositModal.maxDeposit', { amount: `$${MAX_DEPOSIT.toLocaleString()}` }) }}
            </span>
          </div>

          <!-- Toman Equivalent (fiat only) -->
          <div v-if="paymentMethod === 'fiat'" class="toman-section">
            <div v-if="usdtPriceLoading" class="toman-loading">
              <div class="spinner-sm"></div>
              <span>{{ t('wallet.depositModal.loadingRate') }}</span>
            </div>
            <div v-else-if="equivalentToman" class="toman-info">
              <div class="toman-row">
                <span class="toman-label">{{ t('wallet.depositModal.equivalentToman') }}</span>
                <span class="toman-value" dir="ltr">{{ equivalentToman.toLocaleString('fa-IR') }} {{ t('wallet.depositModal.toman') }}</span>
              </div>
              <div v-if="usdtPriceToman" class="rate-row">
                <span class="rate-text">{{ t('wallet.depositModal.usdtRate', { rate: usdtPriceToman.toLocaleString('fa-IR') }) }}</span>
              </div>
            </div>
          </div>

          <!-- Submit -->
          <button
            class="btn btn-primary btn-lg btn-full"
            :disabled="!isAmountValid || depositLoading"
            @click="handleDeposit"
          >
            <span v-if="depositLoading" class="btn-spinner"></span>
            <span v-else>
              {{ paymentMethod === 'fiat' ? t('wallet.depositModal.payViaGateway') : t('wallet.depositModal.payViaCrypto') }}
            </span>
          </button>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: var(--z-modal, 1000);
  padding: var(--spacing-md);
  backdrop-filter: blur(2px);
}

.modal-content {
  background: var(--color-bg-primary);
  border-radius: var(--radius-xl);
  max-width: 440px;
  width: 100%;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.2);
  animation: modalSlideIn 0.2s ease-out;
}

@keyframes modalSlideIn {
  from {
    opacity: 0;
    transform: translateY(-20px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

.modal-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.modal-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
  flex: 1;
}

.modal-back {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
  flex-shrink: 0;
}

.modal-back:hover {
  background: var(--color-bg-secondary);
}

.modal-close {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
  flex-shrink: 0;
}

.modal-close:hover {
  background: var(--color-bg-secondary);
}

.modal-close svg {
  width: 20px;
  height: 20px;
}

.modal-body {
  padding: var(--spacing-lg);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

/* Step 1: Method Selection */
.step-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
}

.method-options {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.method-card {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  background: var(--color-bg-primary);
  cursor: pointer;
  transition: all var(--transition-fast);
  text-align: start;
}

.method-card:hover {
  border-color: var(--color-primary);
  background: var(--color-bg-secondary);
}

.method-icon {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-lg);
  flex-shrink: 0;
}

.method-icon.crypto-icon {
  background: rgba(38, 161, 123, 0.1);
}

.method-icon.fiat-icon {
  background: rgba(99, 102, 241, 0.1);
  color: var(--color-primary);
}

.method-name {
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  display: block;
}

.method-desc {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

/* Step 2: Form */
.preset-amounts {
  display: flex;
  gap: var(--spacing-sm);
}

.preset-btn {
  flex: 1;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg-primary);
  color: var(--color-text-primary);
  font-weight: 600;
  font-size: var(--font-size-sm);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.preset-btn:hover {
  border-color: var(--color-primary);
}

.preset-btn.active {
  border-color: var(--color-primary);
  background: rgba(99, 102, 241, 0.1);
  color: var(--color-primary);
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.form-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.amount-input-wrapper {
  display: flex;
  align-items: center;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  overflow: hidden;
  transition: border-color var(--transition-fast);
}

.amount-input-wrapper:focus-within {
  border-color: var(--color-primary);
}

.currency-symbol {
  padding: 0 var(--spacing-sm) 0 var(--spacing-md);
  color: var(--color-text-secondary);
  font-weight: 600;
}

.amount-input {
  flex: 1;
  border: none;
  outline: none;
  padding: var(--spacing-sm) 0;
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  background: transparent;
  min-width: 0;
  -moz-appearance: textfield;
}

.amount-input::-webkit-outer-spin-button,
.amount-input::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

.input-suffix {
  padding: 0 var(--spacing-md) 0 var(--spacing-sm);
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
  font-weight: 500;
}

.form-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

/* Toman section */
.toman-section {
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.toman-loading {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.spinner-sm {
  width: 14px;
  height: 14px;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.toman-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.toman-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.toman-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.toman-value {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
}

.rate-row {
  text-align: end;
}

.rate-text {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

/* Button */
.btn-full {
  width: 100%;
}

.btn-lg {
  padding: var(--spacing-sm) var(--spacing-lg);
  font-size: var(--font-size-md);
}

.btn-spinner {
  width: 18px;
  height: 18px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
  display: inline-block;
}

/* Transition */
.modal-enter-active { transition: opacity 0.2s ease; }
.modal-leave-active { transition: opacity 0.15s ease; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-active .modal-content { transition: transform 0.2s ease; }
.modal-leave-active .modal-content { transition: transform 0.15s ease; }
.modal-enter-from .modal-content { transform: translateY(16px) scale(0.97); }
.modal-leave-to .modal-content { transform: translateY(8px) scale(0.98); }

/* Mobile */
@media (max-width: 480px) {
  .modal-content {
    max-height: 100vh;
    border-radius: var(--radius-lg) var(--radius-lg) 0 0;
    margin-top: auto;
  }

  .modal-overlay {
    align-items: flex-end;
  }
}
</style>
