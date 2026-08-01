import { api } from './index';

// Types
export interface Symbol {
  symbol: string;
  name: string;
  asset_type: 'stock' | 'crypto' | 'forex' | 'commodity';
  provider_symbol_twelvedata?: string;
  provider_symbol_massive?: string;
  provider_symbol_finnhub?: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface SymbolListResponse {
  symbols: Symbol[];
  total: number;
  limit: number;
  offset: number;
}

export interface SymbolListParams {
  limit?: number;
  offset?: number;
  asset_type?: string;
  is_active?: string;
  search?: string;
}

export interface CreateSymbolRequest {
  symbol: string;
  name: string;
  asset_type: 'stock' | 'crypto' | 'forex' | 'commodity';
  provider_symbol_twelvedata?: string;
  provider_symbol_massive?: string;
  provider_symbol_finnhub?: string;
  is_active?: boolean;
}

export interface UpdateSymbolRequest {
  name?: string;
  asset_type?: 'stock' | 'crypto' | 'forex' | 'commodity';
  provider_symbol_twelvedata?: string;
  provider_symbol_massive?: string;
  provider_symbol_finnhub?: string;
  is_active?: boolean;
}

// API functions

export async function getSymbols(params: SymbolListParams = {}): Promise<SymbolListResponse> {
  const searchParams = new URLSearchParams();
  if (params.limit) searchParams.append('limit', params.limit.toString());
  if (params.offset) searchParams.append('offset', params.offset.toString());
  if (params.asset_type) searchParams.append('asset_type', params.asset_type);
  if (params.is_active !== undefined) searchParams.append('is_active', params.is_active);
  if (params.search) searchParams.append('search', params.search);

  const queryString = searchParams.toString();
  const url = queryString ? `/api/admin/symbols?${queryString}` : '/api/admin/symbols';

  const response = await api.get<SymbolListResponse>(url);
  return response.data;
}

export async function getSymbol(symbol: string): Promise<Symbol> {
  const response = await api.get<Symbol>(`/api/admin/symbols/${encodeURIComponent(symbol)}`);
  return response.data;
}

export async function createSymbol(data: CreateSymbolRequest): Promise<Symbol> {
  const response = await api.post<Symbol>('/api/admin/symbols', data);
  return response.data;
}

export async function updateSymbol(symbol: string, data: UpdateSymbolRequest): Promise<Symbol> {
  const response = await api.put<Symbol>(`/api/admin/symbols/${encodeURIComponent(symbol)}`, data);
  return response.data;
}

export async function toggleSymbolActive(symbol: string, isActive: boolean): Promise<Symbol> {
  return updateSymbol(symbol, { is_active: isActive });
}
