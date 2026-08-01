<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import { t } from '@/i18n';

const props = defineProps<{
  startsAt: string;
  endsAt: string;
  status: 'registration_open' | 'scheduled' | 'running' | 'paused' | 'settling' | 'completed' | 'cancelled';
  compact?: boolean;
}>();

const emit = defineEmits<{
  statusChange: [newStatus: string];
}>();

interface TimeLeft {
  days: number;
  hours: number;
  minutes: number;
  seconds: number;
  total: number;
}

const timeLeft = ref<TimeLeft>({ days: 0, hours: 0, minutes: 0, seconds: 0, total: 0 });
let intervalId: ReturnType<typeof setInterval> | null = null;

const isLive = computed(() => props.status === 'running');
const isEnded = computed(() => props.status === 'completed' || props.status === 'cancelled');
const isPaused = computed(() => props.status === 'paused');
const isStartingSoon = computed(() => timeLeft.value.total > 0 && timeLeft.value.total < 5 * 60 * 1000); // 5 minutes

const targetDate = computed(() => {
  if (isLive.value || isPaused.value) {
    return new Date(props.endsAt);
  }
  return new Date(props.startsAt);
});

const statusBadge = computed(() => {
  if (isEnded.value) return { label: t('countdown.ended'), class: 'badge-ended' };
  if (isLive.value) return { label: t('countdown.live'), class: 'badge-live' };
  if (isPaused.value) return { label: t('countdown.paused'), class: 'badge-paused' };
  if (isStartingSoon.value) return { label: t('countdown.startingSoon'), class: 'badge-soon' };
  return null;
});

const countdownLabel = computed(() => {
  if (isLive.value || isPaused.value) return t('countdown.endsIn');
  return t('countdown.startsIn');
});

function calculateTimeLeft(): void {
  const now = new Date().getTime();
  const target = targetDate.value.getTime();
  const difference = target - now;

  if (difference <= 0) {
    timeLeft.value = { days: 0, hours: 0, minutes: 0, seconds: 0, total: 0 };

    // Emit status change when countdown reaches zero
    if (!isLive.value && !isEnded.value && props.status !== 'running') {
      emit('statusChange', 'running');
    } else if (isLive.value && difference < -1000) {
      emit('statusChange', 'completed');
    }
    return;
  }

  const days = Math.floor(difference / (1000 * 60 * 60 * 24));
  const hours = Math.floor((difference % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
  const minutes = Math.floor((difference % (1000 * 60 * 60)) / (1000 * 60));
  const seconds = Math.floor((difference % (1000 * 60)) / 1000);

  timeLeft.value = { days, hours, minutes, seconds, total: difference };
}

function formatNumber(n: number): string {
  return n.toString().padStart(2, '0');
}

function startTimer(): void {
  calculateTimeLeft();
  if (intervalId) {
    clearInterval(intervalId);
  }
  intervalId = setInterval(calculateTimeLeft, 1000);
}

function stopTimer(): void {
  if (intervalId) {
    clearInterval(intervalId);
    intervalId = null;
  }
}

watch(() => props.startsAt, startTimer);
watch(() => props.endsAt, startTimer);
watch(() => props.status, startTimer);

onMounted(() => {
  startTimer();
});

onUnmounted(() => {
  stopTimer();
});
</script>

<template>
  <div :class="['countdown-timer', { 'countdown-compact': compact }]">
    <!-- Status Badge -->
    <span v-if="statusBadge" :class="['status-badge', statusBadge.class]">
      <span v-if="isLive" class="pulse-dot"></span>
      {{ statusBadge.label }}
    </span>

    <!-- Countdown Display (when not ended) -->
    <template v-if="!isEnded && timeLeft.total > 0">
      <span v-if="!compact" class="countdown-label">{{ countdownLabel }}</span>
      <div class="countdown-display">
        <template v-if="timeLeft.days > 0">
          <div class="time-unit">
            <span class="time-value">{{ timeLeft.days }}</span>
            <span class="time-label">{{ t('countdown.days') }}</span>
          </div>
          <span class="time-separator">:</span>
        </template>
        <div class="time-unit">
          <span class="time-value">{{ formatNumber(timeLeft.hours) }}</span>
          <span class="time-label">{{ t('countdown.hours') }}</span>
        </div>
        <span class="time-separator">:</span>
        <div class="time-unit">
          <span class="time-value">{{ formatNumber(timeLeft.minutes) }}</span>
          <span class="time-label">{{ t('countdown.min') }}</span>
        </div>
        <template v-if="timeLeft.days === 0">
          <span class="time-separator">:</span>
          <div class="time-unit">
            <span class="time-value">{{ formatNumber(timeLeft.seconds) }}</span>
            <span class="time-label">{{ t('countdown.sec') }}</span>
          </div>
        </template>
      </div>
    </template>

    <!-- Just Started (countdown finished, waiting for status update) -->
    <span v-else-if="!isEnded && timeLeft.total <= 0" class="starting-now">
      {{ isLive ? t('countdown.endingNow') : t('countdown.startingNow') }}
    </span>
  </div>
</template>

<style scoped>
.countdown-timer {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.countdown-compact {
  gap: var(--spacing-xs);
}

.countdown-compact .countdown-display {
  gap: 2px;
}

.countdown-compact .time-label {
  display: none;
}

.countdown-compact .time-value {
  font-size: var(--font-size-sm);
}

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 600;
  border-radius: var(--radius-full);
  text-transform: uppercase;
  letter-spacing: 0.025em;
}

.badge-live {
  background-color: #FEE2E2;
  color: #DC2626;
  animation: pulseBackground 2s ease-in-out infinite;
}

.badge-soon {
  background-color: #FEF3C7;
  color: #D97706;
  animation: pulseBackground 1.5s ease-in-out infinite;
}

.badge-paused {
  background-color: #FEF3C7;
  color: #D97706;
}

.badge-ended {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

@keyframes pulseBackground {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.7;
  }
}

.pulse-dot {
  width: 6px;
  height: 6px;
  background-color: currentColor;
  border-radius: 50%;
  animation: pulse 1s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(1.2);
    opacity: 0.7;
  }
}

.countdown-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.countdown-display {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.time-unit {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 32px;
}

.time-value {
  font-size: var(--font-size-md);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-primary);
  line-height: 1;
}

.time-label {
  font-size: 9px;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.time-separator {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-muted);
  line-height: 1;
  margin-bottom: 12px;
}

.countdown-compact .time-separator {
  margin-bottom: 0;
}

.starting-now {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-success);
  animation: blink 1s step-end infinite;
}

@keyframes blink {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.5;
  }
}

/* RTL Support */
[dir="rtl"] .countdown-timer {
  flex-direction: row-reverse;
}

[dir="rtl"] .countdown-display {
  flex-direction: row-reverse;
}
</style>
