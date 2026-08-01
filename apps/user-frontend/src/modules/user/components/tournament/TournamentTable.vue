<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';
import TypeBadge from './TypeBadge.vue';
import CountdownTimer from './CountdownTimer.vue';
import PrizeBadge from './PrizeBadge.vue';
import type { TournamentCardProps } from './TournamentCard.vue';

const props = defineProps<{
  tournaments: TournamentCardProps[];
  /** 'active' for upcoming/live | 'finished' for My Tournaments > Finished */
  variant?: 'active' | 'finished';
}>();

const emit = defineEmits<{
  join: [id: string];
}>();

const isFinished = computed(() => props.variant === 'finished');

function formatCurrency(cents: number): string {
  if (cents === 0) return '-';
  const amount = cents / 100;
  return `$${amount.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function formatDateRange(startDate: string, endDate: string, duration: string): { line1: string; line2: string } {
  const start = new Date(startDate);
  const end = new Date(endDate);

  const startMonth = start.toLocaleString('en-US', { month: 'short' });
  const startDay = start.getDate();
  const startTime = start.toLocaleString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });

  const endMonth = end.toLocaleString('en-US', { month: 'short' });
  const endDay = end.getDate();
  const endTime = end.toLocaleString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });

  // Check if same day
  const sameDay = start.toDateString() === end.toDateString();

  let line1: string;
  if (sameDay) {
    line1 = `${startMonth} ${String(startDay).padStart(2, '0')}-${startTime}\u2192${endTime}`;
  } else {
    line1 = `${startMonth} ${String(startDay).padStart(2, '0')}-${startTime}\u2192${endMonth} ${String(endDay).padStart(2, '0')}`;
  }

  return { line1, line2: duration };
}

function formatEntryFee(cents: number): string {
  if (cents === 0) return 'Free';
  return formatCurrency(cents);
}

function pnlClass(pnl?: number): string {
  if (pnl === undefined) return '';
  if (pnl > 0) return 'pnl--positive';
  if (pnl < 0) return 'pnl--negative';
  return 'pnl--neutral';
}

function pnlDisplay(pnl?: number): string {
  if (pnl === undefined) return '-';
  return `${pnl > 0 ? '+' : ''}${pnl}%`;
}

function positionDisplay(pos?: number): string {
  if (!pos) return '-';
  const suffixes: Record<number, string> = { 1: 'st', 2: 'nd', 3: 'rd' };
  return `${pos}${suffixes[pos] || 'th'}`;
}

function rewardDisplay(cents?: number): string {
  if (!cents || cents === 0) return t('tournament.noPrize');
  return formatCurrency(cents);
}

function handleJoin(id: string): void {
  emit('join', id);
}
</script>

<template>
  <div class="t-table-wrapper">
    <table class="t-table">
      <thead>
        <tr>
          <th class="t-table__th">{{ t('tournament.colType') }}</th>
          <th class="t-table__th">{{ t('tournament.colTournament') }}</th>
          <template v-if="!isFinished">
            <th class="t-table__th">{{ t('tournament.colStartEnd') }}</th>
            <th class="t-table__th t-table__th--center">{{ t('tournament.colTraders') }}</th>
            <th class="t-table__th">{{ t('tournament.colPrize') }}</th>
            <th class="t-table__th">{{ t('tournament.colEntryFee') }}</th>
            <th class="t-table__th">{{ t('tournament.colStartingIn') }}</th>
            <th class="t-table__th t-table__th--center"></th>
          </template>
          <template v-else>
            <th class="t-table__th">{{ t('tournament.colStart') }}</th>
            <th class="t-table__th">{{ t('tournament.colEnd') }}</th>
            <th class="t-table__th t-table__th--center">{{ t('tournament.colTraders') }}</th>
            <th class="t-table__th">{{ t('tournament.colEntryFee') }}</th>
            <th class="t-table__th t-table__th--center">{{ t('tournament.colQuantity') }}</th>
            <th class="t-table__th">{{ t('tournament.colPnl') }}</th>
            <th class="t-table__th">{{ t('tournament.colReward') }}</th>
          </template>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="item in tournaments"
          :key="item.id"
          class="t-table__row"
        >
          <!-- Type Badge -->
          <td class="t-table__td">
            <TypeBadge :duration="item.duration" :market="item.type" :is-hot="item.isHot" />
          </td>

          <!-- Tournament Name + ID -->
          <td class="t-table__td">
            <div class="t-table__name-cell">
              <span class="t-table__name">{{ item.type }}</span>
              <span class="t-table__id">ID{{ item.id }}</span>
            </div>
          </td>

          <!-- Active columns -->
          <template v-if="!isFinished">
            <!-- Start & End -->
            <td class="t-table__td">
              <div class="t-table__date-cell">
                <span class="t-table__date-line1">{{ formatDateRange(item.startDate, item.endDate, item.duration).line1 }}</span>
                <span class="t-table__date-line2">{{ formatDateRange(item.startDate, item.endDate, item.duration).line2 }}</span>
              </div>
            </td>

            <!-- Traders -->
            <td class="t-table__td t-table__td--center">
              <span class="t-table__traders">{{ item.traders || '-' }}</span>
            </td>

            <!-- Prize -->
            <td class="t-table__td">
              <PrizeBadge
                :total-prize-cents="item.totalPrizeCents"
                :first-prize-cents="item.firstPrizeCents"
                compact
              />
            </td>

            <!-- Entry Fee -->
            <td class="t-table__td">
              <span :class="['t-table__fee', { 't-table__fee--free': item.entryFeeCents === 0 }]">
                {{ formatEntryFee(item.entryFeeCents) }}
              </span>
            </td>

            <!-- Starting In -->
            <td class="t-table__td">
              <CountdownTimer :target-date="item.startDate" variant="inline" />
            </td>

            <!-- Join -->
            <td class="t-table__td t-table__td--center">
              <button class="t-table__join-btn" @click="handleJoin(item.id)">
                {{ t('tournament.join') }}
              </button>
            </td>
          </template>

          <!-- Finished columns -->
          <template v-else>
            <!-- Start -->
            <td class="t-table__td">
              <span class="t-table__date-text">{{ new Date(item.startDate).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }) }}</span>
            </td>

            <!-- End -->
            <td class="t-table__td">
              <span class="t-table__date-text">{{ new Date(item.endDate).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }) }}</span>
            </td>

            <!-- Traders -->
            <td class="t-table__td t-table__td--center">
              <span class="t-table__traders">{{ item.traders }}</span>
            </td>

            <!-- Entry Fee -->
            <td class="t-table__td">
              <span :class="['t-table__fee', { 't-table__fee--free': item.entryFeeCents === 0 }]">
                {{ formatEntryFee(item.entryFeeCents) }}
              </span>
            </td>

            <!-- Quantity -->
            <td class="t-table__td t-table__td--center">
              <span class="t-table__quantity">{{ item.quantity ?? '-' }}</span>
            </td>

            <!-- P&L -->
            <td class="t-table__td">
              <span :class="['t-table__pnl', pnlClass(item.pnlPercent)]">
                {{ pnlDisplay(item.pnlPercent) }}
              </span>
            </td>

            <!-- Reward -->
            <td class="t-table__td">
              <div class="t-table__reward-cell">
                <span :class="['t-table__reward', { 't-table__reward--gold': item.rewardCents && item.rewardCents > 0 }]">
                  {{ rewardDisplay(item.rewardCents) }}
                </span>
                <span v-if="item.position" class="t-table__position">
                  {{ positionDisplay(item.position) }} {{ t('tournament.position') }}
                </span>
              </div>
            </td>
          </template>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.t-table-wrapper {
  overflow-x: auto;
  border-radius: 8px;
  border: 1px solid #1a2538;
}

.t-table {
  width: 100%;
  border-collapse: collapse;
  background-color: #0f1923;
  font-size: 13px;
}

/* Header */
.t-table__th {
  padding: 12px 16px;
  text-align: left;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #4a5a6a;
  background-color: #0b1019;
  border-bottom: 1px solid #1a2538;
  white-space: nowrap;
}

.t-table__th--center {
  text-align: center;
}

/* Rows */
.t-table__row {
  border-bottom: 1px solid #1a2538;
  transition: background-color 150ms ease;
}

.t-table__row:last-child {
  border-bottom: none;
}

.t-table__row:hover {
  background-color: rgba(255, 255, 255, 0.02);
}

/* Cells */
.t-table__td {
  padding: 12px 16px;
  vertical-align: middle;
  white-space: nowrap;
}

.t-table__td--center {
  text-align: center;
}

/* Name cell */
.t-table__name-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.t-table__name {
  font-size: 14px;
  font-weight: 600;
  color: #e8eaed;
}

.t-table__id {
  font-size: 11px;
  color: #5a6a7a;
}

/* Date cell */
.t-table__date-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.t-table__date-line1 {
  font-size: 13px;
  color: #e8eaed;
}

.t-table__date-line2 {
  font-size: 11px;
  color: #5a6a7a;
}

.t-table__date-text {
  font-size: 13px;
  color: #e8eaed;
}

/* Traders */
.t-table__traders {
  font-size: 13px;
  color: #e8eaed;
}

/* Fee */
.t-table__fee {
  font-size: 13px;
  font-weight: 500;
  color: #e8eaed;
}

.t-table__fee--free {
  color: #3ecf8e;
  font-weight: 600;
}

/* Quantity */
.t-table__quantity {
  font-size: 13px;
  color: #e8eaed;
}

/* P&L */
.t-table__pnl {
  font-size: 13px;
  font-weight: 600;
}

.pnl--positive {
  color: #3ecf8e;
}

.pnl--negative {
  color: #f5555d;
}

.pnl--neutral {
  color: #5a6a7a;
}

/* Reward */
.t-table__reward-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.t-table__reward {
  font-size: 13px;
  color: #5a6a7a;
}

.t-table__reward--gold {
  color: #f5a623;
  font-weight: 600;
}

.t-table__position {
  font-size: 11px;
  color: #5a6a7a;
}

/* Join button */
.t-table__join-btn {
  padding: 6px 20px;
  background-color: #00e5c3;
  color: #0b1019;
  font-size: 13px;
  font-weight: 600;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 150ms ease, transform 150ms ease;
  white-space: nowrap;
}

.t-table__join-btn:hover {
  background-color: #00ccad;
  transform: translateY(-1px);
}

.t-table__join-btn:active {
  transform: translateY(0);
}

/* RTL support */
[dir="rtl"] .t-table__th {
  text-align: right;
}

[dir="rtl"] .t-table__th--center {
  text-align: center;
}
</style>
