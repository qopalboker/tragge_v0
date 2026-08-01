package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	envconfig "github.com/Parsaeffatravesh/tragge/packages/config"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Config holds configuration for the free contest generator service.
type Config struct {
	Port        string
	PostgresDSN string

	// Kafka configuration
	KafkaBrokers string
	KafkaTopic   string

	// Generator configuration
	Enabled         bool
	IntervalMinutes int      // How often to generate contests (default: 60 = hourly)
	DurationMinutes int      // Default duration for generated contests
	AssetClasses    []string // Asset classes to generate for (forex, crypto)
	WeekdaysOnly    bool     // Only generate on weekdays
	StartHourUTC    int      // Start hour in UTC (default: 6 = 6 AM)
	EndHourUTC      int      // End hour in UTC (default: 22 = 10 PM)
	LeadTimeMinutes int      // Minutes before contest starts after generation (default: 5)

	// System bot configuration
	TBotUserID string // UUID of the T-bot system account

	// Cleanup configuration
	CleanupEnabled         bool
	CleanupIntervalMinutes int // How often to run cleanup
	CleanupRetentionHours  int // How long to keep completed free contests
}

// FreeContestGenerator generates free practice contests on a schedule.
type FreeContestGenerator struct {
	db       *db.Pool
	kafka    sarama.SyncProducer
	config   *Config
	obs      *observability.Observability
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// Run starts the free-contest-generator service in standalone mode with its own resources.
func Run() {
	RunWithSharedDeps(nil, nil)
}

// RunWithSharedDeps starts the free-contest-generator service, optionally using shared resources.
// When parentCtx is non-nil, the service shuts down when the context is cancelled
// instead of registering its own signal handler.
func RunWithSharedDeps(parentCtx context.Context, sharedPool *db.Pool) {
	config := loadConfig()

	// Initialize observability
	ctx := context.Background()
	obs, err := observability.New(ctx, observability.Config{
		Service:              "free-contest-generator",
		Env:                  os.Getenv("ENVIRONMENT"),
		Version:              os.Getenv("VERSION"),
		OTLPEndpoint:         os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		EnableGoMetrics:      true,
		EnableProcessMetrics: true,
	})
	if err != nil {
		fmt.Printf("Failed to initialize observability: %v\n", observability.RedactError(err))
		os.Exit(1)
	}
	defer obs.Shutdown(context.Background())

	logger := obs.Logger.Logger

	// Initialize database connection
	var pool *db.Pool
	if sharedPool != nil {
		pool = sharedPool
		logger.Info("Using shared database pool")
	} else {
		var poolErr error
		pool, poolErr = db.NewPool(ctx, db.Config{PrimaryDSN: config.PostgresDSN})
		if poolErr != nil {
			logger.Fatal("Failed to connect to database", zap.Error(poolErr))
		}
		defer pool.Close()
	}

	// Initialize Kafka producer
	var kafkaProducer sarama.SyncProducer
	if config.KafkaBrokers != "" {
		kafkaConfig := sarama.NewConfig()
		kafkaConfig.Producer.Return.Successes = true
		kafkaConfig.Producer.RequiredAcks = sarama.WaitForAll
		kafkaConfig.Producer.Retry.Max = 3

		brokers := strings.Split(config.KafkaBrokers, ",")
		producer, err := sarama.NewSyncProducer(brokers, kafkaConfig)
		if err != nil {
			logger.Warn("Failed to create Kafka producer, continuing without event publishing", zap.Error(err))
		} else {
			kafkaProducer = producer
			defer producer.Close()
		}
	}

	// Verify the T-bot system account exists in the database
	var sysAccountExists bool
	err = pool.Primary().QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND is_system_account = TRUE)`,
		config.TBotUserID,
	).Scan(&sysAccountExists)
	if err != nil {
		logger.Error("Failed to check T-bot system account", zap.Error(err))
	} else if !sysAccountExists {
		logger.Error("T-bot system account not found in database - free contests will lack the bot participant",
			zap.String("expected_user_id", config.TBotUserID))
	} else {
		logger.Info("T-bot system account verified",
			zap.String("user_id", config.TBotUserID))
	}

	generator := &FreeContestGenerator{
		db:       pool,
		kafka:    kafkaProducer,
		config:   config,
		obs:      obs,
		stopChan: make(chan struct{}),
	}

	// Start HTTP server for health checks
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", generator.handleHealth)
	mux.HandleFunc("/readyz", generator.handleReady)
	mux.Handle("/metrics", validation.InternalOnlyMiddleware(obs.MetricsHandler()))
	mux.HandleFunc("/trigger", generator.handleTrigger) // Manual trigger for testing

	server := &http.Server{
		Addr:    ":" + config.Port,
		Handler: mux,
	}

	// Start generator in background
	if config.Enabled {
		generator.start()
	} else {
		logger.Info("Free contest generator is disabled")
	}

	// Start HTTP server
	infra.SafeGo(logger, "free-contest-generator-http-server", func() {
		logger.Info("Starting HTTP server", zap.String("port", config.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server error", zap.Error(err))
		}
	})

	// Wait for shutdown signal (from parent context or OS signal)
	if parentCtx != nil {
		<-parentCtx.Done()
	} else {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
	}
	logger.Info("Shutting down...")

	// Stop generator
	generator.stop()

	// Shutdown HTTP server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", zap.Error(err))
	}

	logger.Info("Shutdown complete")
}

func loadConfig() *Config {
	config := &Config{
		Port:        freeContestPort(),
		PostgresDSN: secrets.BuildPostgresDSN(),

		KafkaBrokers: envconfig.GetEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopic:   envconfig.GetEnv("KAFKA_CONTESTS_TOPIC", "contests.v1"),

		Enabled:         envconfig.GetEnvBool("FREE_CONTEST_ENABLED", true),
		IntervalMinutes: envconfig.GetEnvInt("FREE_CONTEST_INTERVAL_MINUTES", 60),
		DurationMinutes: envconfig.GetEnvInt("FREE_CONTEST_DURATION_MINUTES", 60),
		WeekdaysOnly:    envconfig.GetEnvBool("FREE_CONTEST_WEEKDAYS_ONLY", true),
		StartHourUTC:    envconfig.GetEnvInt("FREE_CONTEST_START_HOUR_UTC", 6),
		EndHourUTC:      envconfig.GetEnvInt("FREE_CONTEST_END_HOUR_UTC", 22),
		LeadTimeMinutes: envconfig.GetEnvInt("FREE_CONTEST_LEAD_TIME_MINUTES", 5),

		TBotUserID: envconfig.GetEnv("TBOT_USER_ID", envconfig.GetEnv("TRAGGE_TRADER_USER_ID", "00000000-0000-0000-0000-000000000001")),

		CleanupEnabled:         envconfig.GetEnvBool("FREE_CONTEST_CLEANUP_ENABLED", true),
		CleanupIntervalMinutes: envconfig.GetEnvInt("FREE_CONTEST_CLEANUP_INTERVAL_MINUTES", 60),
		CleanupRetentionHours:  envconfig.GetEnvInt("FREE_CONTEST_CLEANUP_RETENTION_HOURS", 24),
	}

	// Parse asset classes
	assetClassesStr := envconfig.GetEnv("FREE_CONTEST_ASSET_CLASSES", "forex,crypto")
	config.AssetClasses = strings.Split(assetClassesStr, ",")
	for i, ac := range config.AssetClasses {
		config.AssetClasses[i] = strings.TrimSpace(ac)
	}

	return config
}

func (g *FreeContestGenerator) start() {
	g.obs.Logger.Logger.Info("Starting free contest generator",
		zap.Int("interval_minutes", g.config.IntervalMinutes),
		zap.Strings("asset_classes", g.config.AssetClasses),
		zap.Bool("weekdays_only", g.config.WeekdaysOnly),
		zap.Int("start_hour_utc", g.config.StartHourUTC),
		zap.Int("end_hour_utc", g.config.EndHourUTC),
	)

	// Start generation ticker
	g.wg.Add(1)
	go g.generationLoop()

	// Start cleanup ticker if enabled
	if g.config.CleanupEnabled {
		g.wg.Add(1)
		go g.cleanupLoop()
	}
}

func (g *FreeContestGenerator) stop() {
	close(g.stopChan)
	g.wg.Wait()
}

func (g *FreeContestGenerator) generationLoop() {
	defer g.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			g.obs.Logger.Logger.Error("generationLoop panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	logger := g.obs.Logger.Logger

	// Calculate time until next hour boundary
	now := time.Now().UTC()
	nextHour := now.Truncate(time.Hour).Add(time.Hour)
	initialWait := time.Until(nextHour)

	logger.Info("Waiting until next hour boundary", zap.Duration("wait", initialWait))

	// Wait until the next hour boundary
	select {
	case <-time.After(initialWait):
		// Generate contests at the hour boundary
		g.generateIfNeeded()
	case <-g.stopChan:
		return
	}

	// Then run every interval
	ticker := time.NewTicker(time.Duration(g.config.IntervalMinutes) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			g.generateIfNeeded()
		case <-g.stopChan:
			return
		}
	}
}

func (g *FreeContestGenerator) generateIfNeeded() {
	logger := g.obs.Logger.Logger
	now := time.Now().UTC()

	// Check if within operating hours
	if now.Hour() < g.config.StartHourUTC || now.Hour() >= g.config.EndHourUTC {
		logger.Debug("Outside operating hours, skipping generation",
			zap.Int("current_hour", now.Hour()),
			zap.Int("start_hour", g.config.StartHourUTC),
			zap.Int("end_hour", g.config.EndHourUTC),
		)
		return
	}

	// Check if weekday (if weekdays only)
	if g.config.WeekdaysOnly {
		weekday := now.Weekday()
		if weekday == time.Saturday || weekday == time.Sunday {
			logger.Debug("Weekend day, skipping generation", zap.String("day", weekday.String()))
			return
		}
	}

	// Generate contests for each asset class
	for _, assetClass := range g.config.AssetClasses {
		if err := g.generateFreeContest(assetClass); err != nil {
			logger.Error("Failed to generate free contest",
				zap.String("asset_class", assetClass),
				zap.Error(err),
			)
		}
	}

	// Reconcile any orphaned contests whose Kafka events were lost
	g.reconcileOrphanedContests()
}

// getActiveSymbolsForAssetClass queries active symbols from the DB for the given asset class.
func getActiveSymbolsForAssetClass(ctx context.Context, tx *sql.Tx, assetClass string) ([]string, error) {
	var assetTypes []string
	switch assetClass {
	case "crypto":
		assetTypes = []string{"crypto"}
	case "forex":
		assetTypes = []string{"forex", "commodity"}
	case "stocks":
		assetTypes = []string{"stock"}
	case "mixed":
		assetTypes = []string{"crypto", "forex"}
	default:
		return nil, fmt.Errorf("unknown asset class: %s", assetClass)
	}

	placeholders := make([]string, len(assetTypes))
	args := make([]interface{}, len(assetTypes))
	for i, t := range assetTypes {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = t
	}

	query := fmt.Sprintf(`SELECT symbol FROM symbols
		WHERE is_active = true AND asset_type IN (%s)
		ORDER BY sort_order ASC, symbol ASC`,
		strings.Join(placeholders, ", "))

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		symbols = append(symbols, s)
	}
	return symbols, rows.Err()
}

func (g *FreeContestGenerator) generateFreeContest(assetClass string) error {
	logger := g.obs.Logger.Logger
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get the template for this asset class
	templateKey := fmt.Sprintf("%s_free_practice", assetClass)
	template := contracts.GetContestTemplate(templateKey)
	if template == nil {
		return fmt.Errorf("template not found: %s", templateKey)
	}

	now := time.Now().UTC()
	startsAt := now.Add(time.Duration(g.config.LeadTimeMinutes) * time.Minute)
	endsAt := startsAt.Add(time.Duration(template.DurationMinutes) * time.Minute)

	// Generate unique contest name with timestamp
	contestName := fmt.Sprintf("Free %s Practice - %s", cases.Title(language.English).String(assetClass), startsAt.Format("15:04 UTC"))

	// Start transaction first so the duplicate check runs inside the transaction
	// boundary. This prevents the TOCTOU race where another instance inserts
	// between our check and our insert.
	tx, err := g.db.Primary().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if a similar contest already exists for this time slot (inside tx)
	var existingCount int
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contests
		 WHERE is_free = true
		   AND asset_class = $1
		   AND starts_at >= $2
		   AND starts_at < $3`,
		assetClass, startsAt.Add(-30*time.Minute), startsAt.Add(30*time.Minute),
	).Scan(&existingCount)
	if err != nil {
		return fmt.Errorf("failed to check existing contests: %w", err)
	}

	if existingCount > 0 {
		logger.Debug("Free contest already exists for this time slot",
			zap.String("asset_class", assetClass),
			zap.Time("starts_at", startsAt),
		)
		return nil
	}

	// Insert the contest
	contestID := uuid.New().String()
	description := fmt.Sprintf("Free practice tournament for %s trading. No entry fee, no prizes - perfect for learning!", cases.Title(language.English).String(assetClass))

	_, err = tx.ExecContext(ctx,
		`INSERT INTO contests (
			id, name, description, starts_at, ends_at, status, entry_fee_cents, platform_fee_bps, qty_total,
			duration_type, asset_class, duration_minutes, min_participants, max_participants,
			registration_deadline, auto_start, commission_rate, is_free, auto_generated
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		contestID,
		contestName,
		description,
		startsAt,
		endsAt,
		"scheduled", // Start as scheduled, will open for registration immediately
		0,           // Free - no entry fee
		0,           // No platform fee
		template.QtyAllocation,
		string(template.DurationType),
		assetClass,
		template.DurationMinutes,
		template.MinParticipants,     // Use template min participants
		template.MaxParticipants,     // Use template max
		startsAt.Add(-1*time.Second), // Registration deadline = 1 second before start
		true,                         // Auto-start when min participants met
		0.0,                          // No commission
		true,                         // Is free
		true,                         // Auto-generated
	)
	if err != nil {
		// Handle unique constraint violation (PostgreSQL error code 23505) gracefully.
		// This can happen if another generator instance inserted the same contest
		// concurrently, despite the in-transaction duplicate check.
		if strings.Contains(err.Error(), "23505") ||
			strings.Contains(err.Error(), "duplicate key") {
			logger.Debug("Contest creation conflict (unique constraint), skipping",
				zap.String("asset_class", assetClass),
				zap.Time("starts_at", startsAt),
			)
			return nil
		}
		return fmt.Errorf("failed to insert contest: %w", err)
	}

	// Add symbols from DB (single source of truth), fallback to template
	contestSymbols, symbolErr := getActiveSymbolsForAssetClass(ctx, tx, assetClass)
	if symbolErr != nil || len(contestSymbols) == 0 {
		logger.Warn("Failed to query symbols from DB, using template defaults",
			zap.String("asset_class", assetClass), zap.Error(symbolErr))
		contestSymbols = template.Symbols
	}
	var insertedSymbols int
	for _, symbol := range contestSymbols {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO contest_symbols (contest_id, symbol, enabled) VALUES ($1, $2, TRUE)
			 ON CONFLICT (contest_id, symbol) DO NOTHING`,
			contestID, symbol,
		)
		if err != nil {
			logger.Warn("Failed to insert contest symbol", zap.Error(err), zap.String("symbol", symbol))
		} else {
			insertedSymbols++
		}
	}
	if insertedSymbols == 0 {
		return fmt.Errorf("no symbols inserted for contest %s (asset_class=%s), rolling back", contestID, assetClass)
	}

	// Transition to registration_open immediately
	_, err = tx.ExecContext(ctx,
		`UPDATE contests SET status = 'registration_open' WHERE id = $1`,
		contestID,
	)
	if err != nil {
		return fmt.Errorf("failed to open registration: %w", err)
	}

	// Record status transition
	_, err = tx.ExecContext(ctx,
		`INSERT INTO contest_status_history (contest_id, from_status, to_status, reason, metadata)
		 VALUES ($1, $2, $3, $4, $5)`,
		contestID,
		"scheduled",
		"registration_open",
		"Auto-generated free contest",
		`{"auto_generated": true}`,
	)
	if err != nil {
		logger.Warn("Failed to record status history", zap.Error(err))
	}

	// Auto-register the T-bot as a participant
	_, err = tx.ExecContext(ctx,
		`INSERT INTO contest_participants (contest_id, user_id, qty_total, qty_available, is_system)
		 VALUES ($1, $2, $3, $3, TRUE)
		 ON CONFLICT (contest_id, user_id) DO NOTHING`,
		contestID, g.config.TBotUserID, template.QtyAllocation,
	)
	if err != nil {
		return fmt.Errorf("failed to auto-register T-bot: %w", err)
	}

	logger.Info("Auto-joined T-bot to contest",
		zap.String("contest_id", contestID),
		zap.String("user_id", g.config.TBotUserID),
	)

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Info("Generated free contest",
		zap.String("contest_id", contestID),
		zap.String("name", contestName),
		zap.String("asset_class", assetClass),
		zap.Int("min_participants", template.MinParticipants),
		zap.Time("starts_at", startsAt),
		zap.Time("ends_at", endsAt),
	)

	// Publish contest state event to Kafka with retry.
	// NOTE: There is an inherent race between DB commit and Kafka publish.
	// If this fails after all retries, the contest exists in DB but downstream
	// consumers (contest-scheduler) won't see it until the reconciliation loop
	// picks it up (see reconcileOrphanedContests).
	if g.kafka != nil {
		if err := g.publishContestEventWithRetry(contestID, "registration_open"); err != nil {
			logger.Error("Failed to publish contest event after retries - contest will be reconciled",
				zap.String("contest_id", contestID),
				zap.Error(err),
			)
		}
	}

	return nil
}

func (g *FreeContestGenerator) publishContestEventWithRetry(contestID, status string) error {
	event := contracts.ContestState{
		ContestID: contestID,
		Status:    contracts.ContestStatus(status),
		Ts:        time.Now().UTC().UnixMilli(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal contest event: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: g.config.KafkaTopic,
		Key:   sarama.StringEncoder(contestID),
		Value: sarama.ByteEncoder(data),
	}

	// Retry up to 3 times with exponential backoff
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		_, _, lastErr = g.kafka.SendMessage(msg)
		if lastErr == nil {
			return nil
		}
		g.obs.Logger.Logger.Warn("Kafka publish attempt failed, retrying",
			zap.String("contest_id", contestID),
			zap.Int("attempt", attempt+1),
			zap.Error(lastErr),
		)
		time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // 1s, 2s, 4s
	}

	return fmt.Errorf("kafka publish failed after 3 attempts: %w", lastErr)
}

// reconcileOrphanedContests finds contests that were created in DB but whose Kafka
// events may have been lost (e.g., due to crash between DB commit and Kafka publish).
// It re-publishes events for any contest in registration_open status that was
// auto-generated and has a starts_at in the future.
func (g *FreeContestGenerator) reconcileOrphanedContests() {
	if g.kafka == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := g.obs.Logger.Logger

	rows, err := g.db.Primary().QueryContext(ctx,
		`SELECT id FROM contests
		 WHERE is_free = true
		   AND auto_generated = true
		   AND status = 'registration_open'
		   AND starts_at > NOW()
		   AND created_at < NOW() - INTERVAL '2 minutes'`)
	if err != nil {
		logger.Error("Failed to query orphaned contests", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var contestID string
		if err := rows.Scan(&contestID); err != nil {
			logger.Error("Failed to scan orphaned contest", zap.Error(err))
			continue
		}
		logger.Info("Re-publishing event for orphaned contest", zap.String("contest_id", contestID))
		if err := g.publishContestEventWithRetry(contestID, "registration_open"); err != nil {
			logger.Error("Failed to re-publish orphaned contest event",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}
	}
}

func (g *FreeContestGenerator) cleanupLoop() {
	defer g.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			g.obs.Logger.Logger.Error("cleanupLoop panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	ticker := time.NewTicker(time.Duration(g.config.CleanupIntervalMinutes) * time.Minute)
	defer ticker.Stop()

	// Run cleanup immediately on startup
	g.cleanupOldContests()

	for {
		select {
		case <-ticker.C:
			g.cleanupOldContests()
		case <-g.stopChan:
			return
		}
	}
}

func (g *FreeContestGenerator) cleanupOldContests() {
	logger := g.obs.Logger.Logger
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	retentionInterval := fmt.Sprintf("%d hours", g.config.CleanupRetentionHours)
	// Use a longer retention for stuck contests (3x normal) that have no ended_at
	stuckRetentionInterval := fmt.Sprintf("%d hours", g.config.CleanupRetentionHours*3)

	result, err := g.db.Primary().ExecContext(ctx,
		`DELETE FROM contests
		 WHERE is_free = true
		   AND auto_generated = true
		   AND status IN ('completed', 'cancelled')
		   AND (
		       (ended_at IS NOT NULL AND ended_at < NOW() - $1::interval)
		       OR
		       (ended_at IS NULL AND starts_at < NOW() - $2::interval)
		   )`,
		retentionInterval,
		stuckRetentionInterval,
	)
	if err != nil {
		logger.Error("Failed to cleanup old free contests", zap.Error(err))
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		logger.Info("Cleaned up old free contests", zap.Int64("count", rowsAffected))
	}
}

// HTTP Handlers

func (g *FreeContestGenerator) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (g *FreeContestGenerator) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check database connection
	if err := g.db.Primary().PingContext(ctx); err != nil {
		http.Error(w, "Database not ready", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (g *FreeContestGenerator) handleTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	g.obs.Logger.Logger.Info("Manual trigger received")

	// Generate contests for all asset classes
	for _, assetClass := range g.config.AssetClasses {
		if err := g.generateFreeContest(assetClass); err != nil {
			g.obs.Logger.Logger.Error("Failed to generate free contest on trigger",
				zap.String("asset_class", assetClass),
				zap.Error(err),
			)
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Triggered"))
}

// Helper functions

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic("required environment variable not set: " + key)
	}
	return val
}

func freeContestPort() string {
	if p := os.Getenv("FREE_CONTEST_GENERATOR_PORT"); p != "" {
		return p
	}
	return envconfig.GetEnv("PORT", "8089")
}
