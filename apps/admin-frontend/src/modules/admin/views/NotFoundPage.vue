<script setup lang="ts">
import { useRouter } from 'vue-router';
import { t } from '@/i18n';

const router = useRouter();

function goBack() {
  if (window.history.length > 1) {
    router.back();
  } else {
    router.push('/admin/users');
  }
}

function goToDashboard() {
  router.push('/admin/users');
}
</script>

<template>
  <div class="error-page">
    <div class="error-content">
      <div class="error-illustration">
        <svg viewBox="0 0 200 200" fill="none" xmlns="http://www.w3.org/2000/svg" class="error-icon">
          <circle cx="100" cy="100" r="90" stroke="currentColor" stroke-width="4" stroke-dasharray="8 8" opacity="0.3"/>
          <path d="M100 30C61.34 30 30 61.34 30 100C30 138.66 61.34 170 100 170C138.66 170 170 138.66 170 100C170 61.34 138.66 30 100 30Z" stroke="currentColor" stroke-width="6" stroke-linecap="round"/>
          <path d="M75 80L90 95M90 80L75 95" stroke="currentColor" stroke-width="6" stroke-linecap="round"/>
          <path d="M110 80L125 95M125 80L110 95" stroke="currentColor" stroke-width="6" stroke-linecap="round"/>
          <path d="M70 130C70 130 85 115 100 115C115 115 130 130 130 130" stroke="currentColor" stroke-width="6" stroke-linecap="round"/>
          <text x="100" y="175" text-anchor="middle" fill="currentColor" font-size="24" font-weight="bold">404</text>
        </svg>
      </div>

      <h1 class="error-title">{{ t('error.notFound.title') }}</h1>
      <p class="error-message">{{ t('error.notFound.message') }}</p>

      <div class="error-actions">
        <button class="btn btn-primary" @click="goToDashboard">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>
            <polyline points="9 22 9 12 15 12 15 22"/>
          </svg>
          {{ t('error.notFound.goToDashboard') }}
        </button>
        <button class="btn btn-secondary" @click="goBack">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="19" y1="12" x2="5" y2="12"/>
            <polyline points="12 19 5 12 12 5"/>
          </svg>
          {{ t('error.notFound.goBack') }}
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
  color: var(--color-primary);
  animation: float 3s ease-in-out infinite;
}

@keyframes float {
  0%, 100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-10px);
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
