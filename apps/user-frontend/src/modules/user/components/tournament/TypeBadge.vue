<script setup lang="ts">
import { computed } from 'vue';

export type DurationVariant = 'Weekly' | '30 minutes' | 'Hourly';
export type MarketVariant = 'Forex' | 'Crypto';

const props = defineProps<{
  duration: DurationVariant;
  market: MarketVariant;
  isHot?: boolean;
}>();

const badgeNumber = computed(() => {
  switch (props.duration) {
    case '30 minutes': return '30';
    case 'Weekly': return '1';
    case 'Hourly': return '1';
    default: return '';
  }
});

const badgeLetter = computed(() => {
  switch (props.duration) {
    case '30 minutes': return 'M';
    case 'Weekly': return 'W';
    case 'Hourly': return 'H';
    default: return '';
  }
});

const borderColor = computed(() => {
  if (props.market === 'Crypto') return '#f5a623';
  return '#3ecf8e';
});

const badgeClass = computed(() => {
  return {
    'type-badge': true,
    'type-badge--circle': props.duration === '30 minutes',
    'type-badge--square': props.duration !== '30 minutes',
    'type-badge--dotted': props.duration === 'Hourly',
    'type-badge--hot': props.isHot,
  };
});
</script>

<template>
  <div class="type-badge-wrapper">
    <div :class="badgeClass" :style="{ borderColor: borderColor }">
      <span class="type-badge__number">{{ badgeNumber }}</span>
      <span class="type-badge__letter">/{{ badgeLetter }}</span>
    </div>
    <span v-if="isHot" class="type-badge__hot-icon" title="Hot">&#128293;</span>
    <span v-else-if="duration === 'Hourly'" class="type-badge__dots">
      <span class="dot"></span>
      <span class="dot"></span>
    </span>
    <button class="type-badge__play" aria-label="Play">
      <svg width="10" height="12" viewBox="0 0 10 12" fill="none">
        <path d="M1 1L9 6L1 11V1Z" fill="currentColor" />
      </svg>
    </button>
  </div>
</template>

<style scoped>
.type-badge-wrapper {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.type-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 11px;
  line-height: 1;
  color: #e8eaed;
  border-width: 2px;
  border-style: solid;
  user-select: none;
}

.type-badge--circle {
  width: 36px;
  height: 36px;
  border-radius: 50%;
}

.type-badge--square {
  width: 36px;
  height: 36px;
  border-radius: 8px;
}

.type-badge--dotted {
  border-style: dotted;
}

.type-badge__number {
  font-size: 13px;
  font-weight: 700;
}

.type-badge__letter {
  font-size: 10px;
  font-weight: 600;
  opacity: 0.8;
}

.type-badge__hot-icon {
  font-size: 14px;
  line-height: 1;
}

.type-badge__dots {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.type-badge__dots .dot {
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background-color: #5a6a7a;
}

.type-badge__play {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  background: transparent;
  border: none;
  color: #5a6a7a;
  cursor: pointer;
  padding: 0;
  transition: color 150ms ease;
}

.type-badge__play:hover {
  color: #e8eaed;
}
</style>
