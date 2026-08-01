<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { getContestTemplates, type ContestTemplate } from '@/api/contest-templates';

const router = useRouter();
const toast = useToast();

const templates = ref<ContestTemplate[]>([]);
const loading = ref(false);
const error = ref(false);

// Filters
const filterAssetClass = ref('');
const filterDurationType = ref('');
const filterFreeOnly = ref(false);

async function fetchTemplates(): Promise<void> {
  loading.value = true;
  error.value = false;
  try {
    templates.value = await getContestTemplates();
  } catch {
    error.value = true;
    toast.error(t('contestTemplates.loadError'));
  } finally {
    loading.value = false;
  }
}

const filteredTemplates = computed(() => {
  return templates.value.filter((tpl) => {
    if (filterAssetClass.value && tpl.asset_class !== filterAssetClass.value) return false;
    if (filterDurationType.value && tpl.duration_type !== filterDurationType.value) return false;
    if (filterFreeOnly.value && !tpl.is_free) return false;
    return true;
  });
});

const hasActiveFilters = computed(() => {
  return filterAssetClass.value !== '' || filterDurationType.value !== '' || filterFreeOnly.value;
});

function clearFilters(): void {
  filterAssetClass.value = '';
  filterDurationType.value = '';
  filterFreeOnly.value = false;
}

function formatEntryFee(cents: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
  }).format(cents / 100);
}

function formatQty(qty: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 0,
  }).format(qty);
}

function formatDuration(minutes: number): string {
  if (minutes < 60) {
    return t('contestTemplates.durationMinutes', { count: String(minutes) });
  }
  if (minutes < 1440) {
    const hours = Math.round(minutes / 60);
    return t('contestTemplates.durationHours', { count: String(hours) });
  }
  const days = Math.round(minutes / 1440);
  if (days === 1) {
    return t('contestTemplates.durationDays', { count: String(days) });
  }
  return t('contestTemplates.durationDaysPlural', { count: String(days) });
}

function formatPlatformFee(bps: number): string {
  return `${(bps / 100).toFixed(1)}%`;
}

function displaySymbols(symbols: string[]): { visible: string[]; remaining: number } {
  if (!symbols || symbols.length <= 5) {
    return { visible: symbols || [], remaining: 0 };
  }
  return { visible: symbols.slice(0, 5), remaining: symbols.length - 5 };
}

function handleCreateContest(template: ContestTemplate): void {
  router.push({
    name: 'admin-contest-new',
    query: { template: template.key },
  });
}

onMounted(fetchTemplates);
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div class="header-text">
        <h1 class="page-title">{{ t('contestTemplates.title') }}</h1>
        <p class="page-subtitle">{{ t('contestTemplates.subtitle') }}</p>
      </div>
    </div>

    <!-- Filter Bar -->
    <div class="filter-bar">
      <div class="filter-group">
        <select v-model="filterAssetClass" class="filter-select">
          <option value="">{{ t('contestTemplates.allAssetClasses') }}</option>
          <option value="crypto">{{ t('contestForm.assetClasses.crypto') }}</option>
          <option value="forex">{{ t('contestForm.assetClasses.forex') }}</option>
          <option value="stocks">{{ t('contestForm.assetClasses.stocks') }}</option>
          <option value="mixed">{{ t('contestForm.assetClasses.mixed') }}</option>
        </select>

        <select v-model="filterDurationType" class="filter-select">
          <option value="">{{ t('contestTemplates.allDurationTypes') }}</option>
          <option value="rush_30min">{{ t('contestForm.durationTypes.rush_30min') }}</option>
          <option value="hourly">{{ t('contestForm.durationTypes.hourly') }}</option>
          <option value="four_hour">{{ t('contestForm.durationTypes.four_hour') }}</option>
          <option value="daily">{{ t('contestForm.durationTypes.daily') }}</option>
          <option value="weekly">{{ t('contestForm.durationTypes.weekly') }}</option>
        </select>

        <label class="filter-checkbox">
          <input v-model="filterFreeOnly" type="checkbox" />
          <span>{{ t('contestTemplates.freeOnly') }}</span>
        </label>
      </div>

      <button v-if="hasActiveFilters" class="btn btn-text" @click="clearFilters">
        {{ t('contestTemplates.clearFilters') }}
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="state-container">
      <div class="spinner" />
      <p>{{ t('contestTemplates.loading') }}</p>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="state-container state-error">
      <p>{{ t('contestTemplates.loadError') }}</p>
      <button class="btn btn-secondary" @click="fetchTemplates">
        {{ t('common.retry') }}
      </button>
    </div>

    <!-- Empty State -->
    <div v-else-if="templates.length === 0" class="state-container">
      <p>{{ t('contestTemplates.noResults') }}</p>
    </div>

    <!-- No Filter Results -->
    <div v-else-if="filteredTemplates.length === 0" class="state-container">
      <p>{{ t('contestTemplates.noFilterResults') }}</p>
      <button class="btn btn-text" @click="clearFilters">
        {{ t('contestTemplates.clearFilters') }}
      </button>
    </div>

    <!-- Template Grid -->
    <div v-else class="template-grid">
      <div
        v-for="tpl in filteredTemplates"
        :key="tpl.key"
        class="template-card"
      >
        <div class="card-header">
          <span class="badge badge-asset" :class="`badge-${tpl.asset_class}`">
            {{ t(`contestForm.assetClasses.${tpl.asset_class}`) }}
          </span>
          <span class="badge badge-duration" :class="`badge-${tpl.duration_type}`">
            {{ t(`contestForm.durationTypes.${tpl.duration_type}`) }}
          </span>
        </div>

        <h3 class="card-name">{{ tpl.name }}</h3>
        <p class="card-description">{{ tpl.description }}</p>

        <!-- Symbols -->
        <div v-if="tpl.default_symbols && tpl.default_symbols.length > 0" class="card-symbols">
          <span class="symbols-label">{{ t('contestTemplates.symbols') }}</span>
          <div class="symbols-list">
            <span
              v-for="symbol in displaySymbols(tpl.default_symbols).visible"
              :key="symbol"
              class="symbol-tag"
            >
              {{ symbol }}
            </span>
            <span
              v-if="displaySymbols(tpl.default_symbols).remaining > 0"
              class="symbol-tag symbol-more"
            >
              {{ t('contestTemplates.moreSymbols', { count: String(displaySymbols(tpl.default_symbols).remaining) }) }}
            </span>
          </div>
        </div>

        <!-- Meta Info -->
        <div class="card-meta">
          <div class="meta-row">
            <span class="meta-label">{{ t('contestTemplates.entryFee') }}</span>
            <span v-if="tpl.is_free" class="meta-value free-badge">{{ t('contestTemplates.free') }}</span>
            <span v-else class="meta-value">{{ formatEntryFee(tpl.entry_fee_cents) }}</span>
          </div>
          <div class="meta-row">
            <span class="meta-label">{{ t('contestTemplates.qty') }}</span>
            <span class="meta-value">{{ formatQty(tpl.qty_total) }}</span>
          </div>
          <div class="meta-row">
            <span class="meta-label">{{ t('contestTemplates.duration') }}</span>
            <span class="meta-value">{{ formatDuration(tpl.duration_minutes) }}</span>
          </div>
          <div class="meta-row">
            <span class="meta-label">{{ t('contestTemplates.participants') }}</span>
            <span v-if="tpl.max_participants" class="meta-value">
              {{ t('contestTemplates.participantsRange', { min: String(tpl.min_participants), max: String(tpl.max_participants) }) }}
            </span>
            <span v-else class="meta-value">
              {{ t('contestTemplates.participantsMin', { min: String(tpl.min_participants) }) }}
            </span>
          </div>
          <div class="meta-row">
            <span class="meta-label">{{ t('contestTemplates.platformFee') }}</span>
            <span class="meta-value">{{ formatPlatformFee(tpl.platform_fee_bps) }}</span>
          </div>
        </div>

        <button class="btn btn-primary card-action" @click="handleCreateContest(tpl)">
          {{ t('contestTemplates.createContest') }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page {
  padding: var(--spacing-lg);
}

.page-header {
  margin-bottom: var(--spacing-lg);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.page-subtitle {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

/* Filter Bar */
.filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
  padding: var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  flex-wrap: wrap;
}

.filter-group {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.filter-select {
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
  cursor: pointer;
  min-width: 160px;
}

.filter-select:focus {
  outline: none;
  border-color: var(--color-primary);
}

.filter-checkbox {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  cursor: pointer;
  user-select: none;
}

.filter-checkbox input[type="checkbox"] {
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary);
  cursor: pointer;
}

/* States */
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-2xl);
  gap: var(--spacing-md);
  color: var(--color-text-secondary);
}

.state-error {
  color: var(--color-error);
}

.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Template Grid */
.template-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-lg);
}

@media (max-width: 1024px) {
  .template-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 768px) {
  .template-grid {
    grid-template-columns: 1fr;
  }

  .filter-bar {
    flex-direction: column;
    align-items: stretch;
  }

  .filter-group {
    flex-direction: column;
  }

  .filter-select {
    width: 100%;
  }
}

/* Template Card */
.template-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  display: flex;
  flex-direction: column;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.template-card:hover {
  border-color: var(--color-primary);
  box-shadow: var(--shadow-lg);
}

.card-header {
  display: flex;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
  flex-wrap: wrap;
}

/* Badges */
.badge {
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

.badge-forex {
  background-color: #DBEAFE;
  color: #1E40AF;
}

.badge-stocks {
  background-color: #D1FAE5;
  color: #065F46;
}

.badge-mixed {
  background-color: #EDE9FE;
  color: #5B21B6;
}

:root[data-theme="dark"] .badge-crypto {
  background-color: #78350F;
  color: #FDE68A;
}

:root[data-theme="dark"] .badge-forex {
  background-color: #1E3A5F;
  color: #93C5FD;
}

:root[data-theme="dark"] .badge-stocks {
  background-color: #064E3B;
  color: #6EE7B7;
}

:root[data-theme="dark"] .badge-mixed {
  background-color: #4C1D95;
  color: #C4B5FD;
}

.badge-duration {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

.badge-rush_30min {
  background-color: #FEE2E2;
  color: #991B1B;
}

:root[data-theme="dark"] .badge-rush_30min {
  background-color: #7F1D1D;
  color: #FCA5A5;
}

.badge-hourly {
  background-color: #FEF3C7;
  color: #92400E;
}

:root[data-theme="dark"] .badge-hourly {
  background-color: #78350F;
  color: #FDE68A;
}

.badge-four_hour {
  background-color: #E0E7FF;
  color: #3730A3;
}

:root[data-theme="dark"] .badge-four_hour {
  background-color: #312E81;
  color: #A5B4FC;
}

.badge-daily {
  background-color: #DBEAFE;
  color: #1E40AF;
}

:root[data-theme="dark"] .badge-daily {
  background-color: #1E3A5F;
  color: #93C5FD;
}

.badge-weekly {
  background-color: #D1FAE5;
  color: #065F46;
}

:root[data-theme="dark"] .badge-weekly {
  background-color: #064E3B;
  color: #6EE7B7;
}

.card-name {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.card-description {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  line-height: 1.5;
  margin-bottom: var(--spacing-md);
  flex: 1;
}

/* Symbols */
.card-symbols {
  margin-bottom: var(--spacing-md);
}

.symbols-label {
  display: block;
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-xs);
}

.symbols-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}

.symbol-tag {
  display: inline-block;
  padding: 2px var(--spacing-sm);
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  color: var(--color-text-primary);
  font-weight: 500;
  font-family: monospace;
}

.symbol-more {
  color: var(--color-text-secondary);
  font-family: inherit;
  font-style: italic;
}

/* Meta Info */
.card-meta {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  padding: var(--spacing-md) 0;
  border-top: 1px solid var(--color-border);
  margin-bottom: var(--spacing-md);
}

.meta-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.meta-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.meta-value {
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  font-weight: 500;
}

.free-badge {
  background-color: #D1FAE5;
  color: #065F46;
  padding: 1px var(--spacing-sm);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

:root[data-theme="dark"] .free-badge {
  background-color: #064E3B;
  color: #6EE7B7;
}

/* Action Button */
.card-action {
  width: 100%;
}

/* Buttons */
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-lg);
  border: none;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 600;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.btn-primary {
  background-color: var(--color-primary);
  color: white;
}

.btn-primary:hover {
  opacity: 0.9;
}

.btn-secondary {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border);
}

.btn-secondary:hover {
  background-color: var(--color-bg-secondary);
}

.btn-text {
  background: none;
  color: var(--color-primary);
  padding: var(--spacing-xs) var(--spacing-sm);
}

.btn-text:hover {
  text-decoration: underline;
}

/* RTL */
[dir="rtl"] .meta-row {
  flex-direction: row-reverse;
}

[dir="rtl"] .symbols-list {
  direction: ltr;
}
</style>
