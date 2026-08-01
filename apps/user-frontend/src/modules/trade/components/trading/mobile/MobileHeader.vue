<template>
  <div class="tp-mhdr">
    <div class="tp-mhdr-top">
      <button class="tp-mhdr-back" @click="goBack">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M19 12H5M12 19l-7-7 7-7"/>
        </svg>
      </button>

      <div class="tp-mhdr-contest">
        <span class="tp-mhdr-cid">{{ contestId }}</span>
        <span class="tp-mhdr-mode">{{ durationMinutes }}M</span>
        <span v-if="isFree" class="tp-mhdr-free">{{ t('contest.free') }}</span>
      </div>

      <button class="tp-mhdr-info" @click="emit('showInfo')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10"/>
          <path d="M12 16v-4M12 8h.01"/>
        </svg>
      </button>
    </div>

    <div class="tp-mhdr-stats">
      <div class="tp-mhdr-stat">
        <span class="tp-mhdr-stat-l">{{ t('mobile.balance') }}</span>
        <span class="tp-mhdr-stat-v">${{ formatNumber(balance) }}</span>
      </div>
      <div class="tp-mhdr-stat">
        <span class="tp-mhdr-stat-l">{{ t('mobile.equity') }}</span>
        <span class="tp-mhdr-stat-v">${{ formatNumber(equity) }}</span>
      </div>
      <div class="tp-mhdr-stat">
        <span class="tp-mhdr-stat-l">{{ t('mobile.pnl') }}</span>
        <span class="tp-mhdr-stat-v" :class="pnl >= 0 ? 'up' : 'down'">
          {{ pnl >= 0 ? '+' : '' }}${{ formatNumber(pnl) }}
        </span>
      </div>
      <div class="tp-mhdr-stat">
        <span class="tp-mhdr-stat-l">{{ t('mobile.rank') }}</span>
        <span class="tp-mhdr-stat-v">#{{ rank }}</span>
      </div>
    </div>

    <div class="tp-mhdr-timer">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="10"/>
        <polyline points="12 6 12 12 16 14"/>
      </svg>
      <span>{{ timerDisplay }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'
import { computed } from 'vue'

const props = defineProps<{
  contestId: string
  durationMinutes: number
  balance: number
  equity: number
  pnl: number
  rank: number
  remainingSeconds: number
  isFree?: boolean
}>()

const emit = defineEmits<{
  (e: 'showInfo'): void
  (e: 'back'): void
}>()

const timerDisplay = computed(() => {
  const seconds = props.remainingSeconds
  const hours = Math.floor(seconds / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60
  return `${String(hours).padStart(2, '0')}:${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`
})

function formatNumber(value: number): string {
  return Math.abs(value).toFixed(2)
}

function goBack() {
  emit('back')
}
</script>

<style scoped>
.tp-mhdr {
  background: var(--tp-bg);
  border-bottom: 1px solid var(--tp-bd);
  padding: 12px 16px;
}

.tp-mhdr-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.tp-mhdr-back,
.tp-mhdr-info {
  width: 36px;
  height: 36px;
  border: none;
  background: var(--tp-bg-2);
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--tp-tw);
  cursor: pointer;
}

.tp-mhdr-back svg,
.tp-mhdr-info svg {
  width: 20px;
  height: 20px;
}

.tp-mhdr-contest {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tp-mhdr-cid {
  font-size: 14px;
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-mhdr-mode {
  padding: 4px 8px;
  background: var(--tp-gn);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  border-radius: 4px;
}

.tp-mhdr-free {
  padding: 4px 8px;
  background: rgba(16, 185, 129, 0.15);
  color: var(--tp-gn);
  font-size: 10px;
  font-weight: 700;
  border-radius: 4px;
  text-transform: uppercase;
}

.tp-mhdr-stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  margin-bottom: 12px;
}

.tp-mhdr-stat {
  text-align: center;
}

.tp-mhdr-stat-l {
  display: block;
  font-size: 10px;
  color: var(--tp-t2);
  text-transform: uppercase;
  margin-bottom: 2px;
}

.tp-mhdr-stat-v {
  font-size: 13px;
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-mhdr-stat-v.up {
  color: var(--tp-gn);
}

.tp-mhdr-stat-v.down {
  color: var(--tp-rd);
}

.tp-mhdr-timer {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px;
  background: var(--tp-bg-2);
  border-radius: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--tp-bl);
}

.tp-mhdr-timer svg {
  width: 16px;
  height: 16px;
}
</style>
