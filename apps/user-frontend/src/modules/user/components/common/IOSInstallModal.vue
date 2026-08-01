<script setup lang="ts">
import { watch } from 'vue';
import { t } from '@/i18n';
import { useMobileOptimizations } from '@/composables/useMobileOptimizations';

const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'dismiss'): void;
}>();

const { lockBodyScroll } = useMobileOptimizations();

// Lock body scroll when modal is open
watch(
  () => props.show,
  (isOpen) => {
    if (isOpen) {
      const unlock = lockBodyScroll();
      // Store unlock function for later (will be called on close)
      (window as unknown as { __iosModalUnlock?: () => void }).__iosModalUnlock = unlock;
    } else {
      const unlock = (window as unknown as { __iosModalUnlock?: () => void }).__iosModalUnlock;
      if (unlock) {
        unlock();
        delete (window as unknown as { __iosModalUnlock?: () => void }).__iosModalUnlock;
      }
    }
  }
);

// Handle backdrop click
const handleBackdropClick = (event: MouseEvent) => {
  if (event.target === event.currentTarget) {
    emit('close');
  }
};

// Handle close
const handleClose = () => {
  emit('close');
};

// Handle dismiss (don't show again for 7 days)
const handleDismiss = () => {
  emit('dismiss');
};
</script>

<template>
  <Teleport to="body">
    <Transition name="modal-fade">
      <div
        v-if="show"
        class="ios-modal-overlay"
        @click="handleBackdropClick"
        role="dialog"
        aria-modal="true"
        :aria-label="t('pwa.install.ios.title')"
      >
        <div class="ios-modal">
          <!-- Header -->
          <div class="ios-modal__header">
            <div class="ios-modal__icon">
              <img
                src="/icons/icon-72x72.png"
                alt="Tragge"
                width="56"
                height="56"
              />
            </div>
            <h2 class="ios-modal__title">{{ t('pwa.install.ios.title') }}</h2>
            <p class="ios-modal__subtitle">{{ t('pwa.install.ios.subtitle') }}</p>
            <button
              class="ios-modal__close"
              @click="handleClose"
              :aria-label="t('common.close')"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>

          <!-- Steps -->
          <div class="ios-modal__steps">
            <!-- Step 1: Tap Share -->
            <div class="ios-modal__step">
              <div class="ios-modal__step-number">1</div>
              <div class="ios-modal__step-content">
                <p class="ios-modal__step-title">{{ t('pwa.install.ios.step1Title') }}</p>
                <p class="ios-modal__step-desc">{{ t('pwa.install.ios.step1Desc') }}</p>
                <div class="ios-modal__step-visual">
                  <!-- Share Icon (iOS style) -->
                  <svg
                    xmlns="http://www.w3.org/2000/svg"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    class="ios-modal__share-icon"
                  >
                    <path d="M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8" />
                    <polyline points="16 6 12 2 8 6" />
                    <line x1="12" y1="2" x2="12" y2="15" />
                  </svg>
                </div>
              </div>
            </div>

            <!-- Step 2: Scroll and tap Add to Home Screen -->
            <div class="ios-modal__step">
              <div class="ios-modal__step-number">2</div>
              <div class="ios-modal__step-content">
                <p class="ios-modal__step-title">{{ t('pwa.install.ios.step2Title') }}</p>
                <p class="ios-modal__step-desc">{{ t('pwa.install.ios.step2Desc') }}</p>
                <div class="ios-modal__step-visual ios-modal__step-visual--row">
                  <!-- Add to Home Screen Icon -->
                  <div class="ios-modal__menu-item">
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                    >
                      <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
                      <line x1="12" y1="8" x2="12" y2="16" />
                      <line x1="8" y1="12" x2="16" y2="12" />
                    </svg>
                    <span>{{ t('pwa.install.ios.addToHomeScreen') }}</span>
                  </div>
                </div>
              </div>
            </div>

            <!-- Step 3: Tap Add -->
            <div class="ios-modal__step">
              <div class="ios-modal__step-number">3</div>
              <div class="ios-modal__step-content">
                <p class="ios-modal__step-title">{{ t('pwa.install.ios.step3Title') }}</p>
                <p class="ios-modal__step-desc">{{ t('pwa.install.ios.step3Desc') }}</p>
                <div class="ios-modal__step-visual">
                  <div class="ios-modal__add-button">
                    {{ t('pwa.install.ios.addButton') }}
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Benefits -->
          <div class="ios-modal__benefits">
            <p class="ios-modal__benefits-title">{{ t('pwa.install.benefitsTitle') }}</p>
            <ul class="ios-modal__benefits-list">
              <li>
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
                </svg>
                {{ t('pwa.install.benefitFaster') }}
              </li>
              <li>
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <path d="M18.36 6.64a9 9 0 1 1-12.73 0" />
                  <line x1="12" y1="2" x2="12" y2="12" />
                </svg>
                {{ t('pwa.install.benefitOffline') }}
              </li>
              <li>
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
                  <polyline points="22 4 12 14.01 9 11.01" />
                </svg>
                {{ t('pwa.install.benefitNative') }}
              </li>
              <li>
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
                  <path d="M13.73 21a2 2 0 0 1-3.46 0" />
                </svg>
                {{ t('pwa.install.benefitNotifications') }}
              </li>
            </ul>
          </div>

          <!-- Footer Actions -->
          <div class="ios-modal__footer">
            <button
              class="ios-modal__button ios-modal__button--secondary"
              @click="handleDismiss"
            >
              {{ t('pwa.install.ios.notNow') }}
            </button>
            <button
              class="ios-modal__button ios-modal__button--primary"
              @click="handleClose"
            >
              {{ t('pwa.install.ios.gotIt') }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.ios-modal-overlay {
  position: fixed;
  inset: 0;
  z-index: var(--z-modal, 300);
  display: flex;
  align-items: flex-end;
  justify-content: center;
  background-color: rgba(0, 0, 0, 0.5);
  padding: 1rem;
  padding-bottom: env(safe-area-inset-bottom, 1rem);
}

.ios-modal {
  width: 100%;
  max-width: 400px;
  max-height: calc(100vh - 2rem);
  overflow-y: auto;
  background: var(--color-bg-primary, #ffffff);
  border-radius: var(--radius-xl, 1rem);
  box-shadow: var(--shadow-lg, 0 10px 15px -3px rgb(0 0 0 / 0.1));
}

.ios-modal__header {
  position: relative;
  padding: 1.5rem;
  text-align: center;
  border-bottom: 1px solid var(--color-border-light, #f3f4f6);
}

.ios-modal__icon {
  width: 56px;
  height: 56px;
  margin: 0 auto 0.75rem;
  border-radius: var(--radius-lg, 0.75rem);
  overflow: hidden;
  box-shadow: var(--shadow-md, 0 4px 6px -1px rgb(0 0 0 / 0.1));
}

.ios-modal__icon img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.ios-modal__title {
  margin: 0;
  font-size: var(--font-size-lg, 1.125rem);
  font-weight: 600;
  color: var(--color-text-primary, #111827);
}

.ios-modal__subtitle {
  margin: 0.25rem 0 0;
  font-size: var(--font-size-sm, 0.875rem);
  color: var(--color-text-secondary, #6b7280);
}

.ios-modal__close {
  position: absolute;
  top: 1rem;
  right: 1rem;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-bg-tertiary, #f3f4f6);
  border: none;
  border-radius: var(--radius-full, 9999px);
  color: var(--color-text-secondary, #6b7280);
  cursor: pointer;
  transition: all var(--transition-fast, 150ms ease);
}

.ios-modal__close:hover {
  background: var(--color-border, #e5e7eb);
  color: var(--color-text-primary, #111827);
}

.ios-modal__close svg {
  width: 1rem;
  height: 1rem;
}

.ios-modal__steps {
  padding: 1rem 1.5rem;
}

.ios-modal__step {
  display: flex;
  gap: 1rem;
  padding: 1rem 0;
}

.ios-modal__step:not(:last-child) {
  border-bottom: 1px solid var(--color-border-light, #f3f4f6);
}

.ios-modal__step-number {
  flex-shrink: 0;
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-primary, #3b82f6);
  color: white;
  font-size: var(--font-size-sm, 0.875rem);
  font-weight: 600;
  border-radius: var(--radius-full, 9999px);
}

.ios-modal__step-content {
  flex: 1;
  min-width: 0;
}

.ios-modal__step-title {
  margin: 0;
  font-size: var(--font-size-sm, 0.875rem);
  font-weight: 600;
  color: var(--color-text-primary, #111827);
}

.ios-modal__step-desc {
  margin: 0.25rem 0 0;
  font-size: var(--font-size-xs, 0.75rem);
  color: var(--color-text-secondary, #6b7280);
}

.ios-modal__step-visual {
  margin-top: 0.75rem;
  padding: 0.75rem;
  background: var(--color-bg-secondary, #f9fafb);
  border-radius: var(--radius-md, 0.5rem);
  display: flex;
  align-items: center;
  justify-content: center;
}

.ios-modal__step-visual--row {
  justify-content: flex-start;
}

.ios-modal__share-icon {
  width: 32px;
  height: 32px;
  color: var(--color-primary, #3b82f6);
}

.ios-modal__menu-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0.75rem;
  background: var(--color-bg-primary, #ffffff);
  border-radius: var(--radius-md, 0.5rem);
  box-shadow: var(--shadow-sm, 0 1px 2px 0 rgb(0 0 0 / 0.05));
}

.ios-modal__menu-item svg {
  width: 24px;
  height: 24px;
  color: var(--color-primary, #3b82f6);
}

.ios-modal__menu-item span {
  font-size: var(--font-size-sm, 0.875rem);
  color: var(--color-text-primary, #111827);
}

.ios-modal__add-button {
  padding: 0.5rem 1rem;
  background: var(--color-primary, #3b82f6);
  color: white;
  font-size: var(--font-size-sm, 0.875rem);
  font-weight: 500;
  border-radius: var(--radius-md, 0.5rem);
}

.ios-modal__benefits {
  padding: 1rem 1.5rem;
  background: var(--color-bg-secondary, #f9fafb);
  border-top: 1px solid var(--color-border-light, #f3f4f6);
}

.ios-modal__benefits-title {
  margin: 0 0 0.75rem;
  font-size: var(--font-size-sm, 0.875rem);
  font-weight: 600;
  color: var(--color-text-primary, #111827);
}

.ios-modal__benefits-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.5rem;
}

.ios-modal__benefits-list li {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  font-size: var(--font-size-xs, 0.75rem);
  color: var(--color-text-secondary, #6b7280);
}

.ios-modal__benefits-list svg {
  width: 0.875rem;
  height: 0.875rem;
  color: var(--color-success, #10b981);
  flex-shrink: 0;
}

.ios-modal__footer {
  display: flex;
  gap: 0.75rem;
  padding: 1rem 1.5rem;
  border-top: 1px solid var(--color-border-light, #f3f4f6);
}

.ios-modal__button {
  flex: 1;
  padding: 0.75rem 1rem;
  font-size: var(--font-size-sm, 0.875rem);
  font-weight: 500;
  border-radius: var(--radius-md, 0.5rem);
  border: none;
  cursor: pointer;
  transition: all var(--transition-fast, 150ms ease);
  min-height: 44px;
}

.ios-modal__button--primary {
  background: var(--color-primary, #3b82f6);
  color: white;
}

.ios-modal__button--primary:hover {
  background: var(--color-primary-hover, #2563eb);
}

.ios-modal__button--secondary {
  background: var(--color-bg-tertiary, #f3f4f6);
  color: var(--color-text-secondary, #6b7280);
}

.ios-modal__button--secondary:hover {
  background: var(--color-border, #e5e7eb);
  color: var(--color-text-primary, #111827);
}

/* RTL support */
[dir="rtl"] .ios-modal__close {
  right: auto;
  left: 1rem;
}

[dir="rtl"] .ios-modal__step {
  flex-direction: row-reverse;
}

[dir="rtl"] .ios-modal__menu-item {
  flex-direction: row-reverse;
}

[dir="rtl"] .ios-modal__benefits-list li {
  flex-direction: row-reverse;
}

/* Modal transition */
.modal-fade-enter-active,
.modal-fade-leave-active {
  transition: opacity 0.3s ease;
}

.modal-fade-enter-active .ios-modal,
.modal-fade-leave-active .ios-modal {
  transition: transform 0.3s ease;
}

.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}

.modal-fade-enter-from .ios-modal,
.modal-fade-leave-to .ios-modal {
  transform: translateY(100%);
}
</style>
