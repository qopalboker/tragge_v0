<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { api } from '@/api';
import { getSymbols } from '@/api/symbols';
import ContestTemplateSelector from '@/components/ContestTemplateSelector.vue';
import type { ContestTemplate } from '@/components/ContestTemplateSelector.vue';

const toast = useToast();

type CreateMode = 'manual' | 'template';

// Duration type to minutes mapping
const durationMinutesMap: Record<string, number> = {
  rush_30min: 30,
  hourly: 60,
  four_hour: 240,
  daily: 1440,
  weekly: 10080,
};

// ── Dynamic symbols from DB (single source of truth) ──────────────
const dbCryptoSymbols = ref<string[]>([]);
const dbForexSymbols = ref<string[]>([]);
const dbStockSymbols = ref<string[]>([]);

const CRYPTO_FALLBACK = ['BTC/USD', 'ETH/USD', 'SOL/USD', 'DOGE/USD', 'XRP/USD', 'ADA/USD', 'AVAX/USD', 'LINK/USD'];
const FOREX_FALLBACK = ['EUR/USD', 'GBP/USD', 'USD/JPY', 'USD/CHF', 'AUD/USD'];
const STOCK_FALLBACK = ['AAPL', 'MSFT', 'GOOGL', 'AMZN', 'TSLA', 'META', 'NVDA'];

async function fetchSymbolsFromDB(): Promise<void> {
  try {
    const [cryptoRes, forexRes, commodityRes, stockRes] = await Promise.all([
      getSymbols({ asset_type: 'crypto', is_active: 'true', limit: 200 }),
      getSymbols({ asset_type: 'forex', is_active: 'true', limit: 200 }),
      getSymbols({ asset_type: 'commodity', is_active: 'true', limit: 200 }),
      getSymbols({ asset_type: 'stock', is_active: 'true', limit: 200 }),
    ]);
    dbCryptoSymbols.value = cryptoRes.symbols.map(s => s.symbol);
    dbForexSymbols.value = [
      ...forexRes.symbols.map(s => s.symbol),
      ...commodityRes.symbols.map(s => s.symbol),
    ];
    dbStockSymbols.value = stockRes.symbols.map(s => s.symbol);
  } catch {
    dbCryptoSymbols.value = CRYPTO_FALLBACK;
    dbForexSymbols.value = FOREX_FALLBACK;
    dbStockSymbols.value = STOCK_FALLBACK;
  }
}

// rush_30min: top 8, hourly/4h: top 16, daily/weekly: all
const symbolSets = computed<Record<string, Record<string, string[]>>>(() => ({
  crypto: {
    rush_30min: dbCryptoSymbols.value.slice(0, 8),
    hourly:     dbCryptoSymbols.value.slice(0, 16),
    four_hour:  dbCryptoSymbols.value.slice(0, 16),
    daily:      dbCryptoSymbols.value,
    weekly:     dbCryptoSymbols.value,
  },
  forex: {
    rush_30min: dbForexSymbols.value.slice(0, 5),
    hourly:     dbForexSymbols.value.slice(0, 15),
    four_hour:  dbForexSymbols.value.slice(0, 15),
    daily:      dbForexSymbols.value,
    weekly:     dbForexSymbols.value,
  },
  stocks: {
    rush_30min: dbStockSymbols.value,
    hourly:     dbStockSymbols.value,
    four_hour:  dbStockSymbols.value,
    daily:      dbStockSymbols.value,
    weekly:     dbStockSymbols.value,
  },
  mixed: {
    rush_30min: [...dbCryptoSymbols.value.slice(0, 5), ...dbForexSymbols.value.slice(0, 5)],
    hourly:     [...dbCryptoSymbols.value.slice(0, 5), ...dbForexSymbols.value.slice(0, 5)],
    four_hour:  [...dbCryptoSymbols.value.slice(0, 5), ...dbForexSymbols.value.slice(0, 5)],
    daily:      [...dbCryptoSymbols.value.slice(0, 5), ...dbForexSymbols.value.slice(0, 5)],
    weekly:     [...dbCryptoSymbols.value.slice(0, 5), ...dbForexSymbols.value.slice(0, 5)],
  },
}));

function getDefaultSymbols(assetClass: string, durationType: string): string[] {
  return symbolSets.value[assetClass]?.[durationType] ?? [];
}

interface ContestForm {
  name: string;
  description: string;
  status: string;
  starts_at: string;
  ends_at: string;
  entry_fee_dollars: number;
  platform_fee_percent: number;
  qty_total: number;
  duration_type: string;
  asset_class: string;
  min_participants: number;
  max_participants: number | null;
  registration_deadline: string;
  auto_start: boolean;
  commission_rate: number;
  is_free: boolean;
  symbols: string;
}

interface TemplateOverrides {
  name: string;
  starts_at: string;
  description: string;
  entry_fee_cents: number | null;
  max_participants: number | null;
}

const router = useRouter();
const route = useRoute();

const isEditMode = computed(() => !!route.params.id);
const contestId = computed(() => route.params.id as string);

const createMode = ref<CreateMode>('manual');
const selectedTemplate = ref<ContestTemplate | null>(null);
const savingTemplate = ref(false);

const templateOverrides = ref<TemplateOverrides>({
  name: '',
  starts_at: '',
  description: '',
  entry_fee_cents: null,
  max_participants: null,
});

function handleTemplateSelect(tpl: ContestTemplate): void {
  selectedTemplate.value = tpl;
  templateOverrides.value = {
    name: '',
    starts_at: '',
    description: '',
    entry_fee_cents: null,
    max_participants: null,
  };
}

function handleChangeTemplate(): void {
  selectedTemplate.value = null;
}

function validateTemplateForm(): boolean {
  const errors: Record<string, string> = {};

  if (!templateOverrides.value.name.trim()) {
    errors.name = t('contestForm.validation.nameRequired');
  }
  if (!templateOverrides.value.starts_at) {
    errors.starts_at = t('contestForm.validation.startsAtRequired');
  } else if (new Date(templateOverrides.value.starts_at) <= new Date()) {
    errors.starts_at = t('contestForm.validation.startsAtFuture');
  }

  validationErrors.value = errors;
  return Object.keys(errors).length === 0;
}

async function handleTemplateSubmit(): Promise<void> {
  if (!selectedTemplate.value || !validateTemplateForm()) return;

  savingTemplate.value = true;
  try {
    const payload: Record<string, unknown> = {
      template_key: selectedTemplate.value.key,
      name: templateOverrides.value.name.trim(),
      starts_at: new Date(templateOverrides.value.starts_at).toISOString(),
    };

    if (templateOverrides.value.description.trim()) {
      payload.description = templateOverrides.value.description.trim();
    }
    if (templateOverrides.value.entry_fee_cents !== null && templateOverrides.value.entry_fee_cents >= 0) {
      payload.entry_fee_cents = templateOverrides.value.entry_fee_cents;
    }
    if (templateOverrides.value.max_participants !== null && templateOverrides.value.max_participants > 0) {
      payload.max_participants = templateOverrides.value.max_participants;
    }

    await api.post('/api/admin/contests/from-template', payload);
    toast.success(t('contestForm.templates.createSuccess'));

    setTimeout(() => {
      router.push({ name: 'admin-contests' });
    }, 1500);
  } catch {
    toast.error(t('contestForm.templates.createError'));
  } finally {
    savingTemplate.value = false;
  }
}

const form = ref<ContestForm>({
  name: '',
  description: '',
  status: 'draft',
  starts_at: '',
  ends_at: '',
  entry_fee_dollars: 5,
  platform_fee_percent: 15,
  qty_total: 100000,
  duration_type: 'hourly',
  asset_class: 'mixed',
  min_participants: 2,
  max_participants: null,
  registration_deadline: '',
  auto_start: true,
  commission_rate: 0,
  is_free: false,
  symbols: '',
});

const loading = ref(false);
const saving = ref(false);
const validationErrors = ref<Record<string, string>>({});

const statuses = ['draft', 'scheduled', 'registration_open', 'running', 'paused', 'completed', 'cancelled'];
const durationTypes = ['rush_30min', 'hourly', 'four_hour', 'daily', 'weekly'];
const assetClasses = ['crypto', 'forex', 'stocks', 'mixed'];

// Computed end time based on starts_at + duration_type
const computedEndsAt = computed(() => {
  if (!form.value.starts_at || !form.value.duration_type) return '';
  const start = new Date(form.value.starts_at);
  const minutes = durationMinutesMap[form.value.duration_type] || 60;
  const end = new Date(start.getTime() + minutes * 60000);
  return end.toLocaleString();
});

// Computed symbols based on asset_class + duration_type
const computedSymbols = computed(() => {
  return getDefaultSymbols(form.value.asset_class, form.value.duration_type);
});

watch(() => form.value.is_free, (isFree) => {
  if (isFree) {
    form.value.entry_fee_dollars = 0;
    form.value.platform_fee_percent = 0;
    form.value.commission_rate = 0;
  }
});

function validate(): boolean {
  const errors: Record<string, string> = {};
  const now = new Date();

  if (!form.value.name.trim()) {
    errors.name = t('contestForm.validation.nameRequired');
  }

  if (!form.value.starts_at) {
    errors.starts_at = t('contestForm.validation.startsAtRequired');
  } else if (!isEditMode.value && new Date(form.value.starts_at) <= now) {
    errors.starts_at = t('contestForm.validation.startsAtFuture');
  }

  if (!form.value.is_free && (form.value.entry_fee_dollars < 1 || form.value.entry_fee_dollars > 50)) {
    errors.entry_fee_dollars = t('contestForm.validation.entryFeeRange');
  }

  if (form.value.qty_total <= 0) {
    errors.qty_total = t('contestForm.validation.qtyTotalRequired');
  }

  validationErrors.value = errors;
  return Object.keys(errors).length === 0;
}

function buildCreatePayload() {
  const payload: Record<string, unknown> = {
    name: form.value.name.trim(),
    starts_at: new Date(form.value.starts_at).toISOString(),
    entry_fee_cents: Math.round(form.value.entry_fee_dollars * 100),
    platform_fee_bps: Math.round(form.value.platform_fee_percent * 100),
    qty_total: Math.round(form.value.qty_total),
    status: form.value.status,
    duration_type: form.value.duration_type,
    asset_class: form.value.asset_class,
    min_participants: form.value.min_participants,
    auto_start: form.value.auto_start,
    is_free: form.value.is_free,
  };

  if (form.value.description.trim()) {
    payload.description = form.value.description.trim();
  }
  if (form.value.max_participants && form.value.max_participants > 0) {
    payload.max_participants = form.value.max_participants;
  }
  if (form.value.commission_rate > 0) {
    payload.commission_rate = form.value.commission_rate;
  }

  return payload;
}

function buildUpdatePayload() {
  const payload: Record<string, unknown> = {
    name: form.value.name.trim(),
    starts_at: new Date(form.value.starts_at).toISOString(),
    entry_fee_cents: Math.round(form.value.entry_fee_dollars * 100),
    platform_fee_bps: Math.round(form.value.platform_fee_percent * 100),
    qty_total: Math.round(form.value.qty_total),
    status: form.value.status,
    duration_type: form.value.duration_type,
    asset_class: form.value.asset_class,
    min_participants: form.value.min_participants,
    auto_start: form.value.auto_start,
    is_free: form.value.is_free,
  };

  if (form.value.description.trim()) {
    payload.description = form.value.description.trim();
  }
  if (form.value.max_participants && form.value.max_participants > 0) {
    payload.max_participants = form.value.max_participants;
  }
  if (form.value.commission_rate > 0) {
    payload.commission_rate = form.value.commission_rate;
  }

  return payload;
}

async function fetchContest(): Promise<void> {
  if (!isEditMode.value) return;

  loading.value = true;
  try {
    const response = await api.get(`/api/admin/contests/${contestId.value}`);
    const c = response.data;
    form.value = {
      name: c.name || '',
      description: c.description || '',
      status: c.status || 'draft',
      starts_at: c.starts_at ? c.starts_at.slice(0, 16) : '',
      ends_at: c.ends_at ? c.ends_at.slice(0, 16) : '',
      entry_fee_dollars: (c.entry_fee_cents || 0) / 100,
      platform_fee_percent: (c.platform_fee_bps || 0) / 100,
      qty_total: c.qty_total || 100000,
      duration_type: c.duration_type || 'hourly',
      asset_class: c.asset_class || 'mixed',
      min_participants: c.min_participants || 2,
      max_participants: c.max_participants || null,
      registration_deadline: c.registration_deadline ? c.registration_deadline.slice(0, 16) : '',
      auto_start: c.auto_start ?? true,
      commission_rate: c.commission_rate || 0,
      is_free: c.is_free || false,
      symbols: (c.symbols || []).join(', '),
    };
  } catch {
    toast.error(t('contestForm.loadError'));
  } finally {
    loading.value = false;
  }
}

async function handleSubmit(): Promise<void> {
  if (!validate()) return;

  saving.value = true;

  try {
    if (isEditMode.value) {
      await api.patch(`/api/admin/contests/${contestId.value}`, buildUpdatePayload());
      toast.success(t('contestForm.updateSuccess'));
    } else {
      await api.post('/api/admin/contests', buildCreatePayload());
      toast.success(t('contestForm.createSuccess'));
    }

    setTimeout(() => {
      router.push({ name: 'admin-contests' });
    }, 1500);
  } catch {
    toast.error(t('contestForm.error'));
  } finally {
    saving.value = false;
  }
}

function handleCancel(): void {
  router.push({ name: 'admin-contests' });
}

onMounted(() => {
  fetchSymbolsFromDB();
  fetchContest();
});
</script>

<template>
  <div class="contest-form-page">
    <div class="page-header">
      <h1 class="page-title">
        {{ isEditMode ? t('contestForm.editTitle') : t('contestForm.createTitle') }}
      </h1>
    </div>

    <!-- Mode Toggle (only for create mode) -->
    <div v-if="!isEditMode" class="mode-toggle">
      <button
        type="button"
        class="mode-btn"
        :class="{ active: createMode === 'manual' }"
        @click="createMode = 'manual'"
      >
        {{ t('contestForm.modeManual') }}
      </button>
      <button
        type="button"
        class="mode-btn"
        :class="{ active: createMode === 'template' }"
        @click="createMode = 'template'"
      >
        {{ t('contestForm.modeTemplate') }}
      </button>
    </div>

    <!-- Template Mode -->
    <div v-if="!isEditMode && createMode === 'template'">
      <div v-if="!selectedTemplate">
        <ContestTemplateSelector @select="handleTemplateSelect" />
      </div>

      <div v-else class="form-card">
        <!-- Selected template banner -->
        <div class="template-selected-banner">
          <div class="template-selected-info">
            <span class="template-selected-label">{{ t('contestForm.templates.selected') }}</span>
            <strong>{{ selectedTemplate.name }}</strong>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" @click="handleChangeTemplate">
            {{ t('contestForm.templates.change') }}
          </button>
        </div>

        <!-- Override form -->
        <form @submit.prevent="handleTemplateSubmit">
          <fieldset class="form-section">
            <legend class="section-title">{{ t('contestForm.templates.overrides') }}</legend>
            <p class="overrides-hint">{{ t('contestForm.templates.overridesHint') }}</p>
            <div class="form-grid">
              <div class="form-group">
                <label class="form-label" for="tpl-name">{{ t('contestForm.name') }} *</label>
                <input
                  id="tpl-name"
                  v-model="templateOverrides.name"
                  type="text"
                  class="input"
                  :class="{ 'input-error': validationErrors.name }"
                  :placeholder="t('contestForm.namePlaceholder')"
                  required
                />
                <span v-if="validationErrors.name" class="field-error">{{ validationErrors.name }}</span>
              </div>

              <div class="form-group">
                <label class="form-label" for="tpl-starts_at">{{ t('contestForm.startsAt') }} *</label>
                <input
                  id="tpl-starts_at"
                  v-model="templateOverrides.starts_at"
                  type="datetime-local"
                  class="input"
                  :class="{ 'input-error': validationErrors.starts_at }"
                  required
                />
                <span v-if="validationErrors.starts_at" class="field-error">{{ validationErrors.starts_at }}</span>
              </div>

              <div class="form-group full-width">
                <label class="form-label" for="tpl-description">{{ t('contestForm.description') }}</label>
                <textarea
                  id="tpl-description"
                  v-model="templateOverrides.description"
                  class="input textarea"
                  :placeholder="t('contestForm.descriptionPlaceholder')"
                  rows="3"
                />
              </div>

              <div class="form-group">
                <label class="form-label" for="tpl-entry_fee">{{ t('contestForm.entryFee') }} (cents)</label>
                <input
                  id="tpl-entry_fee"
                  v-model.number="templateOverrides.entry_fee_cents"
                  type="number"
                  class="input"
                  min="0"
                  step="1"
                  :placeholder="t('contestForm.entryFeeHint')"
                />
              </div>

              <div class="form-group">
                <label class="form-label" for="tpl-max_participants">{{ t('contestForm.maxParticipants') }}</label>
                <input
                  id="tpl-max_participants"
                  v-model.number="templateOverrides.max_participants"
                  type="number"
                  class="input"
                  min="1"
                  :placeholder="t('contestForm.maxParticipantsPlaceholder')"
                />
              </div>

              <div class="form-group full-width">
                <span class="field-hint">{{ t('contestForm.registrationAutoInfo') }}</span>
              </div>
            </div>
          </fieldset>

          <div class="form-actions">
            <button type="button" class="btn btn-secondary" @click="handleCancel">
              {{ t('contestForm.cancel') }}
            </button>
            <button type="submit" class="btn btn-primary" :disabled="savingTemplate">
              <span v-if="savingTemplate" class="spinner" />
              {{ savingTemplate ? t('contestForm.saving') : t('contestForm.save') }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <div v-if="loading" class="loading">
      {{ t('common.loading') }}
    </div>

    <form v-else-if="isEditMode || createMode === 'manual'" class="form-card" @submit.prevent="handleSubmit">

      <!-- Basic Info -->
      <fieldset class="form-section">
        <legend class="section-title">{{ t('contestForm.sections.basicInfo') }}</legend>
        <div class="form-grid">
          <div class="form-group">
            <label class="form-label" for="name">{{ t('contestForm.name') }} *</label>
            <input
              id="name"
              v-model="form.name"
              type="text"
              class="input"
              :class="{ 'input-error': validationErrors.name }"
              :placeholder="t('contestForm.namePlaceholder')"
              required
            />
            <span v-if="validationErrors.name" class="field-error">{{ validationErrors.name }}</span>
          </div>

          <div class="form-group">
            <label class="form-label" for="status">{{ t('contestForm.status') }}</label>
            <select id="status" v-model="form.status" class="input">
              <option v-for="status in statuses" :key="status" :value="status">
                {{ t(`status.${status}`) }}
              </option>
            </select>
          </div>

          <div class="form-group full-width">
            <label class="form-label" for="description">{{ t('contestForm.description') }}</label>
            <textarea
              id="description"
              v-model="form.description"
              class="input textarea"
              :placeholder="t('contestForm.descriptionPlaceholder')"
              rows="3"
            />
          </div>

          <div class="form-group">
            <label class="form-label" for="duration_type">{{ t('contestForm.durationType') }}</label>
            <select id="duration_type" v-model="form.duration_type" class="input">
              <option v-for="dt in durationTypes" :key="dt" :value="dt">
                {{ t(`contestForm.durationTypes.${dt}`) }}
              </option>
            </select>
          </div>

          <div class="form-group">
            <label class="form-label" for="asset_class">{{ t('contestForm.assetClass') }}</label>
            <select id="asset_class" v-model="form.asset_class" class="input">
              <option v-for="ac in assetClasses" :key="ac" :value="ac">
                {{ t(`contestForm.assetClasses.${ac}`) }}
              </option>
            </select>
          </div>
        </div>
      </fieldset>

      <!-- Schedule -->
      <fieldset class="form-section">
        <legend class="section-title">{{ t('contestForm.sections.schedule') }}</legend>
        <div class="form-grid">
          <div class="form-group">
            <label class="form-label" for="starts_at">{{ t('contestForm.startsAt') }} *</label>
            <input
              id="starts_at"
              v-model="form.starts_at"
              type="datetime-local"
              class="input"
              :class="{ 'input-error': validationErrors.starts_at }"
              required
            />
            <span v-if="validationErrors.starts_at" class="field-error">{{ validationErrors.starts_at }}</span>
          </div>

          <div class="form-group">
            <label class="form-label">{{ t('contestForm.computedEndsAt') }}</label>
            <div class="computed-value">{{ computedEndsAt || '—' }}</div>
          </div>

          <div class="form-group">
            <span class="field-hint">{{ t('contestForm.registrationAutoInfo') }}</span>
          </div>

          <div class="form-group">
            <label class="form-label toggle-label">
              <span>{{ t('contestForm.autoStart') }}</span>
              <button
                type="button"
                class="toggle"
                :class="{ active: form.auto_start }"
                @click="form.auto_start = !form.auto_start"
                role="switch"
                :aria-checked="form.auto_start"
              >
                <span class="toggle-knob" />
              </button>
            </label>
          </div>
        </div>
      </fieldset>

      <!-- Pricing -->
      <fieldset class="form-section">
        <legend class="section-title">{{ t('contestForm.sections.pricing') }}</legend>
        <div class="form-grid">
          <div class="form-group">
            <label class="form-label toggle-label">
              <span>{{ t('contestForm.isFree') }}</span>
              <button
                type="button"
                class="toggle"
                :class="{ active: form.is_free }"
                @click="form.is_free = !form.is_free"
                role="switch"
                :aria-checked="form.is_free"
              >
                <span class="toggle-knob" />
              </button>
            </label>
          </div>

          <div class="form-group">
            <label class="form-label" for="entry_fee_dollars">{{ t('contestForm.entryFee') }} ($)</label>
            <input
              id="entry_fee_dollars"
              v-model.number="form.entry_fee_dollars"
              type="number"
              class="input"
              :class="{ 'input-error': validationErrors.entry_fee_dollars }"
              min="1"
              max="50"
              step="1"
              :disabled="form.is_free"
            />
            <span v-if="validationErrors.entry_fee_dollars" class="field-error">{{ validationErrors.entry_fee_dollars }}</span>
            <span v-else class="field-hint">{{ t('contestForm.entryFeeHint') }}</span>
          </div>

          <div class="form-group">
            <label class="form-label" for="platform_fee_percent">{{ t('contestForm.platformFee') }} (%)</label>
            <input
              id="platform_fee_percent"
              v-model.number="form.platform_fee_percent"
              type="number"
              class="input"
              min="0"
              max="100"
              step="0.01"
            />
            <span class="field-hint">{{ t('contestForm.platformFeeHint') }}</span>
          </div>

          <div class="form-group">
            <label class="form-label" for="qty_total">{{ t('contestForm.qtyTotal') }} ($) *</label>
            <input
              id="qty_total"
              v-model.number="form.qty_total"
              type="number"
              class="input"
              :class="{ 'input-error': validationErrors.qty_total }"
              min="1"
              step="1"
              required
            />
            <span v-if="validationErrors.qty_total" class="field-error">{{ validationErrors.qty_total }}</span>
            <span v-else class="field-hint">{{ t('contestForm.qtyTotalHint') }}</span>
          </div>

          <div class="form-group">
            <label class="form-label" for="commission_rate">{{ t('contestForm.commissionRate') }} (%)</label>
            <input
              id="commission_rate"
              v-model.number="form.commission_rate"
              type="number"
              class="input"
              min="0"
              max="50"
              step="0.01"
            />
          </div>
        </div>
      </fieldset>

      <!-- Participants -->
      <fieldset class="form-section">
        <legend class="section-title">{{ t('contestForm.sections.participants') }}</legend>
        <div class="form-grid">
          <div class="form-group">
            <label class="form-label" for="min_participants">{{ t('contestForm.minParticipants') }}</label>
            <input
              id="min_participants"
              v-model.number="form.min_participants"
              type="number"
              class="input"
              min="1"
            />
          </div>

          <div class="form-group">
            <label class="form-label" for="max_participants">{{ t('contestForm.maxParticipants') }}</label>
            <input
              id="max_participants"
              v-model.number="form.max_participants"
              type="number"
              class="input"
              min="1"
              :placeholder="t('contestForm.maxParticipantsPlaceholder')"
            />
          </div>

        </div>
      </fieldset>

      <!-- Symbols (auto-assigned, read-only) -->
      <fieldset class="form-section">
        <legend class="section-title">{{ t('contestForm.autoSymbolsLabel') }}</legend>
        <div class="form-grid">
          <div class="form-group full-width">
            <div class="symbol-tags">
              <span v-for="symbol in computedSymbols" :key="symbol" class="symbol-tag">{{ symbol }}</span>
            </div>
            <span v-if="computedSymbols.length === 0" class="field-hint">{{ t('contestForm.noSymbols') }}</span>
          </div>
        </div>
      </fieldset>

      <div class="form-actions">
        <button type="button" class="btn btn-secondary" @click="handleCancel">
          {{ t('contestForm.cancel') }}
        </button>
        <button type="submit" class="btn btn-primary" :disabled="saving">
          <span v-if="saving" class="spinner" />
          {{ saving ? t('contestForm.saving') : t('contestForm.save') }}
        </button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.contest-form-page {
  padding: var(--spacing-lg) 0;
  max-width: 800px;
}

.mode-toggle {
  display: inline-flex;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-md);
  padding: 3px;
  margin-bottom: var(--spacing-lg);
}

.mode-btn {
  padding: var(--spacing-sm) var(--spacing-lg);
  font-size: var(--font-size-sm);
  font-weight: 500;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  background: transparent;
  color: var(--color-text-secondary);
  transition: all var(--transition-fast);
}

.mode-btn.active {
  background-color: var(--color-bg-primary);
  color: var(--color-text-primary);
  box-shadow: var(--shadow-sm);
}

.template-selected-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md);
  background-color: var(--color-primary-light);
  border-radius: var(--radius-md);
  margin-bottom: var(--spacing-xl);
}

.template-selected-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.template-selected-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
}

.overrides-hint {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-md);
}

.page-header {
  margin-bottom: var(--spacing-xl);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

.loading {
  text-align: center;
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.form-card {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  padding: var(--spacing-xl);
}

.form-section {
  border: none;
  padding: 0;
  margin: 0 0 var(--spacing-xl) 0;
}

.form-section:last-of-type {
  margin-bottom: 0;
}

.section-title {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: var(--spacing-md);
  padding-bottom: var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-md);
}

.form-group {
  display: flex;
  flex-direction: column;
}

.form-group.full-width {
  grid-column: 1 / -1;
}

.form-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.textarea {
  resize: vertical;
  min-height: 80px;
}

.input-error {
  border-color: var(--color-error, #ef4444);
}

.field-error {
  font-size: var(--font-size-xs);
  color: var(--color-error, #ef4444);
  margin-top: 4px;
}

.field-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  margin-top: 4px;
}

.computed-value {
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border);
}

.symbol-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}

.symbol-tag {
  padding: 2px 8px;
  background-color: var(--color-bg-tertiary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-family: monospace;
  color: var(--color-text-primary);
}

/* Toggle switch */
.toggle-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  cursor: pointer;
  height: 100%;
  padding-top: var(--spacing-sm);
}

.toggle {
  position: relative;
  width: 44px;
  height: 24px;
  border-radius: 12px;
  border: none;
  background: var(--color-border);
  cursor: pointer;
  padding: 0;
  transition: background-color var(--transition-fast);
  flex-shrink: 0;
}

.toggle.active {
  background: var(--color-primary);
}

.toggle-knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: white;
  transition: transform var(--transition-fast);
  pointer-events: none;
}

.toggle.active .toggle-knob {
  transform: translateX(20px);
}

[dir="rtl"] .toggle-knob {
  left: auto;
  right: 2px;
}

[dir="rtl"] .toggle.active .toggle-knob {
  transform: translateX(-20px);
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-md);
  margin-top: var(--spacing-xl);
  padding-top: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid transparent;
  border-top-color: currentColor;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  margin-right: var(--spacing-sm);
}

[dir="rtl"] .spinner {
  margin-right: 0;
  margin-left: var(--spacing-sm);
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 767px) {
  .form-grid {
    grid-template-columns: 1fr;
  }

  .form-actions {
    flex-direction: column-reverse;
  }

  .form-actions .btn {
    width: 100%;
  }
}
</style>
