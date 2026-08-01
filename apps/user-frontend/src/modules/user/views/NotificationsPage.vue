<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useNotificationStore } from '@/stores/notifications';
import { useNotificationRenderer } from '@/composables/useNotificationRenderer';
import { useNotificationPrefs } from '@/composables/useNotificationPrefs';
import { useToast } from '@/composables/useToast';
import type { InAppNotification, NotificationType } from '@/api/notifications';

const router = useRouter();
const notificationStore = useNotificationStore();
const { renderNotification } = useNotificationRenderer();
const { shouldShow } = useNotificationPrefs();
const toast = useToast();

// ==================== State ====================
type FilterTab = 'all' | 'unread';
const activeTab = ref<FilterTab>('all');
const deleteConfirmId = ref<string | null>(null);

// ==================== Computed ====================
const filteredNotifications = computed(() => {
  const list = activeTab.value === 'unread'
    ? notificationStore.notifications.filter((n) => !n.read_at)
    : notificationStore.notifications;
  return list
    .filter(n => shouldShow(n.type))
    .map(n => ({
      ...n,
      rendered: renderNotification(n),
    }));
});

const hasNotifications = computed(() => filteredNotifications.value.length > 0);

// ==================== Time Formatting ====================
function formatTimeAgo(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return t('notifications.timeAgo.justNow');
  if (diffMins < 60) return t('notifications.timeAgo.minutesAgo', { n: diffMins });
  if (diffHours < 24) return t('notifications.timeAgo.hoursAgo', { n: diffHours });
  return t('notifications.timeAgo.daysAgo', { n: diffDays });
}

function formatFullDate(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

// ==================== Type Icons & Colors ====================
function getTypeIcon(type: NotificationType): string {
  const icons: Record<NotificationType, string> = {
    contest_starting: 'flag',
    contest_completed: 'check-circle',
    contest_cancelled: 'x-circle',
    prize_won: 'trophy',
    withdrawal_update: 'arrow-down',
    deposit_confirmed: 'arrow-up',
    kyc_update: 'shield',
    system: 'bell',
    contest_ending: 'clock',
    contest_joined: 'user-plus',
    contest_left: 'user-minus',
    deposit_failed: 'x-circle',
    contest_started: 'play',
    registration_closed: 'lock',
    contest_paused: 'pause',
    contest_resumed: 'play',
    ticket_reply: 'message-circle',
  };
  return icons[type] || 'bell';
}

function getTypeColor(type: NotificationType): string {
  const colors: Record<NotificationType, string> = {
    contest_starting: 'var(--color-primary)',
    contest_completed: 'var(--color-success)',
    contest_cancelled: 'var(--color-danger)',
    prize_won: 'var(--color-warning)',
    withdrawal_update: 'var(--color-info)',
    deposit_confirmed: 'var(--color-success)',
    kyc_update: 'var(--color-primary)',
    system: 'var(--color-text-secondary)',
    contest_ending: 'var(--color-warning)',
    contest_joined: 'var(--color-success)',
    contest_left: 'var(--color-text-secondary)',
    deposit_failed: 'var(--color-danger)',
    contest_started: 'var(--color-success)',
    registration_closed: 'var(--color-info)',
    contest_paused: 'var(--color-warning)',
    contest_resumed: 'var(--color-success)',
    ticket_reply: 'var(--color-info)',
  };
  return colors[type] || 'var(--color-text-secondary)';
}

function getTypeLabel(type: NotificationType): string {
  const labels: Record<NotificationType, string> = {
    contest_starting: t('notifications.types.contest_starting'),
    contest_completed: t('notifications.types.contest_completed'),
    contest_cancelled: t('notifications.types.contest_cancelled'),
    prize_won: t('notifications.types.prize_won'),
    withdrawal_update: t('notifications.types.withdrawal_update'),
    deposit_confirmed: t('notifications.types.deposit_confirmed'),
    kyc_update: t('notifications.types.kyc_update'),
    system: t('notifications.types.system'),
    contest_ending: t('notifications.types.contest_ending'),
    contest_joined: t('notifications.types.contest_joined'),
    contest_left: t('notifications.types.contest_left'),
    deposit_failed: t('notifications.types.deposit_failed'),
    contest_started: t('notifications.types.contest_started'),
    registration_closed: t('notifications.types.registration_closed'),
    contest_paused: t('notifications.types.contest_paused'),
    contest_resumed: t('notifications.types.contest_resumed'),
    ticket_reply: t('notifications.types.ticket_reply'),
  };
  return labels[type] || t('notifications.types.system');
}

// ==================== Actions ====================
async function handleNotificationClick(notification: InAppNotification): Promise<void> {
  // Mark as read
  if (!notification.read_at) {
    await notificationStore.markAsRead(notification.id);
  }

  // Navigate based on metadata
  if (notification.metadata?.contest_id) {
    router.push(`/user/contests/${notification.metadata.contest_id}`);
  } else if (notification.metadata?.transaction_id || notification.metadata?.withdrawal_id) {
    router.push('/user/wallet');
  } else if (notification.type === 'deposit_failed') {
    router.push('/user/wallet');
  } else if (notification.type === 'kyc_update') {
    router.push('/user/profile/verify');
  }
}

async function handleMarkAllAsRead(): Promise<void> {
  await notificationStore.markAllAsRead();
  toast.success(t('notifications.markedAllRead'));
}

function showDeleteConfirm(id: string, event: Event): void {
  event.stopPropagation();
  deleteConfirmId.value = id;
}

function cancelDelete(): void {
  deleteConfirmId.value = null;
}

async function confirmDelete(id: string): Promise<void> {
  await notificationStore.deleteNotification(id);
  deleteConfirmId.value = null;
  toast.success(t('notifications.deleted'));
}

function loadMore(): void {
  if (notificationStore.hasMore && !notificationStore.loading) {
    notificationStore.fetchNotifications(true);
  }
}

function setTab(tab: FilterTab): void {
  activeTab.value = tab;
}

// ==================== Lifecycle ====================
onMounted(() => {
  if (!notificationStore.initialized) {
    notificationStore.fetchNotifications();
  }
});

// Reset filter when switching to unread if there are no unread notifications
watch(activeTab, (newTab) => {
  if (newTab === 'unread' && filteredNotifications.value.length === 0) {
    // Keep on unread tab but show empty state
  }
});
</script>

<template>
  <div class="notifications-page">
    <!-- Header -->
    <header class="page-header">
      <div class="header-content">
        <h1>{{ t('notifications.title') }}</h1>
        <button
          v-if="notificationStore.hasUnread"
          class="mark-all-btn"
          @click="handleMarkAllAsRead"
        >
          <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
          </svg>
          {{ t('notifications.markAllRead') }}
        </button>
      </div>

      <!-- Filter Tabs -->
      <div class="filter-tabs">
        <button
          :class="['tab-btn', { active: activeTab === 'all' }]"
          @click="setTab('all')"
        >
          {{ t('notifications.all') }}
          <span v-if="notificationStore.totalCount > 0" class="tab-count">
            {{ notificationStore.totalCount }}
          </span>
        </button>
        <button
          :class="['tab-btn', { active: activeTab === 'unread' }]"
          @click="setTab('unread')"
        >
          {{ t('notifications.unread') }}
          <span v-if="notificationStore.unreadCount > 0" class="tab-count unread">
            {{ notificationStore.unreadCount }}
          </span>
        </button>
      </div>
    </header>

    <!-- Content -->
    <div class="page-content">
      <!-- Loading State -->
      <div v-if="notificationStore.loading && !notificationStore.initialized" class="loading-state">
        <div class="skeleton-list">
          <div v-for="i in 5" :key="i" class="skeleton-item">
            <div class="skeleton-icon" />
            <div class="skeleton-content">
              <div class="skeleton-title" />
              <div class="skeleton-message" />
              <div class="skeleton-time" />
            </div>
          </div>
        </div>
      </div>

      <!-- Empty State -->
      <div v-else-if="!hasNotifications" class="empty-state">
        <div class="empty-icon-wrapper">
          <svg class="empty-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"/>
          </svg>
        </div>
        <h2>{{ activeTab === 'unread' ? t('notifications.noUnread') : t('notifications.noNotifications') }}</h2>
        <p>{{ t('notifications.empty') }}</p>
      </div>

      <!-- Notifications List -->
      <div v-else class="notifications-list">
        <div
          v-for="notification in filteredNotifications"
          :key="notification.id"
          :class="['notification-card', { unread: !notification.read_at }]"
        >
          <!-- Main Content (clickable) -->
          <button class="notification-main" @click="handleNotificationClick(notification)">
            <!-- Icon -->
            <div class="notification-icon" :style="{ backgroundColor: getTypeColor(notification.type) + '20', color: getTypeColor(notification.type) }">
              <!-- Trophy -->
              <svg v-if="getTypeIcon(notification.type) === 'trophy'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"/>
              </svg>
              <!-- Flag -->
              <svg v-else-if="getTypeIcon(notification.type) === 'flag'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 21v-4m0 0V5a2 2 0 012-2h6.5l1 1H21l-3 6 3 6h-8.5l-1-1H5a2 2 0 00-2 2zm9-13.5V9"/>
              </svg>
              <!-- Check Circle -->
              <svg v-else-if="getTypeIcon(notification.type) === 'check-circle'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
              </svg>
              <!-- X Circle -->
              <svg v-else-if="getTypeIcon(notification.type) === 'x-circle'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z"/>
              </svg>
              <!-- Arrow Down (withdrawal) -->
              <svg v-else-if="getTypeIcon(notification.type) === 'arrow-down'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 14l-7 7m0 0l-7-7m7 7V3"/>
              </svg>
              <!-- Arrow Up (deposit) -->
              <svg v-else-if="getTypeIcon(notification.type) === 'arrow-up'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 10l7-7m0 0l7 7m-7-7v18"/>
              </svg>
              <!-- Shield -->
              <svg v-else-if="getTypeIcon(notification.type) === 'shield'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/>
              </svg>
              <!-- Clock -->
              <svg v-else-if="getTypeIcon(notification.type) === 'clock'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="10" stroke-linecap="round" stroke-linejoin="round" stroke-width="2"/>
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6l4 2"/>
              </svg>
              <!-- User Plus -->
              <svg v-else-if="getTypeIcon(notification.type) === 'user-plus'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2"/>
                <circle cx="8.5" cy="7" r="4" stroke-linecap="round" stroke-linejoin="round" stroke-width="2"/>
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 8v6m3-3h-6"/>
              </svg>
              <!-- User Minus -->
              <svg v-else-if="getTypeIcon(notification.type) === 'user-minus'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2"/>
                <circle cx="8.5" cy="7" r="4" stroke-linecap="round" stroke-linejoin="round" stroke-width="2"/>
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M23 11h-6"/>
              </svg>
              <!-- Play -->
              <svg v-else-if="getTypeIcon(notification.type) === 'play'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <polygon points="5,3 19,12 5,21" stroke-linecap="round" stroke-linejoin="round" stroke-width="2"/>
              </svg>
              <!-- Pause -->
              <svg v-else-if="getTypeIcon(notification.type) === 'pause'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 4h4v16H6zM14 4h4v16h-4z"/>
              </svg>
              <!-- Lock -->
              <svg v-else-if="getTypeIcon(notification.type) === 'lock'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2" stroke-linecap="round" stroke-linejoin="round" stroke-width="2"/>
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 11V7a5 5 0 0110 0v4"/>
              </svg>
              <!-- Bell (default) -->
              <svg v-else fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"/>
              </svg>
            </div>

            <!-- Content -->
            <div class="notification-content">
              <div class="notification-header">
                <span class="notification-type" :style="{ color: getTypeColor(notification.type) }">
                  {{ getTypeLabel(notification.type) }}
                </span>
                <span v-if="!notification.read_at" class="unread-badge">{{ t('notifications.new') }}</span>
              </div>
              <h3 :class="['notification-title', { unread: !notification.read_at }]">
                {{ notification.rendered.title }}
              </h3>
              <p class="notification-message">{{ notification.rendered.message }}</p>
              <div class="notification-footer">
                <span class="notification-time" :title="formatFullDate(notification.created_at)">
                  {{ formatTimeAgo(notification.created_at) }}
                </span>
              </div>
            </div>
          </button>

          <!-- Actions -->
          <div class="notification-actions">
            <button
              v-if="deleteConfirmId !== notification.id"
              class="delete-btn"
              :title="t('notifications.delete')"
              @click="showDeleteConfirm(notification.id, $event)"
            >
              <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
              </svg>
            </button>
            <div v-else class="delete-confirm">
              <button class="confirm-btn" @click="confirmDelete(notification.id)">
                <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
              </button>
              <button class="cancel-btn" @click="cancelDelete">
                <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                </svg>
              </button>
            </div>
          </div>
        </div>

        <!-- Load More -->
        <div v-if="notificationStore.hasMore" class="load-more">
          <button
            class="load-more-btn"
            :disabled="notificationStore.loading"
            @click="loadMore"
          >
            <span v-if="notificationStore.loading" class="spinner" />
            <span v-else>{{ t('common.loadMore') }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.notifications-page {
  max-width: 800px;
  margin: 0 auto;
  padding: var(--spacing-lg);
}

/* Header */
.page-header {
  margin-bottom: var(--spacing-lg);
}

.header-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--spacing-md);
}

.page-header h1 {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0;
}

.mark-all-btn {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-primary-light);
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-primary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.mark-all-btn:hover {
  background-color: var(--color-primary);
  color: white;
}

.mark-all-btn svg {
  width: 16px;
  height: 16px;
}

/* Filter Tabs */
.filter-tabs {
  display: flex;
  gap: var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
  padding-bottom: var(--spacing-sm);
}

.tab-btn {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  background: none;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.tab-btn:hover {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

.tab-btn.active {
  background-color: var(--color-primary-light);
  color: var(--color-primary);
}

.tab-count {
  padding: 2px 6px;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
}

.tab-btn.active .tab-count {
  background-color: var(--color-primary);
  color: white;
}

.tab-count.unread {
  background-color: var(--color-primary);
  color: white;
}

/* Loading State */
.loading-state {
  padding: var(--spacing-lg) 0;
}

.skeleton-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.skeleton-item {
  display: flex;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

.skeleton-icon {
  width: 48px;
  height: 48px;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-full);
  animation: pulse 1.5s ease-in-out infinite;
}

.skeleton-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.skeleton-title {
  height: 20px;
  width: 60%;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  animation: pulse 1.5s ease-in-out infinite;
}

.skeleton-message {
  height: 16px;
  width: 80%;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  animation: pulse 1.5s ease-in-out infinite;
}

.skeleton-time {
  height: 14px;
  width: 30%;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.5;
  }
}

/* Empty State */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-3xl) var(--spacing-lg);
  text-align: center;
}

.empty-icon-wrapper {
  width: 80px;
  height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-full);
  margin-bottom: var(--spacing-lg);
}

.empty-icon {
  width: 40px;
  height: 40px;
  color: var(--color-text-tertiary);
}

.empty-state h2 {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-xs);
}

.empty-state p {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
}

/* Notifications List */
.notifications-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.notification-card {
  display: flex;
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  overflow: hidden;
  transition: all var(--transition-fast);
}

.notification-card:hover {
  border-color: var(--color-primary);
  box-shadow: var(--shadow-md);
}

.notification-card.unread {
  background-color: var(--color-primary-light);
  border-color: var(--color-primary);
}

.notification-main {
  flex: 1;
  display: flex;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  background: none;
  border: none;
  cursor: pointer;
  text-align: start;
  min-width: 0;
}

/* Icon */
.notification-icon {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-full);
  flex-shrink: 0;
}

.notification-icon svg {
  width: 24px;
  height: 24px;
}

/* Content */
.notification-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.notification-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.notification-type {
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.unread-badge {
  padding: 2px 8px;
  background-color: var(--color-primary);
  color: white;
  font-size: var(--font-size-xs);
  font-weight: 500;
  border-radius: var(--radius-full);
}

.notification-title {
  font-size: var(--font-size-md);
  font-weight: 500;
  color: var(--color-text-primary);
  margin: 0;
}

.notification-title.unread {
  font-weight: 600;
}

.notification-message {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
  line-height: 1.5;
}

.notification-footer {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  margin-top: var(--spacing-xs);
}

.notification-time {
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
}

/* Actions */
.notification-actions {
  display: flex;
  align-items: center;
  padding: var(--spacing-sm);
  border-left: 1px solid var(--color-border);
}

[dir="rtl"] .notification-actions {
  border-left: none;
  border-right: 1px solid var(--color-border);
}

.delete-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  background: none;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-tertiary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.delete-btn:hover {
  background-color: var(--color-danger-light);
  color: var(--color-danger);
}

.delete-btn svg {
  width: 18px;
  height: 18px;
}

.delete-confirm {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.confirm-btn,
.cancel-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.confirm-btn {
  background-color: var(--color-danger);
  color: white;
}

.confirm-btn:hover {
  background-color: var(--color-danger-dark);
}

.cancel-btn {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

.cancel-btn:hover {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

.confirm-btn svg,
.cancel-btn svg {
  width: 16px;
  height: 16px;
}

/* Load More */
.load-more {
  display: flex;
  justify-content: center;
  padding: var(--spacing-lg) 0;
}

.load-more-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-xl);
  background-color: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
  min-width: 150px;
}

.load-more-btn:hover:not(:disabled) {
  background-color: var(--color-bg-tertiary);
  border-color: var(--color-primary);
}

.load-more-btn:disabled {
  cursor: not-allowed;
  opacity: 0.7;
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Responsive */
@media (max-width: 767px) {
  .notifications-page {
    padding: var(--spacing-md);
  }

  .header-content {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--spacing-sm);
  }

  .mark-all-btn {
    width: 100%;
    justify-content: center;
  }

  .filter-tabs {
    width: 100%;
  }

  .tab-btn {
    flex: 1;
    justify-content: center;
  }

  .notification-main {
    padding: var(--spacing-md);
  }

  .notification-icon {
    width: 40px;
    height: 40px;
  }

  .notification-icon svg {
    width: 20px;
    height: 20px;
  }

  .notification-actions {
    padding: var(--spacing-xs);
  }
}
</style>
