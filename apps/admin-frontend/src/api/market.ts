import { api } from '@/api';

export interface SymbolStatus {
  symbol: string;
  bid: number;
  ask: number;
  last: number;
  ts: number;
  age_ms: number;
  provider: string;
  status: 'fresh' | 'warning' | 'stale' | 'no_data';
  category: string;
}

export interface MarketStatus {
  active_provider: string;
  active_crypto_provider: string;
  available_providers: string[];
  using_fallback: boolean;
  massive_status: Record<string, string>;
  twelvedata_status: Record<string, string>;
  nobitex_status: Record<string, unknown>;
  binance_status: Record<string, unknown>;
  symbols_total: number;
  symbols_receiving: number;
  symbols_stale: number;
  ready: boolean;
  symbols: SymbolStatus[];
}

export interface ProviderStats {
  enabled: boolean;
  connected: boolean;
  tick_count: number;
  error_count: number;
  last_tick: number;
  symbols: number;
  poll_count?: number;
  last_error?: string;
}

export interface ProviderConfig {
  crypto: {
    active: 'nobitex' | 'binance' | 'both';
    available: string[];
    nobitex: ProviderStats;
    binance: ProviderStats;
  };
  forex: {
    active: string;
    available: string[];
    using_fallback?: boolean;
  };
}

export interface PriceData {
  last: number;
  bid: number;
  ask: number;
  ts: number;
}

export async function getMarketStatus(): Promise<MarketStatus> {
  const response = await api.get<MarketStatus>('/api/admin/market/data/status');
  return response.data;
}

export async function getMarketPrices(): Promise<Record<string, PriceData>> {
  const response = await api.get<Record<string, PriceData>>('/api/admin/market/data/prices');
  return response.data;
}

export async function switchProvider(provider: string): Promise<void> {
  await api.post('/api/admin/market/data/switch-provider', { provider });
}

export async function reconnectProvider(): Promise<void> {
  await api.post('/api/admin/market/data/reconnect');
}

export async function getProviderConfig(): Promise<ProviderConfig> {
  const response = await api.get<ProviderConfig>('/api/admin/providerconfig');

  return response.data;
}

export async function switchCryptoProvider(provider: string): Promise<void> {
  await api.post('/api/admin/market/data/crypto-provider', { provider });
}

export async function switchForexProvider(provider: string): Promise<void> {
  await api.post('/api/admin/market/data/forex-provider', { provider });
}
