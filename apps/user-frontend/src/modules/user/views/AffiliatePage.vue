<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { affiliateApi, type AffiliateStats, type Referral } from '@/api';

const toast = useToast();

// State
const loading = ref(true);
const error = ref<string | null>(null);
const stats = ref<AffiliateStats | null>(null);
const referrals = ref<Referral[]>([]);
const referralsLoading = ref(false);
const copySuccess = ref(false);

// Computed
const referralLink = computed(() => {
  if (!stats.value?.referral_code) return '';
  return `https://tragge.com/register?ref=${stats.value.referral_code}`;
});

const formattedTotalEarned = computed(() => {
  return formatCurrency(stats.value?.total_earned ?? 0);
});

const formattedPendingEarnings = computed(() => {
  return formatCurrency(stats.value?.pending_earnings ?? 0);
});

// Functions
function formatCurrency(amount: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amount);
}

function formatDate(dateString: string): string {
  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }).format(new Date(dateString));
}

function maskEmail(email: string): string {
  const [localPart, domain] = email.split('@');
  if (localPart.length <= 2) {
    return `${localPart[0]}***@${domain}`;
  }
  return `${localPart.slice(0, 2)}***@${domain}`;
}

async function copyToClipboard(): Promise<void> {
  try {
    await navigator.clipboard.writeText(referralLink.value);
    copySuccess.value = true;
    toast.success(t('affiliate.linkCopied'));
    setTimeout(() => {
      copySuccess.value = false;
    }, 2000);
  } catch {
    toast.error(t('affiliate.copyFailed'));
  }
}

function shareTwitter(): void {
  const text = encodeURIComponent(t('affiliate.shareMessage'));
  const url = encodeURIComponent(referralLink.value);
  window.open(`https://twitter.com/intent/tweet?text=${text}&url=${url}`, '_blank');
}

function shareTelegram(): void {
  const url = encodeURIComponent(referralLink.value);
  const text = encodeURIComponent(t('affiliate.shareMessage'));
  window.open(`https://t.me/share/url?url=${url}&text=${text}`, '_blank');
}

function shareWhatsApp(): void {
  const text = encodeURIComponent(`${t('affiliate.shareMessage')} ${referralLink.value}`);
  window.open(`https://wa.me/?text=${text}`, '_blank');
}

function shareEmail(): void {
  const subject = encodeURIComponent(t('affiliate.emailSubject'));
  const body = encodeURIComponent(`${t('affiliate.emailBody')}\n\n${referralLink.value}`);
  window.location.href = `mailto:?subject=${subject}&body=${body}`;
}

async function loadStats(): Promise<void> {
  loading.value = true;
  error.value = null;
  try {
    stats.value = await affiliateApi.getStats();
  } catch {
    error.value = t('affiliate.loadError');
  } finally {
    loading.value = false;
  }
}

async function loadReferrals(): Promise<void> {
  referralsLoading.value = true;
  try {
    referrals.value = await affiliateApi.getReferrals();
  } catch {
    // Silently fail for referrals, stats are more important
  } finally {
    referralsLoading.value = false;
  }
}

onMounted(async () => {
  await Promise.all([loadStats(), loadReferrals()]);
});
</script>

<template>
  <div class="affiliate-page">
    <!-- Page Header -->
    <div class="page-header">
      <h1 class="page-title">{{ t('affiliate.title') }}</h1>
      <p class="page-subtitle">{{ t('affiliate.subtitle') }}</p>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="error-state">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="12" cy="12" r="10" />
        <line x1="12" y1="8" x2="12" y2="12" />
        <line x1="12" y1="16" x2="12.01" y2="16" />
      </svg>
      <p>{{ error }}</p>
      <button class="btn btn-primary" @click="loadStats">{{ t('common.retry') }}</button>
    </div>

    <!-- Main Content -->
    <template v-else>
      <!-- Stats Cards -->
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-icon referrals-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
              <circle cx="9" cy="7" r="4" />
              <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
              <path d="M16 3.13a4 4 0 0 1 0 7.75" />
            </svg>
          </div>
          <div class="stat-info">
            <span class="stat-label">{{ t('affiliate.totalReferrals') }}</span>
            <span class="stat-value">{{ stats?.total_referrals ?? 0 }}</span>
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-icon qualified-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
              <polyline points="22 4 12 14.01 9 11.01" />
            </svg>
          </div>
          <div class="stat-info">
            <span class="stat-label">{{ t('affiliate.qualifiedReferrals') }}</span>
            <span class="stat-value">{{ stats?.qualified_referrals ?? 0 }}</span>
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-icon earned-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="12" y1="1" x2="12" y2="23" />
              <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
            </svg>
          </div>
          <div class="stat-info">
            <span class="stat-label">{{ t('affiliate.totalEarned') }}</span>
            <span class="stat-value positive">{{ formattedTotalEarned }}</span>
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-icon pending-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <polyline points="12 6 12 12 16 14" />
            </svg>
          </div>
          <div class="stat-info">
            <span class="stat-label">{{ t('affiliate.pendingEarnings') }}</span>
            <span class="stat-value pending">{{ formattedPendingEarnings }}</span>
          </div>
        </div>
      </div>

      <!-- Referral Link Section -->
      <div class="referral-card">
        <h2 class="section-title">{{ t('affiliate.yourReferralLink') }}</h2>
        <p class="section-description">{{ t('affiliate.shareLinkDescription') }}</p>

        <div class="referral-link-wrapper">
          <input
            type="text"
            class="referral-link-input"
            :value="referralLink"
            readonly
          />
          <button
            :class="['copy-btn', { 'copy-success': copySuccess }]"
            @click="copyToClipboard"
          >
            <svg v-if="!copySuccess" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
              <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
            </svg>
            <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            <span>{{ copySuccess ? t('affiliate.copied') : t('affiliate.copy') }}</span>
          </button>
        </div>

        <div class="share-section">
          <span class="share-label">{{ t('affiliate.shareVia') }}</span>
          <div class="share-buttons">
            <button class="share-btn twitter" @click="shareTwitter" :title="t('affiliate.shareTwitter')">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
              </svg>
            </button>
            <button class="share-btn telegram" @click="shareTelegram" :title="t('affiliate.shareTelegram')">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z" />
              </svg>
            </button>
            <button class="share-btn whatsapp" @click="shareWhatsApp" :title="t('affiliate.shareWhatsApp')">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                <path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z" />
              </svg>
            </button>
            <button class="share-btn email" @click="shareEmail" :title="t('affiliate.shareEmail')">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" />
                <polyline points="22,6 12,13 2,6" />
              </svg>
            </button>
          </div>
        </div>
      </div>

      <!-- How It Works -->
      <div class="how-it-works-card">
        <h2 class="section-title">{{ t('affiliate.howItWorks') }}</h2>

        <div class="steps-container">
          <div class="step">
            <div class="step-number">1</div>
            <div class="step-icon share-icon">
              <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="18" cy="5" r="3" />
                <circle cx="6" cy="12" r="3" />
                <circle cx="18" cy="19" r="3" />
                <line x1="8.59" y1="13.51" x2="15.42" y2="17.49" />
                <line x1="15.41" y1="6.51" x2="8.59" y2="10.49" />
              </svg>
            </div>
            <h3 class="step-title">{{ t('affiliate.step1Title') }}</h3>
            <p class="step-description">{{ t('affiliate.step1Description') }}</p>
          </div>

          <div class="step-connector">
            <svg width="40" height="24" viewBox="0 0 40 24" fill="none">
              <path d="M0 12h35M30 6l6 6-6 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </div>

          <div class="step">
            <div class="step-number">2</div>
            <div class="step-icon signup-icon">
              <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="8.5" cy="7" r="4" />
                <line x1="20" y1="8" x2="20" y2="14" />
                <line x1="23" y1="11" x2="17" y2="11" />
              </svg>
            </div>
            <h3 class="step-title">{{ t('affiliate.step2Title') }}</h3>
            <p class="step-description">{{ t('affiliate.step2Description') }}</p>
          </div>

          <div class="step-connector">
            <svg width="40" height="24" viewBox="0 0 40 24" fill="none">
              <path d="M0 12h35M30 6l6 6-6 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </div>

          <div class="step">
            <div class="step-number">3</div>
            <div class="step-icon earn-icon">
              <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="12" y1="1" x2="12" y2="23" />
                <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
              </svg>
            </div>
            <h3 class="step-title">{{ t('affiliate.step3Title') }}</h3>
            <p class="step-description">{{ t('affiliate.step3Description') }}</p>
          </div>
        </div>
      </div>

      <!-- Terms Summary -->
      <div class="terms-card">
        <h2 class="section-title">{{ t('affiliate.termsTitle') }}</h2>

        <div class="terms-grid">
          <div class="term-item">
            <div class="term-icon">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 2a10 10 0 1 0 10 10 4 4 0 0 1-5-5 4 4 0 0 1-5-5" />
                <path d="M8.5 8.5v.01" />
                <path d="M16 15.5v.01" />
                <path d="M12 12v.01" />
                <path d="M11 17v.01" />
                <path d="M7 14v.01" />
              </svg>
            </div>
            <div class="term-content">
              <span class="term-label">{{ t('affiliate.commissionRate') }}</span>
              <span class="term-value">5%</span>
            </div>
          </div>

          <div class="term-item">
            <div class="term-icon">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
                <polyline points="22 4 12 14.01 9 11.01" />
              </svg>
            </div>
            <div class="term-content">
              <span class="term-label">{{ t('affiliate.qualification') }}</span>
              <span class="term-value">{{ t('affiliate.qualificationValue') }}</span>
            </div>
          </div>

          <div class="term-item">
            <div class="term-icon">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
                <line x1="16" y1="2" x2="16" y2="6" />
                <line x1="8" y1="2" x2="8" y2="6" />
                <line x1="3" y1="10" x2="21" y2="10" />
              </svg>
            </div>
            <div class="term-content">
              <span class="term-label">{{ t('affiliate.payoutSchedule') }}</span>
              <span class="term-value">{{ t('affiliate.payoutScheduleValue') }}</span>
            </div>
          </div>

          <div class="term-item">
            <div class="term-icon">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="12" y1="1" x2="12" y2="23" />
                <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
              </svg>
            </div>
            <div class="term-content">
              <span class="term-label">{{ t('affiliate.minimumPayout') }}</span>
              <span class="term-value">$10</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Referrals Table -->
      <div class="referrals-card">
        <h2 class="section-title">{{ t('affiliate.yourReferrals') }}</h2>

        <div v-if="referralsLoading" class="loading-state small">
          <div class="spinner"></div>
        </div>

        <div v-else-if="referrals.length === 0" class="empty-state">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
            <circle cx="9" cy="7" r="4" />
            <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
            <path d="M16 3.13a4 4 0 0 1 0 7.75" />
          </svg>
          <p>{{ t('affiliate.noReferrals') }}</p>
          <p class="empty-hint">{{ t('affiliate.noReferralsHint') }}</p>
        </div>

        <div v-else class="referrals-table-wrapper">
          <table class="referrals-table">
            <thead>
              <tr>
                <th>{{ t('affiliate.tableUser') }}</th>
                <th>{{ t('affiliate.tableStatus') }}</th>
                <th>{{ t('affiliate.tableJoined') }}</th>
                <th>{{ t('affiliate.tableQualifiedDate') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="referral in referrals" :key="referral.id">
                <td class="user-cell">
                  <div class="user-avatar">
                    {{ referral.email.charAt(0).toUpperCase() }}
                  </div>
                  <span class="user-email">{{ maskEmail(referral.email) }}</span>
                </td>
                <td>
                  <span :class="['status-badge', referral.status]">
                    {{ t(`affiliate.status.${referral.status}`) }}
                  </span>
                </td>
                <td class="date-cell">{{ formatDate(referral.joined_at) }}</td>
                <td class="date-cell">
                  {{ referral.qualified_at ? formatDate(referral.qualified_at) : '-' }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.affiliate-page {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

/* Page Header */
.page-header {
  margin-bottom: var(--spacing-md);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-xs) 0;
}

.page-subtitle {
  font-size: var(--font-size-md);
  color: var(--color-text-secondary);
  margin: 0;
}

/* Stats Grid */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-md);
}

.stat-card {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

.stat-icon {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  flex-shrink: 0;
}

.referrals-icon {
  background-color: #DBEAFE;
  color: #2563EB;
}

.qualified-icon {
  background-color: #D1FAE5;
  color: #059669;
}

.earned-icon {
  background-color: #FEF3C7;
  color: #D97706;
}

.pending-icon {
  background-color: #E0E7FF;
  color: #4F46E5;
}

.stat-info {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.stat-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.stat-value {
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

.stat-value.positive {
  color: var(--color-success);
}

.stat-value.pending {
  color: #D97706;
}

/* Referral Card */
.referral-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
}

.section-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-sm) 0;
}

.section-description {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0 0 var(--spacing-lg) 0;
}

.referral-link-wrapper {
  display: flex;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-lg);
}

.referral-link-input {
  flex: 1;
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  font-family: monospace;
}

.copy-btn {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-md) var(--spacing-lg);
  background-color: var(--color-primary);
  color: white;
  border: none;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.copy-btn:hover {
  background-color: #1D4ED8;
}

.copy-btn.copy-success {
  background-color: var(--color-success);
}

/* Share Section */
.share-section {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding-top: var(--spacing-md);
  border-top: 1px solid var(--color-border);
}

.share-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.share-buttons {
  display: flex;
  gap: var(--spacing-sm);
}

.share-btn {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--radius-full);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.share-btn.twitter {
  background-color: #000000;
  color: white;
}

.share-btn.twitter:hover {
  background-color: #333333;
}

.share-btn.telegram {
  background-color: #0088cc;
  color: white;
}

.share-btn.telegram:hover {
  background-color: #006699;
}

.share-btn.whatsapp {
  background-color: #25D366;
  color: white;
}

.share-btn.whatsapp:hover {
  background-color: #1EBE5B;
}

.share-btn.email {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border);
}

.share-btn.email:hover {
  background-color: var(--color-bg-tertiary);
}

/* How It Works */
.how-it-works-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
}

.steps-container {
  display: flex;
  align-items: flex-start;
  justify-content: center;
  gap: var(--spacing-md);
  margin-top: var(--spacing-lg);
}

.step {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  flex: 1;
  max-width: 200px;
}

.step-number {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--color-primary);
  color: white;
  border-radius: var(--radius-full);
  font-size: var(--font-size-sm);
  font-weight: 600;
  margin-bottom: var(--spacing-md);
}

.step-icon {
  width: 64px;
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-lg);
  margin-bottom: var(--spacing-md);
}

.step-icon.share-icon {
  background-color: #DBEAFE;
  color: #2563EB;
}

.step-icon.signup-icon {
  background-color: #D1FAE5;
  color: #059669;
}

.step-icon.earn-icon {
  background-color: #FEF3C7;
  color: #D97706;
}

.step-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-xs) 0;
}

.step-description {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
  line-height: 1.5;
}

.step-connector {
  display: flex;
  align-items: center;
  color: var(--color-border);
  margin-top: 60px;
}

[dir="rtl"] .step-connector {
  transform: scaleX(-1);
}

/* Terms Card */
.terms-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
}

.terms-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-lg);
  margin-top: var(--spacing-lg);
}

.term-item {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-md);
}

.term-icon {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  color: var(--color-primary);
  flex-shrink: 0;
}

.term-content {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.term-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.term-value {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
}

/* Referrals Card */
.referrals-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
}

.referrals-table-wrapper {
  overflow-x: auto;
  margin-top: var(--spacing-lg);
}

.referrals-table {
  width: 100%;
  border-collapse: collapse;
}

.referrals-table th,
.referrals-table td {
  padding: var(--spacing-md);
  text-align: left;
  border-bottom: 1px solid var(--color-border);
}

[dir="rtl"] .referrals-table th,
[dir="rtl"] .referrals-table td {
  text-align: right;
}

.referrals-table th {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  background-color: var(--color-bg-secondary);
}

.referrals-table th:first-child {
  border-radius: var(--radius-md) 0 0 var(--radius-md);
}

[dir="rtl"] .referrals-table th:first-child {
  border-radius: 0 var(--radius-md) var(--radius-md) 0;
}

.referrals-table th:last-child {
  border-radius: 0 var(--radius-md) var(--radius-md) 0;
}

[dir="rtl"] .referrals-table th:last-child {
  border-radius: var(--radius-md) 0 0 var(--radius-md);
}

.user-cell {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.user-avatar {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--color-primary-light);
  color: var(--color-primary);
  border-radius: var(--radius-full);
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.user-email {
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.status-badge {
  display: inline-flex;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 500;
}

.status-badge.pending {
  background-color: #FEF3C7;
  color: #D97706;
}

.status-badge.qualified {
  background-color: #D1FAE5;
  color: #059669;
}

.date-cell {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

/* Loading & Empty States */
.loading-state,
.empty-state,
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-2xl);
  gap: var(--spacing-md);
  color: var(--color-text-secondary);
}

.loading-state.small {
  padding: var(--spacing-lg);
}

.empty-state svg,
.error-state svg {
  color: var(--color-text-muted);
}

.empty-hint {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  margin: 0;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Responsive */
@media (max-width: 1023px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .terms-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .steps-container {
    flex-direction: column;
    align-items: center;
  }

  .step {
    max-width: 300px;
  }

  .step-connector {
    transform: rotate(90deg);
    margin: var(--spacing-md) 0;
  }

  [dir="rtl"] .step-connector {
    transform: rotate(90deg) scaleX(-1);
  }
}

@media (max-width: 767px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }

  .terms-grid {
    grid-template-columns: 1fr;
  }

  .page-title {
    font-size: var(--font-size-xl);
  }

  .referral-link-wrapper {
    flex-direction: column;
  }

  .copy-btn {
    width: 100%;
    justify-content: center;
  }

  .share-section {
    flex-direction: column;
    align-items: flex-start;
  }

  .referrals-table th:nth-child(4),
  .referrals-table td:nth-child(4) {
    display: none;
  }
}
</style>
