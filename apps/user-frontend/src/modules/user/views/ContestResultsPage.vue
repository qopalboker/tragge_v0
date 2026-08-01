<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import { type Contest, type DurationType, type MarketType } from '@/stores/contests';
import { useAuthStore } from '@/stores/auth';
import { useToast } from '@/composables/useToast';
import { api } from '@/api';
import ContestHeader from '@/components/contests/ContestHeader.vue';
import UserResultCard from '@/components/contests/UserResultCard.vue';
import RankingTable from '@/components/contests/RankingTable.vue';
import PrizeDistributionCard from '@/components/contests/PrizeDistributionCard.vue';
import PodiumDisplay from '@/components/contests/PodiumDisplay.vue';
import ShareResultsModal from '@/components/contests/ShareResultsModal.vue';
import ContestTradeHistory from '@/components/contests/ContestTradeHistory.vue';

// Types
interface LeaderboardEntry {
  rank: number;
  user_id: string;
  username?: string;
  total_score: number;
  pnl_percent?: number;
  reward_cents?: number;
  trade_count?: number;
}

interface UserResult {
  rank: number;
  total_score: number;
  pnl_percent: number;
  reward_cents: number;
  total_participants: number;
  trade_count?: number;
}

// Backend response types (match actual API responses)
interface ContestDetailsAPIResponse {
  id: string;
  name: string;
  description?: string;
  status: string;
  market_type: string;
  duration_type: string;
  start_time: string;
  end_time: string;
  entry_fee_cents: number;
  is_free: boolean;
  prize_pool_cents: number;
  available_qty: number;
  max_participants?: number;
  current_participants: number;
  user_joined: boolean;
  symbols: string[];
}

interface ContestLeaderboardAPIEntry {
  position: number;
  user_id: string;
  username: string;
  pnl_percent: number;
  reward_cents: number;
  trade_count: number;
}

interface ContestLeaderboardAPIResponse {
  leaderboard: ContestLeaderboardAPIEntry[];
  total_participants: number;
  prize_pool_cents: number;
}

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const toast = useToast();

// State
const contest = ref<Contest | null>(null);
const leaderboard = ref<LeaderboardEntry[]>([]);
const userResult = ref<UserResult | null>(null);
const loading = ref(true);
const loadingLeaderboard = ref(false);
const error = ref<string | null>(null);
const leaderboardPrizePoolCents = ref(0);

// Modal states
const showShareModal = ref(false);
const showTradeHistory = ref(false);
const selectedUserId = ref<string | undefined>(undefined);

// Contest ID from route
const contestId = computed(() => route.params.contestId as string);

// Computed
const isFinished = computed(() => {
  return contest.value?.status === 'completed';
});

const estimatedPrizePool = computed(() => {
  if (!contest.value) return 0;
  // Use prize pool from leaderboard response (calculated by backend with platform fee)
  if (leaderboardPrizePoolCents.value > 0) {
    return leaderboardPrizePoolCents.value;
  }
  if (contest.value.estimated_prize_pool_cents) {
    return contest.value.estimated_prize_pool_cents;
  }
  // Fallback: estimate based on current participants and entry fee
  const participantCount = contest.value.participant_count ?? 0;
  return Math.round(participantCount * contest.value.entry_fee_cents * 0.83);
});

const formattedPrizePool = computed(() => {
  if (estimatedPrizePool.value === 0) {
    return t('contestDetails.noPrize');
  }
  const amount = estimatedPrizePool.value / 100;
  if (amount >= 1000) {
    return `$${(amount / 1000).toFixed(1)}K`;
  }
  return `$${amount.toFixed(2)}`;
});

const totalParticipants = computed(() => {
  return contest.value?.participant_count ?? leaderboard.value.length;
});

const prizeWinnersPercentage = computed(() => {
  return contest.value?.prize_winners_percentage ?? 30;
});

const formattedEndDate = computed(() => {
  if (!contest.value?.ends_at) return '';
  const date = new Date(contest.value.ends_at);
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
});

// Share data
const shareData = computed(() => ({
  contestId: contestId.value,
  contestName: contest.value?.name ?? '',
  rank: userResult.value?.rank ?? 0,
  totalParticipants: userResult.value?.total_participants ?? totalParticipants.value,
  pnlPercent: userResult.value?.pnl_percent ?? 0,
  prizeCents: userResult.value?.reward_cents,
}));

/**
 * Map backend ContestDetailsAPIResponse to frontend Contest type.
 * The backend uses different field names (e.g., start_time vs starts_at,
 * current_participants vs participant_count, prize_pool_cents vs estimated_prize_pool_cents).
 */
function mapContestResponse(data: ContestDetailsAPIResponse): Contest {
  return {
    id: data.id,
    name: data.name,
    description: data.description,
    starts_at: data.start_time,
    ends_at: data.end_time,
    status: data.status as Contest['status'],
    entry_fee_cents: data.entry_fee_cents,
    qty_total: data.available_qty,
    duration_type: data.duration_type as DurationType | undefined,
    market_type: data.market_type as MarketType | undefined,
    symbols: data.symbols.map(s => ({ symbol: s, enabled: true })),
    participant_count: data.current_participants,
    max_participants: data.max_participants,
    estimated_prize_pool_cents: data.prize_pool_cents,
    // TODO: Backend does not return prize_winners_percentage. Add this field to
    // ContestDetailsResponse in user-bff (from contests table) so the frontend
    // can display the correct prize cutoff instead of defaulting to 30%.
  };
}

/**
 * Map backend ContestLeaderboardAPIEntry to frontend LeaderboardEntry.
 * The backend uses 'position' instead of 'rank'.
 */
function mapLeaderboardEntries(
  entries: ContestLeaderboardAPIEntry[],
): LeaderboardEntry[] {
  return entries.map((entry) => ({
    rank: entry.position,
    user_id: entry.user_id,
    username: entry.username,
    total_score: 0, // Backend doesn't expose raw total_score; pnl_percent is used for display
    pnl_percent: entry.pnl_percent,
    reward_cents: entry.reward_cents,
    trade_count: entry.trade_count,
  }));
}

// Fetch contest details
async function fetchContestDetails(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    const response = await api.get<ContestDetailsAPIResponse>(`/api/user/contests/${contestId.value}`);
    contest.value = mapContestResponse(response.data);
  } catch (err) {
    console.error('Failed to fetch contest details:', err);
    error.value = t('contestResults.loadError');
  } finally {
    loading.value = false;
  }
}

// Fetch leaderboard
async function fetchLeaderboard(): Promise<void> {
  loadingLeaderboard.value = true;

  try {
    const response = await api.get<ContestLeaderboardAPIResponse>(
      `/api/user/contests/${contestId.value}/leaderboard`
    );
    leaderboard.value = mapLeaderboardEntries(response.data.leaderboard || []);
    leaderboardPrizePoolCents.value = response.data.prize_pool_cents ?? 0;
  } catch (err) {
    console.error('Failed to fetch leaderboard:', err);
    leaderboard.value = [];
  } finally {
    loadingLeaderboard.value = false;
  }
}

// Fetch user's result
// TODO: Backend endpoint GET /api/user/contests/{id}/my-result is not implemented.
// Add a handler in user-bff that queries contest_participants for the authenticated user
// and returns: { rank, total_score, pnl_percent, reward_cents, total_participants, trade_count }.
// Until this endpoint exists, we derive the user's result from the leaderboard data.
async function fetchUserResult(): Promise<void> {
  try {
    const response = await api.get<UserResult>(
      `/api/user/contests/${contestId.value}/my-result`
    );
    userResult.value = response.data;
  } catch (err) {
    // Endpoint not yet implemented or user didn't participate — derive from leaderboard
    deriveUserResultFromLeaderboard();
  }
}

/**
 * Fallback: derive the current user's result from leaderboard data.
 * This is used when /my-result endpoint is not available.
 * Note: limited because the leaderboard currently doesn't return user_id,
 * so we match by username as a best-effort approach.
 */
function deriveUserResultFromLeaderboard(): void {
  if (!authStore.user || leaderboard.value.length === 0) {
    userResult.value = null;
    return;
  }

  // Try to find user in leaderboard by user_id first, then by username
  const userId = authStore.user.id;
  const username = authStore.user.username;
  const entry = leaderboard.value.find(
    (e) => (e.user_id && e.user_id === userId) || (e.username && e.username === username)
  );

  if (!entry) {
    userResult.value = null;
    return;
  }

  userResult.value = {
    rank: entry.rank,
    total_score: entry.total_score,
    pnl_percent: entry.pnl_percent ?? 0,
    reward_cents: entry.reward_cents ?? 0,
    total_participants: totalParticipants.value,
    trade_count: entry.trade_count,
  };
}

// Go back
function goBack(): void {
  router.back();
}

// Navigate to more info
function handleMoreInfo(): void {
  router.push(`/user/contests/${contestId.value}`);
}

// Navigate to find similar contests
function findSimilarContests(): void {
  const params = new URLSearchParams();
  if (contest.value?.market_type) {
    params.set('market_type', contest.value.market_type);
  }
  if (contest.value?.duration_type) {
    params.set('duration_type', contest.value.duration_type);
  }
  router.push(`/user/contests?${params.toString()}`);
}

// Share results
function openShareModal(): void {
  if (!userResult.value) {
    toast.warning(t('contestResults.noResultToShare'));
    return;
  }
  showShareModal.value = true;
}

function closeShareModal(): void {
  showShareModal.value = false;
}

// View trades
function handleViewTrades(userId?: string): void {
  selectedUserId.value = userId;
  showTradeHistory.value = true;
}

function closeTradeHistory(): void {
  showTradeHistory.value = false;
  selectedUserId.value = undefined;
}

// Watch for route changes
watch(contestId, async () => {
  if (contestId.value) {
    // Fetch contest details and leaderboard in parallel, then derive user result
    await Promise.all([fetchContestDetails(), fetchLeaderboard()]);
    await fetchUserResult();
  }
});

// Lifecycle
onMounted(async () => {
  // Fetch contest details and leaderboard in parallel first
  await Promise.all([fetchContestDetails(), fetchLeaderboard()]);
  // Then fetch user result (needs leaderboard data for fallback derivation)
  await fetchUserResult();
});
</script>

<template>
  <div class="contest-results-page">
    <!-- Loading State -->
    <div v-if="loading" class="loading-container">
      <div class="loading-spinner"></div>
      <span>{{ t('common.loading') }}</span>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="error-container">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="12" cy="12" r="10" />
        <line x1="15" y1="9" x2="9" y2="15" />
        <line x1="9" y1="9" x2="15" y2="15" />
      </svg>
      <h2>{{ t('contestResults.errorTitle') }}</h2>
      <p>{{ error }}</p>
      <button class="btn btn-primary" @click="fetchContestDetails">
        {{ t('common.retry') }}
      </button>
    </div>

    <!-- Content -->
    <template v-else-if="contest">
      <!-- Back Button -->
      <button class="back-button" @click="goBack">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M19 12H5M12 19l-7-7 7-7" />
        </svg>
        <span>{{ t('common.back') }}</span>
      </button>

      <!-- Header -->
      <ContestHeader
        :contest-id="contest.id"
        :duration-type="contest.duration_type"
        :market-type="contest.market_type"
        :name="contest.name"
      />

      <!-- Results Banner -->
      <div class="results-banner">
        <div class="banner-decoration">
          <div class="decoration-shape decoration-1"></div>
          <div class="decoration-shape decoration-2"></div>
          <div class="decoration-shape decoration-3"></div>
        </div>

        <div class="banner-content">
          <!-- Prize Pool Section -->
          <div class="prize-section">
            <span class="prize-label">{{ t('contestResults.totalPrizePool') }}</span>
            <span class="prize-value">{{ formattedPrizePool }}</span>
          </div>

          <!-- Divider -->
          <div class="banner-divider"></div>

          <!-- Tournament Status -->
          <div class="status-section">
            <span class="status-label">{{ contest.name }}</span>
            <div class="status-badge finished">
              {{ t('contestResults.finished') }}
            </div>
            <span class="end-date">{{ t('contestResults.endedAt') }}: {{ formattedEndDate }}</span>
          </div>
        </div>
      </div>

      <!-- Top 3 Podium (only show for completed contests with leaderboard) -->
      <PodiumDisplay
        v-if="isFinished && leaderboard.length >= 3"
        :entries="leaderboard"
        :current-user-id="authStore.user?.id"
        class="podium-section"
      />

      <!-- User Result Summary (if participated) -->
      <div v-if="userResult" class="user-summary-banner">
        <div class="user-summary-content">
          <div class="summary-item">
            <span class="summary-label">{{ t('contestResults.yourRank') }}</span>
            <span class="summary-value rank">
              #{{ userResult.rank }} <span class="of">{{ t('contestResults.of') }} {{ userResult.total_participants }}</span>
            </span>
          </div>
          <div class="summary-divider"></div>
          <div class="summary-item">
            <span class="summary-label">{{ t('contestResults.yourScore') }}</span>
            <span class="summary-value" :class="{ positive: userResult.pnl_percent >= 0, negative: userResult.pnl_percent < 0 }">
              {{ userResult.pnl_percent >= 0 ? '+' : '' }}{{ userResult.pnl_percent.toFixed(2) }}%
            </span>
          </div>
          <div class="summary-divider"></div>
          <div class="summary-item">
            <span class="summary-label">{{ t('contestResults.yourPrize') }}</span>
            <span class="summary-value prize">
              ${{ (userResult.reward_cents / 100).toFixed(2) }}
            </span>
          </div>
        </div>
      </div>

      <!-- Main Content Grid -->
      <div class="content-grid">
        <!-- Left Column - Ranking Table -->
        <div class="left-column">
          <!-- Ranking Section -->
          <section class="ranking-section">
            <div class="section-header">
              <div class="header-left">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M12 20v-6M6 20V10M18 20v-4" />
                </svg>
                <h2 class="section-title">{{ t('contestResults.fullLeaderboard') }}</h2>
              </div>
              <div class="help-icon" :title="t('contestResults.rankingHelp')">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="12" cy="12" r="10" />
                  <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3M12 17h.01" />
                </svg>
              </div>
            </div>
            <RankingTable
              :entries="leaderboard"
              :loading="loadingLeaderboard"
              :current-user-id="authStore.user?.id"
              :show-rewards="estimatedPrizePool > 0"
              :show-trade-count="true"
              :prize-winners-percentage="prizeWinnersPercentage"
              :total-participants="totalParticipants"
              @view-trades="handleViewTrades"
            />
          </section>

          <!-- Prize Distribution (only show if there's a prize pool) -->
          <section v-if="estimatedPrizePool > 0" class="prize-distribution-section">
            <PrizeDistributionCard
              :prize-pool-cents="estimatedPrizePool"
              :participant-count="totalParticipants"
              :prize-winners-percentage="prizeWinnersPercentage"
            />
          </section>
        </div>

        <!-- Right Column - User Result Card & Actions -->
        <div class="right-column">
          <UserResultCard
            v-if="userResult"
            :rank="userResult.rank"
            :total-participants="userResult.total_participants || totalParticipants"
            :pnl-percent="userResult.pnl_percent"
            :reward-cents="userResult.reward_cents"
            @more-info="handleMoreInfo"
          />
          <div v-else class="no-participation-card">
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <circle cx="12" cy="12" r="10" />
              <path d="M12 8v4M12 16h.01" />
            </svg>
            <h3>{{ t('contestResults.notParticipated') }}</h3>
            <p>{{ t('contestResults.notParticipatedDesc') }}</p>
          </div>

          <!-- Action Buttons -->
          <div class="action-buttons">
            <button v-if="userResult" class="action-btn share" @click="openShareModal">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="18" cy="5" r="3" />
                <circle cx="6" cy="12" r="3" />
                <circle cx="18" cy="19" r="3" />
                <line x1="8.59" y1="13.51" x2="15.42" y2="17.49" />
                <line x1="15.41" y1="6.51" x2="8.59" y2="10.49" />
              </svg>
              {{ t('contestResults.shareResults') }}
            </button>
            <button v-if="userResult" class="action-btn trades" @click="handleViewTrades()">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="22 7 13.5 15.5 8.5 10.5 2 17" />
                <polyline points="16 7 22 7 22 13" />
              </svg>
              {{ t('contestResults.viewYourTrades') }}
            </button>
            <button class="action-btn similar" @click="findSimilarContests">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="11" cy="11" r="8" />
                <line x1="21" y1="21" x2="16.65" y2="16.65" />
              </svg>
              {{ t('contestResults.findSimilar') }}
            </button>
          </div>
        </div>
      </div>
    </template>

    <!-- Share Modal -->
    <ShareResultsModal
      :show="showShareModal"
      :data="shareData"
      @close="closeShareModal"
    />

    <!-- Trade History Modal -->
    <ContestTradeHistory
      :show="showTradeHistory"
      :contest-id="contestId"
      :user-id="selectedUserId"
      @close="closeTradeHistory"
    />
  </div>
</template>

<style scoped>
.contest-results-page {
  padding: var(--spacing-lg);
  max-width: 1200px;
  margin: 0 auto;
}

.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.loading-spinner {
  width: 32px;
  height: 32px;
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

.error-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  text-align: center;
}

.error-container svg {
  color: var(--color-danger);
}

.error-container h2 {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.error-container p {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
}

.btn {
  padding: var(--spacing-sm) var(--spacing-lg);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 600;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.btn-primary {
  background: var(--color-primary);
  border: none;
  color: white;
}

.btn-primary:hover {
  background: var(--color-secondary);
}

.back-button {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  margin-bottom: var(--spacing-md);
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.back-button:hover {
  background: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

/* Results Banner */
.results-banner {
  position: relative;
  background: linear-gradient(135deg, #0f172a 0%, #1e1b4b 40%, #312e81 70%, #06b6d4 100%);
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
  overflow: hidden;
  color: white;
  margin-top: var(--spacing-lg);
}

.banner-decoration {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  pointer-events: none;
  overflow: hidden;
}

.decoration-shape {
  position: absolute;
}

.decoration-1 {
  width: 300px;
  height: 300px;
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.3) 0%, rgba(99, 102, 241, 0.1) 100%);
  top: -100px;
  right: 20%;
  border-radius: 50%;
  filter: blur(40px);
}

.decoration-2 {
  width: 200px;
  height: 200px;
  background: linear-gradient(135deg, rgba(236, 72, 153, 0.3) 0%, rgba(168, 85, 247, 0.1) 100%);
  bottom: -50px;
  left: 10%;
  border-radius: 50%;
  filter: blur(30px);
}

.decoration-3 {
  width: 150px;
  height: 150px;
  background: linear-gradient(135deg, rgba(6, 182, 212, 0.4) 0%, rgba(59, 130, 246, 0.1) 100%);
  top: 20%;
  right: 5%;
  border-radius: 50%;
  filter: blur(25px);
}

.banner-content {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: var(--spacing-lg);
}

.prize-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.prize-label {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: rgba(255, 255, 255, 0.8);
  text-transform: uppercase;
  letter-spacing: 0.1em;
}

.prize-value {
  font-size: var(--font-size-3xl);
  font-weight: 700;
  color: white;
}

.banner-divider {
  width: 1px;
  height: 80px;
  background: rgba(255, 255, 255, 0.2);
}

.status-section {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--spacing-sm);
}

.status-label {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: white;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  padding: var(--spacing-xs) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.status-badge.finished {
  background: rgba(255, 255, 255, 0.2);
  color: white;
  border: 1px solid rgba(255, 255, 255, 0.3);
}

.end-date {
  font-size: var(--font-size-xs);
  color: rgba(255, 255, 255, 0.7);
}

/* Podium Section */
.podium-section {
  margin-top: var(--spacing-lg);
}

/* User Summary Banner */
.user-summary-banner {
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.1) 0%, rgba(168, 85, 247, 0.1) 100%);
  border: 1px solid rgba(99, 102, 241, 0.2);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  margin-top: var(--spacing-lg);
}

.user-summary-content {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xl);
  flex-wrap: wrap;
}

.summary-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
}

.summary-label {
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.summary-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.summary-value.rank .of {
  font-size: var(--font-size-sm);
  font-weight: 400;
  color: var(--color-text-secondary);
}

.summary-value.positive {
  color: #10B981;
}

.summary-value.negative {
  color: #EF4444;
}

.summary-value.prize {
  color: #FFD700;
}

.summary-divider {
  width: 1px;
  height: 40px;
  background: var(--color-border);
}

/* Content Grid */
.content-grid {
  display: grid;
  grid-template-columns: 1fr 380px;
  gap: var(--spacing-lg);
  margin-top: var(--spacing-lg);
}

.left-column {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

.right-column {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
  position: sticky;
  top: var(--spacing-lg);
  height: fit-content;
}

/* Ranking Section */
.ranking-section {
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg-secondary);
}

.header-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.header-left svg {
  color: var(--color-primary);
}

.section-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.help-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--color-bg-tertiary);
  cursor: help;
}

.help-icon svg {
  color: var(--color-text-muted);
}

/* Prize Distribution Section */
.prize-distribution-section {
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

/* No Participation Card */
.no-participation-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  text-align: center;
}

.no-participation-card svg {
  color: var(--color-text-muted);
}

.no-participation-card h3 {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.no-participation-card p {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
}

/* Action Buttons */
.action-buttons {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 600;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.action-btn.share {
  background: var(--color-primary);
  border: none;
  color: white;
}

.action-btn.share:hover {
  background: var(--color-secondary);
}

.action-btn.trades {
  background: transparent;
  border: 1px solid var(--color-primary);
  color: var(--color-primary);
}

.action-btn.trades:hover {
  background: rgba(99, 102, 241, 0.1);
}

.action-btn.similar {
  background: transparent;
  border: 1px solid var(--color-border);
  color: var(--color-text-secondary);
}

.action-btn.similar:hover {
  background: var(--color-bg-secondary);
  border-color: var(--color-text-secondary);
  color: var(--color-text-primary);
}

/* RTL Support */
[dir="rtl"] .back-button {
  flex-direction: row-reverse;
}

[dir="rtl"] .back-button svg {
  transform: rotate(180deg);
}

[dir="rtl"] .banner-content {
  flex-direction: row-reverse;
}

[dir="rtl"] .status-section {
  align-items: flex-start;
}

[dir="rtl"] .section-header {
  flex-direction: row-reverse;
}

[dir="rtl"] .header-left {
  flex-direction: row-reverse;
}

/* Mobile */
@media (max-width: 1023px) {
  .content-grid {
    grid-template-columns: 1fr;
  }

  .right-column {
    position: static;
    order: -1;
  }
}

@media (max-width: 767px) {
  .contest-results-page {
    padding: var(--spacing-md);
  }

  .results-banner {
    padding: var(--spacing-lg);
  }

  .banner-content {
    flex-direction: column;
    align-items: stretch;
    gap: var(--spacing-md);
  }

  .banner-divider {
    display: none;
  }

  .prize-section {
    text-align: center;
  }

  .prize-value {
    font-size: var(--font-size-2xl);
  }

  .status-section {
    align-items: center;
  }

  .user-summary-banner {
    padding: var(--spacing-md);
  }

  .user-summary-content {
    gap: var(--spacing-md);
  }

  .summary-value {
    font-size: var(--font-size-lg);
  }

  .summary-divider {
    display: none;
  }

  .content-grid {
    gap: var(--spacing-md);
    margin-top: var(--spacing-md);
  }

  .section-header {
    padding: var(--spacing-sm) var(--spacing-md);
  }
}
</style>
