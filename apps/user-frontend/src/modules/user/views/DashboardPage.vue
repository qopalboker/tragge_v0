<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { t } from '@/i18n';
import StatCard from '@/components/dashboard/StatCard.vue';
import LeaderboardPreview from '@/components/dashboard/LeaderboardPreview.vue';
import ContestCarousel from '@/components/dashboard/ContestCarousel.vue';
import FreePracticeSection from '@/components/dashboard/FreePracticeSection.vue';
import TraggePointCard from '@/components/dashboard/TraggePointCard.vue';
import ActiveContestsPanel from '@/components/dashboard/ActiveContestsPanel.vue';
import { type Contest } from '@/stores/contests';
import { userStatsApi, type UserStats, type GlobalLeaderboardEntry, api } from '@/api';
import { useAuthStore } from '@/stores/auth';

const authStore = useAuthStore();

// State for API data
const stats = ref<UserStats | null>(null);
const globalRank = ref<number | null>(null);
const totalPlayers = ref<number>(0);
const leaderboard = ref<{ rank: number; username: string; score: number }[]>([]);
// Single source of truth for contests on the dashboard. Both the
// "starting soon" carousel and the FreePracticeSection consume slices
// of this — folded together so a fresh dashboard load makes ONE
// /api/user/contests request instead of two.
const contests = ref<Contest[]>([]);
const upcomingContests = computed(() =>
  contests.value.filter(
    (c) => c.status === 'registration_open' || c.status === 'scheduled',
  ),
);
const freeContests = computed(() =>
  contests.value.filter((c) => c.is_free === true),
);
const loading = ref(true);
const error = ref(false);

// Computed values from stats
const winRate = computed(() => stats.value?.win_rate || 0);
const totalWins = computed(() => stats.value?.total_wins || 0);
const totalContests = computed(() => stats.value?.total_contests || 0);
const userRank = computed(() => globalRank.value || 0);

const userName = computed(() => {
  const user = authStore.user;
  if (user?.username) return user.username;
  if (user?.email) return user.email.split('@')[0];
  return 'Trader';
});

// Highlights data
const highlights = computed(() => [
  { value: stats.value ? Math.round(stats.value.tragge_point).toLocaleString() : '0', label: t('dashboard.traggePoint'), icon: '🏆' },
  { value: totalContests.value.toString(), label: t('dashboard.totalContests'), icon: '📊' },
  { value: `${winRate.value}%`, label: t('dashboard.winRate'), icon: '🎯' },
  { value: `#${userRank.value || '-'}`, label: t('dashboard.globalRank'), icon: '🌍' },
]);

async function loadDashboardData() {
  loading.value = true;
  error.value = false;

  // Fetch user stats and leaderboard in parallel
  const [statsData, leaderboardData] = await Promise.all([
    userStatsApi.getMyStats().catch(() => null),
    userStatsApi.getGlobalLeaderboard({ limit: 5 }).catch(() => null),
  ]);

  if (statsData) {
    stats.value = statsData;
  }

  if (leaderboardData) {
    leaderboard.value = leaderboardData.entries.map((entry: GlobalLeaderboardEntry) => ({
      rank: entry.rank,
      username: entry.username || `Trader${entry.rank}`,
      score: Math.round(entry.tragge_point),
    }));

    globalRank.value = leaderboardData.user_rank || null;

    if (leaderboardData.entries.length > 0) {
      totalPlayers.value = Math.max(
        leaderboardData.entries.length,
        globalRank.value || 0
      );
    }
  }

  // Set error only if BOTH failed
  error.value = !statsData && !leaderboardData;

  // Fetch contests once with a superset query — covers both the
  // "starting soon" upcoming list and FreePracticeSection (which
  // receives the filtered free subset as a prop). Bumped limit to 10
  // so both sections still have headroom after client-side filtering.
  try {
    const contestsResponse = await api.get<{ contests: Contest[] }>(
      '/api/user/contests',
      {
        params: {
          status: 'running,scheduled,registration_open',
          limit: 10,
        },
      },
    );
    contests.value = (contestsResponse.data.contests || []).filter(
      (c: Contest) => c.status !== 'cancelled',
    );
  } catch {
    // Silent fail for contests - section will show empty state
    contests.value = [];
  }

  loading.value = false;
}

onMounted(() => {
  loadDashboardData();
});
</script>

<template>
  <div class="dashboard">
    <!-- Welcome Header -->
    <div class="welcome-section glass">
      <div class="welcome-content">
        <div class="welcome-text">
          <div class="welcome-greeting">{{ t('dashboard.welcome') }}, <span class="welcome-name">{{ userName }}</span></div>
          <div class="welcome-sub">{{ t('dashboard.subtitle') }}</div>
        </div>
      </div>
    </div>

    <!-- Error State -->
    <div v-if="error && !loading" class="error-banner glass">
      <p>{{ t('dashboard.loadError') }}</p>
      <button class="btn btn-sm btn-secondary" @click="loadDashboardData">
        {{ t('common.retry') }}
      </button>
    </div>

    <!-- Main Content: Two columns on desktop -->
    <div class="dashboard-grid">
      <!-- Left: Main content -->
      <div class="dashboard-main">
        <!-- T-Point Card -->
        <TraggePointCard :stats="stats" :globalRank="globalRank" :loading="loading" />

        <!-- Active/Settling Contests Panel (only shows if user has any) -->
        <ActiveContestsPanel />

        <!-- Stats Row -->
        <div class="stats-row">
          <template v-if="loading">
            <div class="stat-skeleton glass"></div>
            <div class="stat-skeleton glass"></div>
          </template>
          <template v-else>
            <StatCard
              type="winRate"
              :value="winRate"
              :wins="totalWins"
              :total="totalContests"
            />
            <StatCard
              type="rank"
              :value="userRank"
              :totalPlayers="totalPlayers"
            />
          </template>
        </div>

        <!-- Starting Soon -->
        <section class="section">
          <div class="section-header">
            <h2 class="section-title">{{ t('dashboard.startingSoon') }}</h2>
            <RouterLink to="/user/contests" class="see-all-link">
              {{ t('dashboard.seeAll') }} ›
            </RouterLink>
          </div>
          <!-- Loading skeleton -->
          <div v-if="loading" class="carousel-skeleton glass">
            <div class="skeleton-card"></div>
            <div class="skeleton-card"></div>
            <div class="skeleton-card"></div>
          </div>
          <!-- Empty state -->
          <div v-else-if="upcomingContests.length === 0" class="empty-state glass">
            <p>{{ t('contests.noResults') }}</p>
            <RouterLink to="/user/contests" class="btn btn-primary btn-sm">
              {{ t('dashboard.viewAll') }}
            </RouterLink>
          </div>
          <!-- Contests loaded -->
          <ContestCarousel v-else :contests="upcomingContests" />
        </section>

        <!-- Free Practice Section -->
        <FreePracticeSection :contests="freeContests" />
      </div>

      <!-- Right: Sidebar -->
      <div class="dashboard-sidebar">
        <!-- Highlights -->
        <div class="glass highlights-card">
          <div class="card-header">
            <span class="card-header-icon">⚡</span>
            <h3 class="card-header-title">{{ t('dashboard.highlights') }}</h3>
          </div>
          <div class="highlights-grid">
            <div v-for="(h, i) in highlights" :key="i" class="highlight-item">
              <div class="highlight-icon">{{ h.icon }}</div>
              <div class="highlight-value">{{ h.value }}</div>
              <div class="highlight-label">{{ h.label }}</div>
            </div>
          </div>
        </div>

        <!-- Global Ranking -->
        <div class="glass ranking-card">
          <div class="ranking-header">
            <div class="card-header">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="card-header-svg">
                <circle cx="12" cy="12" r="10"/>
                <line x1="2" y1="12" x2="22" y2="12"/>
                <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
              </svg>
              <h3 class="card-header-title">{{ t('dashboard.globalRanking') }}</h3>
            </div>
            <RouterLink to="/user/leaderboard/global" class="see-all-link">
              {{ t('dashboard.seeAll') }} ›
            </RouterLink>
          </div>

          <div v-if="loading" class="ranking-skeleton">
            <div v-for="i in 5" :key="i" class="skeleton-row"></div>
          </div>
          <div v-else-if="leaderboard.length === 0" class="empty-state-small">
            <p>{{ t('leaderboard.noEntries') }}</p>
          </div>
          <LeaderboardPreview v-else :entries="leaderboard" :userRank="userRank" />
        </div>

        <!-- Help & Support -->
        <div class="glass help-card">
          <h3 class="card-header-title" style="margin-bottom: 12px;">{{ t('dashboard.helpSupport') }}</h3>
          <div class="help-links">
            <RouterLink to="/user/settings" class="help-link">
              <span class="help-link-text">{{ t('dashboard.termsConditions') }}</span>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="rtl-flip">
                <polyline points="9 18 15 12 9 6"/>
              </svg>
            </RouterLink>
            <RouterLink to="/user/settings" class="help-link">
              <span class="help-link-text">{{ t('dashboard.helpCenter') }}</span>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="rtl-flip">
                <polyline points="9 18 15 12 9 6"/>
              </svg>
            </RouterLink>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* Welcome Section */
.welcome-section {
  padding: 20px 24px;
}

.welcome-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.welcome-greeting {
  font-size: 22px;
  font-weight: 800;
  color: var(--theme-text);
}

.welcome-name {
  background: linear-gradient(135deg, var(--theme-accent), var(--theme-green));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.welcome-sub {
  font-size: 13px;
  color: var(--theme-text-secondary);
  margin-top: 4px;
}

/* Grid layout */
.dashboard-grid {
  display: flex;
  gap: 16px;
}

.dashboard-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.dashboard-sidebar {
  width: 280px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* Stats Row */
.stats-row {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.stat-skeleton {
  height: 80px;
  animation: pulse 2s ease-in-out infinite;
}

/* Sections */
.section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-title {
  font-size: 16px;
  font-weight: 800;
  color: var(--theme-text);
  margin: 0;
}

.see-all-link {
  font-size: 12px;
  color: var(--theme-accent);
  text-decoration: none;
  font-weight: 600;
}

.see-all-link:hover {
  text-decoration: underline;
}

/* Sidebar cards */
.highlights-card,
.ranking-card,
.help-card {
  padding: 18px;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.card-header-icon {
  font-size: 14px;
  font-weight: 800;
  width: 28px;
  height: 28px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--theme-accent) 12%, transparent);
  color: var(--theme-accent);
}

.card-header-svg {
  color: var(--theme-accent);
}

.card-header-title {
  margin: 0;
  font-size: 17px;
  font-weight: 800;
  color: var(--theme-text);
}

/* Highlights */
.highlights-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.highlight-icon {
  font-size: 16px;
  margin-bottom: 2px;
}

.highlight-value {
  font-size: 28px;
  font-weight: 900;
  color: var(--theme-text);
  line-height: 1.2;
}

.highlight-label {
  font-size: 10px;
  color: var(--theme-text-secondary);
  font-weight: 600;
  text-transform: uppercase;
  line-height: 1.3;
  margin-top: 2px;
}

/* Ranking */
.ranking-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 10px;
}

.ranking-header .card-header {
  margin-bottom: 0;
}

.ranking-skeleton {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.skeleton-row {
  height: 32px;
  background: var(--theme-surface);
  border-radius: 4px;
  animation: pulse 2s ease-in-out infinite;
}

/* Help */
.help-links {
  display: flex;
  flex-direction: column;
}

.help-link {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 4px;
  border-bottom: 1px solid var(--theme-glass-border);
  text-decoration: none;
  transition: background 0.15s;
  cursor: pointer;
}

.help-link:last-child {
  border-bottom: none;
}

.help-link:hover {
  background: var(--theme-surface-hover);
}

.help-link-text {
  font-size: 13px;
  font-weight: 600;
  color: var(--theme-text);
}

.help-link svg {
  color: var(--theme-text-secondary);
}

/* Error banner */
.error-banner {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 14px 20px;
  color: var(--theme-red);
  font-size: 13px;
  font-weight: 600;
}

.error-banner p {
  margin: 0;
}

/* Carousel skeleton */
.carousel-skeleton {
  display: flex;
  gap: 12px;
  padding: 16px;
  overflow: hidden;
}

.skeleton-card {
  flex-shrink: 0;
  width: 220px;
  height: 140px;
  border-radius: 12px;
  background: var(--theme-surface);
  animation: pulse 2s ease-in-out infinite;
}

/* Empty states */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-xl);
  gap: var(--spacing-md);
  text-align: center;
  color: var(--theme-text-secondary);
}

.empty-state p {
  margin: 0;
}

.empty-state-small {
  padding: 16px;
  text-align: center;
  color: var(--theme-text-secondary);
  font-size: 13px;
}

/* Responsive */
@media (max-width: 1023px) {
  .dashboard-grid {
    flex-direction: column;
  }

  .dashboard-sidebar {
    width: 100%;
  }
}

@media (max-width: 767px) {
  .stats-row {
    grid-template-columns: 1fr;
  }

  .welcome-greeting {
    font-size: 18px;
  }

  .highlights-card,
  .ranking-card,
  .help-card {
    padding: 12px;
  }

  .card-header-title {
    font-size: 14px;
  }

  .highlight-value {
    font-size: 20px;
  }

  .highlights-grid {
    gap: 10px;
  }
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
