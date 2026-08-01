<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';
import { useI18nStore } from '@/stores/i18n';

const i18nStore = useI18nStore();
const isRTL = computed(() => i18nStore.locale === 'fa');

defineProps<{
  userName?: string;
  userEmail?: string;
}>();

const emit = defineEmits<{
  continue: [];
}>();
</script>

<template>
  <div class="modal-overlay">
    <div :class="['modal-content', { rtl: isRTL }]">
      <div class="success-icon">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <polyline points="20,6 9,17 4,12" />
        </svg>
      </div>

      <h2>{{ t('verification.successTitle') }}</h2>
      <p class="subtitle">{{ t('verification.successSubtitle') }}</p>

      <button class="continue-btn" @click="emit('continue')">
        {{ t('verification.goToDashboard') }}
      </button>
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
  padding: var(--spacing-2xl) var(--spacing-2xl) var(--spacing-xl);
  max-width: 400px;
  width: 100%;
  text-align: center;
  box-shadow: var(--shadow-lg);
  animation: scaleIn 0.3s ease;
}

.modal-content.rtl {
  direction: rtl;
}

.success-icon {
  width: 80px;
  height: 80px;
  margin: 0 auto var(--spacing-lg);
  border-radius: 50%;
  background: #d1fae5;
  color: #059669;
  display: flex;
  align-items: center;
  justify-content: center;
  animation: checkPop 0.4s ease 0.2s both;
}

h2 {
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: #059669;
  margin: 0 0 var(--spacing-sm) 0;
}

.subtitle {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  margin: 0 0 var(--spacing-xl) 0;
}

.continue-btn {
  width: 100%;
  padding: var(--spacing-md) var(--spacing-lg);
  background: var(--color-primary);
  color: white;
  border: none;
  border-radius: var(--radius-lg);
  font-size: var(--font-size-base);
  font-weight: 600;
  cursor: pointer;
  transition: background-color 0.2s;
}

.continue-btn:hover {
  opacity: 0.9;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes scaleIn {
  from { opacity: 0; transform: scale(0.9); }
  to { opacity: 1; transform: scale(1); }
}

@keyframes checkPop {
  from { transform: scale(0); }
  60% { transform: scale(1.1); }
  to { transform: scale(1); }
}
</style>
