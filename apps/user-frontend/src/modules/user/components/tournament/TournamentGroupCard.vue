<script setup lang="ts">
import { ref, computed } from 'vue';
import { t } from '@/i18n';
import TypeBadge from './TypeBadge.vue';
import CountdownTimer from './CountdownTimer.vue';

export interface TierOption {
  contestId: string;
  entryFeeCents: number;
  tierLabel?: string;
  isFree: boolean;
  prizePoolCents: number;
  currentParticipants: number;
  maxParticipants?: number;
}

export interface TournamentGroupProps {
  templateId: string;
  name: string;
  type: 'Forex' | 'Crypto';
  duration: 'Weekly' | '30 minutes' | 'Hourly';
  isHot?: boolean;
  startDate: string;
  endDate: string;
  tiers: TierOption[];
}

const props = defineProps<TournamentGroupProps>();

const emit = defineEmits<{
  join: [contestId: string, templateId: string];
}>();

const selectedTierIndex = ref(0);

const selectedTier = computed(() => props.tiers[selectedTierIndex.value]);

function formatCurrency(cents: number): string {
  const amount = cents / 100;
  return `$${amount.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

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

function tierButtonLabel(tier: TierOption): string {
  if (tier.isFree) return t('tournament.tierFree');
  if (tier.tierLabel) return tier.tierLabel;
  return formatCurrency(tier.entryFeeCents);
}

function selectTier(index: number): void {
  selectedTierIndex.value = index;
}

function handleJoin(): void {
  if (selectedTier.value) {
    emit('join', selectedTier.value.contestId, props.templateId);
  }
}
</script>

<template>
  <div class="tg-card">
    <!-- Header -->
    <div class="tg-card__header">
      <div class="tg-card__title-group">
        <span class="tg-card__type-name">{{ name }}</span>
      </div>
      <TypeBadge :duration="duration" :market="type" :is-hot="isHot" />
    </div>

    <!-- Date Section -->
    <div class="tg-card__dates">
      <div class="tg-card__date-pair">
        <div class="tg-card__date-col">
          <div class="tg-card__date-block">
            <span class="tg-card__date-month">{{ formatMonth(startDate) }}</span>
            <span class="tg-card__date-day">{{ formatDay(startDate) }}</span>
          </div>
          <div class="tg-card__date-meta">
            <span class="tg-card__date-label">{{ t('tournament.start') }}</span>
            <span class="tg-card__date-time">{{ formatTime(startDate) }}</span>
          </div>
        </div>

        <span class="tg-card__date-arrow">&rarr;</span>

        <div class="tg-card__date-col">
          <div class="tg-card__date-block">
            <span class="tg-card__date-month">{{ formatMonth(endDate) }}</span>
            <span class="tg-card__date-day">{{ formatDay(endDate) }}</span>
          </div>
          <div class="tg-card__date-meta">
            <span class="tg-card__date-label">{{ t('tournament.end') }}</span>
            <span class="tg-card__date-time">{{ formatTime(endDate) }}</span>
          </div>
        </div>
      </div>

      <div class="tg-card__countdown">
        <span class="tg-card__countdown-label">{{ t('tournament.startsIn') }}</span>
        <CountdownTimer :target-date="startDate" variant="block" />
      </div>
    </div>

    <!-- Divider -->
    <div class="tg-card__divider"></div>

    <!-- Tier Selection -->
    <div class="tg-card__tiers">
      <span class="tg-card__tiers-label">{{ t('tournament.tierOptions') }}</span>
      <div class="tg-card__tier-buttons">
        <button
          v-for="(tier, index) in tiers"
          :key="tier.contestId"
          :class="['tg-card__tier-btn', { 'tg-card__tier-btn--active': index === selectedTierIndex }]"
          @click="selectTier(index)"
        >
          {{ tierButtonLabel(tier) }}
        </button>
      </div>
    </div>

    <!-- Selected Tier Details -->
    <div v-if="selectedTier" class="tg-card__footer">
      <div class="tg-card__stat">
        <span class="tg-card__stat-value">{{ selectedTier.currentParticipants || '-' }}</span>
        <span class="tg-card__stat-label">{{ t('tournament.tierParticipants', { count: selectedTier.currentParticipants }) }}</span>
      </div>
      <div v-if="selectedTier.prizePoolCents > 0" class="tg-card__stat">
        <span class="tg-card__stat-value">{{ formatCurrency(selectedTier.prizePoolCents) }}</span>
        <span class="tg-card__stat-label">{{ t('tournament.tierPrizePool') }}</span>
      </div>
      <div v-else class="tg-card__stat">
        <span class="tg-card__no-prize-text">{{ t('tournament.noPrize') }}</span>
      </div>
      <button class="tg-card__join-btn" @click="handleJoin">
        {{ t('tournament.join') }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.tg-card {
  background-color: #0f1923;
  border: 1px solid #1a2538;
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* Header */
.tg-card__header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.tg-card__title-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.tg-card__type-name {
  font-size: 16px;
  font-weight: 600;
  color: #e8eaed;
}

/* Date Section */
.tg-card__dates {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.tg-card__date-pair {
  display: flex;
  align-items: center;
  gap: 12px;
}

.tg-card__date-col {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tg-card__date-block {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 48px;
  background-color: #1a2538;
  border-radius: 8px;
}

.tg-card__date-month {
  font-size: 10px;
  font-weight: 500;
  color: #5a6a7a;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.tg-card__date-day {
  font-size: 18px;
  font-weight: 700;
  color: #e8eaed;
  line-height: 1;
}

.tg-card__date-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.tg-card__date-label {
  font-size: 11px;
  color: #5a6a7a;
}

.tg-card__date-time {
  font-size: 13px;
  font-weight: 500;
  color: #e8eaed;
}

.tg-card__date-arrow {
  color: #5a6a7a;
  font-size: 16px;
  margin: 0 4px;
}

/* Countdown */
.tg-card__countdown {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}

.tg-card__countdown-label {
  font-size: 11px;
  color: #5a6a7a;
}

/* Divider */
.tg-card__divider {
  height: 1px;
  background-color: #1a2538;
}

/* Tier Selection */
.tg-card__tiers {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tg-card__tiers-label {
  font-size: 12px;
  color: #5a6a7a;
  font-weight: 500;
}

.tg-card__tier-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tg-card__tier-btn {
  padding: 6px 14px;
  background-color: #1a2538;
  border: 1px solid #2a3a4e;
  border-radius: 6px;
  color: #e8eaed;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: border-color 150ms ease, color 150ms ease, background-color 150ms ease;
}

.tg-card__tier-btn:hover {
  border-color: #00e5c3;
  color: #00e5c3;
}

.tg-card__tier-btn--active {
  border-color: #00e5c3;
  color: #00e5c3;
  background-color: rgba(0, 229, 195, 0.08);
}

/* Footer */
.tg-card__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.tg-card__stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.tg-card__stat-value {
  font-size: 14px;
  font-weight: 600;
  color: #e8eaed;
}

.tg-card__stat-label {
  font-size: 11px;
  color: #5a6a7a;
}

.tg-card__no-prize-text {
  font-size: 13px;
  color: #5a6a7a;
}

/* Join button */
.tg-card__join-btn {
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

.tg-card__join-btn:hover {
  background-color: #00ccad;
  transform: translateY(-1px);
}

.tg-card__join-btn:active {
  transform: translateY(0);
}

/* RTL support */
[dir="rtl"] .tg-card__date-pair {
  flex-direction: row-reverse;
}

[dir="rtl"] .tg-card__countdown {
  align-items: flex-start;
}

[dir="rtl"] .tg-card__tier-buttons {
  flex-direction: row-reverse;
}
</style>
