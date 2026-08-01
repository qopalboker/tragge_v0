// Package integration provides integration test utilities with testcontainers.
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twmb/franz-go/pkg/kgo"
)

// TestEnv holds all test container instances and connection details.
type TestEnv struct {
	// Containers
	PostgresContainer *postgres.PostgresContainer
	RedisContainer    *tcredis.RedisContainer
	RedpandaContainer *redpanda.Container

	// Connection strings
	PostgresDSN    string
	RedisAddr      string
	KafkaBrokers   []string

	// Clients
	DB          *sql.DB
	PgxConn     *pgx.Conn
	RedisClient *redis.Client
	KafkaClient *kgo.Client

	// Configuration
	JWTSecret string
}

// SetupTestEnv creates all required test containers.
func SetupTestEnv(t *testing.T, ctx context.Context) *TestEnv {
	t.Helper()

	env := &TestEnv{
		JWTSecret: "test-jwt-secret-for-integration-tests-12345",
	}

	var err error

	// Start PostgreSQL container
	t.Log("Starting PostgreSQL container...")
	env.PostgresContainer, err = postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	env.PostgresDSN, err = env.PostgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get PostgreSQL connection string: %v", err)
	}
	t.Logf("PostgreSQL DSN: %s", env.PostgresDSN)

	// Connect to PostgreSQL
	env.DB, err = sql.Open("pgx", env.PostgresDSN)
	if err != nil {
		t.Fatalf("Failed to open PostgreSQL connection: %v", err)
	}

	// Connect with pgx for tests that need a raw connection
	env.PgxConn, err = pgx.Connect(ctx, env.PostgresDSN)
	if err != nil {
		t.Fatalf("Failed to connect with pgx: %v", err)
	}

	// Run migrations
	t.Log("Running database migrations...")
	if err := runMigrations(ctx, env.DB); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Start Redis container
	t.Log("Starting Redis container...")
	env.RedisContainer, err = tcredis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}

	env.RedisAddr, err = env.RedisContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("Failed to get Redis connection string: %v", err)
	}
	// Remove redis:// prefix for go-redis client
	if len(env.RedisAddr) > 8 && env.RedisAddr[:8] == "redis://" {
		env.RedisAddr = env.RedisAddr[8:]
	}
	t.Logf("Redis address: %s", env.RedisAddr)

	// Connect to Redis
	env.RedisClient = redis.NewClient(&redis.Options{
		Addr: env.RedisAddr,
	})

	// Start Redpanda container
	t.Log("Starting Redpanda container...")
	env.RedpandaContainer, err = redpanda.Run(ctx,
		"docker.redpanda.com/redpandadata/redpanda:v24.1.1",
		redpanda.WithAutoCreateTopics(),
	)
	if err != nil {
		t.Fatalf("Failed to start Redpanda container: %v", err)
	}

	kafkaBroker, err := env.RedpandaContainer.KafkaSeedBroker(ctx)
	if err != nil {
		t.Fatalf("Failed to get Kafka broker address: %v", err)
	}
	env.KafkaBrokers = []string{kafkaBroker}
	t.Logf("Kafka brokers: %v", env.KafkaBrokers)

	// Connect to Kafka
	env.KafkaClient, err = kgo.NewClient(
		kgo.SeedBrokers(env.KafkaBrokers...),
	)
	if err != nil {
		t.Fatalf("Failed to create Kafka client: %v", err)
	}

	// Create required topics
	t.Log("Creating Kafka topics...")
	if err := createKafkaTopics(ctx, env.KafkaClient); err != nil {
		t.Fatalf("Failed to create Kafka topics: %v", err)
	}

	return env
}

// Cleanup terminates all containers.
func (env *TestEnv) Cleanup(t *testing.T, ctx context.Context) {
	t.Helper()

	if env.KafkaClient != nil {
		env.KafkaClient.Close()
	}

	if env.RedisClient != nil {
		env.RedisClient.Close()
	}

	if env.PgxConn != nil {
		env.PgxConn.Close(ctx)
	}

	if env.DB != nil {
		env.DB.Close()
	}

	if env.PostgresContainer != nil {
		if err := env.PostgresContainer.Terminate(ctx); err != nil {
			t.Logf("Warning: Failed to terminate PostgreSQL container: %v", err)
		}
	}

	if env.RedisContainer != nil {
		if err := env.RedisContainer.Terminate(ctx); err != nil {
			t.Logf("Warning: Failed to terminate Redis container: %v", err)
		}
	}

	if env.RedpandaContainer != nil {
		if err := env.RedpandaContainer.Terminate(ctx); err != nil {
			t.Logf("Warning: Failed to terminate Redpanda container: %v", err)
		}
	}
}

// runMigrations discovers and applies all *.up.sql migration files from
// packages/db/migrations/ in numeric order. This ensures the test database
// has the complete schema regardless of which migrations are added later.
func runMigrations(ctx context.Context, db *sql.DB) error {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("failed to determine source file path")
	}
	basePath := filepath.Join(filepath.Dir(filename), "../../packages/db/migrations/")

	entries, err := os.ReadDir(basePath)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}

	// Sort by filename (numeric prefix ensures correct order)
	sort.Strings(upFiles)

	for _, migration := range upFiles {
		migrationPath := filepath.Join(basePath, migration)
		migrationSQL, err := os.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", migration, err)
		}

		_, err = db.ExecContext(ctx, string(migrationSQL))
		if err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", migration, err)
		}
	}

	return nil
}

// createKafkaTopics creates the required Kafka topics.
func createKafkaTopics(ctx context.Context, client *kgo.Client) error {
	topics := []string{
		"orders.v1",
		"ticks.v1",
		"fills.v1",
		"positions.v1",
		"order_acks.v1",
		"order_cancelled.v1",
		"pnl_deltas.v1",
		"contests.v1",
		"close_positions.v1",
		"cancel_orders.v1",
		"modify_tpsl.v1",
		"position_closed.v1",
		"settlement_requests.v1",
		"settlement_events.v1",
		"contest_close_positions.v1",
		"contest_cancel_orders.v1",
		"notifications.v1",
		"alerts.v1",
	}

	// Note: Redpanda with auto-create will create topics on first produce/consume
	// We just need to produce a dummy message to create them
	for _, topic := range topics {
		record := &kgo.Record{
			Topic: topic,
			Key:   []byte("init"),
			Value: []byte("{}"),
		}
		results := client.ProduceSync(ctx, record)
		if err := results.FirstErr(); err != nil {
			return fmt.Errorf("failed to create topic %s: %w", topic, err)
		}
	}

	return nil
}

// CreateTestUser creates a test user in the database and returns the user ID.
func (env *TestEnv) CreateTestUser(ctx context.Context, t *testing.T, email, passwordHash string) string {
	t.Helper()

	var userID string
	err := env.DB.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`,
		email, passwordHash,
	).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Assign user role
	_, err = env.DB.ExecContext(ctx,
		`INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE name = 'user'`,
		userID,
	)
	if err != nil {
		t.Fatalf("Failed to assign user role: %v", err)
	}

	return userID
}

// CreateTestContest creates a test contest in the database and returns the contest ID.
func (env *TestEnv) CreateTestContest(ctx context.Context, t *testing.T, name string, status string) string {
	t.Helper()

	var contestID string
	err := env.DB.QueryRowContext(ctx,
		`INSERT INTO contests (name, starts_at, ends_at, status, qty_total)
		 VALUES ($1, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '1 day', $2, 100000)
		 RETURNING id`,
		name, status,
	).Scan(&contestID)
	if err != nil {
		t.Fatalf("Failed to create test contest: %v", err)
	}

	return contestID
}

// AddContestSymbol adds a symbol to a contest.
func (env *TestEnv) AddContestSymbol(ctx context.Context, t *testing.T, contestID, symbol string) {
	t.Helper()

	_, err := env.DB.ExecContext(ctx,
		`INSERT INTO contest_symbols (contest_id, symbol, enabled) VALUES ($1, $2, TRUE)`,
		contestID, symbol,
	)
	if err != nil {
		t.Fatalf("Failed to add contest symbol: %v", err)
	}
}

// JoinContest adds a user as a participant in a contest.
func (env *TestEnv) JoinContest(ctx context.Context, t *testing.T, contestID, userID string, qtyTotal int64) {
	t.Helper()

	_, err := env.DB.ExecContext(ctx,
		`INSERT INTO contest_participants (contest_id, user_id, qty_total, qty_available)
		 VALUES ($1, $2, $3, $3)`,
		contestID, userID, qtyTotal,
	)
	if err != nil {
		t.Fatalf("Failed to join contest: %v", err)
	}
}

// WaitForCondition waits for a condition to be true with timeout.
func WaitForCondition(ctx context.Context, timeout time.Duration, check func() bool) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if check() {
				return nil
			}
		}
	}
}
