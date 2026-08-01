<script setup lang="ts">
import { computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import LeaderboardPanel from '@/components/LeaderboardPanel.vue';

const route = useRoute();
const router = useRouter();

const contestId = computed(() => route.params.contestId as string);

function goBack() {
  router.push({ name: 'trading', params: { contestId: contestId.value } });
}
</script>

<template>
  <div class="leaderboard-page">
    <!-- Navigation Bar -->
    <div class="nav-bar">
      <button class="back-btn" @click="goBack">
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"/>
        </svg>
        <span>{{ t('common.back') }}</span>
      </button>
      <h1 class="page-title">{{ t('leaderboard.title') }}</h1>
      <div class="spacer"></div>
    </div>

    <!-- Main Content -->
    <div class="content-container">
      <LeaderboardPanel :contestId="contestId" />
    </div>
  </div>
</template>

<style scoped>
.leaderboard-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background-color: var(--color-bg-primary);
  color: white;
}

/* Navigation Bar */
.nav-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  background: linear-gradient(180deg, rgba(20, 25, 38, 0.98), rgba(15, 20, 32, 0.95));
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.back-btn {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 0.5rem;
  color: var(--color-text-secondary);
  font-size: 0.875rem;
  transition: all 0.2s;
}

.back-btn:hover {
  background: rgba(255, 255, 255, 0.1);
  color: white;
  border-color: rgba(16, 185, 129, 0.3);
}

.page-title {
  font-size: 1.125rem;
  font-weight: 600;
  color: white;
  margin: 0;
}

.spacer {
  width: 100px;
}

/* Main Content */
.content-container {
  flex: 1;
  padding: 1rem;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.content-container > * {
  flex: 1;
  min-height: 0;
}

/* Mobile Responsive */
@media (max-width: 768px) {
  .nav-bar {
    padding: 0.5rem 0.75rem;
  }

  .back-btn span {
    display: none;
  }

  .back-btn {
    padding: 0.5rem;
  }

  .page-title {
    font-size: 1rem;
  }

  .spacer {
    width: 40px;
  }

  .content-container {
    padding: 0.5rem;
    padding-bottom: calc(60px + 0.5rem); /* Account for mobile bottom nav */
  }
}
</style>
