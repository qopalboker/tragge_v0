<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';

export interface Participant {
  user_id: string;
  username: string;
  joined_at: string;
  avatar_url?: string;
}

const props = defineProps<{
  participants: Participant[];
  loading: boolean;
  totalCount: number;
}>();

// Generate avatar placeholder color based on username
function getAvatarColor(username: string): string {
  const colors = [
    '#3B82F6', // blue
    '#10B981', // green
    '#F59E0B', // amber
    '#EF4444', // red
    '#8B5CF6', // purple
    '#EC4899', // pink
    '#06B6D4', // cyan
    '#F97316', // orange
  ];

  let hash = 0;
  for (let i = 0; i < username.length; i++) {
    hash = username.charCodeAt(i) + ((hash << 5) - hash);
  }

  return colors[Math.abs(hash) % colors.length];
}

// Get initials from username
function getInitials(username: string): string {
  return username.charAt(0).toUpperCase();
}

const hasParticipants = computed(() => props.participants.length > 0);
</script>

<template>
  <div class="participants-list">
    <!-- Header -->
    <div class="list-header">
      <div class="header-icon">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
          <circle cx="9" cy="7" r="4" />
          <path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
        </svg>
      </div>
      <h3 class="list-title">{{ t('contestDetails.traders') }}</h3>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="loading-state">
      <div class="loading-spinner"></div>
      <span>{{ t('common.loading') }}</span>
    </div>

    <!-- Empty State -->
    <div v-else-if="!hasParticipants" class="empty-state">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
        <circle cx="9" cy="7" r="4" />
        <path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
      </svg>
      <p>{{ t('contestDetails.noParticipants') }}</p>
    </div>

    <!-- Participants Table -->
    <div v-else class="table-container">
      <table class="participants-table">
        <thead>
          <tr>
            <th>{{ t('contestDetails.username') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="participant in participants"
            :key="participant.user_id"
            class="participant-row"
          >
            <td>
              <div class="participant-info">
                <div
                  v-if="participant.avatar_url"
                  class="avatar"
                  :style="{ backgroundImage: `url(${participant.avatar_url})` }"
                ></div>
                <div
                  v-else
                  class="avatar avatar-placeholder"
                  :style="{ backgroundColor: getAvatarColor(participant.username) }"
                >
                  {{ getInitials(participant.username) }}
                </div>
                <span class="username">{{ participant.username }}</span>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Show More -->
    <div v-if="totalCount > participants.length" class="show-more">
      <span class="more-count">
        {{ t('contestDetails.andMore', { count: totalCount - participants.length }) }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.participants-list {
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

.list-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-md) var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg-secondary);
}

.header-icon {
  color: var(--color-text-secondary);
}

.list-title {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xl);
  color: var(--color-text-secondary);
}

.loading-spinner {
  width: 24px;
  height: 24px;
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

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  color: var(--color-text-muted);
}

.empty-state svg {
  opacity: 0.5;
}

.empty-state p {
  margin: 0;
  font-size: var(--font-size-sm);
}

.table-container {
  max-height: 300px;
  overflow-y: auto;
}

.participants-table {
  width: 100%;
  border-collapse: collapse;
}

.participants-table th {
  text-align: left;
  padding: var(--spacing-sm) var(--spacing-lg);
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  background: var(--color-bg-secondary);
  position: sticky;
  top: 0;
  z-index: 1;
}

.participant-row {
  border-bottom: 1px solid var(--color-border-light, rgba(0,0,0,0.05));
  transition: background-color var(--transition-fast);
}

.participant-row:hover {
  background: var(--color-bg-secondary);
}

.participant-row:last-child {
  border-bottom: none;
}

.participant-row td {
  padding: var(--spacing-sm) var(--spacing-lg);
}

.participant-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background-size: cover;
  background-position: center;
  flex-shrink: 0;
}

.avatar-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: white;
}

.username {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.show-more {
  display: flex;
  justify-content: center;
  padding: var(--spacing-sm) var(--spacing-lg);
  border-top: 1px solid var(--color-border);
  background: var(--color-bg-secondary);
}

.more-count {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

/* RTL Support */
[dir="rtl"] .list-header {
  flex-direction: row-reverse;
}

[dir="rtl"] .participants-table th {
  text-align: right;
}

[dir="rtl"] .participant-info {
  flex-direction: row-reverse;
}

/* Mobile */
@media (max-width: 767px) {
  .list-header {
    padding: var(--spacing-sm) var(--spacing-md);
  }

  .participants-table th,
  .participant-row td {
    padding: var(--spacing-sm) var(--spacing-md);
  }

  .avatar {
    width: 28px;
    height: 28px;
  }

  .username {
    font-size: var(--font-size-xs);
  }

  .table-container {
    max-height: 200px;
  }
}
</style>
