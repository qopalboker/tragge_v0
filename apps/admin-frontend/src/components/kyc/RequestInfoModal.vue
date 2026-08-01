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
  confirm: [message: string];
}>();

const message = ref('');

const canSubmit = computed(() => message.value.trim().length > 0);

function handleConfirm(): void {
  if (canSubmit.value) {
    emit('confirm', message.value.trim());
  }
}

function handleClose(): void {
  message.value = '';
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
          <h3 class="modal-title">{{ t('kyc.requestInfoTitle') }}</h3>
          <button class="close-btn" @click="handleClose" :disabled="loading">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>
        <div class="modal-body">
          <div class="confirm-icon confirm-icon-info">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="16" x2="12" y2="12" />
              <line x1="12" y1="8" x2="12.01" y2="8" />
            </svg>
          </div>
          <p class="confirm-message">
            {{ t('kyc.requestInfoDescription', { name: userName }) }}
          </p>
          <div class="form-group">
            <label class="form-label required">{{ t('kyc.requestInfoMessage') }}</label>
            <textarea
              v-model="message"
              class="input textarea"
              :placeholder="t('kyc.requestInfoPlaceholder')"
              rows="4"
              :disabled="loading"
            ></textarea>
            <p class="form-hint">{{ t('kyc.requestInfoHint') }}</p>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="handleClose" :disabled="loading">
            {{ t('common.cancel') }}
          </button>
          <button
            class="btn btn-warning"
            @click="handleConfirm"
            :disabled="loading || !canSubmit"
          >
            <span v-if="loading" class="btn-spinner"></span>
            {{ loading ? t('kyc.sending') : t('kyc.sendRequest') }}
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
  max-width: 500px;
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

.confirm-icon-info {
  background-color: #DBEAFE;
  color: #2563EB;
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
  min-height: 100px;
}

.form-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin-top: var(--spacing-xs);
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

.btn-warning {
  background-color: #D97706;
  color: white;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.btn-warning:hover:not(:disabled) {
  background-color: #B45309;
}

.btn-warning:disabled {
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
</style>
