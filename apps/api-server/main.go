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
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	adminserver "github.com/Parsaeffatravesh/tragge/apps/admin-bff/server"
	paymentserver "github.com/Parsaeffatravesh/tragge/apps/payment-service/server"
	userserver "github.com/Parsaeffatravesh/tragge/apps/user-bff/server"
)

func main() {
	observability.InstallStandardLoggerRedaction()
	log.Println("api-server: starting merged services (user-bff :8081, admin-bff :8083, payment-service :8091)")

	// Validate both cryptographic trust domains before opening shared runtime
	// resources. Errors identify fields only and never include secret values.
	authIsolation, err := loadAuthIsolationConfig(os.Getenv, secrets.Load)
	if err != nil {
		log.Fatalf("api-server: %v", err)
	}

	// Create shared database pool (1 pool instead of 3, saving ~15 connections)
	dbMaxOpen := 15
	if v := os.Getenv("DB_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			dbMaxOpen = n
		}
	}
	dbMaxIdle := 5
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
		log.Fatalf("api-server: failed to create shared database pool: %v", err)
	}
	defer pool.Close()
	log.Printf("api-server: shared DB pool created (max_open=%d, max_idle=%d, replicas=%d)", dbMaxOpen, dbMaxIdle, len(postgresReplicaDSNs))

	// Create shared Redis client (1 client instead of 3)
	redisCfg := pkgredis.ConfigFromEnv(os.Getenv)
	var redisClient *pkgredis.Client
	redisClient, err = pkgredis.NewClient(redisCfg)
	if err != nil {
		log.Printf("api-server: WARNING - failed to create shared Redis client: %v", err)
	} else {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
		if pingErr := redisClient.Ping(pingCtx).Err(); pingErr != nil {
			log.Printf("api-server: WARNING - Redis ping failed: %v", pingErr)
			redisClient.Close()
			redisClient = nil
		} else {
			log.Printf("api-server: shared Redis client created (mode=%s)", redisClient.Mode())
		}
		pingCancel()
	}
	defer func() {
		if redisClient != nil {
			redisClient.Close()
		}
	}()

	// Construct two explicit authentication contexts. The Redis client is a
	// shared transport only; session and revocation keys remain namespaced.
	var redisUniversalClient redis.UniversalClient
	if redisClient != nil {
		redisUniversalClient = redisClient.UniversalClient
	}
	userAuth, adminAuth, err := buildAuthContexts(authIsolation, redisUniversalClient)
	if err != nil {
		log.Fatalf("api-server: failed to construct isolated authentication contexts: %v", err)
	}
	log.Println("api-server: isolated User and Admin authentication contexts created")

	// Use context for coordinated shutdown instead of multiple signal.Notify calls.
	// This fixes the race condition where each sub-service registered its own signal
	// handler, leading to uncoordinated shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger, _ := zap.NewProduction()
	logger = observability.WrapLogger(logger)
	defer logger.Sync()

	var wg sync.WaitGroup

	infra.SafeGoWg(&wg, logger, "user-bff", func() {
		userserver.RunWithSharedDeps(ctx, pool, redisClient, userAuth) // default :8081
	})

	infra.SafeGoWg(&wg, logger, "admin-bff", func() {
		adminserver.RunWithSharedDeps(ctx, pool, redisClient, adminAuth) // default :8083
	})

	infra.SafeGoWg(&wg, logger, "payment-service", func() {
		paymentserver.RunWithSharedDeps(ctx, pool, redisClient, userAuth) // default :8091
	})

	<-ctx.Done()
	log.Println("api-server: shutdown signal received, waiting for services...")
	wg.Wait()
	log.Println("api-server: all services stopped")
}
