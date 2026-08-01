<script setup lang="ts">
import { ref } from 'vue';
import ContestCard from '@/components/contests/ContestCard.vue';
import { type Contest } from '@/stores/contests';

defineProps<{
  contests: Contest[];
}>();

const scrollContainer = ref<HTMLElement | null>(null);

function scrollLeft(): void {
  if (scrollContainer.value) {
    scrollContainer.value.scrollBy({ left: -300, behavior: 'smooth' });
  }
}

function scrollRight(): void {
  if (scrollContainer.value) {
    scrollContainer.value.scrollBy({ left: 300, behavior: 'smooth' });
  }
}
</script>

<template>
  <div class="carousel-wrapper">
    <button class="carousel-btn carousel-btn-left hide-mobile" @click="scrollLeft">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="15 18 9 12 15 6" />
      </svg>
    </button>

    <div ref="scrollContainer" class="carousel-container">
      <ContestCard
        v-for="contest in contests"
        :key="contest.id"
        :contest="contest"
        compact
        class="carousel-card"
      />
    </div>

    <button class="carousel-btn carousel-btn-right hide-mobile" @click="scrollRight">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="9 18 15 12 9 6" />
      </svg>
    </button>
  </div>
</template>

<style scoped>
.carousel-wrapper {
  position: relative;
}

.carousel-container {
  display: flex;
  gap: var(--spacing-md);
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  scrollbar-width: none;
  -ms-overflow-style: none;
  padding: var(--spacing-xs) 0;
}

.carousel-container::-webkit-scrollbar {
  display: none;
}

.carousel-card {
  flex-shrink: 0;
  width: 280px;
  scroll-snap-align: start;
}

.carousel-btn {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 36px;
  height: 36px;
  border-radius: var(--radius-full);
  border: none;
  background-color: var(--color-bg-primary);
  box-shadow: var(--shadow-md);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--color-text-secondary);
  transition: all var(--transition-fast);
  z-index: 1;
}

.carousel-btn:hover {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.carousel-btn-left {
  left: -18px;
}

.carousel-btn-right {
  right: -18px;
}

[dir="rtl"] .carousel-btn-left {
  left: auto;
  right: -18px;
}

[dir="rtl"] .carousel-btn-right {
  right: auto;
  left: -18px;
}

[dir="rtl"] .carousel-btn svg {
  transform: scaleX(-1);
}
</style>
