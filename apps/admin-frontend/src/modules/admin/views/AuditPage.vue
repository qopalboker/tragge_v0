<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { t } from '@/i18n';
import { api } from '@/api';

interface AuditLog {
  id: string;
  action: string;
  user_id: string;
  user_email?: string;
  entity_type: string;
  entity_id: string;
  details?: Record<string, unknown>;
  ip_address?: string;
  created_at: string;
}

const auditLogs = ref<AuditLog[]>([]);
const loading = ref(true);
const searchQuery = ref('');

const filteredLogs = computed(() => {
  if (!searchQuery.value) return auditLogs.value;

  const query = searchQuery.value.toLowerCase();
  return auditLogs.value.filter(log =>
    log.action.toLowerCase().includes(query) ||
    log.user_email?.toLowerCase().includes(query) ||
    log.entity_type.toLowerCase().includes(query) ||
    log.entity_id.toLowerCase().includes(query)
  );
});

async function fetchAuditLogs(): Promise<void> {
  loading.value = true;

  try {
    const response = await api.get<{ logs: AuditLog[] }>('/api/admin/audit');
    auditLogs.value = response.data.logs || [];
  } catch {
    // Use mock data for development
    auditLogs.value = [
      { id: '1', action: 'contest.create', user_id: 'u1', user_email: 'admin@example.com', entity_type: 'contest', entity_id: 'c1', ip_address: '192.168.1.1', created_at: '2026-01-04T10:30:00Z' },
      { id: '2', action: 'contest.update', user_id: 'u1', user_email: 'admin@example.com', entity_type: 'contest', entity_id: 'c1', ip_address: '192.168.1.1', created_at: '2026-01-04T10:35:00Z' },
      { id: '3', action: 'user.role.assign', user_id: 'u1', user_email: 'admin@example.com', entity_type: 'user', entity_id: 'u2', ip_address: '192.168.1.1', created_at: '2026-01-04T09:00:00Z' },
      { id: '4', action: 'contest.delete', user_id: 'u2', user_email: 'moderator@example.com', entity_type: 'contest', entity_id: 'c2', ip_address: '192.168.1.2', created_at: '2026-01-03T15:20:00Z' },
      { id: '5', action: 'user.ban', user_id: 'u1', user_email: 'admin@example.com', entity_type: 'user', entity_id: 'u5', ip_address: '192.168.1.1', created_at: '2026-01-03T14:00:00Z' },
    ];
  } finally {
    loading.value = false;
  }
}

function formatTimestamp(dateString: string): string {
  const date = new Date(dateString);
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString()}`;
}

function getActionClass(action: string): string {
  if (action.includes('delete') || action.includes('ban')) {
    return 'action-danger';
  }
  if (action.includes('create')) {
    return 'action-success';
  }
  if (action.includes('update')) {
    return 'action-warning';
  }
  return 'action-default';
}

onMounted(fetchAuditLogs);
</script>

<template>
  <div class="audit-page">
    <div class="page-header">
      <h1 class="page-title">{{ t('audit.title') }}</h1>
    </div>

    <div class="filters">
      <input
        v-model="searchQuery"
        type="text"
        class="input search-input"
        :placeholder="t('audit.search')"
      />
    </div>

    <div v-if="loading" class="loading">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="filteredLogs.length === 0" class="no-results">
      {{ t('audit.noResults') }}
    </div>

    <div v-else class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('audit.timestamp') }}</th>
            <th>{{ t('audit.action') }}</th>
            <th>{{ t('audit.user') }}</th>
            <th>{{ t('audit.entity') }}</th>
            <th>{{ t('audit.entityId') }}</th>
            <th>{{ t('audit.ipAddress') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="log in filteredLogs" :key="log.id">
            <td class="timestamp-cell">{{ formatTimestamp(log.created_at) }}</td>
            <td>
              <span :class="['action-badge', getActionClass(log.action)]">
                {{ log.action }}
              </span>
            </td>
            <td>{{ log.user_email || log.user_id }}</td>
            <td>{{ log.entity_type }}</td>
            <td class="id-cell">{{ log.entity_id }}</td>
            <td class="ip-cell">{{ log.ip_address || '-' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.audit-page {
  padding: var(--spacing-lg) 0;
}

.page-header {
  margin-bottom: var(--spacing-xl);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

.filters {
  display: flex;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.search-input {
  flex: 1;
  max-width: 400px;
}

.loading,
.no-results {
  text-align: center;
  padding: var(--spacing-2xl);
  color: var(--color-text-secondary);
}

.table-container {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: var(--spacing-md);
  text-align: left;
  border-bottom: 1px solid var(--color-border);
}

[dir="rtl"] .data-table th,
[dir="rtl"] .data-table td {
  text-align: right;
}

.data-table th {
  background-color: var(--color-bg-secondary);
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.data-table tbody tr:hover {
  background-color: var(--color-bg-secondary);
}

.timestamp-cell {
  white-space: nowrap;
  font-size: var(--font-size-sm);
}

.id-cell,
.ip-cell {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.action-badge {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: 500;
  font-family: var(--font-family-mono);
}

.action-success {
  background-color: #DCFCE7;
  color: #16A34A;
}

.action-warning {
  background-color: #FEF3C7;
  color: #D97706;
}

.action-danger {
  background-color: #FEE2E2;
  color: #DC2626;
}

.action-default {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

@media (max-width: 767px) {
  .search-input {
    max-width: none;
    width: 100%;
  }

  .table-container {
    overflow-x: auto;
  }

  .data-table {
    min-width: 700px;
  }
}
</style>
