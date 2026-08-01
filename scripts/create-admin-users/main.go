// Package main creates initial admin and user accounts for the tragge platform.
//
// Usage:
//
//	go run ./scripts/create-admin-users
//
// Environment variables:
//
//	POSTGRES_DSN  - Full PostgreSQL connection string (preferred)
//	POSTGRES_HOST - Database host (default: localhost)
//	POSTGRES_PORT - Database port (default: 5432)
//	POSTGRES_USER - Database user (default: app)
//	POSTGRES_DB   - Database name (default: app)
//
// Secrets (Docker secrets with env fallback):
//
//	POSTGRES_PASSWORD - Database password
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"

	_ "github.com/lib/pq"
)

type seedUser struct {
	Email       string
	Username    string
	DisplayName string
	Password    string
	Roles       []string
}

const tbotUserID = "00000000-0000-0000-0000-000000000001"

func main() {
	users := []seedUser{
		{
			Email:       "admin@tragge.com",
			Username:    "admin",
			DisplayName: "Super Admin",
			Password:    "159032000",
			Roles:       []string{"super_admin", "admin"},
		},
		{
			Email:       "user@tragge.com",
			Username:    "user",
			DisplayName: "Test User",
			Password:    "user123456",
			Roles:       []string{"user"},
		},
	}

	dsn := buildDSN()

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	log.Println("connected to database")

	// Ensure T-bot system account exists before creating regular users
	if err := ensureTBot(ctx, db); err != nil {
		log.Fatalf("failed to ensure T-bot account: %v", err)
	}

	for _, u := range users {
		if err := createUser(ctx, db, u); err != nil {
			log.Fatalf("failed to create user %q: %v", u.Username, err)
		}
	}

	log.Println("all accounts created successfully")
}

func ensureTBot(ctx context.Context, db *sql.DB) error {
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)",
		tbotUserID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check existing T-bot: %w", err)
	}

	if exists {
		// Update to canonical T-bot values
		_, err = db.ExecContext(ctx, `
			UPDATE users
			SET username = 't-bot', display_name = 'T-bot', email = 'tbot@tragge.internal',
			    is_system_account = TRUE, updated_at = NOW()
			WHERE id = $1
		`, tbotUserID)
		if err != nil {
			return fmt.Errorf("update T-bot: %w", err)
		}
		log.Println("T-bot account already exists, updated to canonical values")
		return nil
	}

	// Insert new T-bot account with non-loginable password hash
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, username, display_name, status, is_system_account)
		VALUES ($1, 'tbot@tragge.internal', 'SYSTEM_ACCOUNT_NO_LOGIN', 't-bot', 'T-bot', 'active', TRUE)
	`, tbotUserID)
	if err != nil {
		return fmt.Errorf("insert T-bot: %w", err)
	}

	log.Println("created T-bot system account")
	return nil
}

func createUser(ctx context.Context, db *sql.DB, u seedUser) error {
	// Check if user already exists
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 OR username = $2)",
		u.Email, u.Username,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check existing user: %w", err)
	}
	if exists {
		log.Printf("user %q already exists, skipping", u.Username)
		return nil
	}

	// Hash password using the same Argon2id parameters as the platform
	hash, err := auth.HashPassword(u.Password, nil)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert user
	var userID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, username, display_name, status, email_verified, email_verified_at, terms_accepted_at)
		VALUES ($1, $2, $3, $4, 'active', TRUE, NOW(), NOW())
		RETURNING id
	`, u.Email, hash, u.Username, u.DisplayName).Scan(&userID)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	// Assign roles
	for _, role := range u.Roles {
		var roleID int
		err = tx.QueryRowContext(ctx,
			"SELECT id FROM roles WHERE name = $1", role,
		).Scan(&roleID)
		if err != nil {
			return fmt.Errorf("find role %q: %w", role, err)
		}

		_, err = tx.ExecContext(ctx,
			"INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)",
			userID, roleID,
		)
		if err != nil {
			return fmt.Errorf("assign role %q: %w", role, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	log.Printf("created user %q (id=%s) with roles %v", u.Username, userID, u.Roles)
	return nil
}

func buildDSN() string {
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		return dsn
	}

	host := envOrDefault("POSTGRES_HOST", "localhost")
	port := envOrDefault("POSTGRES_PORT", "5432")
	user := envOrDefault("POSTGRES_USER", "app")
	dbName := envOrDefault("POSTGRES_DB", "app")
	sslMode := envOrDefault("POSTGRES_SSLMODE", "disable")

	// Enforce SSL in non-development environments (matches packages/secrets behavior)
	env := os.Getenv("ENVIRONMENT")
	if sslMode == "disable" && env != "" && env != "development" && env != "local" && env != "test" {
		fmt.Fprintf(os.Stderr, "WARNING: POSTGRES_SSLMODE=disable in %s environment, forcing require\n", env)
		sslMode = "require"
	}

	password := secrets.Load("POSTGRES_PASSWORD")

	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbName, sslMode,
	)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
