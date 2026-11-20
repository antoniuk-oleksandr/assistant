package storage_test

import (
	"database/sql"
	"path/filepath"
	"time"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

var _ = Describe("SQLite report run repository", func() {
	var (
		db         *sql.DB
		repo       ports.ReportRunRepository
		outputs    ports.ReportOutputRepository
		results    ports.ReportResultRepository
		deliveries ports.ReportDeliveryRepository
		reminders  ports.ReminderRepository
	)

	BeforeEach(func(ctx SpecContext) {
		app := fx.New(
			fx.NopLogger,
			fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}),
			fx.Provide(database.NewSQLite, storage.NewReportRunRepository, storage.NewReportOutputRepository, storage.NewReportResultRepository, storage.NewReportDeliveryRepository, storage.NewReminderRepository),
			fx.Populate(&db, &repo, &outputs, &results, &deliveries, &reminders),
		)
		Expect(app.Start(ctx)).To(Succeed())
		DeferCleanup(app.Stop)
		_, err := db.ExecContext(ctx, `INSERT INTO scenarios (name, implementation_type, status, created_at, updated_at) VALUES ('science-report', 'ScienceReportScenario', 'active', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO tasks (provider, external_task_id, course_id, title, discovered_at, last_seen_at, status) VALUES ('google-classroom', 'task-1', 'course-1', 'Report', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'bound')`)
		Expect(err).NotTo(HaveOccurred())
	})

	It("creates, updates, and reads a report run with nullable fields", func(ctx SpecContext) {
		started := time.Date(2026, 11, 20, 10, 0, 0, 123000000, time.FixedZone("local", 2*60*60)).UTC()
		run := report.NewRun(1, 1, started)
		Expect(repo.Create(ctx, &run)).To(Succeed())
		Expect(run.ID).To(BeNumerically(">", 0))
		Expect(repo.Get(ctx, run.ID)).To(Equal(run))

		finished := started.Add(time.Hour)
		run.Status = report.Completed
		run.StartedAt = &started
		run.FinishedAt = &finished
		run.Partial = true
		run.ResultPath = "result.json"
		run.ErrorMessage = "partial"
		Expect(repo.Update(ctx, &run)).To(Succeed())
		stored, err := repo.Get(ctx, run.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(Equal(run))
		lookup, ok := repo.(ports.ScenarioTaskReportRunRepository)
		Expect(ok).To(BeTrue())
		found, err := lookup.GetByScenarioTask(ctx, 1, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(Equal(run))
	})

	It("enforces one active run while allowing a terminal retry", func(ctx SpecContext) {
		first := report.NewRun(1, 1, time.Now())
		Expect(repo.Create(ctx, &first)).To(Succeed())
		second := report.NewRun(1, 1, time.Now())
		Expect(repo.Create(ctx, &second)).To(MatchError(ContainSubstring("UNIQUE constraint failed")))
		first.Status = report.Failed
		Expect(repo.Update(ctx, &first)).To(Succeed())
		Expect(repo.Create(ctx, &second)).To(Succeed())
	})

	It("reports missing and unavailable runs", func(ctx SpecContext) {
		_, err := repo.Get(ctx, 999)
		Expect(err).To(MatchError(ContainSubstring("get report run 999")))
		lookup, ok := repo.(ports.ScenarioTaskReportRunRepository)
		Expect(ok).To(BeTrue())
		_, err = lookup.GetByScenarioTask(ctx, 1, 999)
		Expect(err).To(MatchError(ContainSubstring("report run not found")))
		missing := report.Run{ID: 999, Status: report.Completed}
		Expect(repo.Update(ctx, &missing)).To(MatchError("report run 999 not found"))
		Expect(db.Close()).To(Succeed())
		Expect(repo.Create(ctx, &report.Run{ScenarioID: 1, TaskID: 1, Status: report.Scheduled, CreatedAt: time.Now()})).To(MatchError(ContainSubstring("insert report run")))
	})

	It("rejects malformed stored report timestamps", func(ctx SpecContext) {
		run := report.NewRun(1, 1, time.Now())
		Expect(repo.Create(ctx, &run)).To(Succeed())
		Expect(db.ExecContext(ctx, `UPDATE report_runs SET started_at='not-a-time' WHERE id=?`, run.ID)).Error().NotTo(HaveOccurred())
		_, err := repo.Get(ctx, run.ID)
		Expect(err).To(MatchError(ContainSubstring("cannot parse")))
		Expect(db.ExecContext(ctx, `UPDATE report_runs SET started_at=NULL, created_at='not-a-time' WHERE id=?`, run.ID)).Error().NotTo(HaveOccurred())
		_, err = repo.Get(ctx, run.ID)
		Expect(err).To(MatchError(ContainSubstring("cannot parse")))
		Expect(db.ExecContext(ctx, `UPDATE report_runs SET created_at='2026-01-01T00:00:00Z', finished_at='not-a-time' WHERE id=?`, run.ID)).Error().NotTo(HaveOccurred())
		_, err = repo.Get(ctx, run.ID)
		Expect(err).To(MatchError(ContainSubstring("cannot parse")))
	})

	It("returns update and read errors after the database closes", func(ctx SpecContext) {
		run := report.NewRun(1, 1, time.Now())
		Expect(repo.Create(ctx, &run)).To(Succeed())
		Expect(db.Close()).To(Succeed())
		Expect(repo.Update(ctx, &run)).To(MatchError(ContainSubstring("update report run")))
		_, err := repo.Get(ctx, run.ID)
		Expect(err).To(MatchError(ContainSubstring("get report run")))
	})

	It("persists report output metadata and defaults a non-positive version", func(ctx SpecContext) {
		run := report.NewRun(1, 1, time.Now())
		Expect(repo.Create(ctx, &run)).To(Succeed())
		createdAt := time.Date(2026, 11, 20, 12, 0, 0, 0, time.FixedZone("local", 2*60*60))
		Expect(outputs.Create(ctx, ports.ReportOutput{
			ReportRunID: run.ID,
			Format:      "local-package",
			Path:        "/tmp/report.json",
			CreatedAt:   createdAt,
		})).To(Succeed())

		var format, path, storedCreatedAt string
		var version int
		Expect(db.QueryRowContext(ctx, `SELECT format, path, version, created_at FROM report_outputs WHERE report_run_id=?`, run.ID).
			Scan(&format, &path, &version, &storedCreatedAt)).To(Succeed())
		Expect(format).To(Equal("local-package"))
		Expect(path).To(Equal("/tmp/report.json"))
		Expect(version).To(Equal(1))
		Expect(storedCreatedAt).To(Equal(createdAt.UTC().Format(time.RFC3339Nano)))
	})

	It("reports output persistence errors", func(ctx SpecContext) {
		run := report.NewRun(1, 1, time.Now())
		Expect(repo.Create(ctx, &run)).To(Succeed())
		Expect(db.Close()).To(Succeed())
		Expect(outputs.Create(ctx, ports.ReportOutput{ReportRunID: run.ID, Format: "local-package", Path: "report.json", CreatedAt: time.Now()})).To(MatchError(ContainSubstring("insert report output")))
	})

	It("persists versioned structured results and processing status", func(ctx SpecContext) {
		run := report.NewRun(1, 1, time.Now())
		Expect(repo.Create(ctx, &run)).To(Succeed())
		value := report.NewResult(run.ID, 1, 1, scenario.ReportData{ScenarioID: 1, TaskID: 1, MissingSubmissions: []string{"staff-2"}}, time.Now())
		Expect(results.Create(ctx, &value)).To(Succeed())
		stored, err := results.GetLatestByRun(ctx, run.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.ID).To(Equal(value.ID))
		Expect(stored.Version).To(Equal(1))
		Expect(stored.Status).To(Equal(report.ResultProcessing))
		Expect(stored.Data.MissingSubmissions).To(Equal([]string{"staff-2"}))
		stored.Complete(report.ResultPartial, time.Now(), "partial")
		Expect(results.Update(ctx, &stored)).To(Succeed())
		updated, err := results.GetLatestByRun(ctx, run.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status).To(Equal(report.ResultPartial))
		Expect(updated.Partial).To(BeTrue())
		Expect(updated.ErrorMessage).To(Equal("partial"))
	})

	It("reports result lookup and persistence errors", func(ctx SpecContext) {
		_, err := results.GetLatestByRun(ctx, 999)
		Expect(err).To(MatchError(ContainSubstring("report result not found")))
		Expect(db.Close()).To(Succeed())
		value := report.NewResult(1, 1, 1, scenario.ReportData{}, time.Now())
		Expect(results.Create(ctx, &value)).To(MatchError(ContainSubstring("insert report result")))
	})

	It("persists delivery status and lists deliveries by result", func(ctx SpecContext) {
		run := report.NewRun(1, 1, time.Now())
		Expect(repo.Create(ctx, &run)).To(Succeed())
		value := report.NewResult(run.ID, 1, 1, scenario.ReportData{}, time.Now())
		Expect(results.Create(ctx, &value)).To(Succeed())
		now := time.Now().UTC()
		delivery := report.Delivery{ResultID: value.ID, Channel: "email", Destination: "teacher@example.com", Status: report.DeliverySent, AttemptedAt: &now, CreatedAt: now}
		Expect(deliveries.Create(ctx, &delivery)).To(Succeed())
		items, err := deliveries.GetByResult(ctx, value.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Status).To(Equal(report.DeliverySent))
		found, err := deliveries.GetByResultTarget(ctx, value.ID, "email", "teacher@example.com")
		Expect(err).NotTo(HaveOccurred())
		Expect(found.ID).To(Equal(delivery.ID))
		delivery.Status = report.DeliveryFailed
		delivery.Error = "unavailable"
		Expect(deliveries.Update(ctx, &delivery)).To(Succeed())
	})

	It("reports delivery repository errors after database close", func(ctx SpecContext) {
		Expect(db.Close()).To(Succeed())
		value := report.Delivery{ResultID: 1, Channel: "email", Destination: "teacher@example.com", Status: report.DeliveryPending, CreatedAt: time.Now()}
		Expect(deliveries.Create(ctx, &value)).To(MatchError(ContainSubstring("insert report delivery")))
		Expect(deliveries.Update(ctx, &value)).To(MatchError(ContainSubstring("update report delivery")))
		_, err := deliveries.GetByResult(ctx, 1)
		Expect(err).To(MatchError(ContainSubstring("list report deliveries")))
		_, err = deliveries.GetByResultTarget(ctx, 1, "email", "teacher@example.com")
		Expect(err).To(MatchError(ContainSubstring("find report delivery")))
	})

	It("persists, updates, and deduplicates missing-submission reminders", func(ctx SpecContext) {
		item := notification.Reminder{TaskID: 1, PersonID: "person-1", Channel: "email", Destination: "person@example.com", Kind: "missing_submission", Status: notification.Sent, CreatedAt: time.Now()}
		Expect(reminders.Create(ctx, &item)).To(Succeed())
		found, err := reminders.Get(ctx, 1, "person-1", "email", "missing_submission")
		Expect(err).NotTo(HaveOccurred())
		Expect(found.ID).To(Equal(item.ID))
		Expect(reminders.Create(ctx, &notification.Reminder{TaskID: 1, PersonID: "person-1", Channel: "email", Destination: "person@example.com", Kind: "missing_submission", Status: notification.Pending, CreatedAt: time.Now()})).To(MatchError(ContainSubstring("UNIQUE constraint failed")))
		found.Status = notification.Failed
		found.Error = "unavailable"
		Expect(reminders.Update(ctx, &found)).To(Succeed())
		items, err := reminders.ListByTask(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Status).To(Equal(notification.Failed))
	})

	It("reports reminder repository errors after database close", func(ctx SpecContext) {
		Expect(db.Close()).To(Succeed())
		item := notification.Reminder{TaskID: 1, PersonID: "person-1", Channel: "email", Destination: "person@example.com", Kind: "missing_submission", Status: notification.Pending, CreatedAt: time.Now()}
		Expect(reminders.Update(ctx, &item)).To(MatchError(ContainSubstring("update reminder")))
		_, err := reminders.Get(ctx, 1, "person-1", "email", "missing_submission")
		Expect(err).To(MatchError(ContainSubstring("find reminder")))
		_, err = reminders.ListByTask(ctx, 1)
		Expect(err).To(MatchError(ContainSubstring("list reminders")))
	})

	It("rejects malformed reminder timestamps", func(ctx SpecContext) {
		_, err := db.ExecContext(ctx, `INSERT INTO reminders (task_id, person_id, channel, destination, kind, status, attempted_at, created_at) VALUES (1, 'person-2', 'email', 'bad@example.com', 'missing_submission', 'pending', 'bad-time', '2026-01-01T00:00:00Z')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = reminders.ListByTask(ctx, 1)
		Expect(err).To(MatchError(ContainSubstring("cannot parse")))
	})

	It("reports missing reminder and update targets", func(ctx SpecContext) {
		_, err := reminders.Get(ctx, 1, "unknown", "email", "missing_submission")
		Expect(err).To(MatchError(ContainSubstring("reminder not found")))
		item := notification.Reminder{ID: 999, Status: notification.Sent}
		Expect(reminders.Update(ctx, &item)).To(MatchError("reminder 999 not found"))
	})

	It("reports missing delivery targets and malformed delivery timestamps", func(ctx SpecContext) {
		run := report.NewRun(1, 1, time.Now())
		Expect(repo.Create(ctx, &run)).To(Succeed())
		value := report.NewResult(run.ID, 1, 1, scenario.ReportData{}, time.Now())
		Expect(results.Create(ctx, &value)).To(Succeed())
		_, err := deliveries.GetByResultTarget(ctx, value.ID, "email", "missing@example.com")
		Expect(err).To(MatchError(ContainSubstring("report delivery not found")))
		_, err = db.ExecContext(ctx, `INSERT INTO report_deliveries (result_id, channel, destination, status, attempted_at, created_at) VALUES (?, 'email', 'bad@example.com', 'pending', 'not-a-time', '2026-01-01T00:00:00Z')`, value.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = deliveries.GetByResult(ctx, value.ID)
		Expect(err).To(MatchError(ContainSubstring("cannot parse")))
	})
})
