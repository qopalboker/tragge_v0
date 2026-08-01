<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { adminTicketsApi, type AdminTicket, type AdminTicketStats } from '@/api/tickets';

const router = useRouter();

const tickets = ref<AdminTicket[]>([]);
const stats = ref<AdminTicketStats | null>(null);
const total = ref(0);
const loading = ref(true);
const statusFilter = ref('');
const categoryFilter = ref('');
const priorityFilter = ref('');
const searchQuery = ref('');

const statuses = ['', 'open', 'user_replied', 'answered', 'closed', 'resolved'];
const categories = ['', 'account', 'payment', 'contest', 'technical', 'kyc', 'other'];
const priorities = ['', 'low', 'medium', 'high', 'urgent'];

async function loadTickets() {
  loading.value = true;
  try {
    const params: Record<string, unknown> = { limit: 30 };
    if (statusFilter.value) params.status = statusFilter.value;
    if (categoryFilter.value) params.category = categoryFilter.value;
    if (priorityFilter.value) params.priority = priorityFilter.value;
    if (searchQuery.value) params.search = searchQuery.value;

    const [ticketRes, statsRes] = await Promise.all([
      adminTicketsApi.list(params as { limit: number }),
      adminTicketsApi.getStats(),
    ]);
    tickets.value = ticketRes.tickets;
    total.value = ticketRes.total;
    stats.value = statsRes;
  } catch {
    // Error handled by interceptor
  } finally {
    loading.value = false;
  }
}

function openTicket(id: string) {
  router.push(`/admin/tickets/${id}`);
}

function applyFilters() {
  loadTickets();
}

function getPriorityClass(priority: string) {
  const map: Record<string, string> = {
    low: 'priority-low', medium: 'priority-medium',
    high: 'priority-high', urgent: 'priority-urgent',
  };
  return map[priority] || '';
}

function getStatusClass(status: string) {
  const map: Record<string, string> = {
    open: 'status-open', answered: 'status-answered',
    user_replied: 'status-waiting', closed: 'status-closed', resolved: 'status-resolved',
  };
  return map[status] || '';
}

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  });
}

onMounted(loadTickets);
</script>

<template>
  <div class="admin-tickets-page">
    <h1>{{ t('admin.nav.tickets') }}</h1>

    <!-- Stats bar -->
    <div v-if="stats" class="stats-bar">
      <div class="stat-card">
        <span class="stat-value">{{ stats.open }}</span>
        <span class="stat-label">{{ t('tickets.status.open') }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-value highlight">{{ stats.user_replied }}</span>
        <span class="stat-label">{{ t('tickets.status.user_replied') }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{{ stats.answered }}</span>
        <span class="stat-label">{{ t('tickets.status.answered') }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{{ stats.closed + stats.resolved }}</span>
        <span class="stat-label">{{ t('tickets.status.closed') }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{{ Math.round(stats.avg_response_time_minutes) }}m</span>
        <span class="stat-label">{{ t('adminTickets.avgResponse') }}</span>
      </div>
    </div>

    <!-- Filters -->
    <div class="filters">
      <input
        v-model="searchQuery"
        type="text"
        :placeholder="t('tickets.subjectPlaceholder')"
        class="search-input"
        @keydown.enter="applyFilters"
      />
      <select v-model="statusFilter" class="filter-select" @change="applyFilters">
        <option value="">{{ t('adminTickets.filterAllStatus') }}</option>
        <option v-for="s in statuses.slice(1)" :key="s" :value="s">{{ t(`tickets.status.${s}`) }}</option>
      </select>
      <select v-model="categoryFilter" class="filter-select" @change="applyFilters">
        <option value="">{{ t('adminTickets.filterAllCategory') }}</option>
        <option v-for="c in categories.slice(1)" :key="c" :value="c">{{ t(`tickets.category.${c}`) }}</option>
      </select>
      <select v-model="priorityFilter" class="filter-select" @change="applyFilters">
        <option value="">{{ t('adminTickets.filterAllPriority') }}</option>
        <option v-for="p in priorities.slice(1)" :key="p" :value="p">{{ t(`tickets.priority.${p}`) }}</option>
      </select>
    </div>

    <!-- Table -->
    <div v-if="loading" class="loading">
      <div v-for="i in 5" :key="i" class="skeleton-row" />
    </div>

    <div v-else-if="tickets.length === 0" class="empty">
      <p>{{ t('adminTickets.noTicketsFound') }}</p>
    </div>

    <div v-else class="tickets-table">
      <div class="table-header">
        <span class="col-subject">{{ t('adminTickets.subject') }}</span>
        <span class="col-user">{{ t('adminTickets.user') }}</span>
        <span class="col-category">{{ t('adminTickets.category') }}</span>
        <span class="col-status">{{ t('adminTickets.statusHeader') }}</span>
        <span class="col-priority">{{ t('adminTickets.priority') }}</span>
        <span class="col-date">{{ t('adminTickets.lastActivity') }}</span>
      </div>

      <div
        v-for="ticket in tickets"
        :key="ticket.id"
        class="table-row"
        @click="openTicket(ticket.id)"
      >
        <span class="col-subject">
          <strong>{{ ticket.subject }}</strong>
          <span class="msg-count">({{ ticket.message_count }})</span>
        </span>
        <span class="col-user">
          <span class="user-email">{{ ticket.user.username || ticket.user.email }}</span>
        </span>
        <span class="col-category">
          <span class="badge category-badge">{{ t(`tickets.category.${ticket.category}`) }}</span>
        </span>
        <span class="col-status">
          <span class="badge" :class="getStatusClass(ticket.status)">{{ t(`tickets.status.${ticket.status}`) }}</span>
        </span>
        <span class="col-priority">
          <span class="priority-dot" :class="getPriorityClass(ticket.priority)" />
          {{ t(`tickets.priority.${ticket.priority}`) }}
        </span>
        <span class="col-date">{{ formatDate(ticket.last_message_at || ticket.updated_at) }}</span>
      </div>
    </div>

    <div v-if="total > 0" class="total-count">
      {{ total }} {{ t('adminTickets.totalCount', { count: total }) }}
    </div>
  </div>
</template>

<style scoped>
.admin-tickets-page {
  padding: 1.5rem;
}

.admin-tickets-page h1 {
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--theme-text, #fff);
  margin: 0 0 1.5rem;
}

.stats-bar {
  display: flex;
  gap: 1rem;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
}

.stat-card {
  flex: 1;
  min-width: 100px;
  padding: 1rem;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border-radius: 0.75rem;
  display: flex;
  flex-direction: column;
  align-items: center;
}
.stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--theme-text, #fff);
}
.stat-value.highlight { color: #facc15; }
.stat-label {
  font-size: 0.75rem;
  color: var(--theme-text-secondary, #999);
  margin-top: 0.25rem;
}

.filters {
  display: flex;
  gap: 0.75rem;
  margin-bottom: 1.25rem;
  flex-wrap: wrap;
}

.search-input, .filter-select {
  padding: 0.5rem 0.75rem;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 0.5rem;
  color: var(--theme-text, #fff);
  font-size: 0.8125rem;
  outline: none;
}
.search-input { flex: 1; min-width: 200px; }
.filter-select { appearance: none; cursor: pointer; }
.search-input:focus, .filter-select:focus { border-color: var(--theme-accent, #6366f1); }

.loading { display: flex; flex-direction: column; gap: 0.5rem; }
.skeleton-row {
  height: 48px;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border-radius: 0.5rem;
  animation: pulse 1.5s ease-in-out infinite;
}
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }

.empty {
  text-align: center;
  padding: 3rem;
  color: var(--theme-text-secondary, #999);
}

.tickets-table { border-radius: 0.75rem; overflow: hidden; }

.table-header, .table-row {
  display: grid;
  grid-template-columns: 2fr 1fr 1fr 1fr 1fr 1fr;
  gap: 0.5rem;
  padding: 0.75rem 1rem;
  align-items: center;
}

.table-header {
  background: var(--theme-glass, rgba(255,255,255,0.06));
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--theme-text-secondary, #999);
  text-transform: uppercase;
}

.table-row {
  border-bottom: 1px solid rgba(255,255,255,0.05);
  cursor: pointer;
  transition: background 0.15s;
  font-size: 0.8125rem;
  color: var(--theme-text, #fff);
}
.table-row:hover { background: var(--theme-glass, rgba(255,255,255,0.04)); }

.col-subject strong { font-weight: 600; }
.msg-count { font-size: 0.75rem; color: var(--theme-text-secondary, #999); margin-inline-start: 0.25rem; }
.user-email { font-size: 0.75rem; color: var(--theme-text-secondary, #999); }

.badge {
  padding: 0.15rem 0.5rem;
  border-radius: 1rem;
  font-size: 0.6875rem;
  font-weight: 600;
}
.category-badge { background: rgba(99, 102, 241, 0.15); color: var(--theme-accent, #6366f1); }
.status-open { background: rgba(59, 130, 246, 0.15); color: #60a5fa; }
.status-answered { background: rgba(34, 197, 94, 0.15); color: #4ade80; }
.status-waiting { background: rgba(234, 179, 8, 0.15); color: #facc15; }
.status-closed { background: rgba(156, 163, 175, 0.15); color: #9ca3af; }
.status-resolved { background: rgba(34, 197, 94, 0.15); color: #4ade80; }

.priority-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-inline-end: 0.375rem;
}
.priority-low { background: #9ca3af; }
.priority-medium { background: #facc15; }
.priority-high { background: #f97316; }
.priority-urgent { background: #ef4444; }

.total-count {
  margin-top: 1rem;
  font-size: 0.75rem;
  color: var(--theme-text-secondary, #999);
  text-align: end;
}

@media (max-width: 768px) {
  .table-header { display: none; }
  .table-row {
    grid-template-columns: 1fr;
    gap: 0.25rem;
    padding: 1rem;
    background: var(--theme-glass, rgba(255,255,255,0.03));
    margin-bottom: 0.5rem;
    border-radius: 0.5rem;
  }
}

/* RTL overrides */
[dir="rtl"] .search-input {
  text-align: right;
}
</style>
