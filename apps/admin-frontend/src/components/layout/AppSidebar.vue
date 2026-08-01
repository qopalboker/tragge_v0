<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue';
import { useRoute } from 'vue-router';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { useI18nStore } from '@/stores/i18n';
import { useThemeStore } from '@/stores/theme';
import IconContests from '@/components/icons/IconContests.vue';
import IconContestTemplates from '@/components/icons/IconContestTemplates.vue';
import IconAudit from '@/components/icons/IconAudit.vue';
import IconShards from '@/components/icons/IconShards.vue';
import IconKYC from '@/components/icons/IconKYC.vue';
import IconSupport from '@/components/icons/IconSupport.vue';
import IconUsers from '@/components/icons/IconUsers.vue';
import IconWithdrawals from '@/components/icons/IconWithdrawals.vue';
import IconDashboard from '@/components/icons/IconDashboard.vue';
import IconFinancial from '@/components/icons/IconFinancial.vue';
import IconSymbols from '@/components/icons/IconSymbols.vue';
import IconEmailTemplates from '@/components/icons/IconEmailTemplates.vue';
import IconAutoScheduling from '@/components/icons/IconAutoScheduling.vue';
import IconMarketData from '@/components/icons/IconMarketData.vue';
import { getPendingWithdrawalsCount } from '@/api/withdrawals';
import { adminTicketsApi } from '../../api/tickets';

const route = useRoute();
const auth = useAuthStore();
const i18nStore = useI18nStore();
const themeStore = useThemeStore();
const pendingWithdrawalsCount = ref(0);
const pendingTicketCount = ref(0);
const contestsExpanded = ref(true);

function toggleLanguage(): void {
  i18nStore.toggleLocale();
}

interface NavItem {
  path: string;
  name: string;
  label: string;
  icon: typeof IconUsers;
  permission: string;
  badge?: () => number;
}

interface NavGroup {
  type: 'group';
  name: string;
  label: string;
  icon: typeof IconUsers;
  permission: string;
  expanded: boolean;
  children: NavItem[];
}

type NavEntry = NavItem | NavGroup;

function isGroup(entry: NavEntry): entry is NavGroup {
  return 'type' in entry && entry.type === 'group';
}

const allNavEntries: NavEntry[] = [
  {
    path: '/admin/dashboard',
    name: 'dashboard',
    label: t('nav.dashboard'),
    icon: IconDashboard,
    permission: 'users.view',
  },
  {
    type: 'group',
    name: 'contests-group',
    label: t('nav.contestsSection'),
    icon: IconContests,
    permission: 'contests.view',
    expanded: true,
    children: [
      {
        path: '/admin/contests',
        name: 'contests',
        label: t('nav.allContests'),
        icon: IconContests,
        permission: 'contests.view',
      },
      {
        path: '/admin/contest-templates',
        name: 'contest-templates',
        label: t('nav.contestTemplates'),
        icon: IconContestTemplates,
        permission: 'contests.view',
      },
      {
        path: '/admin/auto-scheduling',
        name: 'auto-scheduling',
        label: t('nav.autoScheduling'),
        icon: IconAutoScheduling,
        permission: 'contests.view',
      },
      {
        path: '/admin/tournament-templates',
        name: 'tournament-templates',
        label: t('nav.tournamentTemplates'),
        icon: IconContestTemplates,
        permission: 'contests.view',
      },
    ],
  },
  {
    path: '/admin/users',
    name: 'users',
    label: t('nav.users'),
    icon: IconUsers,
    permission: 'users.view',
  },
  {
    path: '/admin/financial',
    name: 'financial',
    label: t('nav.financial'),
    icon: IconFinancial,
    permission: 'financial.view',
  },
  {
    path: '/admin/kyc-review',
    name: 'kyc-review',
    label: t('nav.kycReview'),
    icon: IconKYC,
    permission: 'kyc.view',
  },
  {
    path: '/admin/tickets',
    name: 'tickets',
    label: t('nav.tickets'),
    icon: IconSupport,
    permission: 'withdrawals.view',
    badge: () => pendingTicketCount.value,
  },
  {
    path: '/admin/withdrawals',
    name: 'withdrawals',
    label: t('nav.withdrawals'),
    icon: IconWithdrawals,
    permission: 'withdrawals.view',
    badge: () => pendingWithdrawalsCount.value,
  },
  {
    path: '/admin/symbols',
    name: 'symbols',
    label: t('nav.symbols'),
    icon: IconSymbols,
    permission: 'symbols.view',
  },
  {
    path: '/admin/email-templates',
    name: 'email-templates',
    label: t('nav.emailTemplates'),
    icon: IconEmailTemplates,
    permission: 'settings.manage',
  },
  {
    path: '/admin/avatars',
    name: 'avatars',
    label: t('nav.avatars'),
    icon: IconUsers,
    permission: 'settings.manage',
  },
  {
    path: '/admin/market-data',
    name: 'market-data',
    label: t('nav.marketData'),
    icon: IconMarketData,
    permission: 'market.view',
  },
  {
    path: '/admin/shards',
    name: 'shards',
    label: t('nav.shards'),
    icon: IconShards,
    permission: 'shards.view',
  },
  {
    path: '/admin/audit',
    name: 'audit',
    label: t('nav.audit'),
    icon: IconAudit,
    permission: 'audit.view',
  },
];

// Fetch pending withdrawals + ticket counts in parallel. Both badges
// are gated by `withdrawals.view` (matches the nav-entry permission
// for /admin/tickets). Per-call .catch preserves the original
// "one failure shouldn't kill the other" behavior.
async function fetchPendingCount(): Promise<void> {
  if (!auth.hasPermission('withdrawals.view')) return;
  await Promise.all([
    getPendingWithdrawalsCount()
      .then((v) => { pendingWithdrawalsCount.value = v; })
      .catch(() => { /* badge stays at last known value */ }),
    adminTicketsApi.getStats()
      .then((s) => { pendingTicketCount.value = s.open + s.user_replied; })
      .catch(() => { /* badge stays at last known value */ }),
  ]);
}

let pendingCountIntervalId: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  fetchPendingCount();
  // Refresh count every 60 seconds
  pendingCountIntervalId = setInterval(fetchPendingCount, 60000);
});

onUnmounted(() => {
  if (pendingCountIntervalId !== null) {
    clearInterval(pendingCountIntervalId);
    pendingCountIntervalId = null;
  }
});

// Filter nav entries based on user permissions (spread to avoid mutating originals)
const navEntries = computed(() =>
  allNavEntries
    .filter(entry => auth.hasPermission(entry.permission))
    .map(entry =>
      isGroup(entry)
        ? { ...entry, children: entry.children.filter(child => auth.hasPermission(child.permission)) }
        : entry
    )
    .filter(entry => !isGroup(entry) || (entry as NavGroup).children.length > 0)
);

function isActive(path: string): boolean {
  return route.path.startsWith(path);
}

function isGroupActive(group: NavGroup): boolean {
  return group.children.some(child => isActive(child.path));
}

function toggleGroup(): void {
  contestsExpanded.value = !contestsExpanded.value;
}

// Display role badge
const roleBadge = computed(() => {
  if (auth.isSuperAdmin) return { label: t('rbac.superAdmin'), class: 'super-admin' };
  if (auth.isAdminRole) return { label: t('rbac.admin'), class: 'admin' };
  if (auth.isViewer) return { label: t('rbac.viewer'), class: 'viewer' };
  return null;
});
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-header">
      <RouterLink to="/admin" class="logo">
        <svg width="32" height="32" viewBox="0 0 32 32" fill="none">
          <rect width="32" height="32" rx="4" fill="var(--color-primary)" />
          <path d="M8 22L12 14L18 18L24 10" stroke="white" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        <span class="logo-text">مدیریت</span>
      </RouterLink>
      <div v-if="roleBadge" :class="['role-badge', roleBadge.class]">
        {{ roleBadge.label }}
      </div>
    </div>

    <nav class="sidebar-nav">
      <template v-for="entry in navEntries" :key="entry.name">
        <!-- Group with expandable children -->
        <template v-if="isGroup(entry)">
          <button
            :class="['nav-group-header', { active: isGroupActive(entry as NavGroup) }]"
            @click="toggleGroup"
          >
            <component :is="entry.icon" class="nav-icon" />
            <span class="nav-label">{{ entry.label }}</span>
            <svg
              :class="['nav-chevron', { expanded: contestsExpanded }]"
              width="16"
              height="16"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <polyline points="6 4 10 8 6 12" />
            </svg>
          </button>
          <div v-show="contestsExpanded" class="nav-group-children">
            <RouterLink
              v-for="child in (entry as NavGroup).children"
              :key="child.name"
              :to="child.path"
              :class="['nav-item nav-child', { active: isActive(child.path) }]"
            >
              <component :is="child.icon" class="nav-icon" />
              <span class="nav-label">{{ child.label }}</span>
            </RouterLink>
          </div>
        </template>

        <!-- Regular nav item -->
        <RouterLink
          v-else
          :to="(entry as NavItem).path"
          :class="['nav-item', { active: isActive((entry as NavItem).path) }]"
        >
          <component :is="entry.icon" class="nav-icon" />
          <span class="nav-label">{{ entry.label }}</span>
          <span v-if="(entry as NavItem).badge && (entry as NavItem).badge!() > 0" class="nav-badge">
            {{ (entry as NavItem).badge!() }}
          </span>
        </RouterLink>
      </template>
    </nav>

    <!-- View-only indicator for viewers -->
    <div v-if="auth.isViewer" class="viewer-notice">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
        <path d="M8 1a7 7 0 100 14A7 7 0 008 1zm0 12.5a5.5 5.5 0 110-11 5.5 5.5 0 010 11z"/>
        <path d="M8 4.5a.75.75 0 01.75.75v3a.75.75 0 01-1.5 0v-3A.75.75 0 018 4.5zm0 6a.75.75 0 110 1.5.75.75 0 010-1.5z"/>
      </svg>
      <span>{{ t('rbac.viewOnlyMode') }}</span>
    </div>

    <div class="sidebar-footer">
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
          <span>{{ themeStore.currentTheme.name }}</span>
        </button>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  width: var(--sidebar-width);
  height: 100%;
  background-color: var(--color-bg-primary);
  border-right: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
}

[dir="rtl"] .sidebar {
  border-right: none;
  border-left: 1px solid var(--color-border);
}

.sidebar-header {
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.logo {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  text-decoration: none;
}

.logo-text {
  font-size: var(--font-size-lg);
  font-weight: 700;
  color: var(--color-text-primary);
}

.role-badge {
  margin-top: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
}

.role-badge.super-admin {
  background-color: var(--color-error-light, #fee2e2);
  color: var(--color-error, #dc2626);
}

.role-badge.admin {
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

.role-badge.viewer {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

.sidebar-nav {
  flex: 1;
  padding: var(--spacing-md);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  overflow-y: auto;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  text-decoration: none;
  transition: all var(--transition-fast);
}

.nav-item:hover {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.nav-item.active {
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

.nav-group-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  background: none;
  border: none;
  cursor: pointer;
  width: 100%;
  text-align: start;
  font-family: inherit;
  transition: all var(--transition-fast);
}

.nav-group-header:hover {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.nav-group-header.active {
  color: var(--color-primary);
}

.nav-chevron {
  margin-inline-start: auto;
  flex-shrink: 0;
  transition: transform var(--transition-fast);
}

.nav-chevron.expanded {
  transform: rotate(90deg);
}

[dir="rtl"] .nav-chevron {
  transform: rotate(180deg);
}

[dir="rtl"] .nav-chevron.expanded {
  transform: rotate(90deg);
}

.nav-group-children {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.nav-child {
  padding-inline-start: calc(var(--spacing-md) + var(--spacing-lg));
}

.nav-icon {
  width: 20px;
  height: 20px;
  flex-shrink: 0;
}

.nav-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  flex: 1;
}

.nav-badge {
  background-color: var(--color-warning, #d97706);
  color: white;
  padding: 2px 6px;
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  min-width: 20px;
  text-align: center;
}

.viewer-notice {
  margin: var(--spacing-md);
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-warning-light, #fef3c7);
  color: var(--color-warning, #d97706);
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.sidebar-footer {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  border-top: 1px solid var(--color-border);
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
  gap: var(--spacing-sm);
  padding: var(--spacing-sm);
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border);
  background: var(--color-bg-secondary, transparent);
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
  font-family: inherit;
}

.sidebar-action-btn:hover {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}
</style>
