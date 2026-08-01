<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { t } from '@/i18n';
import { listTiers, createTier, updateTier, deleteTier, type EntryTier, type CreateTierRequest } from '@/api/tiers';

const props = defineProps<{
  templateId: string;
}>();

const tiers = ref<EntryTier[]>([]);
const loading = ref(false);
const error = ref<string | null>(null);
const showAddForm = ref(false);

// New tier form
const newTier = ref<CreateTierRequest>({
  entry_fee: 0,
  label: '',
  sort_order: 0,
  is_free: false,
});

async function fetchTiers(): Promise<void> {
  loading.value = true;
  error.value = null;
  try {
    const result = await listTiers(props.templateId);
    tiers.value = result.tiers || [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : t('common.error');
  } finally {
    loading.value = false;
  }
}

async function handleAddTier(): Promise<void> {
  try {
    await createTier(props.templateId, newTier.value);
    showAddForm.value = false;
    newTier.value = { entry_fee: 0, label: '', sort_order: tiers.value.length, is_free: false };
    await fetchTiers();
  } catch (err) {
    error.value = err instanceof Error ? err.message : t('common.error');
  }
}

async function handleToggleActive(tier: EntryTier): Promise<void> {
  try {
    if (tier.is_active) {
      await deleteTier(tier.id);
    } else {
      await updateTier(tier.id, { is_active: true });
    }
    await fetchTiers();
  } catch (err) {
    error.value = err instanceof Error ? err.message : t('common.error');
  }
}

function formatFee(fee: number): string {
  if (fee === 0) return t('tiers.free');
  return `${(fee / 10).toLocaleString()} تومان`;
}

onMounted(fetchTiers);
</script>

<template>
  <div class="tier-list">
    <div class="tier-header">
      <h3>{{ t('tiers.title') }}</h3>
      <button class="btn btn-sm btn-primary" @click="showAddForm = !showAddForm">
        {{ showAddForm ? t('common.cancel') : t('tiers.addTier') }}
      </button>
    </div>

    <!-- Error -->
    <div v-if="error" class="alert alert-danger">{{ error }}</div>

    <!-- Add Tier Form -->
    <div v-if="showAddForm" class="tier-form">
      <div class="form-row">
        <div class="form-group">
          <label>{{ t('tiers.entryFee') }}</label>
          <input v-model.number="newTier.entry_fee" type="number" min="0" class="input" />
        </div>
        <div class="form-group">
          <label>{{ t('tiers.label') }}</label>
          <input v-model="newTier.label" type="text" class="input" placeholder="مثلاً برنزی، نقره‌ای، طلایی" />
        </div>
        <div class="form-group">
          <label>{{ t('tiers.sortOrder') }}</label>
          <input v-model.number="newTier.sort_order" type="number" min="0" class="input" />
        </div>
        <div class="form-group form-check">
          <label>
            <input v-model="newTier.is_free" type="checkbox" />
            {{ t('tiers.isFree') }}
          </label>
        </div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label>{{ t('tiers.qtyOverride') }}</label>
          <input v-model.number="newTier.qty_total_override" type="number" min="0" class="input" placeholder="پیش‌فرض از قالب" />
        </div>
        <div class="form-group">
          <label>{{ t('tiers.maxParticipantsOverride') }}</label>
          <input v-model.number="newTier.max_participants_override" type="number" min="0" class="input" placeholder="پیش‌فرض از قالب" />
        </div>
        <div class="form-group">
          <label>{{ t('tiers.commissionOverride') }}</label>
          <input v-model.number="newTier.commission_rate_override" type="number" min="0" max="50" step="0.01" class="input" placeholder="پیش‌فرض از قالب" />
        </div>
      </div>
      <button class="btn btn-primary" @click="handleAddTier">{{ t('tiers.create') }}</button>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="tier-loading">{{ t('common.loading') }}</div>

    <!-- Tier Table -->
    <table v-else-if="tiers.length > 0" class="tier-table">
      <thead>
        <tr>
          <th>{{ t('tiers.order') }}</th>
          <th>{{ t('tiers.label') }}</th>
          <th>{{ t('tiers.entryFee') }}</th>
          <th>{{ t('tiers.qtyOverride') }}</th>
          <th>{{ t('tiers.status') }}</th>
          <th>{{ t('tiers.actions') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="tier in tiers" :key="tier.id" :class="{ 'tier-inactive': !tier.is_active }">
          <td>{{ tier.sort_order }}</td>
          <td>
            <span v-if="tier.label">{{ tier.label }}</span>
            <span v-else class="text-muted">{{ t('tiers.noLabel') }}</span>
            <span v-if="tier.is_free" class="badge badge-info">{{ t('tiers.free') }}</span>
          </td>
          <td>{{ formatFee(tier.entry_fee) }}</td>
          <td>
            <span v-if="tier.qty_total_override">{{ tier.qty_total_override.toLocaleString() }}</span>
            <span v-else class="text-muted">-</span>
          </td>
          <td>
            <span :class="['badge', tier.is_active ? 'badge-success' : 'badge-secondary']">
              {{ tier.is_active ? t('common.active') : t('common.inactive') }}
            </span>
          </td>
          <td>
            <button class="btn btn-sm" :class="tier.is_active ? 'btn-danger' : 'btn-success'" @click="handleToggleActive(tier)">
              {{ tier.is_active ? t('tiers.deactivate') : t('tiers.activate') }}
            </button>
          </td>
        </tr>
      </tbody>
    </table>

    <!-- Empty State -->
    <div v-else class="tier-empty">
      <p>{{ t('tiers.empty') }}</p>
    </div>
  </div>
</template>

<style scoped>
.tier-list {
  margin-top: var(--spacing-lg);
}

.tier-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-md);
}

.tier-header h3 {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.tier-form {
  background: var(--color-bg-secondary);
  padding: var(--spacing-md);
  border-radius: var(--radius-md);
  margin-bottom: var(--spacing-md);
  border: 1px solid var(--color-border);
}

.form-row {
  display: flex;
  gap: var(--spacing-md);
  flex-wrap: wrap;
  margin-bottom: var(--spacing-md);
}

.form-group {
  flex: 1;
  min-width: 150px;
}

.form-group label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-xs);
}

.form-check label {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  cursor: pointer;
}

.tier-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--font-size-sm);
}

.tier-table th,
.tier-table td {
  padding: var(--spacing-sm) var(--spacing-md);
  text-align: left;
  border-bottom: 1px solid var(--color-border);
}

.tier-table th {
  font-weight: 600;
  color: var(--color-text-secondary);
  background: var(--color-bg-secondary);
}

.tier-inactive {
  opacity: 0.5;
}

.tier-empty {
  text-align: center;
  padding: var(--spacing-xl);
  color: var(--color-text-muted);
}

.tier-loading {
  text-align: center;
  padding: var(--spacing-lg);
  color: var(--color-text-muted);
}

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
  margin-left: var(--spacing-xs);
}

.text-muted {
  color: var(--color-text-muted);
}

.alert-danger {
  background: #FEE2E2;
  color: #DC2626;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  margin-bottom: var(--spacing-md);
}
</style>
