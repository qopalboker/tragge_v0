/**
 * MarketStatusEvent represents a market status change event sent via WebSocket.
 * This is used to notify clients when markets open or close.
 */
export interface MarketStatusEvent {
  type: 'market_status';
  asset_class: string;  // "forex", "crypto", "stocks", "commodities", "mixed"
  status: 'open' | 'closed';
  reason?: string;
  reopens_at?: string;  // RFC3339 format when market will reopen
  closes_at?: string;   // RFC3339 format when market will close
  ts: number;
}

/**
 * MarketStatus represents the current status of a market.
 */
export interface MarketStatus {
  asset_class: string;
  is_open: boolean;
  reason?: string;
  next_open?: string;   // RFC3339 format
  next_close?: string;  // RFC3339 format
  override?: string;    // If there's an active override
}

/**
 * MarketOverride represents a manual override for special events.
 */
export interface MarketOverride {
  asset_class: string;
  status: 'open' | 'closed';
  reason: string;
  expires_at: string;   // RFC3339 format
  created_by: string;
  created_at: string;   // RFC3339 format
}

/**
 * MarketTimeSpec represents a specific day and time for market open/close.
 */
export interface MarketTimeSpec {
  day: string;      // Day of week (e.g., "Sunday", "Friday")
  time: string;     // Time in HH:MM format
  timezone: string; // IANA timezone (e.g., "UTC", "America/New_York")
}

/**
 * MarketHoursConfig represents the market hours configuration for an asset class.
 */
export interface MarketHoursConfig {
  asset_class: string;
  open_time?: MarketTimeSpec;   // undefined if always open
  close_time?: MarketTimeSpec;  // undefined if never closes
  always_open: boolean;
  holidays: string[];           // List of holiday dates in YYYY-MM-DD format
}

/**
 * MarketStatusRequest is used by admin to query market status.
 */
export interface MarketStatusRequest {
  asset_class?: string; // If empty, returns all asset classes
}

/**
 * MarketStatusResponse contains the market status for one or more asset classes.
 */
export interface MarketStatusResponse {
  statuses: MarketStatus[];
}

/**
 * SetOverrideRequest is used by admin to set a manual market override.
 */
export interface SetOverrideRequest {
  asset_class: string;
  status: 'open' | 'closed';
  reason: string;
  expires_at: string; // RFC3339 format
}

/**
 * ValidateContestTimesRequest is used to validate contest times against market hours.
 */
export interface ValidateContestTimesRequest {
  asset_class: string;
  starts_at: string; // RFC3339 format
  ends_at: string;   // RFC3339 format
}

/**
 * ValidateContestTimesResponse contains validation results.
 */
export interface ValidateContestTimesResponse {
  valid: boolean;
  reason?: string; // Why validation failed
}

/**
 * Helper to check if a market status event indicates the market is closed.
 */
export function isMarketClosed(event: MarketStatusEvent): boolean {
  return event.status === 'closed';
}

/**
 * Helper to format market reopen time as a human-readable string.
 */
export function formatReopensAt(event: MarketStatusEvent): string | null {
  if (!event.reopens_at) return null;
  try {
    const date = new Date(event.reopens_at);
    return date.toLocaleString();
  } catch {
    return event.reopens_at;
  }
}
