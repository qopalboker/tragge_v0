<script setup lang="ts">
import { ref, computed } from 'vue';
import { t } from '@/i18n';

interface Props {
  show: boolean;
  userName: string;
  loading?: boolean;
}

defineProps<Props>();
const emit = defineEmits<{
  close: [];
  confirm: [reason: string, rejectedFields: string[], fieldMessages: Record<string, string>];
}>();

const presetReasons = [
  'kyc.rejectReasons.documentUnreadable',
  'kyc.rejectReasons.documentExpired',
  'kyc.rejectReasons.selfieMismatch',
  'kyc.rejectReasons.incompleteInfo',
  'kyc.rejectReasons.suspectedFraud',
  'kyc.rejectReasons.other',
];

// Per-field rejection support
const rejectionFieldOptions = [
  { key: 'first_name', label: 'نام' },
  { key: 'last_name', label: 'نام خانوادگی' },
  { key: 'father_name', label: 'نام پدر' },
  { key: 'national_code', label: 'کد ملی' },
  { key: 'date_of_birth', label: 'تاریخ تولد' },
  { key: 'address', label: 'آدرس' },
  { key: 'front_image', label: 'تصویر روی مدرک' },
  { key: 'back_image', label: 'تصویر پشت مدرک' },
  { key: 'selfie_with_doc', label: 'سلفی با مدرک' },
  { key: 'document_number', label: 'شماره مدرک' },
  { key: 'birth_certificate_number', label: 'شماره شناسنامه' },
];

const selectedPreset = ref('');
const customReason = ref('');
const selectedFields = ref<Record<string, boolean>>({});
const fieldMessages = ref<Record<string, string>>({});

const isOtherSelected = computed(() => selectedPreset.value === 'kyc.rejectReasons.other');

const finalReason = computed(() => {
  if (isOtherSelected.value) {
    return customReason.value.trim();
  }
  return selectedPreset.value ? t(selectedPreset.value) : '';
});

const rejectedFieldsList = computed(() => {
  return Object.entries(selectedFields.value)
    .filter(([, v]) => v)
    .map(([k]) => k);
});

const fieldMessagesFiltered = computed(() => {
  const result: Record<string, string> = {};
  for (const field of rejectedFieldsList.value) {
    if (fieldMessages.value[field]?.trim()) {
      result[field] = fieldMessages.value[field].trim();
    }
  }
  return result;
});

const canSubmit = computed(() => {
  if (!selectedPreset.value) return false;
  if (isOtherSelected.value && !customReason.value.trim()) return false;
  return true;
});

function handleConfirm(): void {
  if (canSubmit.value) {
    emit('confirm', finalReason.value, rejectedFieldsList.value, fieldMessagesFiltered.value);
  }
}

function handleClose(): void {
  selectedPreset.value = '';
  customReason.value = '';
  emit('close');
}

function handleBackdropClick(event: MouseEvent): void {
  if ((event.target as HTMLElement).classList.contains('modal-overlay')) {
    handleClose();
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="show" class="modal-overlay" @click="handleBackdropClick">
      <div class="modal-container">
        <div class="modal-header">
          <h3 class="modal-title">{{ t('kyc.rejectTitle') }}</h3>
          <button class="close-btn" @click="handleClose" :disabled="loading">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>
        <div class="modal-body">
          <div class="confirm-icon confirm-icon-danger">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="15" y1="9" x2="9" y2="15" />
              <line x1="9" y1="9" x2="15" y2="15" />
            </svg>
          </div>
          <p class="confirm-message">
            {{ t('kyc.rejectConfirmation', { name: userName }) }}
          </p>
          <div class="form-group">
            <label class="form-label required">{{ t('kyc.rejectReason') }}</label>
            <select
              v-model="selectedPreset"
              class="input"
              :disabled="loading"
            >
              <option value="">{{ t('kyc.selectReason') }}</option>
              <option v-for="reason in presetReasons" :key="reason" :value="reason">
                {{ t(reason) }}
              </option>
            </select>
          </div>
          <div v-if="isOtherSelected" class="form-group">
            <label class="form-label required">{{ t('kyc.customReason') }}</label>
            <textarea
              v-model="customReason"
              class="input textarea"
              :placeholder="t('kyc.customReasonPlaceholder')"
              rows="3"
              :disabled="loading"
            ></textarea>
          </div>

          <!-- Per-field rejection checkboxes -->
          <div class="form-group">
            <label class="form-label">فیلدهای نیازمند اصلاح (اختیاری)</label>
            <div class="field-checkboxes">
              <label v-for="field in rejectionFieldOptions" :key="field.key" class="field-checkbox">
                <input
                  type="checkbox"
                  v-model="selectedFields[field.key]"
                  :disabled="loading"
                />
                <span>{{ field.label }}</span>
              </label>
            </div>
          </div>

          <!-- Per-field messages -->
          <template v-for="field in rejectionFieldOptions" :key="'msg-' + field.key">
            <div v-if="selectedFields[field.key]" class="form-group">
              <label class="form-label">پیام برای {{ field.label }} (اختیاری)</label>
              <input
                v-model="fieldMessages[field.key]"
                type="text"
                class="input"
                :placeholder="`دلیل رد ${field.label} را وارد کنید`"
                :disabled="loading"
              />
            </div>
          </template>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="handleClose" :disabled="loading">
            {{ t('common.cancel') }}
          </button>
          <button
            class="btn btn-danger"
            @click="handleConfirm"
            :disabled="loading || !canSubmit"
          >
            <span v-if="loading" class="btn-spinner"></span>
            {{ loading ? t('kyc.rejecting') : t('kyc.reject') }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  z-index: var(--z-modal);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-lg);
}

.modal-container {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 480px;
  box-shadow: var(--shadow-lg);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.modal-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.close-btn {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.close-btn:hover:not(:disabled) {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.close-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.close-btn svg {
  width: 18px;
  height: 18px;
}

.modal-body {
  padding: var(--spacing-lg);
}

.confirm-icon {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto var(--spacing-lg);
}

.confirm-icon svg {
  width: 32px;
  height: 32px;
}

.confirm-icon-danger {
  background-color: #FEE2E2;
  color: #DC2626;
}

.confirm-message {
  text-align: center;
  color: var(--color-text-primary);
  font-size: var(--font-size-base);
  margin-bottom: var(--spacing-lg);
}

.form-group {
  margin-top: var(--spacing-md);
}

.form-label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-xs);
}

.form-label.required::after {
  content: ' *';
  color: #DC2626;
}

.textarea {
  resize: vertical;
  min-height: 80px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
  padding: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

[dir="rtl"] .modal-footer {
  flex-direction: row-reverse;
}

.btn-danger {
  background-color: #DC2626;
  color: white;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.btn-danger:hover:not(:disabled) {
  background-color: #B91C1C;
}

.btn-danger:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-spinner {
  width: 16px;
  height: 16px;
  border: 2px solid transparent;
  border-top-color: currentColor;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.field-checkboxes {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-xs);
  margin-top: var(--spacing-xs);
}

.field-checkbox {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  cursor: pointer;
}

.field-checkbox input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
}
</style>
