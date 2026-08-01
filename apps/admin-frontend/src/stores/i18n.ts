// The i18n Pinia store is defined in @tragge/frontend-shared so both panels
// share the same singleton runtime. This thin re-export keeps the existing
// `import { useI18nStore } from '@/stores/i18n'` call sites working without
// forcing every view to learn about the shared package path.
export { useI18nStore } from '@tragge/frontend-shared';
