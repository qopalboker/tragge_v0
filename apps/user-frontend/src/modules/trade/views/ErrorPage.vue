<script setup lang="ts">
import { t } from '@/i18n';
import { USER_PANEL_URL } from '@/utils/userPanelUrl';

function tryAgain() {
  window.location.reload();
}

function goToDashboard() {
  window.location.href = USER_PANEL_URL;
}
</script>

<template>
  <div class="error-page">
    <div class="error-content">
      <div class="error-illustration">
        <svg viewBox="0 0 200 200" fill="none" xmlns="http://www.w3.org/2000/svg" class="error-icon">
          <circle cx="100" cy="100" r="90" stroke="currentColor" stroke-width="4" stroke-dasharray="8 8" opacity="0.3"/>
          <!-- Server/gear icon -->
          <rect x="60" y="50" width="80" height="100" rx="8" stroke="currentColor" stroke-width="5" fill="none"/>
          <line x1="60" y1="80" x2="140" y2="80" stroke="currentColor" stroke-width="3"/>
          <line x1="60" y1="110" x2="140" y2="110" stroke="currentColor" stroke-width="3"/>
          <circle cx="75" cy="65" r="5" fill="currentColor"/>
          <circle cx="90" cy="65" r="5" fill="currentColor"/>
          <circle cx="75" cy="95" r="5" fill="currentColor"/>
          <circle cx="90" cy="95" r="5" fill="currentColor"/>
          <!-- Warning triangle -->
          <path d="M130 130L145 155H115L130 130Z" stroke="currentColor" stroke-width="4" fill="none"/>
          <line x1="130" y1="140" x2="130" y2="147" stroke="currentColor" stroke-width="3" stroke-linecap="round"/>
          <circle cx="130" cy="151" r="2" fill="currentColor"/>
          <text x="100" y="185" text-anchor="middle" fill="currentColor" font-size="20" font-weight="bold">500</text>
        </svg>
      </div>

      <h1 class="error-title">{{ t('error.server.title') }}</h1>
      <p class="error-message">{{ t('error.server.message') }}</p>

      <div class="error-actions">
        <button class="btn btn-primary" @click="tryAgain">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="23 4 23 10 17 10"/>
            <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
          </svg>
          {{ t('error.server.tryAgain') }}
        </button>
        <button class="btn btn-secondary" @click="goToDashboard">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>
            <polyline points="9 22 9 12 15 12 15 22"/>
          </svg>
          {{ t('error.server.goToDashboard') }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.error-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-xl);
  background: var(--color-bg-primary);
}

.error-content {
  text-align: center;
  max-width: 480px;
}

.error-illustration {
  margin-bottom: var(--spacing-xl);
}

.error-icon {
  width: 180px;
  height: 180px;
  color: var(--color-danger);
  animation: shake 0.5s ease-in-out;
}

@keyframes shake {
  0%, 100% {
    transform: translateX(0);
  }
  10%, 30%, 50%, 70%, 90% {
    transform: translateX(-5px);
  }
  20%, 40%, 60%, 80% {
    transform: translateX(5px);
  }
}

.error-title {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-sm) 0;
}

.error-message {
  font-size: var(--font-size-md);
  color: var(--color-text-secondary);
  margin: 0 0 var(--spacing-xl) 0;
  line-height: var(--line-height-relaxed);
}

.error-actions {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-lg);
  font-size: var(--font-size-md);
  font-weight: 500;
  border-radius: 8px;
  border: none;
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn-primary {
  background: var(--color-primary);
  color: white;
}

.btn-primary:hover {
  background: var(--color-primary-dark, #2563eb);
  transform: translateY(-1px);
}

.btn-secondary {
  background: var(--color-bg-secondary);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border);
}

.btn-secondary:hover {
  background: var(--color-bg-tertiary);
}

@media (min-width: 480px) {
  .error-actions {
    flex-direction: row;
    justify-content: center;
  }
}

/* RTL Support */
[dir="rtl"] .btn svg {
  transform: scaleX(-1);
}
</style>
