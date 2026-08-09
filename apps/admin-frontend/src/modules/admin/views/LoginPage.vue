<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { useI18nStore } from '@/stores/i18n';
import { useToast } from '@/composables/useToast';
import { checkBackendHealth } from '@/utils/errorHandler';
import { validateAdminRedirectPath } from '@/utils/redirectValidation';

const router = useRouter();
const route = useRoute();
const authStore = useAuthStore();
const i18nStore = useI18nStore();
const toast = useToast();

const email = ref('');
const password = ref('');
const mfaCode = ref('');
const useRecoveryCode = ref(false);
const showPassword = ref(false);

// Login success state for welcome feedback
const loginSuccess = ref(false);

// Backend health check
const backendAvailable = ref<boolean | null>(null);
const checkingBackend = ref(false);

const isValid = computed(() => {
  if (authStore.mfaStage === 'verify' || authStore.mfaStage === 'enroll_verify') {
    return mfaCode.value.trim().length > 0;
  }
  return email.value.length > 0 && password.value.length > 0;
});

const direction = computed(() => i18nStore.direction);

watch(direction, (dir) => {
  document.documentElement.dir = dir;
}, { immediate: true });

onMounted(async () => {
  // Check backend health
  checkingBackend.value = true;
  backendAvailable.value = await checkBackendHealth();
  checkingBackend.value = false;

  // Check for error query param
  if (route.query.error === 'admin_required') {
    authStore.error = t('auth.accessDenied');
  }
});

// Retry backend health check
async function retryBackendCheck(): Promise<void> {
  checkingBackend.value = true;
  backendAvailable.value = await checkBackendHealth();
  checkingBackend.value = false;
}

async function handleSubmit(): Promise<void> {
  if (!isValid.value || authStore.loading) return;

  let success: boolean;
  if (authStore.mfaStage === 'verify' || authStore.mfaStage === 'enroll_verify') {
    success = await authStore.verifyMFA(mfaCode.value.trim(), useRecoveryCode.value);
    mfaCode.value = '';
  } else if (authStore.mfaStage === 'enroll') {
    await authStore.startMFAEnrollment();
    password.value = '';
    return;
  } else if (authStore.mfaStage === 'recovery_codes') {
    authStore.acknowledgeRecoveryCodes();
    success = true;
  } else {
    success = await authStore.login(email.value, password.value);
    password.value = '';
    const stageAfterPassword = String(authStore.mfaStage);
    if (stageAfterPassword === 'enroll') {
      await authStore.startMFAEnrollment();
      return;
    }
    if (stageAfterPassword === 'verify') return;
  }

  if (success) {
    // Show welcome state and success toast
    loginSuccess.value = true;
    toast.success(t('auth.loginSuccess'));

    // Redirect after a short delay to show the welcome state
    setTimeout(() => {
      const redirect = validateAdminRedirectPath(route.query.redirect as string);
      router.push(redirect);
    }, 1000);
  } else {
    // Show error toast on login failure
    toast.error(authStore.error || t('auth.loginError'));
  }
}

function toggleLanguage(): void {
  i18nStore.toggleLocale();
}

function restartLogin(): void {
  authStore.cancelMFA();
  mfaCode.value = '';
  useRecoveryCode.value = false;
}
</script>

<template>
  <div :class="['login-page', { 'rtl': direction === 'rtl' }]">
    <div class="login-container">
      <div class="login-logo">
        <svg width="48" height="48" viewBox="0 0 32 32" fill="none">
          <rect width="32" height="32" rx="4" fill="var(--color-primary)" />
          <path d="M8 22L12 14L18 18L24 10" stroke="white" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        <span class="logo-text">پنل مدیریت ترگ</span>
      </div>

      <form class="login-form" @submit.prevent="handleSubmit">
        <h1 class="login-title">
          {{ authStore.mfaStage === 'password' ? t('auth.login') : t('auth.mfaTitle') }}
        </h1>

        <!-- Backend Unavailable Warning -->
        <div v-if="backendAvailable === false" class="backend-warning">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="12" y1="8" x2="12" y2="12" />
            <line x1="12" y1="16" x2="12.01" y2="16" />
          </svg>
          <div class="backend-warning-content">
            <span class="backend-warning-text">{{ t('errors.backendUnavailable') }}</span>
            <span class="backend-warning-hint">{{ t('errors.backendHint') }}</span>
            <button type="button" class="retry-button" :disabled="checkingBackend" @click="retryBackendCheck">
              <span v-if="checkingBackend" class="spinner-small"></span>
              <span v-else>{{ t('common.retry') }}</span>
            </button>
          </div>
        </div>

        <div v-if="authStore.error" class="error-message">
          {{ authStore.error }}
        </div>

        <template v-if="authStore.mfaStage === 'password'">
          <div class="form-group">
            <label class="form-label" for="email">{{ t('auth.email') }}</label>
            <input
              id="email"
              v-model="email"
              type="email"
              class="input"
              :placeholder="t('auth.email')"
              autocomplete="email"
              required
            />
          </div>

          <div class="form-group">
            <label class="form-label" for="password">{{ t('auth.password') }}</label>
            <div class="password-input">
              <input
                id="password"
                v-model="password"
                :type="showPassword ? 'text' : 'password'"
                class="input"
                :placeholder="t('auth.password')"
                autocomplete="current-password"
                required
              />
              <button
                type="button"
                class="password-toggle"
                @click="showPassword = !showPassword"
              >
                <svg v-if="showPassword" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24" />
                  <line x1="1" y1="1" x2="23" y2="23" />
                </svg>
                <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                  <circle cx="12" cy="12" r="3" />
                </svg>
              </button>
            </div>
          </div>
        </template>

        <template v-else-if="authStore.mfaStage === 'enroll_verify'">
          <p class="mfa-instructions">{{ t('auth.mfaEnrollInstructions') }}</p>
          <div class="provisioning-box" data-testid="mfa-provisioning">
            <code>{{ authStore.mfaSecret }}</code>
            <small>{{ authStore.mfaProvisioningUri }}</small>
          </div>
          <div class="form-group">
            <label class="form-label" for="mfa-code">{{ t('auth.mfaCode') }}</label>
            <input id="mfa-code" v-model="mfaCode" class="input" inputmode="numeric" autocomplete="one-time-code" maxlength="6" />
          </div>
        </template>

        <template v-else-if="authStore.mfaStage === 'verify'">
          <p class="mfa-instructions">{{ useRecoveryCode ? t('auth.mfaRecoveryPrompt') : t('auth.mfaPrompt') }}</p>
          <div class="form-group">
            <label class="form-label" for="mfa-code">{{ useRecoveryCode ? t('auth.mfaRecoveryCode') : t('auth.mfaCode') }}</label>
            <input id="mfa-code" v-model="mfaCode" class="input" :inputmode="useRecoveryCode ? 'text' : 'numeric'" :autocomplete="useRecoveryCode ? 'off' : 'one-time-code'" />
          </div>
          <button type="button" class="secondary-action" @click="useRecoveryCode = !useRecoveryCode">
            {{ useRecoveryCode ? t('auth.mfaUseAuthenticator') : t('auth.mfaUseRecovery') }}
          </button>
        </template>

        <template v-else-if="authStore.mfaStage === 'recovery_codes'">
          <p class="mfa-instructions">{{ t('auth.mfaRecoverySave') }}</p>
          <ul class="recovery-codes" data-testid="mfa-recovery-codes">
            <li v-for="code in authStore.recoveryCodes" :key="code"><code>{{ code }}</code></li>
          </ul>
        </template>

        <button
          type="submit"
          :class="['btn', 'submit-btn', loginSuccess ? 'btn-success' : 'btn-primary']"
          :disabled="!isValid || authStore.loading || loginSuccess || backendAvailable === false"
        >
          <span v-if="authStore.loading" class="spinner" />
          <svg v-else-if="loginSuccess" class="success-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="20,6 9,17 4,12" />
          </svg>
          {{ loginSuccess ? t('auth.welcome') : (authStore.loading ? t('auth.loggingIn') : t('auth.loginButton')) }}
        </button>

        <button v-if="authStore.mfaStage !== 'password' && authStore.mfaStage !== 'recovery_codes'" type="button" class="secondary-action" @click="restartLogin">
          {{ t('auth.mfaRestart') }}
        </button>
      </form>

      <button class="lang-toggle" @click="toggleLanguage">
        {{ i18nStore.locale === 'en' ? 'فارسی' : 'English' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, var(--color-bg-secondary) 0%, var(--color-bg-tertiary) 100%);
  padding: var(--spacing-lg);
}

.login-page.rtl {
  direction: rtl;
}

.login-container {
  width: 100%;
  max-width: 400px;
}

.login-logo {
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

.login-form {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-xl);
  padding: var(--spacing-2xl);
  box-shadow: var(--shadow-lg);
}

.login-title {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  text-align: center;
  margin-bottom: var(--spacing-xl);
  color: var(--color-text-primary);
}

.mfa-instructions { color: var(--color-text-secondary); margin-bottom: var(--spacing-md); text-align: center; }
.provisioning-box { overflow-wrap: anywhere; background: var(--color-bg-tertiary); border-radius: var(--radius-md); padding: var(--spacing-md); margin-bottom: var(--spacing-md); }
.provisioning-box code, .provisioning-box small { display: block; direction: ltr; text-align: left; }
.recovery-codes { columns: 2; direction: ltr; list-style: none; padding: 0; }
.recovery-codes li { margin: var(--spacing-xs); }
.secondary-action { display: block; margin: var(--spacing-md) auto 0; background: none; border: none; color: var(--color-primary); cursor: pointer; }

.error-message {
  background-color: #FEE2E2;
  color: #DC2626;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  margin-bottom: var(--spacing-md);
  text-align: center;
}

.backend-warning {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  background-color: #FEF3C7;
  border: 1px solid #F59E0B;
  border-radius: var(--radius-md);
  margin-bottom: var(--spacing-md);
  color: #92400E;
}

.backend-warning svg {
  flex-shrink: 0;
  margin-top: 2px;
}

.backend-warning-content {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  flex: 1;
}

.backend-warning-text {
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.backend-warning-hint {
  font-size: var(--font-size-xs);
  opacity: 0.9;
}

.retry-button {
  align-self: flex-start;
  padding: var(--spacing-xs) var(--spacing-sm);
  background-color: #F59E0B;
  border: none;
  border-radius: var(--radius-sm);
  color: white;
  font-size: var(--font-size-xs);
  font-weight: 500;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  transition: background-color var(--transition-fast);
}

.retry-button:hover:not(:disabled) {
  background-color: #D97706;
}

.retry-button:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.spinner-small {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
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

.submit-btn {
  width: 100%;
  padding: var(--spacing-md);
  font-size: var(--font-size-md);
  margin-top: var(--spacing-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  transition: all var(--transition-fast);
}

.btn-success {
  background-color: #059669;
  border-color: #059669;
  color: white;
}

.btn-success:hover {
  background-color: #047857;
  border-color: #047857;
}

.success-icon {
  animation: checkmark 0.3s ease-out;
}

@keyframes checkmark {
  0% {
    transform: scale(0);
    opacity: 0;
  }
  50% {
    transform: scale(1.2);
  }
  100% {
    transform: scale(1);
    opacity: 1;
  }
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid transparent;
  border-top-color: currentColor;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin-right: var(--spacing-sm);
}

[dir="rtl"] .spinner {
  margin-right: 0;
  margin-left: var(--spacing-sm);
}

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
