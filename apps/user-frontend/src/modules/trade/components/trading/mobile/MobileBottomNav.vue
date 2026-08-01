<template>
  <div class="tp-mbnav">
    <button
      v-for="tab in tabs"
      :key="tab.id"
      class="tp-mbnav-item"
      :class="{ active: activeTab === tab.id }"
      @click="emit('tabChange', tab.id)"
    >
      <component :is="tab.icon" />
      <span>{{ t(tab.label) }}</span>
      <span v-if="tab.badge && tab.badge > 0" class="tp-mbnav-badge">{{ tab.badge }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'
import { h } from 'vue'

export type MobileTab = 'chart' | 'orders' | 'leaderboard' | 'details'

const props = defineProps<{
  activeTab: MobileTab
  openPositionsCount?: number
}>()

const emit = defineEmits<{
  (e: 'tabChange', tab: MobileTab): void
}>()

const ChartIcon = h('svg', { viewBox: '0 0 24 24', fill: 'currentColor' }, [
  h('rect', { x: '3', y: '12', width: '4', height: '9', rx: '1' }),
  h('rect', { x: '10', y: '6', width: '4', height: '15', rx: '1' }),
  h('rect', { x: '17', y: '2', width: '4', height: '19', rx: '1' })
])

const OrdersIcon = h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': '2' }, [
  h('path', { d: 'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2' }),
  h('rect', { x: '9', y: '3', width: '6', height: '4', rx: '1' }),
  h('path', { d: 'M9 12h6M9 16h6' })
])

const LeaderboardIcon = h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': '2' }, [
  h('path', { d: 'M8 21h8M12 17v4M6 13l-4 4h20l-4-4' }),
  h('path', { d: 'M6 13V5a2 2 0 012-2h8a2 2 0 012 2v8' })
])

const DetailsIcon = h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': '2' }, [
  h('circle', { cx: '12', cy: '12', r: '10' }),
  h('path', { d: 'M12 16v-4M12 8h.01' })
])

const tabs = [
  { id: 'chart' as const, label: 'mobile.nav.chart', icon: ChartIcon },
  { id: 'orders' as const, label: 'mobile.nav.orders', icon: OrdersIcon, badge: props.openPositionsCount },
  { id: 'leaderboard' as const, label: 'mobile.nav.leaderboard', icon: LeaderboardIcon },
  { id: 'details' as const, label: 'mobile.nav.details', icon: DetailsIcon }
]
</script>

<style scoped>
.tp-mbnav {
  display: flex;
  background: var(--tp-bg);
  border-top: 1px solid var(--tp-bd);
  padding: 8px 0;
  padding-bottom: calc(8px + env(safe-area-inset-bottom, 0));
}

.tp-mbnav-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 8px 4px;
  border: none;
  background: none;
  color: var(--tp-t2);
  cursor: pointer;
  position: relative;
  transition: color 0.15s;
}

.tp-mbnav-item:active {
  opacity: 0.7;
}

.tp-mbnav-item.active {
  color: var(--tp-bl);
}

.tp-mbnav-item svg {
  width: 24px;
  height: 24px;
}

.tp-mbnav-item span {
  font-size: 10px;
  font-weight: 500;
}

.tp-mbnav-badge {
  position: absolute;
  top: 4px;
  right: calc(50% - 16px);
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  background: var(--tp-rd);
  color: #fff;
  font-size: 10px;
  font-weight: 600;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
