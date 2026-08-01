<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { useContestsStore, type Contest } from '@/stores/contests';
import { freeTournamentsApi } from '@/api';
import { useToast } from '@/composables/useToast';
import { redirectToTrade } from '@/utils/tradeRedirect';

// When DashboardPage already fetched contests for the page, it passes
// the free subset down. We reuse that data instead of issuing our own
// /api/user/contests request — Phase B Fix 4. The internal fetch is
// retained as a fallback path for any future standalone mount.
const props = defineProps<{ contests?: Contest[] }>();

const router = useRouter();
const authStore = useAuthStore();
const contestsStore = useContestsStore();
const toast = useToast();

const liveTournaments = ref<Contest[]>([]);
const upcomingTournaments = ref<Contest[]>([]);
const loading = ref(true);
const error = ref<string | null>(null);
const joiningId = ref<string | null>(null);

// Countdown state for live tournaments (time remaining)
const liveCountdowns = ref<Record<string, { hours: number; minutes: number; seconds: number }>>({});

// Countdown state for upcoming tournaments (starts in)
const upcomingCountdowns = ref<Record<string, { days: number; hours: number; minutes: number; seconds: number }>>({});

// User rank in joined contests (mock data - would come from API in real implementation)
const userRanks = ref<Record<string, { rank: number; totalParticipants: number }>>({});

const refreshPending = ref(false);
let countdownInterval: ReturnType<typeof setInterval> | null = null;

const isAuthenticated = computed(() => authStore.isAuthenticated);

// Check if there are any tournaments available
const hasTournaments = computed(() =>
  liveTournaments.value.length > 0 || upcomingTournaments.value.length > 0
);

// Get next available time for free tournaments (mock schedule info)
const nextScheduledTime = computed(() => {
  const now = new Date();
  // Free practice tournaments run at the top of every hour on weekdays
  const nextHour = new Date(now);
  nextHour.setMinutes(0, 0, 0);
  nextHour.setHours(nextHour.getHours() + 1);

  return nextHour.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
});

function updateLiveCountdown(contest: Contest): void {
  const now = new Date().getTime();
  const endTime = new Date(contest.ends_at).getTime();
  const diff = endTime - now;

  if (diff <= 0) {
    liveCountdowns.value[contest.id] = { hours: 0, minutes: 0, seconds: 0 };
    if (!refreshPending.value && !loading.value) {
      refreshPending.value = true;
      fetchFreeTournaments().finally(() => {
        setTimeout(() => { refreshPending.value = false; }, 30000);
      });
    }
    return;
  }

  liveCountdowns.value[contest.id] = {
    hours: Math.floor(diff / (1000 * 60 * 60)),
    minutes: Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60)),
    seconds: Math.floor((diff % (1000 * 60)) / 1000),
  };
}

function updateUpcomingCountdown(contest: Contest): void {
  const now = new Date().getTime();
  const startTime = new Date(contest.starts_at).getTime();
  const diff = startTime - now;

  if (diff <= 0) {
    upcomingCountdowns.value[contest.id] = { days: 0, hours: 0, minutes: 0, seconds: 0 };
    if (!refreshPending.value && !loading.value) {
      refreshPending.value = true;
      fetchFreeTournaments().finally(() => {
        setTimeout(() => { refreshPending.value = false; }, 30000);
      });
    }
    return;
  }

  upcomingCountdowns.value[contest.id] = {
    days: Math.floor(diff / (1000 * 60 * 60 * 24)),
    hours: Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60)),
    minutes: Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60)),
    seconds: Math.floor((diff % (1000 * 60)) / 1000),
  };
}

function updateAllCountdowns(): void {
  liveTournaments.value.forEach(updateLiveCountdown);
  upcomingTournaments.value.forEach(updateUpcomingCountdown);
}

function applyContests(data: Contest[]): void {
  // Find live tournaments (running)
  liveTournaments.value = data.filter(c => c.status === 'running').slice(0, 2);

  // Find upcoming tournaments (registration_open or scheduled)
  upcomingTournaments.value = data
    .filter(c => c.status === 'registration_open' || c.status === 'scheduled')
    .sort((a, b) => new Date(a.starts_at).getTime() - new Date(b.starts_at).getTime())
    .slice(0, 2);

  // Initialize countdowns
  updateAllCountdowns();

  // Mock user ranks for joined contests (in real app, this would come from API)
  liveTournaments.value.forEach((contest: Contest) => {
    if (contestsStore.isJoined(contest.id)) {
      userRanks.value[contest.id] = {
        rank: Math.floor(Math.random() * 50) + 1,
        totalParticipants: contest.participant_count || 100,
      };
    }
  });
}

async function fetchFreeTournaments(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    const data = await freeTournamentsApi.getFreeTournaments();
    applyContests(data);
  } catch (err) {
    error.value = t('freeTournaments.loadError');
  } finally {
    loading.value = false;
  }
}

async function handleJoin(contest: Contest): Promise<void> {
  if (!isAuthenticated.value) {
    router.push('/user/login');
    return;
  }

  joiningId.value = contest.id;

  try {
    await contestsStore.joinContest(contest.id);
    toast.success(t('freePractice.welcomeMessage'));

    // If it's a running contest, redirect to trading
    if (contest.status === 'running' && isAuthenticated.value) {
      await redirectToTrade(contest.id);
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : t('common.error');
    toast.error(message);
  } finally {
    joiningId.value = null;
  }
}

async function handleContinueTrading(contest: Contest): Promise<void> {
  if (isAuthenticated.value) {
    await redirectToTrade(contest.id);
  }
}

function formatTimeRemaining(countdown: { hours: number; minutes: number; seconds: number } | undefined): string {
  if (!countdown) return '0m';

  if (countdown.hours > 0) {
    return `${countdown.hours}h ${countdown.minutes}m`;
  }
  return `${countdown.minutes}m ${countdown.seconds}s`;
}

function formatStartsIn(countdown: { days: number; hours: number; minutes: number; seconds: number } | undefined): string {
  if (!countdown) return '0m';

  if (countdown.days > 0) {
    return `${countdown.days}d ${countdown.hours}h`;
  }
  if (countdown.hours > 0) {
    return `${countdown.hours}h ${countdown.minutes}m`;
  }
  return `${countdown.minutes}m`;
}

function isJoined(contestId: string): boolean {
  return contestsStore.isJoined(contestId);
}

function isJoining(contestId: string): boolean {
  return joiningId.value === contestId;
}

function getParticipantCount(contest: Contest): number {
  return contest.participant_count ?? Math.floor(Math.random() * 200) + 20;
}

// React to the parent-supplied list. When DashboardPage hands us
// data, apply it immediately and skip the internal fetch. If the
// parent's list updates (e.g., refresh), re-apply.
watch(
  () => props.contests,
  (next) => {
    if (next && next.length > 0) {
      applyContests(next);
      loading.value = false;
    }
  },
  { immediate: true },
);

onMounted(() => {
  // Only fetch if the parent did not provide data — preserves
  // standalone-mount behaviour.
  if (!props.contests || props.contests.length === 0) {
    fetchFreeTournaments();
  }
  countdownInterval = setInterval(updateAllCountdowns, 1000);
});

onUnmounted(() => {
  if (countdownInterval) {
    clearInterval(countdownInterval);
  }
});
</script>

<template>
  <section class="free-practice-section card">
    <!-- Header -->
    <div class="section-header">
      <div class="header-left">
        <span class="header-icon">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
            <path d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </span>
        <div class="header-text">
          <h2 class="section-title">{{ t('freePractice.title') }}</h2>
          <p class="section-subtitle">{{ t('freePractice.subtitle') }}</p>
        </div>
      </div>
      <span class="free-badge">{{ t('freeTournaments.free') }}</span>
    </div>

    <!-- Content -->
    <div class="section-content">
      <!-- Loading State -->
      <div v-if="loading" class="state-container loading-state">
        <span class="spinner"></span>
        <span>{{ t('common.loading') }}</span>
      </div>

      <!-- Error State -->
      <div v-else-if="error" class="state-container error-state">
        <span>{{ error }}</span>
        <button class="btn btn-secondary btn-sm" @click="fetchFreeTournaments">
          {{ t('common.retry') }}
        </button>
      </div>

      <!-- Empty State - No Tournaments Available -->
      <div v-else-if="!hasTournaments" class="state-container empty-state">
        <div class="empty-icon">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <circle cx="12" cy="12" r="10" />
            <path d="M12 6v6l4 2" />
          </svg>
        </div>
        <p class="empty-title">{{ t('freePractice.noContestsTitle') }}</p>
        <p class="empty-description">
          {{ t('freePractice.noContestsDesc') }}
        </p>
        <p class="schedule-info">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 8v4l3 3" />
            <circle cx="12" cy="12" r="10" />
          </svg>
          {{ t('freePractice.nextAt', { time: nextScheduledTime }) }}
        </p>
        <RouterLink to="/user/contests?filter=free" class="btn btn-secondary btn-sm">
          {{ t('freeTournaments.browseAll') }}
        </RouterLink>
      </div>

      <!-- Tournaments Grid -->
      <div v-else class="tournaments-grid">
        <!-- Live Tournaments -->
        <div
          v-for="contest in liveTournaments"
          :key="contest.id"
          class="tournament-card live-card"
        >
          <div class="card-header-row">
            <div class="live-badge">
              <span class="pulse-dot"></span>
              <span>{{ t('freePractice.live') }}</span>
            </div>
            <span class="time-remaining">
              {{ formatTimeRemaining(liveCountdowns[contest.id]) }} {{ t('freePractice.remaining') }}
            </span>
          </div>

          <h3 class="tournament-name">{{ contest.name }}</h3>

          <div class="tournament-stats">
            <span class="stat">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
              </svg>
              {{ getParticipantCount(contest) }} {{ t('freePractice.traders') }}
            </span>
          </div>

          <!-- User Rank (if joined) -->
          <div v-if="isJoined(contest.id) && userRanks[contest.id]" class="user-rank">
            <span class="rank-label">{{ t('freePractice.yourRank') }}</span>
            <span class="rank-value">
              #{{ userRanks[contest.id].rank }} / {{ userRanks[contest.id].totalParticipants }}
            </span>
          </div>

          <div class="card-actions">
            <!-- Already Joined - Show Continue Trading -->
            <template v-if="isJoined(contest.id)">
              <button
                class="btn btn-success btn-block"
                @click="handleContinueTrading(contest)"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <polygon points="5 3 19 12 5 21 5 3" />
                </svg>
                {{ t('freePractice.continueTrading') }}
              </button>
            </template>
            <!-- Not Joined - Show Join Now -->
            <template v-else>
              <button
                class="btn btn-primary btn-block"
                :disabled="isJoining(contest.id)"
                @click="handleJoin(contest)"
              >
                <span v-if="isJoining(contest.id)" class="btn-loading">
                  <span class="spinner-small"></span>
                </span>
                <span v-else-if="!isAuthenticated">{{ t('freeTournaments.loginToJoin') }}</span>
                <span v-else>{{ t('freePractice.joinNow') }}</span>
              </button>
            </template>
          </div>
        </div>

        <!-- Upcoming Tournaments -->
        <div
          v-for="contest in upcomingTournaments"
          :key="contest.id"
          class="tournament-card upcoming-card"
        >
          <div class="card-header-row">
            <div class="upcoming-badge">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <path d="M12 6v6l4 2" />
              </svg>
              <span>{{ t('freePractice.startingSoon') }}</span>
            </div>
            <span class="starts-in">
              {{ t('freePractice.startsIn') }} {{ formatStartsIn(upcomingCountdowns[contest.id]) }}
            </span>
          </div>

          <h3 class="tournament-name">{{ contest.name }}</h3>

          <div class="tournament-stats">
            <span class="stat">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
              </svg>
              {{ getParticipantCount(contest) }} {{ t('freePractice.registered') }}
            </span>
          </div>

          <div class="card-actions">
            <!-- Already Registered -->
            <template v-if="isJoined(contest.id)">
              <span class="registered-badge">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
                {{ t('freePractice.registered') }}
              </span>
            </template>
            <!-- Not Registered - Show Register -->
            <template v-else>
              <button
                class="btn btn-secondary btn-block"
                :disabled="isJoining(contest.id)"
                @click="handleJoin(contest)"
              >
                <span v-if="isJoining(contest.id)" class="btn-loading">
                  <span class="spinner-small"></span>
                </span>
                <span v-else-if="!isAuthenticated">{{ t('freeTournaments.loginToRegister') }}</span>
                <span v-else>{{ t('freePractice.register') }}</span>
              </button>
            </template>
          </div>
        </div>
      </div>
    </div>

    <!-- Tip Section -->
    <div class="tip-section">
      <div class="tip-icon">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" />
          <line x1="12" y1="17" x2="12.01" y2="17" />
        </svg>
      </div>
      <p class="tip-text">
        <strong>{{ t('freePractice.tipLabel') }}</strong>
        {{ t('freePractice.tipText') }}
      </p>
    </div>

    <!-- Footer Link -->
    <div class="section-footer">
      <RouterLink to="/user/contests?filter=free" class="view-all-link">
        {{ t('freeTournaments.viewAll') }}
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="9 18 15 12 9 6" />
        </svg>
      </RouterLink>
    </div>
  </section>
</template>

<style scoped>
.free-practice-section {
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  background: linear-gradient(135deg, var(--color-bg-primary) 0%, var(--color-bg-secondary) 100%);
}

.section-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: var(--spacing-lg);
  background: linear-gradient(135deg, #059669 0%, #10b981 100%);
  color: white;
}

.header-left {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-md);
}

.header-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  background: rgba(255, 255, 255, 0.2);
  border-radius: var(--radius-md);
  flex-shrink: 0;
}

.header-text {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.section-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  margin: 0;
  color: white;
}

.section-subtitle {
  font-size: var(--font-size-sm);
  margin: 0;
  opacity: 0.9;
}

.free-badge {
  background: rgba(255, 255, 255, 0.2);
  color: white;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.section-content {
  padding: var(--spacing-lg);
  flex: 1;
}

.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-xl);
  text-align: center;
}

.loading-state {
  color: var(--color-text-secondary);
}

.error-state {
  color: var(--color-danger);
}

.empty-state {
  color: var(--color-text-secondary);
}

.empty-icon {
  color: var(--color-text-muted);
  margin-bottom: var(--spacing-sm);
}

.empty-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.empty-description {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
  max-width: 300px;
}

.schedule-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  color: var(--color-primary);
  margin: var(--spacing-sm) 0;
}

.tournaments-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-md);
}

.tournament-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  background: var(--color-bg-primary);
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border);
  transition: box-shadow var(--transition-fast);
}

.tournament-card:hover {
  box-shadow: var(--shadow-md);
}

.live-card {
  border-color: #EF4444;
  background: linear-gradient(135deg, rgba(239, 68, 68, 0.05) 0%, var(--color-bg-primary) 100%);
}

.upcoming-card {
  border-color: #3B82F6;
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.05) 0%, var(--color-bg-primary) 100%);
}

.card-header-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}

.live-badge {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-xs);
  font-weight: 700;
  text-transform: uppercase;
  color: #EF4444;
  letter-spacing: 0.05em;
}

.pulse-dot {
  width: 8px;
  height: 8px;
  background: #EF4444;
  border-radius: 50%;
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.5;
    transform: scale(1.3);
  }
}

.upcoming-badge {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: #3B82F6;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.time-remaining {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

.starts-in {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

.tournament-name {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.tournament-stats {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.stat {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.stat svg {
  flex-shrink: 0;
  color: var(--color-text-muted);
}

.user-rank {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm);
  background: linear-gradient(135deg, #FEF3C7 0%, #FDE68A 100%);
  border-radius: var(--radius-sm);
  margin-top: var(--spacing-xs);
}

.rank-label {
  font-size: var(--font-size-xs);
  color: #92400E;
  font-weight: 500;
}

.rank-value {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: #92400E;
}

.card-actions {
  margin-top: auto;
  padding-top: var(--spacing-sm);
}

.btn-block {
  width: 100%;
}

.btn-success {
  background: linear-gradient(135deg, #10b981, #059669);
  color: white;
  border: none;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
}

.btn-success:hover {
  background: linear-gradient(135deg, #059669, #047857);
}

.registered-badge {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  background: #ECFDF5;
  color: #059669;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.tip-section {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-md);
  padding: var(--spacing-md) var(--spacing-lg);
  background: linear-gradient(135deg, #FEF3C7 0%, #FDE68A50 100%);
  border-top: 1px solid var(--color-border);
}

.tip-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: #F59E0B;
  color: white;
  border-radius: 50%;
  flex-shrink: 0;
}

.tip-text {
  font-size: var(--font-size-sm);
  color: #92400E;
  margin: 0;
  line-height: 1.5;
}

.tip-text strong {
  font-weight: 600;
}

.section-footer {
  padding: var(--spacing-md) var(--spacing-lg);
  border-top: 1px solid var(--color-border);
  background: var(--color-bg-secondary);
}

.view-all-link {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-primary);
  text-decoration: none;
}

.view-all-link:hover {
  text-decoration: underline;
}

[dir="rtl"] .view-all-link svg {
  transform: scaleX(-1);
}

.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.spinner-small {
  width: 14px;
  height: 14px;
  border: 2px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.btn-loading {
  display: flex;
  align-items: center;
  justify-content: center;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-md);
  font-size: var(--font-size-sm);
}

/* Mobile responsive */
@media (max-width: 767px) {
  .section-header {
    padding: var(--spacing-md);
    flex-wrap: wrap;
    gap: var(--spacing-sm);
  }

  .header-left {
    flex: 1;
    min-width: 0;
  }

  .header-icon {
    width: 32px;
    height: 32px;
  }

  .header-icon svg {
    width: 20px;
    height: 20px;
  }

  .section-title {
    font-size: var(--font-size-md);
  }

  .section-content {
    padding: var(--spacing-md);
  }

  .tournaments-grid {
    grid-template-columns: 1fr;
  }

  .tip-section {
    padding: var(--spacing-sm) var(--spacing-md);
  }

  .tip-icon {
    width: 28px;
    height: 28px;
  }

  .tip-text {
    font-size: var(--font-size-xs);
  }
}

/* Tablet */
@media (min-width: 768px) and (max-width: 1023px) {
  .tournaments-grid {
    grid-template-columns: 1fr;
  }
}
</style>
