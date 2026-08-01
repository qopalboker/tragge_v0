import { reactive, computed } from 'vue';
import type { App } from 'vue';
import commonEn from './common/en';
import commonFa from './common/fa';
import type { Locale, Direction, LocaleMessages, LocaleMessageTree } from './types';

export type { Locale, Direction, LocaleMessages, LocaleMessageTree } from './types';

const LOCALE_STORAGE_KEY = 'tragge-locale';

interface I18nState {
  locale: Locale;
  messages: Record<Locale, LocaleMessages>;
  initialized: boolean;
}

// Module-scoped singleton. Each app calls `initializeI18n(appMessages)`
// exactly once at startup, before any component or store reads from it.
// This mirrors the pre-split layout (one i18n per SPA) and keeps the
// `t`/`setLocale`/etc. call sites unchanged.
const state = reactive<I18nState>({
  locale: 'en',
  messages: { en: { ...commonEn }, fa: { ...commonFa } },
  initialized: false,
});

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function deepMerge(
  target: Record<string, unknown>,
  source: Record<string, unknown>,
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...target };
  for (const [key, value] of Object.entries(source)) {
    const existing = out[key];
    if (isPlainObject(existing) && isPlainObject(value)) {
      out[key] = deepMerge(existing, value);
    } else {
      out[key] = value;
    }
  }
  return out;
}

export const direction = computed<Direction>(() =>
  state.locale === 'fa' ? 'rtl' : 'ltr',
);

export function t(key: string, params?: Record<string, string | number>): string {
  const resolve = (obj: unknown, path: string): string | undefined => {
    const parts = path.split('.');
    let current: unknown = obj;
    for (const part of parts) {
      if (current == null || typeof current !== 'object') return undefined;
      current = (current as Record<string, unknown>)[part];
    }
    return typeof current === 'string' ? current : undefined;
  };

  let message =
    resolve(state.messages[state.locale], key) ||
    resolve(state.messages.en, key) ||
    key;

  if (params) {
    for (const [k, v] of Object.entries(params)) {
      message = message.replace(new RegExp(`{${k}}`, 'g'), String(v));
    }
  }

  return message;
}

function applyDocumentLocale(locale: Locale): void {
  if (typeof document === 'undefined') return;
  document.documentElement.lang = locale;
  document.documentElement.dir = locale === 'fa' ? 'rtl' : 'ltr';
}

export function setLocale(locale: Locale): void {
  state.locale = locale;
  try {
    localStorage.setItem(LOCALE_STORAGE_KEY, locale);
  } catch {
    /* private mode — peer tabs recover on next navigation */
  }
  applyDocumentLocale(locale);
}

export function getLocale(): Locale {
  return state.locale;
}

// Initialize the shared i18n singleton with an app's locale messages.
// Must be called exactly once per app at bootstrap, BEFORE any
// component or store reads from the singleton.
//
// The passed messages are deep-merged over the built-in `common` tree
// (errors, validation, buttons) so app-specific overrides win but no
// common key goes missing.
export function initializeI18n(appMessages: LocaleMessageTree): void {
  state.messages.en = deepMerge(commonEn, appMessages.en);
  state.messages.fa = deepMerge(commonFa, appMessages.fa);

  let saved: string | null = null;
  try {
    saved = localStorage.getItem(LOCALE_STORAGE_KEY);
  } catch {
    /* noop */
  }
  if (saved === 'en' || saved === 'fa') {
    state.locale = saved;
  }
  applyDocumentLocale(state.locale);

  if (typeof window !== 'undefined') {
    window.addEventListener('storage', (event) => {
      if (event.key === LOCALE_STORAGE_KEY && event.newValue) {
        const newLocale = event.newValue as Locale;
        if (newLocale === 'en' || newLocale === 'fa') {
          state.locale = newLocale;
          applyDocumentLocale(newLocale);
        }
      }
    });
  }

  state.initialized = true;
}

// Vue plugin that exposes `$t` globally and an `i18n` injection key for
// composables that prefer injection over direct import.
export const i18n = {
  install(app: App): void {
    app.config.globalProperties.$t = t;
    app.provide('i18n', { t, setLocale, getLocale, direction });
  },
};

export { state };
