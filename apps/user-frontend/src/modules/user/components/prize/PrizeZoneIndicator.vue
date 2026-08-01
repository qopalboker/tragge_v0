<script setup lang="ts">
import { computed, toRef } from 'vue';
import { t } from '@/i18n';
import { usePrizeDistribution } from '@/composables/usePrizeDistribution';

interface Props {
  entryFeeCents: number;
  participantCount: number;
  currentViewRank?: number;
  showLabel?: boolean;
  variant?: 'line' | 'badge';
}

const props = withDefaults(defineProps<Props>(), {
  currentViewRank: undefined,
  showLabel: true,
  variant: 'line',
});

const {
  prizeZoneCutoff,
} = usePrizeDistribution({
  entryFeeCents: toRef(() => props.entryFeeCents),
  participantCount: toRef(() => props.participantCount),
});

// Show indicator if current view is at or near the cutoff
const showIndicator = computed(() => {
  if (!props.currentViewRank) return false;
  // Show when within 5 ranks of the cutoff
  const cutoff = prizeZoneCutoff.value;
  return Math.abs(props.currentViewRank - cutoff) <= 5 || props.currentViewRank === cutoff;
});
</script>

<template>
  <!-- Line variant - for showing between rows -->
  <div
    v-if="variant === 'line' && (showIndicator || !currentViewRank)"
    class="prize-zone-line"
  >
    <div class="line-left"></div>
    <div v-if="showLabel" class="line-label">
      <span class="label-icon">&#x1F3C6;</span>
      <span class="label-text">{{ t('prize.prizeZone') }}</span>
      <span class="label-cutoff">(#{{ prizeZoneCutoff }})</span>
    </div>
    <div class="line-right"></div>
  </div>

  <!-- Badge variant - for row indicators -->
  <span
    v-else-if="variant === 'badge'"
    :class="['prize-zone-badge', { 'in-zone': currentViewRank && currentViewRank <= prizeZoneCutoff }]"
  >
    <span v-if="currentViewRank && currentViewRank <= prizeZoneCutoff" class="badge-icon">&#x1F4B0;</span>
    <span v-else class="badge-icon out">&#x2022;</span>
  </span>
</template>

<style scoped>
/* Line Variant */
.prize-zone-line {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs) 0;
  margin: var(--spacing-xs) 0;
}

.line-left,
.line-right {
  flex: 1;
  height: 2px;
  background: linear-gradient(90deg, transparent, #10B981 50%, transparent);
}

.line-left {
  background: linear-gradient(90deg, transparent, #10B981);
}

.line-right {
  background: linear-gradient(90deg, #10B981, transparent);
}

.line-label {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.15), rgba(6, 182, 212, 0.1));
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: 9999px;
  white-space: nowrap;
}

.label-icon {
  font-size: 0.875rem;
}

.label-text {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: #10B981;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.label-cutoff {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

/* Badge Variant */
.prize-zone-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  font-size: 0.75rem;
}

.prize-zone-badge.in-zone {
  background: rgba(16, 185, 129, 0.15);
}

.prize-zone-badge .badge-icon {
  line-height: 1;
}

.prize-zone-badge .badge-icon.out {
  color: var(--color-text-muted);
  font-size: 1.25rem;
}

/* Dark mode support */
@media (prefers-color-scheme: dark) {
  .line-label {
    background: linear-gradient(135deg, rgba(16, 185, 129, 0.2), rgba(6, 182, 212, 0.15));
  }
}
</style>
