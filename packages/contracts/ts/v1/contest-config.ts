import type { AssetClass, ContestDurationType } from './enums';

/**
 * Contest template for quick contest creation.
 */
export interface ContestTemplate {
  /** Unique identifier for the template */
  key: string;
  /** Display name */
  name: string;
  /** Description of the contest type */
  description: string;
  /** Duration category */
  duration_type: ContestDurationType;
  /** Duration in minutes */
  duration_minutes: number;
  /** Entry fee in cents (500 = $5) */
  entry_fee_cents: number;
  /** Platform commission as percentage (20.00 = 20%) */
  commission_rate: number;
  /** Starting virtual currency allocation */
  qty_allocation: number;
  /** Asset category */
  asset_class: AssetClass;
  /** List of allowed trading symbols */
  symbols: string[];
  /** Maximum participants (0 = unlimited) */
  max_participants: number;
  /** Minimum required to start */
  min_participants: number;
  /** Whether this is a free practice contest */
  is_free: boolean;
  /** Auto-start when conditions are met */
  auto_start: boolean;
}

/**
 * Contest configuration for custom contests.
 */
export interface ContestConfig {
  duration_type: ContestDurationType;
  duration_minutes?: number;
  entry_fee_cents: number;
  commission_rate: number;
  qty_allocation: number;
  asset_class: AssetClass;
  symbols?: string[];
  max_participants?: number;
  min_participants: number;
  is_free: boolean;
  auto_start: boolean;
}

/**
 * Full contest response from API.
 */
export interface Contest {
  id: string;
  name: string;
  description?: string;
  starts_at: string;
  ends_at: string;
  status: string;
  entry_fee_cents: number;
  platform_fee_bps: number;
  qty_total: number;
  duration_type: ContestDurationType;
  asset_class: AssetClass;
  duration_minutes: number;
  min_participants: number;
  max_participants?: number;
  registration_deadline?: string;
  auto_start: boolean;
  commission_rate: number;
  is_free: boolean;
  template_id?: string;
  participant_count?: number;
  symbols?: ContestSymbol[];
  created_at: string;
}

/**
 * Contest symbol.
 */
export interface ContestSymbol {
  symbol: string;
  enabled: boolean;
}

/**
 * Request to create a contest from template.
 */
export interface CreateContestFromTemplateRequest {
  template_key: string;
  name: string;
  starts_at: string;
  description?: string;
  entry_fee_cents?: number;
  max_participants?: number;
  registration_deadline?: string;
  symbols?: string[];
}

/**
 * Request to create a custom contest.
 */
export interface CreateContestRequest {
  name: string;
  starts_at: string;
  ends_at: string;
  entry_fee_cents: number;
  platform_fee_bps: number;
  qty_total: number;
  status?: string;
  description?: string;
  duration_type?: ContestDurationType;
  asset_class?: AssetClass;
  duration_minutes?: number;
  min_participants?: number;
  max_participants?: number;
  registration_deadline?: string;
  auto_start?: boolean;
  commission_rate?: number;
  is_free?: boolean;
  symbols?: string[];
}

// Default symbol sets for each asset class
export const FOREX_MAJOR_PAIRS = [
  'EUR/USD', 'GBP/USD', 'USD/JPY', 'USD/CHF', 'AUD/USD',
];

export const FOREX_EXTENDED_PAIRS = [
  'EUR/USD', 'GBP/USD', 'USD/JPY', 'USD/CHF', 'AUD/USD',
  'NZD/USD', 'USD/CAD', 'EUR/GBP', 'EUR/JPY', 'GBP/JPY',
  'AUD/JPY', 'CHF/JPY', 'EUR/AUD', 'GBP/AUD', 'EUR/CAD',
];

export const FOREX_FULL_PAIRS = [
  'EUR/USD', 'GBP/USD', 'USD/JPY', 'USD/CHF', 'AUD/USD',
  'NZD/USD', 'USD/CAD', 'EUR/GBP', 'EUR/JPY', 'GBP/JPY',
  'AUD/JPY', 'CHF/JPY', 'EUR/AUD', 'GBP/AUD', 'EUR/CAD',
  'AUD/NZD', 'CAD/JPY', 'NZD/JPY', 'EUR/CHF', 'GBP/CHF',
  'EUR/NZD', 'GBP/NZD', 'AUD/CAD', 'NZD/CAD', 'SGD/JPY',
  'USD/SGD', 'USD/HKD', 'USD/MXN', 'USD/ZAR', 'EUR/PLN',
  'EUR/TRY', 'USD/TRY', 'GBP/CAD',
];

export const CRYPTO_MAJOR_ASSETS = [
  'BTC/USD', 'ETH/USD', 'SOL/USD', 'DOGE/USD', 'XRP/USD',
];

export const CRYPTO_EXTENDED_ASSETS = [
  'BTC/USD', 'ETH/USD', 'SOL/USD', 'DOGE/USD', 'XRP/USD',
  'ADA/USD', 'AVAX/USD', 'LINK/USD', 'DOT/USD', 'MATIC/USD',
  'SHIB/USD', 'LTC/USD',
];

export const CRYPTO_FULL_ASSETS = [
  'BTC/USD', 'ETH/USD', 'SOL/USD', 'DOGE/USD', 'XRP/USD',
  'ADA/USD', 'AVAX/USD', 'LINK/USD', 'DOT/USD', 'MATIC/USD',
  'SHIB/USD', 'LTC/USD', 'UNI/USD', 'ATOM/USD', 'FIL/USD',
  'TRX/USD', 'ETC/USD', 'XLM/USD', 'VET/USD', 'ALGO/USD',
  'NEAR/USD', 'FTM/USD', 'APE/USD', 'SAND/USD',
];

export const STOCKS_US_TOP30 = [
  'AAPL', 'MSFT', 'GOOGL', 'AMZN', 'TSLA',
  'META', 'NVDA', 'BRK.B', 'JPM', 'JNJ',
  'V', 'PG', 'UNH', 'HD', 'MA',
  'DIS', 'ADBE', 'NFLX', 'CRM', 'PYPL',
  'INTC', 'CSCO', 'VZ', 'KO', 'PFE',
  'MRK', 'ABT', 'WMT', 'NKE', 'XOM',
];

/**
 * Pre-defined contest templates.
 */
export const CONTEST_TEMPLATES: Record<string, ContestTemplate> = {
  // Crypto templates
  crypto_rush_30m: {
    key: 'crypto_rush_30m',
    name: 'Crypto Rush 30min',
    description: 'Fast-paced 30-minute crypto trading competition',
    duration_type: 'rush_30min',
    duration_minutes: 30,
    entry_fee_cents: 500,
    commission_rate: 20.00,
    qty_allocation: 50000,
    asset_class: 'crypto',
    symbols: CRYPTO_MAJOR_ASSETS,
    max_participants: 100,
    min_participants: 2,
    is_free: false,
    auto_start: false,
  },
  crypto_hourly: {
    key: 'crypto_hourly',
    name: 'Crypto Hourly',
    description: 'One-hour crypto trading tournament',
    duration_type: 'hourly',
    duration_minutes: 60,
    entry_fee_cents: 1000,
    commission_rate: 20.00,
    qty_allocation: 100000,
    asset_class: 'crypto',
    symbols: CRYPTO_EXTENDED_ASSETS.slice(0, 8),
    max_participants: 200,
    min_participants: 2,
    is_free: false,
    auto_start: false,
  },
  crypto_daily: {
    key: 'crypto_daily',
    name: 'Crypto Daily Challenge',
    description: 'Full-day crypto trading competition with 20 QTY allocation',
    duration_type: 'daily',
    duration_minutes: 1440,
    entry_fee_cents: 5000,
    commission_rate: 20.00,
    qty_allocation: 200000,
    asset_class: 'crypto',
    symbols: CRYPTO_EXTENDED_ASSETS,
    max_participants: 500,
    min_participants: 5,
    is_free: false,
    auto_start: false,
  },
  crypto_free_practice: {
    key: 'crypto_free_practice',
    name: 'Free Crypto Practice',
    description: 'Free practice tournament to learn crypto trading',
    duration_type: 'hourly',
    duration_minutes: 60,
    entry_fee_cents: 0,
    commission_rate: 0,
    qty_allocation: 100000,
    asset_class: 'crypto',
    symbols: CRYPTO_MAJOR_ASSETS,
    max_participants: 1000,
    min_participants: 2,
    is_free: true,
    auto_start: true,
  },
  crypto_high_stakes: {
    key: 'crypto_high_stakes',
    name: 'High Stakes Crypto',
    description: 'Professional-level crypto trading competition with $100 entry',
    duration_type: 'four_hour',
    duration_minutes: 240,
    entry_fee_cents: 10000,
    commission_rate: 20.00,
    qty_allocation: 200000,
    asset_class: 'crypto',
    symbols: CRYPTO_EXTENDED_ASSETS,
    max_participants: 50,
    min_participants: 5,
    is_free: false,
    auto_start: false,
  },

  // Forex templates
  forex_rush_30m: {
    key: 'forex_rush_30m',
    name: 'Forex Rush 30min',
    description: 'Quick 30-minute forex trading competition',
    duration_type: 'rush_30min',
    duration_minutes: 30,
    entry_fee_cents: 500,
    commission_rate: 20.00,
    qty_allocation: 50000,
    asset_class: 'forex',
    symbols: FOREX_MAJOR_PAIRS,
    max_participants: 100,
    min_participants: 2,
    is_free: false,
    auto_start: false,
  },
  forex_hourly: {
    key: 'forex_hourly',
    name: 'Forex Hourly',
    description: 'One-hour forex trading tournament with 15+ pairs',
    duration_type: 'hourly',
    duration_minutes: 60,
    entry_fee_cents: 1000,
    commission_rate: 20.00,
    qty_allocation: 100000,
    asset_class: 'forex',
    symbols: FOREX_EXTENDED_PAIRS,
    max_participants: 200,
    min_participants: 2,
    is_free: false,
    auto_start: false,
  },
  forex_daily: {
    key: 'forex_daily',
    name: 'Forex Daily Championship',
    description: '24-hour forex trading championship with 33+ pairs',
    duration_type: 'daily',
    duration_minutes: 1440,
    entry_fee_cents: 5000,
    commission_rate: 20.00,
    qty_allocation: 200000,
    asset_class: 'forex',
    symbols: FOREX_FULL_PAIRS,
    max_participants: 500,
    min_participants: 5,
    is_free: false,
    auto_start: false,
  },
  forex_weekly: {
    key: 'forex_weekly',
    name: 'Forex Weekly Grand Prix',
    description: 'Week-long forex competition for serious traders',
    duration_type: 'weekly',
    duration_minutes: 10080,
    entry_fee_cents: 10000,
    commission_rate: 20.00,
    qty_allocation: 500000,
    asset_class: 'forex',
    symbols: FOREX_FULL_PAIRS,
    max_participants: 1000,
    min_participants: 10,
    is_free: false,
    auto_start: false,
  },
  forex_free_practice: {
    key: 'forex_free_practice',
    name: 'Free Forex Practice',
    description: 'Free practice tournament to learn forex trading',
    duration_type: 'hourly',
    duration_minutes: 60,
    entry_fee_cents: 0,
    commission_rate: 0,
    qty_allocation: 100000,
    asset_class: 'forex',
    symbols: FOREX_MAJOR_PAIRS,
    max_participants: 1000,
    min_participants: 2,
    is_free: true,
    auto_start: true,
  },
  forex_high_stakes: {
    key: 'forex_high_stakes',
    name: 'High Stakes Forex',
    description: 'Professional-level forex trading competition with $100 entry',
    duration_type: 'four_hour',
    duration_minutes: 240,
    entry_fee_cents: 10000,
    commission_rate: 20.00,
    qty_allocation: 200000,
    asset_class: 'forex',
    symbols: FOREX_EXTENDED_PAIRS,
    max_participants: 50,
    min_participants: 5,
    is_free: false,
    auto_start: false,
  },

  // Stocks templates
  stocks_daily: {
    key: 'stocks_daily',
    name: 'US Stocks Daily',
    description: 'Daily competition trading top 30 US equities',
    duration_type: 'daily',
    duration_minutes: 1440,
    entry_fee_cents: 5000,
    commission_rate: 20.00,
    qty_allocation: 200000,
    asset_class: 'stocks',
    symbols: STOCKS_US_TOP30,
    max_participants: 500,
    min_participants: 5,
    is_free: false,
    auto_start: false,
  },
};

/**
 * Get a contest template by key.
 */
export function getContestTemplate(key: string): ContestTemplate | undefined {
  return CONTEST_TEMPLATES[key];
}

/**
 * List all contest templates.
 */
export function listContestTemplates(): ContestTemplate[] {
  return Object.values(CONTEST_TEMPLATES);
}

/**
 * List contest templates filtered by asset class.
 */
export function listContestTemplatesByAssetClass(assetClass: AssetClass): ContestTemplate[] {
  return Object.values(CONTEST_TEMPLATES).filter(t => t.asset_class === assetClass);
}

/**
 * List contest templates filtered by duration type.
 */
export function listContestTemplatesByDuration(durationType: ContestDurationType): ContestTemplate[] {
  return Object.values(CONTEST_TEMPLATES).filter(t => t.duration_type === durationType);
}

/**
 * List free practice templates.
 */
export function listFreeContestTemplates(): ContestTemplate[] {
  return Object.values(CONTEST_TEMPLATES).filter(t => t.is_free);
}

/**
 * Get default symbols for a given asset class and duration type.
 * Shorter durations get fewer symbols; longer durations get the full set.
 */
export function getDefaultSymbols(assetClass: AssetClass, durationType: ContestDurationType): string[] {
  switch (assetClass) {
    case 'crypto':
      switch (durationType) {
        case 'rush_30min':
          return CRYPTO_MAJOR_ASSETS;
        case 'hourly':
        case 'four_hour':
          return CRYPTO_EXTENDED_ASSETS;
        case 'daily':
        case 'weekly':
          return CRYPTO_FULL_ASSETS;
        default:
          return CRYPTO_MAJOR_ASSETS;
      }
    case 'forex':
      switch (durationType) {
        case 'rush_30min':
          return FOREX_MAJOR_PAIRS;
        case 'hourly':
        case 'four_hour':
          return FOREX_EXTENDED_PAIRS;
        case 'daily':
        case 'weekly':
          return FOREX_FULL_PAIRS;
        default:
          return FOREX_MAJOR_PAIRS;
      }
    case 'stocks':
      return STOCKS_US_TOP30;
    case 'mixed':
      return [...CRYPTO_MAJOR_ASSETS, ...FOREX_MAJOR_PAIRS];
    default:
      return [];
  }
}
