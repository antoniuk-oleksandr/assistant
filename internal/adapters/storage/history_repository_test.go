package storage_test

import (
	"context"
	"database/sql"
	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
	"path/filepath"
	"time"
)

var _ = Describe("history repository", func() {
	It("summarizes runs", func() {
		ctx := context.Background()
		var db *sql.DB
		var repo ports.HistoryRepository
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewHistoryRepository), fx.Populate(&db, &repo))
		Expect(app.Start(ctx)).To(Succeed())
		defer app.Stop(ctx)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := db.ExecContext(ctx, `INSERT INTO scenarios(id,name,implementation_type,status,created_at,updated_at) VALUES(1,'science','Science','disabled',?,?)`, now, now)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO tasks(id,provider,external_task_id,course_id,title,discovered_at,last_seen_at,status) VALUES(1,'test','task','course','Report',?,?, 'bound')`, now, now)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO report_runs(id,scenario_id,task_id,status,is_partial,created_at) VALUES(1,1,1,'failed',1,?)`, now)
		Expect(err).NotTo(HaveOccurred())
		items, err := repo.ListHistory(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Partial).To(BeTrue())
		Expect(items[0].RunStatus).To(Equal("failed"))
	})

	It("reports query and timestamp errors", func() {
		ctx := context.Background()
		var db *sql.DB
		var repo ports.HistoryRepository
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}), fx.Provide(database.NewSQLite, storage.NewHistoryRepository), fx.Populate(&db, &repo))
		Expect(app.Start(ctx)).To(Succeed())
		defer app.Stop(ctx)
		var err error
		_, err = db.ExecContext(ctx, `INSERT INTO scenarios(id,name,implementation_type,status,created_at,updated_at) VALUES(1,'science','Science','disabled','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO tasks(id,provider,external_task_id,course_id,title,discovered_at,last_seen_at,status) VALUES(1,'test','task','course','Report','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z','bound')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO report_runs(id,scenario_id,task_id,status,is_partial,created_at) VALUES(1,1,1,'failed',0,'bad-time')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = repo.ListHistory(ctx)
		Expect(err).To(HaveOccurred())
		Expect(db.Close()).To(Succeed())
		_, err = repo.ListHistory(ctx)
		Expect(err).To(HaveOccurred())
	})
})
