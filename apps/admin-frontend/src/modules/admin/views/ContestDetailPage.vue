<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { api } from '@/api';
import { getErrorMessage } from '@/utils/errorHandler';

const route = useRoute();
const router = useRouter();
const toast = useToast();

// --- Types ---

interface ContestDetail {
  id: string;
  name: string;
  description: string;
  status: string;
  starts_at: string;
  ends_at: string;
  entry_fee_cents: number;
  platform_fee_bps: number;
  qty_total: number;
  duration_type: string;
  asset_class: string;
  min_participants: number;
  max_participants: number | null;
  registration_deadline: string;
  auto_start: boolean;
  commission_rate: number;
  is_free: boolean;
  symbols: string[];
  participant_count: number;
}

interface ContestState {
  contest_id: string;
  status: string;
  participant_count: number;
  max_participants: number | null;
}

interface StatusHistoryEntry {
  status: string;
  changed_at: string;
  changed_by?: string;
  reason?: string;
}

interface Participant {
  user_id: string;
  username: string;
  joined_at: string;
  qty_total: number;
  qty_available: number;
  total_score: number;
  final_rank: number | null;
  final_prize_cents: number | null;
}

interface LeaderboardEntry {
  rank: number;
  user_id: string;
  username: string;
  total_score: number;
  realized_score: number;
  unrealized_score: number;
}

interface LeaderboardResponse {
  contest_id: string;
  entries: LeaderboardEntry[];
  total_participants: number;
  updated_at: string;
}

interface ParticipantsResponse {
  participants: Participant[];
  total: number;
}

type ContestAction = 'publish' | 'openRegistration' | 'closeRegistration' | 'start' | 'end' | 'pause' | 'resume' | 'cancel';

interface ActionDef {
  key: ContestAction;
  label: string;
  colorClass: string;
  endpoint: string;
}

// --- State ---

const contestId = computed(() => route.params.id as string);
const contest = ref<ContestDetail | null>(null);
const contestState = ref<ContestState | null>(null);
const statusHistory = ref<StatusHistoryEntry[]>([]);
const loading = ref(true);
const stateLoading = ref(false);
const historyLoading = ref(false);
const activeTab = ref('overview');
const now = ref(Date.now());

// Participants state
const participants = ref<Participant[]>([]);
const participantsLoading = ref(false);
const participantsSearch = ref('');
const participantsFetched = ref(false);

// Participant management state
const selectedParticipants = ref<Set<string>>(new Set());
const participantSortField = ref<'joined_at' | 'total_score' | 'username'>('joined_at');
const participantSortAsc = ref(false);
const showRemoveModal = ref(false);
const removeTarget = ref<Participant | null>(null);
const removeLoading = ref(false);

// Leaderboard state
const leaderboardEntries = ref<LeaderboardEntry[]>([]);
const leaderboardTotalParticipants = ref(0);
const leaderboardUpdatedAt = ref('');
const leaderboardLoading = ref(false);
const leaderboardFetched = ref(false);
let leaderboardRefreshInterval: ReturnType<typeof setInterval> | null = null;

const sortedParticipants = computed(() => {
  const sorted = [...participants.value];
  const field = participantSortField.value;
  const asc = participantSortAsc.value;
  sorted.sort((a, b) => {
    let cmp = 0;
    if (field === 'joined_at') {
      cmp = new Date(a.joined_at).getTime() - new Date(b.joined_at).getTime();
    } else if (field === 'total_score') {
      cmp = a.total_score - b.total_score;
    } else if (field === 'username') {
      cmp = a.username.localeCompare(b.username);
    }
    return asc ? cmp : -cmp;
  });
  return sorted;
});

const filteredParticipants = computed(() => {
  const query = participantsSearch.value.toLowerCase().trim();
  if (!query) return sortedParticipants.value;
  return sortedParticipants.value.filter(
    p => p.username.toLowerCase().includes(query)
  );
});

// Participant stats
const participantStats = computed(() => {
  const all = participants.value;
  const total = all.length;
  // Active traders: those who have used some capital (qty_available < qty_total)
  const activeTraders = all.filter(p => p.qty_available < p.qty_total).length;
  const avgScore = total > 0
    ? all.reduce((sum, p) => sum + p.total_score, 0) / total
    : 0;
  const todayStart = new Date();
  todayStart.setHours(0, 0, 0, 0);
  const joinedToday = all.filter(p => new Date(p.joined_at).getTime() >= todayStart.getTime()).length;
  return { total, activeTraders, avgScore, joinedToday };
});

const allSelected = computed(() => {
  return filteredParticipants.value.length > 0 &&
    filteredParticipants.value.every(p => selectedParticipants.value.has(p.user_id));
});

const someSelected = computed(() => {
  return selectedParticipants.value.size > 0 && !allSelected.value;
});

// Modal state
const showModal = ref(false);
const modalAction = ref<ContestAction | null>(null);
const modalLoading = ref(false);
const cancelReason = ref('');

const tabs = ['overview', 'statusHistory', 'participants', 'leaderboard'];

// Timer refs
let refreshInterval: ReturnType<typeof setInterval> | null = null;
let nowInterval: ReturnType<typeof setInterval> | null = null;

// --- Status actions (same state machine as ContestsPage) ---

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

const currentActions = computed(() => {
  const status = contestState.value?.status || contest.value?.status || '';
  return getActionsForStatus(status);
});

const currentStatus = computed(() => {
  return contestState.value?.status || contest.value?.status || '';
});

const isContestCompleted = computed(() => {
  return currentStatus.value === 'completed' || currentStatus.value === 'settling';
});

const participantCount = computed(() => {
  return contestState.value?.participant_count ?? contest.value?.participant_count ?? 0;
});

const maxParticipants = computed(() => {
  return contestState.value?.max_participants ?? contest.value?.max_participants ?? null;
});

// --- Computed info cards ---

const participantProgress = computed(() => {
  const count = participantCount.value;
  const max = maxParticipants.value;
  if (!max || max <= 0) return 0;
  return Math.min((count / max) * 100, 100);
});

const prizePool = computed(() => {
  if (!contest.value) return 0;
  const entryFee = contest.value.entry_fee_cents;
  const count = participantCount.value;
  const platformFeeBps = contest.value.platform_fee_bps;
  return (entryFee * count * (1 - platformFeeBps / 10000)) / 100;
});

const timeDisplay = computed(() => {
  if (!contest.value) return '';
  const status = currentStatus.value;
  const currentTime = now.value;

  if (status === 'scheduled' || status === 'registration_open' || status === 'registration_closed') {
    const startTime = new Date(contest.value.starts_at).getTime();
    const diff = startTime - currentTime;
    if (diff <= 0) return t('contestDetail.time.startingSoon');
    return t('contestDetail.time.startsIn', { time: formatTimeDiff(diff) });
  }

  if (status === 'running' || status === 'paused') {
    const endTime = new Date(contest.value.ends_at).getTime();
    const diff = endTime - currentTime;
    if (diff <= 0) return t('contestDetail.time.endingSoon');
    return t('contestDetail.time.endsIn', { time: formatTimeDiff(diff) });
  }

  if (status === 'completed' || status === 'settling') {
    return t('contestDetail.time.ended', { date: formatDateTime(contest.value.ends_at) });
  }

  return '';
});

// --- Formatting helpers ---

function formatTimeDiff(ms: number): string {
  const totalSeconds = Math.floor(ms / 1000);
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (days > 0) {
    return `${days}d ${hours}h`;
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}

function formatDateTime(dateString: string): string {
  if (!dateString) return '\u2014';
  return new Date(dateString).toLocaleString();
}

function formatCurrency(cents: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
  }).format(cents / 100);
}

function formatDollars(amount: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
  }).format(amount);
}

function formatPercent(bps: number): string {
  return `${(bps / 100).toFixed(2)}%`;
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
    settling: 'status-settling',
  };
  return classes[status] || 'status-default';
}

// --- Data fetching ---

async function fetchContest(): Promise<void> {
  loading.value = true;
  try {
    const response = await api.get(`/api/admin/contests/${contestId.value}`);
    contest.value = response.data;
  } catch {
    toast.error(t('contestDetail.loadError'));
    router.push({ name: 'admin-contests' });
  } finally {
    loading.value = false;
  }
}

async function fetchState(): Promise<void> {
  stateLoading.value = true;
  try {
    const response = await api.get<ContestState>(`/api/admin/contests/${contestId.value}/state`);
    contestState.value = response.data;
  } catch {
    // Silent fail — state endpoint may not exist yet
  } finally {
    stateLoading.value = false;
  }
}

async function fetchStatusHistory(): Promise<void> {
  historyLoading.value = true;
  try {
    const response = await api.get<StatusHistoryEntry[]>(`/api/admin/contests/${contestId.value}/status-history`);
    statusHistory.value = Array.isArray(response.data) ? response.data : [];
  } catch {
    statusHistory.value = [];
  } finally {
    historyLoading.value = false;
  }
}

async function fetchParticipants(): Promise<void> {
  participantsLoading.value = true;
  try {
    const response = await api.get<ParticipantsResponse>(`/api/user/contests/${contestId.value}/participants`);
    participants.value = response.data.participants || [];
    participantsFetched.value = true;
  } catch {
    participants.value = [];
    toast.error(t('contestDetail.participants.loadError'));
  } finally {
    participantsLoading.value = false;
  }
}

async function silentRefreshState(): Promise<void> {
  try {
    const response = await api.get<ContestState>(`/api/admin/contests/${contestId.value}/state`);
    contestState.value = response.data;
  } catch {
    // Silent refresh
  }
}

// --- Leaderboard ---

const leaderboardUpdatedAgo = computed(() => {
  if (!leaderboardUpdatedAt.value) return '';
  const updatedTime = new Date(leaderboardUpdatedAt.value).getTime();
  const diff = now.value - updatedTime;
  if (diff < 0) return t('contestDetail.leaderboard.justNow');
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return t('contestDetail.leaderboard.secondsAgo', { count: String(seconds) });
  const minutes = Math.floor(seconds / 60);
  return t('contestDetail.leaderboard.minutesAgo', { count: String(minutes) });
});

async function fetchLeaderboard(): Promise<void> {
  leaderboardLoading.value = true;
  try {
    const response = await api.get<LeaderboardResponse>(`/api/trade/leaderboard/${contestId.value}`);
    leaderboardEntries.value = response.data.entries || [];
    leaderboardTotalParticipants.value = response.data.total_participants;
    leaderboardUpdatedAt.value = response.data.updated_at;
    leaderboardFetched.value = true;
  } catch {
    leaderboardEntries.value = [];
    toast.error(t('contestDetail.leaderboard.loadError'));
  } finally {
    leaderboardLoading.value = false;
  }
}

async function silentRefreshLeaderboard(): Promise<void> {
  try {
    const response = await api.get<LeaderboardResponse>(`/api/trade/leaderboard/${contestId.value}`);
    leaderboardEntries.value = response.data.entries || [];
    leaderboardTotalParticipants.value = response.data.total_participants;
    leaderboardUpdatedAt.value = response.data.updated_at;
  } catch {
    // Silent refresh
  }
}

function startLeaderboardAutoRefresh(): void {
  stopLeaderboardAutoRefresh();
  leaderboardRefreshInterval = setInterval(silentRefreshLeaderboard, 5000);
}

function stopLeaderboardAutoRefresh(): void {
  if (leaderboardRefreshInterval !== null) {
    clearInterval(leaderboardRefreshInterval);
    leaderboardRefreshInterval = null;
  }
}

function getRankClass(rank: number): string {
  if (rank === 1) return 'rank-gold';
  if (rank === 2) return 'rank-silver';
  if (rank === 3) return 'rank-bronze';
  return '';
}

function formatScore(score: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(score);
}

// --- Auto-refresh ---

function startAutoRefresh(): void {
  stopAutoRefresh();
  refreshInterval = setInterval(silentRefreshState, 10000);
}

function stopAutoRefresh(): void {
  if (refreshInterval !== null) {
    clearInterval(refreshInterval);
    refreshInterval = null;
  }
}

function handleVisibilityChange(): void {
  if (document.hidden) {
    stopAutoRefresh();
    stopLeaderboardAutoRefresh();
  } else if (currentStatus.value === 'running') {
    startAutoRefresh();
    silentRefreshState();
    if (activeTab.value === 'leaderboard') {
      startLeaderboardAutoRefresh();
      silentRefreshLeaderboard();
    }
  }
}

watch(activeTab, (tab: string) => {
  if (tab === 'participants' && !participantsFetched.value) {
    fetchParticipants();
  }
  if (tab === 'leaderboard') {
    if (!leaderboardFetched.value) {
      fetchLeaderboard();
    }
    if (currentStatus.value === 'running') {
      startLeaderboardAutoRefresh();
    }
  } else {
    stopLeaderboardAutoRefresh();
  }
});

watch(currentStatus, (status: string) => {
  if (status === 'running') {
    startAutoRefresh();
    if (activeTab.value === 'leaderboard') {
      startLeaderboardAutoRefresh();
    }
  } else {
    stopAutoRefresh();
    stopLeaderboardAutoRefresh();
  }
});

// --- Participant management ---

function toggleSelectAll(): void {
  if (allSelected.value) {
    selectedParticipants.value = new Set();
  } else {
    selectedParticipants.value = new Set(filteredParticipants.value.map(p => p.user_id));
  }
}

function toggleParticipantSelection(userId: string): void {
  const next = new Set(selectedParticipants.value);
  if (next.has(userId)) {
    next.delete(userId);
  } else {
    next.add(userId);
  }
  selectedParticipants.value = next;
}

function setParticipantSort(field: 'joined_at' | 'total_score' | 'username'): void {
  if (participantSortField.value === field) {
    participantSortAsc.value = !participantSortAsc.value;
  } else {
    participantSortField.value = field;
    participantSortAsc.value = field === 'username';
  }
}

function getSortIndicator(field: string): string {
  if (participantSortField.value !== field) return '';
  return participantSortAsc.value ? ' \u25B2' : ' \u25BC';
}

function generateCsv(items: Participant[]): string {
  const header = 'Username,Joined At,Score,Rank';
  const rows = items.map(p => {
    const username = `"${p.username.replace(/"/g, '""')}"`;
    const joinedAt = `"${new Date(p.joined_at).toISOString()}"`;
    const score = p.total_score.toFixed(2);
    const rank = p.final_rank != null ? String(p.final_rank) : '';
    return `${username},${joinedAt},${score},${rank}`;
  });
  return [header, ...rows].join('\n');
}

function downloadCsv(csvContent: string, filename: string): void {
  const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

function exportAllParticipants(): void {
  const csv = generateCsv(filteredParticipants.value);
  const contestName = contest.value?.name?.replace(/[^a-zA-Z0-9]/g, '_') || 'contest';
  downloadCsv(csv, `${contestName}_participants.csv`);
}

function exportSelectedParticipants(): void {
  const selected = filteredParticipants.value.filter(p => selectedParticipants.value.has(p.user_id));
  if (selected.length === 0) return;
  const csv = generateCsv(selected);
  const contestName = contest.value?.name?.replace(/[^a-zA-Z0-9]/g, '_') || 'contest';
  downloadCsv(csv, `${contestName}_participants_selected.csv`);
}

function openRemoveModal(participant: Participant): void {
  removeTarget.value = participant;
  showRemoveModal.value = true;
}

function closeRemoveModal(): void {
  showRemoveModal.value = false;
  removeTarget.value = null;
  removeLoading.value = false;
}

async function confirmRemoveParticipant(): Promise<void> {
  if (!removeTarget.value || !contest.value) return;
  removeLoading.value = true;
  try {
    await api.delete(`/api/admin/contests/${contest.value.id}/participants/${removeTarget.value.user_id}`);
    toast.success(t('contestDetail.participants.removeSuccess'));
    // Remove from local state
    participants.value = participants.value.filter(p => p.user_id !== removeTarget.value!.user_id);
    selectedParticipants.value.delete(removeTarget.value.user_id);
    closeRemoveModal();
  } catch (err) {
    const message = getErrorMessage(err, t('contestDetail.participants.removeError'));
    toast.error(message);
    removeLoading.value = false;
  }
}

// --- Navigation ---

function goBack(): void {
  router.push({ name: 'admin-contests' });
}

function editContest(): void {
  router.push({ name: 'admin-contest-edit', params: { id: contestId.value } });
}

// --- Action modal ---

function getModalTitle(action: ContestAction): string {
  return t(`contests.confirm.${action}Title`);
}

function getModalMessage(action: ContestAction): string {
  if (action === 'start') {
    return t('contests.confirm.startMessage', { count: String(participantCount.value) });
  }
  return t(`contests.confirm.${action}Message`);
}

function openActionModal(action: ContestAction): void {
  modalAction.value = action;
  cancelReason.value = '';
  showModal.value = true;
}

function closeModal(): void {
  showModal.value = false;
  modalAction.value = null;
  modalLoading.value = false;
  cancelReason.value = '';
}

async function confirmAction(): Promise<void> {
  if (!modalAction.value || !contest.value) return;

  const action = modalAction.value;
  const actionDef = getActionsForStatus(currentStatus.value).find(a => a.key === action);
  if (!actionDef) return;

  modalLoading.value = true;

  try {
    const url = `/api/admin/contests/${contest.value.id}/${actionDef.endpoint}`;
    const body = action === 'cancel' && cancelReason.value.trim()
      ? { reason: cancelReason.value.trim() }
      : undefined;

    await api.post(url, body);
    toast.success(t(`contests.actionSuccess.${action}`));
    closeModal();
    // Refresh all data
    await Promise.all([fetchContest(), fetchState(), fetchStatusHistory()]);
  } catch (err) {
    const message = getErrorMessage(err, t('contests.actionError'));
    toast.error(message);
    modalLoading.value = false;
  }
}

// --- Lifecycle ---

onMounted(async () => {
  await fetchContest();
  await Promise.all([fetchState(), fetchStatusHistory()]);

  // Start the "now" clock for countdown
  nowInterval = setInterval(() => {
    now.value = Date.now();
  }, 1000);

  // Auto-refresh if running
  if (currentStatus.value === 'running') {
    startAutoRefresh();
  }

  document.addEventListener('visibilitychange', handleVisibilityChange);
});

onUnmounted(() => {
  stopAutoRefresh();
  stopLeaderboardAutoRefresh();
  if (nowInterval !== null) {
    clearInterval(nowInterval);
    nowInterval = null;
  }
  document.removeEventListener('visibilitychange', handleVisibilityChange);
});
</script>

<template>
  <div class="contest-detail-page">
    <!-- Loading -->
    <div v-if="loading" class="loading-container">
      <div class="loading">{{ t('common.loading') }}</div>
    </div>

    <!-- Content -->
    <div v-else-if="contest">
      <!-- Breadcrumb -->
      <nav class="breadcrumb">
        <a href="#" class="breadcrumb-link" @click.prevent="goBack">{{ t('nav.contests') }}</a>
        <span class="breadcrumb-separator">/</span>
        <span class="breadcrumb-current">{{ contest.name }}</span>
      </nav>

      <!-- Header -->
      <div class="page-header">
        <div class="header-info">
          <div class="header-title-row">
            <h1 class="page-title">{{ contest.name }}</h1>
            <span :class="['status-badge', getStatusClass(currentStatus)]">
              <span v-if="currentStatus === 'running'" class="pulse-dot pulse-green"></span>
              <span v-else-if="currentStatus === 'paused'" class="pulse-dot pulse-yellow"></span>
              {{ t(`status.${currentStatus}`) }}
            </span>
          </div>
          <p v-if="contest.description" class="contest-description">{{ contest.description }}</p>
        </div>
        <div class="header-actions">
          <button class="btn btn-ghost" @click="editContest">
            {{ t('contestDetail.edit') }}
          </button>
          <button
            v-for="action in currentActions"
            :key="action.key"
            :class="['btn', `btn-${action.colorClass}`]"
            @click="openActionModal(action.key)"
          >
            {{ t(action.label) }}
          </button>
        </div>
      </div>

      <!-- Info Cards -->
      <div class="info-cards">
        <!-- Participants Card -->
        <div class="info-card">
          <span class="info-card-label">{{ t('contestDetail.cards.participants') }}</span>
          <span class="info-card-value">
            {{ participantCount }}<span v-if="maxParticipants" class="info-card-secondary"> / {{ maxParticipants }}</span>
            <span v-else class="info-card-secondary"> / &infin;</span>
          </span>
          <div v-if="maxParticipants" class="progress-bar">
            <div class="progress-fill" :style="{ width: `${participantProgress}%` }"></div>
          </div>
        </div>

        <!-- Prize Pool Card -->
        <div class="info-card">
          <span class="info-card-label">{{ t('contestDetail.cards.prizePool') }}</span>
          <span class="info-card-value">{{ formatDollars(prizePool) }}</span>
          <span v-if="contest.is_free" class="info-card-hint">{{ t('contestDetail.cards.freeContest') }}</span>
        </div>

        <!-- QTY Total Card -->
        <div class="info-card">
          <span class="info-card-label">{{ t('contestDetail.cards.qtyTotal') }}</span>
          <span class="info-card-value">{{ formatDollars(contest.qty_total) }}</span>
        </div>

        <!-- Time Card -->
        <div class="info-card">
          <span class="info-card-label">{{ t('contestDetail.cards.time') }}</span>
          <span class="info-card-value info-card-time">{{ timeDisplay || '\u2014' }}</span>
        </div>
      </div>

      <!-- Tabs -->
      <div class="tabs">
        <button
          v-for="tab in tabs"
          :key="tab"
          :class="['tab-button', { active: activeTab === tab }]"
          @click="activeTab = tab"
        >
          {{ t(`contestDetail.tabs.${tab}`) }}
        </button>
      </div>

      <!-- Tab Content -->
      <div class="tab-content">
        <!-- Overview Tab -->
        <div v-if="activeTab === 'overview'" class="tab-panel">
          <div class="overview-grid">
            <!-- Schedule -->
            <div class="overview-section">
              <h3 class="section-title">{{ t('contestDetail.overview.schedule') }}</h3>
              <div class="info-rows">
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.startDate') }}</span>
                  <span class="info-value">{{ formatDateTime(contest.starts_at) }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.endDate') }}</span>
                  <span class="info-value">{{ formatDateTime(contest.ends_at) }}</span>
                </div>
                <div v-if="contest.registration_deadline" class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.registrationDeadline') }}</span>
                  <span class="info-value">{{ formatDateTime(contest.registration_deadline) }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.durationType') }}</span>
                  <span class="info-value">{{ t(`contestForm.durationTypes.${contest.duration_type}`) }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.autoStart') }}</span>
                  <span class="info-value">{{ contest.auto_start ? t('contestDetail.overview.yes') : t('contestDetail.overview.no') }}</span>
                </div>
              </div>
            </div>

            <!-- Pricing -->
            <div class="overview-section">
              <h3 class="section-title">{{ t('contestDetail.overview.pricing') }}</h3>
              <div class="info-rows">
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.entryFee') }}</span>
                  <span class="info-value">
                    <template v-if="contest.is_free">{{ t('contestDetail.overview.free') }}</template>
                    <template v-else>{{ formatCurrency(contest.entry_fee_cents) }}</template>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.platformFee') }}</span>
                  <span class="info-value">{{ formatPercent(contest.platform_fee_bps) }}</span>
                </div>
                <div v-if="contest.commission_rate > 0" class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.commissionRate') }}</span>
                  <span class="info-value">{{ contest.commission_rate }}%</span>
                </div>
              </div>
            </div>

            <!-- Participants -->
            <div class="overview-section">
              <h3 class="section-title">{{ t('contestDetail.overview.participants') }}</h3>
              <div class="info-rows">
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.currentParticipants') }}</span>
                  <span class="info-value">{{ participantCount }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.minParticipants') }}</span>
                  <span class="info-value">{{ contest.min_participants }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.maxParticipants') }}</span>
                  <span class="info-value">{{ contest.max_participants ?? t('contestDetail.overview.unlimited') }}</span>
                </div>
              </div>
            </div>

            <!-- Details -->
            <div class="overview-section">
              <h3 class="section-title">{{ t('contestDetail.overview.details') }}</h3>
              <div class="info-rows">
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.assetClass') }}</span>
                  <span class="info-value">{{ t(`contestForm.assetClasses.${contest.asset_class}`) }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">{{ t('contestDetail.overview.initialCapital') }}</span>
                  <span class="info-value">{{ formatDollars(contest.qty_total) }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">{{ t('contests.id') }}</span>
                  <span class="info-value mono">{{ contest.id }}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Symbols -->
          <div v-if="contest.symbols && contest.symbols.length > 0" class="symbols-section">
            <h3 class="section-title">{{ t('contestDetail.overview.symbols') }}</h3>
            <div class="symbols-list">
              <span v-for="symbol in contest.symbols" :key="symbol" class="symbol-badge">
                {{ symbol }}
              </span>
            </div>
          </div>
        </div>

        <!-- Status History Tab -->
        <div v-if="activeTab === 'statusHistory'" class="tab-panel">
          <div v-if="historyLoading" class="loading">{{ t('common.loading') }}</div>
          <div v-else-if="statusHistory.length === 0" class="no-data">
            {{ t('contestDetail.statusHistory.noHistory') }}
          </div>
          <div v-else class="timeline">
            <div v-for="(entry, index) in statusHistory" :key="index" class="timeline-item">
              <div class="timeline-marker">
                <div :class="['timeline-dot', getStatusClass(entry.status)]"></div>
                <div v-if="Number(index) < statusHistory.length - 1" class="timeline-line"></div>
              </div>
              <div class="timeline-content">
                <div class="timeline-header">
                  <span :class="['status-badge', 'status-badge-sm', getStatusClass(entry.status)]">
                    {{ t(`status.${entry.status}`) }}
                  </span>
                  <span class="timeline-date">{{ formatDateTime(entry.changed_at) }}</span>
                </div>
                <p v-if="entry.changed_by" class="timeline-meta">
                  {{ t('contestDetail.statusHistory.changedBy', { user: entry.changed_by }) }}
                </p>
                <p v-if="entry.reason" class="timeline-reason">{{ entry.reason }}</p>
              </div>
            </div>
          </div>
        </div>

        <!-- Participants Tab -->
        <div v-if="activeTab === 'participants'" class="tab-panel">
          <div v-if="participantsLoading" class="loading">{{ t('common.loading') }}</div>
          <template v-else>
            <!-- Stats Summary -->
            <div v-if="participants.length > 0" class="participant-stats">
              <div class="stat-card">
                <span class="stat-value">{{ participantStats.total }}</span>
                <span class="stat-label">{{ t('contestDetail.participants.stats.totalParticipants') }}</span>
              </div>
              <div class="stat-card">
                <span class="stat-value">{{ participantStats.activeTraders }}</span>
                <span class="stat-label">{{ t('contestDetail.participants.stats.activeTraders') }}</span>
              </div>
              <div class="stat-card">
                <span class="stat-value">{{ formatDollars(participantStats.avgScore) }}</span>
                <span class="stat-label">{{ t('contestDetail.participants.stats.avgScore') }}</span>
              </div>
              <div class="stat-card">
                <span class="stat-value">{{ participantStats.joinedToday }}</span>
                <span class="stat-label">{{ t('contestDetail.participants.stats.joinedToday') }}</span>
              </div>
            </div>

            <!-- Header: count + search + actions -->
            <div class="participants-header">
              <div class="participants-header-left">
                <span class="participants-count">
                  {{ t('contestDetail.participants.total', { count: String(participants.length) }) }}
                </span>
                <span v-if="selectedParticipants.size > 0" class="participants-selected-count">
                  {{ t('contestDetail.participants.selected', { count: String(selectedParticipants.size) }) }}
                </span>
              </div>
              <div class="participants-header-right">
                <select
                  class="participants-sort-select"
                  :value="participantSortField"
                  @change="setParticipantSort(($event.target as HTMLSelectElement).value as 'joined_at' | 'total_score' | 'username')"
                >
                  <option value="joined_at">{{ t('contestDetail.participants.sortJoinedAt') }}</option>
                  <option value="total_score">{{ t('contestDetail.participants.sortScore') }}</option>
                  <option value="username">{{ t('contestDetail.participants.sortUsername') }}</option>
                </select>
                <button
                  v-if="selectedParticipants.size > 0"
                  class="btn btn-ghost btn-sm"
                  @click="exportSelectedParticipants"
                >
                  {{ t('contestDetail.participants.exportSelected') }}
                </button>
                <button
                  class="btn btn-ghost btn-sm"
                  :disabled="participants.length === 0"
                  @click="exportAllParticipants"
                >
                  {{ t('contestDetail.participants.exportAll') }}
                </button>
                <input
                  v-model="participantsSearch"
                  type="text"
                  class="participants-search"
                  :placeholder="t('contestDetail.participants.search')"
                />
              </div>
            </div>

            <div v-if="filteredParticipants.length === 0" class="no-data">
              {{ participants.length === 0 ? t('contestDetail.participants.noParticipants') : t('contestDetail.participants.noResults') }}
            </div>

            <div v-else class="table-wrapper">
              <table class="data-table">
                <thead>
                  <tr>
                    <th class="th-checkbox">
                      <input
                        type="checkbox"
                        :checked="allSelected"
                        :indeterminate="someSelected"
                        @change="toggleSelectAll"
                      />
                    </th>
                    <th class="sortable-th" @click="setParticipantSort('username')">
                      {{ t('contestDetail.participants.username') }}{{ getSortIndicator('username') }}
                    </th>
                    <th class="sortable-th" @click="setParticipantSort('joined_at')">
                      {{ t('contestDetail.participants.joinedAt') }}{{ getSortIndicator('joined_at') }}
                    </th>
                    <th>{{ t('contestDetail.participants.qtyTotal') }}</th>
                    <th>{{ t('contestDetail.participants.qtyAvailable') }}</th>
                    <th>{{ t('contestDetail.participants.status') }}</th>
                    <th v-if="isContestCompleted" class="sortable-th" @click="setParticipantSort('total_score')">
                      {{ t('contestDetail.participants.finalScore') }}{{ getSortIndicator('total_score') }}
                    </th>
                    <th v-if="isContestCompleted">{{ t('contestDetail.participants.finalRank') }}</th>
                    <th v-if="isContestCompleted">{{ t('contestDetail.participants.prizeWon') }}</th>
                    <th>{{ t('contestDetail.participants.actions') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="p in filteredParticipants" :key="p.user_id">
                    <td class="td-checkbox">
                      <input
                        type="checkbox"
                        :checked="selectedParticipants.has(p.user_id)"
                        @change="toggleParticipantSelection(p.user_id)"
                      />
                    </td>
                    <td>
                      <div class="participant-identity">
                        <span class="participant-username">{{ p.username }}</span>
                      </div>
                    </td>
                    <td>{{ formatDateTime(p.joined_at) }}</td>
                    <td>{{ formatDollars(p.qty_total) }}</td>
                    <td>{{ formatDollars(p.qty_available) }}</td>
                    <td>
                      <span :class="['participant-status', p.qty_available > 0 ? 'status-active-badge' : 'status-inactive-badge']">
                        {{ p.qty_available > 0 ? t('contestDetail.participants.active') : t('contestDetail.participants.inactive') }}
                      </span>
                    </td>
                    <td v-if="isContestCompleted">{{ formatDollars(p.total_score) }}</td>
                    <td v-if="isContestCompleted">{{ p.final_rank != null ? `#${p.final_rank}` : '\u2014' }}</td>
                    <td v-if="isContestCompleted">{{ p.final_prize_cents != null ? formatCurrency(p.final_prize_cents) : '\u2014' }}</td>
                    <td>
                      <button
                        class="btn btn-action-red btn-sm"
                        @click="openRemoveModal(p)"
                      >
                        {{ t('contestDetail.participants.remove') }}
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </template>
        </div>

        <!-- Leaderboard Tab -->
        <div v-if="activeTab === 'leaderboard'" class="tab-panel">
          <div v-if="leaderboardLoading && !leaderboardFetched" class="loading">{{ t('common.loading') }}</div>
          <template v-else>
            <!-- Header: participant count + last updated + refresh -->
            <div class="leaderboard-header">
              <div class="leaderboard-meta">
                <span class="leaderboard-total">
                  {{ t('contestDetail.leaderboard.totalParticipants', { count: String(leaderboardTotalParticipants) }) }}
                </span>
                <span v-if="leaderboardUpdatedAt" class="leaderboard-updated">
                  {{ t('contestDetail.leaderboard.lastUpdated', { time: leaderboardUpdatedAgo }) }}
                </span>
                <span v-if="currentStatus === 'running'" class="leaderboard-live-badge">
                  <span class="pulse-dot pulse-green"></span>
                  {{ t('contestDetail.leaderboard.live') }}
                </span>
              </div>
              <button
                class="btn btn-ghost btn-sm"
                :disabled="leaderboardLoading"
                @click="fetchLeaderboard"
              >
                {{ leaderboardLoading ? t('common.loading') : t('contestDetail.leaderboard.refresh') }}
              </button>
            </div>

            <!-- Final results banner for completed contests -->
            <div v-if="isContestCompleted" class="leaderboard-final-banner">
              {{ t('contestDetail.leaderboard.finalResults') }}
            </div>

            <div v-if="leaderboardEntries.length === 0" class="no-data">
              {{ t('contestDetail.leaderboard.noEntries') }}
            </div>

            <div v-else class="table-wrapper">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>{{ t('contestDetail.leaderboard.rank') }}</th>
                    <th>{{ t('contestDetail.leaderboard.username') }}</th>
                    <th>{{ t('contestDetail.leaderboard.totalScore') }}</th>
                    <th>{{ t('contestDetail.leaderboard.realized') }}</th>
                    <th>{{ t('contestDetail.leaderboard.unrealized') }}</th>
                    <th v-if="isContestCompleted">{{ t('contestDetail.leaderboard.prize') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="entry in leaderboardEntries"
                    :key="entry.user_id"
                    :class="['leaderboard-row', getRankClass(entry.rank)]"
                  >
                    <td>
                      <span :class="['rank-badge', getRankClass(entry.rank)]">
                        <template v-if="entry.rank === 1">&#9679;</template>
                        <template v-else-if="entry.rank === 2">&#9679;</template>
                        <template v-else-if="entry.rank === 3">&#9679;</template>
                        #{{ entry.rank }}
                      </span>
                    </td>
                    <td class="leaderboard-username">{{ entry.username }}</td>
                    <td :class="['score-cell', entry.total_score >= 0 ? 'score-positive' : 'score-negative']">
                      {{ formatScore(entry.total_score) }}
                    </td>
                    <td :class="['score-cell', entry.realized_score >= 0 ? 'score-positive' : 'score-negative']">
                      {{ formatScore(entry.realized_score) }}
                    </td>
                    <td :class="['score-cell', entry.unrealized_score >= 0 ? 'score-positive' : 'score-negative']">
                      {{ formatScore(entry.unrealized_score) }}
                    </td>
                    <td v-if="isContestCompleted">
                      {{ entry.rank <= 3 ? formatDollars(prizePool * (entry.rank === 1 ? 0.5 : entry.rank === 2 ? 0.3 : 0.2)) : '\u2014' }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </template>
        </div>
      </div>
    </div>

    <!-- Action Confirmation Modal -->
    <Teleport to="body">
      <div v-if="showModal" class="modal-overlay" @click.self="closeModal">
        <div class="modal-container">
          <div class="modal-header">
            <h3 class="modal-title">{{ modalAction ? getModalTitle(modalAction) : '' }}</h3>
            <button class="modal-close" @click="closeModal">&times;</button>
          </div>
          <div class="modal-body">
            <p class="modal-message">
              {{ modalAction ? getModalMessage(modalAction) : '' }}
            </p>
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
              :class="['btn', modalAction === 'cancel' ? 'btn-action-red' : 'btn-primary']"
              :disabled="modalLoading"
              @click="confirmAction"
            >
              {{ modalLoading ? t('common.loading') : t('common.confirm') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Remove Participant Confirmation Modal -->
    <Teleport to="body">
      <div v-if="showRemoveModal" class="modal-overlay" @click.self="closeRemoveModal">
        <div class="modal-container">
          <div class="modal-header">
            <h3 class="modal-title">{{ t('contestDetail.participants.removeTitle') }}</h3>
            <button class="modal-close" @click="closeRemoveModal">&times;</button>
          </div>
          <div class="modal-body">
            <p class="modal-message">
              {{ t('contestDetail.participants.removeMessage', { username: removeTarget?.username ?? '' }) }}
            </p>
            <div v-if="contest && !contest.is_free" class="remove-refund-warning">
              {{ t('contestDetail.participants.removeRefundWarning') }}
            </div>
          </div>
          <div class="modal-footer">
            <button
              class="btn btn-ghost"
              :disabled="removeLoading"
              @click="closeRemoveModal"
            >
              {{ t('common.cancel') }}
            </button>
            <button
              class="btn btn-action-red"
              :disabled="removeLoading"
              @click="confirmRemoveParticipant"
            >
              {{ removeLoading ? t('common.loading') : t('contestDetail.participants.remove') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.contest-detail-page {
  padding: var(--spacing-lg) 0;
}

/* Loading */
.loading-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 400px;
}

.loading {
  color: var(--color-text-secondary);
}

/* Breadcrumb */
.breadcrumb {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  margin-bottom: var(--spacing-md);
  font-size: var(--font-size-sm);
}

.breadcrumb-link {
  color: var(--color-primary, #2563EB);
  text-decoration: none;
}

.breadcrumb-link:hover {
  text-decoration: underline;
}

.breadcrumb-separator {
  color: var(--color-text-muted);
}

.breadcrumb-current {
  color: var(--color-text-secondary);
}

/* Header */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: var(--spacing-xl);
  gap: var(--spacing-lg);
}

.header-info {
  flex: 1;
  min-width: 0;
}

.header-title-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-xs);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.contest-description {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  margin: 0;
}

.header-actions {
  display: flex;
  gap: var(--spacing-sm);
  flex-shrink: 0;
  flex-wrap: wrap;
}

/* Status Badge */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
  white-space: nowrap;
}

.status-badge-sm {
  padding: 2px var(--spacing-xs);
  font-size: 10px;
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
  0% { box-shadow: 0 0 0 0 rgba(22, 163, 74, 0.5); }
  70% { box-shadow: 0 0 0 6px rgba(22, 163, 74, 0); }
  100% { box-shadow: 0 0 0 0 rgba(22, 163, 74, 0); }
}

@keyframes pulse-yellow {
  0% { box-shadow: 0 0 0 0 rgba(202, 138, 4, 0.5); }
  70% { box-shadow: 0 0 0 6px rgba(202, 138, 4, 0); }
  100% { box-shadow: 0 0 0 0 rgba(202, 138, 4, 0); }
}

.status-running { background-color: #DCFCE7; color: #16A34A; }
.status-scheduled { background-color: #DBEAFE; color: #2563EB; }
.status-registration { background-color: #FEF3C7; color: #D97706; }
.status-registration-closed { background-color: #FED7AA; color: #C2410C; }
.status-completed { background-color: var(--color-bg-tertiary); color: var(--color-text-secondary); }
.status-draft { background-color: #F3F4F6; color: #6B7280; }
.status-paused { background-color: #FEE2E2; color: #DC2626; }
.status-cancelled { background-color: #FEE2E2; color: #DC2626; }
.status-settling { background-color: #EDE9FE; color: #7C3AED; }

/* Info Cards */
.info-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-xl);
}

.info-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.info-card-label {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.info-card-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.info-card-time {
  font-size: var(--font-size-lg);
}

.info-card-secondary {
  font-weight: 400;
  color: var(--color-text-muted);
  font-size: var(--font-size-lg);
}

.info-card-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

/* Progress bar */
.progress-bar {
  width: 100%;
  height: 6px;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-full);
  overflow: hidden;
  margin-top: var(--spacing-xs);
}

.progress-fill {
  height: 100%;
  background-color: var(--color-primary, #6366F1);
  border-radius: var(--radius-full);
  transition: width 0.3s ease;
}

/* Tabs */
.tabs {
  display: flex;
  gap: var(--spacing-xs);
  border-bottom: 1px solid var(--color-border);
  margin-bottom: var(--spacing-lg);
  overflow-x: auto;
}

.tab-button {
  padding: var(--spacing-sm) var(--spacing-md);
  border: none;
  background: none;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all var(--transition-fast);
  white-space: nowrap;
}

.tab-button:hover {
  color: var(--color-text-primary);
}

.tab-button.active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
}

/* Tab Content */
.tab-content {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
}

.tab-panel {
  padding: var(--spacing-lg);
}

/* Overview Grid */
.overview-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-lg);
}

.overview-section {
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  padding: var(--spacing-lg);
}

.section-title {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: var(--spacing-md);
}

.info-rows {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.info-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.info-value {
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  font-weight: 500;
}

.info-value.mono {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-xs);
}

/* Symbols */
.symbols-section {
  margin-top: var(--spacing-lg);
}

.symbols-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}

.symbol-badge {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-sm);
  background-color: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  font-family: var(--font-family-mono);
}

/* Timeline */
.timeline {
  display: flex;
  flex-direction: column;
}

.timeline-item {
  display: flex;
  gap: var(--spacing-md);
  position: relative;
}

.timeline-marker {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex-shrink: 0;
  width: 24px;
}

.timeline-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  flex-shrink: 0;
  margin-top: 4px;
}

.timeline-dot.status-running { background-color: #16A34A; }
.timeline-dot.status-scheduled { background-color: #2563EB; }
.timeline-dot.status-registration { background-color: #D97706; }
.timeline-dot.status-registration-closed { background-color: #C2410C; }
.timeline-dot.status-completed { background-color: #6B7280; }
.timeline-dot.status-draft { background-color: #9CA3AF; }
.timeline-dot.status-paused { background-color: #DC2626; }
.timeline-dot.status-cancelled { background-color: #DC2626; }
.timeline-dot.status-settling { background-color: #7C3AED; }

.timeline-line {
  width: 2px;
  flex: 1;
  background-color: var(--color-border);
  min-height: 24px;
}

.timeline-content {
  padding-bottom: var(--spacing-lg);
  flex: 1;
}

.timeline-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-xs);
}

.timeline-date {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.timeline-meta {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  margin: 0;
}

.timeline-reason {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: var(--spacing-xs) 0 0 0;
  font-style: italic;
}

/* Leaderboard Tab */
.leaderboard-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-md);
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.leaderboard-meta {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.leaderboard-total {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.leaderboard-updated {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  white-space: nowrap;
}

.leaderboard-live-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: #16A34A;
  text-transform: uppercase;
}

.leaderboard-final-banner {
  background-color: #DBEAFE;
  color: #1E40AF;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 600;
  text-align: center;
  margin-bottom: var(--spacing-md);
}

.leaderboard-row.rank-gold {
  background-color: rgba(255, 215, 0, 0.08);
}

.leaderboard-row.rank-silver {
  background-color: rgba(192, 192, 192, 0.08);
}

.leaderboard-row.rank-bronze {
  background-color: rgba(205, 127, 50, 0.08);
}

.rank-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.rank-badge.rank-gold {
  color: #B8860B;
}

.rank-badge.rank-silver {
  color: #6B7280;
}

.rank-badge.rank-bronze {
  color: #92400E;
}

.leaderboard-username {
  font-weight: 500;
}

.score-cell {
  font-family: var(--font-family-mono);
  font-weight: 500;
}

.score-positive {
  color: #16A34A;
}

.score-negative {
  color: #DC2626;
}

/* No data */
.no-data {
  text-align: center;
  padding: var(--spacing-xl);
  color: var(--color-text-muted);
}

/* Participant Stats */
.participant-stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.stat-card {
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
}

.stat-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.stat-label {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  text-align: center;
}

/* Participants Tab */
.participants-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-md);
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.participants-header-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.participants-header-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.participants-count {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.participants-selected-count {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-primary);
  white-space: nowrap;
}

.participants-sort-select {
  padding: var(--spacing-xs) var(--spacing-sm);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  cursor: pointer;
}

.participants-sort-select:focus {
  outline: none;
  border-color: var(--color-primary);
}

.participants-search {
  max-width: 220px;
  width: 100%;
  padding: var(--spacing-xs) var(--spacing-sm);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
}

.participants-search:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.2);
}

.th-checkbox,
.td-checkbox {
  width: 40px;
  text-align: center;
}

.th-checkbox input[type="checkbox"],
.td-checkbox input[type="checkbox"] {
  cursor: pointer;
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary);
}

.sortable-th {
  cursor: pointer;
  user-select: none;
}

.sortable-th:hover {
  color: var(--color-text-primary);
}

/* Remove participant warning */
.remove-refund-warning {
  background-color: #FEF3C7;
  color: #92400E;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  margin-top: var(--spacing-sm);
  line-height: 1.5;
}

.table-wrapper {
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--font-size-sm);
}

.data-table th {
  text-align: left;
  padding: var(--spacing-sm) var(--spacing-md);
  font-weight: 600;
  color: var(--color-text-muted);
  text-transform: uppercase;
  font-size: var(--font-size-xs);
  letter-spacing: 0.05em;
  border-bottom: 1px solid var(--color-border);
  white-space: nowrap;
}

.data-table td {
  padding: var(--spacing-sm) var(--spacing-md);
  color: var(--color-text-primary);
  border-bottom: 1px solid var(--color-border);
  white-space: nowrap;
}

.data-table tbody tr:hover {
  background-color: var(--color-bg-secondary);
}

.data-table tbody tr:last-child td {
  border-bottom: none;
}

.participant-identity {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.participant-username {
  font-weight: 500;
  color: var(--color-text-primary);
}

.participant-status {
  display: inline-block;
  padding: 2px var(--spacing-xs);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
}

.status-active-badge {
  background-color: #DCFCE7;
  color: #16A34A;
}

.status-inactive-badge {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-muted);
}

/* Action Buttons */
.btn {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all var(--transition-fast);
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background-color: var(--color-primary);
  color: white;
}

.btn-primary:hover:not(:disabled) {
  background-color: var(--color-primary-dark);
}

.btn-ghost {
  background-color: transparent;
  color: var(--color-text-secondary);
  border: 1px solid var(--color-border);
}

.btn-ghost:hover:not(:disabled) {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
}

.btn-action-green { background-color: #16A34A; color: white; }
.btn-action-green:hover:not(:disabled) { background-color: #15803D; }

.btn-action-blue { background-color: #2563EB; color: white; }
.btn-action-blue:hover:not(:disabled) { background-color: #1D4ED8; }

.btn-action-orange { background-color: #D97706; color: white; }
.btn-action-orange:hover:not(:disabled) { background-color: #B45309; }

.btn-action-yellow { background-color: #CA8A04; color: white; }
.btn-action-yellow:hover:not(:disabled) { background-color: #A16207; }

.btn-action-red { background-color: #DC2626; color: white; }
.btn-action-red:hover:not(:disabled) { background-color: #B91C1C; }

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

/* Responsive */
@media (max-width: 1023px) {
  .info-cards {
    grid-template-columns: repeat(2, 1fr);
  }

  .overview-grid {
    grid-template-columns: 1fr;
  }

  .participant-stats {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 767px) {
  .page-header {
    flex-direction: column;
  }

  .header-actions {
    width: 100%;
  }

  .header-title-row {
    flex-wrap: wrap;
  }

  .info-cards {
    grid-template-columns: 1fr;
  }

  .participant-stats {
    grid-template-columns: 1fr 1fr;
  }

  .participants-header {
    flex-direction: column;
    align-items: stretch;
  }

  .participants-header-right {
    flex-wrap: wrap;
  }

  .participants-search {
    max-width: 100%;
  }

  .tabs {
    -webkit-overflow-scrolling: touch;
  }

  .modal-container {
    max-width: 100%;
    margin: var(--spacing-sm);
  }
}

/* RTL */
[dir="rtl"] .breadcrumb-separator {
  transform: scaleX(-1);
}

[dir="rtl"] .info-row {
  flex-direction: row-reverse;
}

[dir="rtl"] .timeline-item {
  flex-direction: row-reverse;
}
</style>
