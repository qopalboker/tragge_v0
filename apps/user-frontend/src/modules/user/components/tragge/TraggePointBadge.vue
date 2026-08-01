<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';

interface Props {
  score: number;
  showLabel?: boolean;
  size?: 'sm' | 'md' | 'lg';
}

const props = withDefaults(defineProps<Props>(), {
  showLabel: true,
  size: 'md',
});

interface BadgeLevel {
  name: string;
  minScore: number;
  color: string;
  bgColor: string;
  icon: string;
}

const badgeLevels: BadgeLevel[] = [
  { name: 'diamond', minScore: 50000, color: '#60A5FA', bgColor: '#DBEAFE', icon: '💎' },
  { name: 'gold', minScore: 10000, color: '#F59E0B', bgColor: '#FEF3C7', icon: '🥇' },
  { name: 'silver', minScore: 5000, color: '#6B7280', bgColor: '#F3F4F6', icon: '🥈' },
  { name: 'bronze', minScore: 1000, color: '#B45309', bgColor: '#FDE68A', icon: '🥉' },
];

const currentBadge = computed<BadgeLevel | null>(() => {
  for (const badge of badgeLevels) {
    if (props.score >= badge.minScore) {
      return badge;
    }
  }
  return null;
});

const nextBadge = computed<BadgeLevel | null>(() => {
  if (!currentBadge.value) {
    return badgeLevels[badgeLevels.length - 1]; // Bronze
  }
  const currentIndex = badgeLevels.findIndex(b => b.name === currentBadge.value?.name);
  return currentIndex > 0 ? badgeLevels[currentIndex - 1] : null;
});

const progressToNext = computed(() => {
  if (!nextBadge.value) return 100;
  const currentMin = currentBadge.value?.minScore || 0;
  const nextMin = nextBadge.value.minScore;
  const progress = ((props.score - currentMin) / (nextMin - currentMin)) * 100;
  return Math.min(Math.max(progress, 0), 100);
});

const badgeLabel = computed(() => {
  if (!currentBadge.value) return t('tragge.noBadge');
  return t(`tragge.badge.${currentBadge.value.name}`);
});
</script>

<template>
  <div :class="['tragge-badge', `badge-${size}`, currentBadge ? `badge-${currentBadge.name}` : 'badge-none']">
    <span v-if="currentBadge" class="badge-icon">{{ currentBadge.icon }}</span>
    <span v-else class="badge-icon badge-icon-empty">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="10" />
      </svg>
    </span>
    <span v-if="showLabel" class="badge-label">{{ badgeLabel }}</span>

    <!-- Progress to next badge -->
    <div v-if="nextBadge && showLabel" class="badge-progress-wrapper">
      <div class="badge-progress">
        <div class="badge-progress-fill" :style="{ width: `${progressToNext}%` }"></div>
      </div>
      <span class="badge-next-hint">
        {{ t('tragge.nextBadge', { badge: t(`tragge.badge.${nextBadge.name}`), score: nextBadge.minScore.toLocaleString() }) }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.tragge-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-weight: 600;
}

/* Size variants */
.badge-sm {
  font-size: var(--font-size-xs);
  padding: 2px var(--spacing-xs);
}

.badge-sm .badge-icon {
  font-size: 12px;
}

.badge-md {
  font-size: var(--font-size-sm);
}

.badge-md .badge-icon {
  font-size: 16px;
}

.badge-lg {
  font-size: var(--font-size-md);
  padding: var(--spacing-sm) var(--spacing-md);
}

.badge-lg .badge-icon {
  font-size: 20px;
}

/* Badge colors */
.badge-diamond {
  background-color: #DBEAFE;
  color: #1E40AF;
}

.badge-gold {
  background-color: #FEF3C7;
  color: #B45309;
}

.badge-silver {
  background-color: #F3F4F6;
  color: #4B5563;
}

.badge-bronze {
  background-color: #FDE68A;
  color: #92400E;
}

.badge-none {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-secondary);
}

.badge-icon-empty {
  opacity: 0.5;
}

.badge-progress-wrapper {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-left: var(--spacing-sm);
}

[dir="rtl"] .badge-progress-wrapper {
  margin-left: 0;
  margin-right: var(--spacing-sm);
}

.badge-progress {
  width: 60px;
  height: 4px;
  background-color: var(--color-border);
  border-radius: var(--radius-full);
  overflow: hidden;
}

.badge-progress-fill {
  height: 100%;
  background-color: var(--color-primary);
  border-radius: var(--radius-full);
  transition: width var(--transition-normal);
}

.badge-next-hint {
  font-size: var(--font-size-xs);
  font-weight: 400;
  color: var(--color-text-tertiary);
  white-space: nowrap;
}
</style>
