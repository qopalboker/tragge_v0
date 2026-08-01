<script setup lang="ts">
import { ref, computed, nextTick, onUnmounted } from 'vue';
import { t } from '@/i18n';
import { useI18nStore } from '@/stores/i18n';

const i18nStore = useI18nStore();
const isRTL = computed(() => i18nStore.locale === 'fa');

const props = defineProps<{
  method: 'sms' | 'email';
  maskedDestination: string;
  loading: boolean;
  error: string | null;
  remainingAttempts: number;
  resendCooldown: number;
}>();

const emit = defineEmits<{
  verify: [code: string];
  resend: [];
  back: [];
}>();

const digits = ref<string[]>(['', '', '', '', '', '']);
const inputRefs = ref<(HTMLInputElement | null)[]>([]);

const canResend = computed(() => props.resendCooldown === 0 && !props.loading);

const codeSentMessage = computed(() => {
  if (props.method === 'sms') {
    return t('verification.codeSentSms', { phone: props.maskedDestination });
  }
  return t('verification.codeSentEmail', { email: props.maskedDestination });
});

// Format cooldown timer display
const timerDisplay = computed(() => {
  const m = Math.floor(props.resendCooldown / 60);
  const s = props.resendCooldown % 60;
  return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
});

function onInput(index: number) {
  const value = digits.value[index];
  if (value.length > 1) {
    digits.value[index] = value.slice(-1);
  }
  if (!/^\d$/.test(digits.value[index])) {
    digits.value[index] = '';
    return;
  }

  // Auto-focus next
  if (digits.value[index] && index < 5) {
    nextTick(() => inputRefs.value[index + 1]?.focus());
  }

  // Auto-submit when all 6 filled
  if (digits.value.every(d => /^\d$/.test(d))) {
    emit('verify', digits.value.join(''));
  }
}

function onKeydown(index: number, event: KeyboardEvent) {
  if (event.key === 'Backspace') {
    if (!digits.value[index] && index > 0) {
      digits.value[index - 1] = '';
      nextTick(() => inputRefs.value[index - 1]?.focus());
      event.preventDefault();
    } else {
      digits.value[index] = '';
    }
  } else if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
    const dir = event.key === 'ArrowLeft' ? -1 : 1;
    const nextIdx = index + dir;
    if (nextIdx >= 0 && nextIdx <= 5) {
      nextTick(() => inputRefs.value[nextIdx]?.focus());
    }
  }
}

function onPaste(event: ClipboardEvent) {
  event.preventDefault();
  const pasted = event.clipboardData?.getData('text')?.trim() || '';
  const digitsOnly = pasted.replace(/\D/g, '').slice(0, 6);

  if (digitsOnly.length === 6) {
    for (let i = 0; i < 6; i++) {
      digits.value[i] = digitsOnly[i];
    }
    nextTick(() => {
      inputRefs.value[5]?.focus();
      emit('verify', digitsOnly);
    });
  }
}

function handleResend() {
  digits.value = ['', '', '', '', '', ''];
  nextTick(() => inputRefs.value[0]?.focus());
  emit('resend');
}

function clearAndFocus() {
  digits.value = ['', '', '', '', '', ''];
  nextTick(() => inputRefs.value[0]?.focus());
}

// Focus first input on mount
nextTick(() => inputRefs.value[0]?.focus());

// Expose clearAndFocus for parent
defineExpose({ clearAndFocus });

onUnmounted(() => {
  // Cleanup handled by parent composable
});
</script>

<template>
  <div class="modal-overlay" @click.self="emit('back')">
    <div :class="['modal-content', { rtl: isRTL }]" @click.stop>
      <div class="modal-icon" :class="method === 'sms' ? 'sms' : 'email'">
        <svg v-if="method === 'email'" width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <rect x="2" y="4" width="20" height="16" rx="2" />
          <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
        </svg>
        <svg v-else width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
        </svg>
      </div>

      <h2>{{ t('verification.enterCodeTitle') }}</h2>
      <p class="subtitle">{{ codeSentMessage }}</p>

      <!-- OTP Inputs -->
      <div class="otp-group" dir="ltr">
        <input
          v-for="(_, i) in 6"
          :key="i"
          :ref="(el) => { inputRefs[i] = el as HTMLInputElement }"
          v-model="digits[i]"
          type="text"
          inputmode="numeric"
          autocomplete="one-time-code"
          maxlength="1"
          :class="['otp-input', { 'otp-error': error, 'otp-filled': digits[i] }]"
          :disabled="loading"
          @input="onInput(i)"
          @keydown="onKeydown(i, $event)"
          @paste="onPaste"
          @focus="($event.target as HTMLInputElement).select()"
        />
      </div>

      <!-- Loading -->
      <div v-if="loading" class="status-row">
        <div class="spinner"></div>
        <span>{{ t('verification.verifying') }}</span>
      </div>

      <!-- Error -->
      <p v-if="error" class="error-text">{{ error }}</p>

      <!-- Remaining attempts -->
      <p v-if="remainingAttempts < 5 && remainingAttempts > 0 && !error" class="attempts-text">
        {{ t('verification.remainingAttempts', { count: remainingAttempts }) }}
      </p>

      <!-- Resend -->
      <div class="resend-row">
        <button
          v-if="canResend"
          class="resend-btn"
          @click="handleResend"
        >
          {{ t('verification.resend') }}
        </button>
        <span v-else-if="resendCooldown > 0" class="timer-text">
          {{ t('verification.resendIn', { seconds: timerDisplay }) }}
        </span>
      </div>

      <!-- Back button -->
      <button class="back-btn" @click="emit('back')" :disabled="loading">
        {{ t('verification.back') }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: var(--spacing-lg);
  animation: fadeIn 0.2s ease;
}

.modal-content {
  background: var(--color-bg-primary);
  border-radius: var(--radius-xl);
  padding: var(--spacing-2xl);
  max-width: 420px;
  width: 100%;
  text-align: center;
  box-shadow: var(--shadow-lg);
  animation: slideUp 0.3s ease;
}

.modal-content.rtl {
  direction: rtl;
}

.modal-icon {
  width: 72px;
  height: 72px;
  margin: 0 auto var(--spacing-lg);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.modal-icon.email {
  background: #dbeafe;
  color: #2563eb;
}

.modal-icon.sms {
  background: #d1fae5;
  color: #059669;
}

h2 {
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-sm) 0;
}

.subtitle {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  margin: 0 0 var(--spacing-xl) 0;
}

/* OTP Inputs */
.otp-group {
  display: flex;
  gap: 10px;
  justify-content: center;
  margin-bottom: var(--spacing-lg);
}

.otp-input {
  width: 52px;
  height: 56px;
  border: 2px solid var(--color-border);
  border-radius: 12px;
  font-size: 24px;
  font-weight: 700;
  text-align: center;
  color: var(--color-text-primary);
  background: var(--color-bg-primary);
  outline: none;
  transition: border-color 0.2s, box-shadow 0.2s;
  caret-color: var(--color-primary);
}

.otp-input:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.15);
}

.otp-input.otp-filled {
  background: var(--color-bg-secondary);
  border-color: var(--color-text-secondary);
}

.otp-input.otp-error {
  border-color: #ef4444;
}

.otp-input:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* Status row */
.status-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  margin-bottom: var(--spacing-md);
}

.spinner {
  width: 18px;
  height: 18px;
  border: 2px solid var(--color-bg-tertiary);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

/* Error */
.error-text {
  color: #ef4444;
  font-size: var(--font-size-sm);
  margin: 0 0 var(--spacing-md) 0;
}

.attempts-text {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  margin: 0 0 var(--spacing-md) 0;
}

/* Resend */
.resend-row {
  margin-bottom: var(--spacing-lg);
}

.resend-btn {
  background: none;
  border: none;
  color: var(--color-primary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-md);
  transition: background-color 0.2s;
}

.resend-btn:hover {
  background: var(--color-bg-tertiary);
}

.timer-text {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

/* Back button */
.back-btn {
  display: inline-block;
  background: none;
  border: none;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  cursor: pointer;
  transition: color 0.2s;
}

.back-btn:hover {
  color: var(--color-primary);
}

.back-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes slideUp {
  from { opacity: 0; transform: translateY(20px); }
  to { opacity: 1; transform: translateY(0); }
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 480px) {
  .otp-group { gap: 6px; }
  .otp-input {
    width: 44px;
    height: 48px;
    font-size: 20px;
    border-radius: 10px;
  }
}
</style>
