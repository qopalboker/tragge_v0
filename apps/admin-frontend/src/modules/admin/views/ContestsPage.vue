<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { api } from '@/api';
import { getErrorMessage } from '@/utils/errorHandler';

const toast = useToast();

interface Contest {
  id: string;
  name: string;
  status: string;
  participant_count: number;
  min_participants?: number;
  max_participants?: number | null;
  start_time: string;
  end_time: string;
  is_free?: boolean;
  entry_fee_cents?: number;
  asset_class?: string;
  auto_generated?: boolean;
}

type ContestAction = 'publish' | 'openRegistration' | 'closeRegistration' | 'start' | 'end' | 'pause' | 'resume' | 'cancel';

interface ActionDef {
  key: ContestAction;
  label: string;
  colorClass: string;
  endpoint: string;
}

type SortField = 'status' | 'start_time' | 'participant_count';
type SortDirection = 'asc' | 'desc';

const router = useRouter();

type ViewTab = 'active' | 'archive';

const contests = ref<Contest[]>([]);
const loading = ref(true);
const error = ref<string | null>(null);
const searchQuery = ref('');
const statusFilter = ref('');
const typeFilter = ref<'' | 'free' | 'paid'>('');
const activeViewTab = ref<ViewTab>('active');
const sortBy = ref<SortField>('start_time');
const sortDirection = ref<SortDirection>('desc');

// Auto-refresh
let refreshInterval: ReturnType<typeof setInterval> | null = null;

// Dropdown state
const openDropdownId = ref<string | null>(null);

// Modal state
const showModal = ref(false);
const modalAction = ref<ContestAction | null>(null);
const modalContest = ref<Contest | null>(null);
const modalLoading = ref(false);
const cancelReason = ref('');

const statuses = ['draft', 'scheduled', 'registration_open', 'registration_closed', 'running', 'paused', 'settling', 'completed', 'cancelled'];

// Status sort order for sorting by status
const statusOrder: Record<string, number> = {
  running: 0,
  paused: 1,
  registration_open: 2,
  registration_closed: 3,
  scheduled: 4,
  draft: 5,
  settling: 6,
  completed: 7,
  cancelled: 8,
};

// Map status to available actions
const statusActions: Record<string, ActionDef[]> = {
  draft: [
    { key: 'publish', label: 'contests.action.publish', colorClass: 'action-green', endpoint: 'publish' },
    { key: 'cancel', label: 'contests.action.cancel', colorClass: 'action-red', endpoint: 'cancel' },
  ],
  scheduled: [
    { key: 'openRegistration', label: 'contests.action.openRegistration', colorClass: 'action-blue', endpoint: 'open-registration' },
    { key: 'closeRegistration', label: 'contests.action.closeRegistration', colorClass: 'action-orange', endpoint: 'close-registration' },
    { key: 'cancel', label: 'contests.action.cancel', colorClass: 'action-red', endpoint: 'cancel' },
  ],
  registration_open: [
    { key: 'closeRegistration', label: 'contests.action.closeRegistration', colorClass: 'action-orange', endpoint: 'close-registration' },
    { key: 'cancel', label: 'contests.action.cancel', colorClass: 'action-red', endpoint: 'cancel' },
  ],
  registration_closed: [
    { key: 'start', label: 'contests.action.start', colorClass: 'action-green', endpoint: 'start' },
    { key: 'cancel', label: 'contests.action.cancel', colorClass: 'action-red', endpoint: 'cancel' },
  ],
  running: [
    { key: 'end', label: 'contests.action.end', colorClass: 'action-orange', endpoint: 'end' },
    { key: 'pause', label: 'contests.action.pause', colorClass: 'action-yellow', endpoint: 'pause' },
  ],
  paused: [
    { key: 'resume', label: 'contests.action.resume', colorClass: 'action-green', endpoint: 'resume' },
    { key: 'end', label: 'contests.action.end', colorClass: 'action-orange', endpoint: 'end' },
  ],
};

function getActionsForStatus(status: string): ActionDef[] {
  return statusActions[status] || [];
}

function formatParticipants(contest: Contest): string {
  const count = contest.participant_count ?? 0;
  const max = contest.max_participants;
  if (max != null && max > 0) {
    return `${count}/${max}`;
  }
  return `${count}/\u221E`;
}

function getTimeInfo(contest: Contest): string {
  const now = Date.now();
  const status = contest.status;

  if (status === 'scheduled' || status === 'registration_open') {
    const startTime = new Date(contest.start_time).getTime();
    const diff = startTime - now;
    if (diff <= 0) return t('contests.time.startingSoon');
    return t('contests.time.startsIn', { time: formatTimeDiff(diff) });
  }

  if (status === 'running') {
    const endTime = new Date(contest.end_time).getTime();
    const diff = endTime - now;
    if (diff <= 0) return t('contests.time.endingSoon');
    return t('contests.time.endsIn', { time: formatTimeDiff(diff) });
  }

  if (status === 'completed') {
    const endTime = new Date(contest.end_time).getTime();
    const diff = now - endTime;
    if (diff < 0) return '';
    return t('contests.time.endedAgo', { time: formatTimeDiff(diff) });
  }

  return '';
}

function formatTimeDiff(ms: number): string {
  const minutes = Math.floor(ms / 60000);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (days > 0) {
    return days === 1 ? t('contests.time.oneDay') : t('contests.time.days', { count: String(days) });
  }
  if (hours > 0) {
    return hours === 1 ? t('contests.time.oneHour') : t('contests.time.hours', { count: String(hours) });
  }
  return minutes <= 1 ? t('contests.time.oneMinute') : t('contests.time.minutes', { count: String(minutes) });
}

function setSortBy(field: SortField): void {
  if (sortBy.value === field) {
    sortDirection.value = sortDirection.value === 'asc' ? 'desc' : 'asc';
  } else {
    sortBy.value = field;
    sortDirection.value = field === 'participant_count' ? 'desc' : 'asc';
  }
}

function getSortIcon(field: SortField): string {
  if (sortBy.value !== field) return '\u2195';
  return sortDirection.value === 'asc' ? '\u2191' : '\u2193';
}

const filteredContests = computed(() => {
  let result = contests.value;

  // Filter by view tab: active shows non-cancelled/non-draft, archive shows cancelled only
  if (activeViewTab.value === 'archive') {
    result = result.filter(c => c.status === 'cancelled');
  } else {
    result = result.filter(c => c.status !== 'cancelled' && c.status !== 'draft');
  }

  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase();
    result = result.filter(c => c.name.toLowerCase().includes(query));
  }

  if (statusFilter.value) {
    result = result.filter(c => c.status === statusFilter.value);
  }

  if (typeFilter.value === 'free') {
    result = result.filter(c => c.is_free === true);
  } else if (typeFilter.value === 'paid') {
    result = result.filter(c => c.is_free !== true);
  }

  // Sort
  const dir = sortDirection.value === 'asc' ? 1 : -1;
  result = [...result].sort((a, b) => {
    switch (sortBy.value) {
      case 'status': {
        const aOrder = statusOrder[a.status] ?? 99;
        const bOrder = statusOrder[b.status] ?? 99;
        return (aOrder - bOrder) * dir;
      }
      case 'start_time': {
        const aTime = new Date(a.start_time).getTime();
        const bTime = new Date(b.start_time).getTime();
        return (aTime - bTime) * dir;
      }
      case 'participant_count':
        return ((a.participant_count ?? 0) - (b.participant_count ?? 0)) * dir;
      default:
        return 0;
    }
  });

  return result;
});

function getContestsFetchParams(): Record<string, string> {
  if (activeViewTab.value === 'archive') {
    return { status: 'cancelled' };
  }
  return {};
}

async function fetchContests(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    const response = await api.get<{ contests: Contest[] }>('/api/admin/contests', {
      params: getContestsFetchParams(),
    });
    contests.value = response.data.contests || [];
  } catch {
    error.value = t('common.error');
    contests.value = [];
  } finally {
    loading.value = false;
  }
}

async function silentRefresh(): Promise<void> {
  try {
    const response = await api.get<{ contests: Contest[] }>('/api/admin/contests', {
      params: getContestsFetchParams(),
    });
    contests.value = response.data.contests || [];
  } catch {
    // Silent refresh — don't overwrite error state
  }
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString();
}

function getStatusClass(status: string): string {
  const classes: Record<string, string> = {
    running: 'status-running',
    scheduled: 'status-scheduled',
    registration_open: 'status-registration',
    registration_closed: 'status-registration-closed',
    completed: 'status-completed',
    draft: 'status-draft',
    paused: 'status-paused',
    cancelled: 'status-cancelled',
  };
  return classes[status] || 'status-default';
}

function viewContestDetail(id: string): void {
  router.push({ name: 'admin-contest-detail', params: { id } });
}

function editContest(id: string): void {
  router.push({ name: 'admin-contest-edit', params: { id } });
}

function createContest(): void {
  router.push({ name: 'admin-contest-new' });
}

function switchViewTab(tab: ViewTab): void {
  if (activeViewTab.value === tab) return;
  activeViewTab.value = tab;
  statusFilter.value = '';
  typeFilter.value = '';
  fetchContests();
}

async function deleteContest(id: string): Promise<void> {
  if (!confirm(t('contests.confirmDelete'))) return;

  try {
    await api.delete(`/api/admin/contests/${id}`);
    contests.value = contests.value.filter(c => c.id !== id);
    toast.success(t('contests.deleteSuccess'));
  } catch {
    toast.error(t('contests.deleteError'));
  }
}

// Dropdown management
function toggleDropdown(contestId: string): void {
  if (openDropdownId.value === contestId) {
    openDropdownId.value = null;
  } else {
    openDropdownId.value = contestId;
  }
}

function closeDropdowns(): void {
  openDropdownId.value = null;
}

function onClickOutside(event: MouseEvent): void {
  const target = event.target as HTMLElement;
  if (!target.closest('.actions-dropdown')) {
    closeDropdowns();
  }
}

// Auto-refresh with visibility API
function handleVisibilityChange(): void {
  if (document.hidden) {
    stopAutoRefresh();
  } else {
    startAutoRefresh();
    silentRefresh();
  }
}

function startAutoRefresh(): void {
  stopAutoRefresh();
  refreshInterval = setInterval(silentRefresh, 30000);
}

function stopAutoRefresh(): void {
  if (refreshInterval !== null) {
    clearInterval(refreshInterval);
    refreshInterval = null;
  }
}

onMounted(() => {
  fetchContests();
  document.addEventListener('click', onClickOutside);
  document.addEventListener('visibilitychange', handleVisibilityChange);
  startAutoRefresh();
});

onUnmounted(() => {
  document.removeEventListener('click', onClickOutside);
  document.removeEventListener('visibilitychange', handleVisibilityChange);
  stopAutoRefresh();
});

// Modal management
function getModalTitle(action: ContestAction): string {
  return t(`contests.confirm.${action}Title`);
}

function getModalMessage(action: ContestAction, contest: Contest): string {
  if (action === 'start') {
    return t('contests.confirm.startMessage', { count: String(contest.participant_count) });
  }
  return t(`contests.confirm.${action}Message`);
}

function openActionModal(action: ContestAction, contest: Contest): void {
  modalAction.value = action;
  modalContest.value = contest;
  cancelReason.value = '';
  showModal.value = true;
  closeDropdowns();
}

function closeModal(): void {
  showModal.value = false;
  modalAction.value = null;
  modalContest.value = null;
  modalLoading.value = false;
  cancelReason.value = '';
}

async function confirmAction(): Promise<void> {
  if (!modalAction.value || !modalContest.value) return;

  const action = modalAction.value;
  const contest = modalContest.value;
  const actionDef = getActionsForStatus(contest.status).find(a => a.key === action);
  if (!actionDef) return;

  modalLoading.value = true;

  try {
    const url = `/api/admin/contests/${contest.id}/${actionDef.endpoint}`;
    const body = action === 'cancel' && cancelReason.value.trim()
      ? { reason: cancelReason.value.trim() }
      : undefined;

    await api.post(url, body);
    toast.success(t(`contests.actionSuccess.${action}`));
    closeModal();
    await fetchContests();
  } catch (err) {
    const message = getErrorMessage(err, t('contests.actionError'));
    toast.error(message);
    modalLoading.value = false;
  }
}
</script>

<template>
  <div class="contests-page">
    <div class="page-header">
      <h1 class="page-title">{{ t('contests.title') }}</h1>
      <button class="btn btn-primary" @click="createContest">
        + {{ t('contests.newContest') }}
      </button>
    </div>

    <!-- View Tabs: Active vs Archive -->
    <div class="view-tabs">
      <button
        :class="['view-tab', { active: activeViewTab === 'active' }]"
        @click="switchViewTab('active')"
      >
        {{ t('contests.viewActive') }}
      </button>
      <button
        :class="['view-tab', { active: activeViewTab === 'archive' }]"
        @click="switchViewTab('archive')"
      >
        {{ t('contests.viewArchive') }}
      </button>
    </div>

    <div class="filters">
      <input
        v-model="searchQuery"
        type="text"
        class="input search-input"
        :placeholder="t('contests.search')"
      />
      <select v-model="statusFilter" class="input status-select">
        <option value="">{{ t('common.all') }}</option>
        <option v-for="status in statuses" :key="status" :value="status">
          {{ t(`status.${status}`) }}
        </option>
      </select>
      <select v-model="typeFilter" class="input status-select">
        <option value="">{{ t('contests.typeAll') }}</option>
        <option value="free">{{ t('contests.typeFree') }}</option>
        <option value="paid">{{ t('contests.typePaid') }}</option>
      </select>
      <div class="sort-controls">
        <span class="sort-label">{{ t('contests.sortBy') }}:</span>
        <button
          :class="['btn', 'btn-ghost', 'btn-sm', 'sort-btn', { active: sortBy === 'status' }]"
          @click="setSortBy('status')"
        >
          {{ t('contests.status') }} {{ getSortIcon('status') }}
        </button>
        <button
          :class="['btn', 'btn-ghost', 'btn-sm', 'sort-btn', { active: sortBy === 'start_time' }]"
          @click="setSortBy('start_time')"
        >
          {{ t('contests.startDate') }} {{ getSortIcon('start_time') }}
        </button>
        <button
          :class="['btn', 'btn-ghost', 'btn-sm', 'sort-btn', { active: sortBy === 'participant_count' }]"
          @click="setSortBy('participant_count')"
        >
          {{ t('contests.participants') }} {{ getSortIcon('participant_count') }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="loading">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="error" class="error-state">
      <p>{{ error }}</p>
      <button class="btn btn-primary" @click="fetchContests">{{ t('common.retry') }}</button>
    </div>

    <div v-else-if="filteredContests.length === 0" class="no-results">
      {{ t('contests.noResults') }}
    </div>

    <div v-else class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('contests.id') }}</th>
            <th>{{ t('contests.name') }}</th>
            <th>{{ t('contests.status') }}</th>
            <th>{{ t('contests.participants') }}</th>
            <th>{{ t('contests.startDate') }}</th>
            <th>{{ t('contests.endDate') }}</th>
            <th>{{ t('contests.timeInfo') }}</th>
            <th>{{ t('contests.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="contest in filteredContests" :key="contest.id">
            <td class="id-cell">{{ contest.id }}</td>
            <td>
              <a
                class="contest-name-link"
                href="#"
                @click.prevent="viewContestDetail(contest.id)"
              >{{ contest.name }}</a>
              <span v-if="contest.is_free" class="free-badge">{{ t('contests.freeBadge') }}</span>
            </td>
            <td>
              <span :class="['status-badge', getStatusClass(contest.status)]">
                <span v-if="contest.status === 'running'" class="pulse-dot pulse-green"></span>
                <span v-else-if="contest.status === 'paused'" class="pulse-dot pulse-yellow"></span>
                {{ t(`status.${contest.status}`) }}
              </span>
            </td>
            <td>{{ formatParticipants(contest) }}</td>
            <td>{{ formatDate(contest.start_time) }}</td>
            <td>{{ formatDate(contest.end_time) }}</td>
            <td class="time-info-cell">
              <span v-if="getTimeInfo(contest)" class="time-info">{{ getTimeInfo(contest) }}</span>
              <span v-else class="time-info-empty">&mdash;</span>
            </td>
            <td class="actions-cell">
              <button class="btn btn-ghost btn-sm" @click="editContest(contest.id)">
                {{ t('contests.edit') }}
              </button>
              <button class="btn btn-ghost btn-sm btn-danger" @click="deleteContest(contest.id)">
                {{ t('contests.delete') }}
              </button>

              <!-- State machine actions dropdown -->
              <template v-if="getActionsForStatus(contest.status).length > 0">
                <div class="actions-dropdown">
                  <button
                    class="btn btn-ghost btn-sm btn-actions-menu"
                    @click.stop="toggleDropdown(contest.id)"
                  >
                    &#x22EE;
                  </button>
                  <div
                    v-if="openDropdownId === contest.id"
                    class="dropdown-menu"
                  >
                    <button
                      v-for="action in getActionsForStatus(contest.status)"
                      :key="action.key"
                      :class="['dropdown-item', action.colorClass]"
                      @click="openActionModal(action.key, contest)"
                    >
                      {{ t(action.label) }}
                    </button>
                  </div>
                </div>
              </template>
              <span v-else class="no-actions">{{ t('contests.action.noActions') }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Confirmation Modal -->
    <Teleport to="body">
      <div v-if="showModal" class="modal-overlay" @click.self="closeModal">
        <div class="modal-container">
          <div class="modal-header">
            <h3 class="modal-title">{{ modalAction ? getModalTitle(modalAction) : '' }}</h3>
            <button class="modal-close" @click="closeModal">&times;</button>
          </div>
          <div class="modal-body">
            <p class="modal-message">
              {{ modalAction && modalContest ? getModalMessage(modalAction, modalContest) : '' }}
            </p>

            <!-- Cancel reason textarea -->
            <div v-if="modalAction === 'cancel'" class="form-group">
              <label class="form-label">{{ t('contests.confirm.cancelReason') }}</label>
              <textarea
                v-model="cancelReason"
                class="form-textarea"
                :placeholder="t('contests.confirm.cancelReasonPlaceholder')"
                rows="3"
              ></textarea>
            </div>
          </div>
          <div class="modal-footer">
            <button
              class="btn btn-ghost"
              :disabled="modalLoading"
              @click="closeModal"
            >
              {{ t('common.cancel') }}
            </button>
            <button
              :class="['btn', modalAction === 'cancel' ? 'btn-danger-solid' : 'btn-primary']"
              :disabled="modalLoading"
              @click="confirmAction"
            >
              {{ modalLoading ? t('common.loading') : t('common.confirm') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.contests-page {
  padding: var(--spacing-lg) 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xl);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

/* View Tabs */
.view-tabs {
  display: flex;
  gap: var(--spacing-xs);
  margin-bottom: var(--spacing-lg);
  background-color: var(--color-bg-tertiary, #F3F4F6);
  padding: var(--spacing-xs);
  border-radius: var(--radius-md);
  width: fit-content;
}

.view-tab {
  padding: var(--spacing-sm) var(--spacing-lg);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  background: none;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.15s ease;
}

.view-tab:hover {
  color: var(--color-text-primary);
}

.view-tab.active {
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
  font-weight: 600;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
  align-items: center;
}

.search-input {
  flex: 1;
  max-width: 300px;
}

.status-select {
  width: 180px;
}

.loading,
.no-results,
.error-state {
  text-align: center;
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
}

.error-state p {
  margin: 0;
}

.table-container {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: var(--spacing-md);
  text-align: left;
  border-bottom: 1px solid var(--color-border);
}

[dir="rtl"] .data-table th,
[dir="rtl"] .data-table td {
  text-align: right;
}

.data-table th {
  background-color: var(--color-bg-secondary);
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.data-table tbody tr:hover {
  background-color: var(--color-bg-secondary);
}

.id-cell {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.free-badge {
  display: inline-block;
  margin-inline-start: 6px;
  padding: 1px 6px;
  border-radius: var(--radius-full);
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  background: var(--color-success, #22c55e);
  color: #fff;
  vertical-align: middle;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
}

.pulse-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.pulse-green {
  background-color: #16A34A;
  animation: pulse-green 2s infinite;
}

.pulse-yellow {
  background-color: #CA8A04;
  animation: pulse-yellow 2s infinite;
}

@keyframes pulse-green {
  0% {
    box-shadow: 0 0 0 0 rgba(22, 163, 74, 0.5);
  }
  70% {
    box-shadow: 0 0 0 6px rgba(22, 163, 74, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(22, 163, 74, 0);
  }
}

@keyframes pulse-yellow {
  0% {
    box-shadow: 0 0 0 0 rgba(202, 138, 4, 0.5);
  }
  70% {
    box-shadow: 0 0 0 6px rgba(202, 138, 4, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(202, 138, 4, 0);
  }
}

.contest-name-link {
  color: var(--color-primary, #2563EB);
  text-decoration: none;
  font-weight: 500;
  cursor: pointer;
}

.contest-name-link:hover {
  text-decoration: underline;
  color: var(--color-primary-hover, #1D4ED8);
}

.time-info-cell {
  white-space: nowrap;
}

.time-info {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.time-info-empty {
  color: var(--color-text-muted);
}

.sort-controls {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  margin-inline-start: auto;
}

.sort-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.sort-btn {
  font-size: var(--font-size-xs);
  white-space: nowrap;
}

.sort-btn.active {
  color: var(--color-primary, #2563EB);
  font-weight: 600;
}

.status-running {
  background-color: #DCFCE7;
  color: #16A34A;
}

.status-scheduled {
  background-color: #DBEAFE;
  color: #2563EB;
}

.status-registration {
  background-color: #FEF3C7;
  color: #D97706;
}

.status-registration-closed {
  background-color: #FED7AA;
  color: #C2410C;
}

.status-completed {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

.status-draft {
  background-color: #F3F4F6;
  color: #6B7280;
}

.status-paused {
  background-color: #FEE2E2;
  color: #DC2626;
}

.status-cancelled {
  background-color: #FEE2E2;
  color: #DC2626;
}

.actions-cell {
  white-space: nowrap;
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
}

.btn-danger {
  color: #DC2626;
}

.btn-danger:hover {
  background-color: #FEE2E2;
}

/* Actions dropdown */
.actions-dropdown {
  display: inline-block;
  position: relative;
}

.btn-actions-menu {
  font-size: var(--font-size-lg);
  line-height: 1;
  padding: var(--spacing-xs) var(--spacing-sm);
  min-width: 32px;
  font-weight: 700;
  letter-spacing: 1px;
}

.dropdown-menu {
  position: absolute;
  right: 0;
  top: 100%;
  z-index: 50;
  min-width: 180px;
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  padding: var(--spacing-xs) 0;
}

[dir="rtl"] .dropdown-menu {
  right: auto;
  left: 0;
}

.dropdown-item {
  display: block;
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  text-align: left;
  background: none;
  border: none;
  cursor: pointer;
  font-size: var(--font-size-sm);
  font-weight: 500;
  transition: background-color 0.15s;
}

[dir="rtl"] .dropdown-item {
  text-align: right;
}

.dropdown-item:hover {
  background-color: var(--color-bg-secondary);
}

.action-green {
  color: #16A34A;
}

.action-green:hover {
  background-color: #F0FDF4;
}

.action-blue {
  color: #2563EB;
}

.action-blue:hover {
  background-color: #EFF6FF;
}

.action-orange {
  color: #D97706;
}

.action-orange:hover {
  background-color: #FFFBEB;
}

.action-yellow {
  color: #CA8A04;
}

.action-yellow:hover {
  background-color: #FEFCE8;
}

.action-red {
  color: #DC2626;
}

.action-red:hover {
  background-color: #FEF2F2;
}

.no-actions {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  padding: 0 var(--spacing-sm);
}

/* Modal */
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: rgba(0, 0, 0, 0.5);
}

.modal-container {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
  width: 100%;
  max-width: 480px;
  margin: var(--spacing-md);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-lg) var(--spacing-lg) 0;
}

.modal-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.modal-close {
  background: none;
  border: none;
  font-size: 24px;
  cursor: pointer;
  color: var(--color-text-muted);
  padding: 0;
  line-height: 1;
}

.modal-close:hover {
  color: var(--color-text-primary);
}

.modal-body {
  padding: var(--spacing-lg);
}

.modal-message {
  color: var(--color-text-secondary);
  line-height: 1.6;
  margin: 0 0 var(--spacing-md) 0;
}

.form-group {
  margin-top: var(--spacing-md);
}

.form-label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.form-textarea {
  width: 100%;
  padding: var(--spacing-sm);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  resize: vertical;
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  font-family: inherit;
  box-sizing: border-box;
}

.form-textarea:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.2);
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
  padding: 0 var(--spacing-lg) var(--spacing-lg);
}

[dir="rtl"] .modal-footer {
  justify-content: flex-start;
}

.btn-danger-solid {
  background-color: #DC2626;
  color: #fff;
  border: none;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  cursor: pointer;
  font-weight: 500;
}

.btn-danger-solid:hover {
  background-color: #B91C1C;
}

.btn-danger-solid:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

@media (max-width: 767px) {
  .page-header {
    flex-direction: column;
    gap: var(--spacing-md);
    align-items: stretch;
  }

  .filters {
    flex-direction: column;
  }

  .search-input,
  .status-select {
    max-width: none;
    width: 100%;
  }

  .sort-controls {
    margin-inline-start: 0;
    flex-wrap: wrap;
  }

  .table-container {
    overflow-x: auto;
  }

  .data-table {
    min-width: 850px;
  }

  .modal-container {
    max-width: 100%;
    margin: var(--spacing-sm);
  }
}
</style>
