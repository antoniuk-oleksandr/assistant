package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestFileDSNRejectsDirectoryCreationBelowAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := fileDSN(filepath.Join(file, "state.db"))
	if err == nil {
		t.Fatal("fileDSN returned nil error")
	}
}

func TestMigrateTaskColumnsAddsLifecycleMetadataToExistingDatabase(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE tasks (id INTEGER PRIMARY KEY, created_at_external TEXT, due_at TEXT, discovered_at TEXT NOT NULL, last_seen_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := migrateTaskColumns(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"updated_at_external", "due_date", "due_time", "time_zone", "phase", "phase_at"} {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pragma_table_info('tasks') WHERE name = ?)`, column).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("column %q was not added", column)
		}
	}
}

func TestMigrationsAddMissingColumnsAndRemainIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE report_runs (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE reminders (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE tasks (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE materials (id INTEGER PRIMARY KEY)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	for _, migrate := range []func(context.Context, *sql.DB) error{
		migrateReportRunColumns,
		migrateReminderColumns,
		migrateTaskColumns,
		migrateMaterialColumns,
	} {
		if err := migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
		if err := migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	for table, columns := range map[string][]string{
		"report_runs": {"input_fingerprint"},
		"reminders":   {"subject", "message", "due_at", "classroom_link"},
		"tasks":       {"updated_at_external", "due_date", "due_time", "time_zone", "phase", "phase_at"},
		"materials":   {"size_bytes", "parse_result"},
	} {
		for _, column := range columns {
			var exists bool
			if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pragma_table_info(?) WHERE name = ?)`, table, column).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if !exists {
				t.Errorf("column %s.%s was not added", table, column)
			}
		}
	}
}
