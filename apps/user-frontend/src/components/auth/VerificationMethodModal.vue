<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';
import { useI18nStore } from '@/stores/i18n';

const i18nStore = useI18nStore();
const isRTL = computed(() => i18nStore.locale === 'fa');

const props = defineProps<{
  maskedPhone?: string;
  maskedEmail?: string;
  availableMethods: string[];
  loading: boolean;
}>();

const emit = defineEmits<{
  select: [method: 'sms' | 'email'];
  close: [];
}>();

const hasSms = computed(() => props.availableMethods.includes('sms'));
const hasEmail = computed(() => props.availableMethods.includes('email'));
</script>

<template>
  <div class="modal-overlay" @click.self="emit('close')">
    <div :class="['modal-content', { rtl: isRTL }]" @click.stop>
      <button class="close-btn" @click="emit('close')" :aria-label="t('verification.close')">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
        </svg>
      </button>

      <div class="modal-icon">
        <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
        </svg>
      </div>

      <h2>{{ t('verification.chooseMethodTitle') }}</h2>
      <p class="subtitle">{{ t('verification.chooseMethodSubtitle') }}</p>

      <div class="methods">
        <!-- Email option -->
        <button
          v-if="hasEmail"
          class="method-card"
          :disabled="loading"
          @click="emit('select', 'email')"
        >
          <div class="method-icon email-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="2" y="4" width="20" height="16" rx="2" />
              <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
            </svg>
          </div>
          <div class="method-info">
            <span class="method-title">{{ t('verification.viaEmail') }}</span>
            <span class="method-desc">{{ t('verification.viaEmailDesc') }}</span>
            <span v-if="maskedEmail" class="method-dest" dir="ltr">{{ maskedEmail }}</span>
          </div>
          <div class="method-arrow">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline :points="isRTL ? '15,18 9,12 15,6' : '9,18 15,12 9,6'" />
            </svg>
          </div>
        </button>

        <!-- SMS option -->
        <button
          v-if="hasSms"
          class="method-card"
          :disabled="loading"
          @click="emit('select', 'sms')"
        >
          <div class="method-icon sms-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
            </svg>
          </div>
          <div class="method-info">
            <span class="method-title">{{ t('verification.viaSms') }}</span>
            <span class="method-desc">{{ t('verification.viaSmsDesc') }}</span>
            <span v-if="maskedPhone" class="method-dest" dir="ltr">{{ maskedPhone }}</span>
          </div>
          <div class="method-arrow">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline :points="isRTL ? '15,18 9,12 15,6' : '9,18 15,12 9,6'" />
            </svg>
          </div>
        </button>
      </div>

      <div v-if="loading" class="loading-row">
        <div class="spinner"></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: var(--spacing-lg);
  animation: fadeIn 0.2s ease;
}

.modal-content {
  background: var(--color-bg-primary);
  border-radius: var(--radius-xl);
  padding: var(--spacing-2xl);
  max-width: 420px;
  width: 100%;
  position: relative;
  box-shadow: var(--shadow-lg);
  animation: slideUp 0.3s ease;
}

.modal-content.rtl {
  direction: rtl;
}

.close-btn {
  position: absolute;
  top: var(--spacing-md);
  right: var(--spacing-md);
  background: none;
  border: none;
  color: var(--color-text-secondary);
  cursor: pointer;
  padding: var(--spacing-xs);
  border-radius: var(--radius-md);
  transition: all 0.2s;
}

.rtl .close-btn {
  right: auto;
  left: var(--spacing-md);
}

.close-btn:hover {
  background: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.modal-icon {
  width: 64px;
  height: 64px;
  margin: 0 auto var(--spacing-lg);
  border-radius: 50%;
  background: #dbeafe;
  color: #2563eb;
  display: flex;
  align-items: center;
  justify-content: center;
}

h2 {
  text-align: center;
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-xs) 0;
}

.subtitle {
  text-align: center;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  margin: 0 0 var(--spacing-xl) 0;
}

.methods {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.method-card {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  border: 2px solid var(--color-border);
  border-radius: var(--radius-lg);
  background: var(--color-bg-primary);
  cursor: pointer;
  transition: all 0.2s;
  text-align: start;
  width: 100%;
}

.method-card:hover:not(:disabled) {
  border-color: var(--color-primary);
  background: var(--color-bg-secondary);
}

.method-card:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.method-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.email-icon {
  background: #dbeafe;
  color: #2563eb;
}

.sms-icon {
  background: #d1fae5;
  color: #059669;
}

.method-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.method-title {
  font-weight: 600;
  color: var(--color-text-primary);
  font-size: var(--font-size-base);
}

.method-desc {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.method-dest {
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  font-family: 'SF Mono', 'Fira Code', monospace;
}

.method-arrow {
  color: var(--color-text-secondary);
  flex-shrink: 0;
}

.loading-row {
  display: flex;
  justify-content: center;
  margin-top: var(--spacing-lg);
}

.spinner {
  width: 24px;
  height: 24px;
  border: 3px solid var(--color-bg-tertiary);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes slideUp {
  from { opacity: 0; transform: translateY(20px); }
  to { opacity: 1; transform: translateY(0); }
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
