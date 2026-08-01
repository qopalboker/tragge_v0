import { ref, watch, computed, onUnmounted } from 'vue';
import { useTradingStore, type RankMilestone } from '@/stores/trading';
import { useToast } from './useToast';
import { t } from '@/i18n';

export interface RankNotificationOptions {
  /** Enable sound effects for notifications */
  soundEnabled?: boolean;
  /** Volume for sound effects (0-1) */
  soundVolume?: number;
  /** Minimum rank change to show notification */
  minRankChangeForNotification?: number;
  /** Debounce interval in ms to prevent notification spam */
  debounceMs?: number;
}

interface RankNotification {
  id: number;
  type: RankMilestone['type'];
  message: string;
  emoji: string;
  timestamp: number;
}

const defaultOptions: Required<RankNotificationOptions> = {
  soundEnabled: false,
  soundVolume: 0.5,
  minRankChangeForNotification: 1,
  debounceMs: 1000,
};

// Audio context for sound effects (created on demand)
let audioContext: AudioContext | null = null;

function getAudioContext(): AudioContext | null {
  if (typeof window === 'undefined') return null;
  if (!audioContext) {
    try {
      audioContext = new (window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext)();
    } catch {
      console.warn('Web Audio API not available');
      return null;
    }
  }
  return audioContext;
}

function playNotificationSound(type: 'up' | 'down' | 'milestone', volume: number): void {
  const ctx = getAudioContext();
  if (!ctx) return;

  // Resume context if suspended (browser autoplay policy)
  if (ctx.state === 'suspended') {
    ctx.resume();
  }

  const oscillator = ctx.createOscillator();
  const gainNode = ctx.createGain();

  oscillator.connect(gainNode);
  gainNode.connect(ctx.destination);

  // Different sounds for different notification types
  switch (type) {
    case 'up':
      // Rising tone for rank up
      oscillator.type = 'sine';
      oscillator.frequency.setValueAtTime(400, ctx.currentTime);
      oscillator.frequency.exponentialRampToValueAtTime(800, ctx.currentTime + 0.15);
      break;
    case 'down':
      // Falling tone for rank down
      oscillator.type = 'sine';
      oscillator.frequency.setValueAtTime(600, ctx.currentTime);
      oscillator.frequency.exponentialRampToValueAtTime(300, ctx.currentTime + 0.15);
      break;
    case 'milestone':
      // Triumphant chord for milestones
      oscillator.type = 'triangle';
      oscillator.frequency.setValueAtTime(523.25, ctx.currentTime); // C5
      oscillator.frequency.setValueAtTime(659.25, ctx.currentTime + 0.1); // E5
      oscillator.frequency.setValueAtTime(783.99, ctx.currentTime + 0.2); // G5
      break;
  }

  gainNode.gain.setValueAtTime(volume * 0.3, ctx.currentTime);
  gainNode.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.3);

  oscillator.start(ctx.currentTime);
  oscillator.stop(ctx.currentTime + 0.3);
}

export function useRankNotifications(options: RankNotificationOptions = {}) {
  const tradingStore = useTradingStore();
  const toast = useToast();

  const mergedOptions = { ...defaultOptions, ...options };

  // Local state
  const notifications = ref<RankNotification[]>([]);
  const lastNotificationTime = ref(0);
  let nextId = 1;

  // User preferences
  const soundEnabled = ref(mergedOptions.soundEnabled);
  const soundVolume = ref(mergedOptions.soundVolume);

  // Computed values for display
  const userRank = computed(() => tradingStore.userRank);
  const rankChange = computed(() => tradingStore.userRankChange);
  const highestRank = computed(() => tradingStore.highestRank);
  const lowestRank = computed(() => tradingStore.lowestRank);
  const rankHistory = computed(() => tradingStore.rankHistory);
  const totalParticipants = computed(() => tradingStore.totalParticipants);
  const isInPrizeZone = computed(() => tradingStore.isInPrizeZone());
  const lastMilestone = computed(() => tradingStore.lastRankMilestone);

  // Computed rank percentile
  const rankPercentile = computed(() => {
    if (userRank.value === null || totalParticipants.value === 0) return null;
    return Math.round((1 - (userRank.value - 1) / totalParticipants.value) * 100);
  });

  // Format rank for display
  function formatRank(rank: number | null): string {
    if (rank === null) return '-';
    return `#${rank}`;
  }

  // Get emoji for rank change
  function getRankChangeEmoji(change: number): string {
    if (change > 5) return '🚀';
    if (change > 0) return '⬆️';
    if (change < -5) return '📉';
    if (change < 0) return '⬇️';
    return '➡️';
  }

  // Get emoji for milestone type
  function getMilestoneEmoji(type: RankMilestone['type']): string {
    switch (type) {
      case 'first_place': return '👑';
      case 'top_10': return '🏆';
      case 'prize_zone': return '💰';
      case 'rank_up': return '🎉';
      case 'rank_down': return '📉';
      default: return '📊';
    }
  }

  // Generate notification message
  function getMilestoneMessage(milestone: RankMilestone): string {
    const { type, rank, previousRank } = milestone;

    switch (type) {
      case 'first_place':
        return t('rank.firstPlace');
      case 'top_10':
        return t('rank.topTen', { rank: rank.toString() });
      case 'prize_zone':
        return t('rank.prizeZone');
      case 'rank_up':
        if (previousRank !== undefined) {
          const change = previousRank - rank;
          return t('rank.movedUp', { rank: rank.toString(), change: change.toString() });
        }
        return t('rank.movedUpTo', { rank: rank.toString() });
      case 'rank_down':
        return t('rank.droppedTo', { rank: rank.toString() });
      default:
        return t('rank.updated', { rank: rank.toString() });
    }
  }

  // Show notification
  function showNotification(milestone: RankMilestone): void {
    const now = Date.now();

    // Debounce notifications
    if (now - lastNotificationTime.value < mergedOptions.debounceMs) {
      return;
    }

    // Check minimum rank change for up/down notifications
    if ((milestone.type === 'rank_up' || milestone.type === 'rank_down') && milestone.previousRank) {
      const change = Math.abs(milestone.previousRank - milestone.rank);
      if (change < mergedOptions.minRankChangeForNotification) {
        return;
      }
    }

    lastNotificationTime.value = now;

    const emoji = getMilestoneEmoji(milestone.type);
    const message = getMilestoneMessage(milestone);

    // Add to local notifications history
    const notification: RankNotification = {
      id: nextId++,
      type: milestone.type,
      message,
      emoji,
      timestamp: now,
    };
    notifications.value.unshift(notification);

    // Limit notification history
    if (notifications.value.length > 50) {
      notifications.value = notifications.value.slice(0, 50);
    }

    // Show toast
    const toastType = milestone.type === 'rank_down' ? 'warning' : 'success';
    toast.show(`${emoji} ${message}`, toastType, 5000);

    // Play sound if enabled
    if (soundEnabled.value) {
      let soundType: 'up' | 'down' | 'milestone' = 'up';
      if (milestone.type === 'rank_down') {
        soundType = 'down';
      } else if (milestone.type === 'first_place' || milestone.type === 'top_10' || milestone.type === 'prize_zone') {
        soundType = 'milestone';
      }
      playNotificationSound(soundType, soundVolume.value);
    }

    // Clear the milestone from store
    tradingStore.clearLastMilestone();
  }

  // Watch for milestone changes
  const stopWatch = watch(
    () => tradingStore.lastRankMilestone,
    (milestone) => {
      if (milestone) {
        showNotification(milestone);
      }
    },
    { immediate: true }
  );

  // Cleanup
  onUnmounted(() => {
    stopWatch();
  });

  // Toggle sound
  function toggleSound(): void {
    soundEnabled.value = !soundEnabled.value;
  }

  // Set sound volume
  function setVolume(volume: number): void {
    soundVolume.value = Math.max(0, Math.min(1, volume));
  }

  // Manually trigger a test notification
  function testNotification(type: RankMilestone['type'] = 'rank_up'): void {
    showNotification({
      type,
      rank: userRank.value ?? 1,
      previousRank: type === 'rank_up' ? (userRank.value ?? 1) + 3 : (userRank.value ?? 1) - 3,
      timestamp: Date.now(),
    });
  }

  // Clear all notifications
  function clearNotifications(): void {
    notifications.value = [];
  }

  return {
    // State
    notifications,
    soundEnabled,
    soundVolume,

    // Computed
    userRank,
    rankChange,
    highestRank,
    lowestRank,
    rankHistory,
    totalParticipants,
    isInPrizeZone,
    lastMilestone,
    rankPercentile,

    // Functions
    formatRank,
    getRankChangeEmoji,
    getMilestoneEmoji,
    toggleSound,
    setVolume,
    testNotification,
    clearNotifications,
  };
}
