<template>
  <div class="tp-mdetails">
    <!-- Contest info header -->
    <div class="tp-mdetails-hdr">
      <div class="tp-mdetails-mode">{{ durationMinutes }}M</div>
      <div class="tp-mdetails-info">
        <h2>{{ contestId }}</h2>
        <span class="tp-mdetails-status" :class="contestStatus">{{ t(`contest.status.${contestStatus}`) }}</span>
      </div>
    </div>

    <!-- Timer card -->
    <div class="tp-mdetails-timer">
      <div class="tp-mdetails-timer-label">{{ t('contest.timeRemaining') }}</div>
      <div class="tp-mdetails-timer-value">
        <div class="tp-mdetails-timer-unit">
          <span class="tp-mdetails-timer-num">{{ timerParts.days }}</span>
          <span class="tp-mdetails-timer-l">{{ t('time.days') }}</span>
        </div>
        <span class="tp-mdetails-timer-sep">:</span>
        <div class="tp-mdetails-timer-unit">
          <span class="tp-mdetails-timer-num">{{ timerParts.hours }}</span>
          <span class="tp-mdetails-timer-l">{{ t('time.hours') }}</span>
        </div>
        <span class="tp-mdetails-timer-sep">:</span>
        <div class="tp-mdetails-timer-unit">
          <span class="tp-mdetails-timer-num">{{ timerParts.minutes }}</span>
          <span class="tp-mdetails-timer-l">{{ t('time.mins') }}</span>
        </div>
        <span class="tp-mdetails-timer-sep">:</span>
        <div class="tp-mdetails-timer-unit">
          <span class="tp-mdetails-timer-num">{{ timerParts.seconds }}</span>
          <span class="tp-mdetails-timer-l">{{ t('time.secs') }}</span>
        </div>
      </div>
    </div>

    <!-- Contest details -->
    <div class="tp-mdetails-section">
      <h3>{{ t('contest.details') }}</h3>
      <div class="tp-mdetails-grid">
        <div class="tp-mdetails-item">
          <span class="tp-mdetails-item-l">{{ t('contest.startTime') }}</span>
          <span class="tp-mdetails-item-v">{{ formatDateTime(startTime) }}</span>
        </div>
        <div class="tp-mdetails-item">
          <span class="tp-mdetails-item-l">{{ t('contest.endTime') }}</span>
          <span class="tp-mdetails-item-v">{{ formatDateTime(endTime) }}</span>
        </div>
        <div class="tp-mdetails-item">
          <span class="tp-mdetails-item-l">{{ t('contest.participants') }}</span>
          <span class="tp-mdetails-item-v">{{ participantCount }}</span>
        </div>
        <div class="tp-mdetails-item">
          <span class="tp-mdetails-item-l">{{ t('contest.maxPositions') }}</span>
          <span class="tp-mdetails-item-v">{{ maxPositions }}</span>
        </div>
        <div class="tp-mdetails-item">
          <span class="tp-mdetails-item-l">{{ t('contest.startingBalance') }}</span>
          <span class="tp-mdetails-item-v">${{ formatNumber(startingBalance) }}</span>
        </div>
        <div class="tp-mdetails-item">
          <span class="tp-mdetails-item-l">{{ t('contest.leverage') }}</span>
          <span class="tp-mdetails-item-v">1:{{ leverage }}</span>
        </div>
      </div>
    </div>

    <!-- Prize pool -->
    <div class="tp-mdetails-section">
      <h3>{{ t('contest.prizePool') }}</h3>
      <template v-if="isFree">
        <div class="tp-mdetails-prize-total">{{ t('contest.freePractice') }}</div>
      </template>
      <template v-else>
        <div class="tp-mdetails-prize-total">${{ formatNumber(prizePool) }}</div>
        <div class="tp-mdetails-prizes">
          <div v-for="(prize, index) in prizes" :key="index" class="tp-mdetails-prize">
            <span class="tp-mdetails-prize-rank">
              <template v-if="index === 0">🥇</template>
              <template v-else-if="index === 1">🥈</template>
              <template v-else-if="index === 2">🥉</template>
              <template v-else>#{{ index + 1 }}</template>
            </span>
            <span class="tp-mdetails-prize-amount">${{ formatNumber(prize) }}</span>
          </div>
        </div>
      </template>
    </div>

    <!-- Rules -->
    <div class="tp-mdetails-section">
      <h3>{{ t('contest.rules') }}</h3>
      <ul class="tp-mdetails-rules">
        <li v-for="(rule, index) in rules" :key="index">{{ rule }}</li>
      </ul>
    </div>

    <!-- Available symbols -->
    <div class="tp-mdetails-section">
      <h3>{{ t('contest.availableSymbols') }}</h3>
      <div class="tp-mdetails-symbols">
        <span v-for="symbol in availableSymbols" :key="symbol" class="tp-mdetails-symbol">
          {{ symbol }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'
import { computed } from 'vue'

const props = defineProps<{
  contestId: string
  contestStatus: 'draft' | 'scheduled' | 'registration_open' | 'running' | 'paused' | 'completed' | 'cancelled'
  durationMinutes: number
  startTime: Date
  endTime: Date
  remainingSeconds: number
  participantCount: number
  maxPositions: number
  startingBalance: number
  leverage: number
  prizePool: number
  prizes: number[]
  rules: string[]
  availableSymbols: string[]
  isFree?: boolean
}>()

const timerParts = computed(() => {
  const seconds = props.remainingSeconds
  return {
    days: String(Math.floor(seconds / 86400)).padStart(2, '0'),
    hours: String(Math.floor((seconds % 86400) / 3600)).padStart(2, '0'),
    minutes: String(Math.floor((seconds % 3600) / 60)).padStart(2, '0'),
    seconds: String(seconds % 60).padStart(2, '0')
  }
})

function formatDateTime(date: Date): string {
  return date.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}

function formatNumber(value: number): string {
  return value.toLocaleString('en-US')
}
</script>

<style scoped>
.tp-mdetails {
  flex: 1;
  overflow-y: auto;
  background: var(--tp-bg);
  padding-bottom: 24px;
}

.tp-mdetails-hdr {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px 16px;
  background: var(--tp-bg-2);
}

.tp-mdetails-mode {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--tp-gn) 0%, #059669 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  font-weight: 700;
  color: #fff;
}

.tp-mdetails-info h2 {
  font-size: 20px;
  font-weight: 600;
  color: var(--tp-tw);
  margin-bottom: 4px;
}

.tp-mdetails-status {
  display: inline-block;
  padding: 4px 10px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
}

.tp-mdetails-status.running {
  background: rgba(16, 185, 129, 0.2);
  color: var(--tp-gn);
}

.tp-mdetails-status.scheduled,
.tp-mdetails-status.registration_open {
  background: rgba(6, 182, 212, 0.2);
  color: var(--tp-bl);
}

.tp-mdetails-status.completed {
  background: rgba(148, 163, 184, 0.2);
  color: var(--tp-t2);
}

.tp-mdetails-status.paused,
.tp-mdetails-status.cancelled {
  background: rgba(239, 68, 68, 0.2);
  color: var(--tp-rd);
}

.tp-mdetails-timer {
  margin: 16px;
  padding: 20px;
  background: var(--tp-bg-2);
  border-radius: 12px;
  text-align: center;
}

.tp-mdetails-timer-label {
  font-size: 13px;
  color: var(--tp-t2);
  margin-bottom: 12px;
}

.tp-mdetails-timer-value {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 8px;
}

.tp-mdetails-timer-unit {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.tp-mdetails-timer-num {
  font-size: 28px;
  font-weight: 700;
  color: var(--tp-bl);
  font-variant-numeric: tabular-nums;
}

.tp-mdetails-timer-l {
  font-size: 10px;
  color: var(--tp-t2);
  text-transform: uppercase;
}

.tp-mdetails-timer-sep {
  font-size: 24px;
  font-weight: 700;
  color: var(--tp-t2);
  margin-top: -16px;
}

.tp-mdetails-section {
  padding: 0 16px;
  margin-bottom: 24px;
}

.tp-mdetails-section h3 {
  font-size: 16px;
  font-weight: 600;
  color: var(--tp-tw);
  margin-bottom: 12px;
}

.tp-mdetails-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.tp-mdetails-item {
  padding: 12px;
  background: var(--tp-bg-2);
  border-radius: 8px;
}

.tp-mdetails-item-l {
  display: block;
  font-size: 12px;
  color: var(--tp-t2);
  margin-bottom: 4px;
}

.tp-mdetails-item-v {
  font-size: 15px;
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-mdetails-prize-total {
  font-size: 32px;
  font-weight: 700;
  color: var(--tp-gn);
  text-align: center;
  margin-bottom: 16px;
}

.tp-mdetails-prizes {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tp-mdetails-prize {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: var(--tp-bg-2);
  border-radius: 8px;
}

.tp-mdetails-prize-rank {
  font-size: 16px;
}

.tp-mdetails-prize-amount {
  font-size: 16px;
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-mdetails-rules {
  list-style: none;
  padding: 0;
  margin: 0;
}

.tp-mdetails-rules li {
  position: relative;
  padding: 10px 0 10px 24px;
  border-bottom: 1px solid var(--tp-bd);
  font-size: 14px;
  color: var(--tp-tw);
}

.tp-mdetails-rules li::before {
  content: '•';
  position: absolute;
  left: 8px;
  color: var(--tp-bl);
}

.tp-mdetails-rules li:last-child {
  border-bottom: none;
}

.tp-mdetails-symbols {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tp-mdetails-symbol {
  padding: 6px 12px;
  background: var(--tp-bg-2);
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  color: var(--tp-tw);
}
</style>
