// Trade module API functions. The shared axios instance + auth
// interceptor + 401 refresh retry all live in @/api/client; this file
// is now purely endpoint functions.
import { AxiosError } from 'axios';
import { api } from '@/api/client';
import { getErrorMessage, isNetworkError } from '@/utils/errorHandler';
import type { ClosePositionResponse, CancelOrderResponse, UpdateTPSLRequest, UpdateTPSLResponse, OrderHistoryResponse, OrderHistoryOptions, BalanceResponse, LeaderboardResponse, LeaderboardOptions, Contest, UserContest, ContestDetailsResponse, PrizePreviewResponse, ContestSymbol } from '@/types/contracts';

/**
 * Close a position (fully or partially).
 * @param positionId - The ID of the position to close
 * @param qty - Optional quantity for partial close. If omitted, closes entire position.
 * @returns The order ID and confirmation message
 * @throws Error with appropriate message based on response status
 */
export async function closePosition(
  positionId: string,
  qty?: number
): Promise<ClosePositionResponse> {
  try {
    const body = qty !== undefined ? { qty } : undefined;
    const response = await api.post<ClosePositionResponse>(
      `/api/trade/positions/${positionId}/close`,
      body
    );
    return response.data;
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to close position. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      const data = error.response.data as Record<string, unknown>;

      switch (status) {
        case 404:
          throw new Error('Position not found');
        case 403:
          throw new Error('Cannot close this position');
        case 400:
          // Use error message from response if available
          if (typeof data.error === 'string') {
            throw new Error(data.error);
          }
          if (typeof data.message === 'string') {
            throw new Error(data.message);
          }
          throw new Error('Invalid close position request');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to close position');
  }
}

/**
 * Cancel a pending order.
 * @param orderId - The ID of the order to cancel
 * @returns The order ID and confirmation message
 * @throws Error with appropriate message based on response status
 */
export async function cancelOrder(orderId: string): Promise<CancelOrderResponse> {
  try {
    const response = await api.delete<CancelOrderResponse>(
      `/api/trade/orders/${orderId}`
    );
    return response.data;
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to cancel order. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      const data = error.response.data as Record<string, unknown>;

      switch (status) {
        case 404:
          throw new Error('Order not found');
        case 403:
          throw new Error('Cannot cancel this order');
        case 400:
          if (typeof data.error === 'string') {
            throw new Error(data.error);
          }
          if (typeof data.message === 'string') {
            throw new Error(data.message);
          }
          throw new Error('Invalid cancel order request');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to cancel order');
  }
}

/**
 * Update TP/SL for a position.
 * @param positionId - The ID of the position to update
 * @param takeProfit - New take profit price (null to remove)
 * @param stopLoss - New stop loss price (null to remove)
 * @returns The updated position TP/SL and confirmation message
 * @throws Error with appropriate message based on response status
 */
export async function updateTPSL(
  positionId: string,
  takeProfit?: number | null,
  stopLoss?: number | null
): Promise<UpdateTPSLResponse> {
  try {
    const body: UpdateTPSLRequest = {
      take_profit: takeProfit,
      stop_loss: stopLoss,
    };
    const response = await api.put<UpdateTPSLResponse>(
      `/api/trade/positions/${positionId}/tpsl`,
      body
    );
    return response.data;
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to update TP/SL. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      const data = error.response.data as Record<string, unknown>;

      switch (status) {
        case 404:
          throw new Error('Position not found');
        case 403:
          throw new Error('Cannot modify this position');
        case 400:
          if (typeof data.error === 'string') {
            throw new Error(data.error);
          }
          if (typeof data.message === 'string') {
            throw new Error(data.message);
          }
          throw new Error('Invalid TP/SL values');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to update TP/SL');
  }
}

/**
 * Get order history for a contest.
 * @param contestId - The contest ID to fetch orders for
 * @param options - Optional query parameters (limit, offset, status, symbol)
 * @returns Paginated order history response
 * @throws Error with appropriate message based on response status
 */
export async function getOrderHistory(
  contestId: string,
  options?: OrderHistoryOptions
): Promise<OrderHistoryResponse> {
  try {
    const params = new URLSearchParams();
    if (options?.limit !== undefined) {
      params.append('limit', options.limit.toString());
    }
    if (options?.offset !== undefined) {
      params.append('offset', options.offset.toString());
    }
    if (options?.status) {
      params.append('status', options.status);
    }
    if (options?.symbol) {
      params.append('symbol', options.symbol);
    }

    const queryString = params.toString();
    const url = `/api/trade/orders/history?contest_id=${contestId}${queryString ? '&' + queryString : ''}`;

    const response = await api.get<OrderHistoryResponse>(url);
    return response.data;
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to fetch order history. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      const data = error.response.data as Record<string, unknown>;

      switch (status) {
        case 404:
          throw new Error('Contest not found');
        case 400:
          if (typeof data.error === 'string') {
            throw new Error(data.error);
          }
          if (typeof data.message === 'string') {
            throw new Error(data.message);
          }
          throw new Error('Invalid request parameters');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to fetch order history');
  }
}

/**
 * Get user's QTY balance/allocation for a contest.
 * @param contestId - The contest ID to fetch balance for
 * @returns Balance response with qty_total, qty_available, qty_used
 * @throws Error with appropriate message based on response status
 */
export async function getBalance(contestId: string): Promise<BalanceResponse> {
  try {
    const response = await api.get<BalanceResponse>(
      `/api/trade/balance?contest_id=${contestId}`
    );
    return response.data;
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to fetch balance. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      const data = error.response.data as Record<string, unknown>;

      switch (status) {
        case 404:
          throw new Error('Contest not found');
        case 403:
          throw new Error('Not a participant in this contest');
        case 400:
          if (typeof data.error === 'string') {
            throw new Error(data.error);
          }
          if (typeof data.message === 'string') {
            throw new Error(data.message);
          }
          throw new Error('Invalid request');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to fetch balance');
  }
}

/**
 * Get leaderboard for a contest.
 * @param contestId - The contest ID to fetch leaderboard for
 * @param options - Optional query parameters (limit, offset)
 * @returns Leaderboard response with entries and total count
 * @throws Error with appropriate message based on response status
 */
export async function getLeaderboard(
  contestId: string,
  options?: LeaderboardOptions
): Promise<LeaderboardResponse> {
  try {
    const params = new URLSearchParams();
    params.append('contest_id', contestId);
    if (options?.limit !== undefined) {
      params.append('limit', options.limit.toString());
    }
    if (options?.offset !== undefined) {
      params.append('offset', options.offset.toString());
    }

    const response = await api.get<LeaderboardResponse>(
      `/api/user/leaderboard?${params.toString()}`
    );
    return response.data;
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to fetch leaderboard. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      const data = error.response.data as Record<string, unknown>;

      switch (status) {
        case 404:
          throw new Error('Contest not found');
        case 400:
          if (typeof data.error === 'string') {
            throw new Error(data.error);
          }
          if (typeof data.message === 'string') {
            throw new Error(data.message);
          }
          throw new Error('Invalid request');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to fetch leaderboard');
  }
}

/**
 * Get list of active contests available for trading.
 * Fetches contests with status 'running' or 'registration_open'.
 * @returns Array of active contests
 * @throws Error with appropriate message based on response status
 */
export async function getActiveContests(): Promise<Contest[]> {
  try {
    const response = await api.get<{ contests: Contest[] }>(
      '/api/user/contests?status=running,registration_open'
    );
    return response.data.contests || [];
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to fetch contests. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      switch (status) {
        case 500:
          throw new Error('Server error fetching contests');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to fetch contests');
  }
}

// Raw response type from /api/user/me/history endpoint
interface ContestHistoryEntry {
  contest_id: string;
  contest_name: string;
  status: string;
  joined_at: string;
  total_score: number;
  final_rank?: number;
  final_prize_cents?: number;
}

/**
 * Get list of contests that the current user has joined.
 * @returns Array of user's joined contests
 * @throws Error with appropriate message based on response status
 */
export async function getUserContests(): Promise<UserContest[]> {
  try {
    const response = await api.get<{ contests: ContestHistoryEntry[] }>(
      '/api/user/me/history'
    );
    // Map server response to UserContest interface
    return (response.data.contests || []).map(entry => ({
      id: entry.contest_id,
      name: entry.contest_name,
      status: entry.status as UserContest['status'],
      joined_at: entry.joined_at,
      total_score: entry.total_score,
      final_rank: entry.final_rank,
      final_prize_cents: entry.final_prize_cents,
    }));
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to fetch your contests. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      switch (status) {
        case 401:
          throw new Error('Please login to view your contests');
        case 500:
          throw new Error('Server error fetching your contests');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to fetch your contests');
  }
}

/**
 * Join a contest.
 * @param contestId - The ID of the contest to join
 * @returns Join response with confirmation
 * @throws Error with appropriate message based on response status
 */
export async function joinContest(contestId: string): Promise<{ message: string; qty_available: number }> {
  try {
    const response = await api.post<{ message: string; qty_available: number }>(
      `/api/user/contests/${contestId}/join`
    );
    return response.data;
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to join contest. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      const data = error.response.data as Record<string, unknown>;

      switch (status) {
        case 404:
          throw new Error('Contest not found');
        case 400:
          if (typeof data.error === 'string') {
            throw new Error(data.error);
          }
          if (typeof data.message === 'string') {
            throw new Error(data.message);
          }
          throw new Error('Cannot join this contest');
        case 409:
          // Already joined - this is fine, return success
          return { message: 'Already joined', qty_available: 0 };
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to join contest');
  }
}

/**
 * Get detailed contest information.
 * @param contestId - The contest ID to fetch details for
 * @returns ContestDetailsResponse with full contest info including symbols and server_time
 */
export async function getContestDetails(contestId: string): Promise<ContestDetailsResponse> {
  try {
    const response = await api.get<ContestDetailsResponse>(
      `/api/user/contests/${contestId}`
    );
    return response.data;
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to fetch contest details. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      const data = error.response.data as Record<string, unknown>;

      switch (status) {
        case 404:
          throw new Error('Contest not found');
        case 400:
          if (typeof data.error === 'string') {
            throw new Error(data.error);
          }
          if (typeof data.message === 'string') {
            throw new Error(data.message);
          }
          throw new Error('Invalid request');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to fetch contest details');
  }
}

/**
 * Get symbols for a contest.
 * @param contestId - The contest ID to fetch symbols for
 * @returns Object with contest_id and symbols array
 */
export async function getContestSymbols(contestId: string): Promise<{ contest_id: string; symbols: ContestSymbol[] }> {
  try {
    const response = await api.get<{ contest_id: string; symbols: ContestSymbol[] }>(
      `/api/trade/contest/${contestId}/symbols`
    );
    return response.data;
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to fetch contest symbols. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      switch (status) {
        case 404:
          throw new Error('Contest not found');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to fetch contest symbols');
  }
}

/**
 * Get prize preview for a contest.
 * @param contestId - The contest ID to fetch prize preview for
 * @returns PrizePreviewResponse with prize distribution info
 */
export async function getPrizePreview(contestId: string): Promise<PrizePreviewResponse> {
  try {
    const response = await api.get<PrizePreviewResponse>(
      `/api/user/contests/${contestId}/prize-preview`
    );
    return response.data;
  } catch (error) {
    if (isNetworkError(error)) {
      throw new Error('Unable to fetch prize info. Please check your connection.');
    }

    if (error instanceof AxiosError && error.response) {
      const status = error.response.status;
      switch (status) {
        case 404:
          throw new Error('Contest not found');
        default:
          throw new Error(getErrorMessage(error));
      }
    }

    throw new Error('Failed to fetch prize info');
  }
}

