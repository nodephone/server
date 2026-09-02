package database

import (
	"context"
	"database/sql"
	"fmt"
)

// Migration represents a named database migration script.
type Migration struct {
	Name string
	SQL  string
}

// SystemMigrations defines the list of core system migrations for NodePhone.
var SystemMigrations = []Migration{
	{
		Name: "001_initial_schema",
		SQL: `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
`,
	},
	{
		Name: "002_storage_engine",
		SQL: `
CREATE TABLE IF NOT EXISTS buckets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    public BOOLEAN NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS objects (
    id TEXT PRIMARY KEY,
    bucket_id TEXT NOT NULL,
    name TEXT NOT NULL,
    size INTEGER NOT NULL,
    mime_type TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    uploaded_by TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (bucket_id) REFERENCES buckets(id) ON DELETE CASCADE,
    FOREIGN KEY (uploaded_by) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(bucket_id, name)
);
`,
	},
}

// AutoMigrate executes all pending system migrations in order within transactions.
// Tracks applied migrations in the 'migrations' system table to guarantee idempotency.
func (db *DB) AutoMigrate(ctx context.Context) error {
	fmt.Fprintf(db.out, "[DB] Running database migrations...\n")

	// Ensure system tracking table exists
	createMigrationsTableSQL := `
CREATE TABLE IF NOT EXISTS migrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

	if _, err := db.ExecContext(ctx, createMigrationsTableSQL); err != nil {
		return fmt.Errorf("failed to create system migrations tracking table: %w", err)
	}

	for _, m := range SystemMigrations {
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM migrations WHERE name = ?", m.Name).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to query migration status for %q: %w", m.Name, err)
		}

		if count > 0 {
			fmt.Fprintf(db.out, "[DB] Migration already applied: %s\n", m.Name)
			continue
		}

		fmt.Fprintf(db.out, "[DB] Executing migration: %s...\n", m.Name)

		err = db.WithTx(ctx, func(tx *sql.Tx) error {
			if _, execErr := tx.ExecContext(ctx, m.SQL); execErr != nil {
				return fmt.Errorf("failed to execute migration SQL for %q: %w", m.Name, execErr)
			}

			recordSQL := "INSERT INTO migrations (name) VALUES (?)"
			if _, recErr := tx.ExecContext(ctx, recordSQL, m.Name); recErr != nil {
				return fmt.Errorf("failed to record migration %q: %w", m.Name, recErr)
			}
			return nil
		})

		if err != nil {
			return err
		}

		fmt.Fprintf(db.out, "[OK] Migration executed successfully: %s\n", m.Name)
	}

	fmt.Fprintf(db.out, "[OK] All database migrations verified\n")
	return nil
}
