package storage_test

import (
	"context"
	"database/sql"
	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	"go.uber.org/fx"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryRepositorySummarizesRuns(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB
	var repo ports.HistoryRepository
	app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewHistoryRepository), fx.Populate(&db, &repo))
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer app.Stop(ctx)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `INSERT INTO scenarios(id,name,implementation_type,status,created_at,updated_at) VALUES(1,'science','Science','disabled',?,?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO tasks(id,provider,external_task_id,course_id,title,discovered_at,last_seen_at,status) VALUES(1,'test','task','course','Report',?,?, 'bound')`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO report_runs(id,scenario_id,task_id,status,is_partial,created_at) VALUES(1,1,1,'failed',1,?)`, now)
	if err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListHistory(ctx)
	if err != nil || len(items) != 1 || !items[0].Partial || items[0].RunStatus != "failed" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestHistoryRepositoryReportsQueryAndTimestampErrors(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB
	var repo ports.HistoryRepository
	app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewHistoryRepository), fx.Populate(&db, &repo))
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer app.Stop(ctx)
	if _, err := db.ExecContext(ctx, `INSERT INTO scenarios(id,name,implementation_type,status,created_at,updated_at) VALUES(1,'science','Science','disabled','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tasks(id,provider,external_task_id,course_id,title,discovered_at,last_seen_at,status) VALUES(1,'test','task','course','Report','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z','bound')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO report_runs(id,scenario_id,task_id,status,is_partial,created_at) VALUES(1,1,1,'failed',0,'bad-time')`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListHistory(ctx); err == nil {
		t.Fatal("malformed history timestamp unexpectedly succeeded")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListHistory(ctx); err == nil {
		t.Fatal("closed history query unexpectedly succeeded")
	}
}
