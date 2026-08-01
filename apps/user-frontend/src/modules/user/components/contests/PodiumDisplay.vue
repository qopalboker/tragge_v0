<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';

interface PodiumEntry {
  rank: number;
  user_id: string;
  username?: string;
  total_score: number;
  pnl_percent?: number;
  reward_cents?: number;
  trade_count?: number;
}

const props = defineProps<{
  entries: PodiumEntry[];
  currentUserId?: string;
}>();

// Get top 3 entries
const topThree = computed(() => {
  return props.entries
    .filter(e => e.rank <= 3)
    .sort((a, b) => a.rank - b.rank);
});

const first = computed(() => topThree.value.find(e => e.rank === 1));
const second = computed(() => topThree.value.find(e => e.rank === 2));
const third = computed(() => topThree.value.find(e => e.rank === 3));

function getDisplayName(entry: PodiumEntry | undefined): string {
  if (!entry) return '-';
  if (entry.username) return entry.username;
  return entry.user_id.substring(0, 8).toUpperCase();
}

function formatScore(pnlPercent: number | undefined): string {
  if (pnlPercent === undefined || pnlPercent === null) return '+0.00%';
  const sign = pnlPercent >= 0 ? '+' : '';
  return `${sign}${pnlPercent.toFixed(2)}%`;
}

function formatPrize(rewardCents: number | undefined): string {
  if (!rewardCents || rewardCents === 0) return '$0';
  const amount = rewardCents / 100;
  if (amount >= 1000) {
    return `$${(amount / 1000).toFixed(1)}K`;
  }
  return `$${amount.toFixed(2)}`;
}

function isCurrentUser(entry: PodiumEntry | undefined): boolean {
  if (!entry || !props.currentUserId) return false;
  return entry.user_id === props.currentUserId;
}
</script>

<template>
  <div class="podium-display">
    <div class="podium-header">
      <svg class="trophy-icon" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6M18 9h1.5a2.5 2.5 0 0 0 0-5H18M4 22h16M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22M18 2H6v7a6 6 0 0 0 12 0V2Z" />
      </svg>
      <h3 class="podium-title">{{ t('contestResults.topWinners') }}</h3>
    </div>

    <div class="podium-container">
      <!-- Second Place (Left) -->
      <div class="podium-position second" :class="{ 'is-you': isCurrentUser(second) }">
        <div class="position-card">
          <div class="medal silver">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z"/>
            </svg>
          </div>
          <div class="rank-number">2</div>
          <div class="username" :title="getDisplayName(second)">
            {{ getDisplayName(second) }}
            <span v-if="isCurrentUser(second)" class="you-badge">{{ t('contestResults.you') }}</span>
          </div>
          <div class="score" :class="{ 'positive': (second?.pnl_percent ?? 0) >= 0, 'negative': (second?.pnl_percent ?? 0) < 0 }">
            {{ formatScore(second?.pnl_percent) }}
          </div>
          <div class="prize">{{ formatPrize(second?.reward_cents) }}</div>
        </div>
        <div class="podium-stand stand-2"></div>
      </div>

      <!-- First Place (Center) -->
      <div class="podium-position first" :class="{ 'is-you': isCurrentUser(first) }">
        <div class="position-card">
          <div class="crown">
            <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
              <path d="M5 16L3 5L8.5 10L12 4L15.5 10L21 5L19 16H5M19 19C19 19.6 18.6 20 18 20H6C5.4 20 5 19.6 5 19V18H19V19Z"/>
            </svg>
          </div>
          <div class="medal gold">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z"/>
            </svg>
          </div>
          <div class="rank-number">1</div>
          <div class="username" :title="getDisplayName(first)">
            {{ getDisplayName(first) }}
            <span v-if="isCurrentUser(first)" class="you-badge">{{ t('contestResults.you') }}</span>
          </div>
          <div class="score" :class="{ 'positive': (first?.pnl_percent ?? 0) >= 0, 'negative': (first?.pnl_percent ?? 0) < 0 }">
            {{ formatScore(first?.pnl_percent) }}
          </div>
          <div class="prize">{{ formatPrize(first?.reward_cents) }}</div>
        </div>
        <div class="podium-stand stand-1"></div>
      </div>

      <!-- Third Place (Right) -->
      <div class="podium-position third" :class="{ 'is-you': isCurrentUser(third) }">
        <div class="position-card">
          <div class="medal bronze">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z"/>
            </svg>
          </div>
          <div class="rank-number">3</div>
          <div class="username" :title="getDisplayName(third)">
            {{ getDisplayName(third) }}
            <span v-if="isCurrentUser(third)" class="you-badge">{{ t('contestResults.you') }}</span>
          </div>
          <div class="score" :class="{ 'positive': (third?.pnl_percent ?? 0) >= 0, 'negative': (third?.pnl_percent ?? 0) < 0 }">
            {{ formatScore(third?.pnl_percent) }}
          </div>
          <div class="prize">{{ formatPrize(third?.reward_cents) }}</div>
        </div>
        <div class="podium-stand stand-3"></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.podium-display {
  background: linear-gradient(180deg, #0f172a 0%, #1e1b4b 50%, #312e81 100%);
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
  overflow: hidden;
}

.podium-header {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-xl);
}

.trophy-icon {
  color: #FFD700;
}

.podium-title {
  font-size: var(--font-size-lg);
  font-weight: 700;
  color: white;
  margin: 0;
}

.podium-container {
  display: flex;
  align-items: flex-end;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-lg) 0;
}

.podium-position {
  display: flex;
  flex-direction: column;
  align-items: center;
  transition: transform var(--transition-base);
}

.podium-position:hover {
  transform: translateY(-4px);
}

.podium-position.is-you .position-card {
  box-shadow: 0 0 20px rgba(99, 102, 241, 0.5);
}

.position-card {
  background: rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.2);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-sm);
  min-width: 120px;
  position: relative;
}

.crown {
  position: absolute;
  top: -24px;
  color: #FFD700;
  animation: float 2s ease-in-out infinite;
}

@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-4px); }
}

.medal {
  display: flex;
  align-items: center;
  justify-content: center;
}

.medal.gold {
  color: #FFD700;
}

.medal.silver {
  color: #C0C0C0;
}

.medal.bronze {
  color: #CD7F32;
}

.rank-number {
  font-size: var(--font-size-2xl);
  font-weight: 800;
  color: white;
  line-height: 1;
}

.username {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: white;
  text-align: center;
  max-width: 100px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.you-badge {
  display: inline-flex;
  margin-left: var(--spacing-xs);
  padding: 2px 6px;
  background: var(--color-primary);
  color: white;
  font-size: 9px;
  font-weight: 700;
  border-radius: var(--radius-sm);
  text-transform: uppercase;
  vertical-align: middle;
}

.score {
  font-size: var(--font-size-md);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.score.positive {
  color: #10B981;
}

.score.negative {
  color: #EF4444;
}

.prize {
  font-size: var(--font-size-lg);
  font-weight: 700;
  color: #FFD700;
  font-variant-numeric: tabular-nums;
}

.podium-stand {
  width: 100%;
  border-radius: var(--radius-md) var(--radius-md) 0 0;
  margin-top: var(--spacing-sm);
}

.stand-1 {
  height: 80px;
  background: linear-gradient(180deg, #FFD700 0%, #FFA500 100%);
}

.stand-2 {
  height: 60px;
  background: linear-gradient(180deg, #E8E8E8 0%, #C0C0C0 100%);
}

.stand-3 {
  height: 40px;
  background: linear-gradient(180deg, #CD7F32 0%, #B87333 100%);
}

/* First place is larger */
.first .position-card {
  min-width: 140px;
  padding: var(--spacing-xl);
}

.first .rank-number {
  font-size: var(--font-size-3xl);
}

/* RTL Support */
[dir="rtl"] .you-badge {
  margin-left: 0;
  margin-right: var(--spacing-xs);
}

[dir="rtl"] .podium-container {
  flex-direction: row-reverse;
}

/* Mobile */
@media (max-width: 767px) {
  .podium-display {
    padding: var(--spacing-lg);
  }

  .podium-container {
    gap: var(--spacing-sm);
  }

  .position-card {
    min-width: 90px;
    padding: var(--spacing-md);
  }

  .first .position-card {
    min-width: 100px;
    padding: var(--spacing-md);
  }

  .rank-number {
    font-size: var(--font-size-xl);
  }

  .first .rank-number {
    font-size: var(--font-size-2xl);
  }

  .username {
    font-size: var(--font-size-xs);
    max-width: 70px;
  }

  .score {
    font-size: var(--font-size-sm);
  }

  .prize {
    font-size: var(--font-size-md);
  }

  .stand-1 {
    height: 60px;
  }

  .stand-2 {
    height: 45px;
  }

  .stand-3 {
    height: 30px;
  }

  .crown {
    top: -20px;
  }

  .crown svg {
    width: 24px;
    height: 24px;
  }
}
</style>
