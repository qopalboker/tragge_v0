// Thin wrapper over @tragge/frontend-shared. The singleton lives in
// the shared package; this module just initialises it with the
// user-frontend's locale messages and re-exports the runtime so the
// existing `import { t } from '@/i18n'` call sites keep working.
//
// NOTE on locale content: Step 3 kept the pre-split en.ts / fa.ts
// files intact (they still contain admin-namespaced keys like
// `admin.*`, `adminTickets`, etc. that no user-frontend code
// references). Step 4 moves those keys into apps/admin-frontend
// and strips them from here — keeping them now would risk losing
// Farsi translations before the admin-frontend is ready to receive
// them.
import {
  initializeI18n as sharedInit,
  t,
  setLocale,
  getLocale,
  direction,
  i18n,
  type Locale,
  type Direction,
} from '@tragge/frontend-shared';
import en from './locales/en';
import fa from './locales/fa';

export function initializeI18n(): void {
  sharedInit({ en, fa });
}

export { t, setLocale, getLocale, direction, i18n };
export type { Locale, Direction };
