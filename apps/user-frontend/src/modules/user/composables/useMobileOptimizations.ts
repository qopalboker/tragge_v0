import { ref, onMounted, onUnmounted } from 'vue';

// CSS custom property for viewport height
const VH_PROPERTY = '--vh';

export function useMobileOptimizations() {
  const isMobile = ref(false);
  const isIOS = ref(false);
  const viewportHeight = ref(0);

  // Detect mobile device
  const detectMobile = (): boolean => {
    return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(
      navigator.userAgent
    );
  };

  // Detect iOS
  const detectIOS = (): boolean => {
    return /iPhone|iPad|iPod/i.test(navigator.userAgent);
  };

  // Update viewport height CSS variable (fixes iOS Safari 100vh issue)
  const updateViewportHeight = () => {
    // Get the actual viewport height
    const vh = window.innerHeight * 0.01;
    viewportHeight.value = window.innerHeight;

    // Set CSS custom property
    document.documentElement.style.setProperty(VH_PROPERTY, `${vh}px`);
  };

  // Disable pull-to-refresh on specific elements
  const disablePullToRefresh = () => {
    // Add CSS to prevent overscroll behavior
    document.body.style.overscrollBehavior = 'none';

    // For iOS, we need additional handling
    if (isIOS.value) {
      let startY = 0;

      const handleTouchStart = (e: TouchEvent) => {
        startY = e.touches[0].clientY;
      };

      const handleTouchMove = (e: TouchEvent) => {
        const scrollTop = document.documentElement.scrollTop || document.body.scrollTop;
        const currentY = e.touches[0].clientY;
        const deltaY = currentY - startY;

        // Prevent pull-to-refresh when at top of page and pulling down
        if (scrollTop <= 0 && deltaY > 0) {
          const target = e.target as HTMLElement;
          // Allow scrolling in scrollable containers
          if (!target.closest('[data-allow-overscroll]')) {
            e.preventDefault();
          }
        }
      };

      document.addEventListener('touchstart', handleTouchStart, { passive: true });
      document.addEventListener('touchmove', handleTouchMove, { passive: false });

      return () => {
        document.removeEventListener('touchstart', handleTouchStart);
        document.removeEventListener('touchmove', handleTouchMove);
      };
    }

    return () => {};
  };

  // Apply mobile-specific styles
  const applyMobileStyles = () => {
    const style = document.createElement('style');
    style.id = 'mobile-optimizations';
    style.textContent = `
      /* Fix iOS Safari viewport height */
      .full-height {
        height: 100vh;
        height: calc(var(${VH_PROPERTY}, 1vh) * 100);
      }

      /* Optimize touch targets (min 44px as per Apple HIG) */
      .touch-target {
        min-height: 44px;
        min-width: 44px;
      }

      /* Disable text selection on UI elements */
      .no-select {
        -webkit-user-select: none;
        user-select: none;
        -webkit-touch-callout: none;
      }

      /* Safe area insets for notched devices */
      .safe-area-top {
        padding-top: env(safe-area-inset-top, 0);
      }

      .safe-area-bottom {
        padding-bottom: env(safe-area-inset-bottom, 0);
      }

      .safe-area-left {
        padding-left: env(safe-area-inset-left, 0);
      }

      .safe-area-right {
        padding-right: env(safe-area-inset-right, 0);
      }

      .safe-area-all {
        padding-top: env(safe-area-inset-top, 0);
        padding-bottom: env(safe-area-inset-bottom, 0);
        padding-left: env(safe-area-inset-left, 0);
        padding-right: env(safe-area-inset-right, 0);
      }

      /* Prevent body scroll when modal is open */
      body.modal-open {
        overflow: hidden;
        position: fixed;
        width: 100%;
        height: 100%;
      }

      /* iOS momentum scrolling */
      .scroll-container {
        -webkit-overflow-scrolling: touch;
        overflow-y: auto;
      }

      /* Prevent zoom on input focus (iOS) */
      @media screen and (max-width: 767px) {
        input[type="text"],
        input[type="email"],
        input[type="password"],
        input[type="number"],
        input[type="tel"],
        textarea,
        select {
          font-size: 16px !important;
        }
      }

      /* Disable double-tap zoom */
      * {
        touch-action: manipulation;
      }

      /* Hide scrollbar on mobile but keep functionality */
      @media screen and (max-width: 767px) {
        .hide-scrollbar {
          -ms-overflow-style: none;
          scrollbar-width: none;
        }

        .hide-scrollbar::-webkit-scrollbar {
          display: none;
        }
      }
    `;

    // Only add if not already present
    if (!document.getElementById('mobile-optimizations')) {
      document.head.appendChild(style);
    }

    return () => {
      const existingStyle = document.getElementById('mobile-optimizations');
      if (existingStyle) {
        existingStyle.remove();
      }
    };
  };

  // Lock body scroll (for modals)
  const lockBodyScroll = () => {
    const scrollY = window.scrollY;
    document.body.classList.add('modal-open');
    document.body.style.top = `-${scrollY}px`;

    return () => {
      document.body.classList.remove('modal-open');
      document.body.style.top = '';
      window.scrollTo(0, scrollY);
    };
  };

  // Cleanup functions
  let cleanupPullToRefresh: () => void;
  let cleanupStyles: () => void;

  // Handle resize events (for viewport height)
  const handleResize = () => {
    updateViewportHeight();
  };

  // Handle orientation change
  const handleOrientationChange = () => {
    // Delay to wait for iOS to finish rotating
    setTimeout(() => {
      updateViewportHeight();
    }, 100);
  };

  onMounted(() => {
    isMobile.value = detectMobile();
    isIOS.value = detectIOS();

    // Apply optimizations
    updateViewportHeight();
    cleanupStyles = applyMobileStyles();

    if (isMobile.value) {
      cleanupPullToRefresh = disablePullToRefresh();
    }

    // Event listeners
    window.addEventListener('resize', handleResize);
    window.addEventListener('orientationchange', handleOrientationChange);

    // iOS-specific: also update on scroll (address bar show/hide)
    if (isIOS.value) {
      window.addEventListener('scroll', updateViewportHeight, { passive: true });
    }

  });

  onUnmounted(() => {
    window.removeEventListener('resize', handleResize);
    window.removeEventListener('orientationchange', handleOrientationChange);

    if (isIOS.value) {
      window.removeEventListener('scroll', updateViewportHeight);
    }

    if (cleanupPullToRefresh) {
      cleanupPullToRefresh();
    }

    if (cleanupStyles) {
      cleanupStyles();
    }
  });

  return {
    // State
    isMobile,
    isIOS,
    viewportHeight,

    // Actions
    lockBodyScroll,
    updateViewportHeight,
  };
}
