<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { t } from '@/i18n';
import { listTournamentTemplates, type TournamentTemplate } from '@/api/tournament-templates';
import TierList from '@/components/TierList.vue';

const templates = ref<TournamentTemplate[]>([]);
const loading = ref(false);
const error = ref(false);
const searchQuery = ref('');
const expandedTemplateId = ref<string | null>(null);

async function fetchTemplates(): Promise<void> {
  loading.value = true;
  error.value = false;
  try {
    const result = await listTournamentTemplates();
    templates.value = result.templates || [];
  } catch {
    error.value = true;
  } finally {
    loading.value = false;
  }
}

const filteredTemplates = computed(() => {
  if (!searchQuery.value) return templates.value;
  const q = searchQuery.value.toLowerCase();
  return templates.value.filter((tpl) =>
    tpl.name.toLowerCase().includes(q) ||
    tpl.template_key?.toLowerCase().includes(q) ||
    tpl.market_type?.toLowerCase().includes(q)
  );
});

function toggleTiers(templateId: string): void {
  expandedTemplateId.value = expandedTemplateId.value === templateId ? null : templateId;
}

function formatDuration(minutes: number): string {
  if (minutes < 60) return `${minutes}m`;
  if (minutes < 1440) return `${Math.round(minutes / 60)}h`;
  return `${Math.round(minutes / 1440)}d`;
}

function formatEntryFee(cents: number): string {
  if (cents === 0) return 'Free';
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
  }).format(cents / 100);
}

onMounted(fetchTemplates);
</script>

<template>
  <div class="tournament-templates-page">
    <div class="page-header">
      <div>
        <h1>{{ t('tournamentTemplates.title') }}</h1>
        <p class="page-subtitle">{{ t('tournamentTemplates.subtitle') }}</p>
      </div>
    </div>

    <!-- Search -->
    <div class="filter-bar">
      <input
        v-model="searchQuery"
        type="text"
        class="input search-input"
        :placeholder="t('tournamentTemplates.search')"
      />
    </div>

    <!-- Loading -->
    <div v-if="loading" class="loading-state">{{ t('common.loading') }}</div>

    <!-- Error -->
    <div v-else-if="error" class="error-state">
      <p>{{ t('common.error') }}</p>
      <button class="btn btn-primary" @click="fetchTemplates">{{ t('common.retry') }}</button>
    </div>

    <!-- Empty -->
    <div v-else-if="filteredTemplates.length === 0" class="empty-state">
      <p>{{ t('tournamentTemplates.noTemplates') }}</p>
    </div>

    <!-- Template Cards -->
    <div v-else class="templates-grid">
      <div
        v-for="tpl in filteredTemplates"
        :key="tpl.id"
        :class="['template-card', { expanded: expandedTemplateId === tpl.id }]"
      >
        <div class="card-header">
          <div class="card-title-row">
            <h3 class="card-title">{{ tpl.name }}</h3>
            <span :class="['badge', tpl.is_active ? 'badge-success' : 'badge-secondary']">
              {{ tpl.is_active ? t('tournamentTemplates.active') : t('tournamentTemplates.inactive') }}
            </span>
          </div>
          <span v-if="tpl.template_key" class="template-key-badge">{{ tpl.template_key }}</span>
        </div>

        <div class="card-meta">
          <div class="meta-item">
            <span class="meta-label">{{ t('tournamentTemplates.duration') }}</span>
            <span class="meta-value">{{ formatDuration(tpl.duration_minutes) }}</span>
          </div>
          <div class="meta-item">
            <span class="meta-label">{{ t('tournamentTemplates.entryFee') }}</span>
            <span class="meta-value">{{ tpl.is_free ? t('tiers.free') : formatEntryFee(tpl.entry_fee_cents) }}</span>
          </div>
          <div v-if="tpl.market_type" class="meta-item">
            <span class="meta-label">{{ t('admin.nav.market') }}</span>
            <span :class="['badge', `badge-${tpl.asset_class || 'default'}`]">{{ tpl.market_type }}</span>
          </div>
          <div class="meta-item">
            <span class="meta-label">{{ t('tournamentTemplates.commission') }}</span>
            <span class="meta-value">{{ (tpl.commission_rate * 100).toFixed(1) }}%</span>
          </div>
          <div v-if="tpl.auto_create" class="meta-item">
            <span class="badge badge-info">{{ t('tournamentTemplates.autoCreate') }}</span>
          </div>
        </div>

        <div class="card-footer">
          <button
            class="btn btn-sm"
            :class="expandedTemplateId === tpl.id ? 'btn-secondary' : 'btn-primary'"
            @click="toggleTiers(tpl.id)"
          >
            {{ expandedTemplateId === tpl.id
              ? t('tournamentTemplates.hideTiers')
              : t('tournamentTemplates.manageTiers') }}
          </button>
        </div>

        <!-- Expanded TierList -->
        <div v-if="expandedTemplateId === tpl.id" class="tier-section">
          <TierList :template-id="tpl.id" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tournament-templates-page {
  padding: var(--spacing-xl);
  max-width: 1200px;
}

.page-header {
  margin-bottom: var(--spacing-lg);
}

.page-header h1 {
  font-size: var(--font-size-2xl, 1.5rem);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-xs) 0;
}

.page-subtitle {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  margin: 0;
}

.filter-bar {
  margin-bottom: var(--spacing-lg);
}

.search-input {
  width: 100%;
  max-width: 400px;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  background: var(--color-bg-primary);
  color: var(--color-text-primary);
}

.loading-state,
.empty-state,
.error-state {
  text-align: center;
  padding: var(--spacing-2xl, 3rem);
  color: var(--color-text-muted);
}

.error-state .btn {
  margin-top: var(--spacing-md);
}

.templates-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: var(--spacing-lg);
}

.template-card {
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg, 12px);
  padding: var(--spacing-lg);
  transition: box-shadow var(--transition-fast);
}

.template-card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.template-card.expanded {
  grid-column: 1 / -1;
}

.card-header {
  margin-bottom: var(--spacing-md);
}

.card-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
}

.card-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.template-key-badge {
  display: inline-block;
  margin-top: var(--spacing-xs);
  padding: 1px 6px;
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-family: monospace;
  background: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

.card-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-md);
}

.meta-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.meta-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.meta-value {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.card-footer {
  display: flex;
  justify-content: flex-end;
}

.tier-section {
  margin-top: var(--spacing-lg);
  padding-top: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

/* Badges */
.badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
}

.badge-success {
  background: #D1FAE5;
  color: #065F46;
}

.badge-secondary {
  background: #E5E7EB;
  color: #4B5563;
}

.badge-info {
  background: #DBEAFE;
  color: #1E40AF;
}

.badge-crypto {
  background: #FEF3C7;
  color: #92400E;
}

.badge-forex {
  background: #DBEAFE;
  color: #1E40AF;
}

.badge-default {
  background: #F3F4F6;
  color: #374151;
}

/* Buttons */
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  border: none;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
}

.btn-primary {
  background: var(--color-primary);
  color: white;
}

.btn-primary:hover {
  opacity: 0.9;
}

.btn-secondary {
  background: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.btn-secondary:hover {
  background: var(--color-border);
}

/* Dark mode */
:root[data-theme="dark"] .badge-success {
  background: #064E3B;
  color: #6EE7B7;
}

:root[data-theme="dark"] .badge-secondary {
  background: #374151;
  color: #D1D5DB;
}

:root[data-theme="dark"] .badge-info {
  background: #1E3A5F;
  color: #93C5FD;
}

:root[data-theme="dark"] .badge-crypto {
  background: #78350F;
  color: #FCD34D;
}

:root[data-theme="dark"] .badge-forex {
  background: #1E3A5F;
  color: #93C5FD;
}

:root[data-theme="dark"] .template-card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
}

/* Responsive */
@media (max-width: 768px) {
  .templates-grid {
    grid-template-columns: 1fr;
  }

  .template-card.expanded {
    grid-column: 1;
  }
}
</style>
