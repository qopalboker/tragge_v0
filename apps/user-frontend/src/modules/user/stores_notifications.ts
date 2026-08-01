import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { notificationsApi, type InAppNotification } from '@/api/notifications';
import { getAccessToken } from '@/api/client';

const POLL_INTERVAL = 30000; // 30 seconds

export const useNotificationStore = defineStore('notifications', () => {
  // ==================== State ====================
  const notifications = ref<InAppNotification[]>([]);
  const unreadCount = ref(0);
  const loading = ref(false);
  const hasMore = ref(true);
  const totalCount = ref(0);
  const currentOffset = ref(0);
  const pollingInterval = ref<ReturnType<typeof setInterval> | null>(null);
  const initialized = ref(false);

  // ==================== Computed ====================
  const unreadNotifications = computed(() =>
    notifications.value.filter((n) => !n.read_at)
  );

  const hasUnread = computed(() => unreadCount.value > 0);

  // ==================== Actions ====================

  /**
   * Fetch unread count - lightweight, poll-friendly
   */
  async function fetchUnreadCount(): Promise<void> {
    // Skip if no auth token (prevents 401 spam after session expiry)
    if (!getAccessToken()) {
      stopPolling();
      return;
    }
    try {
      const response = await notificationsApi.getUnreadCount();
      unreadCount.value = response.count;
    } catch {
      // Silently fail for polling - don't show errors
    }
  }

  /**
   * Fetch notifications with pagination
   */
  async function fetchNotifications(loadMore = false): Promise<void> {
    if (loading.value) return;

    loading.value = true;

    try {
      const offset = loadMore ? currentOffset.value : 0;
      const response = await notificationsApi.getNotifications({
        limit: 20,
        offset,
      });

      if (loadMore) {
        notifications.value = [...notifications.value, ...response.notifications];
      } else {
        notifications.value = response.notifications;
      }

      totalCount.value = response.total;
      if (response.unread_count !== undefined) {
        unreadCount.value = response.unread_count;
      }
      currentOffset.value = notifications.value.length;
      hasMore.value = notifications.value.length < response.total;
      initialized.value = true;
    } catch {
      // Error is handled by API interceptor
    } finally {
      loading.value = false;
    }
  }

  /**
   * Mark a notification as read - optimistic update
   */
  async function markAsRead(id: string): Promise<void> {
    // Find notification and update optimistically
    const notification = notifications.value.find((n) => n.id === id);
    if (notification && !notification.read_at) {
      notification.read_at = new Date().toISOString();
      unreadCount.value = Math.max(0, unreadCount.value - 1);

      try {
        await notificationsApi.markAsRead(id);
      } catch {
        // Revert on error
        notification.read_at = null;
        unreadCount.value += 1;
      }
    }
  }

  /**
   * Mark all notifications as read - optimistic update
   */
  async function markAllAsRead(): Promise<void> {
    const previousStates = notifications.value.map((n) => ({ id: n.id, read_at: n.read_at }));
    const previousUnreadCount = unreadCount.value;

    // Optimistic update
    notifications.value.forEach((n) => {
      n.read_at = n.read_at || new Date().toISOString();
    });
    unreadCount.value = 0;

    try {
      await notificationsApi.markAllAsRead();
    } catch {
      // Revert on error
      notifications.value.forEach((n) => {
        const previous = previousStates.find((p) => p.id === n.id);
        if (previous) {
          n.read_at = previous.read_at;
        }
      });
      unreadCount.value = previousUnreadCount;
    }
  }

  /**
   * Delete a notification
   */
  async function deleteNotification(id: string): Promise<void> {
    const index = notifications.value.findIndex((n) => n.id === id);
    if (index === -1) return;

    const notification = notifications.value[index];
    const wasUnread = !notification.read_at;

    // Optimistic update
    notifications.value.splice(index, 1);
    totalCount.value = Math.max(0, totalCount.value - 1);
    currentOffset.value = Math.max(0, currentOffset.value - 1);
    if (wasUnread) {
      unreadCount.value = Math.max(0, unreadCount.value - 1);
    }

    try {
      await notificationsApi.deleteNotification(id);
    } catch {
      // Revert on error
      notifications.value.splice(index, 0, notification);
      totalCount.value += 1;
      currentOffset.value += 1;
      if (wasUnread) {
        unreadCount.value += 1;
      }
    }
  }

  /**
   * Start polling for unread count
   */
  function startPolling(): void {
    if (pollingInterval.value) return;

    // Fetch immediately
    fetchUnreadCount();

    // Then poll every 30 seconds
    pollingInterval.value = setInterval(() => {
      fetchUnreadCount();
    }, POLL_INTERVAL);
  }

  /**
   * Stop polling
   */
  function stopPolling(): void {
    if (pollingInterval.value) {
      clearInterval(pollingInterval.value);
      pollingInterval.value = null;
    }
  }

  /**
   * Reset store state
   */
  function reset(): void {
    stopPolling();
    notifications.value = [];
    unreadCount.value = 0;
    loading.value = false;
    hasMore.value = true;
    totalCount.value = 0;
    currentOffset.value = 0;
    initialized.value = false;
  }

  return {
    // State
    notifications,
    unreadCount,
    loading,
    hasMore,
    totalCount,
    initialized,
    // Computed
    unreadNotifications,
    hasUnread,
    // Actions
    fetchUnreadCount,
    fetchNotifications,
    markAsRead,
    markAllAsRead,
    deleteNotification,
    startPolling,
    stopPolling,
    reset,
  };
});
