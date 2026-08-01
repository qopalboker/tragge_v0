<script setup lang="ts">
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { redirectToTrade } from '@/utils/tradeRedirect';
import type { MyTournamentEntry, MyTournamentStatus } from '@/api';

const props = defineProps<{
  contest: MyTournamentEntry;
  type: MyTournamentStatus;
}>();

const router = useRouter();
const authStore = useAuthStore();

const isActive = computed(() => props.type === 'active');
const isUpcoming = computed(() => props.type === 'upcoming');
const isCompleted = computed(() => props.type === 'completed');
const isCancelled = computed(() => props.type === 'cancelled');

const pnlClass = computed(() => {
  if (!isActive.value && !isCompleted.value) return '';
  return props.contest.total_score >= 0 ? 'positive' : 'negative';
});

function formatPnl(value: number): string {
  const prefix = value >= 0 ? '+' : '';
  return `${prefix}$${Math.abs(value).toFixed(2)}`;
}

function formatEntryFee(cents: number, isFree: boolean): string {
  if (isFree || cents === 0) return t('myTournaments.free');
  return `$${(cents / 100).toFixed(0)}`;
}

function formatTimeLeft(endsAt: string): string {
  const end = new Date(endsAt).getTime();
  const now = Date.now();
  const diff = end - now;
  if (diff <= 0) return '0:00';
  const h = Math.floor(diff / 3600000);
  const m = Math.floor((diff % 3600000) / 60000);
  const s = Math.floor((diff % 60000) / 1000);
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  return `${m}:${String(s).padStart(2, '0')}`;
}

function formatCountdown(startsAt: string): string {
  const start = new Date(startsAt).getTime();
  const now = Date.now();
  const diff = start - now;
  if (diff <= 0) return t('myTournaments.startingSoon');
  const d = Math.floor(diff / 86400000);
  const h = Math.floor((diff % 86400000) / 3600000);
  const m = Math.floor((diff % 3600000) / 60000);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

async function handleCardClick(): Promise<void> {
  const contestId = props.contest.contest_id;

  if (isActive.value) {
    if (authStore.isAuthenticated) {
      await redirectToTrade(contestId);
    } else {
      router.push({ name: 'login' });
    }
  } else if (isUpcoming.value) {
    router.push({ name: 'contest-details', params: { contestId } });
  } else if (isCompleted.value) {
    router.push({ name: 'contest-results', params: { contestId } });
  } else if (isCancelled.value) {
    router.push({ name: 'contest-details', params: { contestId } });
  }
}
</script>

<template>
  <div class="my-contest-card card clickable" @click="handleCardClick">
    <!-- Contest Name -->
    <div class="card-header">
      <span class="contest-name">{{ contest.contest_name }}</span>
      <span v-if="isActive" class="time-left">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <polyline points="12 6 12 12 16 14" />
        </svg>
        {{ formatTimeLeft(contest.ends_at) }}
      </span>
      <span v-if="isCancelled" class="status-badge cancelled-badge">
        {{ t('myTournaments.cancelled') }}
      </span>
    </div>

    <!-- Active Contest Content -->
    <template v-if="isActive">
      <div class="active-stats">
        <div class="stat-block">
          <span class="stat-label">{{ t('myTournaments.position') }}</span>
          <span class="stat-value">
            {{ contest.final_rank ? `#${contest.final_rank}` : '-' }} / {{ contest.total_participants }}
          </span>
        </div>
        <div class="stat-block">
          <span class="stat-label">{{ t('myTournaments.pnl') }}</span>
          <span :class="['stat-value', 'pnl', pnlClass]">
            {{ formatPnl(contest.total_score) }}
          </span>
        </div>
      </div>
      <span class="btn btn-primary action-btn">
        {{ t('myTournaments.continue') }}
      </span>
    </template>

    <!-- Upcoming Contest Content -->
    <template v-if="isUpcoming">
      <div class="upcoming-stats">
        <div class="stat-item">
          <span class="stat-label">{{ t('myTournaments.startsIn') }}</span>
          <span class="stat-value">{{ formatCountdown(contest.starts_at) }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('contest.entry') }}</span>
          <span class="stat-value">{{ formatEntryFee(contest.entry_fee_cents, contest.is_free) }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('myTournaments.participants') }}</span>
          <span class="stat-value">{{ contest.total_participants }}</span>
        </div>
      </div>
    </template>

    <!-- Completed Contest Content -->
    <template v-if="isCompleted">
      <div class="completed-stats">
        <div class="stat-item">
          <span class="stat-label">{{ t('myTournaments.finalPosition') }}</span>
          <span class="stat-value">
            {{ contest.final_rank ? `#${contest.final_rank}` : '-' }} / {{ contest.total_participants }}
          </span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('myTournaments.pnl') }}</span>
          <span :class="['stat-value', pnlClass]">
            {{ formatPnl(contest.total_score) }}
          </span>
        </div>
        <div v-if="contest.final_prize_cents" class="stat-item">
          <span class="stat-label">{{ t('myTournaments.reward') }}</span>
          <span class="stat-value reward">${{ (contest.final_prize_cents / 100).toFixed(0) }}</span>
        </div>
      </div>
      <span class="btn btn-secondary action-btn">
        {{ t('myTournaments.results') }}
      </span>
    </template>

    <!-- Cancelled Contest Content -->
    <template v-if="isCancelled">
      <div class="cancelled-stats">
        <div class="stat-item">
          <span class="stat-label">{{ t('contest.entry') }}</span>
          <span class="stat-value">{{ formatEntryFee(contest.entry_fee_cents, contest.is_free) }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">{{ t('myTournaments.participants') }}</span>
          <span class="stat-value">{{ contest.total_participants }}</span>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.my-contest-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.my-contest-card.clickable {
  cursor: pointer;
  transition: transform var(--transition-fast), box-shadow var(--transition-fast);
}

.my-contest-card.clickable:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.contest-name {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
}

.time-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

.status-badge {
  font-size: var(--font-size-xs);
  padding: 2px var(--spacing-sm);
  border-radius: var(--radius-sm);
  font-weight: 500;
}

.cancelled-badge {
  background-color: var(--color-danger-bg, rgba(239, 68, 68, 0.1));
  color: var(--color-danger);
}

.active-stats {
  display: flex;
  gap: var(--spacing-lg);
}

.stat-block {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.stat-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.stat-value {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.pnl {
  font-variant-numeric: tabular-nums;
}

.positive {
  color: var(--color-success);
}

.negative {
  color: var(--color-danger);
}

.reward {
  color: var(--color-warning);
}

.upcoming-stats,
.completed-stats,
.cancelled-stats {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-md) var(--spacing-lg);
}

.stat-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.action-btn {
  align-self: flex-start;
}

@media (max-width: 767px) {
  .active-stats {
    flex-direction: column;
    gap: var(--spacing-sm);
  }

  .action-btn {
    width: 100%;
  }
}
</style>
