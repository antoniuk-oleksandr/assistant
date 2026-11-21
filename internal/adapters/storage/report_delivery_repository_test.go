package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	"go.uber.org/fx"
)

func TestReportDeliveryRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB
	var repo ports.ReportDeliveryRepository
	app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewReportDeliveryRepository), fx.Populate(&db, &repo))
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer app.Stop(ctx)
	now := time.Now().UTC()
	for _, query := range []string{
		`INSERT INTO scenarios(id,name,implementation_type,status,created_at,updated_at) VALUES(1,'science-report','ScienceReportScenario','active',?,?)`,
		`INSERT INTO tasks(id,provider,external_task_id,course_id,title,discovered_at,last_seen_at,status) VALUES(1,'google-classroom','task-1','course-1','Report',?,?, 'bound')`,
		`INSERT INTO report_runs(id,scenario_id,task_id,status,is_partial,created_at) VALUES(1,1,1,'completed',0,?)`,
		`INSERT INTO report_results(id,report_run_id,scenario_id,task_id,version,status,is_partial,result_json,created_at,updated_at) VALUES(1,1,1,1,1,'completed',0,'{}',?,?)`,
	} {
		args := []any{now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)}
		if query == `INSERT INTO report_runs(id,scenario_id,task_id,status,is_partial,created_at) VALUES(1,1,1,'completed',0,?)` {
			args = []any{now.Format(time.RFC3339Nano)}
		}
		if query == `INSERT INTO report_results(id,report_run_id,scenario_id,task_id,version,status,is_partial,result_json,created_at,updated_at) VALUES(1,1,1,1,1,'completed',0,'{}',?,?)` {
			args = []any{now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)}
		}
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	delivery := &report.Delivery{ResultID: 1, Channel: "email", Destination: "owner@example.com", Status: report.DeliveryPending, CreatedAt: now}
	if err := repo.Create(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	if delivery.ID == 0 {
		t.Fatal("delivery ID was not assigned")
	}
	delivery.Status, delivery.Error, delivery.AttemptedAt = report.DeliverySent, "", &now
	if err := repo.Update(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	items, err := repo.GetByResult(ctx, 1)
	if err != nil || len(items) != 1 || items[0].Status != report.DeliverySent {
		t.Fatalf("items = %#v, err = %v", items, err)
	}
	item, err := repo.GetByResultTarget(ctx, 1, "email", "owner@example.com")
	if err != nil || item.ID != delivery.ID {
		t.Fatalf("target = %#v, err = %v", item, err)
	}
	if _, err := repo.GetByResultTarget(ctx, 1, "email", "missing@example.com"); !errors.Is(err, ports.ErrDeliveryNotFound) {
		t.Fatalf("missing error = %v", err)
	}
	if err := repo.Update(ctx, &report.Delivery{ID: 999, Status: report.DeliveryFailed}); err == nil {
		t.Fatal("unknown delivery update succeeded")
	}
}
