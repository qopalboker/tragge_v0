<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { api } from '@/api';
import PrizeTable from '@/components/contests/PrizeTable.vue';
import type { PrizeRow } from '@/modules/user/components/contests/PrizeTable.vue';

interface PrizePreviewData {
  contest_id: string;
  current_participants: number;
  min_participants: number;
  quorum_met: boolean;
  entry_fee_cents: number;
  commission_rate: number;
  prize_pool_cents: number;
  winners_count: number;
  prizes: PrizeRow[];
  status: string;
  message: string;
}

const props = defineProps<{
  contestId: string;
  status: string;
  startsAt: string;
}>();

const emit = defineEmits<{
  (e: 'participant-update', count: number): void;
}>();

const router = useRouter();

// State
const data = ref<PrizePreviewData | null>(null);
const loading = ref(true);
const cancelled = ref(false);
let pollTimer: ReturnType<typeof setInterval> | null = null;

// Computed
const isPreStart = computed(() => {
  return props.status === 'registration_open' || props.status === 'scheduled';
});

const formattedStartTime = computed(() => {
  if (!props.startsAt) return '';
  const date = new Date(props.startsAt);
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
});

const participantsNeeded = computed(() => {
  if (!data.value) return 0;
  return Math.max(0, data.value.min_participants - data.value.current_participants);
});

// Fetch data
async function fetchData(): Promise<void> {
  try {
    const response = await api.get<PrizePreviewData>(
      `/api/user/contests/${props.contestId}/prize-preview`
    );
    const prev = data.value;
    data.value = response.data;

    // Emit participant count update if changed
    if (!prev || prev.current_participants !== response.data.current_participants) {
      emit('participant-update', response.data.current_participants);
    }

    // Check for contest cancellation
    if (response.data.status === 'cancelled') {
      cancelled.value = true;
      stopPolling();
    }
  } catch (err) {
    console.error('Failed to fetch contest status:', err);
  } finally {
    loading.value = false;
  }
}

// Polling
function startPolling(): void {
  stopPolling();
  if (isPreStart.value) {
    pollTimer = setInterval(fetchData, 10000);
  }
}

function stopPolling(): void {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

// Handle cancellation redirect
function handleCancelledRedirect(): void {
  router.push('/user/contests');
}

// Watch for status changes
watch(() => props.status, () => {
  if (isPreStart.value) {
    startPolling();
  } else {
    stopPolling();
  }
});

// Lifecycle
onMounted(async () => {
  await fetchData();
  startPolling();
});

onUnmounted(() => {
  stopPolling();
});
</script>

<template>
  <!-- Only show for pre-start contests -->
  <div v-if="isPreStart && !loading" class="contest-status-banner">
    <!-- Cancelled State -->
    <div v-if="cancelled" class="banner-cancelled">
      <div class="banner-header">
        <span class="status-dot cancelled-dot"></span>
        <span class="banner-title">{{ t('contestBanner.cancelled') }}</span>
      </div>
      <p class="cancelled-message">{{ t('contestBanner.cancelledMessage') }}</p>
      <button class="btn-redirect" @click="handleCancelledRedirect">
        {{ t('contestBanner.backToContests') }}
      </button>
    </div>

    <!-- State 1: Quorum Met -->
    <div v-else-if="data && data.quorum_met" class="banner-quorum-met">
      <div class="banner-header">
        <span class="status-dot ready-dot"></span>
        <span class="banner-title">{{ t('contestBanner.readyToStart') }}</span>
      </div>
      <div class="banner-subtitle">
        <span>{{ t('contestBanner.participantCount', { count: data.current_participants }) }}</span>
        <span class="subtitle-separator">|</span>
        <span>{{ t('contestBanner.startTime', { time: formattedStartTime }) }}</span>
      </div>

      <PrizeTable
        :prizes="data.prizes"
        :prize-pool-cents="data.prize_pool_cents"
        :commission-rate="data.commission_rate || 20"
      />

      <div class="banner-note">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 8v4" />
          <circle cx="12" cy="16" r="0.5" fill="currentColor" />
        </svg>
        <span>{{ t('contestBanner.prizesNote') }}</span>
      </div>
    </div>

    <!-- State 2: Quorum Not Met -->
    <div v-else-if="data && !data.quorum_met" class="banner-quorum-not-met">
      <div class="banner-header">
        <span class="status-dot waiting-dot"></span>
        <span class="banner-title">{{ t('contestBanner.waitingForParticipants') }}</span>
      </div>
      <div class="banner-subtitle">
        <span>{{ t('contestBanner.participantProgress', { current: data.current_participants, min: data.min_participants }) }}</span>
        <span class="subtitle-separator">|</span>
        <span>{{ t('contestBanner.startTime', { time: formattedStartTime }) }}</span>
      </div>

      <div class="quorum-warning-box">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
          <line x1="12" y1="9" x2="12" y2="13" />
          <line x1="12" y1="17" x2="12.01" y2="17" />
        </svg>
        <p>{{ t('contestBanner.quorumWarning') }}</p>
      </div>

      <div class="need-more">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M23 4v6h-6" />
          <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
        </svg>
        <span>{{ t('contestBanner.needMore', { count: participantsNeeded }) }}</span>
      </div>
    </div>
  </div>

  <!-- Loading skeleton -->
  <div v-else-if="isPreStart && loading" class="contest-status-banner banner-loading">
    <div class="skeleton-line skeleton-header"></div>
    <div class="skeleton-line skeleton-subtitle"></div>
    <div class="skeleton-line skeleton-body"></div>
  </div>
</template>

<style scoped>
.contest-status-banner {
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

/* Banner Header */
.banner-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-md) var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg-secondary);
}

.banner-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
}

.status-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.ready-dot {
  background: #16A34A;
  box-shadow: 0 0 0 3px rgba(22, 163, 74, 0.2);
}

.waiting-dot {
  background: #CA8A04;
  box-shadow: 0 0 0 3px rgba(202, 138, 4, 0.2);
}

.cancelled-dot {
  background: #DC2626;
  box-shadow: 0 0 0 3px rgba(220, 38, 38, 0.2);
}

/* Banner Subtitle */
.banner-subtitle {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-lg);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.subtitle-separator {
  color: var(--color-border);
}

/* Quorum Met Content */
.banner-quorum-met .prize-table-wrapper {
  padding: 0 var(--spacing-lg);
}

.banner-quorum-met :deep(.prize-table-wrapper) {
  padding: 0 var(--spacing-md);
}

.banner-quorum-met :deep(.prize-footer) {
  margin: var(--spacing-sm) var(--spacing-md);
}

/* Banner Note */
.banner-note {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-lg);
  margin: var(--spacing-sm) var(--spacing-md) var(--spacing-md);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.banner-note svg {
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.banner-note span {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  line-height: 1.4;
}

/* Quorum Not Met Content */
.banner-quorum-not-met {
  padding-bottom: var(--spacing-md);
}

.quorum-warning-box {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-sm);
  margin: var(--spacing-sm) var(--spacing-lg);
  padding: var(--spacing-md);
  background: #FEF3C7;
  border-radius: var(--radius-md);
  border: 1px solid #FDE68A;
}

.quorum-warning-box svg {
  color: #D97706;
  flex-shrink: 0;
  margin-top: 1px;
}

.quorum-warning-box p {
  margin: 0;
  font-size: var(--font-size-sm);
  color: #78350F;
  line-height: 1.5;
}

.need-more {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin: var(--spacing-sm) var(--spacing-lg) 0;
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.need-more svg {
  color: var(--color-primary);
  flex-shrink: 0;
}

.need-more span {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

/* Cancelled State */
.banner-cancelled {
  padding-bottom: var(--spacing-lg);
}

.cancelled-message {
  margin: 0;
  padding: var(--spacing-sm) var(--spacing-lg);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  line-height: 1.5;
}

.btn-redirect {
  margin: var(--spacing-sm) var(--spacing-lg) 0;
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-primary);
  color: white;
  border: none;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: background-color var(--transition-fast);
}

.btn-redirect:hover {
  background: var(--color-primary-dark);
}

/* Loading Skeleton */
.banner-loading {
  padding: var(--spacing-lg);
}

.skeleton-line {
  background: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  animation: skeleton-pulse 1.5s ease-in-out infinite;
}

.skeleton-header {
  width: 60%;
  height: 20px;
  margin-bottom: var(--spacing-sm);
}

.skeleton-subtitle {
  width: 40%;
  height: 16px;
  margin-bottom: var(--spacing-md);
}

.skeleton-body {
  width: 100%;
  height: 80px;
}

@keyframes skeleton-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* RTL Support */
[dir="rtl"] .banner-header {
  flex-direction: row-reverse;
}

[dir="rtl"] .banner-subtitle {
  flex-direction: row-reverse;
}

[dir="rtl"] .quorum-warning-box {
  flex-direction: row-reverse;
}

[dir="rtl"] .need-more {
  flex-direction: row-reverse;
}

[dir="rtl"] .banner-note {
  flex-direction: row-reverse;
}

/* Mobile */
@media (max-width: 767px) {
  .banner-header {
    padding: var(--spacing-sm) var(--spacing-md);
  }

  .banner-subtitle {
    padding: var(--spacing-xs) var(--spacing-md);
    flex-wrap: wrap;
  }

  .quorum-warning-box {
    margin: var(--spacing-sm) var(--spacing-md);
  }

  .need-more {
    margin: var(--spacing-sm) var(--spacing-md) 0;
  }

  .banner-note {
    margin: var(--spacing-sm) var(--spacing-sm) var(--spacing-sm);
    padding: var(--spacing-sm);
  }

  .cancelled-message {
    padding: var(--spacing-sm) var(--spacing-md);
  }

  .btn-redirect {
    margin: var(--spacing-sm) var(--spacing-md) 0;
  }
}
</style>
