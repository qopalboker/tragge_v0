import { logger } from '@tragge/frontend-shared';

export { logger };
export const wsLogger = logger.child('WebSocket');
export const tradingLogger = logger.child('TradingWS');
export const chartLogger = logger.child('TradingViewChart');
export const datafeedLogger = logger.child('Datafeed');
export const leaderboardLogger = logger.child('Leaderboard');
