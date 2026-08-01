<script setup lang="ts">
import { ref, computed } from 'vue';
import { t } from '@/i18n';
import { useMediaQuery } from '@vueuse/core';
import TournamentCard from './TournamentCard.vue';
import TournamentGroupCard from './TournamentGroupCard.vue';
import TournamentTable from './TournamentTable.vue';
import type { TournamentCardProps } from './TournamentCard.vue';
import type { TournamentGroupProps } from './TournamentGroupCard.vue';

export type SectionType = 'next' | 'free' | 'my';
export type MyTab = 'in_progress' | 'pending' | 'finished' | 'cancelled';

const props = defineProps<{
  /** Section type determines header and behavior */
  section: SectionType;
  /** Tournaments to display */
  tournaments: TournamentCardProps[];
  /** Grouped tournaments (contests sharing same template with multiple tiers) */
  groupedTournaments?: TournamentGroupProps[];
  /** Link for "All tournaments >" */
  allTournamentsLink?: string;
  /** Show loading state */
  loading?: boolean;
}>();

const emit = defineEmits<{
  join: [id: string];
  joinGrouped: [contestId: string, templateId: string];
  tabChange: [tab: MyTab];
  filterChange: [filter: string];
}>();

const isMobile = useMediaQuery('(max-width: 768px)');

// Section-specific state
const activeFilter = ref<'hot' | 'all'>('all');
const activeMyTab = ref<MyTab>('in_progress');

// Section title
const sectionTitle = computed(() => {
  switch (props.section) {
    case 'next': return t('tournament.sectionNext');
    case 'free': return t('tournament.sectionFree');
    case 'my': return t('tournament.sectionMy');
    default: return '';
  }
});

// Section subtitle
const sectionSubtitle = computed(() => {
  if (props.section === 'next') {
    return t('tournament.sectionNextSubtitle');
  }
  return '';
});

// My Tournaments tabs
const myTabs: { key: MyTab; labelKey: string }[] = [
  { key: 'in_progress', labelKey: 'tournament.tabInProgress' },
  { key: 'pending', labelKey: 'tournament.tabPending' },
  { key: 'finished', labelKey: 'tournament.tabFinished' },
  { key: 'cancelled', labelKey: 'tournament.tabCancelled' },
];

// Filtered tournaments for next section
const displayedTournaments = computed(() => {
  if (props.section === 'next' && activeFilter.value === 'hot') {
    return props.tournaments.filter(item => item.isHot);
  }
  return props.tournaments;
});

// Table variant
const tableVariant = computed(() => {
  if (props.section === 'my' && activeMyTab.value === 'finished') {
    return 'finished' as const;
  }
  return 'active' as const;
});

function setFilter(filter: 'hot' | 'all'): void {
  activeFilter.value = filter;
  emit('filterChange', filter);
}

function setMyTab(tab: MyTab): void {
  activeMyTab.value = tab;
  emit('tabChange', tab);
}

const hasAnyItems = computed(() => {
  return displayedTournaments.value.length > 0 || (props.groupedTournaments?.length ?? 0) > 0;
});

function handleJoin(id: string): void {
  emit('join', id);
}

function handleJoinGrouped(contestId: string, templateId: string): void {
  emit('joinGrouped', contestId, templateId);
}
</script>

<template>
  <section class="t-section">
    <!-- Section Header -->
    <div class="t-section__header">
      <div class="t-section__title-group">
        <h2 class="t-section__title">{{ sectionTitle }}</h2>
        <p v-if="sectionSubtitle" class="t-section__subtitle">{{ sectionSubtitle }}</p>
      </div>

      <!-- Next Tournaments: Hot/All filter + link -->
      <div v-if="section === 'next'" class="t-section__actions">
        <div class="t-section__filters">
          <button
            :class="['t-section__filter-btn', { 't-section__filter-btn--active': activeFilter === 'hot' }]"
            @click="setFilter('hot')"
          >
            &#128293; {{ t('tournament.filterHot') }}
          </button>
          <button
            :class="['t-section__filter-btn', { 't-section__filter-btn--active': activeFilter === 'all' }]"
            @click="setFilter('all')"
          >
            {{ t('tournament.filterAll') }}
          </button>
        </div>
        <a v-if="allTournamentsLink" :href="allTournamentsLink" class="t-section__link">
          {{ t('tournament.allTournaments') }} &rsaquo;
        </a>
      </div>

      <!-- My Tournaments: Tabs -->
      <div v-if="section === 'my'" class="t-section__tabs">
        <button
          v-for="tab in myTabs"
          :key="tab.key"
          :class="['t-section__tab', { 't-section__tab--active': activeMyTab === tab.key }]"
          @click="setMyTab(tab.key)"
        >
          {{ t(tab.labelKey) }}
        </button>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="t-section__loading">
      <div class="t-section__spinner"></div>
    </div>

    <!-- Empty State -->
    <div v-else-if="!hasAnyItems" class="t-section__empty">
      <span class="t-section__empty-text">{{ t('tournament.noTournaments') }}</span>
    </div>

    <!-- Content -->
    <template v-else>
      <!-- Desktop: Table View (ungrouped only) -->
      <TournamentTable
        v-if="!isMobile && displayedTournaments.length > 0"
        :tournaments="displayedTournaments"
        :variant="tableVariant"
        @join="handleJoin"
      />

      <!-- Grouped tournament cards (both mobile and desktop) -->
      <div v-if="groupedTournaments && groupedTournaments.length > 0" class="t-section__cards">
        <TournamentGroupCard
          v-for="group in groupedTournaments"
          :key="group.templateId + '-' + group.startDate"
          v-bind="group"
          @join="handleJoinGrouped"
        />
      </div>

      <!-- Mobile: Card View (ungrouped) -->
      <div v-if="isMobile && displayedTournaments.length > 0" class="t-section__cards">
        <TournamentCard
          v-for="item in displayedTournaments"
          :key="item.id"
          v-bind="item"
          @join="handleJoin"
        />
      </div>
    </template>
  </section>
</template>

<style scoped>
.t-section {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* Header */
.t-section__header {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.t-section__title-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.t-section__title {
  font-size: 18px;
  font-weight: 700;
  color: #e8eaed;
  margin: 0;
}

.t-section__subtitle {
  font-size: 12px;
  color: #5a6a7a;
  margin: 0;
  line-height: 1.4;
}

/* Actions row (filters + link) */
.t-section__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

/* Filters */
.t-section__filters {
  display: flex;
  gap: 8px;
}

.t-section__filter-btn {
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  color: #5a6a7a;
  background-color: transparent;
  border: 1px solid #1a2538;
  border-radius: 6px;
  cursor: pointer;
  transition: all 150ms ease;
}

.t-section__filter-btn:hover {
  color: #e8eaed;
  border-color: #2a3548;
}

.t-section__filter-btn--active {
  color: #e8eaed;
  background-color: #1a2538;
  border-color: #2a3548;
}

/* All tournaments link */
.t-section__link {
  font-size: 13px;
  font-weight: 500;
  color: #00e5c3;
  text-decoration: none;
  white-space: nowrap;
  transition: opacity 150ms ease;
}

.t-section__link:hover {
  opacity: 0.8;
}

/* Tabs */
.t-section__tabs {
  display: flex;
  gap: 4px;
  background-color: #0b1019;
  border-radius: 8px;
  padding: 4px;
  overflow-x: auto;
}

.t-section__tab {
  padding: 8px 16px;
  font-size: 13px;
  font-weight: 500;
  color: #5a6a7a;
  background-color: transparent;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: all 150ms ease;
  white-space: nowrap;
}

.t-section__tab:hover {
  color: #e8eaed;
}

.t-section__tab--active {
  color: #e8eaed;
  background-color: #1a2538;
}

/* Loading */
.t-section__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 0;
}

.t-section__spinner {
  width: 28px;
  height: 28px;
  border: 3px solid #1a2538;
  border-top-color: #00e5c3;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Empty */
.t-section__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 16px;
  background-color: #0f1923;
  border: 1px solid #1a2538;
  border-radius: 8px;
}

.t-section__empty-text {
  font-size: 14px;
  color: #5a6a7a;
}

/* Cards grid (mobile) */
.t-section__cards {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* Responsive */
@media (max-width: 768px) {
  .t-section__header {
    gap: 8px;
  }

  .t-section__title {
    font-size: 16px;
  }

  .t-section__actions {
    flex-direction: column;
    align-items: flex-start;
  }

  .t-section__tabs {
    width: 100%;
  }
}

/* RTL support */
[dir="rtl"] .t-section__actions {
  flex-direction: row-reverse;
}

[dir="rtl"] .t-section__link {
  direction: rtl;
}
</style>
