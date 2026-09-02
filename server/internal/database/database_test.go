package database_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nodephone/server/internal/database"
)

func TestOpenAndClose(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	var logBuf bytes.Buffer

	db, err := database.Open(dbPath, &logBuf)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if db.Path() != dbPath {
		t.Errorf("expected Path %q, got %q", dbPath, db.Path())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		t.Errorf("Ping failed: %v", err)
	}

	// Verify foreign_keys PRAGMA is enabled
	var fkStatus int
	err = db.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&fkStatus)
	if err != nil {
		t.Fatalf("failed to query foreign_keys PRAGMA: %v", err)
	}
	if fkStatus != 1 {
		t.Errorf("expected PRAGMA foreign_keys = 1, got %d", fkStatus)
	}

	if err := db.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "[OK] SQLite database connection established") {
		t.Errorf("expected connection log in output, got:\n%s", logs)
	}
}

func TestAutoMigrate(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_migrate.db")
	var logBuf bytes.Buffer

	db, err := database.Open(dbPath, &logBuf)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Initial migration run
	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate first run failed: %v", err)
	}

	// Verify the 4 system tables exist
	tables := []string{"migrations", "users", "sessions", "api_keys"}
	for _, table := range tables {
		var name string
		err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?;", table).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist, error: %v", table, err)
		}
	}

	// Verify migration recorded
	var migrationCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(1) FROM migrations WHERE name = '001_initial_schema'").Scan(&migrationCount)
	if err != nil {
		t.Fatalf("failed to query migrations table: %v", err)
	}
	if migrationCount != 1 {
		t.Errorf("expected 1 migration recorded, got %d", migrationCount)
	}

	// Run AutoMigrate a second time (idempotency check)
	logBuf.Reset()
	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate second run failed: %v", err)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "[DB] Migration already applied: 001_initial_schema") {
		t.Errorf("expected log to indicate migration already applied, got:\n%s", logs)
	}
}

func TestForeignKeyEnforcement(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_fk.db")

	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Attempt to insert session for non-existent user_id
	_, err = db.ExecContext(ctx, "INSERT INTO sessions (id, user_id, token, expires_at) VALUES ('sess1', 'nonexistent_user', 'token123', ?);", time.Now().Add(1*time.Hour))
	if err == nil {
		t.Fatal("expected foreign key constraint violation error, got nil")
	}

	// Now insert a valid user and then a session for that user
	_, err = db.ExecContext(ctx, "INSERT INTO users (id, username, email, password_hash) VALUES ('usr1', 'admin', 'admin@example.com', 'hashedpw');")
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}

	_, err = db.ExecContext(ctx, "INSERT INTO sessions (id, user_id, token, expires_at) VALUES ('sess1', 'usr1', 'token123', ?);", time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to insert session for valid user: %v", err)
	}
}

func TestTransactionRollback(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_tx.db")

	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	dummyErr := errors.New("simulated error")
	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		_, execErr := tx.ExecContext(ctx, "INSERT INTO users (id, username, email, password_hash) VALUES ('usr_tx', 'txuser', 'tx@example.com', 'hash');")
		if execErr != nil {
			return execErr
		}
		return dummyErr
	})

	if !errors.Is(err, dummyErr) {
		t.Errorf("expected error %v, got %v", dummyErr, err)
	}

	// Verify user was NOT inserted due to transaction rollback
	var count int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users WHERE id = 'usr_tx'").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 users after rollback, got %d", count)
	}
}
