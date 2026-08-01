<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useI18nStore } from '@/stores/i18n';
import { useAuthStore } from '@/stores/auth';
import { userStatsApi, kycApi, profileApi, type UserStats, type ScoreHistoryEntry, type KYCStatusResponse, type UpdateProfileRequest } from '@/api';
import TraggePointBadge from '@/components/tragge/TraggePointBadge.vue';
import WalletSummaryCard from '@/components/profile/WalletSummaryCard.vue';
import AffiliateSummaryCard from '@/components/profile/AffiliateSummaryCard.vue';
import ChangePasswordModal from '@/components/profile/ChangePasswordModal.vue';
import AvatarPicker from '@/components/profile/AvatarPicker.vue';
import { useToast } from '@/composables/useToast';
import type { PredefinedAvatar } from '@/modules/user/api';

const router = useRouter();
const i18nStore = useI18nStore();
const authStore = useAuthStore();
const toast = useToast();

// User stats data
const stats = ref<UserStats | null>(null);
const kycStatus = ref<KYCStatusResponse | null>(null);
const scoreHistory = ref<ScoreHistoryEntry[]>([]);
const loading = ref(true);
const error = ref<string | null>(null);
const globalRank = ref<number | null>(null);

// Edit mode state
const isEditing = ref(false);
const isSaving = ref(false);
const isSelectingAvatar = ref(false);
const showAvatarPicker = ref(false);

// Edit form data
const editForm = ref<UpdateProfileRequest>({
  username: '',
  display_name: '',
  bio: '',
  country: '',
  phone: '',
});

// Compute user profile from auth store
const userEmail = computed(() => authStore.user?.email || '');
const username = computed(() => authStore.user?.username || '');
const displayName = computed(() => {
  if (authStore.user?.display_name) return authStore.user.display_name;
  if (authStore.user?.username) return authStore.user.username;
  if (!userEmail.value) return '';
  return userEmail.value.split('@')[0];
});
const avatarUrl = computed(() => authStore.user?.avatar_url || '');
const bio = computed(() => authStore.user?.bio || '');
const country = computed(() => authStore.user?.country || '');
const phone = computed(() => authStore.user?.phone || '');
const joinDate = computed(() => {
  if (!authStore.user?.created_at) return '';
  return new Date(authStore.user.created_at).toLocaleDateString(i18nStore.locale === 'fa' ? 'fa-IR' : 'en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  });
});

const avatarLetter = computed(() => {
  if (displayName.value) return displayName.value.charAt(0).toUpperCase();
  if (userEmail.value) return userEmail.value.charAt(0).toUpperCase();
  return '?';
});

// Get current avatar slug from avatar URL
// Supports both static paths (/avatars/shark.png) and S3 URLs (.../predefined-avatars/slug.ext or .../predefined-avatars/slug_timestamp.ext)
const currentAvatarId = computed(() => {
  const url = avatarUrl.value;
  if (!url) return undefined;
  // Static nginx path: /avatars/shark.png
  const staticMatch = url.match(/\/avatars\/(\w+)\.png$/);
  if (staticMatch) return staticMatch[1];
  // S3 path: predefined-avatars/slug.ext or predefined-avatars/slug_1234567890.ext
  const s3Match = url.match(/predefined-avatars\/([a-z0-9_-]+?)(?:_\d+)?\.\w+$/);
  if (s3Match) return s3Match[1];
  return undefined;
});

// Avatar background color is resolved by the AvatarPicker (loaded from API)
const currentAvatarBgColor = computed(() => {
  return 'var(--color-primary)';
});

const userBadge = computed(() => {
  if (!stats.value) return null;
  const score = stats.value.tragge_point;
  if (score >= 1000) return t('tragge.badge.diamond');
  if (score >= 500) return t('tragge.badge.gold');
  if (score >= 200) return t('tragge.badge.silver');
  if (score >= 50) return t('tragge.badge.bronze');
  return null;
});

const expandedSection = ref<string | null>(null);
const showChangePasswordModal = ref(false);

// Computed values from stats
const winRate = computed(() => stats.value?.win_rate.toFixed(1) || 0);
const totalContests = computed(() => stats.value?.total_contests || 0);
const totalWins = computed(() => stats.value?.total_wins || 0);
const totalTop3 = computed(() => stats.value?.total_top3 || 0);
const avgTradeDuration = computed(() => {
  if (!stats.value) return 'N/A';
  const seconds = stats.value.avg_trade_duration_seconds;
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  return `${Math.floor(seconds / 3600)}h`;
});
const bestMarket = computed(() => stats.value?.best_market || 'N/A');
const traggePoint = computed(() => stats.value?.tragge_point.toFixed(2) || 0);

// Recent tournaments from score history
const recentTournaments = computed(() => {
  return scoreHistory.value.slice(0, 5).map(entry => ({
    id: entry.contest_id,
    name: entry.contest_name,
    result: `${entry.rank}${getRankSuffix(entry.rank)}`,
    pnl: entry.pnl > 0 ? `+${entry.pnl.toFixed(2)}` : entry.pnl.toFixed(2),
  }));
});

// Chart data from score history
const chartData = computed(() => {
  return scoreHistory.value.slice().reverse().map((entry, index) => ({
    x: index,
    y: entry.score,
    label: entry.contest_name,
  }));
});

function getRankSuffix(rank: number): string {
  if (rank === 1) return 'st';
  if (rank === 2) return 'nd';
  if (rank === 3) return 'rd';
  return 'th';
}

// KYC status display
const kycStatusLabel = computed(() => {
  if (!kycStatus.value) return t('profile.verificationRequired');
  switch (kycStatus.value.status) {
    case 'verified': return t('profile.verificationComplete');
    case 'pending':
    case 'under_review': return t('profile.verificationPending');
    default: return t('profile.verificationRequired');
  }
});

const kycStatusClass = computed(() => {
  if (!kycStatus.value) return 'status-required';
  switch (kycStatus.value.status) {
    case 'verified': return 'status-verified';
    case 'pending':
    case 'under_review': return 'status-pending';
    default: return 'status-required';
  }
});

function goToVerification(): void {
  router.push({ name: 'verification' });
}

async function loadKYCStatus(): Promise<void> {
  try {
    kycStatus.value = await kycApi.getStatus();
  } catch {
    // Silently fail - KYC status is optional
  }
}

async function loadUserStats() {
  loading.value = true;
  error.value = null;

  try {
    const [statsData, historyData, leaderboardData] = await Promise.all([
      userStatsApi.getMyStats(),
      userStatsApi.getMyScoreHistory({ limit: 10 }),
      userStatsApi.getGlobalLeaderboard({ limit: 1 }),
    ]);
    stats.value = statsData;
    scoreHistory.value = historyData.entries;
    globalRank.value = leaderboardData.user_rank || null;
  } catch (err: any) {
    error.value = err.response?.data?.error || t('common.error');
  } finally {
    loading.value = false;
  }
}

function toggleSection(section: string): void {
  expandedSection.value = expandedSection.value === section ? null : section;
}

function toggleLanguage(): void {
  i18nStore.toggleLocale();
}

function startEditing(): void {
  // Initialize edit form with current values
  editForm.value = {
    username: authStore.user?.username || '',
    display_name: authStore.user?.display_name || '',
    bio: authStore.user?.bio || '',
    country: authStore.user?.country || '',
    phone: authStore.user?.phone || '',
  };
  isEditing.value = true;
}

function cancelEditing(): void {
  isEditing.value = false;
}

async function saveProfile(): Promise<void> {
  if (isSaving.value) return;

  isSaving.value = true;
  try {
    await profileApi.updateProfile(editForm.value);
    // Refresh user data
    await authStore.fetchUser();
    isEditing.value = false;
    toast.success(t('profile.profileUpdated'));
  } catch (err: any) {
    const message = err.response?.data?.error || err.response?.data?.details?.[0]?.message || t('common.error');
    toast.error(message);
  } finally {
    isSaving.value = false;
  }
}

function openAvatarPicker(): void {
  showAvatarPicker.value = true;
}

async function handleAvatarSelect(avatar: PredefinedAvatar): Promise<void> {
  isSelectingAvatar.value = true;
  try {
    await profileApi.selectAvatar(avatar.slug);
    // Refresh user data
    await authStore.fetchUser();
    toast.success(t('profile.avatarUpdated'));
  } catch (err: any) {
    const message = err.response?.data?.error || t('common.error');
    toast.error(message);
  } finally {
    isSelectingAvatar.value = false;
  }
}

// Country list for dropdown
const countries = [
  { code: 'US', name: 'United States' },
  { code: 'GB', name: 'United Kingdom' },
  { code: 'IR', name: 'Iran' },
  { code: 'AE', name: 'United Arab Emirates' },
  { code: 'DE', name: 'Germany' },
  { code: 'FR', name: 'France' },
  { code: 'CA', name: 'Canada' },
  { code: 'AU', name: 'Australia' },
  { code: 'JP', name: 'Japan' },
  { code: 'CN', name: 'China' },
  { code: 'IN', name: 'India' },
  { code: 'BR', name: 'Brazil' },
  { code: 'TR', name: 'Turkey' },
  { code: 'SA', name: 'Saudi Arabia' },
  { code: 'KR', name: 'South Korea' },
];

const countryName = computed(() => {
  const found = countries.find(c => c.code === country.value);
  return found?.name || country.value;
});

onMounted(() => {
  loadUserStats();
  loadKYCStatus();
});
</script>

<template>
  <div class="profile-page">
    <!-- Profile Header -->
    <div class="profile-header card">
      <!-- Loading state for header -->
      <template v-if="loading && !stats">
        <div class="avatar">
          <div class="avatar-skeleton"></div>
        </div>
        <div class="profile-info">
          <div class="skeleton-text skeleton-title"></div>
          <div class="skeleton-text skeleton-stats"></div>
        </div>
      </template>

      <!-- Edit Mode -->
      <template v-else-if="isEditing">
        <div class="avatar avatar-editable" :style="{ backgroundColor: currentAvatarBgColor }" @click="openAvatarPicker">
          <template v-if="avatarUrl">
            <img :src="avatarUrl" :alt="displayName" class="avatar-image" />
          </template>
          <span v-else class="avatar-placeholder">{{ avatarLetter }}</span>
          <div class="avatar-overlay">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="18" height="18" rx="2" ry="2"/>
              <circle cx="8.5" cy="8.5" r="1.5"/>
              <polyline points="21 15 16 10 5 21"/>
            </svg>
          </div>
          <div v-if="isSelectingAvatar" class="avatar-loading">
            <div class="spinner-sm"></div>
          </div>
        </div>

        <div class="edit-form">
          <div class="form-group">
            <label for="username">{{ t('profile.username') }}</label>
            <input
              id="username"
              v-model="editForm.username"
              type="text"
              :placeholder="t('profile.usernamePlaceholder')"
              class="form-input"
              maxlength="30"
            />
            <span class="form-hint">{{ t('profile.usernameHint') }}</span>
          </div>

          <div class="form-group">
            <label for="displayName">{{ t('profile.displayName') }}</label>
            <input
              id="displayName"
              v-model="editForm.display_name"
              type="text"
              :placeholder="t('profile.displayNamePlaceholder')"
              class="form-input"
              maxlength="100"
            />
          </div>

          <div class="form-group">
            <label for="bio">{{ t('profile.bio') }}</label>
            <textarea
              id="bio"
              v-model="editForm.bio"
              :placeholder="t('profile.bioPlaceholder')"
              class="form-input form-textarea"
              maxlength="500"
              rows="3"
            ></textarea>
            <span class="form-hint">{{ (editForm.bio?.length || 0) }}/500</span>
          </div>

          <div class="form-row">
            <div class="form-group">
              <label for="country">{{ t('profile.country') }}</label>
              <select id="country" v-model="editForm.country" class="form-input">
                <option value="">{{ t('profile.selectCountry') }}</option>
                <option v-for="c in countries" :key="c.code" :value="c.code">
                  {{ c.name }}
                </option>
              </select>
            </div>

            <div class="form-group">
              <label for="phone">{{ t('profile.phone') }}</label>
              <input
                id="phone"
                v-model="editForm.phone"
                type="tel"
                :placeholder="t('profile.phonePlaceholder')"
                class="form-input"
              />
            </div>
          </div>

          <div class="form-actions">
            <button class="btn btn-secondary" @click="cancelEditing" :disabled="isSaving">
              {{ t('common.cancel') }}
            </button>
            <button class="btn btn-primary" @click="saveProfile" :disabled="isSaving">
              <span v-if="isSaving" class="spinner-sm"></span>
              {{ isSaving ? t('common.saving') : t('common.save') }}
            </button>
          </div>
        </div>
      </template>

      <!-- View Mode -->
      <template v-else>
        <div class="avatar avatar-clickable" :style="{ backgroundColor: currentAvatarBgColor }" @click="openAvatarPicker">
          <template v-if="avatarUrl">
            <img :src="avatarUrl" :alt="displayName" class="avatar-image" />
          </template>
          <span v-else class="avatar-placeholder">{{ avatarLetter }}</span>
          <div class="avatar-edit-hint">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="18" height="18" rx="2" ry="2"/>
              <circle cx="8.5" cy="8.5" r="1.5"/>
              <polyline points="21 15 16 10 5 21"/>
            </svg>
          </div>
          <div v-if="isSelectingAvatar" class="avatar-loading">
            <div class="spinner-sm"></div>
          </div>
        </div>

        <div class="profile-info">
          <h2 class="username">{{ displayName || t('common.loading') }}</h2>
          <p v-if="username" class="user-handle">@{{ username }}</p>
          <p v-if="bio" class="user-bio">{{ bio }}</p>
          <div class="profile-meta">
            <span v-if="countryName" class="meta-item">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/>
                <line x1="2" y1="12" x2="22" y2="12"/>
                <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
              </svg>
              {{ countryName }}
            </span>
            <span v-if="joinDate" class="meta-item">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
                <line x1="16" y1="2" x2="16" y2="6"/>
                <line x1="8" y1="2" x2="8" y2="6"/>
                <line x1="3" y1="10" x2="21" y2="10"/>
              </svg>
              {{ t('profile.joinedOn') }} {{ joinDate }}
            </span>
          </div>
          <div class="profile-stats">
            <span class="stat">
              {{ t('profile.winRate') }}: <strong>{{ winRate }}%</strong>
            </span>
            <span v-if="globalRank" class="stat">
              {{ t('profile.rank') }}: <strong>#{{ globalRank.toLocaleString() }}</strong>
            </span>
            <span v-if="userBadge" class="badge-tag">{{ userBadge }}</span>
          </div>
        </div>
        <button class="btn btn-secondary edit-btn" @click="startEditing">
          {{ t('profile.editProfile') }}
        </button>
      </template>
    </div>

    <!-- Wallet Summary -->
    <WalletSummaryCard />

    <!-- Affiliate Summary -->
    <AffiliateSummaryCard />

    <!-- Content Grid -->
    <div class="content-grid">
      <!-- Settings -->
      <section class="settings-section card">
        <h3 class="section-title">{{ t('profile.settings') }}</h3>

        <!-- Account -->
        <div class="setting-item" @click="toggleSection('account')">
          <div class="setting-header">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="8" r="4" />
              <path d="M20 21a8 8 0 0 0-16 0" />
            </svg>
            <span>{{ t('profile.account') }}</span>
          </div>
          <svg :class="['chevron', { 'chevron-open': expandedSection === 'account' }]" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </div>
        <div v-if="expandedSection === 'account'" class="setting-content">
          <p><strong>{{ t('auth.email') }}:</strong> {{ userEmail || '-' }}</p>
          <p v-if="username"><strong>{{ t('profile.username') }}:</strong> @{{ username }}</p>
          <p v-if="phone"><strong>{{ t('profile.phone') }}:</strong> {{ phone }}</p>
          <p><strong>{{ t('profile.emailVerified') }}:</strong>
            <span :class="authStore.user?.email_verified ? 'text-success' : 'text-warning'">
              {{ authStore.user?.email_verified ? t('common.yes') : t('common.no') }}
            </span>
          </p>
        </div>

        <!-- Identity Verification -->
        <div class="setting-item" @click="goToVerification">
          <div class="setting-header">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
              <polyline v-if="kycStatus?.status === 'verified'" points="9 12 11 14 15 10" />
            </svg>
            <span>{{ t('profile.verifyIdentity') }}</span>
          </div>
          <div class="setting-right">
            <span :class="['kyc-status-badge', kycStatusClass]">{{ kycStatusLabel }}</span>
            <svg class="chevron" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="9 18 15 12 9 6" />
            </svg>
          </div>
        </div>

        <!-- Security -->
        <div class="setting-item" @click="toggleSection('security')">
          <div class="setting-header">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
              <path d="M7 11V7a5 5 0 0 1 10 0v4" />
            </svg>
            <span>{{ t('profile.security') }}</span>
          </div>
          <svg :class="['chevron', { 'chevron-open': expandedSection === 'security' }]" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </div>
        <div v-if="expandedSection === 'security'" class="setting-content">
          <div class="security-option">
            <div class="security-option-info">
              <span class="security-option-label">{{ t('profile.password') }}</span>
              <span class="security-option-desc">{{ t('profile.changePasswordDesc') }}</span>
            </div>
            <button class="btn btn-secondary btn-sm" @click="showChangePasswordModal = true">
              {{ t('profile.changePassword') }}
            </button>
          </div>
          <div class="security-option">
            <div class="security-option-info">
              <span class="security-option-label">{{ t('profile.twoFactorAuth') }}</span>
              <span class="security-option-desc">{{ t('profile.twoFactorDisabled') }}</span>
            </div>
            <button class="btn btn-secondary btn-sm">{{ t('profile.enable2FA') }}</button>
          </div>
        </div>

        <!-- Theme -->
        <div class="setting-item" @click="toggleSection('theme')">
          <div class="setting-header">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="5"/>
              <path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/>
            </svg>
            <span>{{ t('profile.theme') }}</span>
          </div>
          <svg :class="['chevron', { 'chevron-open': expandedSection === 'theme' }]" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </div>
        <div v-if="expandedSection === 'theme'" class="setting-content">
          <div class="theme-options">
            <label class="theme-option">
              <input type="radio" name="theme" value="light" />
              <span>{{ t('theme.light') }}</span>
            </label>
            <label class="theme-option">
              <input type="radio" name="theme" value="dark" />
              <span>{{ t('theme.dark') }}</span>
            </label>
            <label class="theme-option">
              <input type="radio" name="theme" value="system" checked />
              <span>{{ t('theme.system') }}</span>
            </label>
          </div>
        </div>

        <!-- Language -->
        <div class="setting-item" @click="toggleLanguage">
          <div class="setting-header">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="2" y1="12" x2="22" y2="12" />
              <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
            </svg>
            <span>{{ t('profile.language') }}</span>
          </div>
          <span class="setting-value">{{ i18nStore.locale === 'en' ? 'English' : 'فارسی' }}</span>
        </div>

        <!-- Notifications -->
        <div class="setting-item" @click="toggleSection('notifications')">
          <div class="setting-header">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
              <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
            </svg>
            <span>{{ t('profile.notifications') }}</span>
          </div>
          <svg :class="['chevron', { 'chevron-open': expandedSection === 'notifications' }]" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </div>
        <div v-if="expandedSection === 'notifications'" class="setting-content">
          <label class="toggle-label">
            <input type="checkbox" checked />
            <span>{{ t('profile.emailNotifications') }}</span>
          </label>
          <label class="toggle-label">
            <input type="checkbox" checked />
            <span>{{ t('profile.pushNotifications') }}</span>
          </label>
        </div>
      </section>

      <!-- Stats & Activity -->
      <section class="activity-section card">
        <h3 class="section-title">{{ t('profile.stats') }}</h3>

        <!-- Loading -->
        <div v-if="loading" class="loading-state">
          <div class="spinner"></div>
          <p>{{ t('common.loading') }}</p>
        </div>

        <!-- Error -->
        <div v-else-if="error" class="error-state">
          <p>{{ error }}</p>
          <button class="btn btn-primary" @click="loadUserStats">{{ t('common.retry') }}</button>
        </div>

        <!-- Stats -->
        <div v-else>
          <div class="stats-grid">
            <div class="stat-item">
              <span class="stat-value">{{ totalContests }}</span>
              <span class="stat-label">{{ t('profile.totalContests') }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-value">{{ totalWins }}</span>
              <span class="stat-label">{{ t('profile.totalWins') }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-value">{{ winRate }}%</span>
              <span class="stat-label">{{ t('profile.winRate') }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-value">{{ totalTop3 }}</span>
              <span class="stat-label">{{ t('profile.top3Finishes') }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-value">{{ avgTradeDuration }}</span>
              <span class="stat-label">{{ t('profile.avgTradeDuration') }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-value">{{ bestMarket }}</span>
              <span class="stat-label">{{ t('profile.bestMarket') }}</span>
            </div>
          </div>

          <!-- T-Point -->
          <RouterLink to="/user/leaderboard/global" class="tragge-score-section">
            <div class="tragge-score-header">
              <h4 class="subsection-title">{{ t('tragge.traggePoint') }}</h4>
              <TraggePointBadge v-if="stats" :score="stats.tragge_point" size="sm" :show-label="false" />
            </div>
            <div class="tragge-score-value">{{ traggePoint }}</div>
            <p class="tragge-score-description">
              {{ t('tragge.globalDescription') }}
            </p>
            <span class="tragge-view-link">
              {{ t('tragge.viewLeaderboard') }}
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M5 12h14M12 5l7 7-7 7"/>
              </svg>
            </span>
          </RouterLink>

          <!-- Performance Chart (Simple SVG chart) -->
          <div v-if="chartData.length > 0" class="chart-section">
            <h4 class="subsection-title">{{ t('profile.performanceHistory') }}</h4>
            <div class="simple-chart">
              <svg viewBox="0 0 400 200" class="chart-svg">
                <!-- Y-axis -->
                <line x1="30" y1="10" x2="30" y2="180" stroke="currentColor" stroke-width="1" opacity="0.3" />
                <!-- X-axis -->
                <line x1="30" y1="180" x2="390" y2="180" stroke="currentColor" stroke-width="1" opacity="0.3" />

                <!-- Data points and line -->
                <polyline
                  :points="chartData.map((d, i) => `${40 + i * (350 / Math.max(1, chartData.length - 1))},${180 - (d.y / Math.max(...chartData.map(p => p.y)) * 160)}`).join(' ')"
                  fill="none"
                  stroke="var(--color-primary)"
                  stroke-width="2"
                />

                <!-- Points -->
                <circle
                  v-for="(point, i) in chartData"
                  :key="i"
                  :cx="40 + i * (350 / Math.max(1, chartData.length - 1))"
                  :cy="180 - (point.y / Math.max(...chartData.map(p => p.y)) * 160)"
                  r="4"
                  fill="var(--color-primary)"
                />
              </svg>
            </div>
          </div>

          <!-- Recent Tournaments -->
          <div v-if="recentTournaments.length > 0" class="recent-section">
            <h4 class="subsection-title">{{ t('profile.recentTournaments') }}</h4>
            <div class="recent-list">
              <div v-for="tournament in recentTournaments" :key="tournament.id" class="recent-item">
                <span class="tournament-name">{{ tournament.name }}</span>
                <span class="tournament-result">{{ tournament.result }}</span>
                <span :class="['tournament-prize', { 'positive': parseFloat(tournament.pnl) > 0, 'negative': parseFloat(tournament.pnl) < 0 }]">
                  {{ tournament.pnl }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>

    <!-- Change Password Modal -->
    <ChangePasswordModal
      v-model:show="showChangePasswordModal"
      @success="() => {}"
    />

    <!-- Avatar Picker Modal -->
    <AvatarPicker
      v-model:show="showAvatarPicker"
      :current-avatar-id="currentAvatarId"
      @select="handleAvatarSelect"
    />
  </div>
</template>

<style scoped>
.profile-page {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

.profile-header {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-lg);
  flex-wrap: wrap;
}

.avatar {
  width: 80px;
  height: 80px;
  border-radius: var(--radius-full);
  background-color: var(--color-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  position: relative;
  overflow: hidden;
}

.avatar-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.avatar-placeholder {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  color: white;
}

.avatar-clickable {
  cursor: pointer;
  transition: transform var(--transition-fast);
}

.avatar-clickable:hover {
  transform: scale(1.05);
}

.avatar-edit-hint {
  position: absolute;
  bottom: 0;
  right: 0;
  background-color: var(--color-bg-primary);
  border: 2px solid var(--color-border);
  border-radius: var(--radius-full);
  padding: var(--spacing-xs);
  display: flex;
  align-items: center;
  justify-content: center;
}

.avatar-editable {
  cursor: pointer;
}

.avatar-overlay {
  position: absolute;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity var(--transition-fast);
  color: white;
}

.avatar-editable:hover .avatar-overlay {
  opacity: 1;
}

.avatar-loading {
  position: absolute;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
}

.spinner-sm {
  width: 20px;
  height: 20px;
  border: 2px solid var(--color-border);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

.hidden {
  display: none;
}

.profile-info {
  flex: 1;
  min-width: 200px;
}

.username {
  font-size: var(--font-size-xl);
  font-weight: 600;
  margin-bottom: var(--spacing-xs);
}

.user-handle {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-xs);
}

.user-bio {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-sm);
  line-height: 1.5;
}

.profile-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-sm);
}

.meta-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.profile-stats {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.stat {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.stat strong {
  color: var(--color-text-primary);
}

.badge-tag {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 600;
  background-color: #FEF3C7;
  color: #D97706;
  border-radius: var(--radius-full);
}

.edit-btn {
  margin-left: auto;
}

[dir="rtl"] .edit-btn {
  margin-left: 0;
  margin-right: auto;
}

/* Edit Form Styles */
.edit-form {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  min-width: 300px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.form-group label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.form-input {
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  transition: border-color var(--transition-fast);
}

.form-input:focus {
  outline: none;
  border-color: var(--color-primary);
}

.form-textarea {
  resize: vertical;
  min-height: 80px;
}

.form-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--spacing-md);
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-md);
  margin-top: var(--spacing-md);
}

.text-success {
  color: var(--color-success);
}

.text-warning {
  color: var(--color-warning);
}

.content-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--spacing-lg);
}

.section-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  margin-bottom: var(--spacing-md);
}

.setting-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) 0;
  border-bottom: 1px solid var(--color-border);
  cursor: pointer;
}

.setting-item:last-of-type {
  border-bottom: none;
}

.setting-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.setting-value {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.setting-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.kyc-status-badge {
  font-size: var(--font-size-xs);
  font-weight: 500;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
}

.kyc-status-badge.status-verified {
  background-color: #D1FAE5;
  color: #059669;
}

.kyc-status-badge.status-pending {
  background-color: #FEF3C7;
  color: #D97706;
}

.kyc-status-badge.status-required {
  background-color: #FEE2E2;
  color: #DC2626;
}

.chevron {
  transition: transform var(--transition-fast);
}

.chevron-open {
  transform: rotate(180deg);
}

.setting-content {
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  margin-bottom: var(--spacing-md);
  font-size: var(--font-size-sm);
}

.setting-content p {
  margin-bottom: var(--spacing-sm);
}

.security-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm) 0;
  gap: var(--spacing-md);
}

.security-option:not(:last-child) {
  border-bottom: 1px solid var(--color-border);
  margin-bottom: var(--spacing-sm);
  padding-bottom: var(--spacing-md);
}

.security-option-info {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.security-option-label {
  font-weight: 500;
  color: var(--color-text-primary);
}

.security-option-desc {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  white-space: nowrap;
}

.toggle-label {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-sm);
  cursor: pointer;
}

.theme-options {
  display: flex;
  gap: var(--spacing-lg);
}

.theme-option {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  cursor: pointer;
}

.loading-state,
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-xl);
  gap: var(--spacing-md);
}

.spinner {
  width: 40px;
  height: 40px;
  border: 4px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.stat-item {
  text-align: center;
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.stat-value {
  display: block;
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-primary);
}

.stat-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.subsection-title {
  font-size: var(--font-size-sm);
  font-weight: 600;
  margin-bottom: var(--spacing-sm);
  color: var(--color-text-secondary);
}

.recent-section {
  margin-bottom: var(--spacing-lg);
}

.recent-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.recent-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-sm);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
}

.tournament-name {
  flex: 1;
}

.tournament-result {
  color: var(--color-text-secondary);
}

.tournament-prize {
  font-weight: 500;
}

.tournament-prize.positive {
  color: var(--color-success);
}

.tournament-prize.negative {
  color: var(--color-error);
}

.tragge-score-section {
  display: block;
  margin-bottom: var(--spacing-lg);
  padding: var(--spacing-lg);
  background: linear-gradient(135deg, var(--color-primary) 0%, #7C3AED 100%);
  border-radius: var(--radius-md);
  text-align: center;
  color: white;
  text-decoration: none;
  cursor: pointer;
  transition: transform var(--transition-fast), box-shadow var(--transition-fast);
}

.tragge-score-section:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-lg);
}

.tragge-score-header {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
}

.tragge-score-header .subsection-title {
  color: white;
  margin: 0;
}

.tragge-score-value {
  font-size: var(--font-size-3xl);
  font-weight: 700;
  margin: var(--spacing-md) 0;
}

.tragge-score-description {
  font-size: var(--font-size-xs);
  opacity: 0.9;
  margin-bottom: var(--spacing-sm);
}

.tragge-view-link {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  font-weight: 500;
  opacity: 0.9;
}

.tragge-view-link svg {
  transition: transform var(--transition-fast);
}

.tragge-score-section:hover .tragge-view-link svg {
  transform: translateX(4px);
}

[dir="rtl"] .tragge-view-link svg {
  transform: rotate(180deg);
}

[dir="rtl"] .tragge-score-section:hover .tragge-view-link svg {
  transform: rotate(180deg) translateX(4px);
}

.chart-section {
  margin-bottom: var(--spacing-lg);
}

.simple-chart {
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.chart-svg {
  width: 100%;
  height: auto;
  color: var(--color-text-secondary);
}

@media (max-width: 1023px) {
  .content-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 767px) {
  .profile-header {
    flex-direction: column;
    text-align: center;
  }

  .profile-stats {
    justify-content: center;
  }

  .profile-meta {
    justify-content: center;
  }

  .edit-btn {
    margin-left: 0;
    width: 100%;
  }

  [dir="rtl"] .edit-btn {
    margin-right: 0;
  }

  .stats-grid {
    grid-template-columns: 1fr;
  }

  .form-row {
    grid-template-columns: 1fr;
  }

  .edit-form {
    min-width: 100%;
  }
}

/* Skeleton loading styles */
.avatar-skeleton {
  width: 100%;
  height: 100%;
  background: linear-gradient(90deg, var(--color-bg-tertiary) 25%, var(--color-bg-secondary) 50%, var(--color-bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: skeleton-loading 1.5s infinite;
  border-radius: var(--radius-full);
}

.skeleton-text {
  background: linear-gradient(90deg, var(--color-bg-tertiary) 25%, var(--color-bg-secondary) 50%, var(--color-bg-tertiary) 75%);
  background-size: 200% 100%;
  animation: skeleton-loading 1.5s infinite;
  border-radius: var(--radius-sm);
}

.skeleton-title {
  height: 24px;
  width: 150px;
  margin-bottom: var(--spacing-sm);
}

.skeleton-stats {
  height: 16px;
  width: 200px;
}

@keyframes skeleton-loading {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}
</style>
