<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useI18nStore } from '@/stores/i18n';
import { api } from '@/api';
import { ARCAPTCHA_SITE_KEY } from '@/config/captcha';

const router = useRouter();
const i18nStore = useI18nStore();

// Step state: 1 = identifier, 2 = OTP code, 3 = new password
const step = ref(1);

// Step 1 state
const identifier = ref('');
const step1Loading = ref(false);
const step1Error = ref<string | null>(null);

// Step 2 state
const resetToken = ref('');
const channelHint = ref('');
const maskedDestination = ref('');
const otpDigits = ref<string[]>(['', '', '', '', '', '']);
const step2Loading = ref(false);
const step2Error = ref<string | null>(null);
const remainingAttempts = ref<number | null>(null);
const resendCooldown = ref(0);
let cooldownTimer: ReturnType<typeof setInterval> | null = null;

// Step 3 state
const passwordSetToken = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const showNewPassword = ref(false);
const showConfirmPassword = ref(false);
const step3Loading = ref(false);
const step3Error = ref<string | null>(null);
const success = ref(false);

// Captcha state
const captchaLoading = ref(false);
const captchaWidgetId = ref<string | null>(null);
let captchaResolve: ((token: string) => void) | null = null;
let captchaReject: ((err: Error) => void) | null = null;

const direction = computed(() => i18nStore.direction);

watch(direction, (dir) => {
  document.documentElement.dir = dir;
}, { immediate: true });

// Step 1 validation
const step1Valid = computed(() => identifier.value.trim().length > 0);

// Step 2: OTP code computed
const otpCode = computed(() => otpDigits.value.join(''));
const otpComplete = computed(() => otpCode.value.length === 6 && /^\d{6}$/.test(otpCode.value));

// Step 3 validation
const passwordMinLength = computed(() => newPassword.value.length >= 10);
const passwordsMatch = computed(() => confirmPassword.value.length === 0 || newPassword.value === confirmPassword.value);
const step3Valid = computed(() => {
  return newPassword.value.length >= 10 && newPassword.value === confirmPassword.value;
});

// Password strength
const passwordStrength = computed(() => {
  const p = newPassword.value;
  if (p.length === 0) return 0;
  let score = 0;
  if (p.length >= 10) score++;
  if (/[a-z]/.test(p)) score++;
  if (/[A-Z]/.test(p)) score++;
  if (/\d/.test(p)) score++;
  if (/[^a-zA-Z\d]/.test(p)) score++;
  return score;
});

const strengthLabel = computed(() => {
  const s = passwordStrength.value;
  if (s === 0) return '';
  if (s <= 2) return t('forgotPassword.strengthWeak');
  if (s <= 3) return t('forgotPassword.strengthMedium');
  return t('forgotPassword.strengthStrong');
});

const strengthColor = computed(() => {
  const s = passwordStrength.value;
  if (s <= 2) return '#DC2626';
  if (s <= 3) return '#F59E0B';
  return '#059669';
});

// Step 1: Request reset code
async function handleStep1Submit(): Promise<void> {
  if (!step1Valid.value || step1Loading.value || captchaLoading.value) return;

  // Execute captcha first
  captchaLoading.value = true;
  let captchaToken: string;
  try {
    captchaToken = await executeCaptcha();
    if (!captchaToken) {
      step1Error.value = t('forgotPassword.errors.captchaFailed');
      captchaLoading.value = false;
      return;
    }
  } catch {
    step1Error.value = t('forgotPassword.errors.captchaFailed');
    captchaLoading.value = false;
    return;
  } finally {
    captchaLoading.value = false;
  }

  step1Loading.value = true;
  step1Error.value = null;

  try {
    const res = await api.post<{
      message: string;
      reset_token: string;
      channel_hint?: string;
      masked_destination?: string;
      retry_after_seconds?: number;
    }>('/api/user/auth/forgot-password/request', {
      identifier: identifier.value.trim(),
      captcha_token: captchaToken,
    });

    resetToken.value = res.data.reset_token;
    channelHint.value = res.data.channel_hint || '';
    maskedDestination.value = res.data.masked_destination || '';

    // If server returned retry_after_seconds WITHOUT channel_hint,
    // it means we hit cooldown — stay on step 1 and show wait message
    if (res.data.retry_after_seconds && !res.data.channel_hint) {
      startCooldownTimer(res.data.retry_after_seconds);
      step1Error.value = t('forgotPassword.errors.cooldown', { seconds: res.data.retry_after_seconds });
      return;
    }

    // Start cooldown timer for resend
    const retryAfter = res.data.retry_after_seconds || 120;
    startCooldownTimer(retryAfter);

    step.value = 2;
  } catch (err: unknown) {
    // Even on error, always move to step 2 to prevent enumeration
    // Generate a fake token for UX consistency
    resetToken.value = 'pending';
    step.value = 2;
    startCooldownTimer(120);
  } finally {
    step1Loading.value = false;
  }
}

// Step 2: Verify OTP code
async function handleStep2Submit(): Promise<void> {
  if (!otpComplete.value || step2Loading.value) return;
  step2Loading.value = true;
  step2Error.value = null;

  try {
    const res = await api.post<{
      password_set_token: string;
    }>('/api/user/auth/forgot-password/verify', {
      reset_token: resetToken.value,
      code: otpCode.value,
    });

    passwordSetToken.value = res.data.password_set_token;
    step.value = 3;
  } catch (err: unknown) {
    const apiErr = err as { response?: { data?: { error?: string; remaining_attempts?: number } } };
    const errMsg = apiErr.response?.data?.error;
    if (apiErr.response?.data?.remaining_attempts !== undefined) {
      remainingAttempts.value = apiErr.response.data.remaining_attempts;
    }
    step2Error.value = errMsg || t('forgotPassword.errors.invalidCode');
    // Clear OTP digits on error
    otpDigits.value = ['', '', '', '', '', ''];
    nextTick(() => {
      const firstInput = document.querySelector('.otp-input') as HTMLInputElement;
      firstInput?.focus();
    });
  } finally {
    step2Loading.value = false;
  }
}

// Step 3: Set new password
async function handleStep3Submit(): Promise<void> {
  if (!step3Valid.value || step3Loading.value) return;
  step3Loading.value = true;
  step3Error.value = null;

  try {
    await api.post('/api/user/auth/forgot-password/reset', {
      password_set_token: passwordSetToken.value,
      new_password: newPassword.value,
      confirm_password: confirmPassword.value,
    });

    success.value = true;
    setTimeout(() => {
      router.push('/user/login');
    }, 3000);
  } catch (err: unknown) {
    const apiErr = err as { response?: { data?: { error?: string } } };
    step3Error.value = apiErr.response?.data?.error || t('forgotPassword.errors.resetFailed');
  } finally {
    step3Loading.value = false;
  }
}

// Resend code
async function handleResend(): Promise<void> {
  if (resendCooldown.value > 0 || step1Loading.value || captchaLoading.value) return;

  // Execute captcha first
  captchaLoading.value = true;
  let captchaToken: string;
  try {
    captchaToken = await executeCaptcha();
    if (!captchaToken) {
      step2Error.value = t('forgotPassword.errors.captchaFailed');
      captchaLoading.value = false;
      return;
    }
  } catch {
    step2Error.value = t('forgotPassword.errors.captchaFailed');
    captchaLoading.value = false;
    return;
  } finally {
    captchaLoading.value = false;
  }

  step1Loading.value = true;
  step2Error.value = null;

  try {
    const res = await api.post<{
      message: string;
      reset_token: string;
      channel_hint?: string;
      masked_destination?: string;
    }>('/api/user/auth/forgot-password/request', {
      identifier: identifier.value.trim(),
      captcha_token: captchaToken,
    });

    resetToken.value = res.data.reset_token;
    if (res.data.channel_hint) channelHint.value = res.data.channel_hint;
    if (res.data.masked_destination) maskedDestination.value = res.data.masked_destination;
    startCooldownTimer(120);
    otpDigits.value = ['', '', '', '', '', ''];
  } catch {
    // Silently handle
  } finally {
    step1Loading.value = false;
  }
}

// OTP input handling
function handleOtpInput(index: number, event: Event): void {
  const input = event.target as HTMLInputElement;
  const value = input.value;

  if (!/^\d*$/.test(value)) {
    input.value = otpDigits.value[index];
    return;
  }

  otpDigits.value[index] = value.slice(-1);

  // Auto-advance to next input
  if (value && index < 5) {
    const nextInput = document.querySelectorAll('.otp-input')[index + 1] as HTMLInputElement;
    nextInput?.focus();
  }

  // Auto-submit when complete
  if (otpComplete.value) {
    handleStep2Submit();
  }
}

function handleOtpKeydown(index: number, event: KeyboardEvent): void {
  if (event.key === 'Backspace' && !otpDigits.value[index] && index > 0) {
    const prevInput = document.querySelectorAll('.otp-input')[index - 1] as HTMLInputElement;
    prevInput?.focus();
  }
  if (event.key === 'ArrowLeft' && index > 0) {
    const prevInput = document.querySelectorAll('.otp-input')[index - 1] as HTMLInputElement;
    prevInput?.focus();
  }
  if (event.key === 'ArrowRight' && index < 5) {
    const nextInput = document.querySelectorAll('.otp-input')[index + 1] as HTMLInputElement;
    nextInput?.focus();
  }
}

function handleOtpPaste(event: ClipboardEvent): void {
  event.preventDefault();
  const pasted = event.clipboardData?.getData('text')?.trim() || '';
  if (/^\d{6}$/.test(pasted)) {
    otpDigits.value = pasted.split('');
    nextTick(() => {
      if (otpComplete.value) handleStep2Submit();
    });
  }
}

// Cooldown timer
function startCooldownTimer(seconds: number): void {
  resendCooldown.value = seconds;
  if (cooldownTimer) clearInterval(cooldownTimer);
  cooldownTimer = setInterval(() => {
    resendCooldown.value--;
    if (resendCooldown.value <= 0) {
      if (cooldownTimer) clearInterval(cooldownTimer);
      cooldownTimer = null;
    }
  }, 1000);
}

function formatCooldown(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${s.toString().padStart(2, '0')}`;
}

function toggleLanguage(): void {
  i18nStore.toggleLocale();
}

function goToLogin(): void {
  router.push('/user/login');
}

function goBack(): void {
  if (step.value > 1) {
    step.value = step.value - 1;
  } else {
    goToLogin();
  }
}

function executeCaptcha(): Promise<string> {
  return new Promise((resolve, reject) => {
    const win = window as any;
    if (!win.arcaptcha) {
      reject(new Error('ARCaptcha not loaded'));
      return;
    }

    const timeout = setTimeout(() => {
      captchaResolve = null;
      captchaReject = null;
      reject(new Error('Captcha timeout'));
    }, 30000);

    captchaResolve = (token: string) => {
      clearTimeout(timeout);
      resolve(token);
    };
    captchaReject = (err: Error) => {
      clearTimeout(timeout);
      reject(err);
    };

    if (captchaWidgetId.value === null) {
      const container = document.getElementById('arcaptcha-forgot-container');
      captchaWidgetId.value = win.arcaptcha.render(container, {
        'site-key': ARCAPTCHA_SITE_KEY,
        size: 'invisible',
        callback: (token: string) => {
          if (captchaResolve) captchaResolve(token);
        },
        error_callback: () => {
          if (captchaReject) captchaReject(new Error('Captcha challenge failed'));
        },
      });
    } else {
      win.arcaptcha.reset(captchaWidgetId.value);
    }

    win.arcaptcha.execute(captchaWidgetId.value);
  });
}

onMounted(() => {
  // Load ARCaptcha script
  if (!document.querySelector('script[src*="arcaptcha"]')) {
    const script = document.createElement('script');
    script.src = 'https://widget.arcaptcha.co/1/api.js';
    script.async = true;
    script.defer = true;
    document.head.appendChild(script);
  }
});

onUnmounted(() => {
  if (cooldownTimer) clearInterval(cooldownTimer);
  captchaResolve = null;
  captchaReject = null;
  captchaWidgetId.value = null;
});
</script>

<template>
  <div :class="['forgot-password-page', { 'rtl': direction === 'rtl' }]">
    <div class="forgot-password-container">
      <!-- Logo -->
      <div class="logo">
        <svg width="48" height="48" viewBox="0 0 32 32" fill="none">
          <rect width="32" height="32" rx="4" fill="var(--color-primary)" />
          <path d="M8 22L12 14L18 18L24 10" stroke="white" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        <span class="logo-text">Tragge</span>
      </div>

      <!-- Success State -->
      <div v-if="success" class="card success-card">
        <div class="success-icon">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
            <polyline points="22,4 12,14.01 9,11.01" />
          </svg>
        </div>
        <h1 class="card-title">{{ t('forgotPassword.passwordChanged') }}</h1>
        <p class="card-subtitle">{{ t('forgotPassword.redirectingToLogin') }}</p>
        <button class="btn btn-primary" @click="goToLogin">
          {{ t('auth.backToLogin') }}
        </button>
      </div>

      <!-- Step 1: Enter Identifier -->
      <form v-else-if="step === 1" class="card" @submit.prevent="handleStep1Submit">
        <h1 class="card-title">{{ t('forgotPassword.title') }}</h1>
        <p class="card-subtitle">{{ t('forgotPassword.subtitle') }}</p>

        <div v-if="step1Error" class="error-message">{{ step1Error }}</div>

        <div class="form-group">
          <label class="form-label" for="identifier">{{ t('forgotPassword.identifierLabel') }}</label>
          <input
            id="identifier"
            v-model="identifier"
            type="text"
            class="input"
            :placeholder="t('forgotPassword.identifierPlaceholder')"
            autocomplete="username"
            required
            :dir="direction === 'rtl' ? 'rtl' : 'ltr'"
          />
        </div>

        <button
          type="submit"
          class="btn btn-primary submit-btn"
          :disabled="!step1Valid || step1Loading || captchaLoading"
        >
          <span v-if="step1Loading" class="spinner"></span>
          {{ step1Loading ? t('common.loading') : t('forgotPassword.sendCode') }}
        </button>

        <p class="back-link">
          <router-link to="/user/login" class="link">{{ t('auth.backToLogin') }}</router-link>
        </p>
      </form>

      <!-- Step 2: Enter OTP Code -->
      <div v-else-if="step === 2" class="card">
        <button type="button" class="back-btn" @click="goBack">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline :points="direction === 'rtl' ? '9,18 15,12 9,6' : '15,18 9,12 15,6'" />
          </svg>
        </button>

        <h1 class="card-title">{{ t('forgotPassword.codeSent') }}</h1>
        <p v-if="maskedDestination" class="card-subtitle">
          {{ t('forgotPassword.codeSentTo', { destination: maskedDestination }) }}
        </p>
        <p v-else class="card-subtitle">
          {{ t('forgotPassword.codeSentGeneric') }}
        </p>

        <div v-if="step2Error" class="error-message">
          {{ step2Error }}
          <span v-if="remainingAttempts !== null" class="remaining">
            ({{ t('forgotPassword.remainingAttempts', { count: remainingAttempts }) }})
          </span>
        </div>

        <!-- OTP Input -->
        <div class="otp-container" :dir="'ltr'">
          <input
            v-for="(_, index) in 6"
            :key="index"
            type="text"
            inputmode="numeric"
            maxlength="1"
            class="otp-input"
            :value="otpDigits[index]"
            @input="handleOtpInput(index, $event)"
            @keydown="handleOtpKeydown(index, $event)"
            @paste="handleOtpPaste"
            :disabled="step2Loading"
          />
        </div>

        <button
          type="button"
          class="btn btn-primary submit-btn"
          :disabled="!otpComplete || step2Loading"
          @click="handleStep2Submit"
        >
          <span v-if="step2Loading" class="spinner"></span>
          {{ step2Loading ? t('common.loading') : t('forgotPassword.verifyCode') }}
        </button>

        <!-- Resend -->
        <div class="resend-section">
          <button
            v-if="resendCooldown <= 0"
            type="button"
            class="resend-btn"
            @click="handleResend"
            :disabled="step1Loading"
          >
            {{ t('forgotPassword.resendCode') }}
          </button>
          <span v-else class="resend-timer">
            {{ t('forgotPassword.resendIn', { time: formatCooldown(resendCooldown) }) }}
          </span>
        </div>
      </div>

      <!-- Step 3: Set New Password -->
      <form v-else-if="step === 3" class="card" @submit.prevent="handleStep3Submit">
        <button type="button" class="back-btn" @click="goBack">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline :points="direction === 'rtl' ? '9,18 15,12 9,6' : '15,18 9,12 15,6'" />
          </svg>
        </button>

        <h1 class="card-title">{{ t('forgotPassword.newPasswordTitle') }}</h1>
        <p class="card-subtitle">{{ t('forgotPassword.newPasswordSubtitle') }}</p>

        <div v-if="step3Error" class="error-message">{{ step3Error }}</div>

        <!-- New Password -->
        <div class="form-group">
          <label class="form-label" for="new-password">{{ t('forgotPassword.newPassword') }}</label>
          <div class="password-input">
            <input
              id="new-password"
              v-model="newPassword"
              :type="showNewPassword ? 'text' : 'password'"
              class="input"
              :placeholder="t('forgotPassword.newPasswordPlaceholder')"
              autocomplete="new-password"
              required
            />
            <button type="button" class="password-toggle" @click="showNewPassword = !showNewPassword">
              <svg v-if="showNewPassword" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24" />
                <line x1="1" y1="1" x2="23" y2="23" />
              </svg>
              <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                <circle cx="12" cy="12" r="3" />
              </svg>
            </button>
          </div>
          <!-- Strength bar -->
          <div v-if="newPassword.length > 0" class="strength-bar-container">
            <div class="strength-bar">
              <div class="strength-fill" :style="{ width: (passwordStrength * 20) + '%', backgroundColor: strengthColor }"></div>
            </div>
            <span class="strength-label" :style="{ color: strengthColor }">{{ strengthLabel }}</span>
          </div>
          <p v-if="newPassword.length > 0 && !passwordMinLength" class="field-hint error">
            {{ t('forgotPassword.errors.passwordTooShort') }}
          </p>
        </div>

        <!-- Confirm Password -->
        <div class="form-group">
          <label class="form-label" for="confirm-password">{{ t('forgotPassword.confirmPassword') }}</label>
          <div class="password-input">
            <input
              id="confirm-password"
              v-model="confirmPassword"
              :type="showConfirmPassword ? 'text' : 'password'"
              class="input"
              :placeholder="t('forgotPassword.confirmPassword')"
              autocomplete="new-password"
              required
            />
            <button type="button" class="password-toggle" @click="showConfirmPassword = !showConfirmPassword">
              <svg v-if="showConfirmPassword" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24" />
                <line x1="1" y1="1" x2="23" y2="23" />
              </svg>
              <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                <circle cx="12" cy="12" r="3" />
              </svg>
            </button>
          </div>
          <p v-if="!passwordsMatch" class="field-hint error">
            {{ t('forgotPassword.errors.passwordMismatch') }}
          </p>
        </div>

        <button
          type="submit"
          class="btn btn-primary submit-btn"
          :disabled="!step3Valid || step3Loading"
        >
          <span v-if="step3Loading" class="spinner"></span>
          {{ step3Loading ? t('common.loading') : t('forgotPassword.setPassword') }}
        </button>
      </form>

      <!-- Language Toggle -->
      <button class="lang-toggle" @click="toggleLanguage">
        {{ i18nStore.locale === 'en' ? 'فارسی' : 'English' }}
      </button>
    </div>

    <!-- Hidden ARCaptcha container -->
    <div id="arcaptcha-forgot-container" style="display:none;"></div>
  </div>
</template>

<style scoped>
.forgot-password-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, var(--color-bg-secondary) 0%, var(--color-bg-tertiary) 100%);
  padding: var(--spacing-lg);
}

.forgot-password-page.rtl {
  direction: rtl;
}

.forgot-password-container {
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
  position: relative;
}

.card-title {
  font-size: var(--font-size-xl);
  font-weight: 600;
  text-align: center;
  margin-bottom: var(--spacing-xs);
  color: var(--color-text-primary);
}

.card-subtitle {
  text-align: center;
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-xl);
  font-size: var(--font-size-sm);
}

.back-btn {
  position: absolute;
  top: var(--spacing-lg);
  left: var(--spacing-lg);
  background: none;
  border: none;
  color: var(--color-text-secondary);
  cursor: pointer;
  padding: var(--spacing-xs);
  border-radius: var(--radius-sm);
}

[dir="rtl"] .back-btn {
  left: auto;
  right: var(--spacing-lg);
}

.back-btn:hover {
  color: var(--color-text-primary);
}

.error-message {
  background-color: #FEE2E2;
  color: #DC2626;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  margin-bottom: var(--spacing-md);
  text-align: center;
}

.dark .error-message {
  background-color: rgba(220, 38, 38, 0.2);
  color: #FCA5A5;
}

.remaining {
  display: block;
  font-size: var(--font-size-xs);
  margin-top: var(--spacing-xs);
}

.form-group {
  margin-bottom: var(--spacing-md);
}

.form-label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.submit-btn {
  width: 100%;
  padding: var(--spacing-md);
  font-size: var(--font-size-md);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-md);
}

/* OTP Input */
.otp-container {
  display: flex;
  gap: var(--spacing-sm);
  justify-content: center;
  margin: var(--spacing-lg) 0;
}

.otp-input {
  width: 48px;
  height: 56px;
  text-align: center;
  font-size: var(--font-size-xl);
  font-weight: 600;
  border: 2px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  outline: none;
  transition: border-color var(--transition-fast);
}

.otp-input:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.otp-input:disabled {
  background-color: var(--color-bg-secondary);
  cursor: not-allowed;
}

/* Password Input */
.password-input {
  position: relative;
}

.password-input .input {
  padding-right: 44px;
}

[dir="rtl"] .password-input .input {
  padding-right: var(--spacing-md);
  padding-left: 44px;
}

.password-toggle {
  position: absolute;
  right: var(--spacing-sm);
  top: 50%;
  transform: translateY(-50%);
  padding: var(--spacing-xs);
  background: none;
  border: none;
  color: var(--color-text-muted);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}

[dir="rtl"] .password-toggle {
  right: auto;
  left: var(--spacing-sm);
}

.password-toggle:hover {
  color: var(--color-text-secondary);
}

/* Strength bar */
.strength-bar-container {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-xs);
}

.strength-bar {
  flex: 1;
  height: 4px;
  background-color: var(--color-bg-tertiary);
  border-radius: 2px;
  overflow: hidden;
}

.strength-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.3s ease, background-color 0.3s ease;
}

.strength-label {
  font-size: var(--font-size-xs);
  font-weight: 500;
  white-space: nowrap;
}

.field-hint {
  font-size: var(--font-size-xs);
  margin-top: var(--spacing-xs);
}

.field-hint.error {
  color: #DC2626;
}

/* Resend section */
.resend-section {
  text-align: center;
  margin-top: var(--spacing-lg);
}

.resend-btn {
  background: none;
  border: none;
  color: var(--color-primary);
  font-size: var(--font-size-sm);
  cursor: pointer;
  padding: var(--spacing-xs) var(--spacing-sm);
}

.resend-btn:hover {
  text-decoration: underline;
}

.resend-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.resend-timer {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

/* Success */
.success-card {
  text-align: center;
}

.success-icon {
  width: 80px;
  height: 80px;
  margin: 0 auto var(--spacing-lg);
  border-radius: 50%;
  background-color: #D1FAE5;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #059669;
}

/* Back link */
.back-link {
  text-align: center;
  margin-top: var(--spacing-lg);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.link {
  color: var(--color-primary);
  text-decoration: none;
}

.link:hover {
  text-decoration: underline;
}

/* Spinner */
.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid transparent;
  border-top-color: currentColor;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
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
