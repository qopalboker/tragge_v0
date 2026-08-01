<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';

const props = defineProps<{
  totalPrizeCents: number;
  firstPrizeCents: number;
  /** Show info icon for mobile card */
  showInfo?: boolean;
  /** Compact mode for table cells */
  compact?: boolean;
}>();

const hasPrize = computed(() => props.totalPrizeCents > 0);

function formatCurrency(cents: number): string {
  const amount = cents / 100;
  return `$${amount.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

const firstPrizeDisplay = computed(() => {
  if (!hasPrize.value) return '';
  return `1st: ${formatCurrency(props.firstPrizeCents)}`;
});

const totalPrizeDisplay = computed(() => {
  if (!hasPrize.value) return '';
  return `Tot. ${formatCurrency(props.totalPrizeCents)}`;
});
</script>

<template>
  <div :class="['prize-badge', { 'prize-badge--compact': compact }]">
    <template v-if="hasPrize">
      <span class="prize-badge__first">{{ firstPrizeDisplay }}</span>
      <span class="prize-badge__total">{{ totalPrizeDisplay }}</span>
      <button v-if="showInfo" class="prize-badge__info" aria-label="Prize info">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="16" x2="12" y2="12" />
          <line x1="12" y1="8" x2="12.01" y2="8" />
        </svg>
      </button>
    </template>
    <template v-else>
      <span class="prize-badge__none">{{ t('tournament.noPrize') }}</span>
    </template>
  </div>
</template>

<style scoped>
.prize-badge {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.prize-badge--compact {
  gap: 1px;
}

.prize-badge__first {
  color: #f5a623;
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
}

.prize-badge__total {
  color: #5a6a7a;
  font-size: 12px;
  font-weight: 400;
  white-space: nowrap;
}

.prize-badge__none {
  color: #5a6a7a;
  font-size: 13px;
  font-weight: 400;
}

.prize-badge__info {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  background: transparent;
  border: none;
  color: #5a6a7a;
  cursor: pointer;
  padding: 0;
  align-self: flex-end;
  transition: color 150ms ease;
}

.prize-badge__info:hover {
  color: #e8eaed;
}
</style>
