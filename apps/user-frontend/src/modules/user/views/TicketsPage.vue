<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { ticketsApi, type Ticket } from '../api/tickets';

const router = useRouter();

const tickets = ref<Ticket[]>([]);
const total = ref(0);
const loading = ref(true);
const activeTab = ref<string>('all');

const tabs = computed(() => [
  { key: 'all', label: t('tickets.filter.all') },
  { key: 'open', label: t('tickets.filter.open') },
  { key: 'user_replied', label: t('tickets.status.user_replied') },
  { key: 'answered', label: t('tickets.filter.answered') },
  { key: 'closed', label: t('tickets.filter.closed') },
]);

async function loadTickets() {
  loading.value = true;
  try {
    const params: Record<string, unknown> = { limit: 20 };
    if (activeTab.value !== 'all') params.status = activeTab.value;
    const res = await ticketsApi.list(params as { limit: number; status?: string });
    tickets.value = res.tickets;
    total.value = res.total;
  } catch {
    // Error handled by interceptor
  } finally {
    loading.value = false;
  }
}

function selectTab(key: string) {
  activeTab.value = key;
  loadTickets();
}

function openTicket(id: string) {
  router.push(`/user/tickets/${id}`);
}

function createTicket() {
  router.push('/user/tickets/new');
}

function getStatusClass(status: string) {
  const map: Record<string, string> = {
    open: 'status-open',
    answered: 'status-answered',
    user_replied: 'status-waiting',
    closed: 'status-closed',
    resolved: 'status-resolved',
  };
  return map[status] || '';
}

function getCategoryLabel(category: string) {
  return t(`tickets.category.${category}`) || category;
}

function getStatusLabel(status: string) {
  return t(`tickets.status.${status}`) || status;
}

function formatDate(dateStr: string) {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return t('tickets.today');
  if (diffMins < 60) return `${diffMins}m`;
  if (diffHours < 24) return `${diffHours}h`;
  if (diffDays < 7) return `${diffDays}d`;
  return date.toLocaleDateString();
}

onMounted(loadTickets);
</script>

<template>
  <div class="tickets-page">
    <div class="tickets-header">
      <h1>{{ t('tickets.title') }}</h1>
      <button class="btn-primary" @click="createTicket">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="18" height="18"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
        {{ t('tickets.newTicket') }}
      </button>
    </div>

    <div class="tabs">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="tab"
        :class="{ active: activeTab === tab.key }"
        @click="selectTab(tab.key)"
      >
        {{ tab.label }}
      </button>
    </div>

    <div v-if="loading" class="loading">
      <div v-for="i in 3" :key="i" class="skeleton-card" />
    </div>

    <div v-else-if="tickets.length === 0" class="empty-state">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="48" height="48">
        <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
      </svg>
      <p class="empty-title">{{ t('tickets.noTickets') }}</p>
      <p class="empty-desc">{{ t('tickets.noTicketsDesc') }}</p>
      <button class="btn-primary" @click="createTicket">{{ t('tickets.createFirst') }}</button>
    </div>

    <div v-else class="ticket-list">
      <div
        v-for="ticket in tickets"
        :key="ticket.id"
        class="ticket-card"
        :class="{ unread: ticket.unread }"
        @click="openTicket(ticket.id)"
      >
        <div class="ticket-card-top">
          <span class="ticket-subject">{{ ticket.subject }}</span>
          <span class="ticket-time">{{ formatDate(ticket.last_message_at || ticket.updated_at) }}</span>
        </div>
        <div class="ticket-card-meta">
          <span class="badge category-badge">{{ getCategoryLabel(ticket.category) }}</span>
          <span class="badge" :class="getStatusClass(ticket.status)">{{ getStatusLabel(ticket.status) }}</span>
        </div>
        <p v-if="ticket.last_message_preview" class="ticket-preview">{{ ticket.last_message_preview }}</p>
        <div class="ticket-card-footer">
          <span class="msg-count">{{ ticket.message_count }} {{ t('tickets.message') }}</span>
          <span v-if="ticket.unread" class="unread-dot" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tickets-page {
  max-width: 700px;
  margin: 0 auto;
  padding: 1.5rem 1rem;
}

.tickets-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.5rem;
}

.tickets-header h1 {
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--theme-text, #fff);
  margin: 0;
}

.btn-primary {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.6rem 1.2rem;
  background: var(--theme-accent, #6366f1);
  color: #fff;
  border: none;
  border-radius: 0.75rem;
  font-size: 0.875rem;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.2s;
}
.btn-primary:hover { opacity: 0.9; }

.tabs {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 1.25rem;
  overflow-x: auto;
}

.tab {
  padding: 0.5rem 1rem;
  border: none;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  color: var(--theme-text-secondary, #999);
  border-radius: 0.5rem;
  font-size: 0.8125rem;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}
.tab.active {
  background: var(--theme-accent, #6366f1);
  color: #fff;
}

.loading { display: flex; flex-direction: column; gap: 0.75rem; }
.skeleton-card {
  height: 100px;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border-radius: 0.75rem;
  animation: pulse 1.5s ease-in-out infinite;
}
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }

.empty-state {
  text-align: center;
  padding: 3rem 1rem;
  color: var(--theme-text-secondary, #999);
}
.empty-state svg { margin-bottom: 1rem; opacity: 0.5; }
.empty-title { font-size: 1.125rem; font-weight: 600; color: var(--theme-text, #fff); margin-bottom: 0.5rem; }
.empty-desc { font-size: 0.875rem; margin-bottom: 1.5rem; }

.ticket-list { display: flex; flex-direction: column; gap: 0.75rem; }

.ticket-card {
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border-radius: 0.75rem;
  padding: 1rem;
  cursor: pointer;
  transition: background 0.2s, transform 0.15s;
  border: 1px solid transparent;
}
.ticket-card:hover {
  background: var(--theme-surface, rgba(255,255,255,0.1));
  transform: translateY(-1px);
}
.ticket-card.unread {
  border-inline-start: 3px solid var(--theme-accent, #6366f1);
}

.ticket-card-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}
.ticket-subject {
  font-weight: 600;
  color: var(--theme-text, #fff);
  font-size: 0.9375rem;
}
.ticket-time {
  font-size: 0.75rem;
  color: var(--theme-text-secondary, #999);
}

.ticket-card-meta {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}

.badge {
  padding: 0.2rem 0.6rem;
  border-radius: 1rem;
  font-size: 0.6875rem;
  font-weight: 600;
  background: var(--theme-glass, rgba(255,255,255,0.1));
  color: var(--theme-text-secondary, #999);
}
.category-badge { background: rgba(99, 102, 241, 0.15); color: var(--theme-accent, #6366f1); }
.status-open { background: rgba(59, 130, 246, 0.15); color: #60a5fa; }
.status-answered { background: rgba(34, 197, 94, 0.15); color: #4ade80; }
.status-waiting { background: rgba(234, 179, 8, 0.15); color: #facc15; }
.status-closed { background: rgba(156, 163, 175, 0.15); color: #9ca3af; }
.status-resolved { background: rgba(34, 197, 94, 0.15); color: #4ade80; }

.ticket-preview {
  font-size: 0.8125rem;
  color: var(--theme-text-secondary, #999);
  margin: 0;
  line-height: 1.4;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ticket-card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 0.5rem;
}
.msg-count {
  font-size: 0.75rem;
  color: var(--theme-text-secondary, #999);
}
.unread-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--theme-accent, #6366f1);
}

</style>
