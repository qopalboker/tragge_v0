<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { affiliateApi, type AffiliateStatus } from '@/api';
import { useToast } from '@/composables/useToast';

const router = useRouter();
const toast = useToast();

const affiliateStatus = ref<AffiliateStatus | null>(null);
const isLoading = ref(true);
const isRequesting = ref(false);

// Computed properties for display
const isActivated = computed(() => affiliateStatus.value?.status === 'active');
const isPending = computed(() => affiliateStatus.value?.status === 'pending');
const isRejected = computed(() => affiliateStatus.value?.status === 'rejected');
const isInactive = computed(() => !affiliateStatus.value || affiliateStatus.value.status === 'inactive');

const formattedTotalEarned = computed(() => {
  if (!affiliateStatus.value?.stats) return '$0.00';
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
  }).format(affiliateStatus.value.stats.total_earned / 100);
});

const formattedPendingEarnings = computed(() => {
  if (!affiliateStatus.value?.stats) return '$0.00';
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
  }).format(affiliateStatus.value.stats.pending_earnings / 100);
});

async function loadAffiliateStatus(): Promise<void> {
  isLoading.value = true;
  try {
    affiliateStatus.value = await affiliateApi.getStatus();
  } catch {
    // Silently fail - affiliate status is optional
  } finally {
    isLoading.value = false;
  }
}

async function requestActivation(): Promise<void> {
  if (isRequesting.value) return;

  isRequesting.value = true;
  try {
    await affiliateApi.requestActivation();
    toast.success(t('affiliate.activationRequested'));
    // Reload status to show pending state
    await loadAffiliateStatus();
  } catch {
    toast.error(t('affiliate.activationRequestFailed'));
  } finally {
    isRequesting.value = false;
  }
}

function navigateToAffiliate(): void {
  router.push('/user/affiliate');
}

async function copyReferralCode(): Promise<void> {
  if (!affiliateStatus.value?.code) return;

  try {
    const referralLink = `${window.location.origin}/ref/${affiliateStatus.value.code}`;
    await navigator.clipboard.writeText(referralLink);
    toast.success(t('affiliate.linkCopied'));
  } catch {
    toast.error(t('affiliate.copyFailed'));
  }
}

onMounted(() => {
  loadAffiliateStatus();
});
</script>

<template>
  <div class="affiliate-summary-card" :class="{ 'card-activated': isActivated }">
    <!-- Loading State -->
    <div v-if="isLoading" class="loading-state">
      <div class="spinner-small"></div>
    </div>

    <!-- Inactive State - Not Yet Requested -->
    <div v-else-if="isInactive" class="inactive-state">
      <div class="state-content">
        <div class="icon-wrapper inactive-icon">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
            <circle cx="9" cy="7" r="4" />
            <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
            <path d="M16 3.13a4 4 0 0 1 0 7.75" />
          </svg>
        </div>
        <div class="info">
          <h4 class="title">{{ t('affiliate.programTitle') }}</h4>
          <p class="description">{{ t('affiliate.programDescription') }}</p>
        </div>
      </div>
      <button
        class="btn btn-primary request-btn"
        :disabled="isRequesting"
        @click="requestActivation"
      >
        <span v-if="isRequesting" class="btn-loading">
          <span class="spinner-tiny"></span>
          {{ t('common.loading') }}
        </span>
        <span v-else>{{ t('affiliate.requestActivation') }}</span>
      </button>
    </div>

    <!-- Pending State - Awaiting Approval -->
    <div v-else-if="isPending" class="pending-state">
      <div class="state-content">
        <div class="icon-wrapper pending-icon">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <polyline points="12 6 12 12 16 14" />
          </svg>
          <span class="pulse-ring"></span>
        </div>
        <div class="info">
          <h4 class="title">{{ t('affiliate.programTitle') }}</h4>
          <p class="description pending-text">{{ t('affiliate.pendingApproval') }}</p>
        </div>
      </div>
      <div class="pending-badge">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <polyline points="12 6 12 12 16 14" />
        </svg>
        <span>{{ t('affiliate.statusPending') }}</span>
      </div>
    </div>

    <!-- Rejected State -->
    <div v-else-if="isRejected" class="rejected-state">
      <div class="state-content">
        <div class="icon-wrapper rejected-icon">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="15" y1="9" x2="9" y2="15" />
            <line x1="9" y1="9" x2="15" y2="15" />
          </svg>
        </div>
        <div class="info">
          <h4 class="title">{{ t('affiliate.programTitle') }}</h4>
          <p class="description rejected-text">{{ t('affiliate.activationRejected') }}</p>
        </div>
      </div>
      <button
        class="btn btn-secondary retry-btn"
        :disabled="isRequesting"
        @click="requestActivation"
      >
        <span v-if="isRequesting" class="btn-loading">
          <span class="spinner-tiny"></span>
          {{ t('common.loading') }}
        </span>
        <span v-else>{{ t('affiliate.requestAgain') }}</span>
      </button>
    </div>

    <!-- Activated State - Full Summary -->
    <div v-else-if="isActivated" class="activated-state" @click="navigateToAffiliate">
      <div class="stats-row">
        <div class="stat-item">
          <span class="stat-label">{{ t('affiliate.totalReferrals') }}</span>
          <span class="stat-value">{{ affiliateStatus?.stats?.total_referrals || 0 }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('affiliate.totalEarned') }}</span>
          <span class="stat-value highlight">{{ formattedTotalEarned }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('affiliate.pendingEarnings') }}</span>
          <span class="stat-value">{{ formattedPendingEarnings }}</span>
        </div>
      </div>

      <div class="referral-code-section">
        <div class="code-info">
          <span class="code-label">{{ t('affiliate.yourCode') }}</span>
          <span class="code-value">{{ affiliateStatus?.code }}</span>
        </div>
        <button
          class="copy-btn"
          @click.stop="copyReferralCode"
          :title="t('affiliate.copyLink')"
        >
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
          </svg>
        </button>
      </div>

      <div class="view-details">
        <span>{{ t('affiliate.viewDetails') }}</span>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M5 12h14M12 5l7 7-7 7"/>
        </svg>
      </div>
    </div>
  </div>
</template>

<style scoped>
.affiliate-summary-card {
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  transition: all var(--transition-fast);
}

.affiliate-summary-card.card-activated {
  background: linear-gradient(135deg, #059669 0%, #10B981 100%);
  border: none;
  color: white;
  cursor: pointer;
}

.affiliate-summary-card.card-activated:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-lg);
}

/* Loading */
.loading-state {
  display: flex;
  justify-content: center;
  padding: var(--spacing-md);
}

.spinner-small {
  width: 24px;
  height: 24px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

.spinner-tiny {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  display: inline-block;
  margin-right: var(--spacing-xs);
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* State Content Layout */
.state-content {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-md);
}

.icon-wrapper {
  width: 48px;
  height: 48px;
  border-radius: var(--radius-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  position: relative;
}

.inactive-icon {
  background: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

.pending-icon {
  background: #FEF3C7;
  color: #D97706;
}

.rejected-icon {
  background: #FEE2E2;
  color: #DC2626;
}

.pulse-ring {
  position: absolute;
  width: 100%;
  height: 100%;
  border-radius: var(--radius-lg);
  border: 2px solid #D97706;
  animation: pulse-ring 2s ease-out infinite;
}

@keyframes pulse-ring {
  0% {
    transform: scale(1);
    opacity: 1;
  }
  100% {
    transform: scale(1.3);
    opacity: 0;
  }
}

.info {
  flex: 1;
  min-width: 0;
}

.title {
  font-size: var(--font-size-md);
  font-weight: 600;
  margin: 0 0 var(--spacing-xs) 0;
  color: var(--color-text-primary);
}

.description {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
  line-height: 1.4;
}

.pending-text {
  color: #D97706;
}

.rejected-text {
  color: #DC2626;
}

/* Buttons */
.request-btn,
.retry-btn {
  width: 100%;
  justify-content: center;
}

.btn-loading {
  display: flex;
  align-items: center;
  justify-content: center;
}

/* Pending Badge */
.pending-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: #FEF3C7;
  color: #D97706;
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 500;
}

/* Activated State */
.activated-state {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-md);
}

.stat-item {
  text-align: center;
}

.stat-label {
  display: block;
  font-size: var(--font-size-xs);
  opacity: 0.85;
  margin-bottom: 2px;
}

.stat-value {
  display: block;
  font-size: var(--font-size-lg);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.stat-value.highlight {
  font-size: var(--font-size-xl);
}

.referral-code-section {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: rgba(255, 255, 255, 0.15);
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
}

.code-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.code-label {
  font-size: var(--font-size-xs);
  opacity: 0.85;
}

.code-value {
  font-size: var(--font-size-md);
  font-weight: 600;
  font-family: monospace;
  letter-spacing: 0.05em;
}

.copy-btn {
  background: rgba(255, 255, 255, 0.2);
  border: none;
  padding: var(--spacing-sm);
  border-radius: var(--radius-md);
  color: white;
  cursor: pointer;
  transition: background var(--transition-fast);
}

.copy-btn:hover {
  background: rgba(255, 255, 255, 0.3);
}

.view-details {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  font-weight: 500;
  opacity: 0.9;
  padding-top: var(--spacing-sm);
  border-top: 1px solid rgba(255, 255, 255, 0.2);
}

.view-details svg {
  transition: transform var(--transition-fast);
}

.activated-state:hover .view-details svg {
  transform: translateX(4px);
}

[dir="rtl"] .view-details svg {
  transform: rotate(180deg);
}

[dir="rtl"] .activated-state:hover .view-details svg {
  transform: rotate(180deg) translateX(4px);
}

/* Mobile */
@media (max-width: 767px) {
  .stats-row {
    grid-template-columns: 1fr;
    gap: var(--spacing-sm);
  }

  .stat-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    text-align: left;
    padding: var(--spacing-xs) 0;
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  }

  .stat-item:last-child {
    border-bottom: none;
  }

  .stat-label {
    margin-bottom: 0;
  }

  .stat-value {
    font-size: var(--font-size-md);
  }

  .stat-value.highlight {
    font-size: var(--font-size-lg);
  }
}
</style>
