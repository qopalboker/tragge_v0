<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { t } from '@/i18n';
import { api } from '@/api';

interface AutoSchedulingConfig {
  enabled: boolean;
  interval_minutes: number;
  duration_minutes: number;
  asset_classes: string[];
  weekdays_only: boolean;
  active_hours_start: number;
  active_hours_end: number;
  lead_time_minutes: number;
}

interface Contest {
  id: string;
  name: string;
  status: string;
  starts_at: string;
  ends_at: string;
  asset_class: string;
  template_id?: string;
  is_auto_generated?: boolean;
  participant_count?: number;
}

const config = ref<AutoSchedulingConfig | null>(null);
const recentContests = ref<Contest[]>([]);
const upcomingContests = ref<Contest[]>([]);
const loading = ref(true);
const configError = ref(false);
let refreshInterval: ReturnType<typeof setInterval> | null = null;

const availableAssetClasses = ['crypto', 'forex'];

const formattedActiveHours = computed(() => {
  if (!config.value) return '';
  const start = String(config.value.active_hours_start).padStart(2, '0');
  const end = String(config.value.active_hours_end).padStart(2, '0');
  return `${start}:00 - ${end}:00 UTC`;
});

async function fetchConfig(): Promise<void> {
  try {
    // TODO: needs backend endpoint
    // GET /api/admin/auto-scheduling/config
    // Returns the current free-contest-generator configuration from env vars
    const response = await api.get<{ config: AutoSchedulingConfig }>('/api/admin/auto-scheduling/config');
    config.value = response.data.config;
    configError.value = false;
  } catch {
    configError.value = true;
    // Fallback to mock data for development
    config.value = {
      enabled: true,
      interval_minutes: 60,
      duration_minutes: 30,
      asset_classes: ['crypto', 'forex'],
      weekdays_only: true,
      active_hours_start: 8,
      active_hours_end: 22,
      lead_time_minutes: 5,
    };
  }
}

async function fetchRecentContests(): Promise<void> {
  try {
    // TODO: needs backend endpoint filter support
    // GET /api/admin/contests?is_auto_generated=true&limit=10&sort=starts_at&order=desc
    // If is_auto_generated filter is not supported, filter by template_id not null
    const response = await api.get<{ contests: Contest[] }>('/api/admin/contests');
    const all = response.data.contests || [];
    // Filter auto-generated contests (template_id not null or is_auto_generated flag)
    recentContests.value = all
      .filter((c: Contest) => c.is_auto_generated || c.template_id)
      .sort((a: Contest, b: Contest) => new Date(b.starts_at).getTime() - new Date(a.starts_at).getTime())
      .slice(0, 10);
  } catch {
    // silently fail - section will show empty state
    recentContests.value = [];
  }
}

async function fetchUpcomingContests(): Promise<void> {
  try {
    // TODO: needs backend endpoint
    // GET /api/admin/auto-scheduling/upcoming
    // Returns the next scheduled auto-generated contests
    const response = await api.get<{ contests: Contest[] }>('/api/admin/auto-scheduling/upcoming');
    upcomingContests.value = response.data.contests || [];
  } catch {
    // Fallback: derive from config
    upcomingContests.value = [];
  }
}

async function fetchAll(): Promise<void> {
  loading.value = true;
  await Promise.all([fetchConfig(), fetchRecentContests(), fetchUpcomingContests()]);
  loading.value = false;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

function formatTimeDiff(dateStr: string): string {
  const now = new Date().getTime();
  const target = new Date(dateStr).getTime();
  const diff = target - now;
  if (diff < 0) return t('autoScheduling.recent.ended');

  const minutes = Math.floor(diff / 60000);
  if (minutes < 60) return t('contests.time.minutes', { count: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return t('contests.time.hours', { count: hours });
  const days = Math.floor(hours / 24);
  return t('contests.time.days', { count: days });
}

function statusClass(status: string): string {
  const classes: Record<string, string> = {
    draft: 'status-draft',
    scheduled: 'status-scheduled',
    registration_open: 'status-registration',
    running: 'status-running',
    completed: 'status-completed',
    cancelled: 'status-cancelled',
  };
  return classes[status] || '';
}

onMounted(() => {
  fetchAll();
  refreshInterval = setInterval(fetchAll, 60000);
});

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval);
});
</script>

<template>
  <div class="auto-scheduling-page">
    <div class="page-header">
      <h1 class="page-title">{{ t('autoScheduling.title') }}</h1>
      <button class="btn btn-ghost" @click="fetchAll" :disabled="loading">
        {{ t('autoScheduling.refresh') }}
      </button>
    </div>

    <div v-if="loading" class="loading">
      {{ t('common.loading') }}
    </div>

    <template v-else>
      <!-- Info Note -->
      <div class="note-card">
        <svg class="note-icon" width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
          <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
        </svg>
        <span>{{ t('autoScheduling.envNote') }}</span>
      </div>

      <!-- Configuration Cards -->
      <section class="section">
        <h2 class="section-title">{{ t('autoScheduling.configTitle') }}</h2>

        <div v-if="configError" class="config-error-note">
          {{ t('autoScheduling.configLoadError') }}
        </div>

        <div v-if="config" class="config-grid">
          <!-- Enabled -->
          <div class="config-card">
            <div class="config-label">{{ t('autoScheduling.config.enabled') }}</div>
            <div class="config-value">
              <span :class="['status-indicator', config.enabled ? 'indicator-active' : 'indicator-inactive']"></span>
              {{ config.enabled ? t('autoScheduling.config.enabledYes') : t('autoScheduling.config.enabledNo') }}
            </div>
          </div>

          <!-- Interval -->
          <div class="config-card">
            <div class="config-label">{{ t('autoScheduling.config.interval') }}</div>
            <div class="config-value">
              {{ t('autoScheduling.config.everyMinutes', { count: config.interval_minutes }) }}
            </div>
          </div>

          <!-- Duration -->
          <div class="config-card">
            <div class="config-label">{{ t('autoScheduling.config.duration') }}</div>
            <div class="config-value">
              {{ t('autoScheduling.config.minutesPer', { count: config.duration_minutes }) }}
            </div>
          </div>

          <!-- Lead Time -->
          <div class="config-card">
            <div class="config-label">{{ t('autoScheduling.config.leadTime') }}</div>
            <div class="config-value">
              {{ t('autoScheduling.config.leadTimeMinutes', { count: config.lead_time_minutes }) }}
            </div>
          </div>

          <!-- Asset Classes -->
          <div class="config-card config-card-wide">
            <div class="config-label">{{ t('autoScheduling.config.assetClasses') }}</div>
            <div class="config-value asset-classes">
              <span
                v-for="ac in availableAssetClasses"
                :key="ac"
                :class="['asset-class-chip', config.asset_classes.includes(ac) ? 'chip-active' : 'chip-inactive']"
              >
                {{ t(`contestForm.assetClasses.${ac}`) }}
              </span>
            </div>
          </div>

          <!-- Weekdays Only -->
          <div class="config-card">
            <div class="config-label">{{ t('autoScheduling.config.weekdaysOnly') }}</div>
            <div class="config-value">
              <span :class="['status-indicator', config.weekdays_only ? 'indicator-active' : 'indicator-inactive']"></span>
              {{ config.weekdays_only ? t('contestDetail.overview.yes') : t('contestDetail.overview.no') }}
            </div>
          </div>

          <!-- Active Hours -->
          <div class="config-card">
            <div class="config-label">{{ t('autoScheduling.config.activeHours') }}</div>
            <div class="config-value">{{ formattedActiveHours }}</div>
          </div>
        </div>
      </section>

      <!-- Upcoming Schedule -->
      <section class="section">
        <h2 class="section-title">{{ t('autoScheduling.upcoming.title') }}</h2>

        <div v-if="upcomingContests.length === 0" class="empty-state">
          {{ t('autoScheduling.upcoming.empty') }}
        </div>

        <div v-else class="table-container">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ t('contests.name') }}</th>
                <th>{{ t('contestForm.assetClass') }}</th>
                <th>{{ t('contests.startDate') }}</th>
                <th>{{ t('autoScheduling.upcoming.startsIn') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="contest in upcomingContests" :key="contest.id">
                <td>{{ contest.name }}</td>
                <td>
                  <span class="asset-class-chip chip-active">
                    {{ t(`contestForm.assetClasses.${contest.asset_class}`) }}
                  </span>
                </td>
                <td>{{ formatDate(contest.starts_at) }}</td>
                <td>{{ formatTimeDiff(contest.starts_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- Recent Auto-Generated Contests -->
      <section class="section">
        <h2 class="section-title">{{ t('autoScheduling.recent.title') }}</h2>

        <div v-if="recentContests.length === 0" class="empty-state">
          {{ t('autoScheduling.recent.empty') }}
        </div>

        <div v-else class="table-container">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ t('contests.name') }}</th>
                <th>{{ t('contests.status') }}</th>
                <th>{{ t('contestForm.assetClass') }}</th>
                <th>{{ t('contests.startDate') }}</th>
                <th>{{ t('contests.endDate') }}</th>
                <th>{{ t('contests.participants') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="contest in recentContests" :key="contest.id">
                <td>
                  <RouterLink :to="`/admin/contests/${contest.id}/detail`" class="contest-link">
                    {{ contest.name }}
                  </RouterLink>
                </td>
                <td>
                  <span :class="['status-badge', statusClass(contest.status)]">
                    {{ t(`status.${contest.status}`) }}
                  </span>
                </td>
                <td>
                  <span class="asset-class-chip chip-active">
                    {{ t(`contestForm.assetClasses.${contest.asset_class}`) }}
                  </span>
                </td>
                <td>{{ formatDate(contest.starts_at) }}</td>
                <td>{{ formatDate(contest.ends_at) }}</td>
                <td>{{ contest.participant_count ?? '—' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.auto-scheduling-page {
  max-width: 1280px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-lg);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.loading {
  text-align: center;
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

/* Note card */
.note-card {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-md) var(--spacing-lg);
  background-color: var(--color-primary-light);
  color: var(--color-primary);
  border-radius: var(--radius-md);
  margin-bottom: var(--spacing-xl);
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.note-icon {
  flex-shrink: 0;
}

/* Sections */
.section {
  margin-bottom: var(--spacing-xl);
}

.section-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-md);
}

/* Config error note */
.config-error-note {
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-warning-light, #fef3c7);
  color: var(--color-warning, #d97706);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  margin-bottom: var(--spacing-md);
}

/* Config grid */
.config-grid {
  display: grid;
  grid-template-columns: repeat(1, 1fr);
  gap: var(--spacing-md);
}

@media (min-width: 640px) {
  .config-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1024px) {
  .config-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

.config-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
}

.config-card-wide {
  grid-column: span 2;
}

.config-label {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: var(--spacing-sm);
}

.config-value {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

/* Status indicator */
.status-indicator {
  width: 10px;
  height: 10px;
  border-radius: var(--radius-full);
  flex-shrink: 0;
}

.indicator-active {
  background-color: var(--color-success, #10B981);
}

.indicator-inactive {
  background-color: var(--color-text-tertiary, #9CA3AF);
}

/* Asset class chips */
.asset-classes {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
}

.asset-class-chip {
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
}

.chip-active {
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

.chip-inactive {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-tertiary, #9CA3AF);
  text-decoration: line-through;
}

/* Tables */
.table-container {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th {
  padding: var(--spacing-sm) var(--spacing-md);
  text-align: start;
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-bottom: 1px solid var(--color-border);
  background-color: var(--color-bg-secondary);
}

.data-table td {
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  border-bottom: 1px solid var(--color-border);
}

.data-table tr:last-child td {
  border-bottom: none;
}

.data-table tr:hover td {
  background-color: var(--color-bg-secondary);
}

/* Status badges */
.status-badge {
  padding: 2px 8px;
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  white-space: nowrap;
}

.status-draft { background-color: var(--color-bg-tertiary); color: var(--color-text-secondary); }
.status-scheduled { background-color: var(--color-primary-light); color: var(--color-primary); }
.status-registration { background-color: #DBEAFE; color: #2563EB; }
.status-running { background-color: #D1FAE5; color: #059669; }
.status-completed { background-color: #E0E7FF; color: #4338CA; }
.status-cancelled { background-color: #FEE2E2; color: #DC2626; }

/* Contest link */
.contest-link {
  color: var(--color-primary);
  text-decoration: none;
  font-weight: 500;
}

.contest-link:hover {
  text-decoration: underline;
}

/* Empty state */
.empty-state {
  text-align: center;
  padding: var(--spacing-xl);
  color: var(--color-text-secondary);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-lg);
  font-size: var(--font-size-sm);
}
</style>
