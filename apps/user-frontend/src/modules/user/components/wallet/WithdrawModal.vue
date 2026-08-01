<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { t } from '@/i18n';
import { useRouter } from 'vue-router';
import { useWalletStore } from '@/stores/wallet';
import { useToast } from '@/composables/useToast';

const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
}>();

const walletStore = useWalletStore();
const toast = useToast();
const router = useRouter();

const MIN_WITHDRAW = 10;
const TRC20_REGEX = /^T[1-9A-HJ-NP-Za-km-z]{33}$/;

const amount = ref<number | null>(null);
const walletAddress = ref('');
const withdrawLoading = ref(false);
const addressTouched = ref(false);

const needsKYC = computed(() => {
  return walletStore.kycRequiredForWithdrawal && !walletStore.isKYCVerified;
});

const isAddressValid = computed(() => {
  return TRC20_REGEX.test(walletAddress.value.trim());
});

const addressError = computed(() => {
  if (!addressTouched.value || !walletAddress.value) return null;
  if (!isAddressValid.value) return t('wallet.withdrawModal.invalidAddress');
  return null;
});

const isAmountValid = computed(() => {
  if (!amount.value || amount.value <= 0) return false;
  if (amount.value < MIN_WITHDRAW) return false;
  if (amount.value > walletStore.availableBalance) return false;
  return true;
});

const amountError = computed(() => {
  if (!amount.value || amount.value <= 0) return null;
  if (amount.value < MIN_WITHDRAW) return t('wallet.minimumWithdraw', { amount: `$${MIN_WITHDRAW}` });
  if (amount.value > walletStore.availableBalance) return t('wallet.insufficientBalanceError');
  return null;
});

const isFormValid = computed(() => {
  return isAmountValid.value && isAddressValid.value;
});

// Reset form when modal opens
watch(() => props.show, (visible) => {
  if (visible) {
    amount.value = null;
    walletAddress.value = '';
    withdrawLoading.value = false;
    addressTouched.value = false;
  }
});

function closeModal(): void {
  emit('update:show', false);
}

function setMaxAmount(): void {
  amount.value = walletStore.availableBalance;
}

function goToKYC(): void {
  closeModal();
  router.push('/user/kyc/verify');
}

async function handleWithdraw(): Promise<void> {
  if (!isFormValid.value || !amount.value) return;

  withdrawLoading.value = true;

  try {
    const result = await walletStore.requestWithdraw({
      amount_cents: Math.round(amount.value * 100),
      destination_type: 'crypto',
      crypto_details: {
        address: walletAddress.value.trim(),
        network: 'TRC20',
      },
    });

    if (result) {
      toast.success(t('wallet.withdrawalRequested'));
      closeModal();
    } else if (walletStore.error) {
      toast.error(walletStore.error);
    }
  } finally {
    withdrawLoading.value = false;
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
          <h3 class="modal-title">{{ t('wallet.withdrawModal.title') }}</h3>
          <button class="modal-close" @click="closeModal">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        <!-- KYC Required State -->
        <div v-if="needsKYC" class="modal-body kyc-body">
          <div class="kyc-icon-wrapper">
            <div class="kyc-icon-circle">
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
                <line x1="12" y1="9" x2="12" y2="13" />
                <line x1="12" y1="17" x2="12.01" y2="17" />
              </svg>
            </div>
          </div>
          <h4 class="kyc-title">{{ t('wallet.withdrawModal.kycRequired') }}</h4>
          <p class="kyc-message">{{ t('wallet.withdrawModal.kycMessage') }}</p>
          <button class="btn btn-primary btn-lg btn-full" @click="goToKYC">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
            </svg>
            {{ t('wallet.withdrawModal.goToKYC') }}
          </button>
        </div>

        <!-- Withdraw Form -->
        <div v-else class="modal-body">
          <!-- Amount Input -->
          <div class="form-group">
            <label class="form-label">{{ t('wallet.amount') }}</label>
            <div :class="['amount-input-wrapper', { 'input-error': amountError }]">
              <span class="currency-symbol">$</span>
              <input
                v-model.number="amount"
                type="number"
                class="amount-input"
                :placeholder="t('wallet.enterAmount')"
                :min="MIN_WITHDRAW"
                :max="walletStore.availableBalance"
                step="0.01"
              />
              <button class="max-btn" @click="setMaxAmount">
                {{ t('wallet.max') }}
              </button>
            </div>
            <span v-if="amountError" class="form-error">{{ amountError }}</span>
            <span class="form-hint">
              {{ t('wallet.minimumWithdraw', { amount: `$${MIN_WITHDRAW}` }) }} ·
              {{ t('wallet.availableToWithdraw') }}: {{ walletStore.formattedBalance }}
            </span>
          </div>

          <!-- Wallet Address -->
          <div class="form-group">
            <label class="form-label">
              {{ t('wallet.withdrawModal.walletAddressLabel') }}
              <span class="network-badge">USDT TRC20</span>
            </label>
            <input
              v-model="walletAddress"
              type="text"
              :class="['input', { 'input-error': addressError }]"
              :placeholder="t('wallet.withdrawModal.addressPlaceholder')"
              dir="ltr"
              spellcheck="false"
              autocomplete="off"
              @blur="addressTouched = true"
            />
            <span v-if="addressError" class="form-error">{{ addressError }}</span>
          </div>

          <!-- Submit -->
          <button
            class="btn btn-primary btn-lg btn-full"
            :disabled="!isFormValid || withdrawLoading"
            @click="handleWithdraw"
          >
            <span v-if="withdrawLoading" class="btn-spinner"></span>
            <span v-else>{{ t('wallet.withdrawModal.submit') }}</span>
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
  justify-content: space-between;
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

/* KYC Required */
.kyc-body {
  align-items: center;
  text-align: center;
  padding: var(--spacing-xl);
}

.kyc-icon-wrapper {
  margin-bottom: var(--spacing-sm);
}

.kyc-icon-circle {
  width: 80px;
  height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #FEF3C7 0%, #FDE68A 100%);
  border-radius: 50%;
  color: #D97706;
}

[data-theme="dark"] .kyc-icon-circle {
  background: linear-gradient(135deg, rgba(217, 119, 6, 0.2) 0%, rgba(217, 119, 6, 0.1) 100%);
}

.kyc-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.kyc-message {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
  line-height: 1.6;
}

/* Form */
.form-group {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.form-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.network-badge {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: #10B981;
  background: rgba(16, 185, 129, 0.12);
  padding: 2px 6px;
  border-radius: 4px;
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

.amount-input-wrapper.input-error {
  border-color: var(--color-error, #EF4444);
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

.max-btn {
  padding: var(--spacing-xs) var(--spacing-md);
  border: none;
  background: transparent;
  color: var(--color-primary);
  font-weight: 600;
  font-size: var(--font-size-xs);
  cursor: pointer;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.max-btn:hover {
  opacity: 0.8;
}

.input {
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  background: var(--color-bg-primary);
  transition: border-color var(--transition-fast);
  outline: none;
  box-sizing: border-box;
}

.input:focus {
  border-color: var(--color-primary);
}

.input.input-error {
  border-color: var(--color-error, #EF4444);
}

.form-error {
  font-size: var(--font-size-xs);
  color: var(--color-error, #EF4444);
}

.form-hint {
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
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
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

@keyframes spin {
  to { transform: rotate(360deg); }
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
