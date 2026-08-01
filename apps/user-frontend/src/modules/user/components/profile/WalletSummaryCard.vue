<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useWalletStore } from '@/stores/wallet';
import IconWallet from '@/components/icons/IconWallet.vue';

const router = useRouter();
const walletStore = useWalletStore();

const formattedBalance = computed(() => walletStore.formattedBalance);
const isLoading = computed(() => walletStore.loading);

function navigateToWallet(): void {
  router.push('/user/wallet');
}

onMounted(() => {
  if (!walletStore.wallet) {
    walletStore.fetchWallet();
  }
});
</script>

<template>
  <div class="wallet-summary-card" @click="navigateToWallet">
    <div class="wallet-content">
      <div class="wallet-icon-wrapper">
        <IconWallet class="wallet-icon" />
      </div>
      <div class="wallet-info">
        <span class="wallet-label">{{ t('profile.walletBalance') }}</span>
        <span v-if="isLoading" class="wallet-balance loading">...</span>
        <span v-else class="wallet-balance">{{ formattedBalance }}</span>
      </div>
    </div>
    <div class="wallet-action">
      <span class="view-wallet-text">{{ t('profile.viewWallet') }}</span>
      <svg class="arrow-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M5 12h14M12 5l7 7-7 7"/>
      </svg>
    </div>
  </div>
</template>

<style scoped>
.wallet-summary-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-lg);
  background: linear-gradient(135deg, var(--color-primary) 0%, #7C3AED 100%);
  border-radius: var(--radius-lg);
  color: white;
  cursor: pointer;
  transition: transform var(--transition-fast), box-shadow var(--transition-fast);
}

.wallet-summary-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-lg);
}

.wallet-content {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.wallet-icon-wrapper {
  width: 48px;
  height: 48px;
  background: rgba(255, 255, 255, 0.2);
  border-radius: var(--radius-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.wallet-icon {
  width: 24px;
  height: 24px;
  color: white;
}

.wallet-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.wallet-label {
  font-size: var(--font-size-sm);
  opacity: 0.85;
}

.wallet-balance {
  font-size: var(--font-size-xl);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.wallet-balance.loading {
  opacity: 0.6;
}

.wallet-action {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  opacity: 0.9;
}

.view-wallet-text {
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.arrow-icon {
  transition: transform var(--transition-fast);
}

.wallet-summary-card:hover .arrow-icon {
  transform: translateX(4px);
}

[dir="rtl"] .arrow-icon {
  transform: rotate(180deg);
}

[dir="rtl"] .wallet-summary-card:hover .arrow-icon {
  transform: rotate(180deg) translateX(4px);
}

/* Mobile */
@media (max-width: 767px) {
  .wallet-summary-card {
    flex-direction: column;
    align-items: stretch;
    gap: var(--spacing-md);
  }

  .wallet-content {
    justify-content: flex-start;
  }

  .wallet-action {
    justify-content: center;
    padding-top: var(--spacing-sm);
    border-top: 1px solid rgba(255, 255, 255, 0.2);
  }
}
</style>
