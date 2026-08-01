package exchangerate

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Service provides USD↔IRR exchange rate conversion.
type Service struct {
	config     Config
	httpClient *http.Client
	logger     *zap.Logger
	mu         sync.RWMutex
	cachedRate *Rate
	cachedAt   time.Time
	rdb        *redis.Client
}

// NewService creates a new exchange rate service.
func NewService(config Config, logger *zap.Logger) *Service {
	if config.CacheTTL == 0 {
		config.CacheTTL = DefaultCacheTTL
	}
	if config.NobitexBaseURL == "" {
		config.NobitexBaseURL = DefaultNobitexURL
	}
	if config.StaticUSDToIRR == 0 {
		config.StaticUSDToIRR = DefaultStaticUSDIRR
	}

	s := &Service{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}

	// Initialize Redis client if address is provided
	if config.RedisAddr != "" {
		s.rdb = redis.NewClient(&redis.Options{
			Addr:         config.RedisAddr,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		})
	}

	return s
}

// GetRate returns the current USD→IRR exchange rate.
// It checks in-memory cache, then Redis, then fetches from Nobitex,
// and falls back to a static rate if all sources fail.
func (s *Service) GetRate(ctx context.Context) (*Rate, error) {
	// Check in-memory cache first
	s.mu.RLock()
	if s.cachedRate != nil && time.Since(s.cachedAt) < s.config.CacheTTL {
		rate := *s.cachedRate
		s.mu.RUnlock()
		return &rate, nil
	}
	s.mu.RUnlock()

	// Check Redis cache
	if s.rdb != nil {
		rate, err := s.getRedisCache(ctx)
		if err == nil && rate != nil {
			rate.Source = "cached"
			s.updateMemoryCache(rate)
			return rate, nil
		}
	}

	// Fetch from Nobitex
	rate, err := s.fetchNobitexRate(ctx)
	if err != nil {
		s.logger.Warn("Failed to fetch Nobitex rate, using static fallback",
			zap.Error(err))
		// Use static fallback
		rate = &Rate{
			USDToIRR:  s.config.StaticUSDToIRR,
			USDToIRT:  s.config.StaticUSDToIRR / 10,
			Source:    "static",
			FetchedAt: time.Now(),
		}
	}

	// Update caches
	s.updateMemoryCache(rate)
	if s.rdb != nil {
		s.setRedisCache(ctx, rate)
	}

	return rate, nil
}

// nobitexResponse represents the Nobitex API response for orderbook.
type nobitexResponse struct {
	Status         string `json:"status"`
	LastTradePrice string `json:"lastTradePrice"`
}

// fetchNobitexRate fetches the current USD/IRT rate from Nobitex.
func (s *Service) fetchNobitexRate(ctx context.Context) (*Rate, error) {
	url := s.config.NobitexBaseURL + "/v3/orderbook/USDTIRT"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch nobitex rate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nobitex returned status %d", resp.StatusCode)
	}

	var result nobitexResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode nobitex response: %w", err)
	}

	if result.Status != "ok" {
		return nil, fmt.Errorf("nobitex returned status: %s", result.Status)
	}

	// Parse lastTradePrice — Nobitex returns price in IRT (Toman) for IRT pairs
	priceStr := strings.TrimSpace(result.LastTradePrice)
	if priceStr == "" || priceStr == "0" {
		return nil, fmt.Errorf("nobitex returned invalid price: %q", priceStr)
	}

	priceIRT, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return nil, fmt.Errorf("parse nobitex price %q: %w", priceStr, err)
	}

	if priceIRT <= 0 {
		return nil, fmt.Errorf("nobitex returned non-positive price: %f", priceIRT)
	}

	// Nobitex lastTradePrice for USDTIRT is in Toman (IRT)
	// 1 USDT = priceIRT Toman
	// 1 USDT = priceIRT * 10 Rial (IRR)
	return &Rate{
		USDToIRT:  priceIRT,
		USDToIRR:  priceIRT * 10,
		Source:    "nobitex",
		FetchedAt: time.Now(),
	}, nil
}

// ConvertUSDToIRR converts a USD amount (in cents) to IRR (Rial).
// Uses integer arithmetic to avoid float64 precision loss in currency conversion.
func ConvertUSDToIRR(amountUSDCents int64, rate *Rate) int64 {
	// Multiply rate by 100 to work in integer space, then divide by 100 to cancel.
	// This avoids float64 multiplication of two large values.
	rateScaled := int64(math.Round(rate.USDToIRR * 100))
	return (amountUSDCents * rateScaled) / (100 * 100)
}

// ConvertUSDToIRT converts a USD amount (in cents) to IRT (Toman).
// Uses integer arithmetic to avoid float64 precision loss in currency conversion.
func ConvertUSDToIRT(amountUSDCents int64, rate *Rate) int64 {
	rateScaled := int64(math.Round(rate.USDToIRT * 100))
	return (amountUSDCents * rateScaled) / (100 * 100)
}

func (s *Service) updateMemoryCache(rate *Rate) {
	s.mu.Lock()
	s.cachedRate = rate
	s.cachedAt = time.Now()
	s.mu.Unlock()
}

func (s *Service) getRedisCache(ctx context.Context) (*Rate, error) {
	data, err := s.rdb.Get(ctx, CacheKey).Bytes()
	if err != nil {
		return nil, err
	}

	var rate Rate
	if err := json.Unmarshal(data, &rate); err != nil {
		return nil, err
	}

	// Check if cached rate is still fresh
	if time.Since(rate.FetchedAt) > s.config.CacheTTL {
		return nil, fmt.Errorf("redis cache expired")
	}

	return &rate, nil
}

func (s *Service) setRedisCache(ctx context.Context, rate *Rate) {
	data, err := json.Marshal(rate)
	if err != nil {
		s.logger.Warn("Failed to marshal rate for Redis cache", zap.Error(err))
		return
	}

	if err := s.rdb.Set(ctx, CacheKey, data, s.config.CacheTTL).Err(); err != nil {
		s.logger.Warn("Failed to set Redis cache", zap.Error(err))
	}
}
