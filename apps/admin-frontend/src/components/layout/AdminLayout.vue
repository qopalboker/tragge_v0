<script setup lang="ts">
import AppSidebar from './AppSidebar.vue';

// Auth bootstrap runs once in main.ts before mount and is re-checked
// by the router guard, so the layout itself does not need to await
// any auth state — that work is already done by the time we render.
</script>

<template>
  <div class="admin-layout">
    <AppSidebar class="admin-sidebar" />
    <div class="admin-main">
      <main class="admin-content">
        <RouterView />
      </main>
    </div>
  </div>
</template>

<style scoped>
.admin-layout {
  display: flex;
  min-height: 100vh;
  min-height: 100dvh;
  background: var(--theme-bg);
}

.admin-sidebar {
  position: fixed;
  top: 0;
  height: 100vh;
  z-index: 40;
}

.admin-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  margin-inline-start: var(--sidebar-width, 240px);
}

.admin-content {
  flex: 1;
  padding: 24px 32px;
}

@media (max-width: 767px) {
  .admin-main {
    margin-inline-start: 0;
  }
  .admin-content {
    padding: 16px;
  }
}
</style>
