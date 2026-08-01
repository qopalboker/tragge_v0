<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { useContestsStore, type Contest } from '@/stores/contests';
import { freeTournamentsApi } from '@/api';
import { useToast } from '@/composables/useToast';

const router = useRouter();
const authStore = useAuthStore();
const contestsStore = useContestsStore();
const toast = useToast();

const liveTournament = ref<Contest | null>(null);
const upcomingTournament = ref<Contest | null>(null);
const loading = ref(true);
const error = ref<string | null>(null);
const joiningId = ref<string | null>(null);

// Countdown state
const countdown = ref({ days: 0, hours: 0, minutes: 0, seconds: 0 });
const refreshPending = ref(false);
let countdownInterval: ReturnType<typeof setInterval> | null = null;

const isAuthenticated = computed(() => authStore.isAuthenticated);

const liveTournamentParticipants = computed(() => {
  return liveTournament.value?.participant_count ?? 0;
});

const upcomingTournamentParticipants = computed(() => {
  return upcomingTournament.value?.participant_count ?? 0;
});

function updateCountdown(): void {
  if (!upcomingTournament.value) {
    countdown.value = { days: 0, hours: 0, minutes: 0, seconds: 0 };
    return;
  }

  const now = new Date().getTime();
  const startTime = new Date(upcomingTournament.value.starts_at).getTime();
  const diff = startTime - now;

  if (diff <= 0) {
    countdown.value = { days: 0, hours: 0, minutes: 0, seconds: 0 };
    // Refresh once to get updated tournament status (not every second)
    if (!refreshPending.value && !loading.value) {
      refreshPending.value = true;
      fetchFreeTournaments().finally(() => {
        // Allow next refresh after 30 seconds
        setTimeout(() => { refreshPending.value = false; }, 30000);
      });
    }
    return;
  }

  countdown.value = {
    days: Math.floor(diff / (1000 * 60 * 60 * 24)),
    hours: Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60)),
    minutes: Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60)),
    seconds: Math.floor((diff % (1000 * 60)) / 1000),
  };
}

async function fetchFreeTournaments(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    const data = await freeTournamentsApi.getFreeTournaments();

    // Find live tournament (running)
    liveTournament.value = data.find(c => c.status === 'running') || null;

    // Find next upcoming tournament (registration_open or scheduled)
    const upcoming = data
      .filter(c => c.status === 'registration_open' || c.status === 'scheduled')
      .sort((a, b) => new Date(a.starts_at).getTime() - new Date(b.starts_at).getTime());
    upcomingTournament.value = upcoming[0] || null;
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
    toast.success(t('contests.joinSuccess'));
  } catch (err) {
    const message = err instanceof Error ? err.message : t('common.error');
    toast.error(message);
  } finally {
    joiningId.value = null;
  }
}

function formatCountdownUnit(value: number): string {
  return value.toString().padStart(2, '0');
}

function isJoined(contestId: string): boolean {
  return contestsStore.isJoined(contestId);
}

function isJoining(contestId: string): boolean {
  return joiningId.value === contestId;
}

onMounted(() => {
  fetchFreeTournaments();
  countdownInterval = setInterval(updateCountdown, 1000);
});

onUnmounted(() => {
  if (countdownInterval) {
    clearInterval(countdownInterval);
  }
});
</script>

<template>
  <section class="free-tournaments card">
    <!-- Gradient Header -->
    <div class="card-header-gradient">
      <div class="header-content">
        <h2 class="section-title">{{ t('freeTournaments.title') }}</h2>
        <span class="free-badge">{{ t('freeTournaments.free') }}</span>
      </div>
      <p class="section-subtitle">{{ t('freeTournaments.subtitle') }}</p>
    </div>

    <!-- Content -->
    <div class="card-content">
      <!-- Loading State -->
      <div v-if="loading" class="loading-state">
        <span class="spinner"></span>
        <span>{{ t('common.loading') }}</span>
      </div>

      <!-- Error State -->
      <div v-else-if="error" class="error-state">
        <span>{{ error }}</span>
        <button class="btn btn-secondary btn-sm" @click="fetchFreeTournaments">
          {{ t('common.retry') }}
        </button>
      </div>

      <!-- Empty State -->
      <div v-else-if="!liveTournament && !upcomingTournament" class="empty-state">
        <p>{{ t('freeTournaments.noTournaments') }}</p>
        <RouterLink to="/user/contests" class="btn btn-secondary btn-sm">
          {{ t('freeTournaments.browseAll') }}
        </RouterLink>
      </div>

      <!-- Tournaments -->
      <div v-else class="tournaments-container">
        <!-- Live Tournament -->
        <div v-if="liveTournament" class="tournament-item live-tournament">
          <div class="tournament-header">
            <div class="live-indicator">
              <span class="pulse-dot"></span>
              <span class="live-text">{{ t('freeTournaments.liveNow') }}</span>
            </div>
            <span class="tournament-name">{{ liveTournament.name }}</span>
          </div>

          <div class="tournament-info">
            <span class="participants">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
              </svg>
              {{ liveTournamentParticipants }} {{ t('freeTournaments.tradersCompeting') }}
            </span>
          </div>

          <div class="tournament-actions">
            <span v-if="isJoined(liveTournament.id)" class="joined-badge">
              {{ t('contests.joined') }}
            </span>
            <button
              v-else
              class="btn btn-primary"
              :disabled="isJoining(liveTournament.id)"
              @click="handleJoin(liveTournament)"
            >
              <span v-if="isJoining(liveTournament.id)" class="btn-loading">
                <span class="spinner-small"></span>
              </span>
              <span v-else-if="!isAuthenticated">{{ t('freeTournaments.loginToJoin') }}</span>
              <span v-else>{{ t('freeTournaments.joinNow') }}</span>
            </button>
          </div>
        </div>

        <!-- Upcoming Tournament -->
        <div v-if="upcomingTournament" class="tournament-item upcoming-tournament">
          <div class="tournament-header">
            <span class="upcoming-label">{{ t('freeTournaments.upNext') }}</span>
            <span class="tournament-name">{{ upcomingTournament.name }}</span>
          </div>

          <!-- Countdown -->
          <div class="countdown-container">
            <span class="countdown-label">{{ t('freeTournaments.startsIn') }}</span>
            <div class="countdown">
              <div v-if="countdown.days > 0" class="countdown-unit">
                <span class="countdown-value">{{ formatCountdownUnit(countdown.days) }}</span>
                <span class="countdown-label-unit">{{ t('freeTournaments.days') }}</span>
              </div>
              <div class="countdown-unit">
                <span class="countdown-value">{{ formatCountdownUnit(countdown.hours) }}</span>
                <span class="countdown-label-unit">{{ t('freeTournaments.hours') }}</span>
              </div>
              <span class="countdown-separator">:</span>
              <div class="countdown-unit">
                <span class="countdown-value">{{ formatCountdownUnit(countdown.minutes) }}</span>
                <span class="countdown-label-unit">{{ t('freeTournaments.minutes') }}</span>
              </div>
              <span class="countdown-separator">:</span>
              <div class="countdown-unit">
                <span class="countdown-value">{{ formatCountdownUnit(countdown.seconds) }}</span>
                <span class="countdown-label-unit">{{ t('freeTournaments.seconds') }}</span>
              </div>
            </div>
          </div>

          <div class="tournament-info">
            <span class="participants">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
              </svg>
              {{ upcomingTournamentParticipants }} {{ t('freeTournaments.registered') }}
            </span>
          </div>

          <div class="tournament-actions">
            <span v-if="isJoined(upcomingTournament.id)" class="joined-badge">
              {{ t('contests.joined') }}
            </span>
            <button
              v-else
              class="btn btn-secondary"
              :disabled="isJoining(upcomingTournament.id)"
              @click="handleJoin(upcomingTournament)"
            >
              <span v-if="isJoining(upcomingTournament.id)" class="btn-loading">
                <span class="spinner-small"></span>
              </span>
              <span v-else-if="!isAuthenticated">{{ t('freeTournaments.loginToRegister') }}</span>
              <span v-else>{{ t('freeTournaments.register') }}</span>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Footer Link -->
    <div class="card-footer">
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
.free-tournaments {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 0;
}

.card-header-gradient {
  background: linear-gradient(135deg, var(--color-primary) 0%, #7C3AED 100%);
  padding: var(--spacing-lg);
  color: white;
}

.header-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--spacing-xs);
}

.section-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  margin: 0;
  color: white;
}

.free-badge {
  background-color: rgba(255, 255, 255, 0.2);
  color: white;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.section-subtitle {
  font-size: var(--font-size-sm);
  opacity: 0.9;
  margin: 0;
}

.card-content {
  padding: var(--spacing-lg);
  flex: 1;
}

.loading-state,
.error-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-xl);
  text-align: center;
  color: var(--color-text-secondary);
}

.tournaments-container {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

.tournament-item {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.live-tournament {
  border: 2px solid #EF4444;
  background-color: rgba(239, 68, 68, 0.05);
}

.tournament-header {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.live-indicator {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.pulse-dot {
  width: 8px;
  height: 8px;
  background-color: #EF4444;
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
    transform: scale(1.2);
  }
}

.live-text {
  font-size: var(--font-size-xs);
  font-weight: 700;
  text-transform: uppercase;
  color: #EF4444;
  letter-spacing: 0.05em;
}

.upcoming-label {
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
  color: var(--color-primary);
  letter-spacing: 0.05em;
}

.tournament-name {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
}

.countdown-container {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.countdown-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.countdown {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.countdown-unit {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 40px;
}

.countdown-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
  line-height: 1;
}

.countdown-label-unit {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-transform: uppercase;
}

.countdown-separator {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-muted);
  margin-bottom: var(--spacing-md);
}

.tournament-info {
  display: flex;
  align-items: center;
}

.participants {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.participants svg {
  flex-shrink: 0;
}

.tournament-actions {
  display: flex;
  align-items: center;
}

.tournament-actions .btn {
  width: 100%;
}

.joined-badge {
  width: 100%;
  text-align: center;
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: #ECFDF5;
  color: #059669;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.card-footer {
  padding: var(--spacing-md) var(--spacing-lg);
  border-top: 1px solid var(--color-border);
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
  .card-header-gradient {
    padding: var(--spacing-md);
  }

  .card-content {
    padding: var(--spacing-md);
  }

  .countdown-value {
    font-size: var(--font-size-lg);
  }

  .countdown-unit {
    min-width: 32px;
  }

  .countdown-separator {
    font-size: var(--font-size-lg);
  }

  .header-content {
    flex-wrap: wrap;
    gap: var(--spacing-sm);
  }
}
</style>
