// Thin wrapper over @tragge/frontend-shared. The singleton lives in
// the shared package; this module initialises it with the
// admin-frontend's locale messages and re-exports the runtime so the
// existing `import { t } from '@/i18n'` call sites keep working.
//
// The locale files are full copies of the pre-split bundle (admin
// keys kept alongside user-namespaced keys) to preserve Farsi
// translations and avoid accidental drops during the split. Dead
// user-only keys in these files are harmless — they're never
// referenced by admin views and tree-shake out of runtime use.
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
