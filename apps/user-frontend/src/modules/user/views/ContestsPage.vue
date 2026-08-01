<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useContestsStore, type Contest, type ContestFilters, type DurationType, type MarketType } from '@/stores/contests';
import type { CalendarContest } from '@/api/calendar';
import { useAuthStore } from '@/stores/auth';
import { useToast } from '@/composables/useToast';
import ContestCard from '@/components/contests/ContestCard.vue';
import ContestFiltersComponent from '@/components/contests/ContestFilters.vue';
import JoinConfirmModal from '@/components/contests/JoinConfirmModal.vue';
import MyContestCard from '@/components/contests/MyContestCard.vue';
import ContestCalendar from '@/components/contests/ContestCalendar.vue';
import TournamentListSection from '@/modules/user/components/tournament/TournamentListSection.vue';
import { groupedTournamentsApi, type TournamentGroup } from '@/modules/user/api';
import type { TournamentGroupProps } from '@/modules/user/components/tournament/TournamentGroupCard.vue';

const route = useRoute();
const router = useRouter();
const contestsStore = useContestsStore();
const authStore = useAuthStore();
const toast = useToast();

type ContestTab = 'live' | 'upcoming' | 'all' | 'calendar' | 'my';

const searchQuery = ref('');
const activeTab = ref<ContestTab>('upcoming');
const apiFilters = ref<ContestFilters>({});
const showMobileFilters = ref(false);

// Grouped tournaments (group_by=template)
const groupedTournaments = ref<TournamentGroupProps[]>([]);
const groupedLoading = ref(false);

// Join confirmation modal
const showJoinModal = ref(false);
const selectedContest = ref<Contest | null>(null);

// Map API status to tab
function getTabForStatus(status: Contest['status']): ContestTab {
  if (status === 'running') return 'live';
  if (status === 'registration_open' || status === 'scheduled') return 'upcoming';
  return 'all';
}

const filteredContests = computed(() => {
  // For 'my' tab, show joined contests only (exclude cancelled)
  if (activeTab.value === 'my') {
    return contestsStore.contests.filter((contest) => {
      if (!contestsStore.isJoined(contest.id)) return false;
      if (contest.status === 'cancelled') return false;

      // Search filter
      if (searchQuery.value) {
        const query = searchQuery.value.toLowerCase();
        if (!contest.name.toLowerCase().includes(query)) {
          return false;
        }
      }
      return true;
    });
  }

  return contestsStore.contests.filter((contest) => {
    // Exclude cancelled contests from all active tabs
    if (contest.status === 'cancelled') return false;

    // Search filter (client-side)
    if (searchQuery.value) {
      const query = searchQuery.value.toLowerCase();
      if (!contest.name.toLowerCase().includes(query)) {
        return false;
      }
    }

    // Tab filter (client-side)
    if (activeTab.value !== 'all') {
      const contestTab = getTabForStatus(contest.status);
      if (contestTab !== activeTab.value) {
        return false;
      }
    }

    return true;
  });
});

const liveCount = computed(() =>
  contestsStore.contests.filter(c => c.status === 'running').length
);

const upcomingCount = computed(() =>
  contestsStore.contests.filter(c => c.status === 'registration_open' || c.status === 'scheduled').length
);

const myContestsCount = computed(() =>
  contestsStore.contests.filter(c => contestsStore.isJoined(c.id) && c.status !== 'cancelled').length
);

// My Contests categorized
const myActiveContests = computed(() =>
  contestsStore.userHistory
    .filter(h => h.status === 'running')
    .map(h => {
      const contest = contestsStore.contests.find(c => c.id === h.contest_id);
      return {
        contest_id: h.contest_id,
        contest_name: h.contest_name,
        status: h.status,
        starts_at: contest?.starts_at ?? '',
        ends_at: contest?.ends_at ?? '',
        entry_fee_cents: contest?.entry_fee_cents ?? 0,
        total_score: h.total_score,
        final_rank: h.final_rank,
        total_participants: contest?.participant_count ?? 0,
        is_free: contest ? contest.entry_fee_cents === 0 : false,
        qty_total: contest?.qty_total ?? 0,
      };
    })
);

const myUpcomingContests = computed(() =>
  contestsStore.userHistory
    .filter(h => h.status === 'scheduled' || h.status === 'registration_open')
    .map(h => {
      const contest = contestsStore.contests.find(c => c.id === h.contest_id);
      return {
        contest_id: h.contest_id,
        contest_name: h.contest_name,
        status: h.status,
        starts_at: contest?.starts_at ?? '',
        ends_at: contest?.ends_at ?? '',
        entry_fee_cents: contest?.entry_fee_cents ?? 0,
        total_score: h.total_score,
        total_participants: contest?.participant_count ?? 0,
        is_free: contest ? contest.entry_fee_cents === 0 : false,
        qty_total: contest?.qty_total ?? 0,
      };
    })
);

const myCompletedContests = computed(() =>
  contestsStore.userHistory
    .filter(h => h.status === 'completed')
    .slice(0, 5)
    .map(h => {
      const contest = contestsStore.contests.find(c => c.id === h.contest_id);
      return {
        contest_id: h.contest_id,
        contest_name: h.contest_name,
        status: h.status,
        starts_at: contest?.starts_at ?? '',
        ends_at: contest?.ends_at ?? '',
        entry_fee_cents: contest?.entry_fee_cents ?? 0,
        total_score: h.total_score,
        final_rank: h.final_rank,
        final_prize_cents: h.final_prize_cents,
        total_participants: contest?.participant_count ?? 0,
        is_free: contest ? contest.entry_fee_cents === 0 : false,
        qty_total: contest?.qty_total ?? 0,
      };
    })
);

const activeFilterCount = computed(() => {
  let count = 0;
  if (apiFilters.value.market_type) count++;
  if (apiFilters.value.duration_type) count++;
  if (apiFilters.value.is_free) count++;
  if (apiFilters.value.min_entry || apiFilters.value.max_entry) count++;
  return count;
});

function resetFilters(): void {
  apiFilters.value = {};
  searchQuery.value = '';
  updateUrlParams();
  loadContests();
}

async function loadContests(): Promise<void> {
  try {
    await contestsStore.fetchContests(apiFilters.value);
    // Also fetch user history to know which contests they've joined
    await contestsStore.fetchUserHistory();
  } catch {
    toast.error(t('contests.loadError'));
  }
}

async function loadGroupedTournaments(): Promise<void> {
  groupedLoading.value = true;
  try {
    const data = await groupedTournamentsApi.getGroupedTournaments({
      status: 'registration_open',
    });
    groupedTournaments.value = (data.groups || []).map((g: TournamentGroup): TournamentGroupProps => ({
      templateId: g.template_id,
      name: g.name,
      type: (g.market_type === 'crypto' ? 'Crypto' : 'Forex') as 'Forex' | 'Crypto',
      duration: g.duration_type === 'weekly' ? 'Weekly' : g.duration_minutes <= 30 ? '30 minutes' : 'Hourly',
      startDate: g.start_time.iso,
      endDate: g.end_time.iso,
      tiers: g.tiers.map((t) => ({
        contestId: t.contest_id,
        entryFeeCents: t.entry_fee_cents,
        tierLabel: t.tier_label,
        isFree: t.is_free,
        prizePoolCents: t.prize_pool_cents,
        currentParticipants: t.current_participants,
        maxParticipants: t.max_participants,
      })),
    }));
  } catch {
    groupedTournaments.value = [];
  } finally {
    groupedLoading.value = false;
  }
}

// Handle filter changes from ContestFilters component
function handleFilterChange(filters: ContestFilters): void {
  apiFilters.value = filters;
  updateUrlParams();
  loadContests();
}

// Update URL query params for shareable filtered views
function updateUrlParams(): void {
  const query: Record<string, string> = {};

  if (activeTab.value !== 'upcoming') {
    query.tab = activeTab.value;
  }

  if (apiFilters.value.market_type) {
    query.market = apiFilters.value.market_type;
  }
  if (apiFilters.value.duration_type) {
    query.duration = apiFilters.value.duration_type;
  }
  if (apiFilters.value.is_free) {
    query.free = 'true';
  }
  if (apiFilters.value.min_entry) {
    query.min = String(apiFilters.value.min_entry);
  }
  if (apiFilters.value.max_entry) {
    query.max = String(apiFilters.value.max_entry);
  }

  router.replace({ query });
}

// Parse URL query params on mount
function parseUrlParams(): void {
  const { market, duration, free, min, max, tab } = route.query;

  if (tab && typeof tab === 'string') {
    const validTabs: ContestTab[] = ['live', 'upcoming', 'all', 'calendar', 'my'];
    if (validTabs.includes(tab as ContestTab)) {
      activeTab.value = tab as ContestTab;
    }
  }

  if (market && typeof market === 'string') {
    const validMarkets: MarketType[] = ['crypto', 'forex', 'stocks', 'mixed'];
    if (validMarkets.includes(market as MarketType)) {
      apiFilters.value.market_type = market as MarketType;
    }
  }
  if (duration && typeof duration === 'string') {
    const validTypes: DurationType[] = ['rush_30min', 'hourly', 'four_hour', 'daily', 'weekly'];
    if (validTypes.includes(duration as DurationType)) {
      apiFilters.value.duration_type = duration as DurationType;
    }
  }
  if (free === 'true') {
    apiFilters.value.is_free = true;
  }
  if (min && typeof min === 'string') {
    const minVal = parseInt(min, 10);
    if (!isNaN(minVal)) apiFilters.value.min_entry = minVal;
  }
  if (max && typeof max === 'string') {
    const maxVal = parseInt(max, 10);
    if (!isNaN(maxVal)) apiFilters.value.max_entry = maxVal;
  }
}

// Handle join click from ContestCard or ContestCalendar
function handleJoinClick(contest: Contest | CalendarContest): void {
  if ('entry_fee_cents' in contest) {
    selectedContest.value = contest;
    showJoinModal.value = true;
  } else {
    // CalendarContest lacks full details; navigate to contest details page
    router.push(`/user/contests/${contest.id}`);
  }
}

// Handle successful join
function handleJoined(): void {
  selectedContest.value = null;
  showJoinModal.value = false;
  // Refresh contests to update participant counts
  loadContests();
}

// Toggle mobile filters
function toggleMobileFilters(): void {
  showMobileFilters.value = !showMobileFilters.value;
}

// Watch for route changes
watch(
  () => route.query,
  () => {
    parseUrlParams();
  },
  { deep: true }
);

onMounted(() => {
  parseUrlParams();
  loadContests();
  if (activeTab.value === 'upcoming') {
    loadGroupedTournaments();
  }
});

// Load grouped tournaments when switching to the upcoming tab
watch(activeTab, (tab) => {
  if (tab === 'upcoming') {
    loadGroupedTournaments();
  }
});
</script>

<template>
  <div class="contests-page">
    <!-- Search & Filters Bar -->
    <div class="toolbar">
      <div class="search-box">
        <svg class="search-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="8" />
          <path d="M21 21l-4.35-4.35" />
        </svg>
        <input
          v-model="searchQuery"
          type="text"
          class="search-input"
          :placeholder="t('contests.search')"
        />
      </div>

      <!-- Mobile Filter Toggle -->
      <button class="filter-toggle-btn show-mobile" @click="toggleMobileFilters">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6" />
        </svg>
        <span v-if="activeFilterCount > 0" class="filter-toggle-badge">{{ activeFilterCount }}</span>
      </button>
    </div>

    <!-- Mobile Filters (Expandable) -->
    <div v-if="showMobileFilters" class="mobile-filters show-mobile">
      <ContestFiltersComponent
        v-model="apiFilters"
        @filter="handleFilterChange"
      />
    </div>

    <!-- Status Tabs -->
    <div class="status-tabs">
      <button
        :class="['tab', { 'tab-active': activeTab === 'live' }]"
        @click="activeTab = 'live'; updateUrlParams()"
      >
        {{ t('contests.live') }}
        <span v-if="liveCount > 0" class="tab-count tab-count-live">{{ liveCount }}</span>
      </button>
      <button
        :class="['tab', { 'tab-active': activeTab === 'upcoming' }]"
        @click="activeTab = 'upcoming'; updateUrlParams()"
      >
        {{ t('contests.upcoming') }}
        <span v-if="upcomingCount > 0" class="tab-count">{{ upcomingCount }}</span>
      </button>
      <button
        :class="['tab', { 'tab-active': activeTab === 'all' }]"
        @click="activeTab = 'all'; updateUrlParams()"
      >
        {{ t('contests.all') }}
      </button>
      <button
        :class="['tab', { 'tab-active': activeTab === 'calendar' }]"
        @click="activeTab = 'calendar'; updateUrlParams()"
      >
        <span class="tab-label-desktop">📅 {{ t('contests.calendar') }}</span>
        <span class="tab-label-mobile">📅</span>
      </button>
      <button
        v-if="authStore.isAuthenticated"
        :class="['tab', { 'tab-active': activeTab === 'my' }]"
        @click="activeTab = 'my'; updateUrlParams()"
      >
        {{ t('contests.myContests') }}
        <span v-if="myContestsCount > 0" class="tab-count">{{ myContestsCount }}</span>
      </button>
    </div>

    <!-- Calendar View -->
    <div v-if="activeTab === 'calendar'" class="calendar-container">
      <ContestCalendar @join-click="handleJoinClick" />
    </div>

    <!-- Content Area -->
    <div v-else class="content-area">
      <!-- Filters Sidebar (Desktop) -->
      <aside class="filters-sidebar hide-mobile">
        <ContestFiltersComponent
          v-model="apiFilters"
          @filter="handleFilterChange"
        />
      </aside>

      <!-- Contests Grid -->
      <div class="contests-container">
        <!-- Loading State -->
        <div v-if="contestsStore.loading" class="loading-state">
          <div class="loading-spinner"></div>
          <p>{{ t('common.loading') }}</p>
        </div>

        <!-- Error State -->
        <div v-else-if="contestsStore.error" class="error-state card">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <circle cx="12" cy="12" r="10" />
            <path d="M12 8v4" />
            <path d="M12 16h.01" />
          </svg>
          <p>{{ contestsStore.error }}</p>
          <button class="btn btn-primary" @click="loadContests">
            {{ t('common.retry') }}
          </button>
        </div>

        <!-- Empty State -->
        <div v-else-if="filteredContests.length === 0" class="empty-state">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M21 21l-4.35-4.35M11 6V4M11 20v-2M4 11H2M20 11h2M6.5 6.5L5 5M16.5 6.5L18 5M6.5 16.5L5 18M16.5 16.5L18 18" />
            <circle cx="11" cy="11" r="4" />
          </svg>
          <p>{{ t('contests.noResults') }}</p>
          <button v-if="searchQuery || activeFilterCount > 0" class="btn btn-secondary" @click="resetFilters">
            {{ t('contests.clearFilters') }}
          </button>
        </div>

        <!-- Grouped Tournaments (upcoming tab) -->
        <template v-else-if="activeTab === 'upcoming' && groupedTournaments.length > 0">
          <TournamentListSection
            section="next"
            :tournaments="[]"
            :grouped-tournaments="groupedTournaments"
            :loading="groupedLoading"
            @join-grouped="(contestId: string) => router.push(`/user/contests/${contestId}`)"
          />

          <!-- Also show ungrouped contests below -->
          <div v-if="filteredContests.length > 0" class="contests-grid" style="margin-top: var(--spacing-lg);">
            <ContestCard
              v-for="contest in filteredContests"
              :key="contest.id"
              :contest="contest"
              @join-click="handleJoinClick"
              @joined="handleJoined"
            />
          </div>
        </template>

        <!-- Contests Grid -->
        <div v-else class="contests-grid">
          <ContestCard
            v-for="contest in filteredContests"
            :key="contest.id"
            :contest="contest"
            @join-click="handleJoinClick"
            @joined="handleJoined"
          />
        </div>

        <!-- My Contests Detailed View (when My tab selected) -->
        <template v-if="activeTab === 'my' && authStore.isAuthenticated">
          <!-- Active Contests -->
          <div v-if="myActiveContests.length > 0" class="my-contests-section">
            <h3 class="section-title">{{ t('myTournaments.active') }}</h3>
            <div class="my-contests-list">
              <MyContestCard
                v-for="contest in myActiveContests"
                :key="contest.contest_id"
                :contest="contest"
                type="active"
              />
            </div>
          </div>

          <!-- Upcoming Contests -->
          <div v-if="myUpcomingContests.length > 0" class="my-contests-section">
            <h3 class="section-title">{{ t('myTournaments.upcoming') }}</h3>
            <div class="my-contests-list">
              <MyContestCard
                v-for="contest in myUpcomingContests"
                :key="contest.contest_id"
                :contest="contest"
                type="upcoming"
              />
            </div>
          </div>

          <!-- Completed Contests -->
          <div v-if="myCompletedContests.length > 0" class="my-contests-section">
            <h3 class="section-title">{{ t('myTournaments.completed') }}</h3>
            <div class="my-contests-list">
              <MyContestCard
                v-for="contest in myCompletedContests"
                :key="contest.contest_id"
                :contest="contest"
                type="completed"
              />
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- Join Confirmation Modal -->
    <JoinConfirmModal
      v-if="selectedContest"
      :contest="selectedContest"
      :show="showJoinModal"
      @update:show="showJoinModal = $event"
      @joined="handleJoined"
    />
  </div>
</template>

<style scoped>
.contests-page {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

.toolbar {
  display: flex;
  gap: var(--spacing-md);
}

.search-box {
  flex: 1;
  position: relative;
  max-width: 400px;
}

.search-icon {
  position: absolute;
  left: var(--spacing-md);
  top: 50%;
  transform: translateY(-50%);
  color: var(--color-text-muted);
}

[dir="rtl"] .search-icon {
  left: auto;
  right: var(--spacing-md);
}

.search-input {
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  padding-left: 44px;
  font-size: var(--font-size-sm);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-primary);
}

[dir="rtl"] .search-input {
  padding-left: var(--spacing-md);
  padding-right: 44px;
}

.search-input:focus {
  outline: none;
  border-color: var(--color-primary);
}

.status-tabs {
  display: flex;
  gap: var(--spacing-xs);
  background-color: var(--color-bg-tertiary);
  padding: var(--spacing-xs);
  border-radius: var(--radius-lg);
  width: fit-content;
}

.tab {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
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

.tab-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 var(--spacing-xs);
  font-size: var(--font-size-xs);
  font-weight: 600;
  background-color: var(--color-primary);
  color: white;
  border-radius: var(--radius-full);
}

.tab-count-live {
  background-color: var(--color-danger, #DC2626);
  animation: pulseLive 2s ease-in-out infinite;
}

@keyframes pulseLive {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.7;
  }
}

.content-area {
  display: flex;
  gap: var(--spacing-lg);
}

.filters-sidebar {
  width: 280px;
  flex-shrink: 0;
  height: fit-content;
  position: sticky;
  top: calc(var(--header-height) + var(--spacing-lg));
}

/* Mobile Filter Toggle */
.filter-toggle-btn {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  padding: 0;
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.filter-toggle-btn:hover {
  border-color: var(--color-primary);
}

.filter-toggle-badge {
  position: absolute;
  top: -4px;
  right: -4px;
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 4px;
  font-size: 11px;
  font-weight: 600;
  background-color: var(--color-primary);
  color: white;
  border-radius: var(--radius-full);
}

[dir="rtl"] .filter-toggle-badge {
  right: auto;
  left: -4px;
}

/* Mobile Filters Container */
.mobile-filters {
  animation: slideDown 0.2s ease-out;
}

@keyframes slideDown {
  from {
    opacity: 0;
    transform: translateY(-8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* Show/Hide Utilities */
.show-mobile {
  display: none;
}

.hide-mobile {
  display: block;
}

.contests-container {
  flex: 1;
  min-height: 200px;
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.loading-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  text-align: center;
}

.error-state svg {
  color: var(--color-danger);
}

.error-state p {
  color: var(--color-text-secondary);
}

.contests-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--spacing-md);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  text-align: center;
  color: var(--color-text-secondary);
}

.empty-state svg {
  color: var(--color-text-muted);
}

@media (max-width: 1023px) {
  .filters-sidebar {
    display: none;
  }

  .show-mobile {
    display: flex;
  }

  .hide-mobile {
    display: none;
  }

  .mobile-filters {
    display: block;
  }
}

@media (max-width: 767px) {
  .status-tabs {
    width: 100%;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: none;
    -ms-overflow-style: none;
  }

  .status-tabs::-webkit-scrollbar {
    display: none;
  }

  .tab {
    flex: 1;
    justify-content: center;
    white-space: nowrap;
  }

  .tab-label-desktop {
    display: none;
  }

  .tab-label-mobile {
    display: inline;
  }

  .contests-grid {
    grid-template-columns: 1fr;
  }
}

/* Calendar Container */
.calendar-container {
  flex: 1;
}

/* Tab label: desktop shows full text, mobile shows short/icon */
.tab-label-mobile {
  display: none;
}

/* My Contests Section */
.my-contests-section {
  margin-top: var(--spacing-lg);
}

.section-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-md);
  padding-bottom: var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
}

.my-contests-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}
</style>
