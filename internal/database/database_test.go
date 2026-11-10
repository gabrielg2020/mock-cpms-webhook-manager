package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "nested", "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func openMigratedDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDatabase(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestOpenAppliesConnectionSettings(t *testing.T) {
	db := openTestDatabase(t)
	ctx := context.Background()

	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}

	var busyTimeout int
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}
}

func TestOpenFailsWhenDirectoryCannotBeCreated(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	db, err := Open(context.Background(), blocker)
	if err != nil {
		t.Fatalf("open first database: %v", err)
	}
	_ = db.Close()

	if _, err := Open(context.Background(), filepath.Join(blocker, "inside", "test.db")); err == nil {
		t.Fatal("expected an error when the parent path is a file, got nil")
	}
}

func TestMigrateCreatesSchema(t *testing.T) {
	db := openMigratedDatabase(t)

	wantTables := []string{
		"teams", "users", "environments", "event_types",
		"webhooks", "webhook_events", "audit_log", "sessions", "schema_migrations",
	}
	for _, table := range wantTables {
		if countRows(t, db, "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table) != 1 {
			t.Errorf("table %s was not created", table)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := openMigratedDatabase(t)

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if got := countRows(t, db, "SELECT count(*) FROM schema_migrations"); got != 1 {
		t.Errorf("schema_migrations has %d rows, want 1", got)
	}
}

func TestMigrateRollsBackAFailedMigration(t *testing.T) {
	db := openTestDatabase(t)
	migrations := fstest.MapFS{
		"0001_good.sql": {Data: []byte("CREATE TABLE good (id INTEGER PRIMARY KEY) STRICT;")},
		"0002_bad.sql":  {Data: []byte("CREATE TABLE half_done (id INTEGER PRIMARY KEY) STRICT; CREATE TABLE good (id INTEGER PRIMARY KEY) STRICT;")},
	}

	if err := migrate(context.Background(), db, migrations); err == nil {
		t.Fatal("expected the second migration to fail, got nil")
	}

	if got := countRows(t, db, "SELECT count(*) FROM schema_migrations"); got != 1 {
		t.Errorf("schema_migrations has %d rows, want 1 (only the good migration)", got)
	}
	if got := countRows(t, db, "SELECT count(*) FROM sqlite_master WHERE name = 'half_done'"); got != 0 {
		t.Error("half_done exists, so the failed migration was not rolled back")
	}
}

func TestSchemaEnforcesBusinessRules(t *testing.T) {
	testCases := []struct {
		name      string
		statement string
	}{
		{
			name:      "a member must belong to a team",
			statement: `INSERT INTO users (email, display_name, password_hash, role) VALUES ('m@example.com', 'Member', 'hash', 'member')`,
		},
		{
			name:      "a role must be admin or member",
			statement: `INSERT INTO users (email, display_name, password_hash, role, team_id) VALUES ('s@example.com', 'Super', 'hash', 'superuser', 1)`,
		},
		{
			name:      "email addresses are unique regardless of case",
			statement: `INSERT INTO users (email, display_name, password_hash, role) VALUES ('ADMIN@example.com', 'Copy', 'hash', 'admin')`,
		},
		{
			name:      "a team has one webhook per environment",
			statement: `INSERT INTO webhooks (team_id, environment_id, url, secret_ciphertext, secret_last4) VALUES (1, 1, 'https://example.com/again', x'00', '0000')`,
		},
		{
			name:      "a webhook must belong to an existing team",
			statement: `INSERT INTO webhooks (team_id, environment_id, url, secret_ciphertext, secret_last4) VALUES (999, 1, 'https://example.com/hook', x'00', '0000')`,
		},
		{
			name:      "a team with webhooks cannot be deleted",
			statement: `DELETE FROM teams WHERE id = 1`,
		},
		{
			name:      "audit detail must be valid JSON",
			statement: `INSERT INTO audit_log (action, entity, detail) VALUES ('update', 'webhook', 'not json')`,
		},
		{
			name:      "strict tables reject values of the wrong type",
			statement: `INSERT INTO environments (name, is_production, sort_order) VALUES ('int', 0, 'second')`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			ctx := context.Background()

			fixtures := []string{
				`INSERT INTO teams (id, name) VALUES (1, 'MOT')`,
				`INSERT INTO environments (id, name, is_production, sort_order) VALUES (1, 'dev01', 0, 1)`,
				`INSERT INTO users (email, display_name, password_hash, role) VALUES ('admin@example.com', 'Admin', 'hash', 'admin')`,
				`INSERT INTO webhooks (team_id, environment_id, url, secret_ciphertext, secret_last4) VALUES (1, 1, 'https://example.com/hook', x'00', '0000')`,
			}
			for _, fixture := range fixtures {
				if _, err := db.ExecContext(ctx, fixture); err != nil {
					t.Fatalf("insert fixture: %v", err)
				}
			}

			if _, err := db.ExecContext(ctx, testCase.statement); err == nil {
				t.Error("expected the database to reject the statement, but it was accepted")
			}
		})
	}
}
