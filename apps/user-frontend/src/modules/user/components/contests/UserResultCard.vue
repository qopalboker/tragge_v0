<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';

const props = defineProps<{
  rank: number;
  totalParticipants: number;
  pnlPercent: number;
  rewardCents: number;
}>();

const emit = defineEmits<{
  (e: 'more-info'): void;
}>();

// Computed
const rankSuffix = computed(() => {
  if (props.rank === 1) return 'ST';
  if (props.rank === 2) return 'ND';
  if (props.rank === 3) return 'RD';
  return 'TH';
});

const congratsMessage = computed(() => {
  if (props.rank === 1) {
    return {
      title: t('contestResults.champion'),
      description: t('contestResults.championDesc'),
    };
  }
  if (props.rank === 2) {
    return {
      title: t('contestResults.greatJob'),
      description: t('contestResults.secondPlaceDesc'),
    };
  }
  if (props.rank === 3) {
    return {
      title: t('contestResults.wellDone'),
      description: t('contestResults.thirdPlaceDesc'),
    };
  }
  if (props.rank <= 10) {
    return {
      title: t('contestResults.topTen'),
      description: t('contestResults.topTenDesc'),
    };
  }
  return {
    title: t('contestResults.keepCompeting'),
    description: t('contestResults.keepCompetingDesc'),
  };
});

const rankColor = computed(() => {
  if (props.rank === 1) return '#FFD700'; // Gold
  if (props.rank === 2) return '#C0C0C0'; // Silver
  if (props.rank === 3) return '#CD7F32'; // Bronze
  return '#6366F1'; // Default purple/indigo
});

const formattedPnl = computed(() => {
  const sign = props.pnlPercent >= 0 ? '+' : '';
  return `${sign}${props.pnlPercent.toFixed(2)}%`;
});

const pnlColor = computed(() => {
  if (props.pnlPercent > 0) return 'positive';
  if (props.pnlPercent < 0) return 'negative';
  return 'neutral';
});

const formattedReward = computed(() => {
  if (props.rewardCents === 0) return '$0.00';
  const amount = props.rewardCents / 100;
  return `$${amount.toFixed(2)}`;
});
</script>

<template>
  <div class="user-result-card">
    <!-- Rank Badge with Decorative Elements -->
    <div class="rank-display">
      <!-- Decorative diamonds/shapes -->
      <div class="decorations">
        <div class="diamond diamond-1" :style="{ backgroundColor: rankColor }"></div>
        <div class="diamond diamond-2" :style="{ backgroundColor: rankColor }"></div>
        <div class="diamond diamond-3" :style="{ backgroundColor: rankColor }"></div>
        <div class="diamond diamond-4" :style="{ backgroundColor: rankColor }"></div>
        <div class="diamond diamond-5" :style="{ backgroundColor: rankColor }"></div>
        <div class="diamond diamond-6" :style="{ backgroundColor: rankColor }"></div>
        <div class="diamond diamond-7" :style="{ backgroundColor: rankColor }"></div>
        <div class="diamond diamond-8" :style="{ backgroundColor: rankColor }"></div>
      </div>

      <!-- Main Rank Badge -->
      <div class="rank-badge" :style="{ backgroundColor: rankColor }">
        <span class="rank-number">{{ rank }}</span>
        <span class="rank-suffix">{{ rankSuffix }}</span>
      </div>
    </div>

    <!-- Congratulations Message -->
    <div class="congrats-section">
      <h2 class="congrats-title" :style="{ color: rankColor }">
        {{ congratsMessage.title }}
      </h2>
      <p class="congrats-description">
        {{ congratsMessage.description }}
      </p>
    </div>

    <!-- Stats Section -->
    <div class="stats-section">
      <div class="stat-row">
        <span class="stat-label">{{ t('contestResults.pnlPercent') }}</span>
        <span class="stat-value" :class="pnlColor">{{ formattedPnl }}</span>
      </div>
      <div class="stat-row">
        <span class="stat-label">{{ t('contestResults.reward') }}</span>
        <span class="stat-value reward">{{ formattedReward }}</span>
      </div>
    </div>

    <!-- More Info Button -->
    <button class="more-info-btn" @click="emit('more-info')">
      {{ t('contestResults.moreInfo') }}
    </button>
  </div>
</template>

<style scoped>
.user-result-card {
  background: linear-gradient(180deg, #0f172a 0%, #1e1b4b 100%);
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
  color: white;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-lg);
}

/* Rank Display with Decorations */
.rank-display {
  position: relative;
  width: 160px;
  height: 160px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.decorations {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
}

.diamond {
  position: absolute;
  transform: rotate(45deg);
  opacity: 0.8;
}

.diamond-1 {
  width: 12px;
  height: 12px;
  top: 0;
  left: 50%;
  transform: translateX(-50%) rotate(45deg);
}

.diamond-2 {
  width: 8px;
  height: 8px;
  top: 20px;
  right: 10px;
  opacity: 0.5;
}

.diamond-3 {
  width: 10px;
  height: 10px;
  top: 15px;
  left: 15px;
  opacity: 0.6;
}

.diamond-4 {
  width: 6px;
  height: 6px;
  top: 35px;
  right: 0;
  opacity: 0.4;
}

.diamond-5 {
  width: 8px;
  height: 8px;
  bottom: 35px;
  left: 5px;
  opacity: 0.5;
}

.diamond-6 {
  width: 6px;
  height: 6px;
  bottom: 20px;
  right: 15px;
  opacity: 0.4;
}

.diamond-7 {
  width: 10px;
  height: 10px;
  bottom: 10px;
  left: 25px;
  opacity: 0.6;
}

.diamond-8 {
  width: 8px;
  height: 8px;
  top: 40px;
  left: 0;
  opacity: 0.5;
}

.rank-badge {
  width: 80px;
  height: 80px;
  border-radius: 4px;
  transform: rotate(45deg);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
}

.rank-badge > * {
  transform: rotate(-45deg);
}

.rank-number {
  font-size: 32px;
  font-weight: 800;
  line-height: 1;
  color: #0f172a;
}

.rank-suffix {
  font-size: 12px;
  font-weight: 700;
  color: #0f172a;
  margin-top: -4px;
}

/* Congratulations Section */
.congrats-section {
  text-align: center;
  max-width: 280px;
}

.congrats-title {
  font-size: var(--font-size-xl);
  font-weight: 700;
  margin: 0 0 var(--spacing-sm) 0;
}

.congrats-description {
  font-size: var(--font-size-sm);
  color: rgba(255, 255, 255, 0.8);
  line-height: 1.6;
  margin: 0;
}

/* Stats Section */
.stats-section {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  padding: var(--spacing-md) 0;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.stat-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-xs) 0;
}

.stat-label {
  font-size: var(--font-size-sm);
  color: rgba(255, 255, 255, 0.7);
}

.stat-value {
  font-size: var(--font-size-md);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.stat-value.positive {
  color: #10B981;
}

.stat-value.negative {
  color: #EF4444;
}

.stat-value.neutral {
  color: rgba(255, 255, 255, 0.9);
}

.stat-value.reward {
  color: #FFD700;
}

/* More Info Button */
.more-info-btn {
  width: 100%;
  padding: var(--spacing-md);
  background: transparent;
  border: 1px solid rgba(255, 255, 255, 0.3);
  border-radius: var(--radius-md);
  color: white;
  font-size: var(--font-size-sm);
  font-weight: 600;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.more-info-btn:hover {
  background: rgba(255, 255, 255, 0.1);
  border-color: rgba(255, 255, 255, 0.5);
}

/* RTL Support */
[dir="rtl"] .stat-row {
  flex-direction: row-reverse;
}

/* Mobile */
@media (max-width: 767px) {
  .user-result-card {
    padding: var(--spacing-lg);
  }

  .rank-display {
    width: 140px;
    height: 140px;
  }

  .rank-badge {
    width: 70px;
    height: 70px;
  }

  .rank-number {
    font-size: 28px;
  }

  .rank-suffix {
    font-size: 10px;
  }

  .congrats-title {
    font-size: var(--font-size-lg);
  }

  .congrats-description {
    font-size: var(--font-size-xs);
  }
}
</style>
