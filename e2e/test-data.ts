/**
 * Test data factory for E2E tests
 *
 * All symbols use forex and crypto pairs that are actually supported
 * by the platform (defined in packages/contracts/v1/contest_config.go
 * and infra/docker/docker-compose.yml SYMBOLS env var).
 */

/**
 * Generate a random email for test users
 */
export function generateTestEmail(prefix = 'test'): string {
  const timestamp = Date.now();
  const random = Math.random().toString(36).substring(2, 8);
  return `${prefix}-${timestamp}-${random}@example.com`;
}

/**
 * Generate a random contest name
 */
export function generateContestName(): string {
  const adjectives = ['Grand', 'Ultimate', 'Premier', 'Elite', 'Pro'];
  const nouns = ['Trading', 'Investment', 'Market', 'Crypto', 'Forex'];
  const suffix = ['Cup', 'Challenge', 'Championship', 'Battle', 'Tournament'];

  const adj = adjectives[Math.floor(Math.random() * adjectives.length)];
  const noun = nouns[Math.floor(Math.random() * nouns.length)];
  const suf = suffix[Math.floor(Math.random() * suffix.length)];

  return `${adj} ${noun} ${suf} ${Date.now()}`;
}

/**
 * Test user data
 */
export const TEST_USERS = {
  standard: {
    email: 'test.user@example.com',
    password: 'TestPassword123!',
    name: 'Test User',
  },
  admin: {
    email: 'admin@example.com',
    password: 'AdminPassword123!',
    name: 'Admin User',
  },
  trader: {
    email: 'trader@example.com',
    password: 'TraderPassword123!',
    name: 'Active Trader',
  },
  newUser: {
    email: generateTestEmail('new'),
    password: 'NewUserPass123!',
    name: 'New Test User',
  },
};

/**
 * Test contest data
 */
export const TEST_CONTESTS = {
  active: {
    id: 'contest-active-001',
    name: 'Active Trading Contest',
    status: 'running' as const,
    description: 'An active trading competition',
    prizePool: 50000,
    entryFee: 100,
    maxParticipants: 1000,
    startDate: new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString(), // 2 days ago
    endDate: new Date(Date.now() + 5 * 24 * 60 * 60 * 1000).toISOString(), // 5 days from now
    symbols: ['EUR/USD', 'BTC/USD', 'GBP/USD', 'ETH/USD', 'SOL/USD'],
  },
  upcoming: {
    id: 'contest-upcoming-001',
    name: 'Upcoming Tournament',
    status: 'scheduled' as const,
    description: 'A scheduled trading competition',
    prizePool: 25000,
    entryFee: 50,
    maxParticipants: 500,
    startDate: new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toISOString(), // 3 days from now
    endDate: new Date(Date.now() + 10 * 24 * 60 * 60 * 1000).toISOString(), // 10 days from now
    symbols: ['EUR/USD', 'BTC/USD', 'XAU/USD'],
  },
  completed: {
    id: 'contest-completed-001',
    name: 'Completed Championship',
    status: 'completed' as const,
    description: 'A finished trading competition',
    prizePool: 100000,
    entryFee: 200,
    maxParticipants: 2000,
    startDate: new Date(Date.now() - 14 * 24 * 60 * 60 * 1000).toISOString(), // 14 days ago
    endDate: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(), // 7 days ago
    symbols: ['EUR/USD', 'BTC/USD', 'GBP/USD', 'ETH/USD'],
  },
  draft: {
    id: 'contest-draft-001',
    name: 'Draft Contest',
    status: 'draft' as const,
    description: 'A contest in draft state',
    prizePool: 10000,
    entryFee: 25,
    maxParticipants: 100,
    startDate: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(), // 7 days from now
    endDate: new Date(Date.now() + 14 * 24 * 60 * 60 * 1000).toISOString(), // 14 days from now
    symbols: ['EUR/USD', 'GBP/USD'],
  },
};

/**
 * Test trading symbols — forex and crypto pairs supported by the platform
 */
export const TEST_SYMBOLS = [
  { symbol: 'EUR/USD', name: 'Euro / US Dollar', price: 1.085 },
  { symbol: 'BTC/USD', name: 'Bitcoin / US Dollar', price: 92500.0 },
  { symbol: 'GBP/USD', name: 'British Pound / US Dollar', price: 1.272 },
  { symbol: 'ETH/USD', name: 'Ethereum / US Dollar', price: 3450.0 },
  { symbol: 'SOL/USD', name: 'Solana / US Dollar', price: 148.5 },
  { symbol: 'XAU/USD', name: 'Gold / US Dollar', price: 2340.0 },
  { symbol: 'USD/JPY', name: 'US Dollar / Japanese Yen', price: 153.2 },
];

/**
 * Test order data
 */
export const TEST_ORDERS = {
  marketBuy: {
    symbol: 'EUR/USD',
    side: 'buy' as const,
    type: 'market' as const,
    quantity: 10,
  },
  marketSell: {
    symbol: 'EUR/USD',
    side: 'sell' as const,
    type: 'market' as const,
    quantity: 5,
  },
  limitBuy: {
    symbol: 'BTC/USD',
    side: 'buy' as const,
    type: 'limit' as const,
    quantity: 20,
    price: 91000.0,
    takeProfit: 95000.0,
    stopLoss: 89000.0,
  },
  limitSell: {
    symbol: 'GBP/USD',
    side: 'sell' as const,
    type: 'limit' as const,
    quantity: 15,
    price: 1.28,
    takeProfit: 1.27,
    stopLoss: 1.285,
  },
};

/**
 * Test leaderboard data
 */
export const TEST_LEADERBOARD = [
  { rank: 1, userId: 'user-001', userName: 'TopTrader', pnl: 15420.5, trades: 45 },
  { rank: 2, userId: 'user-002', userName: 'ProInvestor', pnl: 12850.75, trades: 38 },
  { rank: 3, userId: 'user-003', userName: 'MarketMaster', pnl: 10200.0, trades: 52 },
  { rank: 4, userId: 'user-004', userName: 'ForexGuru', pnl: 8750.25, trades: 29 },
  { rank: 5, userId: 'user-005', userName: 'CryptoWizard', pnl: 7100.0, trades: 41 },
  { rank: 6, userId: 'user-006', userName: 'TradingPro', pnl: 5500.5, trades: 33 },
  { rank: 7, userId: 'user-007', userName: 'ChartAnalyst', pnl: 4200.75, trades: 27 },
  { rank: 8, userId: 'user-008', userName: 'RiskTaker', pnl: 3100.0, trades: 56 },
  { rank: 9, userId: 'user-009', userName: 'SafeTrader', pnl: 2500.25, trades: 18 },
  { rank: 10, userId: 'user-010', userName: 'NewTrader', pnl: 1200.0, trades: 12 },
];

/**
 * Test contest results data (for completed contests)
 */
export const TEST_CONTEST_RESULTS = {
  contestId: 'contest-completed-001',
  totalParticipants: 1847,
  prizePool: 100000,
  userResult: {
    rank: 5,
    pnl: 12500.75,
    reward: 8000,
    percentile: 99.7,
  },
  leaderboard: [
    { rank: 1, userId: 'user-001', userName: 'TopTrader', pnl: 45000.5, reward: 60000, trades: 89 },
    { rank: 2, userId: 'user-002', userName: 'ProInvestor', pnl: 38000.25, reward: 40000, trades: 67 },
    { rank: 3, userId: 'user-003', userName: 'MarketMaster', pnl: 28000.0, reward: 25000, trades: 54 },
    { rank: 4, userId: 'user-004', userName: 'ForexGuru', pnl: 18500.75, reward: 15000, trades: 43 },
    { rank: 5, userId: 'current-user', userName: 'You', pnl: 12500.75, reward: 8000, trades: 35, isCurrentUser: true },
    { rank: 6, userId: 'user-006', userName: 'TradingPro', pnl: 10200.5, reward: 5000, trades: 41 },
    { rank: 7, userId: 'user-007', userName: 'ChartAnalyst', pnl: 8750.25, reward: 3000, trades: 29 },
    { rank: 8, userId: 'user-008', userName: 'RiskTaker', pnl: 6500.0, reward: 2000, trades: 62 },
    { rank: 9, userId: 'user-009', userName: 'SafeTrader', pnl: 4200.75, reward: 1500, trades: 18 },
    { rank: 10, userId: 'user-010', userName: 'NewTrader', pnl: 2100.0, reward: 1000, trades: 12 },
  ],
  prizeDistribution: [
    { place: '1st', percentage: 30, amount: 60000 },
    { place: '2nd', percentage: 20, amount: 40000 },
    { place: '3rd', percentage: 12.5, amount: 25000 },
    { place: '4th', percentage: 7.5, amount: 15000 },
    { place: '5th', percentage: 4, amount: 8000 },
    { place: '6th', percentage: 2.5, amount: 5000 },
    { place: '7th', percentage: 1.5, amount: 3000 },
    { place: '8th', percentage: 1, amount: 2000 },
    { place: '9th', percentage: 0.75, amount: 1500 },
    { place: '10th', percentage: 0.5, amount: 1000 },
  ],
};

/**
 * Generate contest results for a given contest
 */
export function generateContestResults(
  contestId: string,
  prizePool: number,
  participantCount: number,
  userRank: number,
  userPnl: number
): typeof TEST_CONTEST_RESULTS {
  const prizeDistribution = [
    { place: '1st', percentage: 30, amount: Math.round(prizePool * 0.3) },
    { place: '2nd', percentage: 20, amount: Math.round(prizePool * 0.2) },
    { place: '3rd', percentage: 12.5, amount: Math.round(prizePool * 0.125) },
    { place: '4th', percentage: 7.5, amount: Math.round(prizePool * 0.075) },
    { place: '5th', percentage: 4, amount: Math.round(prizePool * 0.04) },
  ];

  const userReward = userRank <= 5 ? prizeDistribution[userRank - 1]?.amount || 0 : 0;

  return {
    contestId,
    totalParticipants: participantCount,
    prizePool,
    userResult: {
      rank: userRank,
      pnl: userPnl,
      reward: userReward,
      percentile: Math.round(((participantCount - userRank) / participantCount) * 1000) / 10,
    },
    leaderboard: TEST_CONTEST_RESULTS.leaderboard.map((entry, idx) => ({
      ...entry,
      reward: prizeDistribution[idx]?.amount || 0,
    })),
    prizeDistribution,
  };
}

/**
 * Test audit log entries
 */
export const TEST_AUDIT_LOGS = [
  {
    id: 'audit-001',
    action: 'contest.create',
    userId: 'admin-001',
    userName: 'Admin User',
    details: { contestId: 'contest-001', contestName: 'New Contest' },
    timestamp: new Date(Date.now() - 1 * 60 * 60 * 1000).toISOString(), // 1 hour ago
  },
  {
    id: 'audit-002',
    action: 'contest.update',
    userId: 'admin-001',
    userName: 'Admin User',
    details: { contestId: 'contest-001', field: 'prizePool', oldValue: 10000, newValue: 15000 },
    timestamp: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(), // 2 hours ago
  },
  {
    id: 'audit-003',
    action: 'contest.start',
    userId: 'admin-001',
    userName: 'Admin User',
    details: { contestId: 'contest-002' },
    timestamp: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(), // 1 day ago
  },
  {
    id: 'audit-004',
    action: 'user.ban',
    userId: 'admin-002',
    userName: 'Super Admin',
    details: { targetUserId: 'user-bad-001', reason: 'Violation of terms' },
    timestamp: new Date(Date.now() - 48 * 60 * 60 * 1000).toISOString(), // 2 days ago
  },
];

/**
 * Mock tick data for WebSocket tests
 */
export function generateTickData(symbol: string, basePrice: number): {
  type: string;
  data: {
    symbol: string;
    bid: number;
    ask: number;
    last: number;
    volume: number;
    timestamp: number;
  };
} {
  const spread = basePrice * 0.001; // 0.1% spread
  const variation = (Math.random() - 0.5) * basePrice * 0.02; // +/- 1% variation

  const last = basePrice + variation;
  const bid = last - spread / 2;
  const ask = last + spread / 2;

  return {
    type: 'tick_snapshot',
    data: {
      symbol,
      bid: Math.round(bid * 100) / 100,
      ask: Math.round(ask * 100) / 100,
      last: Math.round(last * 100) / 100,
      volume: Math.floor(Math.random() * 1000000),
      timestamp: Date.now(),
    },
  };
}

/**
 * Generate position update message
 */
export function generatePositionUpdate(
  userId: string,
  contestId: string,
  symbol: string,
  side: 'long' | 'short',
  quantity: number,
  avgPrice: number,
  currentPrice: number
): {
  type: string;
  data: {
    user_id: string;
    contest_id: string;
    symbol: string;
    side: string;
    qty: number;
    avg_price: number;
    unrealized_pnl: number;
  };
} {
  const pnlMultiplier = side === 'long' ? 1 : -1;
  const unrealizedPnl = (currentPrice - avgPrice) * quantity * pnlMultiplier;

  return {
    type: 'position_update',
    data: {
      user_id: userId,
      contest_id: contestId,
      symbol,
      side,
      qty: quantity,
      avg_price: avgPrice,
      unrealized_pnl: Math.round(unrealizedPnl * 100) / 100,
    },
  };
}

/**
 * Generate order acknowledgment message
 */
export function generateOrderAck(
  orderId: string,
  status: 'accepted' | 'rejected' | 'filled',
  reason?: string
): {
  type: string;
  data: {
    order_id: string;
    status: string;
    reason?: string;
    timestamp: number;
  };
} {
  return {
    type: 'order_ack',
    data: {
      order_id: orderId,
      status,
      ...(reason ? { reason } : {}),
      timestamp: Date.now(),
    },
  };
}
