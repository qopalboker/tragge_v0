<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';
import { usePWAInstall } from '@/composables/usePWAInstall';
import IOSInstallModal from './IOSInstallModal.vue';

const {
  canShowInstallPrompt,
  isIOSSafari,
  showIOSModal,
  promptInstall,
  dismissPrompt,
  showIOSInstructions,
  hideIOSInstructions,
  dismissIOSPrompt,
} = usePWAInstall();

// Show banner for non-iOS devices with beforeinstallprompt support
const showBanner = computed(() => {
  return canShowInstallPrompt.value && !isIOSSafari.value;
});

// Show iOS banner (different UI with instructions link)
const showIOSBanner = computed(() => {
  return canShowInstallPrompt.value && isIOSSafari.value;
});

// Handle install button click
const handleInstall = async () => {
  if (isIOSSafari.value) {
    showIOSInstructions();
  } else {
    await promptInstall();
  }
};

// Handle dismiss button click
const handleDismiss = () => {
  if (isIOSSafari.value) {
    dismissIOSPrompt();
  } else {
    dismissPrompt();
  }
};
</script>

<template>
  <!-- Install Banner for Android/Desktop -->
  <Transition name="slide-up">
    <div
      v-if="showBanner || showIOSBanner"
      class="install-banner"
      role="alert"
      aria-live="polite"
    >
      <div class="install-banner__content">
        <!-- App Icon -->
        <div class="install-banner__icon">
          <img
            src="/icons/icon-72x72.png"
            alt="Tragge"
            width="48"
            height="48"
          />
        </div>

        <!-- Text Content -->
        <div class="install-banner__text">
          <p class="install-banner__title">
            {{ t('pwa.install.title') }}
          </p>
          <p class="install-banner__subtitle">
            {{ t('pwa.install.subtitle') }}
          </p>
        </div>

        <!-- Actions -->
        <div class="install-banner__actions">
          <button
            class="install-banner__button install-banner__button--primary"
            @click="handleInstall"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              class="install-banner__button-icon"
            >
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="7 10 12 15 17 10" />
              <line x1="12" y1="15" x2="12" y2="3" />
            </svg>
            {{ t('pwa.install.button') }}
          </button>
          <button
            class="install-banner__button install-banner__button--ghost"
            @click="handleDismiss"
            :aria-label="t('pwa.dismiss')"
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
      </div>

      <!-- Benefits (collapsed on mobile, shown on larger screens) -->
      <div class="install-banner__benefits hide-mobile">
        <div class="install-banner__benefit">
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
          <span>{{ t('pwa.install.benefitFaster') }}</span>
        </div>
        <div class="install-banner__benefit">
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
          <span>{{ t('pwa.install.benefitOffline') }}</span>
        </div>
        <div class="install-banner__benefit">
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
          <span>{{ t('pwa.install.benefitNative') }}</span>
        </div>
      </div>
    </div>
  </Transition>

  <!-- iOS Install Instructions Modal -->
  <IOSInstallModal
    :show="showIOSModal"
    @close="hideIOSInstructions"
    @dismiss="dismissIOSPrompt"
  />
</template>

<style scoped>
.install-banner {
  position: fixed;
  bottom: calc(var(--bottom-nav-height, 64px) + 1rem);
  left: 50%;
  transform: translateX(-50%);
  z-index: var(--z-modal, 300);
  width: calc(100% - 2rem);
  max-width: 480px;
  background: var(--color-bg-primary, #ffffff);
  border: 1px solid var(--color-border, #e5e7eb);
  border-radius: var(--radius-xl, 1rem);
  box-shadow: var(--shadow-lg, 0 10px 15px -3px rgb(0 0 0 / 0.1));
  overflow: hidden;
}

.install-banner__content {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 1rem;
}

.install-banner__icon {
  flex-shrink: 0;
  width: 48px;
  height: 48px;
  border-radius: var(--radius-md, 0.5rem);
  overflow: hidden;
}

.install-banner__icon img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.install-banner__text {
  flex: 1;
  min-width: 0;
}

.install-banner__title {
  margin: 0;
  font-size: var(--font-size-sm, 0.875rem);
  font-weight: 600;
  color: var(--color-text-primary, #111827);
  line-height: 1.3;
}

.install-banner__subtitle {
  margin: 0.25rem 0 0;
  font-size: var(--font-size-xs, 0.75rem);
  color: var(--color-text-secondary, #6b7280);
  line-height: 1.3;
}

.install-banner__actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-shrink: 0;
}

.install-banner__button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
  padding: 0.5rem 0.875rem;
  font-size: var(--font-size-sm, 0.875rem);
  font-weight: 500;
  border-radius: var(--radius-md, 0.5rem);
  border: none;
  cursor: pointer;
  transition: all var(--transition-fast, 150ms ease);
  min-height: 44px;
}

.install-banner__button--primary {
  background: var(--color-primary, #3b82f6);
  color: white;
}

.install-banner__button--primary:hover {
  background: var(--color-primary-hover, #2563eb);
}

.install-banner__button--ghost {
  background: transparent;
  color: var(--color-text-muted, #9ca3af);
  padding: 0.5rem;
  min-width: 44px;
}

.install-banner__button--ghost:hover {
  color: var(--color-text-secondary, #6b7280);
  background: var(--color-bg-tertiary, #f3f4f6);
}

.install-banner__button-icon {
  width: 1rem;
  height: 1rem;
}

.install-banner__button--ghost svg {
  width: 1.25rem;
  height: 1.25rem;
}

.install-banner__benefits {
  display: flex;
  gap: 1rem;
  padding: 0.75rem 1rem;
  background: var(--color-bg-secondary, #f9fafb);
  border-top: 1px solid var(--color-border-light, #f3f4f6);
}

.install-banner__benefit {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  font-size: var(--font-size-xs, 0.75rem);
  color: var(--color-text-secondary, #6b7280);
}

.install-banner__benefit svg {
  width: 0.875rem;
  height: 0.875rem;
  color: var(--color-primary, #3b82f6);
}

/* RTL support */
[dir="rtl"] .install-banner__content {
  flex-direction: row-reverse;
}

[dir="rtl"] .install-banner__actions {
  flex-direction: row-reverse;
}

[dir="rtl"] .install-banner__benefits {
  flex-direction: row-reverse;
}

[dir="rtl"] .install-banner__benefit {
  flex-direction: row-reverse;
}

/* Slide up transition */
.slide-up-enter-active,
.slide-up-leave-active {
  transition: all 0.3s ease;
}

.slide-up-enter-from,
.slide-up-leave-to {
  transform: translateX(-50%) translateY(100%);
  opacity: 0;
}

/* Mobile responsive */
@media (max-width: 767px) {
  .install-banner {
    bottom: calc(var(--bottom-nav-height, 64px) + 0.5rem);
    width: calc(100% - 1rem);
    max-width: none;
    border-radius: var(--radius-lg, 0.75rem);
  }

  .install-banner__content {
    padding: 0.75rem;
    gap: 0.625rem;
  }

  .install-banner__icon {
    width: 40px;
    height: 40px;
  }

  .install-banner__title {
    font-size: var(--font-size-xs, 0.75rem);
  }

  .install-banner__subtitle {
    display: none;
  }

  .install-banner__button {
    padding: 0.5rem 0.75rem;
    font-size: var(--font-size-xs, 0.75rem);
    min-height: 40px;
  }

  .install-banner__button--ghost {
    min-width: 40px;
    padding: 0.375rem;
  }
}

/* Desktop: higher position when no bottom nav */
@media (min-width: 768px) {
  .install-banner {
    bottom: 1.5rem;
  }
}

/* Safe area inset for notched devices */
@supports (padding-bottom: env(safe-area-inset-bottom)) {
  .install-banner {
    bottom: calc(var(--bottom-nav-height, 64px) + 1rem + env(safe-area-inset-bottom, 0));
  }

  @media (min-width: 768px) {
    .install-banner {
      bottom: calc(1.5rem + env(safe-area-inset-bottom, 0));
    }
  }
}
</style>
