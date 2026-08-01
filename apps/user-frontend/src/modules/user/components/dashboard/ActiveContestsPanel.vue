<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { myTournamentsApi, type MyTournamentEntry, api } from '@/api';
import { redirectToTrade } from '@/utils/tradeRedirect';

interface SettlingContest {
  contest_id: string;
  contest_name: string;
  status: string;
  starts_at: string;
  ends_at: string;
  total_score: number;
  total_participants: number;
  pnl_percent?: number;
  final_rank?: number;
}

const router = useRouter();

const activeTab = ref<'active' | 'settling'>('active');
const activeContests = ref<MyTournamentEntry[]>([]);
const settlingContests = ref<SettlingContest[]>([]);
const loading = ref(true);
const now = ref(Date.now());
let tickInterval: ReturnType<typeof setInterval> | null = null;

async function loadData() {
  loading.value = true;
  try {
    const [activeData, historyData] = await Promise.all([
      myTournamentsApi.getMyTournaments({ status: 'active', per_page: 5 }),
      api.get<{ contests: SettlingContest[] }>('/api/user/me/history', { params: { page: 1, per_page: 20 } }),
    ]);
    activeContests.value = activeData.contests;
    settlingContests.value = (historyData.data.contests || ([] as SettlingContest[])).filter(
      (c) => c.status === 'settling'
    );
    if (activeContests.value.length === 0 && settlingContests.value.length > 0) {
      activeTab.value = 'settling';
    }
  } catch {
    // silent
  } finally {
    loading.value = false;
  }
}

const currentList = computed<(MyTournamentEntry | SettlingContest)[]>(() =>
  activeTab.value === 'active' ? activeContests.value : settlingContests.value
);

const hasAny = computed(() =>
  activeContests.value.length > 0 || settlingContests.value.length > 0
);

const activeCount = computed(() => activeContests.value.length);
const settlingCount = computed(() => settlingContests.value.length);

async function goToContest(contest: MyTournamentEntry | SettlingContest) {
  if (activeTab.value === 'active') {
    await redirectToTrade(contest.contest_id);
  } else {
    router.push({ name: 'contest-results', params: { contestId: contest.contest_id } });
  }
}

function formatPnl(value: number | undefined): string {
  if (value === undefined || value === null) return '0.00%';
  const sign = value >= 0 ? '+' : '';
  return `${sign}${value.toFixed(2)}%`;
}

function formatTimeLeft(endsAt: string): string {
  const end = new Date(endsAt).getTime();
  const diff = end - now.value;
  if (diff <= 0) return '—';
  const hours = Math.floor(diff / 3600000);
  const minutes = Math.floor((diff % 3600000) / 60000);
  if (hours >= 24) {
    const days = Math.floor(hours / 24);
    return `${days}d ${hours % 24}h`;
  }
  return `${hours}h ${minutes}m`;
}

onMounted(() => {
  loadData();
  tickInterval = setInterval(() => {
    now.value = Date.now();
  }, 60_000);
});

onUnmounted(() => {
  if (tickInterval) clearInterval(tickInterval);
});
</script>

<template>
  <div v-if="!loading && hasAny" class="active-contests-panel">
    <!-- Header with tabs -->
    <div class="panel-header">
      <h3 class="panel-title">{{ t('dashboard.activeContests') }}</h3>
      <div class="tab-pills">
        <button
          :class="['tab-pill', { active: activeTab === 'active' }]"
          @click="activeTab = 'active'"
        >
          {{ t('dashboard.tabActive') }}
          <span v-if="activeCount > 0" class="tab-badge">{{ activeCount }}</span>
        </button>
        <button
          :class="['tab-pill', { active: activeTab === 'settling' }]"
          @click="activeTab = 'settling'"
        >
          {{ t('dashboard.tabSettling') }}
          <span v-if="settlingCount > 0" class="tab-badge">{{ settlingCount }}</span>
        </button>
      </div>
    </div>

    <!-- Contest list -->
    <div class="contest-list">
      <!-- Empty state -->
      <div v-if="currentList.length === 0" class="empty-state">
        <p>{{ activeTab === 'active' ? t('dashboard.noActiveContests') : t('dashboard.noSettlingContests') }}</p>
        <RouterLink v-if="activeTab === 'active'" to="/user/contests" class="btn-action btn-primary-action">
          {{ t('dashboard.joinTournament') }}
        </RouterLink>
      </div>

      <!-- Contest items -->
      <div
        v-for="contest in currentList"
        :key="contest.contest_id"
        class="contest-item"
        @click="goToContest(contest)"
      >
        <div class="contest-info">
          <div class="contest-name">{{ contest.contest_name }}</div>
          <div v-if="activeTab === 'active'" class="contest-meta">
            <span class="meta-item">
              <span class="meta-label">{{ t('dashboard.timeLeft') }}:</span>
              <span class="meta-value">{{ formatTimeLeft(contest.ends_at) }}</span>
            </span>
            <span class="meta-separator">·</span>
            <span class="meta-item">
              <span class="meta-label">{{ t('dashboard.participants') }}:</span>
              <span class="meta-value">{{ contest.total_participants }}</span>
            </span>
            <span class="meta-separator">·</span>
            <span class="meta-item">
              <span class="meta-label">{{ t('dashboard.pnl') }}:</span>
              <span :class="['meta-value', (contest.pnl_percent || 0) >= 0 ? 'pnl-positive' : 'pnl-negative']">
                {{ formatPnl(contest.pnl_percent) }}
              </span>
            </span>
          </div>
          <div v-else class="contest-meta">
            <span class="settling-info">{{ t('dashboard.settlingInfo') }}</span>
            <span class="meta-separator">·</span>
            <span class="meta-item">
              <span class="meta-label">{{ t('dashboard.participants') }}:</span>
              <span class="meta-value">{{ contest.total_participants }}</span>
            </span>
          </div>
        </div>
        <span class="btn-action" :class="activeTab === 'active' ? 'btn-primary-action' : 'btn-secondary-action'">
          {{ activeTab === 'active' ? t('dashboard.continueTrading') : t('dashboard.viewResults') }}
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.active-contests-panel {
  background: var(--theme-glass);
  border: 1px solid var(--theme-glass-border);
  border-radius: 16px;
  backdrop-filter: blur(20px);
  padding: 18px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}

.panel-title {
  margin: 0;
  font-size: 16px;
  font-weight: 800;
  color: var(--theme-text);
}

.tab-pills {
  display: flex;
  gap: 6px;
}

.tab-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 12px;
  border-radius: 20px;
  border: 1px solid var(--theme-glass-border);
  background: transparent;
  color: var(--theme-text-secondary);
  font-size: 12px;
  font-weight: 600;
  font-family: inherit;
  cursor: pointer;
  transition: all 0.2s;
}

.tab-pill:hover {
  background: var(--theme-surface-hover);
}

.tab-pill.active {
  background: var(--theme-accent);
  color: #fff;
  border-color: var(--theme-accent);
}

.tab-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  border-radius: 9px;
  font-size: 10px;
  font-weight: 700;
  padding: 0 4px;
  background: rgba(255, 255, 255, 0.2);
}

.tab-pill:not(.active) .tab-badge {
  background: var(--theme-surface);
  color: var(--theme-text-secondary);
}

.contest-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.contest-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 12px;
  background: var(--theme-surface);
  border: 1px solid var(--theme-glass-border);
  cursor: pointer;
  transition: background 0.15s;
}

.contest-item:hover {
  background: var(--theme-surface-hover);
}

.contest-info {
  flex: 1;
  min-width: 0;
}

.contest-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--theme-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-bottom: 4px;
}

.contest-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--theme-text-secondary);
}

.meta-label {
  font-weight: 500;
}

.meta-value {
  font-weight: 700;
  color: var(--theme-text);
}

.meta-separator {
  color: var(--theme-glass-border);
}

.settling-info {
  font-style: italic;
  color: var(--theme-text-secondary);
}

.pnl-positive {
  color: var(--theme-green) !important;
}

.pnl-negative {
  color: var(--theme-red) !important;
}

.btn-action {
  padding: 6px 14px;
  border-radius: 8px;
  border: none;
  font-size: 12px;
  font-weight: 700;
  font-family: inherit;
  white-space: nowrap;
  transition: opacity 0.15s;
  flex-shrink: 0;
}

.btn-action:hover {
  opacity: 0.85;
}

.btn-primary-action {
  background: var(--theme-accent);
  color: #fff;
}

.btn-secondary-action {
  background: var(--theme-surface-hover);
  color: var(--theme-text);
  border: 1px solid var(--theme-glass-border);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 20px;
  text-align: center;
  color: var(--theme-text-secondary);
  font-size: 13px;
}

.empty-state p {
  margin: 0;
}

/* Mobile responsive */
@media (max-width: 767px) {
  .active-contests-panel {
    padding: 12px;
  }

  .panel-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
  }

  .contest-item {
    flex-direction: column;
    align-items: stretch;
  }

  .btn-action {
    text-align: center;
  }
}
</style>
