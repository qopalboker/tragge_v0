<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { userStatsApi, type UserStats } from '@/api';
import TraggePointBadge from '@/components/tragge/TraggePointBadge.vue';

const props = defineProps<{
  stats?: UserStats | null;
  globalRank?: number | null;
  loading?: boolean;
}>();

const router = useRouter();
const localStats = ref<UserStats | null>(null);
const localGlobalRank = ref<number | null>(null);
const localLoading = ref(true);
const error = ref(false);

const effectiveStats = computed(() => props.stats !== undefined ? props.stats : localStats.value);
const effectiveGlobalRank = computed(() => props.globalRank !== undefined ? props.globalRank : localGlobalRank.value);
const effectiveLoading = computed(() => props.loading !== undefined ? props.loading : localLoading.value);

const formattedScore = computed(() => {
  if (!effectiveStats.value) return '0';
  return effectiveStats.value.tragge_point.toLocaleString(undefined, { maximumFractionDigits: 0 });
});

const formattedRank = computed(() => {
  if (!effectiveGlobalRank.value) return '-';
  return `#${effectiveGlobalRank.value.toLocaleString()}`;
});

async function loadStats() {
  localLoading.value = true;
  error.value = false;

  try {
    const [statsData, leaderboardData] = await Promise.all([
      userStatsApi.getMyStats(),
      userStatsApi.getGlobalLeaderboard({ limit: 1 }),
    ]);
    localStats.value = statsData;
    localGlobalRank.value = leaderboardData.user_rank || null;
  } catch {
    error.value = true;
  } finally {
    localLoading.value = false;
  }
}

function goToLeaderboard() {
  router.push({ name: 'global-leaderboard' });
}

// Only fetch independently if no props are provided
onMounted(() => {
  if (props.stats === undefined) {
    loadStats();
  }
});
</script>

<template>
  <div class="tragge-score-card card" @click="goToLeaderboard">
    <!-- Loading State -->
    <div v-if="effectiveLoading" class="loading-state">
      <div class="spinner"></div>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="error-state">
      <p>{{ t('tragge.loadError') }}</p>
      <button class="btn btn-sm btn-secondary" @click.stop="loadStats">
        {{ t('common.retry') }}
      </button>
    </div>

    <!-- Content -->
    <div v-else-if="effectiveStats" class="card-content">
      <div class="card-header">
        <div class="header-left">
          <span class="header-icon">🏆</span>
          <span class="header-title">{{ t('tragge.traggePoint') }}</span>
        </div>
        <TraggePointBadge :score="effectiveStats.tragge_point" size="sm" :show-label="false" />
      </div>

      <div class="score-display">
        <span class="score-value">{{ formattedScore }}</span>
        <span class="score-label">{{ t('tragge.points') }}</span>
      </div>

      <div class="stats-row">
        <div class="mini-stat">
          <span class="mini-value">{{ formattedRank }}</span>
          <span class="mini-label">{{ t('tragge.globalRank') }}</span>
        </div>
        <div class="mini-stat">
          <span class="mini-value">{{ effectiveStats.total_wins }}</span>
          <span class="mini-label">{{ t('tragge.wins') }}</span>
        </div>
        <div class="mini-stat">
          <span class="mini-value">{{ effectiveStats.total_contests }}</span>
          <span class="mini-label">{{ t('tragge.contests') }}</span>
        </div>
      </div>

      <div class="card-footer">
        <span class="view-link">
          {{ t('tragge.viewLeaderboard') }}
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M5 12h14M12 5l7 7-7 7"/>
          </svg>
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tragge-score-card {
  background: linear-gradient(135deg, var(--color-primary) 0%, #7C3AED 100%);
  color: white;
  cursor: pointer;
  transition: transform var(--transition-fast), box-shadow var(--transition-fast);
}

.tragge-score-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-lg);
}

.loading-state,
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-xl);
  gap: var(--spacing-md);
  min-height: 180px;
}

.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.error-state p {
  font-size: var(--font-size-sm);
  opacity: 0.9;
}

.card-content {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.header-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.header-icon {
  font-size: var(--font-size-lg);
}

.header-title {
  font-size: var(--font-size-sm);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  opacity: 0.9;
}

.score-display {
  text-align: center;
  padding: var(--spacing-md) 0;
}

.score-value {
  display: block;
  font-size: var(--font-size-3xl);
  font-weight: 700;
  line-height: 1.2;
}

.score-label {
  font-size: var(--font-size-xs);
  opacity: 0.8;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-sm);
  padding-top: var(--spacing-md);
  border-top: 1px solid rgba(255, 255, 255, 0.2);
}

.mini-stat {
  text-align: center;
}

.mini-value {
  display: block;
  font-size: var(--font-size-lg);
  font-weight: 600;
}

.mini-label {
  font-size: var(--font-size-xs);
  opacity: 0.8;
}

.card-footer {
  display: flex;
  justify-content: center;
  padding-top: var(--spacing-sm);
}

.view-link {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  font-weight: 500;
  opacity: 0.9;
}

.view-link svg {
  transition: transform var(--transition-fast);
}

.tragge-score-card:hover .view-link svg {
  transform: translateX(4px);
}

[dir="rtl"] .view-link svg {
  transform: rotate(180deg);
}

[dir="rtl"] .tragge-score-card:hover .view-link svg {
  transform: rotate(180deg) translateX(4px);
}
</style>
