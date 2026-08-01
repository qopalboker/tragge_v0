// Number formatting utilities shared by user-frontend and admin-frontend.
// No imports — pure data in, formatted string out.

export interface FormatOptions {
  locale?: 'en-US' | 'fa-IR';
  decimals?: number;
  showSign?: boolean;
  currency?: boolean;
}

export function formatScore(value: number, options: FormatOptions = {}): string {
  const { locale = 'en-US', decimals = 2, showSign = true, currency = false } = options;

  const formatter = new Intl.NumberFormat(locale, {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  });

  const formatted = formatter.format(Math.abs(value));
  const sign = showSign ? (value >= 0 ? '+' : '-') : value < 0 ? '-' : '';
  const prefix = currency ? '$' : '';

  return `${sign}${prefix}${formatted}`;
}

export function formatCurrency(value: number, locale: 'en-US' | 'fa-IR' = 'en-US'): string {
  return formatScore(value, { locale, decimals: 2, showSign: true, currency: true });
}

export function formatPercent(value: number, locale: 'en-US' | 'fa-IR' = 'en-US'): string {
  const formatter = new Intl.NumberFormat(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });

  const sign = value >= 0 ? '+' : '';
  return `${sign}${formatter.format(value)}%`;
}

export function formatNumber(value: number, decimals = 2, locale: 'en-US' | 'fa-IR' = 'en-US'): string {
  return new Intl.NumberFormat(locale, {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  }).format(value);
}

export function formatRank(rank: number | null): string {
  if (rank === null || rank === undefined) return '-';
  return `#${rank}`;
}

export function getPnLColorClass(value: number): string {
  if (value > 0) return 'text-emerald-400';
  if (value < 0) return 'text-rose-400';
  return 'text-slate-400';
}

export function getPnLGlowClass(value: number): string {
  if (value > 0) return 'neon-green';
  if (value < 0) return 'neon-red';
  return '';
}

export function getPnLBgClass(value: number, type: 'badge' | 'border' | 'bg' = 'badge'): string {
  if (type === 'badge') {
    if (value > 0) return 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30';
    if (value < 0) return 'bg-rose-500/20 text-rose-400 border-rose-500/30';
    return 'bg-slate-700 text-slate-400 border-slate-600';
  }
  if (type === 'border') {
    if (value > 0) return 'border-emerald-500/20';
    if (value < 0) return 'border-rose-500/20';
    return 'border-slate-700/50';
  }
  if (type === 'bg') {
    if (value > 0) return 'from-emerald-500/10 via-cyan-500/5 to-purple-500/10';
    if (value < 0) return 'from-rose-500/10 via-orange-500/5 to-rose-500/10';
    return 'from-slate-700/10 via-slate-600/5 to-slate-700/10';
  }
  return '';
}
