<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { t } from '@/i18n';
import { useI18nStore } from '@/stores/i18n';
import { useReferralStore } from '@/stores/referral';

const router = useRouter();
const route = useRoute();
const i18nStore = useI18nStore();
const referralStore = useReferralStore();

const referralCode = ref('');
const referrerName = ref<string | null>(null);
const loading = ref(true);
const error = ref<string | null>(null);
const isValidCode = ref(false);

const direction = computed(() => i18nStore.direction);

const benefits = computed(() => [
  {
    icon: 'trophy',
    title: t('referral.landing.benefit1Title'),
    description: t('referral.landing.benefit1Desc'),
  },
  {
    icon: 'chart',
    title: t('referral.landing.benefit2Title'),
    description: t('referral.landing.benefit2Desc'),
  },
  {
    icon: 'gift',
    title: t('referral.landing.benefit3Title'),
    description: t('referral.landing.benefit3Desc'),
  },
]);

onMounted(async () => {
  // Get referral code from URL
  const refCode = route.query.ref as string;

  if (!refCode) {
    // No referral code, redirect to regular login
    router.push('/user/login');
    return;
  }

  referralCode.value = refCode;

  // Store the referral code
  referralStore.setReferralCode(refCode, true);

  // Validate and get referrer info
  try {
    const result = await referralStore.validateCode(refCode);

    if (result.valid) {
      isValidCode.value = true;
      referrerName.value = result.referrer_name || null;
    } else {
      isValidCode.value = false;
      error.value = result.error || t('referral.invalidCode');
    }
  } catch {
    error.value = t('referral.validationFailed');
  } finally {
    loading.value = false;
  }
});

function proceedToSignUp(): void {
  // Navigate to login page with referral code preserved
  router.push({
    path: '/user/login',
    query: { ref: referralCode.value },
  });
}

function proceedToLogin(): void {
  router.push('/user/login');
}

function toggleLanguage(): void {
  i18nStore.toggleLocale();
}
</script>

<template>
  <div :class="['referral-landing', { 'rtl': direction === 'rtl' }]">
    <!-- Loading State -->
    <div v-if="loading" class="loading-container">
      <div class="loading-spinner"></div>
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Content -->
    <div v-else class="landing-container">
      <!-- Header -->
      <header class="landing-header">
        <div class="logo">
          <svg width="40" height="40" viewBox="0 0 32 32" fill="none">
            <rect width="32" height="32" rx="4" fill="var(--color-primary)" />
            <path d="M8 22L12 14L18 18L24 10" stroke="white" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <span class="logo-text">Tragge</span>
        </div>
        <button class="lang-toggle" @click="toggleLanguage">
          {{ i18nStore.locale === 'en' ? 'فارسی' : 'English' }}
        </button>
      </header>

      <!-- Hero Section -->
      <section class="hero-section">
        <div class="hero-badge">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
            <circle cx="12" cy="7" r="4" />
          </svg>
          <span>{{ t('referral.landing.invitedBadge') }}</span>
        </div>

        <h1 class="hero-title">{{ t('referral.landing.heroTitle') }}</h1>

        <p v-if="isValidCode && referrerName" class="hero-subtitle">
          {{ t('referral.landing.invitedBy', { name: referrerName }) }}
        </p>
        <p v-else-if="isValidCode" class="hero-subtitle">
          {{ t('referral.landing.invitedByFriend') }}
        </p>
        <p v-else class="hero-subtitle error">
          {{ error || t('referral.invalidCode') }}
        </p>

        <div class="hero-illustration">
          <svg width="200" height="140" viewBox="0 0 200 140" fill="none">
            <!-- Trading chart illustration -->
            <rect x="10" y="10" width="180" height="120" rx="8" fill="var(--color-bg-secondary)" />
            <path d="M30 90 L50 70 L70 80 L90 50 L110 65 L130 40 L150 55 L170 30" stroke="var(--color-primary)" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" />
            <circle cx="90" cy="50" r="6" fill="var(--color-primary)" />
            <circle cx="130" cy="40" r="6" fill="var(--color-primary)" />
            <circle cx="170" cy="30" r="6" fill="var(--color-primary)" />
            <!-- Trophy icon -->
            <rect x="75" y="95" width="50" height="25" rx="4" fill="var(--color-primary)" opacity="0.2" />
            <path d="M88 102 L100 117 L112 102" stroke="var(--color-primary)" stroke-width="2" fill="none" />
          </svg>
        </div>
      </section>

      <!-- Benefits Section -->
      <section class="benefits-section">
        <h2 class="section-title">{{ t('referral.landing.whyJoin') }}</h2>

        <div class="benefits-grid">
          <div v-for="benefit in benefits" :key="benefit.title" class="benefit-card">
            <div class="benefit-icon">
              <svg v-if="benefit.icon === 'trophy'" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6" />
                <path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18" />
                <path d="M4 22h16" />
                <path d="M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22" />
                <path d="M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22" />
                <path d="M18 2H6v7a6 6 0 0 0 12 0V2Z" />
              </svg>
              <svg v-else-if="benefit.icon === 'chart'" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M3 3v18h18" />
                <path d="M18 17V9" />
                <path d="M13 17V5" />
                <path d="M8 17v-3" />
              </svg>
              <svg v-else width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M20 12v10H4V12" />
                <path d="M2 7h20v5H2z" />
                <path d="M12 22V7" />
                <path d="M12 7H7.5a2.5 2.5 0 0 1 0-5C11 2 12 7 12 7z" />
                <path d="M12 7h4.5a2.5 2.5 0 0 0 0-5C13 2 12 7 12 7z" />
              </svg>
            </div>
            <h3 class="benefit-title">{{ benefit.title }}</h3>
            <p class="benefit-description">{{ benefit.description }}</p>
          </div>
        </div>
      </section>

      <!-- CTA Section -->
      <section class="cta-section">
        <div class="cta-card">
          <h2 class="cta-title">{{ t('referral.landing.ctaTitle') }}</h2>
          <p class="cta-description">{{ t('referral.landing.ctaDescription') }}</p>

          <button
            v-if="isValidCode"
            class="btn btn-primary cta-button"
            @click="proceedToSignUp"
          >
            {{ t('referral.landing.signUpButton') }}
          </button>
          <button
            v-else
            class="btn btn-primary cta-button"
            @click="proceedToLogin"
          >
            {{ t('referral.landing.continueButton') }}
          </button>

          <p class="cta-login-hint">
            {{ t('auth.hasAccount') }}
            <button type="button" class="link-button" @click="proceedToLogin">
              {{ t('auth.login') }}
            </button>
          </p>
        </div>
      </section>

      <!-- Footer -->
      <footer class="landing-footer">
        <p>{{ t('referral.landing.footer') }}</p>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.referral-landing {
  min-height: 100vh;
  background: linear-gradient(135deg, var(--color-bg-secondary) 0%, var(--color-bg-tertiary) 100%);
}

.referral-landing.rtl {
  direction: rtl;
}

.loading-container {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  color: var(--color-text-secondary);
}

.loading-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.landing-container {
  max-width: 600px;
  margin: 0 auto;
  padding: var(--spacing-lg);
}

/* Header */
.landing-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) 0;
}

.logo {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.logo-text {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.lang-toggle {
  padding: var(--spacing-xs) var(--spacing-sm);
  background: none;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.lang-toggle:hover {
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
}

/* Hero Section */
.hero-section {
  text-align: center;
  padding: var(--spacing-2xl) 0;
}

.hero-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-md);
  background-color: var(--color-primary);
  color: white;
  border-radius: var(--radius-full);
  font-size: var(--font-size-sm);
  font-weight: 500;
  margin-bottom: var(--spacing-lg);
}

.hero-title {
  font-size: var(--font-size-3xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-md);
  line-height: 1.2;
}

.hero-subtitle {
  font-size: var(--font-size-lg);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-xl);
}

.hero-subtitle.error {
  color: #DC2626;
}

.hero-illustration {
  display: flex;
  justify-content: center;
  margin-top: var(--spacing-xl);
}

/* Benefits Section */
.benefits-section {
  padding: var(--spacing-xl) 0;
}

.section-title {
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-text-primary);
  text-align: center;
  margin-bottom: var(--spacing-xl);
}

.benefits-grid {
  display: grid;
  gap: var(--spacing-md);
}

.benefit-card {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  box-shadow: var(--shadow-sm);
}

.benefit-icon {
  width: 48px;
  height: 48px;
  border-radius: var(--radius-md);
  background-color: var(--color-primary-light, #E0F2FE);
  color: var(--color-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: var(--spacing-md);
}

.benefit-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.benefit-description {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  line-height: 1.5;
}

/* CTA Section */
.cta-section {
  padding: var(--spacing-xl) 0;
}

.cta-card {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-xl);
  padding: var(--spacing-2xl);
  box-shadow: var(--shadow-lg);
  text-align: center;
}

.cta-title {
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-sm);
}

.cta-description {
  font-size: var(--font-size-md);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-xl);
}

.cta-button {
  width: 100%;
  padding: var(--spacing-md) var(--spacing-xl);
  font-size: var(--font-size-md);
  font-weight: 600;
}

.cta-login-hint {
  margin-top: var(--spacing-lg);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.link-button {
  background: none;
  border: none;
  color: var(--color-primary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  padding: 0;
}

.link-button:hover {
  text-decoration: underline;
}

/* Footer */
.landing-footer {
  text-align: center;
  padding: var(--spacing-xl) 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Responsive */
@media (min-width: 480px) {
  .benefits-grid {
    grid-template-columns: repeat(3, 1fr);
  }

  .benefit-card {
    text-align: center;
  }

  .benefit-icon {
    margin-left: auto;
    margin-right: auto;
  }
}
</style>
