// Package database opens the SQLite database and applies schema migrations.
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	options := url.Values{}
	options.Add("_pragma", "foreign_keys(1)")
	options.Add("_pragma", "journal_mode(WAL)")
	options.Add("_pragma", "busy_timeout(5000)")
	options.Set("_txlock", "immediate")

	db, err := sql.Open("sqlite", "file:"+path+"?"+options.Encode())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	migrations, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	return migrate(ctx, db, migrations)
}

func migrate(ctx context.Context, db *sql.DB, migrations fs.FS) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		) STRICT`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// fs.ReadDir returns entries sorted by filename, so 0001 runs before 0002.
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()

		var applied bool
		err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = ?)`, name).Scan(&applied)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		statements, err := fs.ReadFile(migrations, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := applyMigration(ctx, db, name, string(statements)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, name, statements string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, statements); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
		return err
	}
	return tx.Commit()
}
