package fxmodules

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func TestModules(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fx Modules")
}

func TestNewFxLogger(t *testing.T) {
	logger, err := NewFxLogger()
	if err != nil {
		t.Fatalf("NewFxLogger() error = %v", err)
	}
	if logger == nil {
		t.Fatal("NewFxLogger() returned nil logger")
	}
	if _, ok := logger.(*fxevent.ZapLogger); !ok {
		t.Fatalf("logger type = %T, want *fxevent.ZapLogger", logger)
	}
}

func TestDeliveryProviders(t *testing.T) {
	if got := configuredDeliveryProviders(config.EmailConfig{}, config.TelegramConfig{}); len(got) != 2 {
		t.Fatalf("delivery providers = %#v", got)
	}
}

type fxPollingSource struct{}

func (fxPollingSource) ListTasks(context.Context, string) ([]task.Task, error) {
	return []task.Task{{
		ExternalID: "task-1",
		CourseID:   "course-1",
		Title:      "Report",
		Status:     task.Discovered,
	}}, nil
}

func (fxPollingSource) ListSubmissions(context.Context, string) ([]submission.Submission, error) {
	return []submission.Submission{{
		ExternalID: "submission-1",
		State:      submission.Submitted,
	}}, nil
}

var _ = Describe("application modules", func() {
	BeforeEach(func() {
		GinkgoT().Setenv("REPORT_RENDERER", "local-xlsx")
		GinkgoT().Setenv("RESEND_API_KEY", "")
		GinkgoT().Setenv("REPORT_TARGET_EMAIL", "")
	})

	It("registers the compiled science report implementation", func() {
		implementation, err := compiledScenarios().Resolve("science-report")
		Expect(err).NotTo(HaveOccurred())
		Expect(implementation).To(Equal(scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}))
	})

	It("creates an error-level Fx logger", func() {
		logger, err := NewFxLogger()
		Expect(err).NotTo(HaveOccurred())
		Expect(logger).NotTo(BeNil())
	})

	It("wires the polling module into an application lifecycle", func(ctx SpecContext) {
		app := fx.New(
			fx.NopLogger,
			fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}),
			fx.Provide(database.NewSQLite, storage.NewClassroomCourseRepository, storage.NewTaskRepository, storage.NewTaskBindingRepository, storage.NewSubmissionRepository, application.NewPollingService),
			PollingModule,
		)
		Expect(app.Start(ctx)).To(Succeed())
		Expect(app.Stop(ctx)).To(Succeed())
	})

	It("invokes the default polling refresh through Fx", func(ctx SpecContext) {
		app := fx.New(
			fx.NopLogger,
			fx.Supply(config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}),
			fx.Provide(database.NewSQLite, storage.NewClassroomCourseRepository, storage.NewTaskRepository, storage.NewTaskBindingRepository, storage.NewSubmissionRepository, application.NewPollingService),
			PollingModule,
			fx.Invoke(func(polling *application.PollingService, lifecycle fx.Lifecycle) {
				lifecycle.Append(fx.Hook{OnStart: func(context.Context) error { return polling.Refresh(context.Background()) }})
			}),
		)
		Expect(app.Start(ctx)).To(Succeed())
		Expect(app.Stop(ctx)).To(Succeed())
	})

	It("wires collection processing into the complete application module", func(ctx SpecContext) {
		var (
			poller     *application.PollingService
			collection *application.ScenarioCollectionService
			runs       ports.ReportRunRepository
			outputs    ports.ReportOutputRepository
			registry   scenario.Registry
			db         *sql.DB
		)
		GinkgoT().Setenv("SQLITE_DATABASE_PATH", filepath.Join(GinkgoT().TempDir(), "state.db"))
		GinkgoT().Setenv("REPORT_RENDERER", "local-xlsx")
		app := fx.New(
			fx.NopLogger,
			PlatformModule, StorageModule, ClassroomStorageModule, ApplicationModule, PollingModule,
			fx.Decorate(func(ports.SubmissionSource) ports.SubmissionSource { return fxPollingSource{} }),
			fx.Populate(&poller, &collection, &runs, &outputs, &registry, &db),
		)
		Expect(app.Start(ctx)).To(Succeed())
		DeferCleanup(app.Stop)
		Expect(poller).NotTo(BeNil())
		Expect(collection).NotTo(BeNil())
		Expect(runs).NotTo(BeNil())
		processor, err := registry.ResolveProcessor("science-report")
		Expect(err).NotTo(HaveOccurred())
		Expect(processor).NotTo(BeNil())

		_, err = db.ExecContext(ctx, `INSERT INTO classroom_courses (id, provider, external_course_id, source_reference, status, created_at, updated_at) VALUES (1, 'google-classroom', 'course-1', 'course-1', 'active', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO scenarios (id, name, implementation_type, status, created_at, updated_at) VALUES (1, 'science-report', 'ScienceReportScenario', 'active', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO scenario_courses (scenario_id, course_id, bound_at, status) VALUES (1, 1, '2026-01-01T00:00:00Z', 'active')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO tasks (id, provider, external_task_id, course_id, title, discovered_at, last_seen_at, status) VALUES (1, 'google-classroom', 'task-1', 'course-1', 'Report', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'bound')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO scenario_tasks (scenario_id, task_id, bound_at, status) VALUES (1, 1, '2026-01-01T00:00:00Z', 'active')`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO submissions (task_id, external_submission_id, state, first_seen_at, last_seen_at) VALUES (1, 'submission-1', 'submitted', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
		Expect(err).NotTo(HaveOccurred())

		Expect(poller.Refresh(ctx)).To(Succeed())
		run, err := runs.Get(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(run.Status).To(Equal(report.Completed))
		Expect(run.ResultPath).NotTo(BeEmpty())
		Expect(outputs).NotTo(BeNil())
		var outputPath string
		Expect(db.QueryRowContext(ctx, `SELECT path FROM report_outputs WHERE report_run_id=?`, run.ID).Scan(&outputPath)).To(Succeed())
		Expect(outputPath).To(Equal(run.ResultPath))
		Expect(outputPath).To(BeAnExistingFile())
	})
})
