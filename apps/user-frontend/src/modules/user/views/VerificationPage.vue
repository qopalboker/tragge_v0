<script setup lang="ts">
import { ref, computed, onMounted, reactive } from 'vue';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { kycApi, type KYCStatusResponse, isFieldRejected, getFieldRejectionMessage } from '@/api';
import FileUpload from '@/components/kyc/FileUpload.vue';

const toast = useToast();

// Page state
const loading = ref(true);
const submitting = ref(false);
const error = ref<string | null>(null);
const kycStatus = ref<KYCStatusResponse | null>(null);

// Form state
const currentStep = ref(1);
const totalSteps = 2;

// Step 1: Personal Information
const personalInfo = reactive({
  firstName: '',
  lastName: '',
  fatherName: '',
  nationalCode: '',
  dateOfBirth: '',
  phone: '',
  province: '',
  city: '',
  addressLine: '',
  postalCode: '',
});

// Step 2: Documents
const documentType = ref<'national_id' | 'birth_certificate'>('national_id');
const documentNumber = ref('');
const birthCertificateNumber = ref('');
const birthCertificateSerial = ref('');
const documentFront = ref<File | null>(null);
const documentBack = ref<File | null>(null);
const selfieWithDoc = ref<File | null>(null);

// Iranian provinces
const iranProvinces = [
  'آذربایجان شرقی', 'آذربایجان غربی', 'اردبیل', 'اصفهان', 'البرز',
  'ایلام', 'بوشهر', 'تهران', 'چهارمحال و بختیاری', 'خراسان جنوبی',
  'خراسان رضوی', 'خراسان شمالی', 'خوزستان', 'زنجان', 'سمنان',
  'سیستان و بلوچستان', 'فارس', 'قزوین', 'قم', 'کردستان',
  'کرمان', 'کرمانشاه', 'کهگیلویه و بویراحمد', 'گلستان', 'گیلان',
  'لرستان', 'مازندران', 'مرکزی', 'هرمزگان', 'همدان', 'یزد',
];

// Document type options
const documentTypes = [
  { value: 'national_id', label: t('kycManual.documentTypes.national_id') },
  { value: 'birth_certificate', label: t('kycManual.documentTypes.birth_certificate') },
];

// Validation helpers
function validateNationalCode(code: string): boolean {
  if (!/^\d{10}$/.test(code)) return false;
  if (/^(\d)\1{9}$/.test(code)) return false;
  const digits = code.split('').map(Number);
  const sum = digits.slice(0, 9).reduce((acc, d, i) => acc + d * (10 - i), 0);
  const remainder = sum % 11;
  const check = digits[9];
  return remainder < 2 ? check === remainder : check === 11 - remainder;
}

const persianRegex = /^[\u0600-\u06FF\uFB50-\uFDFF\uFE70-\uFEFF\u200C\s]{2,100}$/;

function isPersianName(val: string): boolean {
  return persianRegex.test(val);
}

// National code validation state
const nationalCodeError = computed(() => {
  if (!personalInfo.nationalCode) return '';
  if (!validateNationalCode(personalInfo.nationalCode)) return t('kycManual.nationalCodeInvalid');
  return '';
});

// Computed states
const isRejected = computed(() => kycStatus.value?.status === 'rejected');
const isPartialResubmission = computed(() => {
  return isRejected.value && (kycStatus.value?.rejection_fields?.length ?? 0) > 0;
});

const showForm = computed(() => {
  return kycStatus.value?.status === 'none' || kycStatus.value?.status === 'rejected';
});

const showPending = computed(() => {
  return kycStatus.value?.status === 'pending' || kycStatus.value?.status === 'under_review';
});

const showVerified = computed(() => {
  return kycStatus.value?.status === 'verified';
});

function isFieldEditable(fieldName: string): boolean {
  if (!isPartialResubmission.value) return true;
  return isFieldRejected(kycStatus.value, fieldName);
}

function fieldRejectionMsg(fieldName: string): string | undefined {
  return getFieldRejectionMessage(kycStatus.value, fieldName);
}

const isStep1Valid = computed(() => {
  if (!personalInfo.firstName || !isPersianName(personalInfo.firstName)) return false;
  if (!personalInfo.lastName || !isPersianName(personalInfo.lastName)) return false;
  if (!personalInfo.fatherName || !isPersianName(personalInfo.fatherName)) return false;
  if (!personalInfo.nationalCode || !validateNationalCode(personalInfo.nationalCode)) return false;
  if (!personalInfo.dateOfBirth) return false;

  // Age check (18+)
  const dob = new Date(personalInfo.dateOfBirth);
  const today = new Date();
  const age = today.getFullYear() - dob.getFullYear();
  const monthDiff = today.getMonth() - dob.getMonth();
  const isOver18 = age > 18 || (age === 18 && (monthDiff > 0 || (monthDiff === 0 && today.getDate() >= dob.getDate())));
  if (!isOver18) return false;

  if (!personalInfo.city) return false;
  if (!personalInfo.addressLine) return false;

  return true;
});

const isStep2Valid = computed(() => {
  // For partial resubmission, only require files for rejected fields
  if (isPartialResubmission.value) {
    if (isFieldRejected(kycStatus.value, 'front_image') && !documentFront.value) return false;
    if (isFieldRejected(kycStatus.value, 'selfie_with_doc') && !selfieWithDoc.value) return false;
    return true;
  }
  if (!documentFront.value) return false;
  if (!selfieWithDoc.value) return false;
  return true;
});

const stepProgress = computed(() => {
  return (currentStep.value / totalSteps) * 100;
});

// Functions
async function loadKYCStatus(): Promise<void> {
  loading.value = true;
  error.value = null;

  try {
    kycStatus.value = await kycApi.getStatus();

    // Pre-populate form with existing data for rejected resubmission
    if (kycStatus.value?.status === 'rejected') {
      prefillFromStatus(kycStatus.value);
    }
  } catch (err: any) {
    error.value = err.response?.data?.error || t('kycManual.submitError');
  } finally {
    loading.value = false;
  }
}

function prefillFromStatus(status: KYCStatusResponse): void {
  if (status.first_name) personalInfo.firstName = status.first_name;
  if (status.last_name) personalInfo.lastName = status.last_name;
  if (status.father_name) personalInfo.fatherName = status.father_name;
  if (status.national_code_manual) personalInfo.nationalCode = status.national_code_manual;
  if (status.date_of_birth) personalInfo.dateOfBirth = status.date_of_birth;
  if (status.phone) personalInfo.phone = status.phone;
  if (status.province) personalInfo.province = status.province;
  if (status.city) personalInfo.city = status.city;
  if (status.address_line1) personalInfo.addressLine = status.address_line1;
  if (status.postal_code) personalInfo.postalCode = status.postal_code;
  if (status.document_type) {
    documentType.value = status.document_type as 'national_id' | 'birth_certificate';
  }
  if (status.document_number) documentNumber.value = status.document_number;
  if (status.birth_certificate_number) birthCertificateNumber.value = status.birth_certificate_number;
  if (status.birth_certificate_serial) birthCertificateSerial.value = status.birth_certificate_serial;
}

function nextStep(): void {
  if (currentStep.value < totalSteps) {
    currentStep.value++;
  }
}

function prevStep(): void {
  if (currentStep.value > 1) {
    currentStep.value--;
  }
}

function handleFileError(message: string): void {
  toast.error(message);
}

async function submitVerification(): Promise<void> {
  if (!isStep2Valid.value) return;

  submitting.value = true;

  try {
    const formData = new FormData();

    // Personal info
    formData.append('first_name', personalInfo.firstName);
    formData.append('last_name', personalInfo.lastName);
    formData.append('father_name', personalInfo.fatherName);
    formData.append('national_code_manual', personalInfo.nationalCode);
    formData.append('date_of_birth', personalInfo.dateOfBirth);
    if (personalInfo.phone) formData.append('phone', personalInfo.phone);
    if (personalInfo.province) formData.append('province', personalInfo.province);
    formData.append('city', personalInfo.city);
    formData.append('address_line_1', personalInfo.addressLine);
    if (personalInfo.postalCode) formData.append('postal_code', personalInfo.postalCode);

    // Country defaults
    formData.append('nationality', 'IR');
    formData.append('country', 'IR');

    // Document info
    formData.append('document_type', documentType.value);
    if (documentType.value === 'national_id') {
      formData.append('document_number', personalInfo.nationalCode);
    } else {
      formData.append('document_number', documentNumber.value || '');
      formData.append('birth_certificate_number', birthCertificateNumber.value);
      formData.append('birth_certificate_serial', birthCertificateSerial.value);
    }

    // Files
    if (documentFront.value) {
      formData.append('front_image', documentFront.value);
    }
    if (documentBack.value) {
      formData.append('back_image', documentBack.value);
    }
    if (selfieWithDoc.value) {
      formData.append('selfie_with_doc', selfieWithDoc.value);
    }

    await kycApi.submit(formData);
    toast.success(t('kycManual.submitSuccess'));

    // Reload status
    await loadKYCStatus();
  } catch (err: any) {
    toast.error(err.response?.data?.error || t('kycManual.submitError'));
  } finally {
    submitting.value = false;
  }
}

function formatDate(dateString: string): string {
  return new Intl.DateTimeFormat('fa-IR', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  }).format(new Date(dateString));
}

onMounted(() => {
  loadKYCStatus();
});
</script>

<template>
  <div class="verification-page" dir="rtl">
    <div class="page-header">
      <h1 class="page-title">{{ t('kycManual.title') }}</h1>
      <p class="page-subtitle">{{ t('kycManual.subtitle') }}</p>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="loading-state">
      <div class="spinner" />
      <p>{{ t('common.loading') }}</p>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="error-state">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="12" cy="12" r="10" />
        <line x1="15" y1="9" x2="9" y2="15" />
        <line x1="9" y1="9" x2="15" y2="15" />
      </svg>
      <p>{{ error }}</p>
      <button class="btn btn-primary" @click="loadKYCStatus">{{ t('common.retry') }}</button>
    </div>

    <!-- Verified State -->
    <div v-else-if="showVerified" class="status-card verified">
      <div class="status-icon">
        <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
          <polyline points="22 4 12 14.01 9 11.01" />
        </svg>
      </div>
      <h2 class="status-title">{{ t('kycManual.verifiedTitle') }}</h2>
      <p class="status-message">{{ t('kycManual.verifiedMessage') }}</p>

      <div v-if="kycStatus?.verified_at" class="status-details">
        <div class="detail-item">
          <span class="detail-label">{{ t('kycManual.verifiedTitle') }}</span>
          <span class="detail-value">{{ formatDate(kycStatus.verified_at) }}</span>
        </div>
      </div>
    </div>

    <!-- Pending State -->
    <div v-else-if="showPending" class="status-card pending">
      <div class="status-icon">
        <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <polyline points="12 6 12 12 16 14" />
        </svg>
      </div>
      <h2 class="status-title">{{ t('kycManual.pendingTitle') }}</h2>
      <p class="status-message">{{ t('kycManual.pendingMessage') }}</p>

      <div class="pending-info">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="16" x2="12" y2="12" />
          <line x1="12" y1="8" x2="12.01" y2="8" />
        </svg>
        <p>{{ t('kycManual.estimatedTime') }}</p>
      </div>

      <a href="mailto:support@tragge.com" class="support-link">
        {{ t('kycManual.contactSupport') }}
      </a>
    </div>

    <!-- Verification Form -->
    <div v-else-if="showForm" class="verification-form">
      <!-- Rejection Notice -->
      <div v-if="isRejected" class="rejection-notice">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
          <line x1="12" y1="9" x2="12" y2="13" />
          <line x1="12" y1="17" x2="12.01" y2="17" />
        </svg>
        <div class="rejection-content">
          <strong>{{ t('kycManual.rejectedTitle') }}</strong>
          <p v-if="kycStatus?.rejection_reason">{{ kycStatus.rejection_reason }}</p>
          <p v-else>{{ t('kycManual.rejectedMessage') }}</p>

          <!-- Per-field rejection messages -->
          <div v-if="kycStatus?.rejection_fields?.length" class="rejection-fields">
            <strong>{{ t('kycManual.rejectedFieldLabel') }}</strong>
            <ul>
              <li v-for="field in kycStatus.rejection_fields" :key="field">
                <span class="field-label">{{ t('kycManual.fieldRejectionMessages.' + field) || field }}</span>
                <span v-if="kycStatus.rejection_field_messages?.[field]" class="field-msg">
                  — {{ kycStatus.rejection_field_messages[field] }}
                </span>
              </li>
            </ul>
          </div>
        </div>
      </div>

      <!-- Progress Bar -->
      <div class="progress-container">
        <div class="progress-bar">
          <div class="progress-fill" :style="{ width: `${stepProgress}%` }" />
        </div>
        <div class="progress-steps">
          <button
            :class="['progress-step', { active: currentStep === 1, completed: currentStep > 1 }]"
            @click="currentStep = 1"
          >
            <span class="step-number">1</span>
            <span class="step-label">{{ t('kycManual.step1Title') }}</span>
          </button>
          <button
            :class="['progress-step', { active: currentStep === 2, completed: currentStep > 2 }]"
            :disabled="!isStep1Valid"
            @click="isStep1Valid && (currentStep = 2)"
          >
            <span class="step-number">2</span>
            <span class="step-label">{{ t('kycManual.step2Title') }}</span>
          </button>
        </div>
      </div>

      <!-- Step 1: Personal Information -->
      <div v-if="currentStep === 1" class="form-step">
        <h2 class="step-title">{{ t('kycManual.step1Title') }}</h2>
        <p class="step-description">{{ t('kycManual.step1Description') }}</p>

        <div class="form-grid">
          <!-- First Name -->
          <div :class="['form-group', { 'field-rejected': isFieldRejected(kycStatus, 'first_name') }]">
            <label class="form-label">
              {{ t('kycManual.firstName') }} <span class="required">*</span>
            </label>
            <input
              v-model="personalInfo.firstName"
              type="text"
              class="input"
              :placeholder="t('kycManual.firstNamePlaceholder')"
              :disabled="!isFieldEditable('first_name')"
            />
            <span v-if="fieldRejectionMsg('first_name')" class="field-error">{{ fieldRejectionMsg('first_name') }}</span>
          </div>

          <!-- Last Name -->
          <div :class="['form-group', { 'field-rejected': isFieldRejected(kycStatus, 'last_name') }]">
            <label class="form-label">
              {{ t('kycManual.lastName') }} <span class="required">*</span>
            </label>
            <input
              v-model="personalInfo.lastName"
              type="text"
              class="input"
              :placeholder="t('kycManual.lastNamePlaceholder')"
              :disabled="!isFieldEditable('last_name')"
            />
            <span v-if="fieldRejectionMsg('last_name')" class="field-error">{{ fieldRejectionMsg('last_name') }}</span>
          </div>

          <!-- Father Name -->
          <div :class="['form-group', { 'field-rejected': isFieldRejected(kycStatus, 'father_name') }]">
            <label class="form-label">
              {{ t('kycManual.fatherName') }} <span class="required">*</span>
            </label>
            <input
              v-model="personalInfo.fatherName"
              type="text"
              class="input"
              :placeholder="t('kycManual.fatherNamePlaceholder')"
              :disabled="!isFieldEditable('father_name')"
            />
            <span v-if="fieldRejectionMsg('father_name')" class="field-error">{{ fieldRejectionMsg('father_name') }}</span>
          </div>

          <!-- National Code -->
          <div :class="['form-group', { 'field-rejected': isFieldRejected(kycStatus, 'national_code') }]">
            <label class="form-label">
              {{ t('kycManual.nationalCode') }} <span class="required">*</span>
            </label>
            <input
              v-model="personalInfo.nationalCode"
              type="text"
              class="input"
              dir="ltr"
              maxlength="10"
              :placeholder="t('kycManual.nationalCodePlaceholder')"
              :disabled="!isFieldEditable('national_code')"
            />
            <span v-if="nationalCodeError" class="field-error">{{ nationalCodeError }}</span>
            <span v-if="fieldRejectionMsg('national_code')" class="field-error">{{ fieldRejectionMsg('national_code') }}</span>
          </div>

          <!-- Date of Birth -->
          <div :class="['form-group', { 'field-rejected': isFieldRejected(kycStatus, 'date_of_birth') }]">
            <label class="form-label">
              {{ t('kycManual.dateOfBirth') }} <span class="required">*</span>
            </label>
            <input
              v-model="personalInfo.dateOfBirth"
              type="date"
              class="input"
              dir="ltr"
              :max="new Date(new Date().setFullYear(new Date().getFullYear() - 18)).toISOString().split('T')[0]"
              :disabled="!isFieldEditable('date_of_birth')"
            />
            <span class="form-hint">{{ t('kycManual.mustBe18') }}</span>
            <span v-if="fieldRejectionMsg('date_of_birth')" class="field-error">{{ fieldRejectionMsg('date_of_birth') }}</span>
          </div>

          <!-- Phone -->
          <div class="form-group">
            <label class="form-label">{{ t('kycManual.phone') }}</label>
            <input
              v-model="personalInfo.phone"
              type="tel"
              class="input"
              dir="ltr"
              maxlength="11"
              :placeholder="t('kycManual.phonePlaceholder')"
            />
          </div>
        </div>

        <div class="form-divider">
          <span>{{ t('kycManual.addressSection') }}</span>
        </div>

        <div class="form-grid">
          <!-- Province -->
          <div class="form-group">
            <label class="form-label">{{ t('kycManual.province') }}</label>
            <select v-model="personalInfo.province" class="input">
              <option value="">{{ t('kycManual.provincePlaceholder') }}</option>
              <option v-for="p in iranProvinces" :key="p" :value="p">{{ p }}</option>
            </select>
          </div>

          <!-- City -->
          <div :class="['form-group', { 'field-rejected': isFieldRejected(kycStatus, 'address') }]">
            <label class="form-label">
              {{ t('kycManual.city') }} <span class="required">*</span>
            </label>
            <input
              v-model="personalInfo.city"
              type="text"
              class="input"
              :placeholder="t('kycManual.cityPlaceholder')"
              :disabled="!isFieldEditable('address')"
            />
          </div>

          <!-- Address -->
          <div :class="['form-group', 'full-width', { 'field-rejected': isFieldRejected(kycStatus, 'address') }]">
            <label class="form-label">
              {{ t('kycManual.addressLine') }} <span class="required">*</span>
            </label>
            <input
              v-model="personalInfo.addressLine"
              type="text"
              class="input"
              :placeholder="t('kycManual.addressPlaceholder')"
              :disabled="!isFieldEditable('address')"
            />
            <span v-if="fieldRejectionMsg('address')" class="field-error">{{ fieldRejectionMsg('address') }}</span>
          </div>

          <!-- Postal Code -->
          <div class="form-group">
            <label class="form-label">{{ t('kycManual.postalCode') }}</label>
            <input
              v-model="personalInfo.postalCode"
              type="text"
              class="input"
              dir="ltr"
              maxlength="10"
              :placeholder="t('kycManual.postalCodePlaceholder')"
            />
          </div>
        </div>

        <div class="form-actions">
          <button
            class="btn btn-primary"
            :disabled="!isStep1Valid"
            @click="nextStep"
          >
            {{ t('kycManual.next') }}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="15 18 9 12 15 6" />
            </svg>
          </button>
        </div>
      </div>

      <!-- Step 2: Document Upload -->
      <div v-if="currentStep === 2" class="form-step">
        <h2 class="step-title">{{ t('kycManual.step2Title') }}</h2>
        <p class="step-description">{{ t('kycManual.step2Description') }}</p>

        <div class="form-grid">
          <!-- Document Type -->
          <div class="form-group">
            <label class="form-label">
              {{ t('kycManual.documentType') }} <span class="required">*</span>
            </label>
            <select v-model="documentType" class="input">
              <option v-for="dtype in documentTypes" :key="dtype.value" :value="dtype.value">
                {{ dtype.label }}
              </option>
            </select>
          </div>

          <!-- Document Number (auto-filled for national_id) -->
          <div v-if="documentType === 'national_id'" class="form-group">
            <label class="form-label">{{ t('kycManual.documentNumber') }}</label>
            <input
              :value="personalInfo.nationalCode"
              type="text"
              class="input"
              dir="ltr"
              disabled
            />
          </div>

          <!-- Birth Certificate fields -->
          <template v-if="documentType === 'birth_certificate'">
            <div class="form-group">
              <label class="form-label">{{ t('kycManual.birthCertificateNumber') }}</label>
              <input
                v-model="birthCertificateNumber"
                type="text"
                class="input"
                dir="ltr"
                maxlength="10"
              />
            </div>
            <div class="form-group">
              <label class="form-label">{{ t('kycManual.birthCertificateSerial') }}</label>
              <input
                v-model="birthCertificateSerial"
                type="text"
                class="input"
                dir="ltr"
                maxlength="30"
              />
            </div>
          </template>
        </div>

        <!-- Image Requirements Notice -->
        <div class="requirements-notice">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="12" y1="16" x2="12" y2="12" />
            <line x1="12" y1="8" x2="12.01" y2="8" />
          </svg>
          <div>
            <strong>{{ t('kycManual.imageRequirements') }}</strong>
            <ul>
              <li>{{ t('kycManual.requirementClear') }}</li>
              <li>{{ t('kycManual.requirementCorners') }}</li>
              <li>{{ t('kycManual.requirementNoGlare') }}</li>
              <li>{{ t('kycManual.requirementColor') }}</li>
              <li>{{ t('kycManual.requirementSelfieDoc') }}</li>
            </ul>
          </div>
        </div>

        <div class="upload-grid">
          <!-- Document Front -->
          <div :class="{ 'field-rejected': isFieldRejected(kycStatus, 'front_image') }">
            <FileUpload
              :label="t('kycManual.uploadFront')"
              :hint="t('kycManual.uploadFrontHint')"
              :required="true"
              @update:file="documentFront = $event"
              @error="handleFileError"
            />
            <span v-if="fieldRejectionMsg('front_image')" class="field-error">{{ fieldRejectionMsg('front_image') }}</span>
          </div>

          <!-- Document Back -->
          <div :class="{ 'field-rejected': isFieldRejected(kycStatus, 'back_image') }">
            <FileUpload
              :label="t('kycManual.uploadBack')"
              :hint="t('kycManual.uploadBackHint')"
              :required="documentType === 'national_id'"
              @update:file="documentBack = $event"
              @error="handleFileError"
            />
            <span v-if="fieldRejectionMsg('back_image')" class="field-error">{{ fieldRejectionMsg('back_image') }}</span>
          </div>

          <!-- Selfie with Document -->
          <div :class="{ 'field-rejected': isFieldRejected(kycStatus, 'selfie_with_doc') }">
            <FileUpload
              :label="t('kycManual.uploadSelfieWithDoc')"
              :hint="t('kycManual.uploadSelfieWithDocHint')"
              :required="true"
              :show-camera="true"
              @update:file="selfieWithDoc = $event"
              @error="handleFileError"
            />
            <span v-if="fieldRejectionMsg('selfie_with_doc')" class="field-error">{{ fieldRejectionMsg('selfie_with_doc') }}</span>
          </div>
        </div>

        <p class="file-size-hint">{{ t('kycManual.imageRequirements') }} — {{ t('kycManual.uploadFrontHint') ? '' : '' }} (max 10MB)</p>

        <div class="form-actions">
          <button class="btn btn-secondary" @click="prevStep">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="9 18 15 12 9 6" />
            </svg>
            {{ t('kycManual.back') }}
          </button>
          <button
            class="btn btn-primary"
            :disabled="!isStep2Valid || submitting"
            @click="submitVerification"
          >
            <span v-if="submitting" class="btn-spinner" />
            <span v-else>
              {{ isRejected ? t('kycManual.editAndResubmit') : t('kycManual.submitForReview') }}
            </span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.verification-page {
  max-width: 800px;
  margin: 0 auto;
  direction: rtl;
}

.page-header {
  margin-bottom: var(--spacing-xl);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  margin: 0 0 var(--spacing-xs) 0;
}

.page-subtitle {
  font-size: var(--font-size-md);
  color: var(--color-text-secondary);
  margin: 0;
}

/* Loading & Error States */
.loading-state,
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-3xl);
  gap: var(--spacing-md);
  text-align: center;
}

.spinner {
  width: 48px;
  height: 48px;
  border: 4px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.error-state svg {
  color: var(--color-error);
}

/* Status Cards */
.status-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-xl);
  padding: var(--spacing-2xl);
  text-align: center;
}

.status-card.verified { border-color: var(--color-success); }
.status-card.pending { border-color: var(--color-warning); }

.status-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 96px;
  height: 96px;
  border-radius: var(--radius-full);
  margin-bottom: var(--spacing-lg);
}

.verified .status-icon { background-color: #D1FAE5; color: var(--color-success); }
.pending .status-icon { background-color: #FEF3C7; color: var(--color-warning); }

.status-title {
  font-size: var(--font-size-xl);
  font-weight: 600;
  margin: 0 0 var(--spacing-sm) 0;
}

.status-message {
  color: var(--color-text-secondary);
  margin: 0 0 var(--spacing-lg) 0;
}

.status-details {
  display: flex;
  justify-content: center;
  gap: var(--spacing-xl);
  padding: var(--spacing-lg);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.detail-item { display: flex; flex-direction: column; gap: var(--spacing-xs); }
.detail-label { font-size: var(--font-size-sm); color: var(--color-text-secondary); }
.detail-value { font-weight: 500; }

.pending-info {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  background-color: #FEF3C7;
  border-radius: var(--radius-md);
  color: #92400E;
  margin-bottom: var(--spacing-lg);
}

.pending-info p { margin: 0; font-size: var(--font-size-sm); }

.support-link {
  color: var(--color-primary);
  font-weight: 500;
  text-decoration: none;
}

.support-link:hover { text-decoration: underline; }

/* Rejection Notice */
.rejection-notice {
  display: flex;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  background-color: #FEE2E2;
  border: 1px solid var(--color-error);
  border-radius: var(--radius-lg);
  margin-bottom: var(--spacing-xl);
}

.rejection-notice svg { flex-shrink: 0; color: var(--color-error); }
.rejection-content { color: #991B1B; }
.rejection-content strong { display: block; margin-bottom: var(--spacing-xs); }
.rejection-content p { margin: 0; font-size: var(--font-size-sm); }

.rejection-fields { margin-top: var(--spacing-sm); }
.rejection-fields ul { margin: var(--spacing-xs) 0 0; padding-right: var(--spacing-lg); list-style: disc; }
.rejection-fields li { margin-bottom: var(--spacing-xs); font-size: var(--font-size-sm); }
.field-label { font-weight: 600; }
.field-msg { color: #7F1D1D; }

/* Progress */
.progress-container { margin-bottom: var(--spacing-xl); }

.progress-bar {
  height: 4px;
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-full);
  overflow: hidden;
  margin-bottom: var(--spacing-lg);
}

.progress-fill {
  height: 100%;
  background-color: var(--color-primary);
  transition: width var(--transition-normal);
}

.progress-steps { display: flex; justify-content: space-between; }

.progress-step {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border: none;
  background: none;
  cursor: pointer;
  border-radius: var(--radius-md);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.progress-step:disabled { cursor: not-allowed; opacity: 0.5; }
.progress-step.active { color: var(--color-primary); font-weight: 600; }
.progress-step.completed { color: var(--color-success); }

.step-number {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: var(--radius-full);
  border: 2px solid currentColor;
  font-weight: 600;
  font-size: var(--font-size-sm);
}

.progress-step.active .step-number { background-color: var(--color-primary); color: white; border-color: var(--color-primary); }
.progress-step.completed .step-number { background-color: var(--color-success); color: white; border-color: var(--color-success); }

/* Form */
.form-step { animation: fadeIn 0.3s ease; }
@keyframes fadeIn { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); } }

.step-title { font-size: var(--font-size-lg); font-weight: 600; margin: 0 0 var(--spacing-xs) 0; }
.step-description { color: var(--color-text-secondary); margin: 0 0 var(--spacing-lg) 0; }

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-md);
}

@media (max-width: 640px) {
  .form-grid { grid-template-columns: 1fr; }
}

.form-group { display: flex; flex-direction: column; gap: var(--spacing-xs); }
.form-group.full-width { grid-column: 1 / -1; }

.form-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.required { color: var(--color-error); }

.input {
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-md);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  transition: border-color var(--transition-fast);
}

.input:focus { outline: none; border-color: var(--color-primary); box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1); }
.input:disabled { background-color: var(--color-bg-secondary); opacity: 0.7; cursor: not-allowed; }

.form-hint { font-size: var(--font-size-xs); color: var(--color-text-muted); }

.field-error {
  font-size: var(--font-size-xs);
  color: var(--color-error);
  font-weight: 500;
}

.field-rejected .input,
.field-rejected :deep(.file-upload-area) {
  border-color: var(--color-error) !important;
  box-shadow: 0 0 0 2px rgba(239, 68, 68, 0.15);
}

.form-divider {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  margin: var(--spacing-lg) 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.form-divider::before,
.form-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background-color: var(--color-border);
}

/* Upload Grid */
.upload-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--spacing-lg);
  margin: var(--spacing-lg) 0;
}

.file-size-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin: 0 0 var(--spacing-md) 0;
}

/* Requirements Notice */
.requirements-notice {
  display: flex;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  background-color: #EFF6FF;
  border: 1px solid #BFDBFE;
  border-radius: var(--radius-md);
  margin: var(--spacing-lg) 0;
}

.requirements-notice svg { flex-shrink: 0; color: #3B82F6; }
.requirements-notice strong { display: block; margin-bottom: var(--spacing-xs); font-size: var(--font-size-sm); }
.requirements-notice ul { margin: 0; padding-right: var(--spacing-lg); list-style: disc; }
.requirements-notice li { font-size: var(--font-size-sm); color: var(--color-text-secondary); margin-bottom: var(--spacing-xs); }

/* Form Actions */
.form-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: var(--spacing-xl);
  gap: var(--spacing-md);
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-lg);
  border: none;
  border-radius: var(--radius-md);
  font-size: var(--font-size-md);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.btn:disabled { opacity: 0.5; cursor: not-allowed; }

.btn-primary {
  background-color: var(--color-primary);
  color: white;
}

.btn-primary:hover:not(:disabled) { opacity: 0.9; }

.btn-secondary {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border);
}

.btn-secondary:hover:not(:disabled) { background-color: var(--color-bg-tertiary, #e5e7eb); }

.btn-spinner {
  width: 18px;
  height: 18px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}
</style>
