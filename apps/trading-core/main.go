package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
	"go.uber.org/zap"

	marketingestor "github.com/Parsaeffatravesh/tragge/apps/market-ingestor/server"
	tradebff "github.com/Parsaeffatravesh/tragge/apps/trade-bff/server"
	tradingengine "github.com/Parsaeffatravesh/tragge/apps/trading-engine/server"
)

func main() {
	observability.InstallStandardLoggerRedaction()
	log.Println("trading-core: starting merged services (engine :8085, ingestor :8084, trade-bff :8082)")

	// Create shared database pool (1 pool instead of 3, saving ~20 connections)
	dbMaxOpen := 20
	if v := os.Getenv("DB_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			dbMaxOpen = n
		}
	}
	dbMaxIdle := 8
	if v := os.Getenv("DB_MAX_IDLE_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			dbMaxIdle = n
		}
	}

	postgresDSN := secrets.BuildPostgresDSN()
	var postgresReplicaDSNs []string
	if replicaDSNs := os.Getenv("POSTGRES_REPLICA_DSNS"); replicaDSNs != "" {
		for _, dsn := range strings.Split(replicaDSNs, ",") {
			dsn = strings.TrimSpace(dsn)
			if dsn != "" {
				postgresReplicaDSNs = append(postgresReplicaDSNs, dsn)
			}
		}
	}

	pool, err := db.NewPool(context.Background(), db.Config{
		PrimaryDSN:      postgresDSN,
		ReplicaDSNs:     postgresReplicaDSNs,
		MaxOpenConns:    dbMaxOpen,
		MaxIdleConns:    dbMaxIdle,
		ConnMaxLifetime: 5 * time.Minute,
	})
	if err != nil {
		log.Fatalf("trading-core: failed to create shared database pool: %v", err)
	}
	defer pool.Close()
	log.Printf("trading-core: shared DB pool created (max_open=%d, max_idle=%d, replicas=%d)", dbMaxOpen, dbMaxIdle, len(postgresReplicaDSNs))

	// Create shared Redis client (1 client instead of 3)
	redisCfg := pkgredis.ConfigFromEnv(os.Getenv)
	var redisClient *pkgredis.Client
	redisClient, err = pkgredis.NewClient(redisCfg)
	if err != nil {
		log.Printf("trading-core: WARNING - failed to create shared Redis client: %v", err)
	} else {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
		if pingErr := redisClient.Ping(pingCtx).Err(); pingErr != nil {
			log.Printf("trading-core: WARNING - Redis ping failed: %v", pingErr)
			redisClient.Close()
			redisClient = nil
		} else {
			log.Printf("trading-core: shared Redis client created (mode=%s)", redisClient.Mode())
		}
		pingCancel()
	}
	defer func() {
		if redisClient != nil {
			redisClient.Close()
		}
	}()

	// Use context for coordinated shutdown instead of multiple signal.Notify calls.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger, _ := zap.NewProduction()
	logger = observability.WrapLogger(logger)
	defer logger.Sync()

	var wg sync.WaitGroup

	infra.SafeGoWg(&wg, logger, "market-ingestor", func() {
		marketingestor.RunWithSharedDeps(ctx, pool, redisClient) // default :8084
	})

	infra.SafeGoWg(&wg, logger, "trading-engine", func() {
		tradingengine.RunWithSharedDeps(ctx, pool, redisClient) // default :8085
	})

	infra.SafeGoWg(&wg, logger, "trade-bff", func() {
		tradebff.RunWithSharedDeps(ctx, pool, redisClient, nil) // default :8082, auth created internally
	})

	<-ctx.Done()
	log.Println("trading-core: shutdown signal received, waiting for services...")
	wg.Wait()
	log.Println("trading-core: all services stopped")
}
