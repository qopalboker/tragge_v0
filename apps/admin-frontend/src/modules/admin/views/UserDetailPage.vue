<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { useAuthStore } from '@/stores/auth';
import { SensitiveAdminAction, withPasswordReauthentication } from '@/api/reauthentication';
import {
  getUser,
  updateUserRoles,
  resetSuperAdminMFA,
  banUser,
  unbanUser,
  terminateUserSessions,
  chargeUserWallet,
  getUserWalletHistory,
  type UserDetail,
  type BanUserRequest,
  type AdminWalletHistoryEntry,
  type AdminWalletHistoryResponse,
} from '@/api/users';

const route = useRoute();
const router = useRouter();
const toast = useToast();
const auth = useAuthStore();

// Permission helpers
const canEditUsers = computed(() => auth.hasPermission('users.edit'));
const canChargeWallet = computed(() => auth.hasPermission('users.wallet.charge'));

// State
const user = ref<UserDetail | null>(null);
const loading = ref(true);
const activeTab = ref('profile');

// Modals
const showRoleModal = ref(false);
const showBanModal = ref(false);
const showChargeModal = ref(false);
const showTerminateSessionsModal = ref(false);
const modalLoading = ref(false);

// Form state
const selectedRoles = ref<string[]>([]);
const banReason = ref('');
const banDuration = ref<'permanent' | '7d' | '30d'>('permanent');
const chargeAmount = ref<number>(0);
const chargeReason = ref('');
const chargeErrors = ref<Record<string, string>>({});

// Wallet history state
const walletHistory = ref<AdminWalletHistoryResponse | null>(null);
const walletHistoryLoading = ref(false);
const walletHistoryPage = ref(1);
const walletHistoryFilter = ref<string>('all');
const walletPerPage = 20;

const allRoles = ['user', 'support_admin', 'super_admin'];
const availableRoles = computed(() => {
  if (auth.isSuperAdmin) return allRoles;
  return ['user'];
});
const tabs = ['profile', 'kyc', 'wallet', 'contests', 'affiliate', 'sessions', 'audit'];

const userId = computed(() => route.params.id as string);

async function fetchUser(): Promise<void> {
  if (!userId.value) return;
  loading.value = true;
  try {
    user.value = await getUser(userId.value);
  } catch {
    toast.error(t('userDetail.loadError'));
    router.push('/admin/users');
  } finally {
    loading.value = false;
  }
}

function goBack(): void {
  router.push('/admin/users');
}

function openRoleModal(): void {
  if (!user.value) return;
  selectedRoles.value = [...user.value.roles];
  showRoleModal.value = true;
}

function openBanModal(): void {
  banReason.value = '';
  banDuration.value = 'permanent';
  showBanModal.value = true;
}

function openChargeModal(): void {
  chargeAmount.value = 0;
  chargeReason.value = '';
  chargeErrors.value = {};
  showChargeModal.value = true;
}

async function saveRoles(): Promise<void> {
  if (!user.value) return;
  modalLoading.value = true;
  try {
    const reason = window.prompt('Reason for this privileged role change:')?.trim();
    const password = window.prompt('Confirm your current Admin password:') || '';
    if (!reason || !password) return;
    await withPasswordReauthentication({
      password, action: SensitiveAdminAction.UserRolesUpdate, resourceId: user.value.user.id,
    }, grant => updateUserRoles(user.value!.user.id, { roles: selectedRoles.value, reason }, grant));
    user.value.roles = [...selectedRoles.value];
    toast.success(t('users.rolesUpdated'));
    showRoleModal.value = false;
  } catch {
    toast.error(t('users.rolesUpdateError'));
  } finally {
    modalLoading.value = false;
  }
}

async function handleResetSuperAdminMFA(): Promise<void> {
  if (!user.value || !auth.isSuperAdmin || !user.value.roles.includes('super_admin')) return;
  const reason = window.prompt(t('userDetail.mfaResetReasonPrompt'))?.trim();
  const password = window.prompt(t('userDetail.mfaResetPasswordPrompt')) || '';
  if (!reason || !password) return;
  modalLoading.value = true;
  try {
    await withPasswordReauthentication({
      password,
      action: SensitiveAdminAction.AdminMFAReset,
      resourceId: user.value.user.id,
    }, grant => resetSuperAdminMFA(user.value!.user.id, reason, grant));
    toast.success(t('userDetail.mfaResetSuccess'));
  } catch {
    toast.error(t('userDetail.mfaResetError'));
  } finally {
    modalLoading.value = false;
  }
}

async function handleBanUser(): Promise<void> {
  if (!user.value) return;
  modalLoading.value = true;
  try {
    const req: BanUserRequest = {
      reason: banReason.value,
      duration: banDuration.value,
    };
    await banUser(user.value.user.id, req);
    user.value.user.status = 'suspended';
    toast.success(t('userDetail.banSuccess'));
    showBanModal.value = false;
  } catch {
    toast.error(t('userDetail.banError'));
  } finally {
    modalLoading.value = false;
  }
}

async function handleUnbanUser(): Promise<void> {
  if (!user.value) return;
  modalLoading.value = true;
  try {
    await unbanUser(user.value.user.id);
    user.value.user.status = 'active';
    toast.success(t('userDetail.unbanSuccess'));
  } catch {
    toast.error(t('userDetail.unbanError'));
  } finally {
    modalLoading.value = false;
  }
}

const MAX_CHARGE_AMOUNT = 10000;

function validateCharge(): boolean {
  const errors: Record<string, string> = {};
  if (chargeAmount.value === 0) {
    errors.amount = t('userDetail.validation.amountZero');
  } else if (Math.abs(chargeAmount.value) > MAX_CHARGE_AMOUNT) {
    errors.amount = t('userDetail.validation.amountMax', { max: MAX_CHARGE_AMOUNT.toLocaleString() });
  }
  if (!chargeReason.value.trim()) {
    errors.reason = t('userDetail.validation.reasonRequired');
  }
  chargeErrors.value = errors;
  return Object.keys(errors).length === 0;
}

async function handleChargeWallet(): Promise<void> {
  if (!user.value) return;
  if (!validateCharge()) return;

  const amount = chargeAmount.value;
  const confirmMsg = amount > 0
    ? t('userDetail.confirmChargeCredit', { amount: Math.abs(amount).toFixed(2) })
    : t('userDetail.confirmChargeDebit', { amount: Math.abs(amount).toFixed(2) });
  if (!confirm(confirmMsg)) return;

  modalLoading.value = true;
  try {
    const amountCents = Math.round(chargeAmount.value * 100);
    const reason = chargeReason.value.trim();
    const password = window.prompt('Confirm your current Admin password:') || '';
    if (!password) return;
    const response = await withPasswordReauthentication({
      password, action: SensitiveAdminAction.WalletAdjust, resourceId: user.value.user.id,
    }, grant => chargeUserWallet(user.value!.user.id, { amount: amountCents, reason }, grant));
    user.value.wallet.balance_cents = response.new_balance;
    toast.success(t('userDetail.chargeSuccess'));
    showChargeModal.value = false;
    chargeAmount.value = 0;
    chargeReason.value = '';
    chargeErrors.value = {};
  } catch {
    toast.error(t('userDetail.chargeError'));
  } finally {
    modalLoading.value = false;
  }
}

async function handleTerminateSessions(): Promise<void> {
  if (!user.value) return;
  modalLoading.value = true;
  try {
    const result = await terminateUserSessions(user.value.user.id);
    user.value.sessions = [];
    toast.success(t('userDetail.sessionsTerminated', { count: result.sessions_terminated }));
    showTerminateSessionsModal.value = false;
  } catch {
    toast.error(t('userDetail.sessionsTerminateError'));
  } finally {
    modalLoading.value = false;
  }
}

function toggleRole(role: string): void {
  const idx = selectedRoles.value.indexOf(role);
  if (idx === -1) {
    selectedRoles.value.push(role);
  } else {
    selectedRoles.value.splice(idx, 1);
  }
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString();
}

function formatDateTime(dateString: string): string {
  return new Date(dateString).toLocaleString();
}

function formatCurrency(cents: number, currency: string = 'USD'): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
  }).format(cents / 100);
}

function getStatusClass(status: string): string {
  const classes: Record<string, string> = {
    active: 'status-active',
    suspended: 'status-suspended',
    pending: 'status-pending',
  };
  return classes[status] || 'status-default';
}

function getKycStatusClass(status: string): string {
  const classes: Record<string, string> = {
    verified: 'kyc-verified',
    approved: 'kyc-verified',
    pending: 'kyc-pending',
    rejected: 'kyc-rejected',
    none: 'kyc-none',
  };
  return classes[status] || 'kyc-none';
}

function getRoleBadgeClass(role: string): string {
  const classes: Record<string, string> = {
    admin: 'role-admin',
    moderator: 'role-moderator',
    user: 'role-user',
    viewer: 'role-viewer',
    super_admin: 'role-super-admin',
  };
  return classes[role] || 'role-user';
}

function getTransactionTypeClass(type: string): string {
  const positive = ['deposit', 'credit', 'admin_charge', 'prize', 'refund'];
  return positive.includes(type) ? 'tx-positive' : 'tx-negative';
}

// Wallet history functions
async function fetchWalletHistory(page = 1, type?: string): Promise<void> {
  if (!user.value) return;
  walletHistoryLoading.value = true;
  try {
    walletHistory.value = await getUserWalletHistory(user.value.user.id, {
      page,
      limit: walletPerPage,
      type: type && type !== 'all' ? type : undefined,
    });
    walletHistoryPage.value = page;
  } catch (err) {
    console.error('Failed to fetch wallet history', err);
  } finally {
    walletHistoryLoading.value = false;
  }
}

function getWalletEntryLabel(entry: AdminWalletHistoryEntry): string {
  if (entry.description) return entry.description;
  if (entry.reason_code) {
    const key = `userDetail.walletReasonCodes.${entry.reason_code}`;
    const translated = t(key);
    if (translated !== key) return translated;
  }
  const typeKey = `userDetail.txTypes.${entry.type}`;
  const translated = t(typeKey);
  return translated !== typeKey ? translated : entry.type;
}

function formatCents(cents: number, currency = 'USD'): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
    minimumFractionDigits: 2,
  }).format(cents / 100);
}

const walletFilterOptions = [
  { value: 'all', label: '\u0647\u0645\u0647' },
  { value: 'deposit', label: '\u0648\u0627\u0631\u06CC\u0632' },
  { value: 'withdrawal', label: '\u0628\u0631\u062F\u0627\u0634\u062A' },
  { value: 'prize_credit', label: '\u062C\u0627\u06CC\u0632\u0647' },
  { value: 'contest_entry', label: '\u0648\u0631\u0648\u062F\u06CC \u0645\u0633\u0627\u0628\u0642\u0647' },
  { value: 'contest_refund', label: '\u0628\u0627\u0632\u06AF\u0634\u062A \u0648\u0631\u0648\u062F\u06CC' },
  { value: 'adjustment', label: '\u062A\u0646\u0638\u06CC\u0645' },
  { value: 'affiliate_commission', label: '\u06A9\u0645\u06CC\u0633\u06CC\u0648\u0646' },
];

const walletTotalPages = computed(() => {
  if (!walletHistory.value) return 1;
  return Math.ceil(walletHistory.value.total / walletPerPage);
});

watch(activeTab, (newTab) => {
  if (newTab === 'wallet' && !walletHistory.value) {
    fetchWalletHistory();
  }
});

watch(userId, fetchUser);
onMounted(fetchUser);
</script>

<template>
  <div class="user-detail-page">
    <!-- Header with back button -->
    <div class="page-header">
      <button class="btn btn-ghost" @click="goBack">
        <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
          <path fill-rule="evenodd" d="M9.707 16.707a1 1 0 01-1.414 0l-6-6a1 1 0 010-1.414l6-6a1 1 0 011.414 1.414L5.414 9H17a1 1 0 110 2H5.414l4.293 4.293a1 1 0 010 1.414z" clip-rule="evenodd" />
        </svg>
        {{ t('common.back') }}
      </button>
      <h1 class="page-title">{{ t('userDetail.title') }}</h1>
    </div>

    <!-- Loading state -->
    <div v-if="loading" class="loading-container">
      <div class="loading">{{ t('common.loading') }}</div>
    </div>

    <!-- User content -->
    <div v-else-if="user" class="user-content">
      <!-- User profile card -->
      <div class="profile-card">
        <div class="profile-header">
          <div class="avatar-container">
            <img
              v-if="user.user.avatar_url"
              :src="user.user.avatar_url"
              :alt="user.user.display_name || user.user.email"
              class="avatar"
            />
            <div v-else class="avatar-placeholder">
              {{ (user.user.display_name || user.user.email)[0].toUpperCase() }}
            </div>
          </div>
          <div class="profile-info">
            <h2 class="user-name">{{ user.user.display_name || user.user.username || user.user.email }}</h2>
            <p class="user-email">{{ user.user.email }}</p>
            <div class="badges">
              <span :class="['status-badge', getStatusClass(user.user.status)]">
                {{ t(`users.status.${user.user.status}`) }}
              </span>
              <span
                v-for="role in user.roles"
                :key="role"
                :class="['role-badge', getRoleBadgeClass(role)]"
              >
                {{ t(`users.role.${role}`) }}
              </span>
              <span v-if="user.user.email_verified" class="verified-badge">
                {{ t('userDetail.emailVerified') }}
              </span>
            </div>
          </div>
          <div v-if="canEditUsers" class="profile-actions">
            <button
              v-if="auth.isSuperAdmin && user.roles.includes('super_admin')"
              class="btn btn-danger"
              data-testid="reset-super-admin-mfa"
              :disabled="modalLoading"
              @click="handleResetSuperAdminMFA"
            >
              {{ t('userDetail.mfaReset') }}
            </button>
            <button class="btn btn-ghost" @click="openRoleModal">
              {{ t('users.editRoles') }}
            </button>
            <button
              v-if="user.user.status === 'active'"
              class="btn btn-danger"
              @click="openBanModal"
            >
              {{ t('userDetail.ban') }}
            </button>
            <button
              v-else
              class="btn btn-success"
              :disabled="modalLoading"
              @click="handleUnbanUser"
            >
              {{ t('userDetail.unban') }}
            </button>
          </div>
        </div>
      </div>

      <!-- Tabs -->
      <div class="tabs">
        <button
          v-for="tab in tabs"
          :key="tab"
          :class="['tab-button', { active: activeTab === tab }]"
          @click="activeTab = tab"
        >
          {{ t(`userDetail.tabs.${tab}`) }}
        </button>
      </div>

      <!-- Tab content -->
      <div class="tab-content">
        <!-- Profile Tab -->
        <div v-if="activeTab === 'profile'" class="tab-panel">
          <div class="info-grid">
            <div class="info-card">
              <h3 class="card-title">{{ t('userDetail.basicInfo') }}</h3>
              <div class="info-rows">
                <div class="info-row">
                  <span class="info-label">{{ t('users.id') }}</span>
                  <span class="info-value mono">{{ user.user.id }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">{{ t('users.email') }}</span>
                  <span class="info-value">{{ user.user.email }}</span>
                </div>
                <div v-if="user.user.username" class="info-row">
                  <span class="info-label">{{ t('userDetail.username') }}</span>
                  <span class="info-value">{{ user.user.username }}</span>
                </div>
                <div v-if="user.user.display_name" class="info-row">
                  <span class="info-label">{{ t('userDetail.displayName') }}</span>
                  <span class="info-value">{{ user.user.display_name }}</span>
                </div>
                <div v-if="user.user.country" class="info-row">
                  <span class="info-label">{{ t('userDetail.country') }}</span>
                  <span class="info-value">{{ user.user.country }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">{{ t('users.createdAt') }}</span>
                  <span class="info-value">{{ formatDate(user.user.created_at) }}</span>
                </div>
              </div>
            </div>

            <div class="info-card">
              <h3 class="card-title">{{ t('userDetail.statistics') }}</h3>
              <div class="stats-grid">
                <div class="stat-item">
                  <span class="stat-value">{{ user.stats.total_contests }}</span>
                  <span class="stat-label">{{ t('userDetail.totalContests') }}</span>
                </div>
                <div class="stat-item">
                  <span class="stat-value">{{ user.stats.total_wins }}</span>
                  <span class="stat-label">{{ t('userDetail.totalWins') }}</span>
                </div>
                <div class="stat-item">
                  <span class="stat-value">{{ user.stats.tragge_point.toFixed(0) }}</span>
                  <span class="stat-label">{{ t('userDetail.traggePoint') }}</span>
                </div>
                <div class="stat-item">
                  <span class="stat-value">{{ user.stats.total_trades }}</span>
                  <span class="stat-label">{{ t('userDetail.totalTrades') }}</span>
                </div>
                <div class="stat-item">
                  <span :class="['stat-value', user.stats.total_pnl >= 0 ? 'pnl-positive' : 'pnl-negative']">
                    {{ user.stats.total_pnl >= 0 ? '+' : '' }}{{ user.stats.total_pnl.toFixed(2) }}
                  </span>
                  <span class="stat-label">{{ t('userDetail.totalPnL') }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- KYC Tab -->
        <div v-if="activeTab === 'kyc'" class="tab-panel">
          <div class="info-card">
            <h3 class="card-title">{{ t('userDetail.kycStatus') }}</h3>
            <div class="kyc-status-container">
              <span :class="['kyc-status-badge', getKycStatusClass(user.kyc.status)]">
                {{ t(`userDetail.kycStatusValues.${user.kyc.status}`) }}
              </span>
            </div>
            <div class="info-rows">
              <div v-if="user.kyc.submitted_at" class="info-row">
                <span class="info-label">{{ t('userDetail.kycSubmittedAt') }}</span>
                <span class="info-value">{{ formatDateTime(user.kyc.submitted_at) }}</span>
              </div>
              <div v-if="user.kyc.reviewed_at" class="info-row">
                <span class="info-label">{{ t('userDetail.kycReviewedAt') }}</span>
                <span class="info-value">{{ formatDateTime(user.kyc.reviewed_at) }}</span>
              </div>
            </div>
            <div v-if="user.kyc.status === 'pending'" class="kyc-actions">
              <router-link :to="`/admin/kyc-review?user=${user.user.id}`" class="btn btn-primary">
                {{ t('userDetail.reviewKyc') }}
              </router-link>
            </div>
          </div>
        </div>

        <!-- Wallet Tab -->
        <div v-if="activeTab === 'wallet'" class="tab-panel">
          <div class="wallet-header">
            <div class="balance-card">
              <span class="balance-label">{{ t('userDetail.currentBalance') }}</span>
              <span class="balance-value">{{ formatCurrency(user.wallet.balance_cents, user.wallet.currency) }}</span>
              <span :class="['wallet-status', `status-${user.wallet.status}`]">
                {{ t(`userDetail.walletStatus.${user.wallet.status}`) }}
              </span>
            </div>
            <button
              v-if="canChargeWallet"
              class="btn btn-primary"
              @click="openChargeModal"
            >
              {{ t('userDetail.chargeWallet') }}
            </button>
          </div>

          <!-- Wallet History Section -->
          <div class="transactions-section">
            <h3 class="section-title">{{ t('userDetail.walletHistory') }}</h3>

            <!-- Filters -->
            <div class="wallet-filters">
              <button
                v-for="opt in walletFilterOptions"
                :key="opt.value"
                :class="['filter-chip', { active: walletHistoryFilter === opt.value }]"
                @click="walletHistoryFilter = opt.value; fetchWalletHistory(1, opt.value)"
              >
                {{ opt.label }}
              </button>
            </div>

            <!-- Loading -->
            <div v-if="walletHistoryLoading" class="loading-state">
              <div class="spinner"></div>
            </div>

            <!-- Empty -->
            <div v-else-if="!walletHistory || walletHistory.entries.length === 0" class="no-data">
              {{ t('userDetail.noTransactions') }}
            </div>

            <!-- Table -->
            <table v-else class="data-table">
              <thead>
                <tr>
                  <th>{{ t('userDetail.txDate') }}</th>
                  <th>{{ t('userDetail.txType') }}</th>
                  <th>{{ t('userDetail.txDescription') }}</th>
                  <th>{{ t('userDetail.txAmount') }}</th>
                  <th>{{ t('userDetail.txBalance') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="entry in walletHistory.entries" :key="entry.id">
                  <td>{{ formatDateTime(entry.created_at) }}</td>
                  <td>
                    <span :class="['tx-type-badge', `type-${entry.type}`]">
                      {{ t(`userDetail.txTypes.${entry.type}`) || entry.type }}
                    </span>
                  </td>
                  <td class="tx-description">{{ getWalletEntryLabel(entry) }}</td>
                  <td :class="entry.amount_cents > 0 ? 'amount-positive' : 'amount-negative'">
                    {{ formatCents(entry.amount_cents, walletHistory?.currency) }}
                  </td>
                  <td>{{ formatCents(entry.balance_after_cents, walletHistory?.currency) }}</td>
                </tr>
              </tbody>
            </table>

            <!-- Pagination -->
            <div v-if="walletHistory && walletHistory.total > walletPerPage" class="pagination">
              <button
                class="pagination-btn"
                :disabled="walletHistoryPage <= 1"
                @click="fetchWalletHistory(walletHistoryPage - 1, walletHistoryFilter)"
              >
                &larr;
              </button>
              <span class="pagination-info">
                {{ t('userDetail.walletHistory') }} {{ walletHistoryPage }} / {{ walletTotalPages }}
                ({{ walletHistory.total }})
              </span>
              <button
                class="pagination-btn"
                :disabled="!walletHistory.has_more"
                @click="fetchWalletHistory(walletHistoryPage + 1, walletHistoryFilter)"
              >
                &rarr;
              </button>
            </div>
          </div>
        </div>

        <!-- Contests Tab -->
        <div v-if="activeTab === 'contests'" class="tab-panel">
          <h3 class="section-title">{{ t('userDetail.contestHistory') }}</h3>
          <div v-if="user.recent_contests.length === 0" class="no-data">
            {{ t('userDetail.noContests') }}
          </div>
          <table v-else class="data-table">
            <thead>
              <tr>
                <th>{{ t('userDetail.contestName') }}</th>
                <th>{{ t('userDetail.contestDate') }}</th>
                <th>{{ t('userDetail.contestRank') }}</th>
                <th>{{ t('userDetail.contestPnL') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="contest in user.recent_contests" :key="contest.id">
                <td>
                  <router-link :to="`/admin/contests/${contest.id}`" class="link">
                    {{ contest.name }}
                  </router-link>
                </td>
                <td>{{ formatDate(contest.date) }}</td>
                <td>{{ contest.rank ? `#${contest.rank}` : '-' }}</td>
                <td :class="contest.pnl >= 0 ? 'pnl-positive' : 'pnl-negative'">
                  {{ contest.pnl >= 0 ? '+' : '' }}{{ contest.pnl.toFixed(2) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Affiliate Tab -->
        <div v-if="activeTab === 'affiliate'" class="tab-panel">
          <div class="info-card">
            <h3 class="card-title">{{ t('userDetail.affiliateInfo') }}</h3>
            <div class="affiliate-status">
              <span :class="['affiliate-badge', `status-${user.affiliate.status}`]">
                {{ t(`userDetail.affiliateStatus.${user.affiliate.status}`) }}
              </span>
            </div>
            <div class="info-rows">
              <div v-if="user.affiliate.code" class="info-row">
                <span class="info-label">{{ t('userDetail.affiliateCode') }}</span>
                <span class="info-value mono">{{ user.affiliate.code }}</span>
              </div>
              <div class="info-row">
                <span class="info-label">{{ t('userDetail.totalReferrals') }}</span>
                <span class="info-value">{{ user.affiliate.total_referrals }}</span>
              </div>
              <div class="info-row">
                <span class="info-label">{{ t('userDetail.totalEarned') }}</span>
                <span class="info-value">{{ formatCurrency(user.affiliate.total_earned) }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Sessions Tab -->
        <div v-if="activeTab === 'sessions'" class="tab-panel">
          <div class="sessions-header">
            <h3 class="section-title">{{ t('userDetail.activeSessions') }}</h3>
            <button
              v-if="canEditUsers && user.sessions.length > 0"
              class="btn btn-danger btn-sm"
              @click="showTerminateSessionsModal = true"
            >
              {{ t('userDetail.terminateAllSessions') }}
            </button>
          </div>
          <div v-if="user.sessions.length === 0" class="no-data">
            {{ t('userDetail.noSessions') }}
          </div>
          <table v-else class="data-table">
            <thead>
              <tr>
                <th>{{ t('userDetail.sessionId') }}</th>
                <th>{{ t('userDetail.sessionDevice') }}</th>
                <th>{{ t('userDetail.sessionIP') }}</th>
                <th>{{ t('userDetail.sessionLastActive') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="session in user.sessions" :key="session.id">
                <td class="mono">{{ session.id }}</td>
                <td>{{ session.device || '-' }}</td>
                <td class="mono">{{ session.ip || '-' }}</td>
                <td>{{ formatDateTime(session.last_active) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Audit Tab -->
        <div v-if="activeTab === 'audit'" class="tab-panel">
          <div class="audit-info">
            <p>{{ t('userDetail.auditDescription') }}</p>
            <router-link :to="`/admin/audit?target_id=${user.user.id}`" class="btn btn-primary">
              {{ t('userDetail.viewAuditLog') }}
            </router-link>
          </div>
        </div>
      </div>
    </div>

    <!-- Role Modal -->
    <Teleport to="body">
      <div v-if="showRoleModal" class="modal-overlay" @click.self="showRoleModal = false">
        <div class="modal">
          <div class="modal-header">
            <h3 class="modal-title">{{ t('users.editRolesTitle') }}</h3>
            <button class="modal-close" @click="showRoleModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <p class="modal-description">{{ t('users.editRolesDescription') }}</p>
            <div class="role-checkboxes">
              <label v-for="role in availableRoles" :key="role" class="role-checkbox">
                <input
                  type="checkbox"
                  :checked="selectedRoles.includes(role)"
                  @change="toggleRole(role)"
                />
                <span :class="['role-badge', 'role-large', getRoleBadgeClass(role)]">
                  {{ t(`users.role.${role}`) }}
                </span>
              </label>
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-ghost" @click="showRoleModal = false">
              {{ t('common.cancel') }}
            </button>
            <button class="btn btn-primary" :disabled="modalLoading || selectedRoles.length === 0" @click="saveRoles">
              {{ modalLoading ? t('common.loading') : t('common.save') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Ban Modal -->
    <Teleport to="body">
      <div v-if="showBanModal" class="modal-overlay" @click.self="showBanModal = false">
        <div class="modal">
          <div class="modal-header">
            <h3 class="modal-title">{{ t('userDetail.banTitle') }}</h3>
            <button class="modal-close" @click="showBanModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <p class="modal-description">{{ t('userDetail.banConfirmation', { email: user?.user.email ?? '' }) }}</p>
            <div class="form-group">
              <label class="form-label">{{ t('userDetail.banDuration') }}</label>
              <select v-model="banDuration" class="input">
                <option value="permanent">{{ t('userDetail.banDurations.permanent') }}</option>
                <option value="7d">{{ t('userDetail.banDurations.7d') }}</option>
                <option value="30d">{{ t('userDetail.banDurations.30d') }}</option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">{{ t('userDetail.banReason') }} *</label>
              <textarea
                v-model="banReason"
                class="input textarea"
                :placeholder="t('userDetail.banReasonPlaceholder')"
                rows="3"
              />
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-ghost" @click="showBanModal = false">
              {{ t('common.cancel') }}
            </button>
            <button
              class="btn btn-danger"
              :disabled="modalLoading || !banReason.trim()"
              @click="handleBanUser"
            >
              {{ modalLoading ? t('common.loading') : t('userDetail.ban') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Charge Wallet Modal -->
    <Teleport to="body">
      <div v-if="showChargeModal" class="modal-overlay" @click.self="showChargeModal = false">
        <div class="modal">
          <div class="modal-header">
            <h3 class="modal-title">{{ t('userDetail.chargeWalletTitle') }}</h3>
            <button class="modal-close" @click="showChargeModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <p class="modal-description">{{ t('userDetail.chargeWalletDescription') }}</p>
            <div class="form-group">
              <label class="form-label">{{ t('userDetail.chargeAmount') }} *</label>
              <div class="input-with-prefix">
                <span class="input-prefix">$</span>
                <input
                  v-model.number="chargeAmount"
                  type="number"
                  step="0.01"
                  class="input"
                  :class="{ 'input-error': chargeErrors.amount }"
                  :placeholder="t('userDetail.chargeAmountPlaceholder')"
                />
              </div>
              <p class="form-hint">{{ t('userDetail.chargeAmountHint') }}</p>
              <span v-if="chargeErrors.amount" class="field-error">{{ chargeErrors.amount }}</span>
            </div>
            <div class="form-group">
              <label class="form-label">{{ t('userDetail.chargeReason') }} *</label>
              <textarea
                v-model="chargeReason"
                class="input textarea"
                :class="{ 'input-error': chargeErrors.reason }"
                :placeholder="t('userDetail.chargeReasonPlaceholder')"
                rows="2"
              />
              <span v-if="chargeErrors.reason" class="field-error">{{ chargeErrors.reason }}</span>
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-ghost" @click="showChargeModal = false">
              {{ t('common.cancel') }}
            </button>
            <button
              class="btn btn-primary"
              :disabled="modalLoading"
              @click="handleChargeWallet"
            >
              {{ modalLoading ? t('common.loading') : t('userDetail.charge') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Terminate Sessions Modal -->
    <Teleport to="body">
      <div v-if="showTerminateSessionsModal" class="modal-overlay" @click.self="showTerminateSessionsModal = false">
        <div class="modal">
          <div class="modal-header">
            <h3 class="modal-title">{{ t('userDetail.terminateSessionsTitle') }}</h3>
            <button class="modal-close" @click="showTerminateSessionsModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <p class="modal-description">{{ t('userDetail.terminateSessionsConfirmation') }}</p>
          </div>
          <div class="modal-footer">
            <button class="btn btn-ghost" @click="showTerminateSessionsModal = false">
              {{ t('common.cancel') }}
            </button>
            <button
              class="btn btn-danger"
              :disabled="modalLoading"
              @click="handleTerminateSessions"
            >
              {{ modalLoading ? t('common.loading') : t('common.confirm') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.user-detail-page {
  padding: var(--spacing-lg) 0;
}

.page-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-xl);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

.loading-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 400px;
}

.loading {
  color: var(--color-text-secondary);
}

/* Profile Card */
.profile-card {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  padding: var(--spacing-xl);
  margin-bottom: var(--spacing-xl);
}

.profile-header {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-lg);
}

.avatar-container {
  flex-shrink: 0;
}

.avatar {
  width: 80px;
  height: 80px;
  border-radius: var(--radius-full);
  object-fit: cover;
}

.avatar-placeholder {
  width: 80px;
  height: 80px;
  border-radius: var(--radius-full);
  background-color: var(--color-primary);
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-2xl);
  font-weight: 600;
}

.profile-info {
  flex: 1;
}

.user-name {
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.user-email {
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-sm);
}

.badges {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}

.profile-actions {
  display: flex;
  gap: var(--spacing-sm);
}

/* Status & Role Badges */
.status-badge {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
}

.status-active {
  background-color: #DCFCE7;
  color: #16A34A;
}

.status-suspended {
  background-color: #FEE2E2;
  color: #DC2626;
}

.status-pending {
  background-color: #FEF3C7;
  color: #D97706;
}

.verified-badge {
  display: inline-flex;
  align-items: center;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 500;
  background-color: #DBEAFE;
  color: #2563EB;
}

.role-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
}

.role-large {
  padding: var(--spacing-xs) var(--spacing-sm);
}

.role-admin {
  background-color: #DBEAFE;
  color: #2563EB;
}

.role-moderator {
  background-color: #F3E8FF;
  color: #9333EA;
}

.role-user {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

.role-viewer {
  background-color: #FEF3C7;
  color: #D97706;
}

.role-super-admin {
  background-color: #FEE2E2;
  color: #DC2626;
}

/* Tabs */
.tabs {
  display: flex;
  gap: var(--spacing-xs);
  border-bottom: 1px solid var(--color-border);
  margin-bottom: var(--spacing-lg);
  overflow-x: auto;
}

.tab-button {
  padding: var(--spacing-sm) var(--spacing-md);
  border: none;
  background: none;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all var(--transition-fast);
  white-space: nowrap;
}

.tab-button:hover {
  color: var(--color-text-primary);
}

.tab-button.active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
}

/* Tab Content */
.tab-content {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
}

.tab-panel {
  padding: var(--spacing-lg);
}

/* Info Grid */
.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: var(--spacing-lg);
}

.info-card {
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  padding: var(--spacing-lg);
}

.card-title {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: var(--spacing-md);
}

.info-rows {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.info-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.info-value {
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  font-weight: 500;
}

.info-value.mono {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-xs);
}

/* Stats Grid */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(100px, 1fr));
  gap: var(--spacing-md);
}

.stat-item {
  text-align: center;
}

.stat-value {
  display: block;
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

.stat-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.pnl-positive {
  color: #16A34A;
}

.pnl-negative {
  color: #DC2626;
}

/* Wallet */
.wallet-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xl);
}

.balance-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.balance-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.balance-value {
  font-size: var(--font-size-3xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.wallet-status {
  font-size: var(--font-size-xs);
  text-transform: uppercase;
  font-weight: 600;
}

.wallet-status.status-active {
  color: #16A34A;
}

.wallet-status.status-frozen {
  color: #DC2626;
}

/* Section Title */
.section-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-md);
}

/* Data Table */
.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: var(--spacing-sm) var(--spacing-md);
  text-align: left;
  border-bottom: 1px solid var(--color-border);
}

.data-table th {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.data-table td {
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.data-table td.mono {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-xs);
}

.tx-positive {
  color: #16A34A;
}

.tx-negative {
  color: #DC2626;
}

.link {
  color: var(--color-primary);
  text-decoration: none;
}

.link:hover {
  text-decoration: underline;
}

.no-data {
  text-align: center;
  padding: var(--spacing-xl);
  color: var(--color-text-muted);
}

/* KYC */
.kyc-status-container {
  margin-bottom: var(--spacing-lg);
}

.kyc-status-badge {
  display: inline-block;
  padding: var(--spacing-sm) var(--spacing-lg);
  border-radius: var(--radius-md);
  font-size: var(--font-size-lg);
  font-weight: 600;
}

.kyc-verified {
  background-color: #DCFCE7;
  color: #16A34A;
}

.kyc-pending {
  background-color: #FEF3C7;
  color: #D97706;
}

.kyc-rejected {
  background-color: #FEE2E2;
  color: #DC2626;
}

.kyc-none {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-muted);
}

.kyc-actions {
  margin-top: var(--spacing-lg);
}

/* Affiliate */
.affiliate-status {
  margin-bottom: var(--spacing-lg);
}

.affiliate-badge {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.affiliate-badge.status-active {
  background-color: #DCFCE7;
  color: #16A34A;
}

.affiliate-badge.status-pending {
  background-color: #FEF3C7;
  color: #D97706;
}

.affiliate-badge.status-inactive,
.affiliate-badge.status-none {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-muted);
}

/* Sessions */
.sessions-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-md);
}

/* Audit */
.audit-info {
  text-align: center;
  padding: var(--spacing-xl);
}

.audit-info p {
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-lg);
}

/* Buttons */
.btn {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all var(--transition-fast);
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background-color: var(--color-primary);
  color: white;
}

.btn-primary:hover:not(:disabled) {
  background-color: var(--color-primary-dark);
}

.btn-ghost {
  background-color: transparent;
  color: var(--color-text-secondary);
}

.btn-ghost:hover:not(:disabled) {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

.btn-success {
  background-color: #16A34A;
  color: white;
}

.btn-success:hover:not(:disabled) {
  background-color: #15803D;
}

.btn-danger {
  background-color: #DC2626;
  color: white;
}

.btn-danger:hover:not(:disabled) {
  background-color: #B91C1C;
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
}

/* Modal Styles */
.modal-overlay {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 450px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: var(--shadow-lg);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.modal-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
}

.modal-close {
  background: none;
  border: none;
  font-size: 24px;
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 0;
  line-height: 1;
}

.modal-close:hover {
  color: var(--color-text-primary);
}

.modal-body {
  padding: var(--spacing-lg);
}

.modal-description {
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-lg);
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

/* Form Elements */
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

.form-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin-top: var(--spacing-xs);
}

.input-error {
  border-color: var(--color-error, #ef4444);
}

.field-error {
  font-size: var(--font-size-xs);
  color: var(--color-error, #ef4444);
  margin-top: var(--spacing-xs);
}

.input {
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
}

.input:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-light);
}

.textarea {
  resize: vertical;
  min-height: 80px;
}

.input-with-prefix {
  display: flex;
  align-items: center;
}

.input-prefix {
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-right: none;
  border-radius: var(--radius-md) 0 0 var(--radius-md);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.input-with-prefix .input {
  border-radius: 0 var(--radius-md) var(--radius-md) 0;
}

.role-checkboxes {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.role-checkbox {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  cursor: pointer;
}

.role-checkbox input {
  width: 18px;
  height: 18px;
  cursor: pointer;
}

/* Responsive */
@media (max-width: 767px) {
  .profile-header {
    flex-direction: column;
    text-align: center;
  }

  .profile-actions {
    width: 100%;
    justify-content: center;
  }

  .badges {
    justify-content: center;
  }

  .wallet-header {
    flex-direction: column;
    gap: var(--spacing-md);
    text-align: center;
  }

  .sessions-header {
    flex-direction: column;
    gap: var(--spacing-md);
  }

  .tabs {
    -webkit-overflow-scrolling: touch;
  }
}

/* Wallet History Enhancements */
.wallet-filters {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.filter-chip {
  padding: 6px 14px;
  border-radius: 20px;
  border: 1px solid var(--color-border, #e5e7eb);
  background: var(--color-bg-secondary, #f9fafb);
  color: var(--color-text-secondary, #6b7280);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: all 0.2s;
}

.filter-chip:hover {
  border-color: var(--color-primary, #6366f1);
  color: var(--color-primary, #6366f1);
}

.filter-chip.active {
  background: var(--color-primary, #6366f1);
  color: #fff;
  border-color: var(--color-primary, #6366f1);
}

.tx-description {
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.875rem;
  color: var(--color-text-secondary, #6b7280);
}

.tx-type-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 0.75rem;
  font-weight: 500;
}

.tx-type-badge.type-deposit { background: rgba(37, 99, 235, 0.1); color: #2563eb; }
.tx-type-badge.type-withdrawal { background: rgba(220, 38, 38, 0.1); color: #dc2626; }
.tx-type-badge.type-prize_credit { background: rgba(234, 179, 8, 0.1); color: #ca8a04; }
.tx-type-badge.type-contest_entry { background: rgba(217, 119, 6, 0.1); color: #d97706; }
.tx-type-badge.type-contest_refund { background: rgba(79, 70, 229, 0.1); color: #4f46e5; }
.tx-type-badge.type-adjustment { background: rgba(107, 114, 128, 0.1); color: #6b7280; }
.tx-type-badge.type-affiliate_commission { background: rgba(16, 185, 129, 0.1); color: #059669; }

.amount-positive { color: #059669; font-weight: 600; }
.amount-negative { color: #dc2626; font-weight: 600; }

.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 16px;
  margin-top: 16px;
  padding: 12px 0;
}

.pagination-btn {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-bg-secondary, #f9fafb);
  border: 1px solid var(--color-border, #e5e7eb);
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
}

.pagination-btn:hover:not(:disabled) {
  border-color: var(--color-primary, #6366f1);
  color: var(--color-primary, #6366f1);
}

.pagination-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.pagination-info {
  font-size: 0.8125rem;
  color: var(--color-text-secondary, #6b7280);
}

.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--color-border, #e5e7eb);
  border-top-color: var(--color-primary, #6366f1);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin: 24px auto;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
