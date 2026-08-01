<script setup lang="ts">
import { computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';

const route = useRoute();
const router = useRouter();

const contestId = computed(() => route.params.contestId as string);

const isActive = (name: string) => {
  return route.name === name;
};

const navigateTo = (name: string) => {
  if (name === 'trading') {
    router.push({ name: 'trading', params: { contestId: contestId.value } });
  } else if (name === 'leaderboard') {
    router.push({ name: 'leaderboard', params: { contestId: contestId.value } });
  }
};
</script>

<template>
  <nav class="mobile-bottom-nav">
    <button
      :class="['nav-item', { active: isActive('trading') }]"
      @click="navigateTo('trading')"
    >
      <svg
        width="24"
        height="24"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
      </svg>
      <span>{{ t('nav.trading') }}</span>
    </button>

    <button
      :class="['nav-item', { active: isActive('leaderboard') }]"
      @click="navigateTo('leaderboard')"
    >
      <svg
        width="24"
        height="24"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
      </svg>
      <span>{{ t('nav.leaderboard') }}</span>
    </button>
  </nav>
</template>

<style scoped>
.mobile-bottom-nav {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 60px;
  background-color: var(--color-bg-secondary);
  border-top: 1px solid var(--color-border);
  display: none;
  z-index: var(--z-sticky);
}

.mobile-bottom-nav {
  display: flex;
  justify-content: space-around;
  align-items: center;
}

.nav-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  height: 100%;
  background: none;
  border: none;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all var(--transition-fast);
  padding: var(--spacing-xs);
}

.nav-item svg {
  width: 24px;
  height: 24px;
}

.nav-item span {
  font-size: var(--font-size-xs);
  font-weight: 500;
}

.nav-item:active {
  transform: scale(0.95);
}

.nav-item.active {
  color: var(--color-buy);
  background-color: rgba(16, 185, 129, 0.1);
}

.nav-item.active svg {
  stroke: var(--color-buy);
}

/* Show only on mobile */
@media (min-width: 769px) {
  .mobile-bottom-nav {
    display: none !important;
  }
}

/* RTL Support */
[dir="rtl"] .mobile-bottom-nav {
  direction: rtl;
}
</style>
