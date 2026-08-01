<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch } from 'vue';
import { t } from '@/i18n';

const props = defineProps<{
  prizePoolCents: number;
  startsAt: string;
  endsAt: string;
  status: 'registration_open' | 'scheduled' | 'running' | 'paused' | 'settling' | 'completed' | 'cancelled';
  entryFeeCents: number;
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

const targetDate = computed(() => {
  if (isLive.value || isPaused.value) {
    return new Date(props.endsAt);
  }
  return new Date(props.startsAt);
});

const countdownLabel = computed(() => {
  if (isLive.value || isPaused.value) return t('countdown.endsIn');
  if (isEnded.value) return t('countdown.ended');
  return t('countdown.startsIn');
});

const formattedPrizePool = computed(() => {
  if (props.prizePoolCents === 0) {
    if (props.entryFeeCents === 0) {
      return t('contestDetails.noPrize');
    }
    return t('contestDetails.noPrize');
  }
  const amount = props.prizePoolCents / 100;
  if (amount >= 1000) {
    return `$${(amount / 1000).toFixed(1)}K`;
  }
  return `$${amount.toFixed(0)}`;
});

function calculateTimeLeft(): void {
  const now = new Date().getTime();
  const target = targetDate.value.getTime();
  const difference = target - now;

  if (difference <= 0) {
    timeLeft.value = { days: 0, hours: 0, minutes: 0, seconds: 0, total: 0 };
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
  <div class="contest-banner">
    <!-- Background decoration -->
    <div class="banner-decoration">
      <div class="decoration-shape decoration-1"></div>
      <div class="decoration-shape decoration-2"></div>
    </div>

    <div class="banner-content">
      <!-- Prize Pool Section -->
      <div class="prize-section">
        <span class="prize-label">{{ t('contestDetails.totalPrizePool') }}</span>
        <span class="prize-value">{{ formattedPrizePool }}</span>
      </div>

      <!-- Divider -->
      <div class="banner-divider"></div>

      <!-- Countdown Section -->
      <div class="countdown-section">
        <span class="countdown-label">{{ countdownLabel }}</span>
        <div v-if="!isEnded && timeLeft.total > 0" class="countdown-display">
          <div class="time-unit">
            <span class="time-value">{{ timeLeft.days }}</span>
            <span class="time-suffix">{{ t('contestDetails.daysShort') }}</span>
          </div>
          <span class="time-separator">:</span>
          <div class="time-unit">
            <span class="time-value">{{ formatNumber(timeLeft.hours) }}</span>
            <span class="time-suffix">{{ t('contestDetails.hoursShort') }}</span>
          </div>
          <span class="time-separator">:</span>
          <div class="time-unit">
            <span class="time-value">{{ formatNumber(timeLeft.minutes) }}</span>
            <span class="time-suffix">{{ t('contestDetails.minutesShort') }}</span>
          </div>
          <span class="time-separator">:</span>
          <div class="time-unit">
            <span class="time-value">{{ formatNumber(timeLeft.seconds) }}</span>
            <span class="time-suffix">{{ t('contestDetails.secondsShort') }}</span>
          </div>
        </div>
        <div v-else-if="isEnded" class="ended-message">
          {{ t('contestDetails.contestEnded') }}
        </div>
        <div v-else class="starting-now">
          {{ isLive ? t('countdown.endingNow') : t('countdown.startingNow') }}
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.contest-banner {
  position: relative;
  background: linear-gradient(135deg, #1e1b4b 0%, #312e81 50%, #4338ca 100%);
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
  overflow: hidden;
  color: white;
}

.banner-decoration {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  pointer-events: none;
  overflow: hidden;
}

.decoration-shape {
  position: absolute;
  border-radius: 50%;
  opacity: 0.15;
}

.decoration-1 {
  width: 200px;
  height: 200px;
  background: linear-gradient(135deg, #06b6d4 0%, #3b82f6 100%);
  top: -50px;
  right: -50px;
}

.decoration-2 {
  width: 150px;
  height: 150px;
  background: linear-gradient(135deg, #f472b6 0%, #c084fc 100%);
  bottom: -30px;
  left: 30%;
  transform: rotate(45deg);
  border-radius: 20%;
}

.banner-content {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: var(--spacing-lg);
}

.prize-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.prize-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: rgba(255, 255, 255, 0.8);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.prize-value {
  font-size: var(--font-size-3xl);
  font-weight: 700;
  color: white;
}

.banner-divider {
  width: 1px;
  height: 60px;
  background: rgba(255, 255, 255, 0.2);
}

.countdown-section {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--spacing-xs);
}

.countdown-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: rgba(255, 255, 255, 0.8);
  text-transform: uppercase;
  letter-spacing: 0.05em;
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
  min-width: 44px;
  background: rgba(0, 0, 0, 0.2);
  border-radius: var(--radius-md);
  padding: var(--spacing-xs) var(--spacing-sm);
}

.time-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1;
}

.time-suffix {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.7);
  text-transform: lowercase;
}

.time-separator {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: rgba(255, 255, 255, 0.5);
  margin-bottom: 14px;
}

.ended-message {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: rgba(255, 255, 255, 0.8);
}

.starting-now {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: #34d399;
  animation: pulse 1s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.6;
  }
}

/* RTL Support */
[dir="rtl"] .banner-content {
  flex-direction: row-reverse;
}

[dir="rtl"] .countdown-section {
  align-items: flex-start;
}

[dir="rtl"] .countdown-display {
  flex-direction: row-reverse;
}

/* Mobile */
@media (max-width: 767px) {
  .contest-banner {
    padding: var(--spacing-lg);
  }

  .banner-content {
    flex-direction: column;
    align-items: stretch;
    gap: var(--spacing-md);
  }

  .banner-divider {
    display: none;
  }

  .prize-section {
    text-align: center;
  }

  .prize-value {
    font-size: var(--font-size-2xl);
  }

  .countdown-section {
    align-items: center;
  }

  .time-unit {
    min-width: 38px;
    padding: var(--spacing-xs);
  }

  .time-value {
    font-size: var(--font-size-lg);
  }

  .time-separator {
    font-size: var(--font-size-lg);
    margin-bottom: 12px;
  }
}
</style>
