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

	contestscheduler "github.com/Parsaeffatravesh/tragge/apps/contest-scheduler/server"
	freecontestgenerator "github.com/Parsaeffatravesh/tragge/apps/free-contest-generator/server"
	leaderboardworker "github.com/Parsaeffatravesh/tragge/apps/leaderboard-worker/server"
	settlementservice "github.com/Parsaeffatravesh/tragge/apps/settlement-service/server"
)

func main() {
	observability.InstallStandardLoggerRedaction()
	log.Println("worker: starting merged workers (leaderboard :8086, settlement :8087, scheduler :8088, generator :8089)")

	// Create shared database pool (1 pool instead of 4, saving ~20 connections)
	dbMaxOpen := 10
	if v := os.Getenv("DB_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			dbMaxOpen = n
		}
	}
	dbMaxIdle := 4
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
		log.Fatalf("worker: failed to create shared database pool: %v", err)
	}
	defer pool.Close()
	log.Printf("worker: shared DB pool created (max_open=%d, max_idle=%d, replicas=%d)", dbMaxOpen, dbMaxIdle, len(postgresReplicaDSNs))

	// Create shared Redis client for services that use pkgredis.Client
	// (leaderboard-worker, contest-scheduler use pkgredis; settlement-service uses raw redis.Client)
	redisCfg := pkgredis.ConfigFromEnv(os.Getenv)
	var redisClient *pkgredis.Client
	redisClient, err = pkgredis.NewClient(redisCfg)
	if err != nil {
		log.Printf("worker: WARNING - failed to create shared Redis client: %v", err)
	} else {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
		if pingErr := redisClient.Ping(pingCtx).Err(); pingErr != nil {
			log.Printf("worker: WARNING - Redis ping failed: %v", pingErr)
			redisClient.Close()
			redisClient = nil
		} else {
			log.Printf("worker: shared Redis client created (mode=%s)", redisClient.Mode())
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

	infra.SafeGoWg(&wg, logger, "leaderboard-worker", func() {
		leaderboardworker.RunWithSharedDeps(ctx, pool, redisClient) // default :8086
	})

	infra.SafeGoWg(&wg, logger, "settlement-service", func() {
		settlementservice.RunWithSharedDeps(ctx, pool, nil) // default :8087, uses raw redis.Client internally
	})

	infra.SafeGoWg(&wg, logger, "contest-scheduler", func() {
		contestscheduler.RunWithSharedDeps(ctx, pool, redisClient) // default :8088
	})

	infra.SafeGoWg(&wg, logger, "free-contest-generator", func() {
		freecontestgenerator.RunWithSharedDeps(ctx, pool) // default :8089, no Redis needed
	})

	<-ctx.Done()
	log.Println("worker: shutdown signal received, waiting for workers...")
	wg.Wait()
	log.Println("worker: all workers stopped")
}
