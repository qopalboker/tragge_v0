import { ref, computed, watch, onMounted, onUnmounted, type Ref, type ComputedRef } from 'vue';

export interface TimeRemaining {
  days: number;
  hours: number;
  minutes: number;
  seconds: number;
  totalMs: number;
  isExpired: boolean;
}

export interface CountdownState {
  /** Normal countdown state */
  normal: boolean;
  /** Less than warningThreshold remaining */
  warning: boolean;
  /** Less than criticalThreshold remaining */
  critical: boolean;
  /** Countdown has ended */
  expired: boolean;
}

export interface UseCountdownOptions {
  /** The target date to count down to */
  targetDate: Ref<Date | string | number> | Date | string | number;
  /** Server time delta in milliseconds (serverTime - clientTime) for time sync */
  serverTimeDelta?: Ref<number> | number;
  /** Threshold in ms for warning state (default: 5 minutes) */
  warningThreshold?: number;
  /** Threshold in ms for critical state (default: 1 minute) */
  criticalThreshold?: number;
  /** Update interval in ms (default: 1000) */
  interval?: number;
  /** Callback when countdown completes */
  onComplete?: () => void;
  /** Callback when warning threshold is reached */
  onWarning?: () => void;
  /** Callback when critical threshold is reached */
  onCritical?: () => void;
  /** Callback for milestone announcements (for accessibility) */
  onMilestone?: (milestone: string) => void;
}

export interface UseCountdownReturn {
  /** Time remaining breakdown */
  timeRemaining: ComputedRef<TimeRemaining>;
  /** Current state of the countdown */
  state: ComputedRef<CountdownState>;
  /** Formatted display for compact mode (e.g., "2d 05:32:17") */
  compactDisplay: ComputedRef<string>;
  /** Formatted display for minimal mode (e.g., "5h 32m" or "2d 5h") */
  minimalDisplay: ComputedRef<string>;
  /** Whether the countdown is running */
  isRunning: Ref<boolean>;
  /** Previous time values for animation tracking */
  previousValues: Ref<TimeRemaining | null>;
  /** Start the countdown */
  start: () => void;
  /** Stop the countdown */
  stop: () => void;
  /** Reset the countdown */
  reset: () => void;
  /** Current announcement for screen readers */
  announcement: Ref<string>;
}

const FIVE_MINUTES = 5 * 60 * 1000;
const ONE_MINUTE = 60 * 1000;
const ONE_HOUR = 60 * 60 * 1000;
const ONE_DAY = 24 * 60 * 60 * 1000;

function toNumber(value: Ref<number> | number): number {
  return typeof value === 'number' ? value : value.value;
}

function toDate(value: Ref<Date | string | number> | Date | string | number): Date {
  const raw = typeof value === 'object' && 'value' in value ? value.value : value;
  if (raw instanceof Date) return raw;
  if (typeof raw === 'number') return new Date(raw);
  return new Date(raw);
}

function padNumber(n: number): string {
  return n.toString().padStart(2, '0');
}

export function useCountdown(options: UseCountdownOptions): UseCountdownReturn {
  const {
    targetDate,
    serverTimeDelta = 0,
    warningThreshold = FIVE_MINUTES,
    criticalThreshold = ONE_MINUTE,
    interval = 1000,
    onComplete,
    onWarning,
    onCritical,
    onMilestone,
  } = options;

  const isRunning = ref(false);
  const previousValues = ref<TimeRemaining | null>(null);
  const announcement = ref('');

  // Internal state for tracking callbacks
  let intervalId: ReturnType<typeof setInterval> | null = null;
  let hasCalledWarning = false;
  let hasCalledCritical = false;
  let hasCalledComplete = false;
  let lastMilestone = '';

  function getCurrentTime(): number {
    return Date.now() + toNumber(serverTimeDelta);
  }

  function calculateTimeRemaining(): TimeRemaining {
    const target = toDate(targetDate).getTime();
    const current = getCurrentTime();
    const difference = target - current;

    if (difference <= 0) {
      return {
        days: 0,
        hours: 0,
        minutes: 0,
        seconds: 0,
        totalMs: 0,
        isExpired: true,
      };
    }

    const days = Math.floor(difference / ONE_DAY);
    const hours = Math.floor((difference % ONE_DAY) / ONE_HOUR);
    const minutes = Math.floor((difference % ONE_HOUR) / ONE_MINUTE);
    const seconds = Math.floor((difference % ONE_MINUTE) / 1000);

    return {
      days,
      hours,
      minutes,
      seconds,
      totalMs: difference,
      isExpired: false,
    };
  }

  // Reactive time remaining
  const internalTimeRemaining = ref<TimeRemaining>(calculateTimeRemaining());

  const timeRemaining = computed(() => internalTimeRemaining.value);

  const state = computed<CountdownState>(() => {
    const { totalMs, isExpired } = timeRemaining.value;

    if (isExpired) {
      return { normal: false, warning: false, critical: false, expired: true };
    }

    if (totalMs <= criticalThreshold) {
      return { normal: false, warning: false, critical: true, expired: false };
    }

    if (totalMs <= warningThreshold) {
      return { normal: false, warning: true, critical: false, expired: false };
    }

    return { normal: true, warning: false, critical: false, expired: false };
  });

  const compactDisplay = computed(() => {
    const { days, hours, minutes, seconds, isExpired } = timeRemaining.value;

    if (isExpired) {
      return '00:00:00';
    }

    if (days > 0) {
      return `${days}d ${padNumber(hours)}:${padNumber(minutes)}:${padNumber(seconds)}`;
    }

    return `${padNumber(hours)}:${padNumber(minutes)}:${padNumber(seconds)}`;
  });

  const minimalDisplay = computed(() => {
    const { days, hours, minutes, totalMs, isExpired } = timeRemaining.value;

    if (isExpired) {
      return '0m';
    }

    // Less than 1 hour: show minutes
    if (totalMs < ONE_HOUR) {
      return `${minutes}m`;
    }

    // Less than 1 day: show hours and minutes
    if (totalMs < ONE_DAY) {
      return `${hours}h ${minutes}m`;
    }

    // 1 day or more: show days and hours
    return `${days}d ${hours}h`;
  });

  function getMilestone(totalMs: number): string {
    // Announce milestones at key intervals
    if (totalMs <= 0) return 'expired';
    if (totalMs <= 10 * 1000) return '10s';
    if (totalMs <= 30 * 1000) return '30s';
    if (totalMs <= ONE_MINUTE) return '1m';
    if (totalMs <= 5 * ONE_MINUTE) return '5m';
    if (totalMs <= 10 * ONE_MINUTE) return '10m';
    if (totalMs <= 30 * ONE_MINUTE) return '30m';
    if (totalMs <= ONE_HOUR) return '1h';
    return '';
  }

  function tick(): void {
    // Store previous values for animation tracking
    previousValues.value = { ...internalTimeRemaining.value };

    // Update current time
    internalTimeRemaining.value = calculateTimeRemaining();

    const { totalMs, isExpired } = internalTimeRemaining.value;

    // Handle complete callback
    if (isExpired && !hasCalledComplete) {
      hasCalledComplete = true;
      onComplete?.();
    }

    // Handle warning callback
    if (!isExpired && totalMs <= warningThreshold && !hasCalledWarning) {
      hasCalledWarning = true;
      onWarning?.();
    }

    // Handle critical callback
    if (!isExpired && totalMs <= criticalThreshold && !hasCalledCritical) {
      hasCalledCritical = true;
      onCritical?.();
    }

    // Handle milestone announcements
    const milestone = getMilestone(totalMs);
    if (milestone && milestone !== lastMilestone) {
      lastMilestone = milestone;

      // Create announcement text for screen readers
      if (milestone === 'expired') {
        announcement.value = 'Timer has ended';
      } else {
        announcement.value = `${milestone} remaining`;
      }

      onMilestone?.(milestone);
    }
  }

  function start(): void {
    if (isRunning.value) return;

    isRunning.value = true;
    tick(); // Initial tick

    intervalId = setInterval(tick, interval);
  }

  function stop(): void {
    if (!isRunning.value) return;

    isRunning.value = false;

    if (intervalId) {
      clearInterval(intervalId);
      intervalId = null;
    }
  }

  function reset(): void {
    stop();
    hasCalledWarning = false;
    hasCalledCritical = false;
    hasCalledComplete = false;
    lastMilestone = '';
    previousValues.value = null;
    internalTimeRemaining.value = calculateTimeRemaining();
  }

  // Watch for target date changes
  watch(
    () => toDate(targetDate).getTime(),
    () => {
      reset();
      start();
    }
  );

  // Auto-start on mount
  onMounted(() => {
    start();
  });

  // Cleanup on unmount
  onUnmounted(() => {
    stop();
  });

  return {
    timeRemaining,
    state,
    compactDisplay,
    minimalDisplay,
    isRunning,
    previousValues,
    start,
    stop,
    reset,
    announcement,
  };
}
