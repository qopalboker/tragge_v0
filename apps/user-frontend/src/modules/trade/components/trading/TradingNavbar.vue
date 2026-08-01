<template>
  <div class="tp-nav">
    <!-- Logo -->
    <div class="tp-nav-logo">
      <svg viewBox="0 0 40 40" fill="none">
        <path d="M20 4L8 12v16l12 8 12-8V12L20 4z" stroke="#10b981" stroke-width="1.5"/>
        <path d="M20 8l-8 5.3v10.7L20 29.3l8-5.3V13.3L20 8z" stroke="#06b6d4" stroke-width=".8" opacity=".5"/>
        <circle cx="20" cy="16" r="2.5" fill="#10b981"/>
        <circle cx="15" cy="22" r="1.8" fill="#8b5cf6"/>
        <circle cx="25" cy="22" r="1.8" fill="#06b6d4"/>
        <line x1="20" y1="16" x2="15" y2="22" stroke="#10b981" stroke-width=".8" opacity=".6"/>
        <line x1="20" y1="16" x2="25" y2="22" stroke="#06b6d4" stroke-width=".8" opacity=".6"/>
      </svg>
      <span class="tp-nav-logo-text">{{ t('app.name') }}</span>
    </div>

    <!-- My tournaments button -->
    <div class="tp-nav-tourn" @click="goToTournaments">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M19 12H5M12 19l-7-7 7-7"/>
      </svg>
      <span>{{ t('nav.myTournaments') }}</span>
    </div>

    <!-- Mode circle -->
    <div class="tp-nav-mode">
      <span class="tp-nav-mode-inner">
        <span class="tp-nav-mode-value">{{ durationValue }}</span>
        <span class="tp-nav-mode-unit">{{ durationUnit }}</span>
      </span>
    </div>

    <!-- Contest info -->
    <div class="tp-nav-contest">
      <div>
        <div class="tp-nav-cid">
          {{ contestId }}
          <span v-if="isFree" class="tp-nav-free-badge">{{ t('contest.free') }}</span>
        </div>
        <div class="tp-nav-cdates">{{ contestDates }}</div>
      </div>
    </div>

    <!-- Center tabs -->
    <div class="tp-nav-center-tabs">
      <button
        class="tp-nctab"
        :class="{ active: activeTab === 'trade' }"
        @click="setActiveTab('trade')"
      >
        <svg viewBox="0 0 24 24" fill="currentColor">
          <rect x="3" y="12" width="4" height="9" rx="1"/>
          <rect x="10" y="6" width="4" height="15" rx="1"/>
          <rect x="17" y="2" width="4" height="19" rx="1"/>
        </svg>
        <span>{{ t('nav.chart') }}</span>
      </button>
      <button
        class="tp-nctab"
        :class="{ active: activeTab === 'leaderboard' }"
        @click="setActiveTab('leaderboard')"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M8 21h8M12 17v4M6 13l-4 4h20l-4-4"/>
          <path d="M6 13V5a2 2 0 012-2h8a2 2 0 012 2v8"/>
        </svg>
        <span>{{ t('nav.leaderboard') }}</span>
      </button>
    </div>

    <!-- Right section -->
    <div class="tp-nright">
      <span class="tp-nav-timer">{{ timerDisplay }}</span>

      <div class="tp-nav-stat" :title="t('nav.participants')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2"/>
          <circle cx="9" cy="7" r="4"/>
          <path d="M23 21v-2a4 4 0 00-3-3.87M16 3.13a4 4 0 010 7.75"/>
        </svg>
        <span class="tp-nav-stat-val">{{ participantCount }}</span>
      </div>

      <div class="tp-nav-stat" :title="t('nav.maxPositions')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="3" y="3" width="18" height="18" rx="1"/>
          <path d="M3 9h18M3 15h18"/>
        </svg>
        <span class="tp-nav-stat-val">{{ maxPositions }}</span>
      </div>

      <button class="tp-nibtn" @click="showInfo" :title="t('nav.info')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10"/>
          <path d="M12 16v-4M12 8h.01"/>
        </svg>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'
import { computed, ref } from 'vue'

const props = defineProps<{
  contestId: string
  startTime: Date
  endTime: Date
  participantCount: number
  maxPositions: number
  durationMinutes: number
  remainingSeconds: number
  isFree?: boolean
}>()

const emit = defineEmits<{
  (e: 'tabChange', tab: 'trade' | 'leaderboard'): void
  (e: 'showInfo'): void
}>()

const activeTab = ref<'trade' | 'leaderboard'>('trade')

const durationValue = computed(() => {
  if (props.durationMinutes < 60) return props.durationMinutes
  return Math.floor(props.durationMinutes / 60)
})

const durationUnit = computed(() => {
  return props.durationMinutes < 60 ? 'M' : 'H'
})

const contestDates = computed(() => {
  const formatDate = (date: Date) => {
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    const hours = String(date.getHours()).padStart(2, '0')
    const minutes = String(date.getMinutes()).padStart(2, '0')
    return `${month}/${day} - ${hours}:${minutes}`
  }
  return `Start: ${formatDate(props.startTime)} &nbsp; End: ${formatDate(props.endTime)}`
})

const timerDisplay = computed(() => {
  const seconds = props.remainingSeconds
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60

  return `${days}d : ${String(hours).padStart(2, '0')}h : ${String(mins).padStart(2, '0')}m : ${String(secs).padStart(2, '0')}s`
})

function setActiveTab(tab: 'trade' | 'leaderboard') {
  activeTab.value = tab
  emit('tabChange', tab)
}

function goToTournaments() {
  // Navigate to user panel tournaments
  window.location.href = '/user/tournaments'
}

function showInfo() {
  emit('showInfo')
}
</script>

<style scoped>
.tp-nav-mode-value,
.tp-nav-mode-unit {
  display: block;
}
</style>
