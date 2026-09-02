// Package database provides SQLite database connection management, schema migrations,
// transaction execution, and query handling for the NodePhone server.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a standard sql.DB connection pool with NodePhone specific logging and helper functionality.
type DB struct {
	sqlDB *sql.DB
	path  string
	out   io.Writer
}

// Open initializes and verifies a connection to the SQLite database at dbPath.
// Enables foreign key enforcement and WAL journal mode.
func Open(dbPath string, out io.Writer) (*DB, error) {
	if out == nil {
		out = io.Discard
	}

	fmt.Fprintf(out, "[DB] Connecting to SQLite database at %s...\n", dbPath)

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %q: %w", dbPath, err)
	}

	// SQLite operates best with a single write connection pool to avoid lock contention
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database at %q: %w", dbPath, err)
	}

	// Enable foreign key constraints
	if _, err := sqlDB.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to enable foreign keys PRAGMA: %w", err)
	}

	// Enable Write-Ahead Logging (WAL) for concurrency
	if _, err := sqlDB.ExecContext(ctx, "PRAGMA journal_mode = WAL;"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to enable WAL journal mode: %w", err)
	}

	fmt.Fprintf(out, "[OK] SQLite database connection established (foreign_keys=ON, WAL=ON)\n")

	return &DB{
		sqlDB: sqlDB,
		path:  dbPath,
		out:   out,
	}, nil
}

// Path returns the filesystem path of the database.
func (db *DB) Path() string {
	return db.path
}

// SQLDB returns the underlying *sql.DB instance.
func (db *DB) SQLDB() *sql.DB {
	return db.sqlDB
}

// Close gracefully closes the database connection.
func (db *DB) Close() error {
	if db.sqlDB == nil {
		return nil
	}
	fmt.Fprintf(db.out, "[DB] Closing SQLite database connection...\n")
	err := db.sqlDB.Close()
	if err == nil {
		fmt.Fprintf(db.out, "[OK] SQLite database connection closed\n")
	}
	return err
}

// Ping verifies connection to the database.
func (db *DB) Ping(ctx context.Context) error {
	return db.sqlDB.PingContext(ctx)
}

// ExecContext executes a query without returning any rows.
func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.sqlDB.ExecContext(ctx, query, args...)
}

// QueryContext executes a query that returns rows.
func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return db.sqlDB.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that is expected to return at most one row.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.sqlDB.QueryRowContext(ctx, query, args...)
}

// WithTx executes a function within a database transaction.
// Automatically commits on success or rolls back on panic / error.
func (db *DB) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-throw panic after rollback
		} else if err != nil {
			_ = tx.Rollback()
		} else {
			if commitErr := tx.Commit(); commitErr != nil {
				err = fmt.Errorf("failed to commit transaction: %w", commitErr)
			}
		}
	}()

	err = fn(tx)
	return err
}
