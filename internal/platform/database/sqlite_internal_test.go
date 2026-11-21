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
	for _, column := range []string{"updated_at_external", "due_date", "due_time", "time_zone"} {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pragma_table_info('tasks') WHERE name = ?)`, column).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("column %q was not added", column)
		}
	}
}
