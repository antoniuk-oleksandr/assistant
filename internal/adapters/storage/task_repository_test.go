package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

var _ = Describe("SQLite task bindings", func() {
	var db *sql.DB
	var tasks ports.TaskRepository
	var bindings ports.TaskBindingRepository
	var scenarios ports.ScenarioRepository

	BeforeEach(func(ctx SpecContext) {
		cfg := config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}
		app := fx.New(fx.NopLogger, fx.Supply(cfg), fx.Provide(database.NewSQLite, storage.NewTaskRepository, storage.NewTaskBindingRepository, storage.NewScenarioRepository), fx.Populate(&db, &tasks, &bindings, &scenarios))
		Expect(app.Start(ctx)).To(Succeed())
		DeferCleanup(app.Stop)
	})

	It("registers assignments idempotently and only returns active bindings", func(ctx SpecContext) {
		implementation := scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}
		Expect(scenarios.Create(ctx, implementation)).To(Succeed())
		assignment := task.Task{Provider: "google-classroom", ExternalID: "coursework-1", CourseID: "course-1", Title: "Report"}
		Expect(tasks.Register(ctx, assignment)).To(Succeed())
		assignment.Title = "Updated report"
		Expect(tasks.Register(ctx, assignment)).To(Succeed())
		items, err := tasks.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Title).To(Equal("Updated report"))

		Expect(bindings.Bind(ctx, implementation.Name, assignment.ExternalID)).To(Succeed())
		Expect(bindings.Bind(ctx, implementation.Name, assignment.ExternalID)).To(Succeed())
		active, err := bindings.Active(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(HaveLen(1))
		Expect(active[0].Task.ExternalID).To(Equal(assignment.ExternalID))

		Expect(scenarios.SetStatus(ctx, implementation.Name, scenario.Disabled)).To(Succeed())
		active, err = bindings.Active(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(BeEmpty())

		listed, err := bindings.List(ctx, implementation.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(listed).To(HaveLen(1))
		Expect(listed[0].Status).To(Equal("active"))

		Expect(scenarios.SetStatus(ctx, implementation.Name, scenario.Active)).To(Succeed())
		Expect(bindings.Unbind(ctx, assignment.ExternalID)).To(Succeed())
		Expect(bindings.Active(ctx)).Error().NotTo(HaveOccurred())
		Expect(bindings.Unbind(ctx, assignment.ExternalID)).To(Succeed())
		listed, err = bindings.List(ctx, implementation.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(listed).To(BeEmpty())
	})

	It("keeps tasks and active bindings across a database lifecycle", func(ctx SpecContext) {
		path := filepath.Join(GinkgoT().TempDir(), "restart.db")
		start := func() (*fx.App, ports.TaskRepository, ports.TaskBindingRepository, ports.ScenarioRepository) {
			cfg := config.DatabaseConfig{SQLitePath: path}
			var localTasks ports.TaskRepository
			var localBindings ports.TaskBindingRepository
			var localScenarios ports.ScenarioRepository
			app := fx.New(fx.NopLogger, fx.Supply(cfg), fx.Provide(database.NewSQLite, storage.NewTaskRepository, storage.NewTaskBindingRepository, storage.NewScenarioRepository), fx.Populate(&localTasks, &localBindings, &localScenarios))
			Expect(app.Start(ctx)).To(Succeed())
			return app, localTasks, localBindings, localScenarios
		}

		app, localTasks, localBindings, localScenarios := start()
		implementation := scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}
		Expect(localScenarios.Create(ctx, implementation)).To(Succeed())
		Expect(localTasks.Register(ctx, task.Task{Provider: "google-classroom", ExternalID: "coursework-1", CourseID: "course-1", Title: "Report"})).To(Succeed())
		Expect(localBindings.Bind(ctx, implementation.Name, "coursework-1")).To(Succeed())
		Expect(app.Stop(ctx)).To(Succeed())

		app, localTasks, localBindings, localScenarios = start()
		DeferCleanup(app.Stop)
		items, err := localTasks.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		active, err := localBindings.Active(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(HaveLen(1))
		Expect(active[0].ScenarioName).To(Equal(implementation.Name))
		registered, err := localScenarios.ListActive(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(registered).To(HaveLen(1))
	})

	It("rejects missing and ambiguous binding targets and inactive scenarios", func(ctx SpecContext) {
		implementation := scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}
		Expect(scenarios.Create(ctx, implementation)).To(Succeed())
		Expect(tasks.Register(ctx, task.Task{Provider: "google-classroom", ExternalID: "duplicate", CourseID: "course-1", Title: "One"})).To(Succeed())
		Expect(tasks.Register(ctx, task.Task{Provider: "google-classroom", ExternalID: "duplicate", CourseID: "course-2", Title: "Two"})).To(Succeed())

		Expect(bindings.Bind(ctx, implementation.Name, "missing")).To(MatchError(ContainSubstring("TaskNotFound")))
		Expect(bindings.Bind(ctx, implementation.Name, "duplicate")).To(MatchError(ContainSubstring("TaskAmbiguous")))
		Expect(bindings.Unbind(ctx, "missing")).To(MatchError(ContainSubstring("TaskNotFound")))
		Expect(bindings.Unbind(ctx, "duplicate")).To(MatchError(ContainSubstring("TaskAmbiguous")))
		Expect(tasks.Register(ctx, task.Task{Provider: "google-classroom", ExternalID: "unbound", CourseID: "course-1", Title: "Unbound"})).To(Succeed())
		Expect(bindings.Unbind(ctx, "unbound")).To(MatchError(ContainSubstring("TaskNotFound")))

		Expect(scenarios.SetStatus(ctx, implementation.Name, scenario.Disabled)).To(Succeed())
		Expect(bindings.Bind(ctx, implementation.Name, "duplicate")).To(MatchError(ContainSubstring("sql: no rows")))
		listed, err := bindings.List(ctx, "missing-scenario")
		Expect(err).NotTo(HaveOccurred())
		Expect(listed).To(BeEmpty())
	})

	It("removes active and inactive bindings without deleting task history or violating foreign keys", func(ctx SpecContext) {
		removable := scenario.Implementation{Name: "removable", Type: "RemovableScenario"}
		keeper := scenario.Implementation{Name: "keeper", Type: "KeeperScenario"}
		Expect(scenarios.Create(ctx, removable)).To(Succeed())
		Expect(scenarios.Create(ctx, keeper)).To(Succeed())

		orphaned := task.Task{
			Provider: "google-classroom", ExternalID: "orphaned", CourseID: "course-1",
			Title: "Historical report", Description: "Keep this description", DueAt: "2026-02-03T04:05:06Z",
		}
		shared := task.Task{Provider: "google-classroom", ExternalID: "shared", CourseID: "course-1", Title: "Shared report"}
		Expect(tasks.Register(ctx, orphaned)).To(Succeed())
		Expect(tasks.Register(ctx, shared)).To(Succeed())
		Expect(bindings.Bind(ctx, removable.Name, orphaned.ExternalID)).To(Succeed())
		Expect(bindings.Bind(ctx, removable.Name, shared.ExternalID)).To(Succeed())
		Expect(bindings.Bind(ctx, keeper.Name, shared.ExternalID)).To(Succeed())

		// Leave a historical, inactive binding behind. Removing the scenario must
		// delete it, while the task itself remains available for historical reads.
		Expect(bindings.Unbind(ctx, orphaned.ExternalID)).To(Succeed())
		Expect(scenarios.Remove(ctx, removable.Name)).To(Succeed())

		remaining, err := scenarios.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(remaining).To(HaveLen(1))
		Expect(remaining[0].Implementation).To(Equal(keeper))

		removedBindings, err := bindings.List(ctx, removable.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(removedBindings).To(BeEmpty())
		active, err := bindings.Active(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(HaveLen(1))
		Expect(active[0].ScenarioName).To(Equal(keeper.Name))
		Expect(active[0].Task.ExternalID).To(Equal(shared.ExternalID))

		remainingTasks, err := tasks.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(remainingTasks).To(HaveLen(2))
		byExternalID := map[string]task.Task{}
		for _, item := range remainingTasks {
			byExternalID[item.ExternalID] = item
		}
		Expect(byExternalID).To(HaveKey(orphaned.ExternalID))
		orphanedAfterRemoval := byExternalID[orphaned.ExternalID]
		Expect(orphanedAfterRemoval.CourseID).To(Equal(orphaned.CourseID))
		Expect(orphanedAfterRemoval.Title).To(Equal(orphaned.Title))
		Expect(orphanedAfterRemoval.Description).To(Equal(orphaned.Description))
		Expect(orphanedAfterRemoval.DueAt).To(Equal(orphaned.DueAt))
		Expect(orphanedAfterRemoval.Status).To(Equal(task.Discovered))
		Expect(byExternalID).To(HaveKey(shared.ExternalID))
		Expect(byExternalID[shared.ExternalID].Title).To(Equal(shared.Title))
		Expect(byExternalID[shared.ExternalID].Status).To(Equal(task.Bound))

		rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(rows.Close)
		Expect(rows.Next()).To(BeFalse(), "scenario removal must leave no foreign-key violations")
		Expect(rows.Err()).NotTo(HaveOccurred())
	})

	It("returns database errors from canceled operations", func(ctx SpecContext) {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		item := task.Task{Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1", Title: "Report"}
		Expect(tasks.Register(canceled, item)).To(MatchError(ContainSubstring("context canceled")))
		_, err := tasks.List(canceled)
		Expect(err).To(MatchError(ContainSubstring("context canceled")))
		Expect(bindings.Bind(canceled, "science-report", "task-1")).To(MatchError(ContainSubstring("context canceled")))
		Expect(bindings.Unbind(canceled, "task-1")).To(MatchError(ContainSubstring("context canceled")))
		_, err = bindings.List(canceled, "science-report")
		Expect(err).To(MatchError(ContainSubstring("context canceled")))
		_, err = bindings.Active(canceled)
		Expect(err).To(MatchError(ContainSubstring("context canceled")))
	})

	It("returns database errors when the database is closed", func(ctx SpecContext) {
		Expect(db.Close()).To(Succeed())

		item := task.Task{Provider: "google-classroom", ExternalID: "task-1", CourseID: "course-1", Title: "Report"}
		Expect(tasks.Register(ctx, item)).To(MatchError(ContainSubstring("register task \"task-1\": sql: database is closed")))
		_, err := tasks.List(ctx)
		Expect(err).To(MatchError(ContainSubstring("list tasks: sql: database is closed")))
		Expect(bindings.Bind(ctx, "science-report", "task-1")).To(MatchError(ContainSubstring("find active scenario \"science-report\": sql: database is closed")))
		Expect(bindings.Unbind(ctx, "task-1")).To(MatchError(ContainSubstring("find task \"task-1\": sql: database is closed")))
		_, err = bindings.List(ctx, "science-report")
		Expect(err).To(MatchError(ContainSubstring("list task bindings: sql: database is closed")))
		_, err = bindings.Active(ctx)
		Expect(err).To(MatchError(ContainSubstring("list task bindings: sql: database is closed")))
	})
})
