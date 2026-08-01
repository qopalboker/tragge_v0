import { ref } from 'vue';

export function usePWA() {
  const needRefresh = ref(false);
  const offlineReady = ref(false);
  const canInstall = ref(false);

  function updateServiceWorker() {
    // PWA disabled
  }

  function updateApp() {
    // PWA disabled
  }

  function installApp() {
    // PWA disabled
  }

  function dismissInstall() {
    // PWA disabled
  }

  function dismissUpdate() {
    // PWA disabled
  }

  function dismissOfflineReady() {
    // PWA disabled
  }

  return {
    needRefresh,
    offlineReady,
    canInstall,
    updateServiceWorker,
    updateApp,
    installApp,
    dismissInstall,
    dismissUpdate,
    dismissOfflineReady,
  };
}
