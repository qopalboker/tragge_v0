<template>
  <div class="tp-mlb">
    <!-- Header -->
    <div class="tp-mlb-hdr">
      <h2>{{ t('leaderboard.title') }}</h2>
      <div class="tp-mlb-meta">
        <span>{{ participants.length }} {{ t('leaderboard.participants') }}</span>
      </div>
    </div>

    <!-- Top 3 podium -->
    <div class="tp-mlb-podium">
      <!-- 2nd place -->
      <div class="tp-mlb-pod tp-mlb-pod-2" v-if="topThree[1]">
        <div class="tp-mlb-pod-avatar">2</div>
        <div class="tp-mlb-pod-name">{{ topThree[1].username }}</div>
        <div class="tp-mlb-pod-pnl" :class="topThree[1].pnl >= 0 ? 'up' : 'down'">
          {{ formatPnL(topThree[1].pnl) }}
        </div>
      </div>

      <!-- 1st place -->
      <div class="tp-mlb-pod tp-mlb-pod-1" v-if="topThree[0]">
        <div class="tp-mlb-pod-crown">
          <svg viewBox="0 0 24 24" fill="currentColor">
            <path d="M5 16L3 5l5.5 5L12 4l3.5 6L21 5l-2 11H5zm14 3c0 .6-.4 1-1 1H6c-.6 0-1-.4-1-1v-1h14v1z"/>
          </svg>
        </div>
        <div class="tp-mlb-pod-avatar tp-mlb-pod-avatar-1">1</div>
        <div class="tp-mlb-pod-name">{{ topThree[0].username }}</div>
        <div class="tp-mlb-pod-pnl" :class="topThree[0].pnl >= 0 ? 'up' : 'down'">
          {{ formatPnL(topThree[0].pnl) }}
        </div>
      </div>

      <!-- 3rd place -->
      <div class="tp-mlb-pod tp-mlb-pod-3" v-if="topThree[2]">
        <div class="tp-mlb-pod-avatar">3</div>
        <div class="tp-mlb-pod-name">{{ topThree[2].username }}</div>
        <div class="tp-mlb-pod-pnl" :class="topThree[2].pnl >= 0 ? 'up' : 'down'">
          {{ formatPnL(topThree[2].pnl) }}
        </div>
      </div>
    </div>

    <!-- Your position card -->
    <div v-if="currentUserPosition" class="tp-mlb-mypos">
      <div class="tp-mlb-mypos-label">{{ t('leaderboard.yourPosition') }}</div>
      <div class="tp-mlb-mypos-card">
        <span class="tp-mlb-mypos-rank">#{{ currentUserPosition.rank }}</span>
        <span class="tp-mlb-mypos-name">{{ currentUserPosition.username }}</span>
        <span class="tp-mlb-mypos-pnl" :class="currentUserPosition.pnl >= 0 ? 'up' : 'down'">
          {{ formatPnL(currentUserPosition.pnl) }}
        </span>
      </div>
    </div>

    <!-- Full list -->
    <div class="tp-mlb-list scrollbar-thin">
      <div
        v-for="(item, index) in remainingParticipants"
        :key="item.userId"
        class="tp-mlb-row"
        :class="{ 'tp-mlb-row-me': item.userId === currentUserId }"
      >
        <span class="tp-mlb-row-rank">{{ index + 4 }}</span>
        <div class="tp-mlb-row-user">
          <span class="tp-mlb-row-avatar">{{ item.username.charAt(0).toUpperCase() }}</span>
          <span class="tp-mlb-row-name">{{ item.username }}</span>
        </div>
        <span class="tp-mlb-row-pnl" :class="item.pnl >= 0 ? 'up' : 'down'">
          {{ formatPnL(item.pnl) }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'
import { computed } from 'vue'

interface LeaderboardItem {
  userId: string
  username: string
  pnl: number
  prize: number
}

const props = defineProps<{
  participants: LeaderboardItem[]
  currentUserId: string
}>()

const topThree = computed(() => props.participants.slice(0, 3))

const remainingParticipants = computed(() => props.participants.slice(3))

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
</script>

<style scoped>
.tp-mlb {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--tp-bg);
}

.tp-mlb-hdr {
  padding: 16px;
  border-bottom: 1px solid var(--tp-bd);
}

.tp-mlb-hdr h2 {
  font-size: 20px;
  font-weight: 600;
  color: var(--tp-tw);
  margin-bottom: 4px;
}

.tp-mlb-meta {
  font-size: 13px;
  color: var(--tp-t2);
}

.tp-mlb-podium {
  display: flex;
  justify-content: center;
  align-items: flex-end;
  gap: 12px;
  padding: 24px 16px;
  background: linear-gradient(180deg, var(--tp-bg-2) 0%, transparent 100%);
}

.tp-mlb-pod {
  display: flex;
  flex-direction: column;
  align-items: center;
  position: relative;
}

.tp-mlb-pod-crown {
  position: absolute;
  top: -24px;
  width: 24px;
  height: 24px;
  color: #fbbf24;
}

.tp-mlb-pod-avatar {
  width: 44px;
  height: 44px;
  border-radius: 50%;
  background: var(--tp-bg-3);
  border: 2px solid var(--tp-bd);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  font-weight: 600;
  color: #fff;
  margin-bottom: 6px;
}

.tp-mlb-pod-avatar-1 {
  width: 56px;
  height: 56px;
  border-color: #fbbf24;
  background: linear-gradient(135deg, #fbbf24 0%, #f59e0b 100%);
  font-size: 20px;
}

.tp-mlb-pod-2 .tp-mlb-pod-avatar {
  border-color: #94a3b8;
  background: linear-gradient(135deg, #94a3b8 0%, #64748b 100%);
}

.tp-mlb-pod-3 .tp-mlb-pod-avatar {
  border-color: #d97706;
  background: linear-gradient(135deg, #d97706 0%, #b45309 100%);
}

.tp-mlb-pod-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--tp-tw);
  margin-bottom: 2px;
  max-width: 80px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tp-mlb-pod-pnl {
  font-size: 14px;
  font-weight: 600;
}

.tp-mlb-pod-pnl.up {
  color: var(--tp-gn);
}

.tp-mlb-pod-pnl.down {
  color: var(--tp-rd);
}

.tp-mlb-mypos {
  margin: 0 16px 16px;
}

.tp-mlb-mypos-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--tp-gn);
  text-transform: uppercase;
  margin-bottom: 8px;
}

.tp-mlb-mypos-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: 12px;
}

.tp-mlb-mypos-rank {
  font-size: 16px;
  font-weight: 700;
  color: var(--tp-gn);
  min-width: 40px;
}

.tp-mlb-mypos-name {
  flex: 1;
  font-size: 15px;
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-mlb-mypos-pnl {
  font-size: 15px;
  font-weight: 600;
}

.tp-mlb-mypos-pnl.up {
  color: var(--tp-gn);
}

.tp-mlb-mypos-pnl.down {
  color: var(--tp-rd);
}

.tp-mlb-list {
  flex: 1;
  overflow-y: auto;
  padding: 0 16px 16px;
}

.tp-mlb-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  border-bottom: 1px solid var(--tp-bd);
}

.tp-mlb-row-me {
  background: rgba(16, 185, 129, 0.05);
  border-radius: 8px;
  border-bottom: none;
  margin-bottom: 4px;
}

.tp-mlb-row-rank {
  font-size: 14px;
  font-weight: 500;
  color: var(--tp-t2);
  min-width: 32px;
}

.tp-mlb-row-user {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 10px;
}

.tp-mlb-row-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--tp-bg-3);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 600;
  color: var(--tp-t2);
}

.tp-mlb-row-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--tp-tw);
}

.tp-mlb-row-pnl {
  font-size: 14px;
  font-weight: 600;
}

.tp-mlb-row-pnl.up {
  color: var(--tp-gn);
}

.tp-mlb-row-pnl.down {
  color: var(--tp-rd);
}
</style>
