<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useContestsStore, type Contest } from '@/stores/contests';
import { useWalletStore } from '@/stores/wallet';
import { useToast } from '@/composables/useToast';
import CountdownTimer from './CountdownTimer.vue';

const props = defineProps<{
  contest: Contest;
  show: boolean;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
  'joined': [contestId: string];
}>();

const router = useRouter();
const contestsStore = useContestsStore();
const walletStore = useWalletStore();
const toast = useToast();

const isJoining = ref(false);

// Computed values
const entryFee = computed(() => {
  if (props.contest.entry_fee_cents === 0) {
    return t('contests.free');
  }
  return `$${(props.contest.entry_fee_cents / 100).toFixed(2)}`;
});

const isFree = computed(() => props.contest.entry_fee_cents === 0);

const duration = computed(() => {
  const start = new Date(props.contest.starts_at);
  const end = new Date(props.contest.ends_at);
  const diffMs = end.getTime() - start.getTime();
  const hours = Math.floor(diffMs / (1000 * 60 * 60));
  const minutes = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60));

  if (hours >= 24) {
    const days = Math.floor(hours / 24);
    return t('contests.durationDays', { days });
  }
  if (hours > 0) {
    return minutes > 0
      ? t('contests.durationHoursMinutes', { hours, minutes })
      : t('contests.durationHours', { hours });
  }
  return t('contests.durationMinutes', { minutes });
});

const participantCount = computed(() => props.contest.participant_count ?? 0);
const maxParticipants = computed(() => props.contest.max_participants);

const estimatedPrizePool = computed(() => {
  if (props.contest.estimated_prize_pool_cents) {
    return props.contest.estimated_prize_pool_cents;
  }
  return Math.round(participantCount.value * props.contest.entry_fee_cents * 0.83);
});

const formattedPrizePool = computed(() => {
  if (estimatedPrizePool.value === 0 && props.contest.entry_fee_cents === 0) {
    return t('contests.practice');
  }
  const amount = estimatedPrizePool.value / 100;
  return `~$${amount.toFixed(0)}`;
});

const prizeWinnersPercentage = computed(() =>
  props.contest.prize_winners_percentage ?? 30
);

const currentBalance = computed(() => {
  return walletStore.balanceCents / 100;
});

const balanceAfterJoin = computed(() => {
  return (walletStore.balanceCents - props.contest.entry_fee_cents) / 100;
});

const symbolsList = computed(() => {
  return props.contest.symbols
    .filter(s => s.enabled)
    .map(s => s.symbol)
    .join(', ');
});

// Methods
function closeModal(): void {
  emit('update:show', false);
}

async function handleConfirmJoin(): Promise<void> {
  isJoining.value = true;

  try {
    await contestsStore.joinContest(props.contest.id);
    toast.success(t('contests.joinSuccess'));
    emit('joined', props.contest.id);
    closeModal();

    // If contest is running, offer to enter trading
    if (props.contest.status === 'running') {
      // Small delay to let the success toast show
      setTimeout(() => {
        router.push(`/trade/${props.contest.id}`);
      }, 1000);
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : t('common.error');
    toast.error(message);
  } finally {
    isJoining.value = false;
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="show" class="modal-overlay" @click.self="closeModal">
      <div class="modal-content">
        <!-- Header -->
        <div class="modal-header">
          <h3 class="modal-title">{{ t('contests.confirmJoin') }}</h3>
          <button class="modal-close" @click="closeModal" :disabled="isJoining">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        <!-- Body -->
        <div class="modal-body">
          <!-- Contest Name -->
          <div class="contest-name-section">
            <span class="contest-icon">
              <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6" />
                <path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18" />
                <path d="M4 22h16" />
                <path d="M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22" />
                <path d="M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22" />
                <path d="M18 2H6v7a6 6 0 0 0 12 0V2Z" />
              </svg>
            </span>
            <h4 class="contest-name">{{ contest.name }}</h4>
          </div>

          <!-- Countdown -->
          <div class="countdown-wrapper">
            <CountdownTimer
              :starts-at="contest.starts_at"
              :ends-at="contest.ends_at"
              :status="contest.status"
            />
          </div>

          <!-- Contest Details -->
          <div class="details-grid">
            <div class="detail-item">
              <span class="detail-label">{{ t('contests.duration') }}</span>
              <span class="detail-value">{{ duration }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">{{ t('contests.participants') }}</span>
              <span class="detail-value">
                {{ participantCount }}{{ maxParticipants ? `/${maxParticipants}` : '' }}
              </span>
            </div>
            <div class="detail-item">
              <span class="detail-label">{{ t('contests.prizePool') }}</span>
              <span class="detail-value highlight">{{ formattedPrizePool }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">{{ t('contests.topWin') }}</span>
              <span class="detail-value">{{ t('contests.topPercent', { percent: prizeWinnersPercentage }) }}</span>
            </div>
          </div>

          <!-- Symbols -->
          <div class="symbols-section">
            <span class="section-label">{{ t('contest.availableSymbols') }}</span>
            <span class="symbols-value">{{ symbolsList }}</span>
          </div>

          <!-- Entry Fee Summary -->
          <div class="fee-summary">
            <div class="fee-row">
              <span>{{ t('contests.entryFee') }}</span>
              <span :class="['fee-amount', { 'fee-free': isFree }]">{{ entryFee }}</span>
            </div>
            <template v-if="!isFree">
              <div class="fee-row fee-row-small">
                <span>{{ t('contests.currentBalance') }}</span>
                <span>${{ currentBalance.toFixed(2) }}</span>
              </div>
              <div class="fee-divider"></div>
              <div class="fee-row">
                <span>{{ t('contests.balanceAfter') }}</span>
                <span :class="{ 'balance-warning': balanceAfterJoin < 0 }">
                  ${{ balanceAfterJoin.toFixed(2) }}
                </span>
              </div>
            </template>
          </div>
        </div>

        <!-- Footer -->
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeModal" :disabled="isJoining">
            {{ t('common.cancel') }}
          </button>
          <button class="btn btn-primary" @click="handleConfirmJoin" :disabled="isJoining">
            <span v-if="isJoining" class="btn-loading">
              <span class="spinner"></span>
              {{ t('common.loading') }}
            </span>
            <span v-else>
              {{ isFree ? t('contests.joinFree') : t('contests.payAndJoin', { amount: entryFee }) }}
            </span>
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
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
  z-index: var(--z-modal, 1000);
  padding: var(--spacing-md);
  backdrop-filter: blur(2px);
}

.modal-content {
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  max-width: 440px;
  width: 100%;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.2);
  animation: modalSlideIn 0.2s ease-out;
}

@keyframes modalSlideIn {
  from {
    opacity: 0;
    transform: translateY(-20px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
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
  transition: all var(--transition-fast);
}

.modal-close:hover:not(:disabled) {
  background: var(--color-bg-secondary);
}

.modal-close:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.modal-close svg {
  width: 20px;
  height: 20px;
}

.modal-body {
  padding: var(--spacing-lg);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

.contest-name-section {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  text-align: center;
  flex-direction: column;
}

.contest-icon {
  width: 56px;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, var(--color-primary-light, #EEF2FF), var(--color-bg-secondary));
  border-radius: var(--radius-lg);
  color: var(--color-primary);
}

.contest-name {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.countdown-wrapper {
  display: flex;
  justify-content: center;
  padding: var(--spacing-sm);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.details-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-md);
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--spacing-sm);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.detail-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.detail-value {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.detail-value.highlight {
  color: var(--color-success);
  font-weight: 600;
}

.symbols-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.section-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.symbols-value {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.fee-summary {
  padding: var(--spacing-md);
  background: var(--color-bg-tertiary);
  border-radius: var(--radius-md);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.fee-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.fee-row-small {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.fee-amount {
  font-weight: 600;
}

.fee-free {
  color: var(--color-success);
}

.fee-divider {
  height: 1px;
  background: var(--color-border);
}

.balance-warning {
  color: var(--color-danger);
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

@media (max-width: 480px) {
  .modal-content {
    max-height: 100vh;
    border-radius: 0;
    margin: 0;
  }

  .details-grid {
    grid-template-columns: 1fr;
  }

  .modal-footer {
    flex-direction: column;
  }
}
</style>
