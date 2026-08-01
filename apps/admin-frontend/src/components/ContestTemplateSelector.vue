<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { t } from '@/i18n';
import { api } from '@/api';

export interface ContestTemplate {
  key: string;
  name: string;
  description: string;
  asset_class: string;
  duration_type: string;
  default_symbols: string[];
}

const emit = defineEmits<{
  (e: 'select', template: ContestTemplate): void;
}>();

const templates = ref<ContestTemplate[]>([]);
const loading = ref(false);
const error = ref(false);

async function fetchTemplates(): Promise<void> {
  loading.value = true;
  error.value = false;
  try {
    const response = await api.get('/api/admin/contests/templates');
    templates.value = response.data;
  } catch {
    error.value = true;
  } finally {
    loading.value = false;
  }
}

function handleSelect(template: ContestTemplate): void {
  emit('select', template);
}

onMounted(fetchTemplates);
</script>

<template>
  <div class="template-selector">
    <div class="template-header">
      <h2 class="template-title">{{ t('contestForm.templates.selectTitle') }}</h2>
      <p class="template-description">{{ t('contestForm.templates.selectDescription') }}</p>
    </div>

    <div v-if="loading" class="template-loading">
      {{ t('contestForm.templates.loading') }}
    </div>

    <div v-else-if="error" class="template-error">
      <p>{{ t('contestForm.templates.loadError') }}</p>
      <button class="btn btn-secondary" @click="fetchTemplates">
        {{ t('common.retry') }}
      </button>
    </div>

    <div v-else-if="templates.length === 0" class="template-empty">
      {{ t('contestForm.templates.empty') }}
    </div>

    <div v-else class="template-grid">
      <div
        v-for="tpl in templates"
        :key="tpl.key"
        class="template-card"
        @click="handleSelect(tpl)"
      >
        <div class="template-card-header">
          <span class="template-badge" :class="`badge-${tpl.asset_class}`">
            {{ t(`contestForm.assetClasses.${tpl.asset_class}`) }}
          </span>
        </div>

        <h3 class="template-card-name">{{ tpl.name }}</h3>
        <p class="template-card-desc">{{ tpl.description }}</p>

        <div class="template-card-meta">
          <div class="meta-item">
            <span class="meta-label">{{ t('contestForm.templates.duration') }}</span>
            <span class="meta-value">{{ t(`contestForm.durationTypes.${tpl.duration_type}`) }}</span>
          </div>
          <div v-if="tpl.default_symbols && tpl.default_symbols.length > 0" class="meta-item">
            <span class="meta-label">{{ t('contestForm.templates.symbols') }}</span>
            <span class="meta-value symbols-list">{{ tpl.default_symbols.join(', ') }}</span>
          </div>
        </div>

        <button class="btn btn-primary template-select-btn" @click.stop="handleSelect(tpl)">
          {{ t('contestForm.templates.select') }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.template-selector {
  width: 100%;
}

.template-header {
  margin-bottom: var(--spacing-lg);
}

.template-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.template-description {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.template-loading,
.template-empty {
  text-align: center;
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.template-error {
  text-align: center;
  padding: var(--spacing-2xl);
  color: var(--color-danger);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
}

.template-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-md);
}

.template-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  cursor: pointer;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
  display: flex;
  flex-direction: column;
}

.template-card:hover {
  border-color: var(--color-primary);
  box-shadow: var(--shadow-md);
}

.template-card-header {
  margin-bottom: var(--spacing-sm);
}

.template-badge {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.badge-crypto {
  background-color: #FEF3C7;
  color: #92400E;
}

.dark .badge-crypto {
  background-color: #78350F;
  color: #FDE68A;
}

.badge-forex {
  background-color: #DBEAFE;
  color: #1E40AF;
}

.dark .badge-forex {
  background-color: #1E3A5F;
  color: #93C5FD;
}

.badge-stocks {
  background-color: #D1FAE5;
  color: #065F46;
}

.dark .badge-stocks {
  background-color: #064E3B;
  color: #6EE7B7;
}

.badge-mixed {
  background-color: #EDE9FE;
  color: #5B21B6;
}

.dark .badge-mixed {
  background-color: #4C1D95;
  color: #C4B5FD;
}

.template-card-name {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.template-card-desc {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  line-height: var(--line-height-relaxed);
  margin-bottom: var(--spacing-md);
  flex: 1;
}

.template-card-meta {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
  padding-top: var(--spacing-sm);
  border-top: 1px solid var(--color-border);
}

.meta-item {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--spacing-sm);
}

.meta-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.meta-value {
  font-size: var(--font-size-xs);
  color: var(--color-text-primary);
  font-weight: 500;
  text-align: right;
}

.symbols-list {
  word-break: break-word;
}

.template-select-btn {
  width: 100%;
}

@media (max-width: 767px) {
  .template-grid {
    grid-template-columns: 1fr;
  }
}
</style>
