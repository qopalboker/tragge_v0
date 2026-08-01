<script setup lang="ts">
import { computed, ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useContestsStore, type Contest, type DurationType, type MarketType } from '@/stores/contests';
import { useWalletStore } from '@/stores/wallet';
import { useToast } from '@/composables/useToast';
import CountdownTimer from './CountdownTimer.vue';

const props = defineProps<{
  contest: Contest;
  compact?: boolean;
}>();

const emit = defineEmits<{
  joined: [contestId: string];
  joinClick: [contest: Contest];
}>();

const router = useRouter();
const contestsStore = useContestsStore();
const walletStore = useWalletStore();
const toast = useToast();
const showDetails = ref(false);
const showDepositModal = ref(false);

// Ensure wallet is loaded for balance checks on paid contests
onMounted(() => {
  if (props.contest.entry_fee_cents > 0 && !walletStore.wallet) {
    walletStore.fetchWallet();
  }
});

const entryFee = computed(() => {
  if (props.contest.entry_fee_cents === 0) {
    return t('contests.free');
  }
  return `$${(props.contest.entry_fee_cents / 100).toFixed(2)}`;
});

const formattedStartTime = computed(() => {
  const date = new Date(props.contest.starts_at);
  return date.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
});

const formattedEndTime = computed(() => {
  const date = new Date(props.contest.ends_at);
  return date.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
});

const duration = computed(() => {
  const start = new Date(props.contest.starts_at);
  const end = new Date(props.contest.ends_at);
  const diffMs = end.getTime() - start.getTime();
  const hours = Math.floor(diffMs / (1000 * 60 * 60));
  const minutes = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60));

  if (hours >= 24) {
    const days = Math.floor(hours / 24);
    return `${days}d`;
  }
  if (hours > 0) {
    return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
  }
  return `${minutes}m`;
});

// Duration type badge
const durationTypeIcons: Record<DurationType, string> = {
  rush_30min: '\u26A1',
  hourly: '\u23F1\uFE0F',
  four_hour: '\uD83D\uDD53',
  daily: '\uD83D\uDCC5',
  weekly: '\uD83D\uDCC6',
};

// Market type icons
const marketTypeIcons: Record<MarketType, string> = {
  crypto: '\u20BF',
  forex: '\uD83D\uDCB1',
  stocks: '\uD83D\uDCC8',
  mixed: '\uD83C\uDFAF',
};

const durationTypeLabel = computed(() => {
  if (!props.contest.duration_type) return null;
  return t(`filters.duration.${props.contest.duration_type}`);
});

const durationTypeIcon = computed(() => {
  if (!props.contest.duration_type) return null;
  return durationTypeIcons[props.contest.duration_type] || '';
});

const marketTypeLabel = computed(() => {
  if (!props.contest.market_type) return null;
  return t(`filters.market.${props.contest.market_type}`);
});

const marketTypeIcon = computed(() => {
  if (!props.contest.market_type) return null;
  return marketTypeIcons[props.contest.market_type] || '';
});

// Participant count
const participantCount = computed(() => props.contest.participant_count ?? 0);
const maxParticipants = computed(() => props.contest.max_participants);
const participantDisplay = computed(() => {
  if (maxParticipants.value) {
    return `${participantCount.value}/${maxParticipants.value}`;
  }
  return participantCount.value.toString();
});
const participantPercentage = computed(() => {
  if (!maxParticipants.value) return 0;
  return Math.min(100, (participantCount.value / maxParticipants.value) * 100);
});

// Estimated prize pool
const estimatedPrizePool = computed(() => {
  // If provided directly from API
  if (props.contest.estimated_prize_pool_cents) {
    return props.contest.estimated_prize_pool_cents;
  }
  // Estimate based on current participants and entry fee
  // Assuming 83% of entry fees go to prize pool (17% platform fee)
  return Math.round(participantCount.value * props.contest.entry_fee_cents * 0.83);
});

const formattedPrizePool = computed(() => {
  if (estimatedPrizePool.value === 0 && props.contest.entry_fee_cents === 0) {
    return t('contests.practice');
  }
  const amount = estimatedPrizePool.value / 100;
  if (amount >= 1000) {
    return `~$${(amount / 1000).toFixed(1)}K`;
  }
  return `~$${amount.toFixed(0)}`;
});

// Prize winners percentage
const prizeWinnersPercentage = computed(() =>
  props.contest.prize_winners_percentage ?? 30
);

const symbolsList = computed(() => {
  return props.contest.symbols
    .filter(s => s.enabled)
    .map(s => s.symbol)
    .slice(0, 3)
    .join(', ') + (props.contest.symbols.length > 3 ? '...' : '');
});

const canJoin = computed(() => {
  return props.contest.status === 'registration_open' && !contestsStore.isJoined(props.contest.id);
});

const isJoining = computed(() => contestsStore.isJoining(props.contest.id));
const isJoined = computed(() => contestsStore.isJoined(props.contest.id));

// Balance check for paid contests
const hasSufficientBalance = computed(() => {
  if (props.contest.entry_fee_cents === 0) return true;
  return walletStore.balanceCents >= props.contest.entry_fee_cents;
});

const amountNeeded = computed(() => {
  if (hasSufficientBalance.value) return 0;
  return props.contest.entry_fee_cents - walletStore.balanceCents;
});

const formattedAmountNeeded = computed(() => {
  return `$${(amountNeeded.value / 100).toFixed(2)}`;
});

function toggleDetails(): void {
  showDetails.value = !showDetails.value;
}

function closeDepositModal(): void {
  showDepositModal.value = false;
}

function goToDeposit(): void {
  showDepositModal.value = false;
  walletStore.openDepositModal();
}

function handleJoinClick(): void {
  // Check balance for paid contests
  if (props.contest.entry_fee_cents > 0 && !hasSufficientBalance.value) {
    showDepositModal.value = true;
    return;
  }

  // Emit joinClick for parent to show confirmation modal
  emit('joinClick', props.contest);
}

async function handleJoin(): Promise<void> {
  try {
    await contestsStore.joinContest(props.contest.id);
    toast.success(t('contests.joinSuccess'));
    emit('joined', props.contest.id);
  } catch (err) {
    const message = err instanceof Error ? err.message : t('common.error');
    // Check if error is insufficient balance from backend
    if (message.toLowerCase().includes('insufficient') || message.toLowerCase().includes('balance')) {
      showDepositModal.value = true;
    } else {
      toast.error(message);
    }
  }
}

// Expose handleJoin for parent to call after confirmation
defineExpose({ handleJoin });
</script>

<template>
  <div :class="['contest-card', 'card', { 'contest-card-compact': compact }]">
    <!-- Header -->
    <div class="card-header">
      <div class="contest-title">
        <span class="contest-name">{{ contest.name }}</span>
        <span class="contest-duration">{{ duration }}</span>
      </div>
      <div class="header-badges">
        <span v-if="marketTypeLabel" class="market-type-badge">
          <span class="market-icon">{{ marketTypeIcon }}</span>
          <span class="market-label">{{ marketTypeLabel }}</span>
        </span>
        <span v-if="durationTypeLabel" class="duration-type-badge">
          <span class="duration-icon">{{ durationTypeIcon }}</span>
          <span class="duration-label">{{ durationTypeLabel }}</span>
        </span>
      </div>
    </div>

    <!-- Countdown Timer -->
    <div class="countdown-section">
      <CountdownTimer
        :starts-at="contest.starts_at"
        :ends-at="contest.ends_at"
        :status="contest.status"
        :compact="compact"
      />
    </div>

    <!-- Description -->
    <p v-if="contest.description && !compact" class="contest-description">
      {{ contest.description }}
    </p>

    <!-- Main Info Row -->
    <div class="info-row">
      <div class="info-item">
        <span class="info-icon">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <path d="M12 6v6l4 2" />
          </svg>
        </span>
        <span class="info-text">{{ duration }}</span>
      </div>
      <div class="info-item">
        <span class="info-icon">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="2" y="4" width="20" height="16" rx="2" />
            <path d="M7 15h10M7 11h4" />
          </svg>
        </span>
        <span class="info-text">{{ entryFee }}</span>
      </div>
      <div class="info-item">
        <span class="info-icon">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
            <circle cx="9" cy="7" r="4" />
            <path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
          </svg>
        </span>
        <span class="info-text">{{ participantDisplay }} {{ t('contests.joined') }}</span>
      </div>
    </div>

    <!-- Prize Pool and Participants -->
    <div class="prize-section">
      <div class="prize-info">
        <span class="prize-label">{{ t('contests.prizePool') }}</span>
        <span class="prize-value">{{ formattedPrizePool }}</span>
      </div>
      <div class="prize-info">
        <span class="prize-label">{{ t('contests.topWin') }}</span>
        <span class="prize-value">{{ t('contests.topPercent', { percent: prizeWinnersPercentage }) }}</span>
      </div>
    </div>

    <!-- Participant Progress Bar -->
    <div v-if="maxParticipants" class="participants-progress">
      <div class="progress-bar">
        <div
          class="progress-fill"
          :style="{ width: `${participantPercentage}%` }"
          :class="{ 'progress-full': participantPercentage >= 90 }"
        ></div>
      </div>
      <span class="progress-label">{{ participantDisplay }} {{ t('contests.slots') }}</span>
    </div>

    <!-- Stats Grid (collapsed by default) -->
    <div v-if="showDetails" class="stats-grid">
      <div class="stat">
        <span class="stat-label">{{ t('contest.starts') }}</span>
        <span class="stat-value">{{ formattedStartTime }}</span>
      </div>
      <div class="stat">
        <span class="stat-label">{{ t('contest.ends') }}</span>
        <span class="stat-value">{{ formattedEndTime }}</span>
      </div>
      <div class="stat">
        <span class="stat-label">{{ t('contest.symbols') }}</span>
        <span class="stat-value">{{ symbolsList || '-' }}</span>
      </div>
      <div class="stat">
        <span class="stat-label">{{ t('contest.tradingCapital') }}</span>
        <span class="stat-value">${{ contest.qty_total.toLocaleString() }}</span>
      </div>
    </div>

    <!-- Expandable Details -->
    <div v-if="showDetails" class="details-section">
      <div class="details-content">
        <h4>{{ t('contest.availableSymbols') }}</h4>
        <div class="symbols-list">
          <span
            v-for="symbol in contest.symbols"
            :key="symbol.symbol"
            :class="['symbol-tag', { 'symbol-disabled': !symbol.enabled }]"
          >
            {{ symbol.symbol }}
          </span>
        </div>

        <template v-if="contest.rules">
          <h4>{{ t('contest.rules') }}</h4>
          <p class="rules-text">{{ JSON.stringify(contest.rules, null, 2) }}</p>
        </template>
      </div>
    </div>

    <!-- Actions -->
    <div class="card-actions">
      <button class="btn btn-secondary btn-sm" @click="toggleDetails">
        {{ showDetails ? t('common.hide') : t('contests.details') }}
      </button>
      <button
        v-if="canJoin"
        class="btn btn-primary join-btn"
        :disabled="isJoining"
        @click="handleJoinClick"
      >
        <span v-if="isJoining" class="btn-loading">
          <span class="spinner"></span>
        </span>
        <span v-else>{{ t('contests.joinNow') }}</span>
      </button>
      <template v-else-if="isJoined">
        <span class="joined-badge">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="20 6 9 17 4 12" />
          </svg>
          {{ t('contests.joined') }}
        </span>
        <a
          v-if="contest.status === 'running'"
          :href="`/trade/${contest.id}`"
          class="btn btn-primary enter-trading-btn"
        >
          {{ t('contests.enterTrading') }}
        </a>
      </template>
    </div>

    <!-- Insufficient Balance Modal -->
    <Teleport to="body">
      <div v-if="showDepositModal" class="modal-overlay" @click.self="closeDepositModal">
        <div class="modal-content">
          <div class="modal-header">
            <h3 class="modal-title">{{ t('contests.depositRequired') }}</h3>
            <button class="modal-close" @click="closeDepositModal">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>
          <div class="modal-body">
            <div class="deposit-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 12V7H5a2 2 0 0 1 0-4h14v4" />
                <path d="M3 5v14a2 2 0 0 0 2 2h16v-5" />
                <path d="M18 12a2 2 0 0 0 0 4h4v-4h-4z" />
              </svg>
            </div>
            <p class="deposit-message">
              {{ t('contests.depositRequiredDesc').replace('{amount}', formattedAmountNeeded) }}
            </p>
          </div>
          <div class="modal-footer">
            <button class="btn btn-secondary" @click="closeDepositModal">
              {{ t('common.cancel') }}
            </button>
            <button class="btn btn-primary" @click="goToDeposit">
              {{ t('contests.depositNow') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.contest-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.contest-card-compact {
  padding: var(--spacing-md);
}

.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--spacing-md);
}

.header-badges {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--spacing-xs);
}

[dir="rtl"] .header-badges {
  align-items: flex-start;
}

.duration-type-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
  border-radius: var(--radius-md);
}

.duration-icon {
  font-size: var(--font-size-sm);
  line-height: 1;
}

.duration-label {
  line-height: 1.2;
}

.market-type-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
  background-color: var(--color-primary-light, #EEF2FF);
  color: var(--color-primary);
  border-radius: var(--radius-md);
}

.market-icon {
  font-size: var(--font-size-sm);
  line-height: 1;
}

.market-label {
  line-height: 1.2;
}

.countdown-section {
  padding: var(--spacing-sm) 0;
  border-bottom: 1px solid var(--color-border-light, var(--color-border));
}

.info-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-md);
  padding: var(--spacing-sm) 0;
}

.info-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.info-icon {
  display: flex;
  align-items: center;
  color: var(--color-text-muted);
}

.info-text {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.prize-section {
  display: flex;
  justify-content: space-between;
  padding: var(--spacing-md);
  background: linear-gradient(135deg, var(--color-bg-secondary), var(--color-bg-tertiary));
  border-radius: var(--radius-md);
}

.prize-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.prize-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.prize-value {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
}

.participants-progress {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.progress-bar {
  height: 6px;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-full);
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--color-primary), var(--color-primary-light, #6366F1));
  border-radius: var(--radius-full);
  transition: width 0.3s ease;
}

.progress-fill.progress-full {
  background: linear-gradient(90deg, var(--color-warning), var(--color-danger));
}

.progress-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-align: right;
}

[dir="rtl"] .progress-label {
  text-align: left;
}

.contest-title {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-sm);
}

.contest-name {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
}

.contest-duration {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.contest-description {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  line-height: 1.5;
}

.status-badge {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 600;
  border-radius: var(--radius-md);
  text-transform: uppercase;
  letter-spacing: 0.025em;
  flex-shrink: 0;
}

.status-open {
  background-color: #ECFDF5;
  color: #059669;
}

.status-scheduled {
  background-color: #FEF3C7;
  color: #D97706;
}

.status-live {
  background-color: #FEE2E2;
  color: #DC2626;
}

.status-paused {
  background-color: #FEF3C7;
  color: #D97706;
}

.status-ended {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-sm);
}

.stat {
  display: flex;
  justify-content: space-between;
  padding: var(--spacing-xs) var(--spacing-sm);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-sm);
}

.stat-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.stat-value {
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-primary);
}

.details-section {
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
}

.details-content h4 {
  font-size: var(--font-size-sm);
  font-weight: 600;
  margin-bottom: var(--spacing-xs);
  color: var(--color-text-primary);
}

.details-content p {
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-sm);
}

.details-content p:last-child {
  margin-bottom: 0;
}

.symbols-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
  margin-bottom: var(--spacing-sm);
}

.symbol-tag {
  padding: var(--spacing-xs) var(--spacing-sm);
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
}

.symbol-disabled {
  opacity: 0.5;
  text-decoration: line-through;
}

.rules-text {
  font-family: monospace;
  font-size: var(--font-size-xs);
  white-space: pre-wrap;
  background-color: var(--color-bg-primary);
  padding: var(--spacing-sm);
  border-radius: var(--radius-sm);
}

.card-actions {
  display: flex;
  gap: var(--spacing-sm);
  align-items: center;
  margin-top: auto;
  padding-top: var(--spacing-sm);
}

.card-actions .btn-sm {
  flex: 0 0 auto;
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
}

.join-btn {
  flex: 1;
  padding: var(--spacing-sm) var(--spacing-md);
  background: linear-gradient(135deg, var(--color-primary), var(--color-primary-dark, #4F46E5));
  font-weight: 600;
}

.join-btn:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(79, 70, 229, 0.3);
}

.joined-badge {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  text-align: center;
  background-color: #ECFDF5;
  color: #059669;
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: 500;
}

.enter-trading-btn {
  flex: 1;
  text-align: center;
  text-decoration: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #10b981, #059669);
  color: white !important;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.btn-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
}

.spinner {
  width: 14px;
  height: 14px;
  border: 2px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Modal Styles */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: var(--spacing-md);
}

.modal-content {
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  max-width: 400px;
  width: 100%;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.15);
  animation: modalSlideIn 0.2s ease-out;
}

@keyframes modalSlideIn {
  from {
    opacity: 0;
    transform: translateY(-20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.modal-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.modal-close {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: background-color var(--transition-fast);
}

.modal-close:hover {
  background: var(--color-bg-secondary);
}

.modal-close svg {
  width: 20px;
  height: 20px;
}

.modal-body {
  padding: var(--spacing-xl);
  text-align: center;
}

.deposit-icon {
  width: 64px;
  height: 64px;
  background: var(--color-primary-light, #EEF2FF);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto var(--spacing-lg);
}

.deposit-icon svg {
  width: 32px;
  height: 32px;
  color: var(--color-primary);
}

.deposit-message {
  font-size: var(--font-size-md);
  color: var(--color-text-secondary);
  line-height: 1.5;
  margin: 0;
}

.modal-footer {
  display: flex;
  gap: var(--spacing-sm);
  padding: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

.modal-footer .btn {
  flex: 1;
}
</style>
