<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';
import type { Shard, ShardStatus } from '@/api/shards';

interface Props {
  shard: Shard;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  (e: 'drain', shardId: number): void;
  (e: 'activate', shardId: number): void;
}>();

const statusClass = computed(() => {
  const statusClasses: Record<ShardStatus, string> = {
    active: 'status-active',
    draining: 'status-draining',
    inactive: 'status-inactive',
  };
  return statusClasses[props.shard.status] || 'status-inactive';
});

const canDrain = computed(() => props.shard.status === 'active');
const canActivate = computed(() => props.shard.status === 'inactive');

function handleDrain(): void {
  if (canDrain.value) {
    emit('drain', props.shard.id);
  }
}

function handleActivate(): void {
  if (canActivate.value) {
    emit('activate', props.shard.id);
  }
}
</script>

<template>
  <div class="shard-card">
    <div class="shard-header">
      <div class="shard-id">
        <span class="shard-label">{{ t('shards.shard') }}</span>
        <span class="shard-number">#{{ shard.id }}</span>
      </div>
      <span :class="['status-badge', statusClass]">
        {{ t(`shards.status.${shard.status}`) }}
      </span>
    </div>

    <div class="shard-stats">
      <div class="stat-item">
        <span class="stat-value">{{ shard.contest_count }}</span>
        <span class="stat-label">{{ t('shards.contests') }}</span>
      </div>
      <div class="stat-item">
        <span class="stat-value">{{ shard.participant_count }}</span>
        <span class="stat-label">{{ t('shards.participants') }}</span>
      </div>
      <div class="stat-item">
        <span class="stat-value">{{ shard.orders_per_sec.toFixed(1) }}</span>
        <span class="stat-label">{{ t('shards.ordersPerSec') }}</span>
      </div>
    </div>

    <div class="shard-actions">
      <button
        v-if="canDrain"
        class="btn btn-warning btn-sm"
        @click="handleDrain"
      >
        {{ t('shards.drain') }}
      </button>
      <button
        v-if="canActivate"
        class="btn btn-success btn-sm"
        @click="handleActivate"
      >
        {{ t('shards.activate') }}
      </button>
      <button
        v-if="shard.status === 'draining'"
        class="btn btn-ghost btn-sm"
        disabled
      >
        {{ t('shards.draining') }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.shard-card {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  transition: box-shadow var(--transition-fast);
}

.shard-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.shard-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.shard-id {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-xs);
}

.shard-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.shard-number {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--color-text-primary);
}

.status-badge {
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
}

.status-active {
  background-color: #DCFCE7;
  color: #16A34A;
}

.status-draining {
  background-color: #FEF3C7;
  color: #D97706;
}

.status-inactive {
  background-color: #F3F4F6;
  color: #6B7280;
}

.shard-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-sm);
  padding: var(--spacing-md) 0;
  border-top: 1px solid var(--color-border);
  border-bottom: 1px solid var(--color-border);
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}

.stat-value {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
}

.stat-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  margin-top: var(--spacing-xs);
}

.shard-actions {
  display: flex;
  gap: var(--spacing-sm);
  justify-content: flex-end;
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-md);
  font-size: var(--font-size-sm);
}

.btn-warning {
  background-color: #FEF3C7;
  color: #D97706;
  border: 1px solid #FCD34D;
}

.btn-warning:hover:not(:disabled) {
  background-color: #FDE68A;
}

.btn-success {
  background-color: #DCFCE7;
  color: #16A34A;
  border: 1px solid #86EFAC;
}

.btn-success:hover:not(:disabled) {
  background-color: #BBF7D0;
}
</style>
