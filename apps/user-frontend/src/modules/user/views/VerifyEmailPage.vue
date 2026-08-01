<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { t } from '@/i18n';
import { useI18nStore } from '@/stores/i18n';
import { useAuthStore } from '@/stores/auth';
import { api } from '@/api';

const router = useRouter();
const route = useRoute();
const i18nStore = useI18nStore();
const authStore = useAuthStore();

const direction = computed(() => i18nStore.direction);

// OTP state
const digits = ref<string[]>(['', '', '', '', '', '']);
const inputRefs = ref<(HTMLInputElement | null)[]>([]);
const loading = ref(false);
const success = ref(false);
const errorMsg = ref<string | null>(null);
const initialLoading = ref(true);

// Resend timer
const RESEND_COOLDOWN = 120;
const resendTimer = ref(RESEND_COOLDOWN);
const resendLoading = ref(false);
let timerInterval: ReturnType<typeof setInterval> | null = null;

// Mask email: u***@email.com
const maskedEmail = computed(() => {
  const email = authStore.user?.email || '';
  if (!email.includes('@')) return email;
  const [local, domain] = email.split('@');
  if (local.length <= 2) return `${local[0]}***@${domain}`;
  return `${local[0]}${local[1]}***@${domain}`;
});

// Start countdown timer from current value (don't reset to RESEND_COOLDOWN)
function startTimerFromCurrent() {
  if (timerInterval) clearInterval(timerInterval);
  timerInterval = setInterval(() => {
    if (resendTimer.value > 0) {
      resendTimer.value--;
    } else {
      if (timerInterval) clearInterval(timerInterval);
    }
  }, 1000);
}

// Start countdown timer (resets to RESEND_COOLDOWN)
function startTimer() {
  resendTimer.value = RESEND_COOLDOWN;
  startTimerFromCurrent();
}

// Format timer as MM:SS
const timerDisplay = computed(() => {
  const m = Math.floor(resendTimer.value / 60);
  const s = resendTimer.value % 60;
  return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
});

const canResend = computed(() => resendTimer.value === 0 && !resendLoading.value);

// Handle input on each digit box
function onInput(index: number) {
  const value = digits.value[index];

  // Only allow single digit
  if (value.length > 1) {
    digits.value[index] = value.slice(-1);
  }

  // Only allow digits
  if (!/^\d$/.test(digits.value[index])) {
    digits.value[index] = '';
    return;
  }

  // Clear error on new input
  errorMsg.value = null;

  // Auto-focus next
  if (digits.value[index] && index < 5) {
    nextTick(() => inputRefs.value[index + 1]?.focus());
  }

  // Check if all 6 digits filled → auto-submit
  if (digits.value.every(d => /^\d$/.test(d))) {
    submitCode();
  }
}

// Handle keydown for backspace and arrow keys
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

// Handle paste
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
      submitCode();
    });
  }
}

// Ensure auth store has email_verified = true before redirecting to dashboard.
// If fetchUser fails, manually patch the store to prevent infinite redirect loop.
// Next page load will fetch fresh data anyway.
async function ensureStoreUpdated() {
  try {
    await authStore.fetchUser();
  } catch {
    if (authStore.user) {
      authStore.user.email_verified = true;
    }
  }
}

// Submit verification code
async function submitCode() {
  const code = digits.value.join('');
  if (code.length !== 6) return;

  loading.value = true;
  errorMsg.value = null;

  try {
    await api.post('/api/user/auth/verify-email', { code });
    success.value = true;
    // Refresh user data to update email_verified
    await ensureStoreUpdated();
    // Redirect after 2 seconds
    setTimeout(() => router.push('/user/dashboard'), 2000);
  } catch (err: unknown) {
    const apiErr = err as { response?: { data?: { error?: string; message?: string } } };
    const errCode = apiErr.response?.data?.error;

    if (errCode === 'wrong_code') {
      errorMsg.value = apiErr.response?.data?.message || t('emailVerification.wrongCode');
      digits.value = ['', '', '', '', '', ''];
      nextTick(() => inputRefs.value[0]?.focus());
    } else if (errCode === 'rate_limited') {
      errorMsg.value = t('emailVerification.tooManyAttempts');
      // Don't clear digits — let user wait and retry
    } else if (errCode === 'already_verified') {
      await ensureStoreUpdated();
      router.push('/user/dashboard');
    } else if (errCode === 'code_exhausted' || errCode === 'no_valid_code') {
      errorMsg.value = t('emailVerification.codeExpired');
      digits.value = ['', '', '', '', '', ''];
    } else {
      errorMsg.value = t('emailVerification.unknownError');
    }
  } finally {
    loading.value = false;
  }
}

// Resend verification code
async function resendCode() {
  if (!canResend.value) return;

  resendLoading.value = true;
  errorMsg.value = null;

  try {
    await api.post('/api/user/auth/resend-verification');
    startTimer();
    digits.value = ['', '', '', '', '', ''];
    nextTick(() => inputRefs.value[0]?.focus());
  } catch (err: unknown) {
    const apiErr = err as { response?: { data?: { error?: string } } };
    if (apiErr.response?.data?.error === 'rate_limited') {
      errorMsg.value = t('emailVerification.rateLimited');
    } else if (apiErr.response?.data?.error === 'already_verified') {
      await ensureStoreUpdated();
      router.push('/user/dashboard');
    }
  } finally {
    resendLoading.value = false;
  }
}

onMounted(async () => {
  // Check if coming from registration (code was just sent via welcome email)
  const fromRegister = route.query.source === 'register';

  if (fromRegister) {
    // Code was just sent during registration — don't send another one
    // Just start the resend timer
    startTimer();
    initialLoading.value = false;
    nextTick(() => inputRefs.value[0]?.focus());
    // Clean up the query param from URL without navigation
    router.replace({ path: '/user/verify-email' });
    return;
  }

  // Not from registration — request a fresh code (login, router guard, direct visit)
  try {
    await api.post('/api/user/auth/resend-verification');
    startTimer();
  } catch (err: unknown) {
    const apiErr = err as { response?: { data?: { error?: string } } };
    if (apiErr.response?.data?.error === 'already_verified') {
      await ensureStoreUpdated();
      router.push('/user/dashboard');
      return;
    } else if (apiErr.response?.data?.error === 'rate_limited') {
      // Recently sent — code is still valid
      const serverRetry = (apiErr.response?.data as Record<string, unknown>)?.retry_after_seconds;
      if (typeof serverRetry === 'number' && serverRetry > 0 && serverRetry <= RESEND_COOLDOWN) {
        resendTimer.value = serverRetry;
      }
      startTimerFromCurrent();
    } else {
      errorMsg.value = t('emailVerification.unknownError');
    }
  } finally {
    initialLoading.value = false;
  }
  nextTick(() => inputRefs.value[0]?.focus());
});

onUnmounted(() => {
  if (timerInterval) clearInterval(timerInterval);
});

watch(direction, (dir) => {
  document.documentElement.dir = dir;
}, { immediate: true });
</script>

<template>
  <div :class="['verify-page', { 'rtl': direction === 'rtl' }]">
    <div class="verify-container">
      <!-- Logo -->
      <div class="logo">
        <svg width="48" height="48" viewBox="0 0 32 32" fill="none">
          <rect width="32" height="32" rx="4" fill="var(--color-primary)" />
          <path d="M8 22L12 14L18 18L24 10" stroke="white" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        <span class="logo-text">Tragge</span>
      </div>

      <!-- Initial Loading -->
      <div v-if="initialLoading" class="card">
        <div class="spinner-row" style="padding: 40px 0;">
          <div class="spinner"></div>
          <span>{{ t('emailVerification.sendingCode') }}</span>
        </div>
      </div>

      <!-- Success State -->
      <div v-else-if="success" class="card success-card">
        <div class="icon-circle success">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <polyline points="20,6 9,17 4,12" />
          </svg>
        </div>
        <h2>{{ t('emailVerification.success') }}</h2>
        <p class="subtitle">{{ t('emailVerification.redirecting') }}</p>
      </div>

      <!-- OTP Entry -->
      <div v-else class="card">
        <div class="icon-circle email">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <rect x="2" y="4" width="20" height="16" rx="2" />
            <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
          </svg>
        </div>

        <h2>{{ t('emailVerification.enterCode') }}</h2>
        <p class="subtitle">
          {{ t('emailVerification.codeSentTo') }}
          <span class="email-display">{{ maskedEmail }}</span>
        </p>

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
            :class="['otp-input', { 'otp-error': errorMsg, 'otp-filled': digits[i] }]"
            :disabled="loading"
            @input="onInput(i)"
            @keydown="onKeydown(i, $event)"
            @paste="onPaste"
            @focus="($event.target as HTMLInputElement).select()"
          />
        </div>

        <!-- Loading spinner -->
        <div v-if="loading" class="spinner-row">
          <div class="spinner"></div>
          <span>{{ t('emailVerification.verifying') }}</span>
        </div>

        <!-- Error message -->
        <p v-if="errorMsg" class="error-text">{{ errorMsg }}</p>

        <!-- Resend timer -->
        <div class="resend-row">
          <button
            v-if="canResend"
            class="resend-btn"
            :disabled="resendLoading"
            @click="resendCode"
          >
            <span v-if="resendLoading" class="spinner-sm"></span>
            {{ t('emailVerification.resendCode') }}
          </button>
          <span v-else class="timer-text">
            {{ t('emailVerification.resendIn') }}
            <span class="timer-count">{{ timerDisplay }}</span>
          </span>
        </div>

        <router-link to="/user/login" class="back-link">
          {{ t('auth.backToLogin') }}
        </router-link>
      </div>

      <!-- Language Toggle -->
      <button class="lang-toggle" @click="i18nStore.toggleLocale()">
        {{ i18nStore.locale === 'en' ? 'فارسی' : 'English' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.verify-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, var(--color-bg-secondary) 0%, var(--color-bg-tertiary) 100%);
  padding: var(--spacing-lg);
}

.verify-page.rtl {
  direction: rtl;
}

.verify-container {
  width: 100%;
  max-width: 420px;
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

.card {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-xl);
  padding: var(--spacing-2xl);
  box-shadow: var(--shadow-lg);
  text-align: center;
}

.icon-circle {
  width: 72px;
  height: 72px;
  margin: 0 auto var(--spacing-lg);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.icon-circle.success {
  background-color: #d1fae5;
  color: #059669;
}

.icon-circle.email {
  background-color: #dbeafe;
  color: #2563eb;
}

.card h2 {
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

.email-display {
  display: block;
  color: var(--color-text-primary);
  font-weight: 500;
  margin-top: var(--spacing-xs);
  direction: ltr;
  unicode-bidi: embed;
}

/* OTP Input Group */
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
  background-color: var(--color-bg-primary);
  outline: none;
  transition: border-color 0.2s, box-shadow 0.2s;
  caret-color: var(--color-primary);
}

.otp-input:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.15);
}

.otp-input.otp-filled {
  background-color: var(--color-bg-secondary);
  border-color: var(--color-text-secondary);
}

.otp-input.otp-error {
  border-color: #ef4444;
}

.otp-input:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* Spinner */
.spinner-row {
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

.spinner-sm {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid transparent;
  border-top-color: currentColor;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin-inline-end: var(--spacing-xs);
}

/* Error */
.error-text {
  color: #ef4444;
  font-size: var(--font-size-sm);
  margin: 0 0 var(--spacing-md) 0;
}

/* Resend */
.resend-row {
  margin-bottom: var(--spacing-lg);
}

.resend-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
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
  background-color: var(--color-bg-tertiary);
}

.resend-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.timer-text {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.timer-count {
  font-family: 'SF Mono', 'Fira Code', 'Courier New', monospace;
  color: var(--color-primary);
  font-weight: 600;
}

/* Back link */
.back-link {
  display: inline-block;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  text-decoration: none;
  transition: color 0.2s;
}

.back-link:hover {
  color: var(--color-primary);
}

/* Success card */
.success-card h2 {
  color: #059669;
}

.success-card .subtitle {
  color: var(--color-text-secondary);
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
  transition: all 0.2s;
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

/* Responsive */
@media (max-width: 480px) {
  .otp-group {
    gap: 6px;
  }
  .otp-input {
    width: 44px;
    height: 48px;
    font-size: 20px;
    border-radius: 10px;
  }
}
</style>
