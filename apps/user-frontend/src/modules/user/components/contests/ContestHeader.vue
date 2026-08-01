<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';
import type { DurationType, MarketType } from '@/stores/contests';

const props = defineProps<{
  contestId: string;
  durationType?: DurationType;
  marketType?: MarketType;
  name?: string;
}>();

// Duration type icons
const durationTypeIcons: Record<DurationType, string> = {
  rush_30min: '\u26A1',
  hourly: '\u23F1\uFE0F',
  four_hour: '\uD83D\uDD53',
  daily: '\uD83D\uDCC5',
  weekly: '\uD83D\uDCC6',
};

// Market type icons
const marketTypeIcons: Record<MarketType, string> = {
  crypto: '\u20BF',
  forex: '\uD83D\uDCB1',
  stocks: '\uD83D\uDCC8',
  mixed: '\uD83C\uDFAF',
};

const durationIcon = computed(() => {
  if (!props.durationType) return '\u23F1\uFE0F';
  return durationTypeIcons[props.durationType] || '\u23F1\uFE0F';
});

const marketIcon = computed(() => {
  if (!props.marketType) return '';
  return marketTypeIcons[props.marketType] || '';
});

const durationLabel = computed(() => {
  if (!props.durationType) return '';
  return t(`filters.duration.${props.durationType}`);
});

const marketLabel = computed(() => {
  if (!props.marketType) return '';
  return t(`filters.market.${props.marketType}`);
});

// Format contest ID for display (show shortened version)
const displayId = computed(() => {
  if (props.contestId.length > 8) {
    return props.contestId.substring(0, 8).toUpperCase();
  }
  return props.contestId.toUpperCase();
});
</script>

<template>
  <div class="contest-header">
    <div class="header-left">
      <span class="duration-icon">{{ durationIcon }}</span>
      <div class="header-text">
        <span class="contest-id">ID{{ displayId }}</span>
        <span class="contest-subtitle">{{ t('contestDetails.tournamentInfo') }}</span>
      </div>
    </div>
    <div class="header-right">
      <span v-if="marketLabel" class="market-badge">
        <span class="market-icon">{{ marketIcon }}</span>
        <span class="market-text">{{ marketLabel }}</span>
      </span>
      <span v-if="durationLabel" class="duration-badge">
        {{ durationLabel }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.contest-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) var(--spacing-lg);
  background-color: var(--color-bg-primary);
  border-bottom: 1px solid var(--color-border);
}

.header-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.duration-icon {
  font-size: var(--font-size-xl);
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.header-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.contest-id {
  font-size: var(--font-size-lg);
  font-weight: 700;
  color: var(--color-text-primary);
  font-family: monospace;
}

.contest-subtitle {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.header-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.market-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
  background-color: var(--color-primary-light, #EEF2FF);
  color: var(--color-primary);
  border-radius: var(--radius-md);
}

.market-icon {
  font-size: var(--font-size-sm);
}

.market-text {
  line-height: 1.2;
}

.duration-badge {
  display: inline-flex;
  align-items: center;
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
  border-radius: var(--radius-md);
}

/* RTL Support */
[dir="rtl"] .header-left {
  flex-direction: row-reverse;
}

[dir="rtl"] .header-right {
  flex-direction: row-reverse;
}

/* Mobile */
@media (max-width: 767px) {
  .contest-header {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm) var(--spacing-md);
  }

  .header-right {
    width: 100%;
    justify-content: flex-start;
  }

  .contest-id {
    font-size: var(--font-size-md);
  }

  .duration-icon {
    width: 32px;
    height: 32px;
    font-size: var(--font-size-lg);
  }
}
</style>
