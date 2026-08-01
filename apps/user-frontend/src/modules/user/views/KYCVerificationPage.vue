<script setup lang="ts">
import { ref, computed, onUnmounted, nextTick } from 'vue';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { useCamera } from '@/composables/useCamera';
import {
  kycApi,
  type KYCPhoneVerifyResponse,
  type KYCFaceVerifyResponse,
  type KYCCardVerifyResponse,
} from '@/api';

const toast = useToast();

// Step state: 0 = phone, 1 = selfie, 2 = card, 3 = success
const currentStep = ref(0);
const loading = ref(false);
const errorMessage = ref<string | null>(null);

// Step 1: Phone verification
const nationalCode = ref('');
const mobileNumber = ref('');

// Step 2: Selfie
const selfieVideoEl = ref<HTMLVideoElement | null>(null);
const selfieCamera = useCamera({ facingMode: 'user' });
const selfieCaptured = ref<Blob | null>(null);
const selfiePreviewUrl = ref<string | null>(null);
const faceResult = ref<KYCFaceVerifyResponse | null>(null);

// Step 3: Card
const cardVideoEl = ref<HTMLVideoElement | null>(null);
const cardCamera = useCamera({ facingMode: 'environment' });
const cardCaptured = ref<Blob | null>(null);
const cardPreviewUrl = ref<string | null>(null);
const cardResult = ref<KYCCardVerifyResponse | null>(null);

const steps = computed(() => [
  { label: t('kycJibit.stepPhone') },
  { label: t('kycJibit.stepSelfie') },
  { label: t('kycJibit.stepCard') },
]);

// Validation
const isNationalCodeValid = computed(() => /^\d{10}$/.test(nationalCode.value));
const isMobileValid = computed(() => /^09\d{9}$/.test(mobileNumber.value));
const isStep1Valid = computed(() => isNationalCodeValid.value && isMobileValid.value);

// Step 1: Verify phone
async function verifyPhone(): Promise<void> {
  if (!isStep1Valid.value) return;

  loading.value = true;
  errorMessage.value = null;

  try {
    const result: KYCPhoneVerifyResponse = await kycApi.verifyPhone({
      national_code: nationalCode.value,
      mobile_number: mobileNumber.value,
    });

    if (result.matched) {
      currentStep.value = 1;
      await nextTick();
      startSelfieCamera();
    } else {
      errorMessage.value = t('kycJibit.phoneMismatch');
    }
  } catch (err: unknown) {
    const axiosErr = err as { response?: { data?: { message?: string } } };
    errorMessage.value = axiosErr.response?.data?.message || t('kycJibit.phoneVerifyError');
  } finally {
    loading.value = false;
  }
}

// Step 2: Selfie camera
async function startSelfieCamera(): Promise<void> {
  if (!selfieVideoEl.value) return;
  selfieCaptured.value = null;
  selfiePreviewUrl.value = null;
  await selfieCamera.start(selfieVideoEl.value);
  if (selfieCamera.error.value) {
    errorMessage.value = t('kycJibit.cameraAccessDenied');
  }
}

async function captureSelfie(): Promise<void> {
  if (!selfieVideoEl.value) return;

  const blob = await selfieCamera.captureFrame(selfieVideoEl.value);
  if (!blob) return;

  selfieCaptured.value = blob;
  selfiePreviewUrl.value = URL.createObjectURL(blob);
  selfieCamera.stop();
}

async function retakeSelfie(): Promise<void> {
  if (selfiePreviewUrl.value) {
    URL.revokeObjectURL(selfiePreviewUrl.value);
  }
  selfieCaptured.value = null;
  selfiePreviewUrl.value = null;
  errorMessage.value = null;
  faceResult.value = null;
  await nextTick();
  startSelfieCamera();
}

async function submitSelfie(): Promise<void> {
  if (!selfieCaptured.value) return;

  loading.value = true;
  errorMessage.value = null;

  try {
    const result = await kycApi.verifyFace(selfieCaptured.value, nationalCode.value);
    faceResult.value = result;

    if (result.matched) {
      currentStep.value = 2;
      await nextTick();
      startCardCamera();
    } else {
      errorMessage.value = t('kycJibit.faceVerifyFailed');
    }
  } catch (err: unknown) {
    const axiosErr = err as { response?: { data?: { message?: string } } };
    errorMessage.value = axiosErr.response?.data?.message || t('kycJibit.faceVerifyError');
  } finally {
    loading.value = false;
  }
}

// Step 3: Card camera
async function startCardCamera(): Promise<void> {
  if (!cardVideoEl.value) return;
  cardCaptured.value = null;
  cardPreviewUrl.value = null;
  await cardCamera.start(cardVideoEl.value);
  if (cardCamera.error.value) {
    errorMessage.value = t('kycJibit.cameraAccessDenied');
  }
}

async function captureCard(): Promise<void> {
  if (!cardVideoEl.value) return;

  const blob = await cardCamera.captureFrame(cardVideoEl.value);
  if (!blob) return;

  cardCaptured.value = blob;
  cardPreviewUrl.value = URL.createObjectURL(blob);
  cardCamera.stop();
}

async function retakeCard(): Promise<void> {
  if (cardPreviewUrl.value) {
    URL.revokeObjectURL(cardPreviewUrl.value);
  }
  cardCaptured.value = null;
  cardPreviewUrl.value = null;
  errorMessage.value = null;
  cardResult.value = null;
  await nextTick();
  startCardCamera();
}

async function submitCard(): Promise<void> {
  if (!cardCaptured.value) return;

  loading.value = true;
  errorMessage.value = null;

  try {
    const result = await kycApi.verifyCard(cardCaptured.value, nationalCode.value);
    cardResult.value = result;

    if (result.matched) {
      currentStep.value = 3;
      toast.success(t('kycJibit.verificationApproved'));
    } else {
      errorMessage.value = t('kycJibit.cardVerifyFailed');
    }
  } catch (err: unknown) {
    const axiosErr = err as { response?: { data?: { message?: string } } };
    errorMessage.value = axiosErr.response?.data?.message || t('kycJibit.cardVerifyError');
  } finally {
    loading.value = false;
  }
}

// Cleanup
onUnmounted(() => {
  selfieCamera.stop();
  cardCamera.stop();
  if (selfiePreviewUrl.value) URL.revokeObjectURL(selfiePreviewUrl.value);
  if (cardPreviewUrl.value) URL.revokeObjectURL(cardPreviewUrl.value);
});
</script>

<template>
  <div class="kyc-page">
    <div class="page-header">
      <h1 class="page-title">{{ t('kycJibit.title') }}</h1>
      <p class="page-subtitle">{{ t('kycJibit.subtitle') }}</p>
    </div>

    <!-- Progress Steps -->
    <div class="kyc-steps">
      <div
        v-for="(step, idx) in steps"
        :key="idx"
        :class="['step', { active: currentStep === idx, completed: idx < currentStep }]"
      >
        <div class="step-number">
          <svg v-if="idx < currentStep" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
            <polyline points="20 6 9 17 4 12" />
          </svg>
          <span v-else>{{ idx + 1 }}</span>
        </div>
        <span class="step-label">{{ step.label }}</span>
      </div>
      <div class="step-connector" />
    </div>

    <!-- Step 1: National Code + Phone -->
    <div v-if="currentStep === 0" class="step-content">
      <div class="step-card">
        <h2 class="step-title">{{ t('kycJibit.step1Title') }}</h2>
        <p class="step-description">{{ t('kycJibit.step1Description') }}</p>

        <div class="form-group">
          <label class="form-label">
            {{ t('kycJibit.nationalCode') }} <span class="required">*</span>
          </label>
          <input
            v-model="nationalCode"
            type="text"
            inputmode="numeric"
            class="input"
            maxlength="10"
            dir="ltr"
            :placeholder="t('kycJibit.nationalCodePlaceholder')"
            @input="errorMessage = null"
          />
          <span v-if="nationalCode && !isNationalCodeValid" class="form-error">
            {{ t('kycJibit.nationalCodeInvalid') }}
          </span>
        </div>

        <div class="form-group">
          <label class="form-label">
            {{ t('kycJibit.mobileNumber') }} <span class="required">*</span>
          </label>
          <input
            v-model="mobileNumber"
            type="tel"
            inputmode="numeric"
            class="input"
            maxlength="11"
            dir="ltr"
            placeholder="09123456789"
            @input="errorMessage = null"
          />
          <span v-if="mobileNumber && !isMobileValid" class="form-error">
            {{ t('kycJibit.mobileNumberInvalid') }}
          </span>
        </div>

        <!-- Error message -->
        <div v-if="errorMessage" class="error-banner">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="15" y1="9" x2="9" y2="15" />
            <line x1="9" y1="9" x2="15" y2="15" />
          </svg>
          <span>{{ errorMessage }}</span>
        </div>

        <button
          class="btn btn-primary btn-block"
          :disabled="!isStep1Valid || loading"
          @click="verifyPhone"
        >
          <span v-if="loading" class="btn-spinner" />
          <span v-else>{{ t('kycJibit.verifyAndContinue') }}</span>
        </button>
      </div>
    </div>

    <!-- Step 2: Selfie Camera -->
    <div v-if="currentStep === 1" class="step-content">
      <div class="step-card">
        <h2 class="step-title">{{ t('kycJibit.step2Title') }}</h2>
        <p class="step-description">{{ t('kycJibit.step2Description') }}</p>

        <!-- Camera preview -->
        <div v-if="!selfieCaptured" class="camera-container">
          <video ref="selfieVideoEl" autoplay playsinline muted class="camera-preview mirror" />
          <div class="face-guide-overlay">
            <svg viewBox="0 0 300 400" class="guide-svg">
              <defs>
                <mask id="faceMask">
                  <rect width="300" height="400" fill="white" />
                  <ellipse cx="150" cy="175" rx="95" ry="125" fill="black" />
                </mask>
              </defs>
              <rect width="300" height="400" fill="rgba(0,0,0,0.45)" mask="url(#faceMask)" />
              <ellipse cx="150" cy="175" rx="95" ry="125" fill="none" stroke="#4CAF50" stroke-width="2.5" stroke-dasharray="8 4" />
            </svg>
            <p class="guide-text">{{ t('kycJibit.faceGuide') }}</p>
          </div>

          <!-- Camera error -->
          <div v-if="selfieCamera.error.value" class="camera-error">
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
              <line x1="1" y1="1" x2="23" y2="23" />
            </svg>
            <p>{{ t('kycJibit.cameraAccessDenied') }}</p>
            <button class="btn btn-secondary" @click="startSelfieCamera">
              {{ t('common.retry') }}
            </button>
          </div>
        </div>

        <!-- Selfie preview -->
        <div v-else class="capture-preview">
          <img :src="selfiePreviewUrl!" alt="Selfie preview" class="preview-image mirror" />
        </div>

        <!-- Error message -->
        <div v-if="errorMessage" class="error-banner">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="15" y1="9" x2="9" y2="15" />
            <line x1="9" y1="9" x2="15" y2="15" />
          </svg>
          <span>{{ errorMessage }}</span>
        </div>

        <!-- Face result info -->
        <div v-if="faceResult && faceResult.matched" class="result-info success-info">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="20 6 9 17 4 12" />
          </svg>
          <span>{{ t('kycJibit.faceMatched', { score: Math.round(faceResult.liveness_score * 100) }) }}</span>
        </div>

        <div class="camera-actions">
          <template v-if="!selfieCaptured">
            <button
              class="btn btn-primary btn-block"
              :disabled="!selfieCamera.isActive.value || loading"
              @click="captureSelfie"
            >
              {{ t('kycJibit.captureSelfie') }}
            </button>
          </template>
          <template v-else>
            <button class="btn btn-secondary" @click="retakeSelfie">
              {{ t('kycJibit.retake') }}
            </button>
            <button
              class="btn btn-primary"
              :disabled="loading"
              @click="submitSelfie"
            >
              <span v-if="loading" class="btn-spinner" />
              <span v-else>{{ t('kycJibit.verifyAndContinue') }}</span>
            </button>
          </template>
        </div>
      </div>
    </div>

    <!-- Step 3: National Card Camera -->
    <div v-if="currentStep === 2" class="step-content">
      <div class="step-card">
        <h2 class="step-title">{{ t('kycJibit.step3Title') }}</h2>
        <p class="step-description">{{ t('kycJibit.step3Description') }}</p>

        <!-- Camera preview -->
        <div v-if="!cardCaptured" class="camera-container">
          <video ref="cardVideoEl" autoplay playsinline muted class="camera-preview" />
          <div class="card-guide-overlay">
            <svg viewBox="0 0 400 260" class="guide-svg">
              <defs>
                <mask id="cardMask">
                  <rect width="400" height="260" fill="white" />
                  <rect x="30" y="20" width="340" height="220" rx="12" fill="black" />
                </mask>
              </defs>
              <rect width="400" height="260" fill="rgba(0,0,0,0.45)" mask="url(#cardMask)" />
              <rect x="30" y="20" width="340" height="220" rx="12" fill="none" stroke="#4CAF50" stroke-width="2.5" stroke-dasharray="8 4" />
            </svg>
            <p class="guide-text">{{ t('kycJibit.cardGuide') }}</p>
          </div>

          <!-- Camera error -->
          <div v-if="cardCamera.error.value" class="camera-error">
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
              <line x1="1" y1="1" x2="23" y2="23" />
            </svg>
            <p>{{ t('kycJibit.cameraAccessDenied') }}</p>
            <button class="btn btn-secondary" @click="startCardCamera">
              {{ t('common.retry') }}
            </button>
          </div>
        </div>

        <!-- Card preview -->
        <div v-else class="capture-preview">
          <img :src="cardPreviewUrl!" alt="Card preview" class="preview-image" />
        </div>

        <!-- Error message -->
        <div v-if="errorMessage" class="error-banner">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="15" y1="9" x2="9" y2="15" />
            <line x1="9" y1="9" x2="15" y2="15" />
          </svg>
          <span>{{ errorMessage }}</span>
        </div>

        <!-- Card extracted info -->
        <div v-if="cardResult && cardResult.matched" class="extracted-info">
          <h4>{{ t('kycJibit.extractedInfo') }}</h4>
          <div class="info-grid">
            <div v-if="cardResult.extracted_data.first_name" class="info-item">
              <span class="info-label">{{ t('kycJibit.firstName') }}</span>
              <span class="info-value">{{ cardResult.extracted_data.first_name }}</span>
            </div>
            <div v-if="cardResult.extracted_data.last_name" class="info-item">
              <span class="info-label">{{ t('kycJibit.lastName') }}</span>
              <span class="info-value">{{ cardResult.extracted_data.last_name }}</span>
            </div>
            <div v-if="cardResult.extracted_data.national_code" class="info-item">
              <span class="info-label">{{ t('kycJibit.nationalCode') }}</span>
              <span class="info-value" dir="ltr">{{ cardResult.extracted_data.national_code }}</span>
            </div>
            <div v-if="cardResult.extracted_data.birth_date" class="info-item">
              <span class="info-label">{{ t('kycJibit.birthDate') }}</span>
              <span class="info-value" dir="ltr">{{ cardResult.extracted_data.birth_date }}</span>
            </div>
          </div>
        </div>

        <div class="camera-actions">
          <template v-if="!cardCaptured">
            <button
              class="btn btn-primary btn-block"
              :disabled="!cardCamera.isActive.value || loading"
              @click="captureCard"
            >
              {{ t('kycJibit.captureCard') }}
            </button>
          </template>
          <template v-else>
            <button class="btn btn-secondary" @click="retakeCard">
              {{ t('kycJibit.retake') }}
            </button>
            <button
              class="btn btn-primary"
              :disabled="loading"
              @click="submitCard"
            >
              <span v-if="loading" class="btn-spinner" />
              <span v-else>{{ t('kycJibit.verifyAndFinish') }}</span>
            </button>
          </template>
        </div>
      </div>
    </div>

    <!-- Step 4: Success -->
    <div v-if="currentStep === 3" class="step-content">
      <div class="step-card success-card">
        <div class="success-icon-wrapper">
          <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
            <polyline points="22 4 12 14.01 9 11.01" />
          </svg>
        </div>
        <h2 class="step-title">{{ t('kycJibit.verificationComplete') }}</h2>
        <p class="step-description">{{ t('kycJibit.verificationCompleteDesc') }}</p>

        <div class="success-benefits">
          <div class="benefit-item">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            <span>{{ t('kycJibit.benefitWithdraw') }}</span>
          </div>
          <div class="benefit-item">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            <span>{{ t('kycJibit.benefitHigherLimits') }}</span>
          </div>
          <div class="benefit-item">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            <span>{{ t('kycJibit.benefitFasterProcessing') }}</span>
          </div>
        </div>

        <router-link to="/user/wallet" class="btn btn-primary btn-block">
          {{ t('kycJibit.goToWallet') }}
        </router-link>
      </div>
    </div>
  </div>
</template>

<style scoped>
.kyc-page {
  max-width: 560px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: var(--spacing-lg);
  text-align: center;
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

/* Progress Steps */
.kyc-steps {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xl);
  margin-bottom: var(--spacing-xl);
  position: relative;
  padding: 0 var(--spacing-md);
}

.step {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
  position: relative;
  z-index: 1;
}

.step-number {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-full);
  font-weight: 600;
  font-size: var(--font-size-sm);
  background: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
  border: 2px solid var(--color-border);
  transition: all var(--transition-normal);
}

.step.active .step-number {
  background: var(--color-primary);
  color: white;
  border-color: var(--color-primary);
}

.step.completed .step-number {
  background: var(--color-success);
  color: white;
  border-color: var(--color-success);
}

.step-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  white-space: nowrap;
}

.step.active .step-label {
  color: var(--color-primary);
  font-weight: 500;
}

.step.completed .step-label {
  color: var(--color-success);
}

.step-connector {
  position: absolute;
  top: 18px;
  left: 25%;
  right: 25%;
  height: 2px;
  background: var(--color-border);
  z-index: 0;
}

/* Step Content */
.step-content {
  animation: fadeIn 0.3s ease;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

.step-card {
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-xl);
  padding: var(--spacing-xl);
}

.step-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  margin: 0 0 var(--spacing-xs) 0;
}

.step-description {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  margin: 0 0 var(--spacing-lg) 0;
  line-height: var(--line-height-relaxed);
}

/* Form */
.form-group {
  margin-bottom: var(--spacing-lg);
}

.form-label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.required {
  color: var(--color-error);
}

.form-error {
  display: block;
  font-size: var(--font-size-xs);
  color: var(--color-error);
  margin-top: var(--spacing-xs);
}

/* Error Banner */
.error-banner {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  background: #FEE2E2;
  border: 1px solid #FECACA;
  border-radius: var(--radius-md);
  color: #991B1B;
  font-size: var(--font-size-sm);
  margin-bottom: var(--spacing-lg);
}

[data-theme="dark"] .error-banner {
  background: rgba(239, 68, 68, 0.15);
  border-color: rgba(239, 68, 68, 0.3);
  color: #FCA5A5;
}

.error-banner svg {
  flex-shrink: 0;
}

/* Camera */
.camera-container {
  position: relative;
  background: #000;
  border-radius: var(--radius-lg);
  overflow: hidden;
  margin-bottom: var(--spacing-lg);
  aspect-ratio: 3 / 4;
  max-height: 400px;
}

.camera-preview {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.camera-preview.mirror {
  transform: scaleX(-1);
}

.face-guide-overlay,
.card-guide-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}

.guide-svg {
  width: 80%;
  max-width: 300px;
}

.card-guide-overlay .guide-svg {
  max-width: 400px;
  width: 90%;
}

.guide-text {
  color: white;
  font-size: var(--font-size-sm);
  text-align: center;
  margin-top: var(--spacing-sm);
  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.8);
}

.camera-error {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  background: var(--color-bg-secondary);
  color: var(--color-text-secondary);
  text-align: center;
  padding: var(--spacing-lg);
}

/* Capture Preview */
.capture-preview {
  border-radius: var(--radius-lg);
  overflow: hidden;
  margin-bottom: var(--spacing-lg);
  max-height: 400px;
}

.preview-image {
  width: 100%;
  height: auto;
  display: block;
  max-height: 400px;
  object-fit: contain;
}

.preview-image.mirror {
  transform: scaleX(-1);
}

/* Result Info */
.result-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  margin-bottom: var(--spacing-lg);
}

.success-info {
  background: #D1FAE5;
  color: #065F46;
  border: 1px solid #A7F3D0;
}

[data-theme="dark"] .success-info {
  background: rgba(16, 185, 129, 0.15);
  border-color: rgba(16, 185, 129, 0.3);
  color: #6EE7B7;
}

.result-info svg {
  flex-shrink: 0;
}

/* Extracted Info */
.extracted-info {
  background: var(--color-bg-secondary);
  border-radius: var(--radius-lg);
  padding: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.extracted-info h4 {
  font-size: var(--font-size-sm);
  font-weight: 600;
  margin: 0 0 var(--spacing-sm) 0;
}

.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--spacing-sm);
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.info-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.info-value {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

/* Camera Actions */
.camera-actions {
  display: flex;
  gap: var(--spacing-sm);
}

.camera-actions .btn {
  flex: 1;
}

/* Buttons */
.btn-block {
  width: 100%;
  justify-content: center;
}

.btn-spinner {
  width: 20px;
  height: 20px;
  border: 2px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  display: inline-block;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Success Card */
.success-card {
  text-align: center;
  border-color: var(--color-success);
}

.success-icon-wrapper {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 96px;
  height: 96px;
  border-radius: var(--radius-full);
  background: #D1FAE5;
  color: var(--color-success);
  margin-bottom: var(--spacing-lg);
}

[data-theme="dark"] .success-icon-wrapper {
  background: rgba(16, 185, 129, 0.2);
}

.success-benefits {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-xl);
  text-align: start;
}

.benefit-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  color: var(--color-success);
  font-weight: 500;
  font-size: var(--font-size-sm);
}

/* Card camera aspect ratio */
.step-content:has(.card-guide-overlay) .camera-container {
  aspect-ratio: 4 / 3;
}

/* Responsive */
@media (max-width: 767px) {
  .kyc-page {
    padding: 0;
  }

  .step-card {
    padding: var(--spacing-lg);
    border-radius: var(--radius-lg);
  }

  .kyc-steps {
    gap: var(--spacing-lg);
  }

  .step-label {
    font-size: 0.65rem;
  }

  .camera-container {
    max-height: 350px;
  }

  .camera-actions {
    flex-direction: column;
  }

  .info-grid {
    grid-template-columns: 1fr;
  }
}
</style>
