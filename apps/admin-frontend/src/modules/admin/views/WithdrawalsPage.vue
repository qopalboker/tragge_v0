<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { useAuthStore } from '@/stores/auth';
import { SensitiveAdminAction, withPasswordReauthentication } from '@/api/reauthentication';
import {
  getWithdrawals,
  getWithdrawal,
  approveWithdrawal,
  rejectWithdrawal,
  completeWithdrawal,
  failWithdrawal,
  addWithdrawalComment,
  formatAmount,
  getStatusColor,
  canApprove,
  canReject,
  canComplete,
  canFail,
  WithdrawalStatus,
  type Withdrawal,
  type WithdrawalDetail,
} from '@/api/withdrawals';

const toast = useToast();
const auth = useAuthStore();

// State
const withdrawals = ref<Withdrawal[]>([]);
const selectedWithdrawal = ref<WithdrawalDetail | null>(null);
const loading = ref(true);
const loadingDetail = ref(false);
const error = ref<string | null>(null);
const statusFilter = ref<WithdrawalStatus | ''>('');
const currentPage = ref(1);
const totalWithdrawals = ref(0);
const perPage = 20;

// Modal states
const showDetailModal = ref(false);
const showApproveModal = ref(false);
const showRejectModal = ref(false);
const showCompleteModal = ref(false);
const showFailModal = ref(false);
const showCommentModal = ref(false);
const actionLoading = ref(false);
const approveComment = ref('');
const rejectReason = ref('');
const completeComment = ref('');
const completeTransactionId = ref('');
const failReason = ref('');
const commentText = ref('');

// Permissions
const canManage = computed(() => auth.hasPermission('withdrawals.manage'));

// Computed
const pendingCount = computed(() => {
  return withdrawals.value.filter(w => w.status === WithdrawalStatus.Pending).length;
});

const hasMorePages = computed(() => {
  return currentPage.value * perPage < totalWithdrawals.value;
});

// Status tabs
const statusTabs = [
  { value: '', label: t('withdrawals.tabs.all') },
  { value: WithdrawalStatus.Pending, label: t('withdrawals.tabs.pending') },
  { value: WithdrawalStatus.Processing, label: t('withdrawals.tabs.processing') },
  { value: WithdrawalStatus.Succeeded, label: t('withdrawals.tabs.completed') },
  { value: WithdrawalStatus.Rejected, label: t('withdrawals.tabs.rejected') },
];

// Methods
async function fetchWithdrawals(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    const response = await getWithdrawals({
      status: statusFilter.value || undefined,
      page: currentPage.value,
      limit: perPage,
    });
    withdrawals.value = response.withdrawals || [];
    totalWithdrawals.value = response.total;
  } catch {
    error.value = t('common.error');
    withdrawals.value = [];
    totalWithdrawals.value = 0;
  } finally {
    loading.value = false;
  }
}

async function openDetailModal(withdrawal: Withdrawal): Promise<void> {
  showDetailModal.value = true;
  loadingDetail.value = true;

  try {
    selectedWithdrawal.value = await getWithdrawal(withdrawal.id);
  } catch {
    toast.error(t('withdrawals.loadError'));
    showDetailModal.value = false;
  } finally {
    loadingDetail.value = false;
  }
}

function closeDetailModal(): void {
  showDetailModal.value = false;
  selectedWithdrawal.value = null;
}

function openApproveModal(): void {
  approveComment.value = '';
  showApproveModal.value = true;
}

function openRejectModal(): void {
  rejectReason.value = '';
  showRejectModal.value = true;
}

function openCommentModal(): void {
  commentText.value = selectedWithdrawal.value?.admin_comment || '';
  showCommentModal.value = true;
}

async function handleApprove(): Promise<void> {
  if (!selectedWithdrawal.value) return;

  actionLoading.value = true;
  try {
    await approveWithdrawal(selectedWithdrawal.value.id, { comment: approveComment.value });
    toast.success(t('withdrawals.approveSuccess'));
    showApproveModal.value = false;
    closeDetailModal();
    await fetchWithdrawals();
  } catch {
    toast.error(t('withdrawals.approveError'));
  } finally {
    actionLoading.value = false;
  }
}

async function handleReject(): Promise<void> {
  if (!selectedWithdrawal.value || !rejectReason.value.trim()) return;

  actionLoading.value = true;
  try {
    await rejectWithdrawal(selectedWithdrawal.value.id, { comment: rejectReason.value });
    toast.success(t('withdrawals.rejectSuccess'));
    showRejectModal.value = false;
    closeDetailModal();
    await fetchWithdrawals();
  } catch {
    toast.error(t('withdrawals.rejectError'));
  } finally {
    actionLoading.value = false;
  }
}

async function handleComment(): Promise<void> {
  if (!selectedWithdrawal.value || !commentText.value.trim()) return;

  actionLoading.value = true;
  try {
    await addWithdrawalComment(selectedWithdrawal.value.id, { comment: commentText.value });
    toast.success(t('withdrawals.commentSuccess'));
    showCommentModal.value = false;
    // Refresh the detail
    selectedWithdrawal.value = await getWithdrawal(selectedWithdrawal.value.id);
  } catch {
    toast.error(t('withdrawals.commentError'));
  } finally {
    actionLoading.value = false;
  }
}

function openCompleteModal(): void {
  completeComment.value = '';
  completeTransactionId.value = '';
  showCompleteModal.value = true;
}

function openFailModal(): void {
  failReason.value = '';
  showFailModal.value = true;
}

async function handleComplete(): Promise<void> {
  if (!selectedWithdrawal.value || !completeComment.value.trim() || !completeTransactionId.value.trim()) return;

  actionLoading.value = true;
  try {
    const password = window.prompt('Confirm your current Admin password:') || '';
    if (!password) return;
    await withPasswordReauthentication({
      password, action: SensitiveAdminAction.WithdrawalComplete, resourceId: selectedWithdrawal.value.id,
    }, grant => completeWithdrawal(selectedWithdrawal.value!.id, {
      comment: completeComment.value.trim(), transaction_id: completeTransactionId.value.trim(),
    }, grant));
    toast.success(t('withdrawals.completeSuccess'));
    showCompleteModal.value = false;
    closeDetailModal();
    await fetchWithdrawals();
  } catch {
    toast.error(t('withdrawals.completeError'));
  } finally {
    actionLoading.value = false;
  }
}

async function handleFail(): Promise<void> {
  if (!selectedWithdrawal.value || !failReason.value.trim()) return;

  actionLoading.value = true;
  try {
    await failWithdrawal(selectedWithdrawal.value.id, { comment: failReason.value });
    toast.success(t('withdrawals.failSuccess'));
    showFailModal.value = false;
    closeDetailModal();
    await fetchWithdrawals();
  } catch {
    toast.error(t('withdrawals.failError'));
  } finally {
    actionLoading.value = false;
  }
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function getStatusLabel(status: WithdrawalStatus): string {
  return t(`withdrawals.status.${status}`);
}

// Lifecycle
onMounted(() => {
  fetchWithdrawals();
});

// Watchers
watch(statusFilter, () => {
  currentPage.value = 1;
  fetchWithdrawals();
});
</script>

<template>
  <div class="withdrawals-page">
    <!-- Header -->
    <div class="page-header">
      <div class="header-content">
        <h1 class="page-title">{{ t('withdrawals.title') }}</h1>
        <div v-if="pendingCount > 0" class="pending-badge">
          {{ pendingCount }} {{ t('withdrawals.pendingReviews') }}
        </div>
      </div>
    </div>

    <!-- Status Tabs -->
    <div class="status-tabs">
      <button
        v-for="tab in statusTabs"
        :key="tab.value"
        :class="['tab-btn', { active: statusFilter === tab.value }]"
        @click="statusFilter = tab.value as WithdrawalStatus | ''"
      >
        {{ tab.label }}
        <span v-if="tab.value === WithdrawalStatus.Pending && pendingCount > 0" class="tab-badge">
          {{ pendingCount }}
        </span>
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="error-state">
      <p>{{ error }}</p>
      <button class="btn btn-primary" @click="fetchWithdrawals">{{ t('common.retry') }}</button>
    </div>

    <!-- Empty State -->
    <div v-else-if="withdrawals.length === 0" class="empty-state">
      <p>{{ t('withdrawals.noResults') }}</p>
    </div>

    <!-- Withdrawals Table -->
    <div v-else class="table-container">
      <table class="withdrawals-table">
        <thead>
          <tr>
            <th>{{ t('withdrawals.user') }}</th>
            <th>{{ t('withdrawals.amount') }}</th>
            <th>{{ t('withdrawals.destination') }}</th>
            <th>{{ t('withdrawals.status') }}</th>
            <th>{{ t('withdrawals.date') }}</th>
            <th>{{ t('withdrawals.reviewer') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="withdrawal in withdrawals"
            :key="withdrawal.id"
            :class="['withdrawal-row', { pending: withdrawal.status === WithdrawalStatus.Pending }]"
            @click="openDetailModal(withdrawal)"
          >
            <td class="user-cell">
              <span class="user-email">{{ withdrawal.user.email }}</span>
              <span v-if="withdrawal.user.username" class="user-username">@{{ withdrawal.user.username }}</span>
            </td>
            <td class="amount-cell">
              <span class="amount">{{ formatAmount(withdrawal.amount_cents, withdrawal.currency) }}</span>
            </td>
            <td class="destination-cell">
              <span v-if="withdrawal.destination_type" class="destination-type">{{ withdrawal.destination_type }}</span>
              <span v-else class="no-destination">-</span>
            </td>
            <td>
              <span :class="['status-badge', getStatusColor(withdrawal.status)]">
                {{ getStatusLabel(withdrawal.status) }}
              </span>
            </td>
            <td class="date-cell">{{ formatDate(withdrawal.created_at) }}</td>
            <td class="reviewer-cell">
              <span v-if="withdrawal.reviewed_by">{{ withdrawal.reviewed_by.slice(0, 8) }}...</span>
              <span v-else class="no-reviewer">-</span>
            </td>
          </tr>
        </tbody>
      </table>

      <!-- Pagination -->
      <div v-if="hasMorePages" class="pagination">
        <button class="btn btn-secondary" @click="currentPage++; fetchWithdrawals()">
          {{ t('withdrawals.loadMore') }}
        </button>
      </div>
    </div>

    <!-- Detail Modal -->
    <Teleport to="body">
      <div v-if="showDetailModal" class="modal-overlay" @click.self="closeDetailModal">
        <div class="modal detail-modal">
          <div class="modal-header">
            <h2>{{ t('withdrawals.detailTitle') }}</h2>
            <button class="close-btn" @click="closeDetailModal">&times;</button>
          </div>

          <div v-if="loadingDetail" class="modal-loading">
            <div class="spinner"></div>
          </div>

          <div v-else-if="selectedWithdrawal" class="modal-body">
            <!-- User Info Section -->
            <div class="section">
              <h3>{{ t('withdrawals.userInfo') }}</h3>
              <div class="info-grid">
                <div class="info-item">
                  <label>{{ t('withdrawals.email') }}</label>
                  <span>{{ selectedWithdrawal.user.email }}</span>
                </div>
                <div v-if="selectedWithdrawal.user.full_name" class="info-item">
                  <label>{{ t('withdrawals.fullName') }}</label>
                  <span>{{ selectedWithdrawal.user.full_name }}</span>
                </div>
                <div class="info-item">
                  <label>{{ t('withdrawals.walletBalance') }}</label>
                  <span>{{ formatAmount(selectedWithdrawal.user.wallet_balance, selectedWithdrawal.currency) }}</span>
                </div>
                <div v-if="selectedWithdrawal.user.kyc_status" class="info-item">
                  <label>{{ t('withdrawals.kycStatus') }}</label>
                  <span :class="['status-badge', selectedWithdrawal.user.kyc_status === 'verified' ? 'success' : 'warning']">
                    {{ selectedWithdrawal.user.kyc_status }}
                  </span>
                </div>
              </div>
            </div>

            <!-- Withdrawal Info Section -->
            <div class="section">
              <h3>{{ t('withdrawals.withdrawalInfo') }}</h3>
              <div class="info-grid">
                <div class="info-item amount-highlight">
                  <label>{{ t('withdrawals.amount') }}</label>
                  <span class="large-amount">{{ formatAmount(selectedWithdrawal.amount_cents, selectedWithdrawal.currency) }}</span>
                </div>
                <div class="info-item">
                  <label>{{ t('withdrawals.status') }}</label>
                  <span :class="['status-badge', getStatusColor(selectedWithdrawal.status)]">
                    {{ getStatusLabel(selectedWithdrawal.status) }}
                  </span>
                </div>
                <div v-if="selectedWithdrawal.destination_type" class="info-item">
                  <label>{{ t('withdrawals.destinationType') }}</label>
                  <span>{{ selectedWithdrawal.destination_type }}</span>
                </div>
                <div class="info-item">
                  <label>{{ t('withdrawals.createdAt') }}</label>
                  <span>{{ formatDate(selectedWithdrawal.created_at) }}</span>
                </div>
                <div v-if="selectedWithdrawal.reviewer_email" class="info-item">
                  <label>{{ t('withdrawals.reviewedBy') }}</label>
                  <span>{{ selectedWithdrawal.reviewer_email }}</span>
                </div>
                <div v-if="selectedWithdrawal.reviewed_at" class="info-item">
                  <label>{{ t('withdrawals.reviewedAt') }}</label>
                  <span>{{ formatDate(selectedWithdrawal.reviewed_at) }}</span>
                </div>
              </div>
            </div>

            <!-- Destination Info -->
            <div v-if="selectedWithdrawal.destination_info && Object.keys(selectedWithdrawal.destination_info).length > 0" class="section">
              <h3>{{ t('withdrawals.destinationDetails') }}</h3>
              <div class="destination-details">
                <pre>{{ JSON.stringify(selectedWithdrawal.destination_info, null, 2) }}</pre>
              </div>
            </div>

            <!-- Admin Comment -->
            <div v-if="selectedWithdrawal.admin_comment" class="section">
              <h3>{{ t('withdrawals.adminComment') }}</h3>
              <div class="comment-box">
                {{ selectedWithdrawal.admin_comment }}
              </div>
            </div>

            <!-- Audit History -->
            <div v-if="selectedWithdrawal.audit_history.length > 0" class="section">
              <h3>{{ t('withdrawals.auditHistory') }}</h3>
              <div class="audit-list">
                <div v-for="entry in selectedWithdrawal.audit_history" :key="entry.id" class="audit-entry">
                  <div class="audit-header">
                    <span class="audit-action">{{ entry.action }}</span>
                    <span class="audit-date">{{ formatDate(entry.created_at) }}</span>
                  </div>
                  <div class="audit-actor">{{ entry.actor_email || entry.actor_id }}</div>
                </div>
              </div>
            </div>

            <!-- Action Buttons -->
            <div class="modal-actions">
              <button class="btn btn-secondary" @click="openCommentModal">
                {{ t('withdrawals.addComment') }}
              </button>
              <template v-if="canManage">
                <button
                  v-if="canApprove(selectedWithdrawal.status)"
                  class="btn btn-success"
                  @click="openApproveModal"
                >
                  {{ t('withdrawals.approve') }}
                </button>
                <button
                  v-if="canReject(selectedWithdrawal.status)"
                  class="btn btn-danger"
                  @click="openRejectModal"
                >
                  {{ t('withdrawals.reject') }}
                </button>
                <button
                  v-if="canComplete(selectedWithdrawal.status)"
                  class="btn btn-success"
                  @click="openCompleteModal"
                >
                  {{ t('withdrawals.completeBtn') }}
                </button>
                <button
                  v-if="canFail(selectedWithdrawal.status)"
                  class="btn btn-danger"
                  @click="openFailModal"
                >
                  {{ t('withdrawals.failBtn') }}
                </button>
              </template>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Approve Modal -->
    <Teleport to="body">
      <div v-if="showApproveModal" class="modal-overlay" @click.self="showApproveModal = false">
        <div class="modal action-modal">
          <div class="modal-header">
            <h2>{{ t('withdrawals.approveTitle') }}</h2>
            <button class="close-btn" @click="showApproveModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <p>{{ t('withdrawals.approveConfirmation') }}</p>
            <div class="form-group">
              <label>{{ t('withdrawals.commentOptional') }}</label>
              <textarea
                v-model="approveComment"
                :placeholder="t('withdrawals.commentPlaceholder')"
                rows="3"
              ></textarea>
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-secondary" @click="showApproveModal = false">{{ t('common.cancel') }}</button>
            <button class="btn btn-success" :disabled="actionLoading" @click="handleApprove">
              <span v-if="actionLoading" class="spinner-small"></span>
              {{ actionLoading ? t('withdrawals.approving') : t('withdrawals.approve') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Reject Modal -->
    <Teleport to="body">
      <div v-if="showRejectModal" class="modal-overlay" @click.self="showRejectModal = false">
        <div class="modal action-modal">
          <div class="modal-header">
            <h2>{{ t('withdrawals.rejectTitle') }}</h2>
            <button class="close-btn" @click="showRejectModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <p>{{ t('withdrawals.rejectConfirmation') }}</p>
            <div class="form-group">
              <label>{{ t('withdrawals.rejectReason') }} <span class="required">*</span></label>
              <textarea
                v-model="rejectReason"
                :placeholder="t('withdrawals.rejectReasonPlaceholder')"
                rows="4"
                required
              ></textarea>
            </div>
            <p class="refund-notice">{{ t('withdrawals.refundNotice') }}</p>
          </div>
          <div class="modal-footer">
            <button class="btn btn-secondary" @click="showRejectModal = false">{{ t('common.cancel') }}</button>
            <button class="btn btn-danger" :disabled="actionLoading || !rejectReason.trim()" @click="handleReject">
              <span v-if="actionLoading" class="spinner-small"></span>
              {{ actionLoading ? t('withdrawals.rejecting') : t('withdrawals.reject') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Comment Modal -->
    <Teleport to="body">
      <div v-if="showCommentModal" class="modal-overlay" @click.self="showCommentModal = false">
        <div class="modal action-modal">
          <div class="modal-header">
            <h2>{{ t('withdrawals.addCommentTitle') }}</h2>
            <button class="close-btn" @click="showCommentModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <div class="form-group">
              <label>{{ t('withdrawals.internalNote') }}</label>
              <textarea
                v-model="commentText"
                :placeholder="t('withdrawals.internalNotePlaceholder')"
                rows="4"
              ></textarea>
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-secondary" @click="showCommentModal = false">{{ t('common.cancel') }}</button>
            <button class="btn btn-primary" :disabled="actionLoading || !commentText.trim()" @click="handleComment">
              <span v-if="actionLoading" class="spinner-small"></span>
              {{ actionLoading ? t('common.saving') : t('common.save') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Complete Modal -->
    <Teleport to="body">
      <div v-if="showCompleteModal" class="modal-overlay" @click.self="showCompleteModal = false">
        <div class="modal action-modal">
          <div class="modal-header">
            <h2>{{ t('withdrawals.completeBtn') }}</h2>
            <button class="close-btn" @click="showCompleteModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <p>{{ t('withdrawals.completeConfirmation') }}</p>
            <div class="form-group">
              <label>{{ t('withdrawals.transactionId') }}</label>
              <input
                v-model="completeTransactionId"
                type="text"
                :placeholder="t('withdrawals.transactionIdPlaceholder')"
              />
            </div>
            <div class="form-group">
              <label>{{ t('withdrawals.commentOptional') }}</label>
              <textarea
                v-model="completeComment"
                :placeholder="t('withdrawals.commentPlaceholder')"
                rows="3"
              ></textarea>
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-secondary" @click="showCompleteModal = false">{{ t('common.cancel') }}</button>
            <button class="btn btn-success" :disabled="actionLoading" @click="handleComplete">
              <span v-if="actionLoading" class="spinner-small"></span>
              {{ actionLoading ? t('common.saving') : t('withdrawals.completeBtn') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Fail Modal -->
    <Teleport to="body">
      <div v-if="showFailModal" class="modal-overlay" @click.self="showFailModal = false">
        <div class="modal action-modal">
          <div class="modal-header">
            <h2>{{ t('withdrawals.failBtn') }}</h2>
            <button class="close-btn" @click="showFailModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <p>{{ t('withdrawals.failConfirmation') }}</p>
            <div class="form-group">
              <label>{{ t('withdrawals.rejectReason') }} <span class="required">*</span></label>
              <textarea
                v-model="failReason"
                :placeholder="t('withdrawals.rejectReasonPlaceholder')"
                rows="4"
                required
              ></textarea>
            </div>
            <p class="refund-notice">{{ t('withdrawals.refundNotice') }}</p>
          </div>
          <div class="modal-footer">
            <button class="btn btn-secondary" @click="showFailModal = false">{{ t('common.cancel') }}</button>
            <button class="btn btn-danger" :disabled="actionLoading || !failReason.trim()" @click="handleFail">
              <span v-if="actionLoading" class="spinner-small"></span>
              {{ actionLoading ? t('common.saving') : t('withdrawals.failBtn') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.withdrawals-page {
  padding: var(--spacing-lg);
  max-width: 1400px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: var(--spacing-lg);
}

.header-content {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0;
}

.pending-badge {
  background-color: var(--color-warning);
  color: white;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-sm);
  font-weight: 600;
}

/* Status Tabs */
.status-tabs {
  display: flex;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
  padding-bottom: var(--spacing-sm);
}

.tab-btn {
  padding: var(--spacing-sm) var(--spacing-md);
  background: none;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.tab-btn:hover {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.tab-btn.active {
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

.tab-badge {
  background-color: var(--color-warning);
  color: white;
  padding: 2px 6px;
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
}

/* Loading/Error/Empty States */
.loading-state,
.error-state,
.empty-state {
  text-align: center;
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto var(--spacing-md);
}

.spinner-small {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  display: inline-block;
  margin-right: var(--spacing-xs);
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Table */
.table-container {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

.withdrawals-table {
  width: 100%;
  border-collapse: collapse;
}

.withdrawals-table th {
  text-align: left;
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 600;
  border-bottom: 1px solid var(--color-border);
}

.withdrawals-table td {
  padding: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
  font-size: var(--font-size-sm);
}

.withdrawal-row {
  cursor: pointer;
  transition: background-color var(--transition-fast);
}

.withdrawal-row:hover {
  background-color: var(--color-bg-tertiary);
}

.withdrawal-row.pending {
  background-color: rgba(var(--color-warning-rgb, 217, 119, 6), 0.05);
}

.user-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.user-email {
  font-weight: 500;
  color: var(--color-text-primary);
}

.user-username {
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
}

.amount-cell .amount {
  font-weight: 600;
  color: var(--color-text-primary);
}

.destination-type {
  text-transform: capitalize;
}

.no-destination,
.no-reviewer {
  color: var(--color-text-secondary);
}

.date-cell {
  color: var(--color-text-secondary);
}

/* Status Badge */
.status-badge {
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: capitalize;
}

.status-badge.warning {
  background-color: var(--color-warning-light, #fef3c7);
  color: var(--color-warning, #d97706);
}

.status-badge.info {
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

.status-badge.success {
  background-color: var(--color-success-light, #dcfce7);
  color: var(--color-success, #16a34a);
}

.status-badge.error {
  background-color: var(--color-error-light, #fee2e2);
  color: var(--color-error, #dc2626);
}

.status-badge.secondary {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

/* Pagination */
.pagination {
  padding: var(--spacing-md);
  text-align: center;
}

/* Modal Styles */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: var(--spacing-lg);
}

.modal {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
  width: 100%;
  max-height: 90vh;
  overflow-y: auto;
}

.detail-modal {
  max-width: 700px;
}

.action-modal {
  max-width: 500px;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.modal-header h2 {
  margin: 0;
  font-size: var(--font-size-lg);
  font-weight: 600;
}

.close-btn {
  background: none;
  border: none;
  font-size: 24px;
  color: var(--color-text-secondary);
  cursor: pointer;
  padding: 0;
  line-height: 1;
}

.close-btn:hover {
  color: var(--color-text-primary);
}

.modal-body {
  padding: var(--spacing-lg);
}

.modal-loading {
  padding: var(--spacing-2xl);
  text-align: center;
}

.modal-footer {
  padding: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-lg);
  padding-top: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

/* Sections in Detail Modal */
.section {
  margin-bottom: var(--spacing-lg);
}

.section h3 {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-md) 0;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-md);
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.info-item label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  text-transform: uppercase;
  font-weight: 500;
}

.info-item span {
  color: var(--color-text-primary);
}

.info-item.amount-highlight .large-amount {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-primary);
}

.destination-details {
  background-color: var(--color-bg-secondary);
  padding: var(--spacing-md);
  border-radius: var(--radius-md);
  overflow-x: auto;
}

.destination-details pre {
  margin: 0;
  font-size: var(--font-size-sm);
  white-space: pre-wrap;
  word-break: break-word;
}

.comment-box {
  background-color: var(--color-bg-secondary);
  padding: var(--spacing-md);
  border-radius: var(--radius-md);
  border-left: 3px solid var(--color-primary);
}

/* Audit List */
.audit-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.audit-entry {
  padding: var(--spacing-sm);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.audit-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: var(--spacing-xs);
}

.audit-action {
  font-weight: 500;
  color: var(--color-text-primary);
}

.audit-date {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.audit-actor {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

/* Form Elements */
.form-group {
  margin-bottom: var(--spacing-md);
}

.form-group label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.form-group .required {
  color: var(--color-error);
}

.form-group textarea {
  width: 100%;
  padding: var(--spacing-sm);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  resize: vertical;
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
}

.form-group textarea:focus {
  outline: none;
  border-color: var(--color-primary);
}

.refund-notice {
  background-color: var(--color-success-light, #dcfce7);
  color: var(--color-success, #16a34a);
  padding: var(--spacing-sm);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  margin-top: var(--spacing-md);
}

/* Buttons */
.btn {
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all var(--transition-fast);
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-primary {
  background-color: var(--color-primary);
  color: white;
}

.btn-primary:hover:not(:disabled) {
  background-color: var(--color-primary-dark, #1d4ed8);
}

.btn-secondary {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.btn-secondary:hover:not(:disabled) {
  background-color: var(--color-border);
}

.btn-success {
  background-color: var(--color-success, #16a34a);
  color: white;
}

.btn-success:hover:not(:disabled) {
  background-color: #15803d;
}

.btn-danger {
  background-color: var(--color-error, #dc2626);
  color: white;
}

.btn-danger:hover:not(:disabled) {
  background-color: #b91c1c;
}

/* Responsive */
@media (max-width: 768px) {
  .info-grid {
    grid-template-columns: 1fr;
  }

  .status-tabs {
    flex-wrap: wrap;
  }

  .modal {
    margin: var(--spacing-md);
    max-height: calc(100vh - 2 * var(--spacing-md));
  }
}
</style>
