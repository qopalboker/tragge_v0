<script setup lang="ts">
import { computed, toRef } from 'vue';
import { useCountdown } from '@/composables/useCountdown';

const props = defineProps<{
  targetDate: string;
  /** 'inline' for desktop table column, 'block' for mobile card corner */
  variant?: 'inline' | 'block';
}>();

const target = toRef(props, 'targetDate');

const { timeRemaining } = useCountdown({
  targetDate: computed(() => new Date(target.value)),
});

const display = computed(() => {
  const { days, hours, minutes, isExpired } = timeRemaining.value;
  if (isExpired) return '0d:00h:00m';
  return `${days}d:${String(hours).padStart(2, '0')}h:${String(minutes).padStart(2, '0')}m`;
});
</script>

<template>
  <span :class="['countdown', { 'countdown--block': variant === 'block' }]">
    <span class="countdown__value">{{ display }}</span>
  </span>
</template>

<style scoped>
.countdown {
  display: inline-flex;
  align-items: center;
}

.countdown__value {
  color: #00e5c3;
  font-family: 'JetBrains Mono', 'Fira Code', 'SF Mono', 'Menlo', monospace;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.02em;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.countdown--block {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
}

.countdown--block .countdown__value {
  font-size: 12px;
}
</style>
