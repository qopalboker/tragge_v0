<script setup lang="ts">
import { t } from '@/i18n';

export interface PrizeRow {
  rank: number;
  amount_cents: number;
  percentage: number;
}

defineProps<{
  prizes: PrizeRow[];
  prizePoolCents: number;
  commissionRate: number;
}>();

function formatCents(cents: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(cents / 100);
}

function getRankMedal(rank: number): string {
  switch (rank) {
    case 1: return '\u{1F947}';
    case 2: return '\u{1F948}';
    case 3: return '\u{1F949}';
    default: return '';
  }
}
</script>

<template>
  <div class="prize-table-wrapper">
    <table class="prize-table">
      <thead>
        <tr>
          <th>{{ t('contestBanner.rank') }}</th>
          <th class="text-right">{{ t('contestBanner.poolPercent') }}</th>
          <th class="text-right">{{ t('contestBanner.prizeAmount') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="prize in prizes"
          :key="prize.rank"
          :class="{ 'top-rank': prize.rank <= 3 }"
        >
          <td class="rank-cell">
            <span v-if="getRankMedal(prize.rank)" class="rank-medal">{{ getRankMedal(prize.rank) }}</span>
            <span class="rank-number">{{ prize.rank }}</span>
          </td>
          <td class="pct-cell text-right">{{ prize.percentage.toFixed(2) }}%</td>
          <td class="amount-cell text-right">{{ formatCents(prize.amount_cents) }}</td>
        </tr>
      </tbody>
    </table>

    <div class="prize-footer">
      <div class="footer-item">
        <span class="footer-label">{{ t('contestBanner.prizePool') }}:</span>
        <span class="footer-value prize-pool-value">{{ formatCents(prizePoolCents) }}</span>
      </div>
      <div class="footer-divider"></div>
      <div class="footer-item">
        <span class="footer-label">{{ t('contestBanner.fee') }}:</span>
        <span class="footer-value">{{ commissionRate }}%</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.prize-table-wrapper {
  overflow-x: auto;
}

.prize-table {
  width: 100%;
  border-collapse: collapse;
}

.prize-table th {
  padding: 8px 12px;
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-bottom: 1px solid var(--color-border);
}

.prize-table td {
  padding: 8px 12px;
  font-size: var(--font-size-sm);
  border-bottom: 1px solid var(--color-border-light, rgba(0, 0, 0, 0.06));
}

.prize-table tr:last-child td {
  border-bottom: none;
}

.prize-table tr.top-rank {
  background: var(--color-bg-secondary);
}

.text-right {
  text-align: right;
}

.rank-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.rank-medal {
  font-size: 1.1rem;
}

.rank-number {
  font-weight: 500;
  color: var(--color-text-primary);
}

.amount-cell {
  font-weight: 600;
  color: #059669;
  font-variant-numeric: tabular-nums;
}

.pct-cell {
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

.prize-footer {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-sm) var(--spacing-md);
  margin-top: var(--spacing-sm);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.footer-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.footer-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.footer-value {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
}

.prize-pool-value {
  color: #059669;
}

.footer-divider {
  width: 1px;
  height: 16px;
  background: var(--color-border);
}

/* RTL Support */
[dir="rtl"] .rank-cell {
  flex-direction: row-reverse;
}

[dir="rtl"] .text-right {
  text-align: left;
}

[dir="rtl"] .prize-footer {
  flex-direction: row-reverse;
}

[dir="rtl"] .footer-item {
  flex-direction: row-reverse;
}

/* Mobile */
@media (max-width: 767px) {
  .prize-table th,
  .prize-table td {
    padding: 6px 8px;
  }

  .prize-footer {
    flex-wrap: wrap;
    gap: var(--spacing-sm);
  }

  .footer-divider {
    display: none;
  }
}
</style>
