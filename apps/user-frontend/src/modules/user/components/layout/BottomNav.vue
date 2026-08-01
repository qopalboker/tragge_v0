<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue';
import { useRoute } from 'vue-router';
import { t } from '@/i18n';
import IconDashboard from '@/components/icons/IconDashboard.vue';
import IconTournaments from '@/components/icons/IconTournaments.vue';
import IconSupport from '@/components/icons/IconSupport.vue';
import IconProfile from '@/components/icons/IconProfile.vue';
import { ticketsApi } from '../../api/tickets';

const route = useRoute();

interface NavItem {
  name: string;
  path: string;
  icon: typeof IconDashboard;
  label: string;
}

const navItems = computed<NavItem[]>(() => [
  { name: 'dashboard', path: '/user/dashboard', icon: IconDashboard, label: t('nav.dashboard').split(' ')[0] },
  { name: 'contests', path: '/user/contests', icon: IconTournaments, label: t('contests.title').split(' ')[0] },
  { name: 'support', path: '/user/tickets', icon: IconSupport, label: t('nav.support') },
  { name: 'profile', path: '/user/profile', icon: IconProfile, label: t('nav.profile') },
]);

function isActive(path: string): boolean {
  return route.path === path || route.path.startsWith(path + '/');
}

const unreadTicketCount = ref(0);
let ticketBadgeTimer: ReturnType<typeof setInterval> | null = null;

async function fetchUnreadTicketCount() {
  try {
    const res = await ticketsApi.getUnreadCount();
    unreadTicketCount.value = res.count;
  } catch {
    // Silent fail
  }
}

onMounted(() => {
  fetchUnreadTicketCount();
  ticketBadgeTimer = setInterval(fetchUnreadTicketCount, 30000);
});

onUnmounted(() => {
  if (ticketBadgeTimer) clearInterval(ticketBadgeTimer);
});
</script>

<template>
  <nav class="bottom-nav">
    <RouterLink
      v-for="item in navItems"
      :key="item.name"
      :to="item.path"
      :class="['bottom-nav-item', { 'bottom-nav-item-active': isActive(item.path) }]"
    >
      <div v-if="isActive(item.path)" class="active-dot" />
      <div class="bottom-nav-icon-wrapper">
        <component :is="item.icon" class="bottom-nav-icon" />
        <span
          v-if="item.name === 'support' && unreadTicketCount > 0"
          class="nav-badge"
        >{{ unreadTicketCount > 9 ? '9+' : unreadTicketCount }}</span>
      </div>
      <span class="bottom-nav-label">{{ item.label }}</span>
    </RouterLink>
  </nav>
</template>

<style scoped>
.bottom-nav {
  display: flex;
  align-items: center;
  justify-content: space-around;
  height: var(--bottom-nav-height);
  background: var(--theme-nav-bg);
  backdrop-filter: blur(24px) saturate(1.5);
  -webkit-backdrop-filter: blur(24px) saturate(1.5);
  border-top: 1px solid var(--theme-glass-border);
  padding: 0 var(--spacing-sm);
  padding-bottom: env(safe-area-inset-bottom, 0px);
}

.bottom-nav-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 1px;
  padding: 5px 10px;
  flex: 1;
  height: 100%;
  color: var(--theme-text-secondary);
  text-decoration: none;
  transition: color var(--transition-fast);
  position: relative;
  border: none;
  background: transparent;
  cursor: pointer;
}

.bottom-nav-item-active {
  color: var(--theme-accent);
}

.active-dot {
  position: absolute;
  top: -1px;
  width: 18px;
  height: 2px;
  border-radius: 1px;
  background: var(--theme-accent);
  box-shadow: 0 0 8px var(--theme-accent-glow);
}

.bottom-nav-icon-wrapper {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
}

.bottom-nav-icon {
  width: 18px;
  height: 18px;
}

.nav-badge {
  position: absolute;
  top: -6px;
  right: -8px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 8px;
  background: var(--theme-red, #ef4444);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  line-height: 16px;
  text-align: center;
}

[dir="rtl"] .nav-badge {
  right: auto;
  left: -8px;
}

.bottom-nav-label {
  font-size: 8px;
  font-weight: 500;
}

.bottom-nav-item-active .bottom-nav-label {
  font-weight: 700;
}
</style>
