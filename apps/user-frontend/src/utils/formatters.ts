// Thin wrapper: formatters live in @tragge/frontend-shared. Kept here
// so existing `@/utils/formatters` imports continue to resolve.
export {
  formatScore,
  formatCurrency,
  formatPercent,
  formatNumber,
  formatRank,
  getPnLColorClass,
  getPnLGlowClass,
  getPnLBgClass,
  type FormatOptions,
} from '@tragge/frontend-shared';
