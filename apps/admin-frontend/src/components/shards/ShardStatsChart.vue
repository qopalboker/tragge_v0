<script setup lang="ts">
import { computed } from 'vue';
import { t } from '@/i18n';
import type { ShardStats } from '@/api/shards';

interface Props {
  stats: ShardStats[];
}

const props = defineProps<Props>();

const maxOrdersPerSec = computed(() => {
  if (props.stats.length === 0) return 100;
  return Math.max(...props.stats.map(s => s.orders_per_sec), 100);
});

const maxParticipants = computed(() => {
  if (props.stats.length === 0) return 1000;
  return Math.max(...props.stats.map(s => s.participant_count), 1000);
});

function getBarHeight(value: number, max: number): string {
  const percentage = Math.min((value / max) * 100, 100);
  return `${Math.max(percentage, 5)}%`;
}

function getBarColor(index: number): string {
  const colors = [
    'var(--color-primary)',
    '#10B981',
    '#F59E0B',
    '#EF4444',
    '#8B5CF6',
    '#EC4899',
    '#06B6D4',
    '#84CC16',
  ];
  return colors[index % colors.length];
}
</script>

<template>
  <div class="stats-chart-container">
    <h2 class="chart-title">{{ t('shards.statsTitle') }}</h2>

    <div v-if="stats.length === 0" class="no-data">
      {{ t('shards.noStats') }}
    </div>

    <div v-else class="charts-grid">
      <!-- Orders per Second Chart -->
      <div class="chart-section">
        <h3 class="chart-subtitle">{{ t('shards.ordersPerSecChart') }}</h3>
        <div class="bar-chart">
          <div class="chart-y-axis">
            <span>{{ maxOrdersPerSec.toFixed(0) }}</span>
            <span>{{ (maxOrdersPerSec / 2).toFixed(0) }}</span>
            <span>0</span>
          </div>
          <div class="chart-bars">
            <div
              v-for="(stat, index) in stats"
              :key="`orders-${stat.shard_id}`"
              class="bar-wrapper"
            >
              <div
                class="bar"
                :style="{
                  height: getBarHeight(stat.orders_per_sec, maxOrdersPerSec),
                  backgroundColor: getBarColor(index),
                }"
                :title="`${stat.orders_per_sec.toFixed(1)} orders/sec`"
              />
              <span class="bar-label">{{ t('shards.shardAbbrev') }}{{ stat.shard_id }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Participants Chart -->
      <div class="chart-section">
        <h3 class="chart-subtitle">{{ t('shards.participantsChart') }}</h3>
        <div class="bar-chart">
          <div class="chart-y-axis">
            <span>{{ maxParticipants }}</span>
            <span>{{ Math.floor(maxParticipants / 2) }}</span>
            <span>0</span>
          </div>
          <div class="chart-bars">
            <div
              v-for="(stat, index) in stats"
              :key="`participants-${stat.shard_id}`"
              class="bar-wrapper"
            >
              <div
                class="bar"
                :style="{
                  height: getBarHeight(stat.participant_count, maxParticipants),
                  backgroundColor: getBarColor(index),
                }"
                :title="`${stat.participant_count} participants`"
              />
              <span class="bar-label">{{ t('shards.shardAbbrev') }}{{ stat.shard_id }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Legend -->
    <div v-if="stats.length > 0" class="chart-legend">
      <div
        v-for="(stat, index) in stats"
        :key="`legend-${stat.shard_id}`"
        class="legend-item"
      >
        <span
          class="legend-color"
          :style="{ backgroundColor: getBarColor(index) }"
        />
        <span class="legend-label">
          {{ t('shards.shard') }} #{{ stat.shard_id }}
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.stats-chart-container {
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
}

.chart-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-lg);
}

.no-data {
  text-align: center;
  color: var(--color-text-secondary);
  padding: var(--spacing-xl);
}

.charts-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-xl);
}

@media (max-width: 767px) {
  .charts-grid {
    grid-template-columns: 1fr;
  }
}

.chart-section {
  min-height: 200px;
}

.chart-subtitle {
  font-size: var(--font-size-md);
  font-weight: 500;
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-md);
}

.bar-chart {
  display: flex;
  height: 180px;
  gap: var(--spacing-sm);
}

.chart-y-axis {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  align-items: flex-end;
  padding-bottom: 24px;
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  min-width: 40px;
}

.chart-bars {
  flex: 1;
  display: flex;
  align-items: flex-end;
  gap: var(--spacing-sm);
  padding-bottom: 24px;
  border-left: 1px solid var(--color-border);
  border-bottom: 1px solid var(--color-border);
}

[dir="rtl"] .chart-bars {
  border-left: none;
  border-right: 1px solid var(--color-border);
}

.bar-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  height: 100%;
  max-width: 60px;
}

.bar {
  width: 100%;
  max-width: 40px;
  border-radius: var(--radius-sm) var(--radius-sm) 0 0;
  transition: height var(--transition-normal);
  cursor: pointer;
}

.bar:hover {
  opacity: 0.8;
}

.bar-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  margin-top: var(--spacing-xs);
  position: absolute;
  bottom: 0;
  transform: translateY(100%);
}

.bar-wrapper {
  position: relative;
}

.chart-legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-md);
  margin-top: var(--spacing-lg);
  padding-top: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.legend-color {
  width: 12px;
  height: 12px;
  border-radius: var(--radius-sm);
}

.legend-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}
</style>
