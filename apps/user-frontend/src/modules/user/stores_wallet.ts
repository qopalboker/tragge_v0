import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import {
  walletApi,
  kycApi,
  WalletApiError,
  type Wallet,
  type WalletTransaction,
  type WalletTransactionType,
  type FiatDepositResponse,
  type WalletCryptoDepositResponse,
  type WalletWithdrawRequest,
	type PayoutStatus,
	type KYCStatusResponse,
} from '@/api';

export const useWalletStore = defineStore('wallet', () => {
  // ==================== State ====================

  const showDepositModal = ref(false);
  const showWithdrawModal = ref(false);

  const wallet = ref<Wallet | null>(null);
  const transactions = ref<WalletTransaction[]>([]);
  const totalTransactions = ref(0);
  const hasMoreTransactions = ref(false);
  const kycStatus = ref<KYCStatusResponse | null>(null);

  // Loading states
  const loading = ref(false);
  const transactionsLoading = ref(false);
  const error = ref<string | null>(null);
  const errorCode = ref<string | null>(null);

  // ==================== Computed ====================

  const availableBalance = computed(() => (wallet.value?.balance_cents ?? 0) / 100);
  const pendingBalance = computed(() => 0);
  const currency = computed(() => wallet.value?.currency ?? 'USD');
  const balanceCents = computed(() => wallet.value?.balance_cents ?? 0);
  const walletStatus = computed(() => wallet.value?.status ?? 'active');
  const isWalletActive = computed(() => walletStatus.value === 'active');
  const isWalletFrozen = computed(() => walletStatus.value === 'frozen');
  const isKYCVerified = computed(() => kycStatus.value?.status === 'verified');
  const kycRequiredForWithdrawal = computed(() => true);

  /**
   * Formatted balance string (e.g., "$123.45")
   */
  const formattedBalance = computed(() => {
    const cents = wallet.value?.balance_cents ?? 0;
    const curr = wallet.value?.currency ?? 'USD';
    const amount = cents / 100;

    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: curr,
    }).format(amount);
  });

  /**
   * Formatted balance with explicit sign for positive amounts
   */
  const formattedBalanceWithSign = computed(() => {
    const cents = wallet.value?.balance_cents ?? 0;
    const curr = wallet.value?.currency ?? 'USD';
    const amount = cents / 100;
    const sign = amount > 0 ? '+' : '';

    const formatted = new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: curr,
    }).format(amount);

    return amount > 0 ? `${sign}${formatted}` : formatted;
  });

  // ==================== Actions ====================

  /**
   * Fetch wallet data
   */
  async function fetchWallet(): Promise<void> {
    loading.value = true;
    error.value = null;
    errorCode.value = null;

    try {
      wallet.value = await walletApi.getWallet();
    } catch (err: unknown) {
      handleWalletError(err, 'Failed to load wallet');
    } finally {
      loading.value = false;
    }
  }

  /**
   * Refresh wallet balance (alias for fetchWallet)
   */
  async function refreshBalance(): Promise<void> {
    await fetchWallet();
  }

  /**
   * Fetch transaction history
   */
  async function fetchHistory(params?: {
    limit?: number;
    offset?: number;
    type?: WalletTransactionType;
  }): Promise<void> {
    transactionsLoading.value = true;
    error.value = null;
    errorCode.value = null;

    try {
      const response = await walletApi.getHistory(params ?? {});
      transactions.value = response.transactions;
      totalTransactions.value = response.total;
      hasMoreTransactions.value = response.has_more;
    } catch (err: unknown) {
      handleWalletError(err, 'Failed to load transactions');
    } finally {
      transactionsLoading.value = false;
    }
  }

  /**
   * Load more transactions (append to existing)
   */
  async function loadMoreTransactions(params?: {
    limit?: number;
    type?: WalletTransactionType;
  }): Promise<void> {
    if (!hasMoreTransactions.value) return;

    transactionsLoading.value = true;
    error.value = null;

    try {
      const response = await walletApi.getHistory({
        ...params,
        offset: transactions.value.length,
      });
      transactions.value = [...transactions.value, ...response.transactions];
      hasMoreTransactions.value = response.has_more;
    } catch (err: unknown) {
      handleWalletError(err, 'Failed to load more transactions');
    } finally {
      transactionsLoading.value = false;
    }
  }

  /**
   * Fetch KYC verification status
   */
  async function fetchKYCStatus(): Promise<void> {
    try {
      kycStatus.value = await kycApi.getStatus();
    } catch {
      // KYC status endpoint might not exist, treat as not verified
      kycStatus.value = null;
    }
  }

  // ==================== Deposit Actions ====================

  /**
   * Create fiat deposit (Jibit gateway)
   * @param amount_usd_cents - Amount in USD cents (backend converts to IRR)
   */
  async function createFiatDeposit(amount_usd_cents: number): Promise<FiatDepositResponse | null> {
    loading.value = true;
    error.value = null;
    errorCode.value = null;

    try {
      return await walletApi.createFiatDeposit(amount_usd_cents);
    } catch (err: unknown) {
      handleWalletError(err, 'Failed to create deposit');
      return null;
    } finally {
      loading.value = false;
    }
  }

  /**
	 * Create crypto deposit through the active crypto gateway.
   */
	async function createCryptoDeposit(
		amount_cents: number,
		pay_currency: string = 'usdttrc20'
	): Promise<WalletCryptoDepositResponse | null> {
    loading.value = true;
    error.value = null;
    errorCode.value = null;

    try {
		return await walletApi.createCryptoDeposit(amount_cents, pay_currency);
    } catch (err: unknown) {
      handleWalletError(err, 'Failed to create crypto deposit');
      return null;
    } finally {
      loading.value = false;
    }
  }

  // ==================== Withdrawal Actions ====================

  /**
   * Request withdrawal
   */
  async function requestWithdraw(request: WalletWithdrawRequest): Promise<{ payout_id: string; status: string } | null> {
    loading.value = true;
    error.value = null;
    errorCode.value = null;

    try {
      const result = await walletApi.requestWithdraw(request);
      // Refresh wallet after withdrawal request
      await fetchWallet();
      return result;
    } catch (err: unknown) {
      handleWalletError(err, 'Failed to request withdrawal');
      return null;
    } finally {
      loading.value = false;
    }
  }

  /**
   * Get withdrawal status
   */
  async function getWithdrawStatus(payout_id: string): Promise<PayoutStatus | null> {
    loading.value = true;
    error.value = null;
    errorCode.value = null;

    try {
      return await walletApi.getWithdrawStatus(payout_id);
    } catch (err: unknown) {
      handleWalletError(err, 'Failed to get withdrawal status');
      return null;
    } finally {
      loading.value = false;
    }
  }

  // ==================== Initialization ====================

  async function initialize(): Promise<void> {
    await Promise.all([
      fetchWallet(),
      fetchKYCStatus(),
      fetchHistory({ limit: 10 }),
    ]);
  }

  // ==================== Utilities ====================

  function openDepositModal(): void {
    showDepositModal.value = true;
  }

  function openWithdrawModal(): void {
    showWithdrawModal.value = true;
  }

  function clearError(): void {
    error.value = null;
    errorCode.value = null;
  }

  function handleWalletError(err: unknown, fallback: string): void {
    if (err instanceof WalletApiError) {
      error.value = err.message;
      errorCode.value = err.code;
    } else if (err && typeof err === 'object' && 'response' in err) {
      const axiosError = err as { response?: { data?: { error?: string; message?: string } } };
      error.value = axiosError.response?.data?.error || axiosError.response?.data?.message || fallback;
      errorCode.value = null;
    } else {
      error.value = fallback;
      errorCode.value = null;
    }
  }

  /**
   * Check if error is KYC required
   */
  const isKYCError = computed(() => errorCode.value === 'KYC_REQUIRED');

  /**
   * Format cents to currency string
   */
  function formatCents(cents: number, curr?: string): string {
    const amount = cents / 100;
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: curr ?? currency.value,
    }).format(amount);
  }

  return {
    // State
    showDepositModal,
    showWithdrawModal,
    wallet,
    transactions,
    totalTransactions,
    hasMoreTransactions,
    kycStatus,
    loading,
    transactionsLoading,
    error,
    errorCode,
    // Computed
    availableBalance,
    pendingBalance,
    currency,
    balanceCents,
    walletStatus,
    isWalletActive,
    isWalletFrozen,
    formattedBalance,
    formattedBalanceWithSign,
    isKYCVerified,
    kycRequiredForWithdrawal,
    isKYCError,
    // Actions
    fetchWallet,
    refreshBalance,
    fetchHistory,
    loadMoreTransactions,
    fetchKYCStatus,
    createFiatDeposit,
    createCryptoDeposit,
    requestWithdraw,
    getWithdrawStatus,
    initialize,
    // Modal actions
    openDepositModal,
    openWithdrawModal,
    // Utilities
    clearError,
    formatCents,
  };
});
