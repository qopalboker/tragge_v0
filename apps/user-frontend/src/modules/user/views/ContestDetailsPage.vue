<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useContestsStore, type Contest } from '@/stores/contests';
import { useWalletStore } from '@/stores/wallet';
import { useToast } from '@/composables/useToast';
import { api } from '@/api';
import { redirectToTrade } from '@/utils/tradeRedirect';
import ContestHeader from '@/components/contests/ContestHeader.vue';
import ContestBanner from '@/components/contests/ContestBanner.vue';
import TournamentDetailsCard from '@/components/contests/TournamentDetailsCard.vue';
import ParticipantsList from '@/components/contests/ParticipantsList.vue';
import type { Participant } from '@/modules/user/components/contests/ParticipantsList.vue';
import JoinConfirmModal from '@/components/contests/JoinConfirmModal.vue';
import PrizePreviewTable from '@/components/contests/PrizePreviewTable.vue';
import ContestStatusBanner from '@/components/contests/ContestStatusBanner.vue';

const route = useRoute();
const router = useRouter();
const contestsStore = useContestsStore();
const walletStore = useWalletStore();
const toast = useToast();

// State
const contest = ref<Contest | null>(null);
const participants = ref<Participant[]>([]);
const loading = ref(true);
const loadingParticipants = ref(false);
const error = ref<string | null>(null);
const showJoinModal = ref(false);

// Contest ID from route
const contestId = computed(() => route.params.contestId as string);

// Computed
const isJoined = computed(() => contestsStore.isJoined(contestId.value));
const isJoining = computed(() => contestsStore.isJoining(contestId.value));

const canJoin = computed(() => {
  if (!contest.value) return false;
  return contest.value.status === 'registration_open' && !isJoined.value;
});

const showStatusBanner = computed(() => {
  if (!contest.value) return false;
  const isPreStart = contest.value.status === 'registration_open' || contest.value.status === 'scheduled';
  const isPaid = contest.value.entry_fee_cents > 0;
  return isPreStart && isPaid;
});

const estimatedPrizePool = computed(() => {
  if (!contest.value) return 0;
  if (contest.value.estimated_prize_pool_cents) {
    return contest.value.estimated_prize_pool_cents;
  }
  // Estimate based on current participants and entry fee
  // Assuming 83% of entry fees go to prize pool (17% platform fee)
  const participantCount = contest.value.participant_count ?? 0;
  return Math.round(participantCount * contest.value.entry_fee_cents * 0.83);
});

// Check balance for paid contests
const hasSufficientBalance = computed(() => {
  if (!contest.value || contest.value.entry_fee_cents === 0) return true;
  return walletStore.balanceCents >= contest.value.entry_fee_cents;
});

// Navigate to contest results page
function navigateToResults(): void {
  router.replace({
    name: 'contest-results',
    params: { contestId: contestId.value },
  });
}

// Fetch contest details
async function fetchContestDetails(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    const response = await api.get<Contest & { user_joined?: boolean }>(`/api/user/contests/${contestId.value}`);
    contest.value = response.data;

    // Sync join status from contest details API response
    if (response.data.user_joined) {
      contestsStore.joinedContestIds.add(contestId.value);
    }

    // Redirect to results page if contest is already completed
    if (contest.value?.status === 'completed' || contest.value?.status === 'settling') {
      navigateToResults();
      return;
    }

    // Fetch user's history to populate join status for other contests
    if (contest.value) {
      await contestsStore.fetchUserHistory();
    }
  } catch (err) {
    console.error('Failed to fetch contest details:', err);
    error.value = t('contestDetails.loadError');
  } finally {
    loading.value = false;
  }
}

// Fetch participants
async function fetchParticipants(): Promise<void> {
  loadingParticipants.value = true;

  try {
    const response = await api.get<{ participants: Participant[] }>(
      `/api/user/contests/${contestId.value}/participants`
    );
    participants.value = response.data.participants || [];
  } catch (err) {
    console.error('Failed to fetch participants:', err);
    // Don't show error for participants - it's not critical
    participants.value = [];
  } finally {
    loadingParticipants.value = false;
  }
}

// Handle join click
function handleJoinClick(): void {
  if (!contest.value) return;

  // Check balance for paid contests
  if (contest.value.entry_fee_cents > 0 && !hasSufficientBalance.value) {
    walletStore.openDepositModal();
    return;
  }

  // Show confirmation modal
  showJoinModal.value = true;
}

// Handle join confirm
async function handleJoinConfirm(): Promise<void> {
  if (!contest.value) return;

  try {
    await contestsStore.joinContest(contest.value.id);
    toast.success(t('contests.joinSuccess'));
    showJoinModal.value = false;

    // Refresh data
    await fetchContestDetails();
    await fetchParticipants();
  } catch (err) {
    const message = err instanceof Error ? err.message : t('common.error');
    toast.error(message);
  }
}

// Handle view results
function handleViewResults(): void {
  navigateToResults();
}

// Handle enter trading
function handleEnterTrading(): void {
  if (!contest.value) return;
  redirectToTrade(contest.value.id);
}

// Go back
function goBack(): void {
  router.back();
}

// Watch for route changes
watch(contestId, () => {
  if (contestId.value) {
    fetchContestDetails();
    fetchParticipants();
  }
});

// Watch for contest status changes to redirect when completed
watch(() => contest.value?.status, (newStatus) => {
  if (newStatus === 'completed' || newStatus === 'settling') {
    navigateToResults();
  }
});

// Lifecycle
onMounted(async () => {
  // Ensure wallet is loaded for balance checks
  if (!walletStore.wallet) {
    walletStore.fetchWallet();
  }

  await fetchContestDetails();
  await fetchParticipants();
});
</script>

<template>
  <div class="contest-details-page">
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
      <h2>{{ t('contestDetails.errorTitle') }}</h2>
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

      <!-- Main Content Grid -->
      <div class="content-grid">
        <!-- Left Column -->
        <div class="left-column">
          <!-- Banner -->
          <ContestBanner
            :prize-pool-cents="estimatedPrizePool"
            :starts-at="contest.starts_at"
            :ends-at="contest.ends_at"
            :status="contest.status"
            :entry-fee-cents="contest.entry_fee_cents"
          />

          <!-- Contest Status Banner (pre-start paid contests) -->
          <ContestStatusBanner
            v-if="showStatusBanner"
            :contest-id="contest.id"
            :status="contest.status"
            :starts-at="contest.starts_at"
          />

          <!-- Introduction Section -->
          <section class="introduction-section">
            <div class="section-header">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <path d="M12 16v-4M12 8h.01" />
              </svg>
              <h2 class="section-title">{{ t('contestDetails.introduction') }}</h2>
            </div>
            <div class="introduction-content">
              <p v-if="contest.description">{{ contest.description }}</p>
              <p v-else>
                {{ t('contestDetails.defaultDescription', {
                  duration: contest.duration_type ? t(`filters.duration.${contest.duration_type}`) : '',
                  contestId: contest.id.substring(0, 8).toUpperCase()
                }) }}
              </p>
            </div>
          </section>

          <!-- Prize Preview Section -->
          <section v-if="contest.entry_fee_cents > 0" class="prize-preview-section">
            <PrizePreviewTable
              :contest-id="contest.id"
              :status="contest.status"
            />
          </section>

          <!-- Participants Section -->
          <section class="participants-section">
            <ParticipantsList
              :participants="participants"
              :loading="loadingParticipants"
              :total-count="contest.participant_count ?? 0"
            />
          </section>
        </div>

        <!-- Right Column -->
        <div class="right-column">
          <TournamentDetailsCard
            :starts-at="contest.starts_at"
            :ends-at="contest.ends_at"
            :market-type="contest.market_type"
            :participant-count="contest.participant_count ?? 0"
            :max-participants="contest.max_participants"
            :qty-total="contest.qty_total"
            :entry-fee-cents="contest.entry_fee_cents"
            :is-joined="isJoined"
            :is-joining="isJoining"
            :can-join="canJoin"
            :status="contest.status"
            @join="handleJoinClick"
            @enter-trading="handleEnterTrading"
            @view-results="handleViewResults"
          />
        </div>
      </div>
    </template>

    <!-- Join Confirmation Modal -->
    <JoinConfirmModal
      v-if="contest"
      :contest="contest"
      :show="showJoinModal"
      @update:show="showJoinModal = $event"
      @joined="handleJoinConfirm"
    />
  </div>
</template>

<style scoped>
.contest-details-page {
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

.content-grid {
  display: grid;
  grid-template-columns: 1fr 340px;
  gap: var(--spacing-lg);
  margin-top: var(--spacing-lg);
}

.left-column {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

.right-column {
  position: sticky;
  top: var(--spacing-lg);
  height: fit-content;
}

.introduction-section {
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

.section-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-md) var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg-secondary);
}

.section-header svg {
  color: var(--color-text-secondary);
}

.section-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.introduction-content {
  padding: var(--spacing-lg);
}

.introduction-content p {
  font-size: var(--font-size-sm);
  line-height: 1.6;
  color: var(--color-text-secondary);
  margin: 0;
}

.introduction-content p strong {
  color: var(--color-primary);
  font-weight: 600;
}

/* RTL Support */
[dir="rtl"] .back-button {
  flex-direction: row-reverse;
}

[dir="rtl"] .back-button svg {
  transform: rotate(180deg);
}

[dir="rtl"] .section-header {
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
  .contest-details-page {
    padding: var(--spacing-md);
  }

  .content-grid {
    gap: var(--spacing-md);
    margin-top: var(--spacing-md);
  }

  .section-header {
    padding: var(--spacing-sm) var(--spacing-md);
  }

  .introduction-content {
    padding: var(--spacing-md);
  }
}
</style>
