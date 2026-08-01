<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue';
import { t } from '@/i18n';
import { myTournamentsApi, type MyTournamentEntry, type MyTournamentStatus, type MyTournamentCounts } from '@/api';
import { useToast } from '@/composables/useToast';
import MyContestCard from '@/components/contests/MyContestCard.vue';

const toast = useToast();

const activeTab = ref<MyTournamentStatus>('active');
const contests = ref<MyTournamentEntry[]>([]);
const counts = ref<MyTournamentCounts>({ active: 0, upcoming: 0, completed: 0, cancelled: 0 });
const loading = ref(false);
const error = ref<string | null>(null);
const page = ref(1);
const perPage = 10;
const total = ref(0);
const refreshing = ref(false);

const hasMore = computed(() => contests.value.length < total.value);

async function loadTournaments(resetPage = true): Promise<void> {
  if (resetPage) {
    page.value = 1;
    contests.value = [];
  }
  loading.value = true;
  error.value = null;
  try {
    const data = await myTournamentsApi.getMyTournaments({
      status: activeTab.value,
      page: page.value,
      per_page: perPage,
    });
    if (resetPage) {
      contests.value = data.contests;
    } else {
      contests.value = [...contests.value, ...data.contests];
    }
    total.value = data.total;
    counts.value = data.counts;
  } catch {
    error.value = t('myTournaments.loadError');
    toast.error(t('myTournaments.loadError'));
  } finally {
    loading.value = false;
  }
}

async function loadMore(): Promise<void> {
  if (!hasMore.value || loading.value) return;
  page.value++;
  await loadTournaments(false);
}

async function refresh(): Promise<void> {
  if (refreshing.value) return;
  refreshing.value = true;
  try {
    await loadTournaments(true);
  } finally {
    refreshing.value = false;
  }
}

function switchTab(tab: MyTournamentStatus): void {
  if (activeTab.value === tab) return;
  activeTab.value = tab;
}

watch(activeTab, () => {
  loadTournaments(true);
});

onMounted(() => {
  loadTournaments(true);
});

// Pull-to-refresh support
let touchStartY = 0;
function onTouchStart(e: TouchEvent): void {
  touchStartY = e.touches[0].clientY;
}
function onTouchEnd(e: TouchEvent): void {
  const touchEndY = e.changedTouches[0].clientY;
  const diff = touchEndY - touchStartY;
  if (diff > 80 && window.scrollY === 0) {
    refresh();
  }
}
</script>

<template>
  <div
    class="my-tournaments-page"
    @touchstart="onTouchStart"
    @touchend="onTouchEnd"
  >
    <!-- Pull-to-refresh indicator -->
    <div v-if="refreshing" class="refresh-indicator">
      <div class="spinner" />
      <span>{{ t('myTournaments.refreshing') }}</span>
    </div>

    <!-- Counters -->
    <div class="counters">
      <span class="counter">
        {{ t('myTournaments.active') }}: <strong>{{ counts.active }}</strong>
      </span>
      <span class="counter-divider">|</span>
      <span class="counter">
        {{ t('myTournaments.upcoming') }}: <strong>{{ counts.upcoming }}</strong>
      </span>
      <span class="counter-divider">|</span>
      <span class="counter">
        {{ t('myTournaments.completed') }}: <strong>{{ counts.completed }}</strong>
      </span>
      <span class="counter-divider">|</span>
      <span class="counter">
        {{ t('myTournaments.cancelled') }}: <strong>{{ counts.cancelled }}</strong>
      </span>
    </div>

    <!-- Tabs -->
    <div class="tabs">
      <button
        :class="['tab', { 'tab-active': activeTab === 'active' }]"
        @click="switchTab('active')"
      >
        {{ t('myTournaments.active') }}
      </button>
      <button
        :class="['tab', { 'tab-active': activeTab === 'upcoming' }]"
        @click="switchTab('upcoming')"
      >
        {{ t('myTournaments.upcoming') }}
      </button>
      <button
        :class="['tab', { 'tab-active': activeTab === 'completed' }]"
        @click="switchTab('completed')"
      >
        {{ t('myTournaments.completed') }}
      </button>
      <button
        :class="['tab', { 'tab-active': activeTab === 'cancelled' }]"
        @click="switchTab('cancelled')"
      >
        {{ t('myTournaments.cancelled') }}
      </button>
    </div>

    <!-- Loading state -->
    <div v-if="loading && contests.length === 0" class="loading-state">
      <div class="spinner" />
      <p>{{ t('myTournaments.loading') }}</p>
    </div>

    <!-- Error state -->
    <div v-else-if="error && contests.length === 0" class="error-state">
      <p>{{ error }}</p>
      <button class="btn btn-primary" @click="loadTournaments(true)">
        {{ t('myTournaments.retry') }}
      </button>
    </div>

    <!-- Contest List -->
    <div v-else class="contests-list">
      <template v-if="contests.length === 0">
        <div class="empty-state">
          <p v-if="activeTab === 'active'">{{ t('myTournaments.noActive') }}</p>
          <p v-else-if="activeTab === 'upcoming'">{{ t('myTournaments.noUpcoming') }}</p>
          <p v-else-if="activeTab === 'completed'">{{ t('myTournaments.noCompleted') }}</p>
          <p v-else>{{ t('myTournaments.noCancelled') }}</p>
        </div>
      </template>
      <template v-else>
        <MyContestCard
          v-for="contest in contests"
          :key="contest.contest_id"
          :contest="contest"
          :type="activeTab"
        />

        <!-- Load more -->
        <div v-if="hasMore" class="load-more">
          <button
            class="btn btn-secondary load-more-btn"
            :disabled="loading"
            @click="loadMore"
          >
            <span v-if="loading" class="spinner spinner-sm" />
            <span v-else>{{ t('myTournaments.loadMore') }}</span>
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.my-tournaments-page {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

.refresh-indicator {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.counters {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  flex-wrap: wrap;
}

.counter strong {
  color: var(--color-text-primary);
}

.counter-divider {
  color: var(--color-border);
}

.tabs {
  display: flex;
  gap: var(--spacing-xs);
  background-color: var(--color-bg-tertiary);
  padding: var(--spacing-xs);
  border-radius: var(--radius-lg);
  width: fit-content;
}

.tab {
  padding: var(--spacing-sm) var(--spacing-lg);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  background: none;
  border: none;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.tab:hover {
  color: var(--color-text-primary);
}

.tab-active {
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  box-shadow: var(--shadow-sm);
}

.contests-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.loading-state,
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.empty-state {
  text-align: center;
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

.spinner {
  width: 24px;
  height: 24px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}

.spinner-sm {
  width: 16px;
  height: 16px;
  border-width: 2px;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.load-more {
  display: flex;
  justify-content: center;
  padding: var(--spacing-md) 0;
}

.load-more-btn {
  min-width: 140px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
}

@media (max-width: 767px) {
  .tabs {
    width: 100%;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }

  .tab {
    flex: 1;
    text-align: center;
    padding: var(--spacing-sm) var(--spacing-sm);
    white-space: nowrap;
  }

  .counters {
    justify-content: center;
  }
}
</style>
