package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"time"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

var _ = Describe("report delivery repository", func() {
	It("round trips deliveries", func() {
		ctx := context.Background()
		var db *sql.DB
		var repo ports.ReportDeliveryRepository
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewReportDeliveryRepository), fx.Populate(&db, &repo))
		Expect(app.Start(ctx)).To(Succeed())
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
			_, err := db.ExecContext(ctx, query, args...)
			Expect(err).NotTo(HaveOccurred())
		}
		delivery := &report.Delivery{ResultID: 1, Channel: "email", Destination: "owner@example.com", Status: report.DeliveryPending, CreatedAt: now}
		Expect(repo.Create(ctx, delivery)).To(Succeed())
		Expect(delivery.ID).NotTo(BeZero())
		delivery.Status, delivery.Error, delivery.AttemptedAt = report.DeliverySent, "", &now
		Expect(repo.Update(ctx, delivery)).To(Succeed())
		items, err := repo.GetByResult(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Status).To(Equal(report.DeliverySent))
		item, err := repo.GetByResultTarget(ctx, 1, "email", "owner@example.com")
		Expect(err).NotTo(HaveOccurred())
		Expect(item.ID).To(Equal(delivery.ID))
		_, err = repo.GetByResultTarget(ctx, 1, "email", "missing@example.com")
		Expect(errors.Is(err, ports.ErrDeliveryNotFound)).To(BeTrue())
		Expect(repo.Update(ctx, &report.Delivery{ID: 999, Status: report.DeliveryFailed})).To(HaveOccurred())
	})
})
