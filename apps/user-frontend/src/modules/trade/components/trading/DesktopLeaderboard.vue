<template>
  <div class="tp-desk-lb">
    <!-- Leaderboard header -->
    <div class="tp-lb-hdr">
      <h2 class="tp-lb-title">{{ t('leaderboard.title') }}</h2>
      <div class="tp-lb-meta">
        <span>{{ t('leaderboard.participants') }}: {{ participants.length }}</span>
        <span>{{ t('leaderboard.prizePool') }}: ${{ formatNumber(prizePool) }}</span>
      </div>
    </div>

    <!-- Top 3 podium -->
    <div class="tp-lb-podium">
      <!-- 2nd place -->
      <div class="tp-lb-pod tp-lb-pod-2" v-if="topThree[1]">
        <div class="tp-lb-pod-avatar">
          <span class="tp-lb-pod-rank">2</span>
        </div>
        <div class="tp-lb-pod-name">{{ topThree[1].username }}</div>
        <div class="tp-lb-pod-pnl" :class="topThree[1].pnl >= 0 ? 'up' : 'down'">
          {{ formatPnL(topThree[1].pnl) }}
        </div>
        <div class="tp-lb-pod-prize">${{ formatNumber(topThree[1].prize) }}</div>
        <div class="tp-lb-pod-bar tp-lb-bar-2"></div>
      </div>

      <!-- 1st place -->
      <div class="tp-lb-pod tp-lb-pod-1" v-if="topThree[0]">
        <div class="tp-lb-pod-crown">
          <svg viewBox="0 0 24 24" fill="currentColor">
            <path d="M5 16L3 5l5.5 5L12 4l3.5 6L21 5l-2 11H5zm14 3c0 .6-.4 1-1 1H6c-.6 0-1-.4-1-1v-1h14v1z"/>
          </svg>
        </div>
        <div class="tp-lb-pod-avatar tp-lb-pod-avatar-1">
          <span class="tp-lb-pod-rank">1</span>
        </div>
        <div class="tp-lb-pod-name">{{ topThree[0].username }}</div>
        <div class="tp-lb-pod-pnl" :class="topThree[0].pnl >= 0 ? 'up' : 'down'">
          {{ formatPnL(topThree[0].pnl) }}
        </div>
        <div class="tp-lb-pod-prize">${{ formatNumber(topThree[0].prize) }}</div>
        <div class="tp-lb-pod-bar tp-lb-bar-1"></div>
      </div>

      <!-- 3rd place -->
      <div class="tp-lb-pod tp-lb-pod-3" v-if="topThree[2]">
        <div class="tp-lb-pod-avatar">
          <span class="tp-lb-pod-rank">3</span>
        </div>
        <div class="tp-lb-pod-name">{{ topThree[2].username }}</div>
        <div class="tp-lb-pod-pnl" :class="topThree[2].pnl >= 0 ? 'up' : 'down'">
          {{ formatPnL(topThree[2].pnl) }}
        </div>
        <div class="tp-lb-pod-prize">${{ formatNumber(topThree[2].prize) }}</div>
        <div class="tp-lb-pod-bar tp-lb-bar-3"></div>
      </div>
    </div>

    <!-- Leaderboard table -->
    <div class="tp-lb-table">
      <div class="tp-lb-thead">
        <span class="tp-lb-col-rank">#</span>
        <span class="tp-lb-col-user">{{ t('leaderboard.username') }}</span>
        <span class="tp-lb-col-pnl">{{ t('leaderboard.profit') }}</span>
        <span class="tp-lb-col-prize">{{ t('leaderboard.prize') }}</span>
      </div>

      <div class="tp-lb-tbody scrollbar-thin">
        <div
          v-for="(item, index) in remainingParticipants"
          :key="item.userId"
          class="tp-lb-row"
          :class="{ 'tp-lb-row-me': item.userId === currentUserId }"
        >
          <span class="tp-lb-col-rank">{{ index + 4 }}</span>
          <span class="tp-lb-col-user">
            <span class="tp-lb-user-avatar">{{ item.username.charAt(0).toUpperCase() }}</span>
            {{ item.username }}
            <span v-if="item.userId === currentUserId" class="tp-lb-me-badge">{{ t('leaderboard.you') }}</span>
          </span>
          <span class="tp-lb-col-pnl" :class="item.pnl >= 0 ? 'up' : 'down'">
            {{ formatPnL(item.pnl) }}
          </span>
          <span class="tp-lb-col-prize">
            <template v-if="item.prize > 0">${{ formatNumber(item.prize) }}</template>
            <template v-else>-</template>
          </span>
        </div>
      </div>
    </div>

    <!-- Current user position (if not in view) -->
    <div v-if="currentUserPosition && currentUserPosition.rank > 10" class="tp-lb-mypos">
      <div class="tp-lb-mypos-label">{{ t('leaderboard.yourPosition') }}</div>
      <div class="tp-lb-row tp-lb-row-me">
        <span class="tp-lb-col-rank">{{ currentUserPosition.rank }}</span>
        <span class="tp-lb-col-user">
          <span class="tp-lb-user-avatar">{{ currentUserPosition.username.charAt(0).toUpperCase() }}</span>
          {{ currentUserPosition.username }}
        </span>
        <span class="tp-lb-col-pnl" :class="currentUserPosition.pnl >= 0 ? 'up' : 'down'">
          {{ formatPnL(currentUserPosition.pnl) }}
        </span>
        <span class="tp-lb-col-prize">
          <template v-if="currentUserPosition.prize > 0">${{ formatNumber(currentUserPosition.prize) }}</template>
          <template v-else>-</template>
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'
import { computed } from 'vue'

export interface LeaderboardItem {
  userId: string
  username: string
  pnl: number
  prize: number
  rank?: number
}

const props = defineProps<{
  participants: LeaderboardItem[]
  currentUserId: string
  prizePool: number
}>()

const topThree = computed(() => {
  return props.participants.slice(0, 3)
})

const remainingParticipants = computed(() => {
  return props.participants.slice(3)
})

const currentUserPosition = computed(() => {
  const index = props.participants.findIndex(p => p.userId === props.currentUserId)
  if (index === -1) return null
  return {
    ...props.participants[index],
    rank: index + 1
  }
})

function formatPnL(value: number): string {
  const prefix = value >= 0 ? '+' : ''
  return `${prefix}$${Math.abs(value).toFixed(2)}`
}

function formatNumber(value: number): string {
  return value.toLocaleString('en-US', { minimumFractionDigits: 0, maximumFractionDigits: 0 })
}
</script>

<style scoped>
.tp-lb-hdr {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-bottom: 1px solid var(--tp-bd);
}

.tp-lb-title {
  font-size: 20px;
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-lb-meta {
  display: flex;
  gap: 20px;
  font-size: 13px;
  color: var(--tp-t2);
}

.tp-lb-podium {
  display: flex;
  justify-content: center;
  align-items: flex-end;
  gap: 16px;
  padding: 40px 24px 20px;
  background: linear-gradient(180deg, var(--tp-bg-2) 0%, transparent 100%);
}

.tp-lb-pod {
  display: flex;
  flex-direction: column;
  align-items: center;
  position: relative;
}

.tp-lb-pod-crown {
  position: absolute;
  top: -30px;
  width: 32px;
  height: 32px;
  color: #fbbf24;
}

.tp-lb-pod-avatar {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: var(--tp-bg-3);
  border: 3px solid var(--tp-bd);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 8px;
}

.tp-lb-pod-avatar-1 {
  width: 72px;
  height: 72px;
  border-color: #fbbf24;
  background: linear-gradient(135deg, #fbbf24 0%, #f59e0b 100%);
}

.tp-lb-pod-1 .tp-lb-pod-rank {
  font-size: 24px;
  font-weight: 700;
  color: #fff;
}

.tp-lb-pod-2 .tp-lb-pod-avatar {
  border-color: #94a3b8;
  background: linear-gradient(135deg, #94a3b8 0%, #64748b 100%);
}

.tp-lb-pod-3 .tp-lb-pod-avatar {
  border-color: #d97706;
  background: linear-gradient(135deg, #d97706 0%, #b45309 100%);
}

.tp-lb-pod-rank {
  font-size: 18px;
  font-weight: 600;
  color: #fff;
}

.tp-lb-pod-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--tp-tw);
  margin-bottom: 4px;
}

.tp-lb-pod-pnl {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 4px;
}

.tp-lb-pod-pnl.up {
  color: var(--tp-gn);
}

.tp-lb-pod-pnl.down {
  color: var(--tp-rd);
}

.tp-lb-pod-prize {
  font-size: 13px;
  color: var(--tp-t2);
}

.tp-lb-pod-bar {
  width: 80px;
  margin-top: 12px;
  border-radius: 4px 4px 0 0;
}

.tp-lb-bar-1 {
  height: 100px;
  background: linear-gradient(180deg, #fbbf24 0%, #f59e0b 100%);
}

.tp-lb-bar-2 {
  height: 70px;
  background: linear-gradient(180deg, #94a3b8 0%, #64748b 100%);
}

.tp-lb-bar-3 {
  height: 50px;
  background: linear-gradient(180deg, #d97706 0%, #b45309 100%);
}

.tp-lb-table {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.tp-lb-thead {
  display: grid;
  grid-template-columns: 60px 1fr 120px 100px;
  padding: 12px 24px;
  background: var(--tp-bg-2);
  border-bottom: 1px solid var(--tp-bd);
  font-size: 12px;
  font-weight: 500;
  color: var(--tp-t2);
  text-transform: uppercase;
}

.tp-lb-tbody {
  flex: 1;
  overflow-y: auto;
}

.tp-lb-row {
  display: grid;
  grid-template-columns: 60px 1fr 120px 100px;
  padding: 14px 24px;
  border-bottom: 1px solid var(--tp-bd);
  font-size: 14px;
  color: var(--tp-tw);
  transition: background 0.15s;
}

.tp-lb-row:hover {
  background: var(--tp-bg-h);
}

.tp-lb-row-me {
  background: rgba(16, 185, 129, 0.1);
}

.tp-lb-row-me:hover {
  background: rgba(16, 185, 129, 0.15);
}

.tp-lb-col-rank {
  font-weight: 500;
  color: var(--tp-t2);
}

.tp-lb-col-user {
  display: flex;
  align-items: center;
  gap: 10px;
}

.tp-lb-user-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--tp-bg-3);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 600;
  color: var(--tp-t2);
}

.tp-lb-me-badge {
  padding: 2px 8px;
  background: var(--tp-gn);
  color: #fff;
  font-size: 10px;
  font-weight: 600;
  border-radius: 4px;
  text-transform: uppercase;
}

.tp-lb-col-pnl {
  font-weight: 500;
}

.tp-lb-col-pnl.up {
  color: var(--tp-gn);
}

.tp-lb-col-pnl.down {
  color: var(--tp-rd);
}

.tp-lb-col-prize {
  color: var(--tp-t2);
}

.tp-lb-mypos {
  border-top: 2px solid var(--tp-gn);
  background: var(--tp-bg-2);
  padding-top: 8px;
}

.tp-lb-mypos-label {
  padding: 0 24px 8px;
  font-size: 12px;
  font-weight: 500;
  color: var(--tp-gn);
  text-transform: uppercase;
}
</style>
