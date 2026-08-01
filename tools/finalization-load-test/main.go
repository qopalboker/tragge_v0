// Package main provides a contest finalization load testing tool for the tragge trading platform.
// It tests prize distribution performance for contests with large participant counts.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Config holds the load test configuration.
type Config struct {
	// Test settings
	NumParticipants int
	ContestID       string    // If empty, will create a new contest
	CreateContest   bool      // Create a new contest for testing
	TriggerMethod   string    // "kafka" or "api"

	// Connection settings
	PostgresDSN     string
	RedisAddr       string
	KafkaBrokers    []string
	AdminBFFURL     string
	UserBFFURL      string

	// Authentication (for API method)
	AdminEmail    string
	AdminPassword string

	// Test parameters
	EntryFeeCents  int
	PlatformFeeBps int
	MinScore       float64
	MaxScore       float64
}

// Metrics tracks finalization metrics.
type Metrics struct {
	SetupDuration       time.Duration
	ParticipantsDuration time.Duration
	TriggerDuration      time.Duration
	FinalizationDuration time.Duration
	TotalDuration        time.Duration

	ParticipantsCreated int
	RanksWritten        int
	PrizesDistributed   int
	WalletsUpdated      int

	Errors []string
}

// ContestState represents the ContestState Kafka message.
type ContestState struct {
	ContestID string `json:"contest_id"`
	Phase     string `json:"phase"`
	Timestamp int64  `json:"timestamp"`
}

// LoginRequest represents the login API request.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents the authentication response.
type AuthResponse struct {
	AccessToken string `json:"access_token"`
}

// CreateContestRequest represents the contest creation request.
type CreateContestRequest struct {
	Name           string    `json:"name"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	EntryFeeCents  int       `json:"entry_fee_cents"`
	PlatformFeeBps int       `json:"platform_fee_bps"`
	QtyTotal       int64     `json:"qty_total"`
	Status         string    `json:"status"`
}

// ContestResponse represents the contest API response.
type ContestResponse struct {
	ID string `json:"id"`
}

// UpdateContestRequest represents the contest update request.
type UpdateContestRequest struct {
	Status *string `json:"status,omitempty"`
}

func main() {
	cfg := parseFlags()

	if err := run(cfg); err != nil {
		log.Fatalf("Load test failed: %v", err)
	}
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.IntVar(&cfg.NumParticipants, "participants", 1000, "Number of participants to simulate")
	flag.StringVar(&cfg.ContestID, "contest-id", "", "Existing contest ID (creates new if empty)")
	flag.BoolVar(&cfg.CreateContest, "create-contest", true, "Create a new contest for testing")
	flag.StringVar(&cfg.TriggerMethod, "trigger", "kafka", "Finalization trigger method: kafka or api")
	flag.StringVar(&cfg.PostgresDSN, "postgres-dsn", "postgres://app:app@localhost:5432/app?sslmode=disable", "PostgreSQL DSN")
	flag.StringVar(&cfg.RedisAddr, "redis-addr", "localhost:6379", "Redis address")
	flag.StringVar(&cfg.AdminBFFURL, "admin-bff", "http://localhost:8083", "Admin BFF URL")
	flag.StringVar(&cfg.UserBFFURL, "user-bff", "http://localhost:8081", "User BFF URL")
	flag.StringVar(&cfg.AdminEmail, "admin-email", "", "Admin email (for API trigger)")
	flag.StringVar(&cfg.AdminPassword, "admin-password", "", "Admin password (for API trigger)")
	flag.IntVar(&cfg.EntryFeeCents, "entry-fee", 1000, "Entry fee in cents")
	flag.IntVar(&cfg.PlatformFeeBps, "platform-fee-bps", 1000, "Platform fee in basis points")
	flag.Float64Var(&cfg.MinScore, "min-score", -10000, "Minimum score for participants")
	flag.Float64Var(&cfg.MaxScore, "max-score", 50000, "Maximum score for participants")

	var kafkaBrokersStr string
	flag.StringVar(&kafkaBrokersStr, "kafka-brokers", "localhost:9092", "Kafka brokers (comma-separated)")

	flag.Parse()

	cfg.KafkaBrokers = parseCommaSeparated(kafkaBrokersStr)

	// Validate configuration
	if cfg.TriggerMethod == "api" && (cfg.AdminEmail == "" || cfg.AdminPassword == "") {
		fmt.Fprintln(os.Stderr, "Error: -admin-email and -admin-password are required for API trigger method")
		flag.Usage()
		os.Exit(1)
	}

	if !cfg.CreateContest && cfg.ContestID == "" {
		fmt.Fprintln(os.Stderr, "Error: either -create-contest or -contest-id is required")
		flag.Usage()
		os.Exit(1)
	}

	return cfg
}

func parseCommaSeparated(s string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > start {
				item := s[start:i]
				for len(item) > 0 && item[0] == ' ' {
					item = item[1:]
				}
				for len(item) > 0 && item[len(item)-1] == ' ' {
					item = item[:len(item)-1]
				}
				if len(item) > 0 {
					result = append(result, item)
				}
			}
			start = i + 1
		}
	}
	return result
}

func run(cfg *Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("\nReceived shutdown signal, stopping test...")
		cancel()
	}()

	// Print configuration
	fmt.Println("=== Contest Finalization Load Test ===")
	fmt.Printf("Participants:    %d\n", cfg.NumParticipants)
	fmt.Printf("Entry Fee:       %d cents\n", cfg.EntryFeeCents)
	fmt.Printf("Platform Fee:    %d bps\n", cfg.PlatformFeeBps)
	fmt.Printf("Trigger Method:  %s\n", cfg.TriggerMethod)
	fmt.Printf("PostgreSQL:      %s\n", maskDSN(cfg.PostgresDSN))
	fmt.Printf("Redis:           %s\n", cfg.RedisAddr)
	fmt.Printf("Kafka:           %v\n", cfg.KafkaBrokers)
	fmt.Println()

	metrics := &Metrics{
		Errors: make([]string, 0),
	}
	totalStart := time.Now()

	// Step 1: Connect to services
	fmt.Println("Connecting to services...")
	setupStart := time.Now()

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}
	fmt.Println("  PostgreSQL: connected")

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to ping Redis: %w", err)
	}
	fmt.Println("  Redis: connected")

	// Connect to Kafka (only if using Kafka trigger)
	var kafkaClient *kgo.Client
	if cfg.TriggerMethod == "kafka" {
		kafkaClient, err = kgo.NewClient(
			kgo.SeedBrokers(cfg.KafkaBrokers...),
		)
		if err != nil {
			return fmt.Errorf("failed to connect to Kafka: %w", err)
		}
		defer kafkaClient.Close()
		fmt.Println("  Kafka: connected")
	}

	metrics.SetupDuration = time.Since(setupStart)
	fmt.Printf("  Setup completed in %s\n", metrics.SetupDuration)
	fmt.Println()

	// Step 2: Create or get contest
	var contestID string
	if cfg.CreateContest {
		fmt.Println("Creating test contest...")
		contestID, err = createTestContest(ctx, cfg, db)
		if err != nil {
			return fmt.Errorf("failed to create contest: %w", err)
		}
		cfg.ContestID = contestID
	} else {
		contestID = cfg.ContestID
	}
	fmt.Printf("  Using contest: %s\n", contestID)
	fmt.Println()

	// Step 3: Create participants and populate leaderboard
	fmt.Printf("Creating %d participants...\n", cfg.NumParticipants)
	participantsStart := time.Now()

	participants, err := createParticipants(ctx, cfg, db, rdb, contestID)
	if err != nil {
		return fmt.Errorf("failed to create participants: %w", err)
	}
	metrics.ParticipantsCreated = len(participants)
	metrics.ParticipantsDuration = time.Since(participantsStart)

	fmt.Printf("  Created %d participants in %s\n", metrics.ParticipantsCreated, metrics.ParticipantsDuration)
	fmt.Println()

	// Step 4: Verify leaderboard is populated
	fmt.Println("Verifying leaderboard...")
	count, err := rdb.ZCard(ctx, "lb:"+contestID).Result()
	if err != nil {
		return fmt.Errorf("failed to check leaderboard: %w", err)
	}
	fmt.Printf("  Leaderboard has %d entries\n", count)
	fmt.Println()

	// Step 5: Set contest status to running (so it can be ended)
	fmt.Println("Setting contest status to 'running'...")
	_, err = db.ExecContext(ctx, `UPDATE contests SET status = 'running' WHERE id = $1`, contestID)
	if err != nil {
		return fmt.Errorf("failed to update contest status: %w", err)
	}
	fmt.Println("  Contest is now running")
	fmt.Println()

	// Step 6: Trigger finalization
	fmt.Printf("Triggering finalization via %s...\n", cfg.TriggerMethod)
	triggerStart := time.Now()

	if cfg.TriggerMethod == "kafka" {
		err = triggerViaKafka(ctx, kafkaClient, contestID)
	} else {
		err = triggerViaAPI(ctx, cfg, contestID)
	}
	if err != nil {
		return fmt.Errorf("failed to trigger finalization: %w", err)
	}
	metrics.TriggerDuration = time.Since(triggerStart)
	fmt.Printf("  Trigger sent in %s\n", metrics.TriggerDuration)
	fmt.Println()

	// Step 7: Wait for and monitor finalization
	fmt.Println("Waiting for finalization to complete...")
	finalizationStart := time.Now()

	// Poll for completion
	maxWait := 5 * time.Minute
	pollInterval := 500 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var status string
		err := db.QueryRowContext(ctx, `SELECT status FROM contests WHERE id = $1`, contestID).Scan(&status)
		if err != nil {
			metrics.Errors = append(metrics.Errors, fmt.Sprintf("status check: %v", err))
			time.Sleep(pollInterval)
			continue
		}

		if status == "completed" {
			metrics.FinalizationDuration = time.Since(finalizationStart)
			fmt.Printf("  Finalization completed in %s\n", metrics.FinalizationDuration)
			break
		}

		time.Sleep(pollInterval)
	}

	if metrics.FinalizationDuration == 0 {
		return fmt.Errorf("finalization timed out after %s", maxWait)
	}
	fmt.Println()

	// Step 8: Verify results
	fmt.Println("Verifying finalization results...")

	// Check how many participants have final ranks
	var rankedCount int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1 AND final_rank IS NOT NULL`,
		contestID,
	).Scan(&rankedCount)
	if err != nil {
		metrics.Errors = append(metrics.Errors, fmt.Sprintf("ranked count: %v", err))
	} else {
		metrics.RanksWritten = rankedCount
		fmt.Printf("  Participants with ranks: %d\n", rankedCount)
	}

	// Check how many have prizes
	var prizesCount int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1 AND final_prize_cents > 0`,
		contestID,
	).Scan(&prizesCount)
	if err != nil {
		metrics.Errors = append(metrics.Errors, fmt.Sprintf("prizes count: %v", err))
	} else {
		metrics.PrizesDistributed = prizesCount
		fmt.Printf("  Participants with prizes: %d\n", prizesCount)
	}

	// Check leaderboard snapshot
	var snapshotCount int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM leaderboard_snapshots WHERE contest_id = $1`,
		contestID,
	).Scan(&snapshotCount)
	if err != nil {
		metrics.Errors = append(metrics.Errors, fmt.Sprintf("snapshot count: %v", err))
	} else {
		fmt.Printf("  Leaderboard snapshots: %d\n", snapshotCount)
	}
	fmt.Println()

	metrics.TotalDuration = time.Since(totalStart)

	// Print final results
	printResults(cfg, metrics)

	return nil
}

func createTestContest(ctx context.Context, cfg *Config, db *sql.DB) (string, error) {
	contestID := uuid.New().String()
	now := time.Now()

	_, err := db.ExecContext(ctx,
		`INSERT INTO contests (id, name, starts_at, ends_at, entry_fee_cents, platform_fee_bps, qty_total, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		contestID,
		fmt.Sprintf("Finalization Test %s", now.Format("2006-01-02 15:04:05")),
		now.Add(-1*time.Hour),
		now,
		cfg.EntryFeeCents,
		cfg.PlatformFeeBps,
		10000, // qty_total
		"draft",
	)
	if err != nil {
		return "", err
	}

	return contestID, nil
}

type participant struct {
	UserID string
	Score  float64
}

func createParticipants(ctx context.Context, cfg *Config, db *sql.DB, rdb *redis.Client, contestID string) ([]participant, error) {
	participants := make([]participant, cfg.NumParticipants)

	// Generate random scores
	for i := 0; i < cfg.NumParticipants; i++ {
		participants[i] = participant{
			UserID: uuid.New().String(),
			Score:  cfg.MinScore + rand.Float64()*(cfg.MaxScore-cfg.MinScore),
		}
	}

	// Sort by score descending for insertion order
	sort.Slice(participants, func(i, j int) bool {
		return participants[i].Score > participants[j].Score
	})

	// Batch insert into database
	batchSize := 100
	for i := 0; i < len(participants); i += batchSize {
		end := i + batchSize
		if end > len(participants) {
			end = len(participants)
		}

		batch := participants[i:end]

		// Insert users first (simplified - in production would use proper user registration)
		for _, p := range batch {
			_, err := db.ExecContext(ctx,
				`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`,
				p.UserID,
				fmt.Sprintf("%s@finalization-test.local", p.UserID[:8]),
				"$argon2id$v=19$m=65536,t=1,p=2$dummy$hash", // Dummy hash
			)
			if err != nil {
				return nil, fmt.Errorf("insert user: %w", err)
			}
		}

		// Insert contest participants
		for _, p := range batch {
			_, err := db.ExecContext(ctx,
				`INSERT INTO contest_participants (contest_id, user_id, qty_total, qty_available, total_score)
				 VALUES ($1, $2, $3, $4, $5)
				 ON CONFLICT (contest_id, user_id) DO UPDATE SET total_score = $5`,
				contestID, p.UserID, 10000, 10000, p.Score,
			)
			if err != nil {
				return nil, fmt.Errorf("insert participant: %w", err)
			}
		}

		// Add to Redis leaderboard
		members := make([]redis.Z, len(batch))
		for j, p := range batch {
			members[j] = redis.Z{
				Score:  p.Score,
				Member: p.UserID,
			}
		}

		if err := rdb.ZAdd(ctx, "lb:"+contestID, members...).Err(); err != nil {
			return nil, fmt.Errorf("zadd: %w", err)
		}

		// Progress indicator
		if (i+batchSize)%500 == 0 || end == len(participants) {
			fmt.Printf("  Progress: %d/%d participants\n", end, len(participants))
		}
	}

	return participants, nil
}

func triggerViaKafka(ctx context.Context, client *kgo.Client, contestID string) error {
	state := ContestState{
		ContestID: contestID,
		Phase:     "ENDED",
		Timestamp: time.Now().UnixMilli(),
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	record := &kgo.Record{
		Topic: "contests.v1",
		Key:   []byte(contestID),
		Value: data,
	}

	result := client.ProduceSync(ctx, record)
	if err := result.FirstErr(); err != nil {
		return err
	}

	return nil
}

func triggerViaAPI(ctx context.Context, cfg *Config, contestID string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	// Login as admin
	loginReq := LoginRequest{
		Email:    cfg.AdminEmail,
		Password: cfg.AdminPassword,
	}
	loginBody, _ := json.Marshal(loginReq)

	resp, err := client.Post(cfg.UserBFFURL+"/api/user/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed: %s", string(body))
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("decode auth response: %w", err)
	}

	// Update contest status to completed via admin API
	updateReq := UpdateContestRequest{
		Status: strPtr("completed"),
	}
	updateBody, _ := json.Marshal(updateReq)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPatch,
		cfg.AdminBFFURL+"/api/admin/contests/"+contestID,
		bytes.NewReader(updateBody))
	req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("update contest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func strPtr(s string) *string {
	return &s
}

func maskDSN(dsn string) string {
	// Simple masking - replace password
	// In a real implementation, would use proper URL parsing
	return "postgres://***:***@..."
}

func printResults(cfg *Config, metrics *Metrics) {
	fmt.Println()
	fmt.Println("=== Contest Finalization Load Test Results ===")
	fmt.Println()

	fmt.Println("Configuration:")
	fmt.Printf("  Participants:     %d\n", cfg.NumParticipants)
	fmt.Printf("  Entry Fee:        %d cents\n", cfg.EntryFeeCents)
	fmt.Printf("  Platform Fee:     %d bps\n", cfg.PlatformFeeBps)
	fmt.Printf("  Trigger Method:   %s\n", cfg.TriggerMethod)
	fmt.Println()

	fmt.Println("Timing:")
	fmt.Printf("  Setup:            %s\n", metrics.SetupDuration.Round(time.Millisecond))
	fmt.Printf("  Participants:     %s\n", metrics.ParticipantsDuration.Round(time.Millisecond))
	fmt.Printf("  Trigger:          %s\n", metrics.TriggerDuration.Round(time.Millisecond))
	fmt.Printf("  Finalization:     %s\n", metrics.FinalizationDuration.Round(time.Millisecond))
	fmt.Printf("  Total:            %s\n", metrics.TotalDuration.Round(time.Millisecond))
	fmt.Println()

	// Calculate throughput
	if metrics.FinalizationDuration > 0 {
		participantsPerSecond := float64(metrics.ParticipantsCreated) / metrics.FinalizationDuration.Seconds()
		fmt.Println("Throughput:")
		fmt.Printf("  Participants/sec: %.2f\n", participantsPerSecond)
		fmt.Println()
	}

	fmt.Println("Results:")
	fmt.Printf("  Participants Created: %d\n", metrics.ParticipantsCreated)
	fmt.Printf("  Ranks Written:        %d\n", metrics.RanksWritten)
	fmt.Printf("  Prizes Distributed:   %d\n", metrics.PrizesDistributed)
	fmt.Println()

	// Verification
	successRate := float64(metrics.RanksWritten) / float64(metrics.ParticipantsCreated) * 100
	fmt.Println("Verification:")
	fmt.Printf("  Rank Success Rate:    %.2f%%\n", successRate)
	if metrics.RanksWritten == metrics.ParticipantsCreated {
		fmt.Println("  Status:               PASSED ✓")
	} else {
		fmt.Println("  Status:               FAILED ✗")
	}
	fmt.Println()

	if len(metrics.Errors) > 0 {
		fmt.Println("Errors:")
		for _, e := range metrics.Errors {
			fmt.Printf("  - %s\n", e)
		}
		fmt.Println()
	}

	// Performance assessment
	fmt.Println("Performance Assessment:")
	if metrics.FinalizationDuration < 30*time.Second {
		fmt.Println("  ✓ Excellent: Finalization completed in under 30 seconds")
	} else if metrics.FinalizationDuration < 60*time.Second {
		fmt.Println("  ✓ Good: Finalization completed in under 1 minute")
	} else if metrics.FinalizationDuration < 120*time.Second {
		fmt.Println("  ⚠ Acceptable: Finalization completed in under 2 minutes")
	} else {
		fmt.Println("  ✗ Needs Improvement: Finalization took over 2 minutes")
	}
}
