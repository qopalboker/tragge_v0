<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useAuthStore } from '@/stores/auth';
import { useI18nStore } from '@/stores/i18n';
import { validateState, clearStoredState } from '@/services/oauth';
import { validateRedirectPath, getTradeRedirect } from '@/utils/redirectValidation';

const router = useRouter();
const route = useRoute();
const authStore = useAuthStore();
const i18nStore = useI18nStore();

const isRTL = computed(() => i18nStore.locale === 'fa');
const lang = computed(() => i18nStore.locale);

const loading = ref(true);
const error = ref<string | null>(null);

const translations = computed(() => {
  if (lang.value === 'fa') {
    return {
      processing: 'در حال پردازش...',
      signingIn: 'در حال ورود با گوگل...',
      error: 'خطا در ورود',
      tryAgain: 'تلاش مجدد',
      backToLogin: 'بازگشت به صفحه ورود',
      invalidState: 'وضعیت نامعتبر یا منقضی شده. لطفا دوباره تلاش کنید.',
      missingParams: 'پارامترهای مورد نیاز یافت نشد.',
      missingCode: 'کد احراز هویت یافت نشد.',
      stateExpired: 'جلسه منقضی شده است. لطفا دوباره وارد شوید.',
      networkError: 'خطای شبکه. لطفا اتصال اینترنت خود را بررسی کنید.',
    };
  }
  return {
    processing: 'Processing...',
    signingIn: 'Signing in with Google...',
    error: 'Sign in failed',
    tryAgain: 'Try Again',
    backToLogin: 'Back to Login',
    invalidState: 'Invalid or expired state. Please try again.',
    missingParams: 'Missing required parameters.',
    missingCode: 'Authorization code not found.',
    stateExpired: 'Session expired. Please sign in again.',
    networkError: 'Network error. Please check your connection.',
  };
});

async function handleOAuthCallback() {
  loading.value = true;
  error.value = null;

  try {
    // Extract code and state from URL query params
    const code = route.query.code as string | undefined;
    const state = route.query.state as string | undefined;

    // Handle edge case: missing code parameter
    if (!code) {
      error.value = translations.value.missingCode;
      loading.value = false;
      return;
    }

    // Handle edge case: missing state parameter
    if (!state) {
      error.value = translations.value.missingParams;
      loading.value = false;
      return;
    }

    // Validate state parameter against stored value (CSRF protection)
    // This checks both the state match and expiration (5-minute window)
    if (!validateState(state)) {
      error.value = translations.value.invalidState;
      clearStoredState();
      loading.value = false;
      return;
    }

    // Use authStore.loginWithGoogle which handles token exchange, storage,
    // user fetch, and referral code application for new OAuth users
    const result = await authStore.loginWithGoogle(code, state);

    if (result.success) {
      // If email not verified, redirect to verification page
      if (!authStore.user?.email_verified) {
        router.push('/user/verify-email');
        return;
      }

      // Handle redirect - check for stored redirect destination
      const rawRedirect = sessionStorage.getItem('oauth_redirect');
      sessionStorage.removeItem('oauth_redirect');
      const redirect = validateRedirectPath(rawRedirect);

      // Check for trade panel redirect (same-origin, shared localStorage)
      const tradeUrl = getTradeRedirect(redirect);
      if (tradeUrl) {
        window.location.href = tradeUrl;
        return;
      }

      router.push(redirect);
    } else {
      error.value = authStore.error || translations.value.error;
    }
  } catch (err: unknown) {
    // Handle truly unexpected errors (loginWithGoogle handles its own errors internally)
    error.value = translations.value.error;
    clearStoredState();
  } finally {
    loading.value = false;
  }
}

function goToLogin() {
  router.push('/user/login');
}

function tryAgain() {
  // Clear state and go back to login to start fresh
  sessionStorage.removeItem('oauth_redirect');
  router.push('/user/login');
}

onMounted(() => {
  handleOAuthCallback();
});
</script>

<template>
  <div
    class="callback-root"
    :style="{ direction: isRTL ? 'rtl' : 'ltr' }"
  >
    <!-- Background -->
    <div class="bg-gradient" />
    <div class="grid-overlay" />

    <!-- Card -->
    <div class="callback-card">
      <!-- Loading state -->
      <div v-if="loading" class="callback-content">
        <div class="spinner-wrapper">
          <div class="spinner" />
        </div>
        <p class="status-text">{{ translations.signingIn }}</p>
      </div>

      <!-- Error state -->
      <div v-else-if="error" class="callback-content error-state">
        <div class="error-icon">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10"/>
            <line x1="15" y1="9" x2="9" y2="15"/>
            <line x1="9" y1="9" x2="15" y2="15"/>
          </svg>
        </div>
        <h2 class="error-title">{{ translations.error }}</h2>
        <p class="error-message">{{ error }}</p>
        <div class="button-group">
          <button type="button" class="btn-primary" @click="tryAgain">
            {{ translations.tryAgain }}
          </button>
          <button type="button" class="btn-secondary" @click="goToLogin">
            {{ translations.backToLogin }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@import url('https://fonts.googleapis.com/css2?family=Syne:wght@400;500;600;700;800&family=DM+Sans:ital,wght@0,300;0,400;0,500;1,300;1,400&display=swap');

.callback-root {
  min-height: 100vh;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: radial-gradient(ellipse at 30% 20%, #0a1510 0%, #060b08 100%);
  font-family: 'DM Sans', sans-serif;
  position: relative;
  padding: 16px;
}

.bg-gradient {
  position: fixed;
  inset: 0;
  background: radial-gradient(ellipse at 50% 0%, rgba(200, 149, 100, 0.08) 0%, transparent 60%);
  pointer-events: none;
  z-index: 0;
}

.grid-overlay {
  position: fixed;
  inset: 0;
  background-image: linear-gradient(rgba(255, 255, 255, 0.015) 1px, transparent 1px),
                    linear-gradient(90deg, rgba(255, 255, 255, 0.015) 1px, transparent 1px);
  background-size: 70px 70px;
  pointer-events: none;
  z-index: 1;
}

.callback-card {
  width: 100%;
  max-width: 400px;
  position: relative;
  z-index: 10;
  background: rgba(12, 20, 16, 0.85);
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 22px;
  padding: 48px 38px;
  backdrop-filter: blur(40px);
}

.callback-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}

.spinner-wrapper {
  width: 64px;
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 24px;
}

.spinner {
  width: 48px;
  height: 48px;
  border: 3px solid rgba(200, 149, 100, 0.2);
  border-top-color: #c89564;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.status-text {
  font-size: 16px;
  color: rgba(255, 255, 255, 0.7);
  font-weight: 400;
}

.error-state {
  gap: 8px;
}

.error-icon {
  color: #ef4444;
  margin-bottom: 16px;
}

.error-title {
  font-family: 'Syne', sans-serif;
  font-size: 24px;
  font-weight: 600;
  color: #f5f5f5;
  margin: 0;
}

.error-message {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.55);
  margin: 8px 0 24px;
  line-height: 1.5;
}

.button-group {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
}

.btn-primary {
  width: 100%;
  padding: 13px;
  background: linear-gradient(135deg, #c89564, #a37545);
  border: none;
  border-radius: 10px;
  color: #0a0f0c;
  font-size: 15px;
  font-weight: 600;
  font-family: 'Syne', sans-serif;
  cursor: pointer;
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}

.btn-primary:hover {
  transform: translateY(-1px);
  box-shadow: 0 8px 24px rgba(200, 149, 100, 0.3);
}

.btn-secondary {
  width: 100%;
  padding: 13px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  color: rgba(255, 255, 255, 0.7);
  font-size: 15px;
  font-weight: 500;
  font-family: 'DM Sans', sans-serif;
  cursor: pointer;
  transition: all 0.3s;
}

.btn-secondary:hover {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 255, 255, 0.15);
}

@media (max-width: 480px) {
  .callback-card {
    padding: 32px 24px;
  }
}
</style>
