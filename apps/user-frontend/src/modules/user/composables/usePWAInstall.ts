import { ref, computed, onMounted, onUnmounted } from 'vue';

// Type for beforeinstallprompt event
interface BeforeInstallPromptEvent extends Event {
  prompt(): Promise<void>;
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed'; platform: string }>;
}

// localStorage keys
const STORAGE_KEYS = {
  DISMISSED_TIMESTAMP: 'pwa_install_dismissed_at',
  IS_INSTALLED: 'pwa_is_installed',
  PAGE_VIEWS: 'pwa_page_views',
};

// How many days to wait after user dismisses the prompt
const DISMISS_DURATION_DAYS = 7;

// Minimum page views before showing install prompt
const MIN_PAGE_VIEWS = 2;

export function usePWAInstall() {
  // State
  const deferredPrompt = ref<BeforeInstallPromptEvent | null>(null);
  const isInstalled = ref(false);
  const isIOSDevice = ref(false);
  const isIOSSafari = ref(false);
  const showIOSModal = ref(false);
  const pageViews = ref(0);

  // Check if prompt was dismissed recently (within DISMISS_DURATION_DAYS)
  const wasDismissedRecently = (): boolean => {
    const dismissedAt = localStorage.getItem(STORAGE_KEYS.DISMISSED_TIMESTAMP);
    if (!dismissedAt) return false;

    const dismissedDate = new Date(parseInt(dismissedAt, 10));
    const now = new Date();
    const daysDiff = (now.getTime() - dismissedDate.getTime()) / (1000 * 60 * 60 * 24);

    return daysDiff < DISMISS_DURATION_DAYS;
  };

  // Check if user has engaged enough (2+ page views)
  const hasEngagedEnough = computed(() => pageViews.value >= MIN_PAGE_VIEWS);

  // Can show install prompt (not installed, not dismissed, engaged enough)
  const canShowInstallPrompt = computed(() => {
    return (
      !isInstalled.value &&
      !wasDismissedRecently() &&
      hasEngagedEnough.value &&
      (deferredPrompt.value !== null || isIOSSafari.value)
    );
  });

  // Is using standalone mode (already installed as PWA)
  const isStandalone = computed(() => {
    return (
      window.matchMedia('(display-mode: standalone)').matches ||
      (window.navigator as unknown as { standalone?: boolean }).standalone === true ||
      document.referrer.includes('android-app://')
    );
  });

  // Detect iOS device
  const detectIOS = (): boolean => {
    const userAgent = window.navigator.userAgent.toLowerCase();
    return /iphone|ipad|ipod/.test(userAgent);
  };

  // Detect if running in iOS Safari (not in-app browser)
  const detectIOSSafari = (): boolean => {
    const userAgent = window.navigator.userAgent.toLowerCase();
    const isIOS = detectIOS();
    const isSafari = /safari/.test(userAgent) && !/chrome|crios|fxios|opios|edgios/.test(userAgent);
    return isIOS && isSafari;
  };

  // Handle beforeinstallprompt event
  const handleBeforeInstallPrompt = (event: Event) => {
    // Prevent Chrome 67+ from showing the mini-infobar
    event.preventDefault();
    // Store the event for later use
    deferredPrompt.value = event as BeforeInstallPromptEvent;

  };

  // Handle appinstalled event
  const handleAppInstalled = () => {
    isInstalled.value = true;
    deferredPrompt.value = null;
    localStorage.setItem(STORAGE_KEYS.IS_INSTALLED, 'true');

    // Track installation (placeholder for analytics)
    trackInstallation();
  };

  // Track installation for analytics
  const trackInstallation = () => {
    // You can add analytics tracking here
    // Example: gtag('event', 'pwa_install', { ... });
  };

  // Trigger the install prompt
  const promptInstall = async (): Promise<boolean> => {
    if (!deferredPrompt.value) {
      console.warn('[PWA Install] No deferred prompt available');
      return false;
    }

    try {
      // Show the prompt
      await deferredPrompt.value.prompt();

      // Wait for user choice
      const { outcome } = await deferredPrompt.value.userChoice;

      if (outcome === 'accepted') {
        isInstalled.value = true;
        localStorage.setItem(STORAGE_KEYS.IS_INSTALLED, 'true');
      }

      // Clear the deferred prompt (can only be used once)
      deferredPrompt.value = null;

      return outcome === 'accepted';
    } catch (error) {
      console.error('[PWA Install] Error prompting install:', error);
      return false;
    }
  };

  // Dismiss the install prompt (hide for DISMISS_DURATION_DAYS days)
  const dismissPrompt = () => {
    localStorage.setItem(STORAGE_KEYS.DISMISSED_TIMESTAMP, Date.now().toString());
  };

  // Show iOS installation instructions modal
  const showIOSInstructions = () => {
    showIOSModal.value = true;
  };

  // Hide iOS installation instructions modal
  const hideIOSInstructions = () => {
    showIOSModal.value = false;
  };

  // Dismiss iOS prompt and hide for 7 days
  const dismissIOSPrompt = () => {
    dismissPrompt();
    hideIOSInstructions();
  };

  // Increment page views
  const incrementPageViews = () => {
    const currentViews = parseInt(localStorage.getItem(STORAGE_KEYS.PAGE_VIEWS) || '0', 10);
    const newViews = currentViews + 1;
    localStorage.setItem(STORAGE_KEYS.PAGE_VIEWS, newViews.toString());
    pageViews.value = newViews;
  };

  // Initialize state from localStorage
  const initializeState = () => {
    // Check if already installed
    isInstalled.value =
      localStorage.getItem(STORAGE_KEYS.IS_INSTALLED) === 'true' || isStandalone.value;

    // Check iOS
    isIOSDevice.value = detectIOS();
    isIOSSafari.value = detectIOSSafari();

    // Get page views
    pageViews.value = parseInt(localStorage.getItem(STORAGE_KEYS.PAGE_VIEWS) || '0', 10);

    // Increment page views on mount
    incrementPageViews();

    // If running in standalone mode, mark as installed
    if (isStandalone.value) {
      localStorage.setItem(STORAGE_KEYS.IS_INSTALLED, 'true');
    }
  };

  // Lifecycle hooks
  onMounted(() => {
    initializeState();

    // Listen for beforeinstallprompt
    window.addEventListener('beforeinstallprompt', handleBeforeInstallPrompt);

    // Listen for appinstalled
    window.addEventListener('appinstalled', handleAppInstalled);

  });

  onUnmounted(() => {
    window.removeEventListener('beforeinstallprompt', handleBeforeInstallPrompt);
    window.removeEventListener('appinstalled', handleAppInstalled);
  });

  return {
    // State
    deferredPrompt,
    isInstalled,
    isIOSDevice,
    isIOSSafari,
    showIOSModal,
    pageViews,

    // Computed
    canShowInstallPrompt,
    hasEngagedEnough,
    isStandalone,

    // Actions
    promptInstall,
    dismissPrompt,
    showIOSInstructions,
    hideIOSInstructions,
    dismissIOSPrompt,
    incrementPageViews,
  };
}
