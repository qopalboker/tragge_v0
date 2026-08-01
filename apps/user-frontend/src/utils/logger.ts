// Re-export the shared logger + pre-configured child loggers that
// trade-module code uses. The child logger names are prefix hints, so
// they're more UX sugar than real config and duplicating them here
// (instead of exporting from frontend-shared) keeps the shared
// package audience-agnostic.
import { logger } from '@tragge/frontend-shared';

export { logger };

// Scoped child loggers for the trade module. Additional names can be
// added here without touching frontend-shared.
export const wsLogger = logger.child('WebSocket');
export const tradingLogger = logger.child('TradingWS');
export const chartLogger = logger.child('TradingViewChart');
export const datafeedLogger = logger.child('Datafeed');
export const leaderboardLogger = logger.child('Leaderboard');
