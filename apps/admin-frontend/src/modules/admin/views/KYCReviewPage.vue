<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import {
  getKYCSubmissions,
  getKYCSubmission,
  approveKYC,
  rejectKYC,
  requestMoreInfo,
  bulkAutoApproveKYC,
  getDocumentTypeLabel,
  KYCStatus,
  DocumentType,
  type KYCSubmission,
  type KYCAuditEntry,
} from '@/api/kyc';
import ImageViewer from '@/components/kyc/ImageViewer.vue';
import ApproveModal from '@/components/kyc/ApproveModal.vue';
import RejectModal from '@/components/kyc/RejectModal.vue';
import RequestInfoModal from '@/components/kyc/RequestInfoModal.vue';

const toast = useToast();

// State
const submissions = ref<KYCSubmission[]>([]);
const selectedSubmission = ref<KYCSubmission | null>(null);
const auditHistory = ref<KYCAuditEntry[]>([]);
const previousSubmissions = ref<KYCSubmission[]>([]);
const loading = ref(true);
const loadingDetail = ref(false);
const error = ref<string | null>(null);
const statusFilter = ref<KYCStatus | ''>('');
const currentPage = ref(1);
const totalSubmissions = ref(0);
const perPage = 20;

// Modal states
const showImageViewer = ref(false);
const viewerImage = ref({ url: '', title: '' });
const showApproveModal = ref(false);
const showRejectModal = ref(false);
const showRequestInfoModal = ref(false);
const actionLoading = ref(false);

// Computed
const pendingCount = computed(() => {
  return submissions.value.filter(
    s => s.status === KYCStatus.Pending || s.status === KYCStatus.UnderReview
  ).length;
});

const filteredSubmissions = computed(() => {
  if (!statusFilter.value) {
    return submissions.value.filter(
      s => s.status === KYCStatus.Pending || s.status === KYCStatus.UnderReview
    );
  }
  return submissions.value.filter(s => s.status === statusFilter.value);
});

const hasMorePages = computed(() => {
  return currentPage.value * perPage < totalSubmissions.value;
});

const bulkApproveLoading = ref(false);

const autoApprovableCount = computed(() => {
  return submissions.value.filter(
    s => s.auto_approved && s.status !== KYCStatus.Approved
  ).length;
});

function hasJibitData(submission: KYCSubmission): boolean {
  return submission.shahkar_verified !== undefined;
}

function getFailedStep(submission: KYCSubmission): string | null {
  if (!hasJibitData(submission)) return null;
  if (!submission.shahkar_verified) return 'shahkar';
  if (!submission.face_verified) return 'face';
  if (!submission.card_ocr_verified) return 'card';
  return null;
}

function formatScore(score: number | undefined): string {
  if (score === undefined || score === null) return '0%';
  return `${Math.round(score * 100)}%`;
}

async function handleBulkAutoApprove(): Promise<void> {
  bulkApproveLoading.value = true;
  try {
    const result = await bulkAutoApproveKYC();
    toast.success(t('kyc.bulkApproveSuccess', { count: String(result.approved) }));
    await fetchSubmissions();
    selectedSubmission.value = null;
  } catch {
    toast.error(t('kyc.bulkApproveError'));
  } finally {
    bulkApproveLoading.value = false;
  }
}

// Methods
async function fetchSubmissions(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    const response = await getKYCSubmissions(
      statusFilter.value || undefined,
      currentPage.value,
      perPage
    );
    submissions.value = response.submissions || [];
    totalSubmissions.value = response.total;
  } catch {
    error.value = t('common.error');
    if (import.meta.env.DEV) {
      // Mock data for development only
      submissions.value = [
        {
          id: 'kyc-1',
          user_id: 'user-1',
          user_email: 'john.doe@example.com',
          full_name: 'John Doe',
          date_of_birth: '1990-05-15',
          nationality: 'United States',
          address: '123 Main St, New York, NY 10001',
          document: {
            id: 'doc-1',
            type: DocumentType.Passport,
            document_number: 'AB1234567',
            front_image_url: 'https://placehold.co/600x400?text=ID+Front',
            back_image_url: 'https://placehold.co/600x400?text=ID+Back',
            selfie_image_url: 'https://placehold.co/400x400?text=Selfie',
          },
          status: KYCStatus.Pending,
          submitted_at: '2026-01-20T10:30:00Z',
        },
        {
          id: 'kyc-2',
          user_id: 'user-2',
          user_email: 'jane.smith@example.com',
          full_name: 'Jane Smith',
          date_of_birth: '1985-08-22',
          nationality: 'Canada',
          address: '456 Oak Ave, Toronto, ON M5V 2H1',
          document: {
            id: 'doc-2',
            type: DocumentType.NationalId,
            document_number: 'NID987654321',
            front_image_url: 'https://placehold.co/600x400?text=ID+Front',
            selfie_image_url: 'https://placehold.co/400x400?text=Selfie',
          },
          status: KYCStatus.UnderReview,
          submitted_at: '2026-01-19T14:45:00Z',
        },
        {
          id: 'kyc-3',
          user_id: 'user-3',
          user_email: 'bob.wilson@example.com',
          full_name: 'Bob Wilson',
          date_of_birth: '1978-12-03',
          nationality: 'United Kingdom',
          address: '789 High Street, London, UK SW1A 1AA',
          document: {
            id: 'doc-3',
            type: DocumentType.DriversLicense,
            document_number: 'WILSO812035AB1CD',
            front_image_url: 'https://placehold.co/600x400?text=License+Front',
            back_image_url: 'https://placehold.co/600x400?text=License+Back',
            selfie_image_url: 'https://placehold.co/400x400?text=Selfie',
          },
          status: KYCStatus.Pending,
          submitted_at: '2026-01-18T09:15:00Z',
        },
      ];
      totalSubmissions.value = 3;
      error.value = null;
    }
  } finally {
    loading.value = false;
  }
}

async function selectSubmission(submission: KYCSubmission): Promise<void> {
  selectedSubmission.value = submission;
  loadingDetail.value = true;

  try {
    const response = await getKYCSubmission(submission.id);
    selectedSubmission.value = response.submission;
    auditHistory.value = response.audit_history || [];
    previousSubmissions.value = response.previous_submissions || [];
  } catch {
    if (import.meta.env.DEV) {
      // Mock audit data for development only
      auditHistory.value = [
        {
          id: 'audit-1',
          submission_id: submission.id,
          action: 'kyc.submitted',
          performed_by: submission.user_email,
          performed_at: submission.submitted_at,
        },
      ];
      previousSubmissions.value = [];
    }
  } finally {
    loadingDetail.value = false;
  }
}

function openImageViewer(url: string, title: string): void {
  viewerImage.value = { url, title };
  showImageViewer.value = true;
}

function getStatusClass(status: KYCStatus): string {
  const classes: Record<KYCStatus, string> = {
    [KYCStatus.Pending]: 'status-pending',
    [KYCStatus.UnderReview]: 'status-review',
    [KYCStatus.Approved]: 'status-approved',
    [KYCStatus.Rejected]: 'status-rejected',
    [KYCStatus.MoreInfoRequired]: 'status-info',
  };
  return classes[status] || 'status-default';
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString();
}

function formatDateTime(dateString: string): string {
  const date = new Date(dateString);
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString()}`;
}

function formatDOB(dateString: string): string {
  const date = new Date(dateString);
  const age = Math.floor(
    (Date.now() - date.getTime()) / (365.25 * 24 * 60 * 60 * 1000)
  );
  return `${date.toLocaleDateString()} (${age} ${t('kyc.yearsOld')})`;
}

async function handleApprove(notes: string): Promise<void> {
  if (!selectedSubmission.value) return;

  actionLoading.value = true;
  try {
    await approveKYC(selectedSubmission.value.id, { notes });
    showApproveModal.value = false;
    toast.success(t('kyc.approveSuccess'));
    await fetchSubmissions();
    selectedSubmission.value = null;
  } catch {
    toast.error(t('kyc.approveError'));
  } finally {
    actionLoading.value = false;
  }
}

async function handleReject(reason: string, rejectedFields?: string[], fieldMessages?: Record<string, string>): Promise<void> {
  if (!selectedSubmission.value) return;

  actionLoading.value = true;
  try {
    await rejectKYC(selectedSubmission.value.id, {
      reason,
      rejected_fields: rejectedFields,
      field_messages: fieldMessages,
    });
    showRejectModal.value = false;
    toast.success(t('kyc.rejectSuccess'));
    await fetchSubmissions();
    selectedSubmission.value = null;
  } catch {
    toast.error(t('kyc.rejectError'));
  } finally {
    actionLoading.value = false;
  }
}

async function handleRequestInfo(message: string): Promise<void> {
  if (!selectedSubmission.value) return;

  actionLoading.value = true;
  try {
    await requestMoreInfo(selectedSubmission.value.id, { message });
    showRequestInfoModal.value = false;
    toast.success(t('kyc.requestInfoSuccess'));
    await fetchSubmissions();
    selectedSubmission.value = null;
  } catch {
    toast.error(t('kyc.requestInfoError'));
  } finally {
    actionLoading.value = false;
  }
}

function loadMore(): void {
  currentPage.value++;
  fetchSubmissions();
}

watch(statusFilter, () => {
  currentPage.value = 1;
  fetchSubmissions();
});

onMounted(fetchSubmissions);
</script>

<template>
  <div class="kyc-review-page">
    <div class="page-header">
      <div class="page-header-left">
        <h1 class="page-title">{{ t('kyc.title') }}</h1>
        <span class="pending-count">{{ pendingCount }} {{ t('kyc.pendingReviews') }}</span>
      </div>
      <button
        v-if="autoApprovableCount > 0"
        class="btn btn-success bulk-approve-btn"
        :disabled="bulkApproveLoading"
        @click="handleBulkAutoApprove"
      >
        {{ bulkApproveLoading ? t('kyc.bulkApproving') : t('kyc.bulkAutoApprove', { count: String(autoApprovableCount) }) }}
      </button>
    </div>

    <div class="kyc-layout">
      <!-- Left Panel: Submissions List -->
      <div class="submissions-panel">
        <div class="panel-header">
          <select v-model="statusFilter" class="input status-filter">
            <option value="">{{ t('kyc.pendingAndReview') }}</option>
            <option :value="KYCStatus.Pending">{{ t('kyc.status.pending') }}</option>
            <option :value="KYCStatus.UnderReview">{{ t('kyc.status.under_review') }}</option>
            <option :value="KYCStatus.Approved">{{ t('kyc.status.approved') }}</option>
            <option :value="KYCStatus.Rejected">{{ t('kyc.status.rejected') }}</option>
          </select>
        </div>

        <div v-if="loading" class="loading">
          {{ t('common.loading') }}
        </div>

        <div v-else-if="filteredSubmissions.length === 0" class="no-results">
          {{ t('kyc.noSubmissions') }}
        </div>

        <div v-else class="submissions-list">
          <div
            v-for="submission in filteredSubmissions"
            :key="submission.id"
            :class="['submission-card', { selected: selectedSubmission?.id === submission.id }]"
            @click="selectSubmission(submission)"
          >
            <div class="submission-header">
              <span class="submission-name">{{ submission.full_name }}</span>
              <div class="submission-badges">
                <span v-if="submission.auto_approved" class="status-badge status-auto-approved">
                  {{ t('kyc.autoApproved') }}
                </span>
                <span :class="['status-badge', getStatusClass(submission.status)]">
                  {{ t(`kyc.status.${submission.status}`) }}
                </span>
              </div>
            </div>
            <div class="submission-email">{{ submission.user_email }}</div>
            <div v-if="hasJibitData(submission)" class="submission-verification-steps">
              <span :class="['step-indicator', submission.shahkar_verified ? 'step-pass' : 'step-fail']">
                {{ submission.shahkar_verified ? '\u2713' : '\u2717' }} {{ t('kyc.jibit.shahkar') }}
              </span>
              <span :class="['step-indicator', submission.face_verified ? 'step-pass' : 'step-fail']">
                {{ submission.face_verified ? '\u2713' : '\u2717' }} {{ t('kyc.jibit.face') }}
              </span>
              <span :class="['step-indicator', submission.card_ocr_verified ? 'step-pass' : 'step-fail']">
                {{ submission.card_ocr_verified ? '\u2713' : '\u2717' }} {{ t('kyc.jibit.card') }}
              </span>
            </div>
            <div class="submission-meta">
              <span class="doc-type">{{ submission.document ? getDocumentTypeLabel(submission.document.type) : '' }}</span>
              <span class="submit-date">{{ formatDate(submission.submitted_at) }}</span>
            </div>
          </div>

          <button
            v-if="hasMorePages"
            class="btn btn-ghost load-more-btn"
            @click="loadMore"
          >
            {{ t('kyc.loadMore') }}
          </button>
        </div>
      </div>

      <!-- Right Panel: Submission Details -->
      <div class="detail-panel">
        <div v-if="!selectedSubmission" class="empty-state">
          <div class="empty-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="3" y="4" width="18" height="16" rx="2" />
              <circle cx="9" cy="10" r="2" />
              <path d="M15 8h2" />
              <path d="M15 12h2" />
              <path d="M7 16h10" />
            </svg>
          </div>
          <p>{{ t('kyc.selectSubmission') }}</p>
        </div>

        <div v-else-if="loadingDetail" class="loading">
          {{ t('common.loading') }}
        </div>

        <div v-else class="detail-content">
          <!-- User Info Section -->
          <section class="detail-section">
            <h3 class="section-title">{{ t('kyc.userInfo') }}</h3>
            <div class="info-grid">
              <div class="info-item">
                <label>{{ t('kyc.fullName') }}</label>
                <span>{{ selectedSubmission.full_name }}</span>
              </div>
              <div class="info-item">
                <label>{{ t('kyc.dateOfBirth') }}</label>
                <span>{{ formatDOB(selectedSubmission.date_of_birth) }}</span>
              </div>
              <div class="info-item">
                <label>{{ t('kyc.nationality') }}</label>
                <span>{{ selectedSubmission.nationality }}</span>
              </div>
              <div class="info-item full-width">
                <label>{{ t('kyc.address') }}</label>
                <span>{{ selectedSubmission.address }}</span>
              </div>
              <div class="info-item">
                <label>{{ t('kyc.email') }}</label>
                <a :href="`mailto:${selectedSubmission.user_email}`" class="email-link">
                  {{ selectedSubmission.user_email }}
                </a>
              </div>
              <div v-if="selectedSubmission.father_name" class="info-item">
                <label>نام پدر</label>
                <span>{{ selectedSubmission.father_name }}</span>
              </div>
              <div v-if="selectedSubmission.national_code_manual" class="info-item">
                <label>کد ملی</label>
                <span class="mono">{{ selectedSubmission.national_code_manual }}</span>
              </div>
              <div v-if="selectedSubmission.province" class="info-item">
                <label>استان</label>
                <span>{{ selectedSubmission.province }}</span>
              </div>
            </div>
          </section>

          <!-- Jibit Verification Results -->
          <section v-if="hasJibitData(selectedSubmission)" class="detail-section">
            <h3 class="section-title">{{ t('kyc.jibit.title') }}</h3>

            <div v-if="selectedSubmission.auto_approved" class="auto-approved-banner">
              {{ t('kyc.jibit.autoApprovedBanner') }}
            </div>

            <div v-if="getFailedStep(selectedSubmission)" class="failed-step-banner">
              {{ t('kyc.jibit.failedAt', { step: t(`kyc.jibit.${getFailedStep(selectedSubmission)}`) }) }}
            </div>

            <!-- Verification Steps -->
            <div class="verification-steps">
              <div class="verification-step">
                <div class="step-header">
                  <span :class="['step-icon', selectedSubmission.shahkar_verified ? 'step-pass' : 'step-fail']">
                    {{ selectedSubmission.shahkar_verified ? '\u2713' : '\u2717' }}
                  </span>
                  <span class="step-name">{{ t('kyc.jibit.shahkarVerification') }}</span>
                </div>
                <div class="step-description">{{ t('kyc.jibit.shahkarDesc') }}</div>
                <div v-if="selectedSubmission.national_code" class="step-detail">
                  {{ t('kyc.jibit.nationalCode') }}: <span class="mono">{{ selectedSubmission.national_code }}</span>
                </div>
              </div>

              <div class="verification-step">
                <div class="step-header">
                  <span :class="['step-icon', selectedSubmission.face_verified ? 'step-pass' : 'step-fail']">
                    {{ selectedSubmission.face_verified ? '\u2713' : '\u2717' }}
                  </span>
                  <span class="step-name">{{ t('kyc.jibit.faceVerification') }}</span>
                </div>
                <div class="step-description">{{ t('kyc.jibit.faceDesc') }}</div>
                <div v-if="selectedSubmission.face_match_score !== undefined" class="score-bar-container">
                  <label>{{ t('kyc.jibit.faceMatchScore') }}</label>
                  <div class="score-bar">
                    <div
                      class="score-bar-fill"
                      :class="{ 'score-high': (selectedSubmission.face_match_score ?? 0) > 0.85, 'score-low': (selectedSubmission.face_match_score ?? 0) <= 0.85 }"
                      :style="{ width: formatScore(selectedSubmission.face_match_score) }"
                    ></div>
                  </div>
                  <span class="score-value">{{ formatScore(selectedSubmission.face_match_score) }}</span>
                </div>
                <div v-if="selectedSubmission.liveness_score !== undefined" class="score-bar-container">
                  <label>{{ t('kyc.jibit.livenessScore') }}</label>
                  <div class="score-bar">
                    <div
                      class="score-bar-fill"
                      :class="{ 'score-high': (selectedSubmission.liveness_score ?? 0) > 0.85, 'score-low': (selectedSubmission.liveness_score ?? 0) <= 0.85 }"
                      :style="{ width: formatScore(selectedSubmission.liveness_score) }"
                    ></div>
                  </div>
                  <span class="score-value">{{ formatScore(selectedSubmission.liveness_score) }}</span>
                </div>
                <div v-if="selectedSubmission.liveness_result" class="step-detail">
                  {{ t('kyc.jibit.livenessResult') }}:
                  <span :class="['liveness-badge', selectedSubmission.liveness_result === 'LIVE' ? 'liveness-live' : 'liveness-fake']">
                    {{ selectedSubmission.liveness_result }}
                  </span>
                </div>
              </div>

              <div class="verification-step">
                <div class="step-header">
                  <span :class="['step-icon', selectedSubmission.card_ocr_verified ? 'step-pass' : 'step-fail']">
                    {{ selectedSubmission.card_ocr_verified ? '\u2713' : '\u2717' }}
                  </span>
                  <span class="step-name">{{ t('kyc.jibit.cardVerification') }}</span>
                </div>
                <div class="step-description">{{ t('kyc.jibit.cardDesc') }}</div>
              </div>
            </div>
          </section>

          <!-- Documents Section -->
          <section class="detail-section">
            <h3 class="section-title">{{ t('kyc.documents') }}</h3>
            <div class="doc-info">
              <span class="doc-label">{{ t('kyc.documentType') }}:</span>
              <span>{{ getDocumentTypeLabel(selectedSubmission.document.type) }}</span>
              <span class="doc-label">{{ t('kyc.documentNumber') }}:</span>
              <span class="doc-number">{{ selectedSubmission.document.document_number }}</span>
            </div>
            <div class="documents-grid">
              <div class="document-card">
                <div
                  class="document-image"
                  @click="openImageViewer(selectedSubmission.document.front_image_url, t('kyc.idFront'))"
                >
                  <img :src="selectedSubmission.document.front_image_url" :alt="t('kyc.idFront')" />
                  <div class="image-overlay">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="11" cy="11" r="8" />
                      <line x1="21" y1="21" x2="16.65" y2="16.65" />
                      <line x1="11" y1="8" x2="11" y2="14" />
                      <line x1="8" y1="11" x2="14" y2="11" />
                    </svg>
                  </div>
                </div>
                <span class="document-label">{{ t('kyc.idFront') }}</span>
              </div>

              <div v-if="selectedSubmission.document.back_image_url" class="document-card">
                <div
                  class="document-image"
                  @click="openImageViewer(selectedSubmission.document.back_image_url!, t('kyc.idBack'))"
                >
                  <img :src="selectedSubmission.document.back_image_url" :alt="t('kyc.idBack')" />
                  <div class="image-overlay">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="11" cy="11" r="8" />
                      <line x1="21" y1="21" x2="16.65" y2="16.65" />
                      <line x1="11" y1="8" x2="11" y2="14" />
                      <line x1="8" y1="11" x2="14" y2="11" />
                    </svg>
                  </div>
                </div>
                <span class="document-label">{{ t('kyc.idBack') }}</span>
              </div>

              <div class="document-card">
                <div
                  class="document-image selfie"
                  @click="openImageViewer(selectedSubmission.document.selfie_image_url, t('kyc.selfie'))"
                >
                  <img :src="selectedSubmission.document.selfie_image_url" :alt="t('kyc.selfie')" />
                  <div class="image-overlay">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="11" cy="11" r="8" />
                      <line x1="21" y1="21" x2="16.65" y2="16.65" />
                      <line x1="11" y1="8" x2="11" y2="14" />
                      <line x1="8" y1="11" x2="14" y2="11" />
                    </svg>
                  </div>
                </div>
                <span class="document-label">{{ t('kyc.selfie') }}</span>
              </div>

              <div v-if="selectedSubmission.document.selfie_with_doc_url" class="document-card">
                <div
                  class="document-image selfie"
                  @click="openImageViewer(selectedSubmission.document.selfie_with_doc_url!, 'سلفی با مدرک')"
                >
                  <img :src="selectedSubmission.document.selfie_with_doc_url" alt="سلفی با مدرک" />
                  <div class="image-overlay">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="11" cy="11" r="8" />
                      <line x1="21" y1="21" x2="16.65" y2="16.65" />
                      <line x1="11" y1="8" x2="11" y2="14" />
                      <line x1="8" y1="11" x2="14" y2="11" />
                    </svg>
                  </div>
                </div>
                <span class="document-label">سلفی با مدرک</span>
              </div>
            </div>
          </section>

          <!-- Audit History Section -->
          <section class="detail-section">
            <h3 class="section-title">{{ t('kyc.auditHistory') }}</h3>
            <div v-if="auditHistory.length === 0" class="no-history">
              {{ t('kyc.noHistory') }}
            </div>
            <div v-else class="audit-timeline">
              <div v-for="entry in auditHistory" :key="entry.id" class="timeline-item">
                <div class="timeline-dot"></div>
                <div class="timeline-content">
                  <span class="timeline-action">{{ t(entry.action) }}</span>
                  <span class="timeline-meta">
                    {{ entry.performed_by }} &bull; {{ formatDateTime(entry.performed_at) }}
                  </span>
                  <p v-if="entry.details" class="timeline-details">{{ entry.details }}</p>
                </div>
              </div>
            </div>
          </section>

          <!-- Previous Submissions Section -->
          <section v-if="previousSubmissions.length > 0" class="detail-section">
            <h3 class="section-title">{{ t('kyc.previousSubmissions') }}</h3>
            <div class="previous-list">
              <div
                v-for="prev in previousSubmissions"
                :key="prev.id"
                class="previous-item"
              >
                <span :class="['status-badge', getStatusClass(prev.status)]">
                  {{ t(`kyc.status.${prev.status}`) }}
                </span>
                <span class="previous-date">{{ formatDateTime(prev.submitted_at) }}</span>
                <span v-if="prev.rejection_reason" class="previous-reason">
                  {{ prev.rejection_reason }}
                </span>
              </div>
            </div>
          </section>

          <!-- Action Buttons -->
          <div class="action-buttons">
            <button
              class="btn btn-success"
              @click="showApproveModal = true"
              :disabled="selectedSubmission.status === KYCStatus.Approved"
            >
              {{ t('kyc.approve') }}
            </button>
            <button
              class="btn btn-danger"
              @click="showRejectModal = true"
              :disabled="selectedSubmission.status === KYCStatus.Rejected"
            >
              {{ t('kyc.reject') }}
            </button>
            <button
              class="btn btn-warning"
              @click="showRequestInfoModal = true"
            >
              {{ t('kyc.requestMoreInfo') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Modals -->
    <ImageViewer
      :show="showImageViewer"
      :image-url="viewerImage.url"
      :title="viewerImage.title"
      @close="showImageViewer = false"
    />

    <ApproveModal
      :show="showApproveModal"
      :user-name="selectedSubmission?.full_name || ''"
      :loading="actionLoading"
      @close="showApproveModal = false"
      @confirm="handleApprove"
    />

    <RejectModal
      :show="showRejectModal"
      :user-name="selectedSubmission?.full_name || ''"
      :loading="actionLoading"
      @close="showRejectModal = false"
      @confirm="handleReject"
    />

    <RequestInfoModal
      :show="showRequestInfoModal"
      :user-name="selectedSubmission?.full_name || ''"
      :loading="actionLoading"
      @close="showRequestInfoModal = false"
      @confirm="handleRequestInfo"
    />
  </div>
</template>

<style scoped>
.kyc-review-page {
  padding: var(--spacing-lg) 0;
  height: calc(100vh - var(--header-height) - var(--spacing-lg) * 2);
  display: flex;
  flex-direction: column;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-lg);
}

.page-header-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.bulk-approve-btn {
  white-space: nowrap;
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

.pending-count {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  background-color: var(--color-bg-tertiary);
  padding: var(--spacing-xs) var(--spacing-md);
  border-radius: var(--radius-full);
}

.kyc-layout {
  display: grid;
  grid-template-columns: 380px 1fr;
  gap: var(--spacing-lg);
  flex: 1;
  min-height: 0;
}

/* Submissions Panel */
.submissions-panel {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.panel-header {
  padding: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
}

.status-filter {
  width: 100%;
}

.submissions-list {
  flex: 1;
  overflow-y: auto;
  padding: var(--spacing-sm);
}

.submission-card {
  padding: var(--spacing-md);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
  margin-bottom: var(--spacing-xs);
}

.submission-card:hover {
  background-color: var(--color-bg-secondary);
}

.submission-card.selected {
  background-color: var(--color-primary-light);
  border: 1px solid var(--color-primary);
}

.submission-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xs);
}

.submission-name {
  font-weight: 600;
  color: var(--color-text-primary);
}

.submission-badges {
  display: flex;
  gap: var(--spacing-xs);
  align-items: center;
}

.submission-email {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-xs);
}

.submission-verification-steps {
  display: flex;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-xs);
  flex-wrap: wrap;
}

.step-indicator {
  font-size: var(--font-size-xs);
  font-weight: 500;
  padding: 1px var(--spacing-xs);
  border-radius: var(--radius-sm);
}

.step-pass {
  color: #16A34A;
  background-color: #DCFCE7;
}

.step-fail {
  color: #DC2626;
  background-color: #FEE2E2;
}

.submission-meta {
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.load-more-btn {
  width: 100%;
  margin-top: var(--spacing-sm);
}

/* Detail Panel */
.detail-panel {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow-y: auto;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--color-text-muted);
}

.empty-icon {
  width: 80px;
  height: 80px;
  margin-bottom: var(--spacing-md);
  color: var(--color-border);
}

.empty-icon svg {
  width: 100%;
  height: 100%;
}

.loading,
.no-results {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.detail-content {
  padding: var(--spacing-lg);
}

.detail-section {
  margin-bottom: var(--spacing-xl);
}

.section-title {
  font-size: var(--font-size-base);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-md);
  padding-bottom: var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
}

/* Info Grid */
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

.info-item.full-width {
  grid-column: 1 / -1;
}

.info-item label {
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-muted);
  text-transform: uppercase;
}

.info-item span,
.info-item a {
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.email-link {
  color: var(--color-primary);
  text-decoration: none;
}

.email-link:hover {
  text-decoration: underline;
}

/* Documents */
.doc-info {
  display: flex;
  gap: var(--spacing-sm);
  align-items: center;
  margin-bottom: var(--spacing-md);
  flex-wrap: wrap;
}

.doc-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.doc-number {
  font-family: var(--font-family-mono);
  background-color: var(--color-bg-tertiary);
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-sm);
}

.documents-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--spacing-md);
}

.document-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.document-image {
  position: relative;
  aspect-ratio: 3/2;
  border-radius: var(--radius-md);
  overflow: hidden;
  cursor: pointer;
  border: 1px solid var(--color-border);
}

.document-image.selfie {
  aspect-ratio: 1;
}

.document-image img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.image-overlay {
  position: absolute;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity var(--transition-fast);
}

.document-image:hover .image-overlay {
  opacity: 1;
}

.image-overlay svg {
  width: 32px;
  height: 32px;
  color: white;
}

.document-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  text-align: center;
}

/* Audit Timeline */
.audit-timeline {
  position: relative;
  padding-left: var(--spacing-lg);
}

[dir="rtl"] .audit-timeline {
  padding-left: 0;
  padding-right: var(--spacing-lg);
}

.timeline-item {
  position: relative;
  padding-bottom: var(--spacing-md);
}

.timeline-item:last-child {
  padding-bottom: 0;
}

.timeline-item::before {
  content: '';
  position: absolute;
  left: calc(var(--spacing-lg) * -1 + 4px);
  top: 10px;
  bottom: 0;
  width: 2px;
  background-color: var(--color-border);
}

[dir="rtl"] .timeline-item::before {
  left: auto;
  right: calc(var(--spacing-lg) * -1 + 4px);
}

.timeline-item:last-child::before {
  display: none;
}

.timeline-dot {
  position: absolute;
  left: calc(var(--spacing-lg) * -1);
  top: 6px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background-color: var(--color-primary);
}

[dir="rtl"] .timeline-dot {
  left: auto;
  right: calc(var(--spacing-lg) * -1);
}

.timeline-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.timeline-action {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.timeline-meta {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.timeline-details {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-top: var(--spacing-xs);
}

.no-history {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  font-style: italic;
}

/* Previous Submissions */
.previous-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.previous-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  flex-wrap: wrap;
}

.previous-date {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.previous-reason {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  font-style: italic;
  width: 100%;
}

/* Status Badges */
.status-badge {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
}

.status-pending {
  background-color: #FEF3C7;
  color: #D97706;
}

.status-review {
  background-color: #DBEAFE;
  color: #2563EB;
}

.status-approved {
  background-color: #DCFCE7;
  color: #16A34A;
}

.status-rejected {
  background-color: #FEE2E2;
  color: #DC2626;
}

.status-info {
  background-color: #E0E7FF;
  color: #4F46E5;
}

.status-auto-approved {
  background-color: #D1FAE5;
  color: #065F46;
  border: 1px solid #6EE7B7;
}

/* Jibit Verification Section */
.auto-approved-banner {
  background-color: #D1FAE5;
  color: #065F46;
  border: 1px solid #6EE7B7;
  border-radius: var(--radius-md);
  padding: var(--spacing-sm) var(--spacing-md);
  margin-bottom: var(--spacing-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.failed-step-banner {
  background-color: #FEE2E2;
  color: #991B1B;
  border: 1px solid #FCA5A5;
  border-radius: var(--radius-md);
  padding: var(--spacing-sm) var(--spacing-md);
  margin-bottom: var(--spacing-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.verification-steps {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.verification-step {
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
}

.step-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-xs);
}

.step-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  font-size: var(--font-size-sm);
  font-weight: 700;
}

.step-icon.step-pass {
  background-color: #16A34A;
  color: white;
}

.step-icon.step-fail {
  background-color: #DC2626;
  color: white;
}

.step-name {
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.step-description {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin-bottom: var(--spacing-sm);
}

.step-detail {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-top: var(--spacing-xs);
}

.mono {
  font-family: var(--font-family-mono);
  background-color: var(--color-bg-tertiary);
  padding: 1px var(--spacing-xs);
  border-radius: var(--radius-sm);
}

/* Score Bars */
.score-bar-container {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-xs);
}

.score-bar-container label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  min-width: 100px;
  flex-shrink: 0;
}

.score-bar {
  flex: 1;
  height: 8px;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-full);
  overflow: hidden;
}

.score-bar-fill {
  height: 100%;
  border-radius: var(--radius-full);
  transition: width 0.3s ease;
}

.score-bar-fill.score-high {
  background-color: #16A34A;
}

.score-bar-fill.score-low {
  background-color: #D97706;
}

.score-value {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-primary);
  min-width: 36px;
  text-align: right;
}

/* Liveness Badge */
.liveness-badge {
  display: inline-block;
  padding: 1px var(--spacing-sm);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
}

.liveness-live {
  background-color: #DCFCE7;
  color: #16A34A;
}

.liveness-fake {
  background-color: #FEE2E2;
  color: #DC2626;
}

/* Action Buttons */
.action-buttons {
  display: flex;
  gap: var(--spacing-md);
  padding-top: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

.btn-success {
  background-color: #16A34A;
  color: white;
}

.btn-success:hover:not(:disabled) {
  background-color: #15803D;
}

.btn-danger {
  background-color: #DC2626;
  color: white;
}

.btn-danger:hover:not(:disabled) {
  background-color: #B91C1C;
}

.btn-warning {
  background-color: #D97706;
  color: white;
}

.btn-warning:hover:not(:disabled) {
  background-color: #B45309;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Responsive */
@media (max-width: 1024px) {
  .kyc-layout {
    grid-template-columns: 1fr;
    grid-template-rows: auto 1fr;
  }

  .submissions-panel {
    max-height: 300px;
  }
}

@media (max-width: 767px) {
  .page-header {
    flex-direction: column;
    gap: var(--spacing-md);
    align-items: stretch;
  }

  .info-grid {
    grid-template-columns: 1fr;
  }

  .documents-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .action-buttons {
    flex-direction: column;
  }
}
</style>
