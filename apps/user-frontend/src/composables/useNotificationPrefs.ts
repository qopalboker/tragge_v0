import { ref } from 'vue';
import { notificationPrefsApi, type NotificationPref } from '@/api/notificationPrefs';

const CACHE_KEY = 'tragge-notification-prefs-v2';

// Category -> notif types mapping (mirrors backend)
const categoryTypes: Record<string, string[]> = {
  contest_reminders: ['contest_starting', 'contest_ending', 'registration_closed'],
  contest_results: ['contest_completed', 'contest_cancelled', 'prize_won', 'contest_paused', 'contest_resumed'],
  contest_activity: ['contest_joined', 'contest_left', 'contest_started'],
  transactions: ['deposit_confirmed', 'deposit_failed', 'withdrawal_update'],
  account: ['kyc_update', 'system'],
};

const preferences = ref<NotificationPref[]>([]);
const categories = ref<string[]>([]);
const channels = ref<string[]>([]);
const loading = ref(false);
const initialized = ref(false);

// Load from cache initially
try {
  const cached = localStorage.getItem(CACHE_KEY);
  if (cached) {
    const parsed = JSON.parse(cached);
    preferences.value = parsed.preferences || [];
    categories.value = parsed.categories || [];
    channels.value = parsed.channels || [];
  }
} catch {
  // Use defaults
}

export function useNotificationPrefs() {

  async function fetchPreferences(): Promise<void> {
    if (loading.value) return;
    loading.value = true;
    try {
      const response = await notificationPrefsApi.getPreferences();
      preferences.value = response.preferences;
      categories.value = response.categories;
      channels.value = response.channels;
      initialized.value = true;
      // Cache
      localStorage.setItem(CACHE_KEY, JSON.stringify(response));
    } catch {
      // Use cache
    } finally {
      loading.value = false;
    }
  }

  async function togglePreference(category: string, channel: string): Promise<void> {
    // Find current state
    const existing = preferences.value.find(p => p.category === category && p.channel === channel);
    const currentEnabled = existing ? existing.enabled : true;
    const newEnabled = !currentEnabled;

    // Optimistic update
    if (existing) {
      existing.enabled = newEnabled;
    } else {
      preferences.value.push({ category, channel, enabled: newEnabled });
    }

    try {
      await notificationPrefsApi.updatePreferences([
        { category, channel, enabled: newEnabled },
      ]);
      // Update cache
      localStorage.setItem(CACHE_KEY, JSON.stringify({
        preferences: preferences.value,
        categories: categories.value,
        channels: channels.value,
      }));
    } catch {
      // Revert
      if (existing) {
        existing.enabled = currentEnabled;
      } else {
        preferences.value = preferences.value.filter(p => !(p.category === category && p.channel === channel));
      }
    }
  }

  function isEnabled(category: string, channel: string): boolean {
    const pref = preferences.value.find(p => p.category === category && p.channel === channel);
    return pref ? pref.enabled : true; // Default: enabled
  }

  function shouldShow(notifType: string): boolean {
    // Find category for this notif type
    for (const [cat, types] of Object.entries(categoryTypes)) {
      if (types.includes(notifType)) {
        return isEnabled(cat, 'in_app');
      }
    }
    return true; // Unknown type: show
  }

  return {
    preferences,
    categories,
    channels,
    loading,
    initialized,
    fetchPreferences,
    togglePreference,
    isEnabled,
    shouldShow,
  };
}
