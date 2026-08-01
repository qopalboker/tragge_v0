// Package main is a one-shot admin password reset utility intended for
// emergency recovery when an admin account is locked out of the /admin/login
// flow and no self-service recovery path exists.
//
// It hashes the new password using the EXACT Argon2id routine in
// packages/auth (the same auth.HashPassword called by registration,
// forgot-password, and the admin seed at apps/user-bff/server/helpers.go),
// writes users.password_hash in a transaction alongside password_changed_at
// and updated_at, and inserts an audit_logs row tagged
// "user.password.reset.manual" so the event is traceable.
//
// IMPORTANT: this path writes the hash directly, bypassing the password
// policy enforced by packages/validation.ValidatePassword (uppercase +
// lowercase + digit + special, min 10 chars). The CLI prints a reminder
// that the operator MUST rotate via the admin UI on next login.
//
// Usage (run from repo root):
//
//	docker run --rm -it \
//	  --network platform_net \
//	  -v "$PWD":/workspace:ro \
//	  -v "$PWD/infra/docker/secrets/postgres_app_password.txt":/run/secrets/pg_app:ro \
//	  -w /workspace/tools/admin-password-reset \
//	  -e POSTGRES_HOST=postgres \
//	  -e POSTGRES_USER=tragge_app \
//	  -e POSTGRES_DB=app \
//	  -e POSTGRES_PASSWORD_FILE=/run/secrets/pg_app \
//	  golang:1.24-alpine \
//	  go run . -email admin@tragge.com
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"golang.org/x/term"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

func main() {
	var (
		email  string
		reason string
	)
	flag.StringVar(&email, "email", "", "target user's email (required)")
	flag.StringVar(&reason, "reason",
		"manual reset via bootstrap script — account locked out",
		"audit_log reason text")
	flag.Parse()

	if email == "" {
		die("usage: -email <email> is required")
	}

	// Use /dev/tty directly: stdin may be piped, and we want a real interactive
	// prompt that echoes nothing.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		die("open /dev/tty (run container with -it so a TTY is attached): %v", err)
	}
	defer tty.Close()

	fmt.Fprintf(tty, "New password for %s (not echoed): ", email)
	pw1, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(tty)
	if err != nil {
		die("read password: %v", err)
	}
	fmt.Fprint(tty, "Repeat: ")
	pw2, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(tty)
	if err != nil {
		die("read password: %v", err)
	}
	if !constantEq(pw1, pw2) {
		die("passwords do not match")
	}
	if len(pw1) == 0 {
		die("empty password")
	}
	if len(pw1) < 10 {
		fmt.Fprintln(tty,
			"WARNING: password <10 chars. DefaultPasswordConstraints "+
				"(packages/validation/validation.go:419) is being bypassed.")
	}

	hash, err := auth.HashPassword(string(pw1), nil)
	if err != nil {
		die("hash password: %v", err)
	}
	// Clear plaintext from process memory as soon as we have the hash.
	for i := range pw1 {
		pw1[i] = 0
	}
	for i := range pw2 {
		pw2[i] = 0
	}

	dsn, err := buildDSN()
	if err != nil {
		die("build DSN: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		die("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		die("db ping: %v", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		die("begin tx: %v", err)
	}
	defer tx.Rollback()

	var userID string
	var isSystem bool
	err = tx.QueryRowContext(ctx,
		`SELECT id, COALESCE(is_system_account, FALSE)
		   FROM users WHERE email = $1`,
		email,
	).Scan(&userID, &isSystem)
	if errors.Is(err, sql.ErrNoRows) {
		die("no user with email %q", email)
	} else if err != nil {
		die("query user: %v", err)
	}
	if isSystem {
		die("refusing to reset: %q is a system account", email)
	}

	roles, err := queryRoles(ctx, tx, userID)
	if err != nil {
		die("query roles: %v", err)
	}

	fmt.Fprintf(tty, "\nAbout to reset password for %s\n  id:    %s\n  roles: %v\n",
		email, userID, roles)
	fmt.Fprint(tty, "Type RESET to confirm: ")
	var confirm string
	if _, err := fmt.Fscanln(tty, &confirm); err != nil || confirm != "RESET" {
		die("not confirmed; rolled back")
	}

	// trigger_set_updated_at (packages/db/migrations/0001_init.up.sql:271) already
	// maintains updated_at, but set it explicitly to survive if the trigger is
	// ever dropped.
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		   SET password_hash       = $1,
		       password_changed_at = NOW(),
		       updated_at          = NOW()
		 WHERE id = $2`,
		hash, userID,
	)
	if err != nil {
		die("update users: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		die("expected 1 row updated, got %d", n)
	}

	payload := map[string]any{
		"email":  email,
		"roles":  roles,
		"reason": reason,
		"tool":   "tools/admin-password-reset",
		"note":   "policy bypassed; operator MUST rotate via admin UI on next login",
	}
	payloadJSON, _ := json.Marshal(payload)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs
		  (actor_user_id, action, target_type, target_id, payload_json)
		VALUES
		  (NULL, 'user.password.reset.manual', 'user', $1, $2)`,
		userID, string(payloadJSON),
	)
	if err != nil {
		die("insert audit_log: %v", err)
	}

	if err := tx.Commit(); err != nil {
		die("commit: %v", err)
	}

	fmt.Fprintf(tty, "\nPassword reset for %s (user_id=%s).\n", email, userID)
	fmt.Fprintln(tty, "Audit event: user.password.reset.manual")
	fmt.Fprintln(tty, "REQUIRED follow-up: log in via /admin/login and rotate the")
	fmt.Fprintln(tty, "password immediately — this path bypassed the app's password")
	fmt.Fprintln(tty, "policy validation and is not acceptable as a long-lived credential.")
}

func queryRoles(ctx context.Context, tx *sql.Tx, userID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT r.name
		  FROM user_roles ur
		  JOIN roles r ON r.id = ur.role_id
		 WHERE ur.user_id = $1
		 ORDER BY r.name`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func buildDSN() (string, error) {
	host := getenvOr("POSTGRES_HOST", "localhost")
	port := getenvOr("POSTGRES_PORT", "5432")
	user := getenvOr("POSTGRES_USER", "tragge_app")
	dbname := getenvOr("POSTGRES_DB", "app")
	sslmode := getenvOr("POSTGRES_SSLMODE", "disable")

	pw := os.Getenv("POSTGRES_PASSWORD")
	if pw == "" {
		if pf := os.Getenv("POSTGRES_PASSWORD_FILE"); pf != "" {
			b, err := os.ReadFile(pf)
			if err != nil {
				return "", fmt.Errorf("read password file %s: %w", pf, err)
			}
			pw = trimTrailingWhitespace(string(b))
		}
	}
	if pw == "" {
		return "", fmt.Errorf("POSTGRES_PASSWORD or POSTGRES_PASSWORD_FILE required")
	}

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, pw, dbname, sslmode), nil
}

func getenvOr(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func trimTrailingWhitespace(s string) string {
	for len(s) > 0 {
		c := s[len(s)-1]
		if c == '\n' || c == '\r' || c == ' ' || c == '\t' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}

func constantEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var d byte
	for i := range a {
		d |= a[i] ^ b[i]
	}
	return d == 0
}

func die(format string, a ...any) {
	fmt.Fprintln(os.Stderr, "error: "+fmt.Sprintf(format, a...))
	os.Exit(1)
}
