<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import BottomNav from './BottomNav.vue';
import VerificationFlow from '@/components/auth/VerificationFlow.vue';
import IconDashboard from '@/components/icons/IconDashboard.vue';
import IconTournaments from '@/components/icons/IconTournaments.vue';
import IconSupport from '@/components/icons/IconSupport.vue';
import IconProfile from '@/components/icons/IconProfile.vue';
import IconWallet from '@/components/icons/IconWallet.vue';
import IconMyTournaments from '@/components/icons/IconMyTournaments.vue';
import IconSettings from '@/components/icons/IconSettings.vue';
import { useThemeStore } from '@/stores/theme';
import { useI18nStore } from '@/stores/i18n';
import { ticketsApi } from '../../api/tickets';

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const themeStore = useThemeStore();
const i18nStore = useI18nStore();

function toggleLanguage() {
  i18nStore.toggleLocale();
}

// Responsive
const isMobile = ref(window.innerWidth < 768);
const isTablet = ref(window.innerWidth >= 768 && window.innerWidth < 1024);

function handleResize() {
  isMobile.value = window.innerWidth < 768;
  isTablet.value = window.innerWidth >= 768 && window.innerWidth < 1024;
}

onMounted(() => window.addEventListener('resize', handleResize));
onUnmounted(() => window.removeEventListener('resize', handleResize));

// Sidebar nav items
const navItems = [
  { name: 'dashboard', path: '/user/dashboard', icon: IconDashboard, label: () => t('nav.dashboard') },
  { name: 'contests', path: '/user/contests', icon: IconTournaments, label: () => t('nav.contests') },
  { name: 'my-tournaments', path: '/user/my-tournaments', icon: IconMyTournaments, label: () => t('nav.myTournaments') },
  { name: 'wallet', path: '/user/wallet', icon: IconWallet, label: () => t('nav.wallet') },
  { name: 'support', path: '/user/tickets', icon: IconSupport, label: () => t('nav.support') },
  { name: 'profile', path: '/user/profile', icon: IconProfile, label: () => t('nav.profile') },
  { name: 'settings', path: '/user/settings', icon: IconSettings, label: () => t('nav.settings') },
];

// Unread ticket badge
const unreadTicketCount = ref(0);
let badgeTimer: ReturnType<typeof setInterval> | null = null;

async function fetchBadge() {
  try {
    const res = await ticketsApi.getUnreadCount();
    unreadTicketCount.value = res.count;
  } catch { /* silent */ }
}

onMounted(() => {
  fetchBadge();
  badgeTimer = setInterval(fetchBadge, 30000);
});
onUnmounted(() => { if (badgeTimer) clearInterval(badgeTimer); });

function isActive(path: string): boolean {
  return route.path === path || route.path.startsWith(path + '/');
}

// Verification modal state
const showVerification = ref(false);
const verificationDismissed = ref(false);

const needsVerification = computed(() =>
  authStore.isAuthenticated && authStore.user
  && !authStore.user.email_verified
  && !authStore.user.phone_verified
);

const showUnverifiedBanner = computed(() =>
  needsVerification.value && !showVerification.value && verificationDismissed.value
);

function openVerificationModal() {
  const authResp = authStore.lastAuthResponse;
  showVerification.value = true;
}

const verificationMethods = computed(() => {
  const authResp = authStore.lastAuthResponse;
  if (authResp?.available_methods?.length) return authResp.available_methods;
  const methods: string[] = [];
  if (authStore.user?.email) methods.push('email');
  if (authStore.user?.phone) methods.push('sms');
  if (methods.length === 0) methods.push('email');
  return methods;
});

const maskedEmail = computed(() => {
  const authResp = authStore.lastAuthResponse;
  if (authResp?.masked_email) return authResp.masked_email;
  const email = authStore.user?.email;
  if (!email) return '';
  const parts = email.split('@');
  if (parts.length !== 2) return '***';
  const local = parts[0];
  return (local.length <= 1 ? local + '***' : local[0] + '***') + '@' + parts[1];
});

const maskedPhone = computed(() => {
  const authResp = authStore.lastAuthResponse;
  if (authResp?.masked_phone) return authResp.masked_phone;
  const phone = authStore.user?.phone;
  if (!phone) return '';
  if (phone.length <= 4) return '***';
  if (phone.length <= 7) return phone.slice(0, 2) + '***' + phone.slice(-2);
  return phone.slice(0, 4) + '***' + phone.slice(-2);
});

async function onVerified() {
  showVerification.value = false;
  verificationDismissed.value = false;
  sessionStorage.removeItem('verification_dismissed');
  await authStore.fetchUser();
}

function onVerificationClose() {
  showVerification.value = false;
  verificationDismissed.value = true;
  sessionStorage.setItem('verification_dismissed', '1');
}

// Show verification modal automatically for unverified users on first load,
// but only if user hasn't dismissed it already in this session.
onMounted(() => {
  const dismissed = sessionStorage.getItem('verification_dismissed');
  if (needsVerification.value && !dismissed) {
    showVerification.value = true;
  } else if (needsVerification.value && dismissed) {
    verificationDismissed.value = true;
  }
});

const userName = computed(() => {
  const user = authStore.user;
  if (user?.username) return user.username;
  if (user?.email) return user.email.split('@')[0];
  return 'User';
});

const userInitial = computed(() => userName.value.charAt(0).toUpperCase());

function handleLogout(): void {
  authStore.logout();
  router.push('/user/login');
}
</script>

<template>
  <div class="user-layout">
    <!-- Desktop/Tablet Sidebar -->
    <aside v-if="!isMobile" :class="['sidebar', { 'sidebar-collapsed': isTablet }]">
      <!-- Logo -->
      <div class="sidebar-logo">
        <div class="logo-icon">T</div>
        <template v-if="!isTablet">
          <div class="logo-info">
            <div class="logo-text">{{ t('app.name') }}</div>
            <div class="logo-sub">{{ t('app.tagline') }}</div>
          </div>
        </template>
      </div>

      <!-- Nav -->
      <nav class="sidebar-nav">
        <RouterLink
          v-for="item in navItems"
          :key="item.name"
          :to="item.path"
          :class="['nav-item', { 'nav-item-active': isActive(item.path) }]"
          :title="isTablet ? item.label() : undefined"
        >
          <div v-if="isActive(item.path)" class="nav-active-indicator" />
          <div class="nav-icon-wrapper">
            <component :is="item.icon" class="nav-icon" />
            <span
              v-if="item.name === 'support' && unreadTicketCount > 0"
              class="sidebar-badge"
            >{{ unreadTicketCount > 9 ? '9+' : unreadTicketCount }}</span>
          </div>
          <span v-if="!isTablet" class="nav-label">{{ item.label() }}</span>
        </RouterLink>
      </nav>

      <!-- Footer -->
      <div class="sidebar-footer">
        <!-- Language + Theme -->
        <div class="sidebar-actions">
          <button class="sidebar-action-btn" @click="toggleLanguage">
            {{ i18nStore.locale === 'en' ? 'EN' : 'FA' }}
          </button>
          <button class="sidebar-action-btn" @click="themeStore.toggleTheme">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="5" />
              <line x1="12" y1="1" x2="12" y2="3" /><line x1="12" y1="21" x2="12" y2="23" />
              <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" /><line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
              <line x1="1" y1="12" x2="3" y2="12" /><line x1="21" y1="12" x2="23" y2="12" />
              <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" /><line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
            </svg>
            <span v-if="!isTablet">{{ themeStore.currentTheme.name }}</span>
          </button>
        </div>

        <button class="nav-item logout-btn" @click="handleLogout">
          <svg class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4" />
            <polyline points="16 17 21 12 16 7" />
            <line x1="21" y1="12" x2="9" y2="12" />
          </svg>
          <span v-if="!isTablet" class="nav-label">{{ t('auth.logout') }}</span>
        </button>

        <div class="sidebar-user">
          <div class="user-avatar">{{ userInitial }}</div>
          <template v-if="!isTablet">
            <div class="user-info">
              <div class="user-name">{{ userName }}</div>
            </div>
          </template>
        </div>
      </div>
    </aside>

    <!-- Main content -->
    <div class="layout-main">
      <!-- Unverified account banner -->
      <div v-if="showUnverifiedBanner" class="unverified-banner">
        <span>{{ t('verification.unverifiedBanner') }}</span>
        <button class="banner-verify-btn" @click="openVerificationModal">
          {{ t('verification.verifyNow') }}
        </button>
      </div>
      <main class="layout-content">
        <RouterView />
      </main>
    </div>

    <!-- Verification modal -->
    <VerificationFlow
      v-if="showVerification && needsVerification"
      :available-methods="verificationMethods"
      :masked-phone="maskedPhone"
      :masked-email="maskedEmail"
      :user-name="authStore.user?.display_name"
      :user-email="authStore.user?.email"
      @verified="onVerified"
      @close="onVerificationClose"
    />

    <!-- Mobile Bottom Nav -->
    <BottomNav v-if="isMobile" class="layout-bottom-nav" />
  </div>
</template>

<style scoped>
.user-layout {
  display: flex;
  min-height: 100vh;
  min-height: 100dvh;
  background: var(--theme-bg);
}

/* Sidebar */
.sidebar {
  width: var(--sidebar-width, 240px);
  height: 100vh;
  position: fixed;
  top: 0;
  background: var(--theme-sidebar-bg, var(--theme-glass));
  backdrop-filter: blur(20px);
  border-inline-end: 1px solid var(--theme-glass-border);
  display: flex;
  flex-direction: column;
  padding: 24px 16px;
  z-index: 40;
}

.sidebar-collapsed {
  width: 72px;
  padding: 24px 8px;
}

.sidebar-logo {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 8px;
  margin-bottom: 36px;
}
.logo-icon {
  width: 42px; height: 42px; min-width: 42px;
  border-radius: 6px;
  background: linear-gradient(135deg, var(--theme-accent), var(--theme-green));
  display: flex; align-items: center; justify-content: center;
  font-size: 20px; font-weight: 900; color: #fff;
  box-shadow: 0 4px 16px var(--theme-accent-glow);
}
.logo-text { font-size: 20px; font-weight: 800; color: var(--theme-text); }
.logo-sub { font-size: 10px; color: var(--theme-text-secondary); font-weight: 500; letter-spacing: 1px; }

.sidebar-nav {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.nav-item {
  display: flex; align-items: center; gap: 14px;
  padding: 12px 16px; border-radius: 6px;
  color: var(--theme-text-secondary); text-decoration: none;
  transition: all 0.2s; cursor: pointer;
  border: none; background: none; width: 100%;
  font-size: 14px; font-weight: 500;
  position: relative; font-family: inherit; text-align: start;
}
.nav-item:hover { background: var(--theme-surface-hover); color: var(--theme-text); }
.nav-item-active {
  background: color-mix(in srgb, var(--theme-accent) 10%, transparent);
  color: var(--theme-accent); font-weight: 700;
}

.nav-active-indicator {
  position: absolute; inset-inline-start: 0; top: 50%;
  transform: translateY(-50%);
  width: 3px; height: 20px; border-radius: 2px;
  background: var(--theme-accent);
}

.nav-icon-wrapper { position: relative; display: flex; align-items: center; }
.nav-icon { width: 20px; height: 20px; flex-shrink: 0; }

.sidebar-badge {
  position: absolute; top: -6px; inset-inline-end: -10px;
  min-width: 16px; height: 16px; padding: 0 4px;
  border-radius: 8px;
  background: var(--theme-red, #ef4444); color: #fff;
  font-size: 10px; font-weight: 700; line-height: 16px; text-align: center;
}

.sidebar-footer {
  display: flex; flex-direction: column; gap: 8px; margin-top: 8px;
}

.sidebar-actions {
  display: flex;
  gap: 6px;
}
.sidebar-action-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 10px;
  border-radius: 6px;
  border: 1px solid var(--theme-glass-border);
  background: var(--theme-glass);
  color: var(--theme-text-secondary);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  font-family: inherit;
}
.sidebar-action-btn:hover {
  background: var(--theme-surface-hover);
  color: var(--theme-text);
}

.logout-btn {
  color: var(--theme-red, #ef4444);
  border: 1px solid color-mix(in srgb, var(--theme-red, #ef4444) 15%, transparent);
  background: color-mix(in srgb, var(--theme-red, #ef4444) 5%, transparent);
  border-radius: 6px;
}
.logout-btn:hover { background: color-mix(in srgb, var(--theme-red, #ef4444) 12%, transparent); }

.sidebar-user {
  display: flex; align-items: center; gap: 12px;
  padding: 14px 12px; border-radius: 6px;
  background: var(--theme-surface);
}
.user-avatar {
  width: 38px; height: 38px; min-width: 38px; border-radius: 6px;
  background: linear-gradient(135deg, var(--theme-accent), var(--theme-green));
  display: flex; align-items: center; justify-content: center;
  font-size: 16px; color: #fff; font-weight: 700;
}
.user-name {
  font-size: 13px; font-weight: 700; color: var(--theme-text);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}

.sidebar-collapsed .sidebar-logo { justify-content: center; padding: 0; }
.sidebar-collapsed .nav-item { justify-content: center; padding: 12px 8px; }
.sidebar-collapsed .sidebar-user { justify-content: center; padding: 10px; }

/* Main content */
.layout-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  margin-inline-start: var(--sidebar-width, 240px);
}

.layout-content {
  flex: 1;
  max-width: var(--max-content-width, 1200px);
  margin: 0 auto;
  padding: 32px;
  width: 100%;
  box-sizing: border-box;
}

.layout-bottom-nav {
  position: fixed; bottom: 0; left: 0; right: 0;
  z-index: 40;
}

/* Unverified account banner */
.unverified-banner {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 10px 16px;
  background: color-mix(in srgb, #f59e0b 12%, transparent);
  border-bottom: 1px solid color-mix(in srgb, #f59e0b 25%, transparent);
  color: #92400e;
  font-size: 13px;
  font-weight: 500;
}

.banner-verify-btn {
  background: #f59e0b;
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: 4px 14px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.2s;
  font-family: inherit;
}

.banner-verify-btn:hover {
  background: #d97706;
}

/* Tablet */
@media (min-width: 768px) and (max-width: 1023px) {
  .layout-main { margin-inline-start: 72px; }
}

/* Mobile */
@media (max-width: 767px) {
  .layout-main { margin-inline-start: 0; }
  .layout-content {
    padding: 12px 10px;
    padding-bottom: calc(var(--bottom-nav-height, 64px) + 24px);
  }
}
</style>
