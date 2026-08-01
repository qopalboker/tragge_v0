// Thin wrapper: the store itself lives in @tragge/frontend-shared.
// Re-exported here so existing `@/stores/theme` imports keep working.
export {
  useThemeStore,
  themes,
  type Theme,
  type ResolvedTheme,
  type ThemeId,
  type ThemeColors,
} from '@tragge/frontend-shared';
