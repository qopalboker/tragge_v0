<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';
import TypeBadge from './TypeBadge.vue';
import CountdownTimer from './CountdownTimer.vue';

export interface TournamentCardProps {
  id: string;
  type: 'Forex' | 'Crypto';
  duration: 'Weekly' | '30 minutes' | 'Hourly';
  isHot?: boolean;
  startDate: string;
  endDate: string;
  traders: number;
  entryFeeCents: number;
  totalPrizeCents: number;
  firstPrizeCents: number;
  /** For "My Tournaments > Finished" */
  quantity?: number;
  pnlPercent?: number;
  position?: number;
  rewardCents?: number;
}

const props = defineProps<TournamentCardProps>();

const emit = defineEmits<{
  join: [id: string];
}>();

const isFree = computed(() => props.entryFeeCents === 0);
const isFinished = computed(() =>
  props.pnlPercent !== undefined || props.position !== undefined
);
const hasPrize = computed(() => props.totalPrizeCents > 0);

const tournamentIdDisplay = computed(() => `ID${props.id}`);

function formatCurrency(cents: number): string {
  const amount = cents / 100;
  return `$${amount.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

const entryFeeDisplay = computed(() => {
  if (isFree.value) return null;
  return formatCurrency(props.entryFeeCents);
});

// Date formatting helpers
function formatMonth(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleString('en-US', { month: 'short' }).toUpperCase();
}

function formatDay(dateStr: string): number {
  return new Date(dateStr).getDate();
}

function formatTime(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
}

// P&L styling
const pnlClass = computed(() => {
  if (props.pnlPercent === undefined) return '';
  if (props.pnlPercent > 0) return 'pnl--positive';
  if (props.pnlPercent < 0) return 'pnl--negative';
  return 'pnl--neutral';
});

const pnlDisplay = computed(() => {
  if (props.pnlPercent === undefined) return '';
  return `${props.pnlPercent > 0 ? '+' : ''}${props.pnlPercent}%`;
});

const rewardDisplay = computed(() => {
  if (props.rewardCents && props.rewardCents > 0) {
    return formatCurrency(props.rewardCents);
  }
  return t('tournament.noPrize');
});

const positionDisplay = computed(() => {
  if (!props.position) return '';
  const suffixes: Record<number, string> = { 1: 'st', 2: 'nd', 3: 'rd' };
  const suffix = suffixes[props.position] || 'th';
  return `${props.position}${suffix}`;
});

function handleJoin(): void {
  emit('join', props.id);
}
</script>

<template>
  <div class="t-card">
    <!-- Header -->
    <div class="t-card__header">
      <div class="t-card__title-group">
        <span class="t-card__type-name">{{ type }}</span>
        <span class="t-card__id">{{ tournamentIdDisplay }}</span>
      </div>
      <TypeBadge :duration="duration" :market="type" :is-hot="isHot" />
    </div>

    <!-- Date Section -->
    <div class="t-card__dates">
      <div class="t-card__date-pair">
        <!-- Start Date Block -->
        <div class="t-card__date-col">
          <div class="t-card__date-block">
            <span class="t-card__date-month">{{ formatMonth(startDate) }}</span>
            <span class="t-card__date-day">{{ formatDay(startDate) }}</span>
          </div>
          <div class="t-card__date-meta">
            <span class="t-card__date-label">{{ t('tournament.start') }}</span>
            <span class="t-card__date-time">{{ formatTime(startDate) }}</span>
          </div>
        </div>

        <span class="t-card__date-arrow">&rarr;</span>

        <!-- End Date Block -->
        <div class="t-card__date-col">
          <div class="t-card__date-block">
            <span class="t-card__date-month">{{ formatMonth(endDate) }}</span>
            <span class="t-card__date-day">{{ formatDay(endDate) }}</span>
          </div>
          <div class="t-card__date-meta">
            <span class="t-card__date-label">{{ t('tournament.end') }}</span>
            <span class="t-card__date-time">{{ formatTime(endDate) }}</span>
          </div>
        </div>
      </div>

      <!-- Countdown (top-right corner of date section) -->
      <div v-if="!isFinished" class="t-card__countdown">
        <span class="t-card__countdown-label">{{ t('tournament.startsIn') }}</span>
        <CountdownTimer :target-date="startDate" variant="block" />
      </div>
    </div>

    <!-- Prize Section (paid tournaments) -->
    <div v-if="hasPrize && !isFinished" class="t-card__prize-section">
      <div class="t-card__prize-row">
        <span class="t-card__trophy">&#127942;</span>
        <span class="t-card__prize-total">
          {{ t('tournament.totalPrize') }}: {{ formatCurrency(totalPrizeCents) }}
        </span>
      </div>
      <div class="t-card__prize-detail">
        <span class="t-card__first-prize">
          {{ t('tournament.firstPrize') }} {{ formatCurrency(firstPrizeCents) }}
        </span>
        <button class="t-card__info-btn" aria-label="Prize info">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="12" y1="16" x2="12" y2="12" />
            <line x1="12" y1="8" x2="12.01" y2="8" />
          </svg>
        </button>
      </div>
    </div>

    <!-- No Prize (free tournaments) -->
    <div v-else-if="!isFinished && !hasPrize" class="t-card__no-prize">
      <span class="t-card__no-prize-text">{{ t('tournament.noPrize') }}</span>
    </div>

    <!-- Divider -->
    <div class="t-card__divider"></div>

    <!-- Footer: Active/Upcoming Tournament -->
    <template v-if="!isFinished">
      <div class="t-card__footer">
        <div class="t-card__stat">
          <span class="t-card__stat-value">{{ traders || '-' }}</span>
          <span class="t-card__stat-label">{{ t('tournament.traders') }}</span>
        </div>
        <div v-if="isFree" class="t-card__stat">
          <span class="t-card__free-badge">{{ t('tournament.free') }}</span>
        </div>
        <div v-else class="t-card__stat">
          <span class="t-card__stat-value">{{ entryFeeDisplay }}</span>
          <span class="t-card__stat-label">{{ t('tournament.entryFee') }}</span>
        </div>
        <button class="t-card__join-btn" @click="handleJoin">
          {{ t('tournament.join') }}
        </button>
      </div>
    </template>

    <!-- Footer: Finished Tournament (My Tournaments) -->
    <template v-else>
      <div class="t-card__footer t-card__footer--finished">
        <div class="t-card__stat">
          <span class="t-card__stat-value">{{ traders }}</span>
          <span class="t-card__stat-label">{{ t('tournament.traders') }}</span>
        </div>
        <div class="t-card__stat">
          <span :class="['t-card__stat-value', pnlClass]">P&amp;L {{ pnlDisplay }}</span>
        </div>
        <div class="t-card__stat">
          <span class="t-card__stat-value t-card__reward-value">{{ rewardDisplay }}</span>
          <span class="t-card__stat-label">{{ t('tournament.totalPrizeLabel') }}</span>
        </div>
        <div class="t-card__stat">
          <span class="t-card__stat-value">{{ positionDisplay }}</span>
          <span class="t-card__stat-label">{{ t('tournament.position') }}</span>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.t-card {
  background-color: #0f1923;
  border: 1px solid #1a2538;
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* Header */
.t-card__header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.t-card__title-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.t-card__type-name {
  font-size: 16px;
  font-weight: 600;
  color: #e8eaed;
}

.t-card__id {
  font-size: 12px;
  color: #5a6a7a;
}

/* Date Section */
.t-card__dates {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.t-card__date-pair {
  display: flex;
  align-items: center;
  gap: 12px;
}

.t-card__date-col {
  display: flex;
  align-items: center;
  gap: 8px;
}

.t-card__date-block {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 48px;
  background-color: #1a2538;
  border-radius: 8px;
}

.t-card__date-month {
  font-size: 10px;
  font-weight: 500;
  color: #5a6a7a;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.t-card__date-day {
  font-size: 18px;
  font-weight: 700;
  color: #e8eaed;
  line-height: 1;
}

.t-card__date-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.t-card__date-label {
  font-size: 11px;
  color: #5a6a7a;
}

.t-card__date-time {
  font-size: 13px;
  font-weight: 500;
  color: #e8eaed;
}

.t-card__date-arrow {
  color: #5a6a7a;
  font-size: 16px;
  margin: 0 4px;
}

/* Countdown */
.t-card__countdown {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}

.t-card__countdown-label {
  font-size: 11px;
  color: #5a6a7a;
}

/* Prize Section */
.t-card__prize-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.t-card__prize-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.t-card__trophy {
  font-size: 16px;
}

.t-card__prize-total {
  font-size: 14px;
  font-weight: 600;
  color: #e8eaed;
}

.t-card__prize-detail {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-left: 22px;
}

.t-card__first-prize {
  font-size: 13px;
  color: #f5a623;
  font-weight: 500;
}

.t-card__info-btn {
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

.t-card__info-btn:hover {
  color: #e8eaed;
}

/* No Prize */
.t-card__no-prize {
  padding: 4px 0;
}

.t-card__no-prize-text {
  font-size: 13px;
  color: #5a6a7a;
}

/* Divider */
.t-card__divider {
  height: 1px;
  background-color: #1a2538;
}

/* Footer */
.t-card__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.t-card__footer--finished {
  flex-wrap: wrap;
  gap: 8px;
}

.t-card__stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.t-card__stat-value {
  font-size: 14px;
  font-weight: 600;
  color: #e8eaed;
}

.t-card__stat-label {
  font-size: 11px;
  color: #5a6a7a;
}

/* Free badge */
.t-card__free-badge {
  display: inline-flex;
  align-items: center;
  padding: 4px 12px;
  border: 1px solid #3ecf8e;
  border-radius: 4px;
  color: #3ecf8e;
  font-size: 13px;
  font-weight: 600;
}

/* Join button */
.t-card__join-btn {
  padding: 8px 24px;
  background-color: #00e5c3;
  color: #0b1019;
  font-size: 14px;
  font-weight: 600;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 150ms ease, transform 150ms ease;
  white-space: nowrap;
}

.t-card__join-btn:hover {
  background-color: #00ccad;
  transform: translateY(-1px);
}

.t-card__join-btn:active {
  transform: translateY(0);
}

/* P&L colors */
.pnl--positive {
  color: #3ecf8e;
}

.pnl--negative {
  color: #f5555d;
}

.pnl--neutral {
  color: #5a6a7a;
}

/* Reward value */
.t-card__reward-value {
  color: #f5a623;
}

/* RTL support */
[dir="rtl"] .t-card__date-pair {
  flex-direction: row-reverse;
}

[dir="rtl"] .t-card__countdown {
  align-items: flex-start;
}

[dir="rtl"] .t-card__prize-detail {
  padding-left: 0;
  padding-right: 22px;
}
</style>
