<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { t } from '@/i18n';
import { accountApi } from '@/api';
import { useToast } from '@/composables/useToast';

const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void;
  (e: 'success'): void;
}>();

const toast = useToast();

// Form state
const currentPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const showCurrentPassword = ref(false);
const showNewPassword = ref(false);
const showConfirmPassword = ref(false);
const loading = ref(false);
const error = ref<string | null>(null);

// Reset form when modal closes
watch(() => props.show, (isOpen) => {
  if (!isOpen) {
    currentPassword.value = '';
    newPassword.value = '';
    confirmPassword.value = '';
    showCurrentPassword.value = false;
    showNewPassword.value = false;
    showConfirmPassword.value = false;
    error.value = null;
  }
});

// Validation
const passwordsMatch = computed(() => {
  return confirmPassword.value.length === 0 || newPassword.value === confirmPassword.value;
});

const passwordMinLength = computed(() => {
  return newPassword.value.length === 0 || newPassword.value.length >= 10;
});

const passwordDifferent = computed(() => {
  return newPassword.value.length === 0 || currentPassword.value !== newPassword.value;
});

const isValid = computed(() => {
  return (
    currentPassword.value.length > 0 &&
    newPassword.value.length >= 10 &&
    newPassword.value === confirmPassword.value &&
    currentPassword.value !== newPassword.value
  );
});

function closeModal() {
  emit('update:show', false);
}

async function handleSubmit() {
  if (!isValid.value || loading.value) return;

  loading.value = true;
  error.value = null;

  try {
    await accountApi.changePassword({
      current_password: currentPassword.value,
      new_password: newPassword.value,
      confirm_password: confirmPassword.value,
    });

    toast.success(t('profile.passwordChangedSuccess'));
    emit('success');
    closeModal();
  } catch (err: any) {
    const errorMessage = err.response?.data?.error || t('common.error');
    // Map backend error messages to i18n keys
    if (errorMessage === 'current password is incorrect') {
      error.value = t('profile.currentPasswordIncorrect');
    } else if (errorMessage.includes('too many')) {
      error.value = t('profile.tooManyPasswordAttempts');
    } else {
      error.value = errorMessage;
    }
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="show" class="modal-overlay" @click.self="closeModal">
        <div class="modal-content">
          <div class="modal-header">
            <h3>{{ t('profile.changePassword') }}</h3>
            <button class="close-btn" @click="closeModal" :disabled="loading">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          <form class="modal-body" @submit.prevent="handleSubmit">
            <!-- Error message -->
            <div v-if="error" class="error-message">
              {{ error }}
            </div>

            <!-- Current Password -->
            <div class="form-group">
              <label class="form-label" for="current-password">
                {{ t('profile.currentPassword') }}
              </label>
              <div class="password-input-wrapper">
                <input
                  id="current-password"
                  v-model="currentPassword"
                  :type="showCurrentPassword ? 'text' : 'password'"
                  class="input"
                  :placeholder="t('profile.currentPasswordPlaceholder')"
                  :disabled="loading"
                  autocomplete="current-password"
                />
                <button
                  type="button"
                  class="password-toggle"
                  @click="showCurrentPassword = !showCurrentPassword"
                >
                  <svg v-if="showCurrentPassword" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
                    <line x1="1" y1="1" x2="23" y2="23" />
                  </svg>
                  <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                    <circle cx="12" cy="12" r="3" />
                  </svg>
                </button>
              </div>
            </div>

            <!-- New Password -->
            <div class="form-group">
              <label class="form-label" for="new-password">
                {{ t('profile.newPassword') }}
              </label>
              <div class="password-input-wrapper">
                <input
                  id="new-password"
                  v-model="newPassword"
                  :type="showNewPassword ? 'text' : 'password'"
                  class="input"
                  :class="{ 'input-error': !passwordMinLength || !passwordDifferent }"
                  :placeholder="t('profile.newPasswordPlaceholder')"
                  :disabled="loading"
                  autocomplete="new-password"
                />
                <button
                  type="button"
                  class="password-toggle"
                  @click="showNewPassword = !showNewPassword"
                >
                  <svg v-if="showNewPassword" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
                    <line x1="1" y1="1" x2="23" y2="23" />
                  </svg>
                  <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                    <circle cx="12" cy="12" r="3" />
                  </svg>
                </button>
              </div>
              <p v-if="!passwordMinLength" class="field-error">
                {{ t('profile.passwordMinLength') }}
              </p>
              <p v-else-if="!passwordDifferent" class="field-error">
                {{ t('profile.passwordMustBeDifferent') }}
              </p>
            </div>

            <!-- Confirm Password -->
            <div class="form-group">
              <label class="form-label" for="confirm-password">
                {{ t('profile.confirmNewPassword') }}
              </label>
              <div class="password-input-wrapper">
                <input
                  id="confirm-password"
                  v-model="confirmPassword"
                  :type="showConfirmPassword ? 'text' : 'password'"
                  class="input"
                  :class="{ 'input-error': !passwordsMatch }"
                  :placeholder="t('profile.confirmNewPasswordPlaceholder')"
                  :disabled="loading"
                  autocomplete="new-password"
                />
                <button
                  type="button"
                  class="password-toggle"
                  @click="showConfirmPassword = !showConfirmPassword"
                >
                  <svg v-if="showConfirmPassword" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
                    <line x1="1" y1="1" x2="23" y2="23" />
                  </svg>
                  <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                    <circle cx="12" cy="12" r="3" />
                  </svg>
                </button>
              </div>
              <p v-if="!passwordsMatch" class="field-error">
                {{ t('profile.passwordsDoNotMatch') }}
              </p>
            </div>

            <!-- Info text -->
            <p class="info-text">
              {{ t('profile.passwordChangeInfo') }}
            </p>

            <div class="modal-actions">
              <button
                type="button"
                class="btn btn-secondary"
                @click="closeModal"
                :disabled="loading"
              >
                {{ t('common.cancel') }}
              </button>
              <button
                type="submit"
                class="btn btn-primary"
                :disabled="!isValid || loading"
              >
                <span v-if="loading" class="spinner-small"></span>
                {{ loading ? t('common.loading') : t('profile.changePassword') }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: var(--z-modal);
  padding: var(--spacing-md);
}

.modal-content {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  width: 100%;
  max-width: 400px;
  max-height: 90vh;
  overflow-y: auto;
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.modal-header h3 {
  font-size: var(--font-size-lg);
  font-weight: 600;
  margin: 0;
}

.close-btn {
  background: none;
  border: none;
  color: var(--color-text-secondary);
  cursor: pointer;
  padding: var(--spacing-xs);
  border-radius: var(--radius-sm);
  transition: color var(--transition-fast);
}

.close-btn:hover {
  color: var(--color-text-primary);
}

.close-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.modal-body {
  padding: var(--spacing-lg);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.form-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.password-input-wrapper {
  position: relative;
}

.password-input-wrapper .input {
  width: 100%;
  padding-right: 44px;
}

[dir="rtl"] .password-input-wrapper .input {
  padding-right: var(--spacing-md);
  padding-left: 44px;
}

.password-toggle {
  position: absolute;
  right: var(--spacing-sm);
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: var(--color-text-secondary);
  cursor: pointer;
  padding: var(--spacing-xs);
}

[dir="rtl"] .password-toggle {
  right: auto;
  left: var(--spacing-sm);
}

.password-toggle:hover {
  color: var(--color-text-primary);
}

.input {
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.input:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.input:disabled {
  background-color: var(--color-bg-secondary);
  cursor: not-allowed;
}

.input-error {
  border-color: var(--color-error);
}

.input-error:focus {
  box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.1);
}

.field-error {
  font-size: var(--font-size-xs);
  color: var(--color-error);
  margin: 0;
}

.error-message {
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: #FEE2E2;
  color: #DC2626;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
}

.dark .error-message {
  background-color: rgba(220, 38, 38, 0.2);
  color: #FCA5A5;
}

.info-text {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  margin: 0;
}

.modal-actions {
  display: flex;
  gap: var(--spacing-sm);
  justify-content: flex-end;
  margin-top: var(--spacing-sm);
}

.btn {
  padding: var(--spacing-sm) var(--spacing-lg);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
  border: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background-color: var(--color-primary);
  color: white;
}

.btn-primary:hover:not(:disabled) {
  background-color: var(--color-primary-hover);
}

.btn-secondary {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

.btn-secondary:hover:not(:disabled) {
  background-color: var(--color-bg-tertiary);
}

.spinner-small {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Modal transition */
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active .modal-content,
.modal-leave-active .modal-content {
  transition: transform 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .modal-content,
.modal-leave-to .modal-content {
  transform: scale(0.95);
}

@media (max-width: 480px) {
  .modal-actions {
    flex-direction: column-reverse;
  }

  .btn {
    width: 100%;
  }
}
</style>
