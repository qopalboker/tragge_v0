<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useThemeStore, type Theme } from '@/stores/theme';
import { useI18nStore } from '@/stores/i18n';
import { useToast } from '@/composables/useToast';
import { useNotificationPrefs } from '@/composables/useNotificationPrefs';
import { sessionsApi } from '@/api/index';

const router = useRouter();
const themeStore = useThemeStore();
const i18nStore = useI18nStore();
const toast = useToast();

// ==================== Theme Settings ====================
const themeOptions: { value: Theme; labelKey: string; icon: string }[] = [
  { value: 'light', labelKey: 'theme.light', icon: 'sun' },
  { value: 'dark', labelKey: 'theme.dark', icon: 'moon' },
  { value: 'system', labelKey: 'theme.system', icon: 'system' },
];

function selectTheme(theme: Theme): void {
  themeStore.setTheme(theme);
}

// ==================== Language Settings ====================
const languageOptions = [
  { value: 'en', label: 'English', nativeLabel: 'English' },
  { value: 'fa', label: 'فارسی', nativeLabel: 'Persian' },
];

function selectLanguage(lang: 'en' | 'fa'): void {
  i18nStore.setLocale(lang);
}

// ==================== Notification Settings ====================
const {
  preferences: notifPrefs,
  categories: notifCategories,
  channels: notifChannels,
  loading: notifLoading,
  initialized: notifInitialized,
  fetchPreferences,
  togglePreference,
  isEnabled: isNotifEnabled,
} = useNotificationPrefs();

function getCategoryLabel(cat: string): string {
  return t(`settings.notifCategory.${cat}`);
}

function getCategoryDesc(cat: string): string {
  return t(`settings.notifCategoryDesc.${cat}`);
}

function getChannelLabel(ch: string): string {
  return t(`settings.notifChannel.${ch}`);
}

// ==================== Session Management ====================
interface Session {
  id: string;
  device: string;
  browser: string;
  ip_address: string;
  last_active: string;
  is_current: boolean;
}

const sessions = ref<Session[]>([]);
const loadingSessions = ref(false);
const revokingAll = ref(false);

async function loadSessions(): Promise<void> {
  loadingSessions.value = true;
  try {
    sessions.value = await sessionsApi.getSessions();
  } catch {
    // Sessions API may not be implemented yet
    sessions.value = [];
  } finally {
    loadingSessions.value = false;
  }
}

async function revokeAllOtherSessions(): Promise<void> {
  revokingAll.value = true;
  try {
    await sessionsApi.revokeAllOtherSessions();
    toast.success(t('settings.sessionsRevoked'));
    await loadSessions();
  } catch {
    toast.error(t('settings.sessionsRevokeError'));
  } finally {
    revokingAll.value = false;
  }
}

const otherSessionsCount = computed(() => {
  return sessions.value?.filter(s => !s.is_current).length || 0;
});

// Format relative time for sessions
function formatLastActive(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return t('settings.justNow');
  if (diffMins < 60) return t('settings.minutesAgo', { count: diffMins });
  if (diffHours < 24) return t('settings.hoursAgo', { count: diffHours });
  return t('settings.daysAgo', { count: diffDays });
}

// ==================== Navigation ====================
function goToChangePassword(): void {
  router.push('/user/profile');
  // The change password modal is on the profile page
}

function goToKYC(): void {
  router.push('/user/profile/verify');
}

// ==================== Lifecycle ====================
onMounted(() => {
  loadSessions();
  fetchPreferences();
});
</script>

<template>
  <div class="settings-page">
    <header class="page-header">
      <h1>{{ t('settings.title') }}</h1>
      <p class="subtitle">{{ t('settings.subtitle') }}</p>
    </header>

    <div class="settings-content">
      <!-- Appearance Section -->
      <section class="settings-section">
        <h2 class="section-title">
          <svg class="section-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01"/>
          </svg>
          {{ t('settings.appearance') }}
        </h2>

        <!-- Theme Selector -->
        <div class="setting-item">
          <div class="setting-info">
            <span class="setting-label">{{ t('settings.theme') }}</span>
            <span class="setting-description">{{ t('settings.themeDescription') }}</span>
          </div>
          <div class="theme-options">
            <button
              v-for="option in themeOptions"
              :key="option.value"
              :class="['theme-option', { active: themeStore.theme === option.value }]"
              @click="selectTheme(option.value)"
            >
              <!-- Sun Icon -->
              <svg v-if="option.icon === 'sun'" class="option-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"/>
              </svg>
              <!-- Moon Icon -->
              <svg v-else-if="option.icon === 'moon'" class="option-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"/>
              </svg>
              <!-- System Icon -->
              <svg v-else class="option-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
              </svg>
              <span>{{ t(option.labelKey) }}</span>
            </button>
          </div>
        </div>

        <!-- Language Selector -->
        <div class="setting-item">
          <div class="setting-info">
            <span class="setting-label">{{ t('settings.language') }}</span>
            <span class="setting-description">{{ t('settings.languageDescription') }}</span>
          </div>
          <div class="language-options">
            <button
              v-for="option in languageOptions"
              :key="option.value"
              :class="['language-option', { active: i18nStore.locale === option.value }]"
              @click="selectLanguage(option.value as 'en' | 'fa')"
            >
              <span class="lang-label">{{ option.label }}</span>
            </button>
          </div>
        </div>
      </section>

      <!-- Notifications Section -->
      <section class="settings-section">
        <h2 class="section-title">
          <svg class="section-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"/>
          </svg>
          {{ t('settings.notifications') }}
        </h2>
        <p class="section-description">{{ t('settings.notificationsDesc') }}</p>

        <!-- Loading -->
        <div v-if="notifLoading && !notifInitialized" class="loading-state">
          <div class="spinner" />
        </div>

        <!-- Preferences Matrix -->
        <div v-else class="notif-prefs">
          <!-- Channel headers -->
          <div class="notif-header-row">
            <div class="notif-category-col"></div>
            <div v-for="ch in notifChannels" :key="ch" class="notif-channel-col">
              {{ getChannelLabel(ch) }}
            </div>
          </div>

          <!-- Category rows -->
          <div v-for="cat in notifCategories" :key="cat" class="notif-row">
            <div class="notif-category-col">
              <span class="notif-cat-label">{{ getCategoryLabel(cat) }}</span>
              <span class="notif-cat-desc">{{ getCategoryDesc(cat) }}</span>
            </div>
            <div v-for="ch in notifChannels" :key="ch" class="notif-channel-col">
              <button
                :class="['toggle-switch', { active: isNotifEnabled(cat, ch) }]"
                @click="togglePreference(cat, ch)"
                role="switch"
                :aria-checked="isNotifEnabled(cat, ch)"
              >
                <span class="toggle-slider" />
              </button>
            </div>
          </div>
        </div>
      </section>

      <!-- Security & Sessions Section -->
      <section class="settings-section">
        <h2 class="section-title">
          <svg class="section-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/>
          </svg>
          {{ t('settings.security') }}
        </h2>

        <!-- Active Sessions -->
        <div class="setting-item sessions-item">
          <div class="setting-info">
            <span class="setting-label">{{ t('settings.activeSessions') }}</span>
            <span class="setting-description">{{ t('settings.activeSessionsDesc') }}</span>
          </div>
        </div>

        <div class="sessions-list" v-if="!loadingSessions">
          <div v-if="sessions.length === 0" class="no-sessions">
            <p>{{ t('settings.noSessionsData') }}</p>
          </div>
          <div
            v-for="session in sessions"
            :key="session.id"
            :class="['session-item', { current: session.is_current }]"
          >
            <div class="session-icon">
              <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
              </svg>
            </div>
            <div class="session-details">
              <div class="session-device">
                {{ session.device || t('settings.unknownDevice') }}
                <span v-if="session.is_current" class="current-badge">{{ t('settings.currentSession') }}</span>
              </div>
              <div class="session-meta">
                <span>{{ session.browser || t('settings.unknownBrowser') }}</span>
                <span class="separator">•</span>
                <span>{{ session.ip_address }}</span>
                <span class="separator">•</span>
                <span>{{ formatLastActive(session.last_active) }}</span>
              </div>
            </div>
          </div>
        </div>

        <div v-else class="sessions-loading">
          <div class="spinner" />
          <span>{{ t('common.loading') }}</span>
        </div>

        <button
          v-if="otherSessionsCount > 0"
          class="revoke-btn"
          :disabled="revokingAll"
          @click="revokeAllOtherSessions"
        >
          <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"/>
          </svg>
          {{ revokingAll ? t('common.loading') : t('settings.logoutAllDevices') }}
        </button>
      </section>

      <!-- Account Section -->
      <section class="settings-section">
        <h2 class="section-title">
          <svg class="section-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/>
          </svg>
          {{ t('settings.account') }}
        </h2>

        <div class="account-links">
          <button class="account-link" @click="goToChangePassword">
            <div class="link-content">
              <svg class="link-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/>
              </svg>
              <div class="link-text">
                <span class="link-title">{{ t('settings.changePassword') }}</span>
                <span class="link-description">{{ t('settings.changePasswordDesc') }}</span>
              </div>
            </div>
            <svg class="chevron" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
            </svg>
          </button>

          <button class="account-link" @click="goToKYC">
            <div class="link-content">
              <svg class="link-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/>
              </svg>
              <div class="link-text">
                <span class="link-title">{{ t('settings.kycVerification') }}</span>
                <span class="link-description">{{ t('settings.kycVerificationDesc') }}</span>
              </div>
            </div>
            <svg class="chevron" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
            </svg>
          </button>

          <div class="account-link disabled">
            <div class="link-content">
              <svg class="link-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
              </svg>
              <div class="link-text">
                <span class="link-title">{{ t('settings.deleteAccount') }}</span>
                <span class="link-description">{{ t('settings.deleteAccountDesc') }}</span>
              </div>
            </div>
            <span class="contact-support">{{ t('settings.contactSupport') }}</span>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.settings-page {
  padding: var(--spacing-lg);
  max-width: 800px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: var(--spacing-xl);
}

.page-header h1 {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-xs);
}

.subtitle {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  margin: 0;
}

.settings-content {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xl);
}

.settings-section {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
}

.section-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-lg);
  padding-bottom: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
}

.section-icon {
  width: 20px;
  height: 20px;
  color: var(--color-primary);
}

.setting-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) 0;
  border-bottom: 1px solid var(--color-border);
}

.setting-item:last-child {
  border-bottom: none;
}

.setting-info {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  flex: 1;
  min-width: 0;
}

.setting-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.setting-description {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

/* Theme Options */
.theme-options {
  display: flex;
  gap: var(--spacing-sm);
}

.theme-option {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border: 2px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  min-width: 80px;
}

.theme-option:hover {
  border-color: var(--color-primary);
  color: var(--color-text-primary);
}

.theme-option.active {
  border-color: var(--color-primary);
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

.option-icon {
  width: 24px;
  height: 24px;
}

/* Language Options */
.language-options {
  display: flex;
  gap: var(--spacing-sm);
}

.language-option {
  padding: var(--spacing-sm) var(--spacing-lg);
  background-color: var(--color-bg-secondary);
  border: 2px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
}

.language-option:hover {
  border-color: var(--color-primary);
  color: var(--color-text-primary);
}

.language-option.active {
  border-color: var(--color-primary);
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

/* Toggle Switch */
.toggle-switch {
  position: relative;
  width: 48px;
  height: 26px;
  background-color: var(--color-bg-tertiary);
  border: none;
  border-radius: var(--radius-full);
  cursor: pointer;
  transition: background-color var(--transition-fast);
  flex-shrink: 0;
}

.toggle-switch.active {
  background-color: var(--color-primary);
}

.toggle-slider {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 20px;
  height: 20px;
  background-color: white;
  border-radius: var(--radius-full);
  transition: transform var(--transition-fast);
  box-shadow: var(--shadow-sm);
}

.toggle-switch.active .toggle-slider {
  transform: translateX(22px);
}

[dir="rtl"] .toggle-slider {
  left: auto;
  right: 3px;
}

[dir="rtl"] .toggle-switch.active .toggle-slider {
  transform: translateX(-22px);
}

/* Notification Preferences Matrix */
.section-description {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-md);
}

.notif-prefs {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.notif-header-row,
.notif-row {
  display: grid;
  grid-template-columns: 1fr repeat(2, 80px);
  align-items: center;
  gap: var(--spacing-sm);
}

.notif-header-row {
  padding-bottom: var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
}

.notif-channel-col {
  text-align: center;
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-secondary);
}

.notif-row {
  padding: var(--spacing-sm) 0;
  border-bottom: 1px solid var(--color-border);
}

.notif-row:last-child {
  border-bottom: none;
}

.notif-cat-label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.notif-cat-desc {
  display: block;
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
  margin-top: 2px;
}

.loading-state {
  display: flex;
  justify-content: center;
  padding: var(--spacing-lg);
}

.spinner {
  width: 24px;
  height: 24px;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Sessions */
.sessions-item {
  border-bottom: none;
  padding-bottom: 0;
}

.sessions-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-md);
}

.session-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border);
}

.session-item.current {
  border-color: var(--color-primary);
  background-color: var(--color-primary-light);
}

.session-icon {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-md);
  flex-shrink: 0;
}

.session-icon svg {
  width: 20px;
  height: 20px;
  color: var(--color-text-secondary);
}

.session-item.current .session-icon {
  background-color: var(--color-primary);
}

.session-item.current .session-icon svg {
  color: white;
}

.session-details {
  flex: 1;
  min-width: 0;
}

.session-device {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.current-badge {
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-primary);
  background-color: white;
  padding: 2px 8px;
  border-radius: var(--radius-full);
}

.session-meta {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
  margin-top: 2px;
}

.separator {
  color: var(--color-border);
}

.sessions-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-lg);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.no-sessions {
  padding: var(--spacing-lg);
  text-align: center;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.revoke-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  width: 100%;
  padding: var(--spacing-md);
  margin-top: var(--spacing-md);
  background-color: transparent;
  border: 1px solid var(--color-danger);
  border-radius: var(--radius-md);
  color: var(--color-danger);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.revoke-btn:hover:not(:disabled) {
  background-color: var(--color-danger);
  color: white;
}

.revoke-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.revoke-btn svg {
  width: 18px;
  height: 18px;
}

/* Account Links */
.account-links {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.account-link {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
  text-align: start;
  width: 100%;
}

.account-link:hover:not(.disabled) {
  border-color: var(--color-primary);
  background-color: var(--color-primary-light);
}

.account-link.disabled {
  cursor: default;
  opacity: 0.7;
}

.link-content {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.link-icon {
  width: 24px;
  height: 24px;
  color: var(--color-text-secondary);
  flex-shrink: 0;
}

.account-link:hover:not(.disabled) .link-icon {
  color: var(--color-primary);
}

.link-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.link-title {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.link-description {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.chevron {
  width: 20px;
  height: 20px;
  color: var(--color-text-secondary);
  flex-shrink: 0;
}

[dir="rtl"] .chevron {
  transform: rotate(180deg);
}

.contact-support {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  background-color: var(--color-bg-tertiary);
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-sm);
}

/* Responsive */
@media (max-width: 767px) {
  .settings-page {
    padding: var(--spacing-md);
  }

  .settings-section {
    padding: var(--spacing-md);
  }

  .setting-item {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--spacing-md);
  }

  .theme-options,
  .language-options {
    width: 100%;
    justify-content: flex-start;
  }

  .theme-option {
    flex: 1;
    min-width: 0;
  }

  .language-option {
    flex: 1;
    text-align: center;
  }

  .toggle-switch {
    align-self: flex-end;
  }

  .session-meta {
    flex-direction: column;
    gap: 0;
  }

  .separator {
    display: none;
  }

  .notif-header-row,
  .notif-row {
    grid-template-columns: 1fr repeat(2, 60px);
  }
}
</style>
