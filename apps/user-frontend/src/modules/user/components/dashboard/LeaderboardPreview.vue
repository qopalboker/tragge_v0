<script setup lang="ts">
import { useRouter } from 'vue-router';
import { t } from '@/i18n';

interface LeaderboardEntry {
  rank: number;
  username: string;
  score: number;
}

defineProps<{
  entries: LeaderboardEntry[];
  userRank?: number;
}>();

const router = useRouter();

function formatScore(score: number): string {
  return score.toLocaleString();
}

function navigateToLeaderboard(): void {
  router.push('/user/leaderboard/global');
}
</script>

<template>
  <div class="leaderboard-preview card" @click="navigateToLeaderboard">
    <table class="leaderboard-table">
      <thead>
        <tr>
          <th class="col-rank">{{ t('leaderboard.rank') }}</th>
          <th class="col-player">{{ t('leaderboard.player') }}</th>
          <th class="col-score">{{ t('leaderboard.score') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="entry in entries"
          :key="entry.rank"
          :class="{ 'highlighted': entry.rank === userRank }"
        >
          <td class="col-rank">
            <span :class="['rank-badge', `rank-${entry.rank}`]">{{ entry.rank }}</span>
          </td>
          <td class="col-player">{{ entry.username }}</td>
          <td class="col-score">{{ formatScore(entry.score) }}</td>
        </tr>
      </tbody>
    </table>
    <div class="leaderboard-footer">
      <span class="view-full-link">{{ t('leaderboard.viewFull') }} →</span>
    </div>
  </div>
</template>

<style scoped>
.leaderboard-preview {
  padding: 0;
  overflow: hidden;
  cursor: pointer;
  transition: box-shadow var(--transition-fast), transform var(--transition-fast);
}

.leaderboard-preview:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

.leaderboard-table {
  width: 100%;
  border-collapse: collapse;
}

.leaderboard-table th,
.leaderboard-table td {
  padding: var(--spacing-sm) var(--spacing-md);
  text-align: left;
}

[dir="rtl"] .leaderboard-table th,
[dir="rtl"] .leaderboard-table td {
  text-align: right;
}

.leaderboard-table th {
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  background-color: var(--color-bg-secondary);
}

.leaderboard-table td {
  font-size: var(--font-size-sm);
  border-bottom: 1px solid var(--color-border-light);
}

.leaderboard-table tr:last-child td {
  border-bottom: none;
}

.highlighted {
  background-color: var(--color-primary-light);
}

.col-rank {
  width: 60px;
}

.col-score {
  text-align: right;
  font-variant-numeric: tabular-nums;
  font-weight: 500;
}

[dir="rtl"] .col-score {
  text-align: left;
}

.rank-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  font-size: var(--font-size-xs);
  font-weight: 600;
  border-radius: var(--radius-full);
  background-color: var(--color-bg-tertiary);
}

.rank-1 {
  background-color: #FEF3C7;
  color: #D97706;
}

.rank-2 {
  background-color: #E5E7EB;
  color: #6B7280;
}

.rank-3 {
  background-color: #FED7AA;
  color: #C2410C;
}

.leaderboard-footer {
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-bg-secondary);
  border-top: 1px solid var(--color-border-light);
  text-align: center;
}

.view-full-link {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-primary);
  transition: color var(--transition-fast);
}

.leaderboard-preview:hover .view-full-link {
  color: var(--color-primary-dark);
}
</style>
