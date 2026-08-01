<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';

const props = defineProps<{
  type: 'winRate' | 'rank';
  value: number;
  wins?: number;
  total?: number;
  totalPlayers?: number;
  change?: number;
}>();

const title = computed(() =>
  props.type === 'winRate' ? t('dashboard.winRate') : t('dashboard.rank')
);

const displayValue = computed(() => {
  if (props.type === 'winRate') return `${props.value}%`;
  return props.value > 0 ? `#${props.value.toLocaleString()}` : '-';
});

const subtitle = computed(() => {
  if (props.type === 'winRate' && props.wins !== undefined && props.total !== undefined) {
    return `(${props.wins}/${props.total})`;
  }
  if (props.type === 'rank' && props.totalPlayers !== undefined && props.value > 0) {
    return `/ ${props.totalPlayers.toLocaleString()}`;
  }
  return '';
});

const changeText = computed(() => {
  if (props.change === undefined) return null;
  const prefix = props.change > 0 ? '+' : '';
  return `${prefix}${props.change}`;
});

const isPositiveChange = computed(() => props.change !== undefined && props.change > 0);
</script>

<template>
  <div class="stat-card card">
    <div class="stat-header">
      <span class="stat-title">{{ title }}</span>
      <span v-if="changeText" :class="['stat-change', { 'positive': isPositiveChange }]">
        <svg v-if="isPositiveChange" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
          <polyline points="18 15 12 9 6 15" />
        </svg>
        {{ changeText }}
      </span>
    </div>
    <div class="stat-value-row">
      <span class="stat-value">{{ displayValue }}</span>
      <span class="stat-subtitle">{{ subtitle }}</span>
    </div>
  </div>
</template>

<style scoped>
.stat-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.stat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.stat-title {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.stat-change {
  display: flex;
  align-items: center;
  gap: 2px;
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-secondary);
}

.stat-change.positive {
  color: var(--color-success);
}

.stat-value-row {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-sm);
}

.stat-value {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
}

.stat-subtitle {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}
</style>
