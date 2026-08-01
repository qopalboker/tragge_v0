package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"go.uber.org/zap"
)

// maxProxyResponseBytes limits the response body size from market-ingestor (1 MB).
const maxProxyResponseBytes = 1 << 20

// handleGetMarketStatus2 proxies the market-ingestor status endpoint.
// Named with 2 suffix to avoid conflict with existing handleGetMarketStatus in handlers_market_hours.go.
func (a *App) handleGetMarketStatus2(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	url := strings.TrimRight(a.config.MarketIngestorURL, "/") + "/status/subscriptions"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		a.log().Error("Failed to create market status request", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if a.config.MarketIngestorAPIKey != "" {
		req.Header.Set("X-API-Key", a.config.MarketIngestorAPIKey)
	}

	resp, err := a.marketIngestorClient.Do(req)
	if err != nil {
		a.log().Warn("Market ingestor unreachable", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   adminMsg.MarketIngestorUnreachable,
			"message": adminMsg.MarketIngestorDown,
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProxyResponseBytes))
	if err != nil {
		a.log().Error("Failed to read market status response", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.ResponseReadFailed})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// handleGetMarketPrices returns current prices from Redis (prices:latest hash).
func (a *App) handleGetMarketPrices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if a.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.RedisUnavailable})
		return
	}

	// Read all prices from the Redis hash set by market-ingestor
	redisResult, err := a.circuits.ExecuteRedisWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.redis.Client().HGetAll(ctx, "prices:latest").Result()
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to get prices from Redis", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.PricesFailed})
		return
	}
	result := redisResult.(map[string]string)

	// Parse each symbol's tick data
	prices := make(map[string]json.RawMessage, len(result))
	for symbol, data := range result {
		prices[symbol] = json.RawMessage(data)
	}

	writeJSON(w, http.StatusOK, prices)
}

// handleSwitchMarketProvider sends a switch-provider command to market-ingestor.
func (a *App) handleSwitchMarketProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var body struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ProviderRequired})
		return
	}

	if body.Provider != "massive" && body.Provider != "twelvedata" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidProvider})
		return
	}

	url := fmt.Sprintf("%s/control/switch-provider?provider=%s",
		strings.TrimRight(a.config.MarketIngestorURL, "/"), body.Provider)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		a.log().Error("Failed to create switch-provider request", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if a.config.MarketIngestorAPIKey != "" {
		req.Header.Set("X-API-Key", a.config.MarketIngestorAPIKey)
	}

	resp, err := a.marketIngestorClient.Do(req)
	if err != nil {
		a.log().Warn("Market ingestor unreachable for switch-provider", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.MarketIngestorDown})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxProxyResponseBytes))
	if err != nil {
		a.log().Error("Failed to read switch-provider response", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.ResponseReadFailed})
		return
	}

	// Log the admin action
	actorUserID := auth.GetUserID(ctx)
	a.logAuditEvent(ctx, actorUserID, "market.switch_provider", "market", body.Provider,
		map[string]string{"provider": body.Provider})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// handleGetProviderConfig returns the current provider configuration from market-ingestor.
func (a *App) handleGetProviderConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	url := strings.TrimRight(a.config.MarketIngestorURL, "/") + "/control/provider-config"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		a.log().Error("Failed to create provider-config request", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if a.config.MarketIngestorAPIKey != "" {
		req.Header.Set("X-API-Key", a.config.MarketIngestorAPIKey)
	}

	resp, err := a.marketIngestorClient.Do(req)
	if err != nil {
		a.log().Warn("Market ingestor unreachable for provider-config", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.MarketIngestorDown})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProxyResponseBytes))
	if err != nil {
		a.log().Error("Failed to read provider-config response", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.ResponseReadFailed})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// handleSwitchCryptoProvider sends a crypto provider switch command to market-ingestor.
func (a *App) handleSwitchCryptoProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var body struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ProviderRequired})
		return
	}

	if body.Provider != "nobitex" && body.Provider != "binance" && body.Provider != "both" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidProvider})
		return
	}

	url := fmt.Sprintf("%s/control/crypto-provider?provider=%s",
		strings.TrimRight(a.config.MarketIngestorURL, "/"), body.Provider)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		a.log().Error("Failed to create switch-crypto-provider request", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if a.config.MarketIngestorAPIKey != "" {
		req.Header.Set("X-API-Key", a.config.MarketIngestorAPIKey)
	}

	resp, err := a.marketIngestorClient.Do(req)
	if err != nil {
		a.log().Warn("Market ingestor unreachable for switch-crypto-provider", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.MarketIngestorDown})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxProxyResponseBytes))
	if err != nil {
		a.log().Error("Failed to read switch-crypto-provider response", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.ResponseReadFailed})
		return
	}

	// Log the admin action
	actorUserID := auth.GetUserID(ctx)
	a.logAuditEvent(ctx, actorUserID, "market.switch_crypto_provider", "market", body.Provider,
		map[string]string{"provider": body.Provider})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// handleSwitchForexProvider sends a forex provider switch command to market-ingestor.
func (a *App) handleSwitchForexProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var body struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ProviderRequired})
		return
	}

	if body.Provider != "massive" && body.Provider != "twelvedata" && body.Provider != "finnhub" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidProvider})
		return
	}

	url := fmt.Sprintf("%s/control/switch-provider?provider=%s",
		strings.TrimRight(a.config.MarketIngestorURL, "/"), body.Provider)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		a.log().Error("Failed to create switch-forex-provider request", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if a.config.MarketIngestorAPIKey != "" {
		req.Header.Set("X-API-Key", a.config.MarketIngestorAPIKey)
	}

	resp, err := a.marketIngestorClient.Do(req)
	if err != nil {
		a.log().Warn("Market ingestor unreachable for switch-forex-provider", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.MarketIngestorDown})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxProxyResponseBytes))
	if err != nil {
		a.log().Error("Failed to read switch-forex-provider response", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.ResponseReadFailed})
		return
	}

	actorUserID := auth.GetUserID(ctx)
	a.logAuditEvent(ctx, actorUserID, "market.switch_forex_provider", "market", body.Provider,
		map[string]string{"provider": body.Provider})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// handleMarketReconnect sends a reconnect command to market-ingestor.
func (a *App) handleMarketReconnect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	url := strings.TrimRight(a.config.MarketIngestorURL, "/") + "/control/reconnect"

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		a.log().Error("Failed to create reconnect request", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if a.config.MarketIngestorAPIKey != "" {
		req.Header.Set("X-API-Key", a.config.MarketIngestorAPIKey)
	}

	resp, err := a.marketIngestorClient.Do(req)
	if err != nil {
		a.log().Warn("Market ingestor unreachable for reconnect", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.MarketIngestorDown})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxProxyResponseBytes))
	if err != nil {
		a.log().Error("Failed to read reconnect response", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.ResponseReadFailed})
		return
	}

	// Log the admin action
	actorUserID := auth.GetUserID(ctx)
	a.logAuditEvent(ctx, actorUserID, "market.reconnect", "market", "active", nil)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}
