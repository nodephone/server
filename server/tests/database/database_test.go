package database_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nodephone/server/internal/database"
)

func TestDatabaseEngineAndMigrations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "db_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test_main.db")
	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Execute AutoMigrate
	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// 2. Verify System Tables Exist
	tables := []string{"users", "sessions", "api_keys", "buckets", "objects", "policies", "migrations"}
	for _, tbl := range tables {
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&count)
		if err != nil || count == 0 {
			t.Errorf("expected table %q to exist in database, count=%d, err=%v", tbl, count, err)
		}
	}

	// 3. Test Foreign Key Enforcement
	// Inserting a session with non-existent user_id should fail
	_, err = db.ExecContext(ctx, "INSERT INTO sessions (id, user_id, token, expires_at) VALUES ('s1', 'non_existent_user', 'tok1', CURRENT_TIMESTAMP)")
	if err == nil {
		t.Error("expected foreign key constraint failure when inserting invalid user_id into sessions")
	}

	// 4. Test Transaction Rollback
	txErr := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, execErr := tx.ExecContext(ctx, "INSERT INTO users (id, username, email, password_hash) VALUES ('u1', 'txuser', 'tx@example.com', 'hash')")
		if execErr != nil {
			return execErr
		}
		// Intentionally return error to trigger rollback
		return sql.ErrTxDone
	})
	if txErr == nil {
		t.Error("expected WithTx to return error")
	}

	// Verify user was NOT inserted due to rollback
	var userCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users WHERE id = 'u1'").Scan(&userCount)
	if userCount != 0 {
		t.Errorf("expected rolled back transaction user count to be 0, got %d", userCount)
	}
}
