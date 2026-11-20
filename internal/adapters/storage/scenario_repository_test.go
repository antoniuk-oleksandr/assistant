package storage_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/adapters/storage"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/scenario"
	"github.com/alex/assistent/internal/platform/config"
	"github.com/alex/assistent/internal/platform/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

func TestScenarioRepository(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQLite Scenario Repository")
}

var _ = Describe("SQLite scenario selection", func() {
	var db *sql.DB
	var repo ports.ScenarioRepository

	science := scenario.Implementation{Name: "science-report", Type: "ScienceReportScenario"}
	other := scenario.Implementation{Name: "other-report", Type: "OtherReportScenario"}

	BeforeEach(func(ctx SpecContext) {
		cfg := config.DatabaseConfig{SQLitePath: filepath.Join(GinkgoT().TempDir(), "state.db")}
		app := fx.New(fx.NopLogger, fx.Supply(cfg), fx.Provide(database.NewSQLite, storage.NewScenarioRepository), fx.Populate(&db, &repo))
		Expect(app.Start(ctx)).To(Succeed())
		DeferCleanup(app.Stop)
	})

	It("excludes disabled scenarios from the active selection and restores them when enabled", func(ctx SpecContext) {
		Expect(repo.Create(ctx, science)).To(Succeed())
		Expect(repo.Create(ctx, other)).To(Succeed())
		Expect(repo.SetStatus(ctx, science.Name, scenario.Disabled)).To(Succeed())
		Expect(repo.Create(ctx, science)).To(Succeed())

		all, err := repo.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(all).To(HaveLen(2))
		Expect(all[0].Implementation).To(Equal(other))
		Expect(all[0].Status).To(Equal(scenario.Active))
		Expect(all[1].Implementation).To(Equal(science))
		Expect(all[1].Status).To(Equal(scenario.Disabled))

		active, err := repo.ListActive(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(Equal(all[:1]))

		Expect(repo.SetStatus(ctx, science.Name, scenario.Active)).To(Succeed())
		active, err = repo.ListActive(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(HaveLen(2))
		Expect(active[1].ID).To(Equal(all[1].ID))
		Expect(active[1].Implementation).To(Equal(science))
		Expect(active[1].Status).To(Equal(scenario.Active))
		Expect(active[1].CreatedAt).To(Equal(all[1].CreatedAt))

		Expect(repo.Remove(ctx, science.Name)).To(Succeed())
		remaining, err := repo.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(remaining).To(Equal(all[:1]))
	})

	It("reports a missing registration on status changes and removal", func(ctx SpecContext) {
		Expect(repo.SetStatus(ctx, science.Name, scenario.Disabled)).To(MatchError(scenario.ErrNotFound))
		Expect(repo.Remove(ctx, science.Name)).To(MatchError(scenario.ErrNotFound))
		Expect(repo.Create(ctx, science)).To(Succeed())
		Expect(repo.Remove(ctx, science.Name)).To(Succeed())
		Expect(repo.Remove(ctx, science.Name)).To(MatchError(scenario.ErrNotFound))
	})

	It("removes active bindings and resets their task status", func(ctx SpecContext) {
		Expect(repo.Create(ctx, science)).To(Succeed())
		_, err := db.ExecContext(ctx, `INSERT INTO tasks
			(provider, external_task_id, course_id, title, discovered_at, last_seen_at, status)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, "google-classroom", "task-1", "course-1", "Report",
			"2026-11-20T00:00:00Z", "2026-11-20T00:00:00Z", "bound")
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `INSERT INTO scenario_tasks (scenario_id, task_id, bound_at, status)
			SELECT id, ?, ?, ? FROM scenarios WHERE name = ?`, 1, "2026-11-20T00:00:00Z", "active", science.Name)
		Expect(err).NotTo(HaveOccurred())

		Expect(repo.Remove(ctx, science.Name)).To(Succeed())

		var taskStatus string
		Expect(db.QueryRowContext(ctx, "SELECT status FROM tasks WHERE id = 1").Scan(&taskStatus)).To(Succeed())
		Expect(taskStatus).To(Equal("discovered"))
		var bindingCount int
		Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM scenario_tasks").Scan(&bindingCount)).To(Succeed())
		Expect(bindingCount).To(Equal(0))
	})

	It("translates database and mapping failures", func(ctx SpecContext) {
		var db *sql.DB
		var localRepo ports.ScenarioRepository
		path := filepath.Join(GinkgoT().TempDir(), "invalid.db")
		app := fx.New(fx.NopLogger, fx.Supply(config.DatabaseConfig{SQLitePath: path}),
			fx.Provide(database.NewSQLite, storage.NewScenarioRepository), fx.Populate(&db, &localRepo))
		Expect(app.Start(ctx)).To(Succeed())
		_, err := db.ExecContext(ctx, `INSERT INTO scenarios
			(name, implementation_type, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"invalid", "Scenario", scenario.Active, "bad", "bad")
		Expect(err).NotTo(HaveOccurred())
		Expect(localRepo.List(ctx)).Error().To(MatchError(ContainSubstring(`map scenario "invalid"`)))
		Expect(db.ExecContext(ctx, `UPDATE scenarios SET created_at='2026-01-02T03:04:05Z', updated_at='invalid'`)).Error().NotTo(HaveOccurred())
		Expect(localRepo.List(ctx)).Error().To(MatchError(ContainSubstring(`map scenario "invalid"`)))
		Expect(db.ExecContext(ctx, `UPDATE scenarios SET updated_at='2026-01-02T03:04:05Z', last_run_at='invalid'`)).Error().NotTo(HaveOccurred())
		Expect(localRepo.List(ctx)).Error().To(MatchError(ContainSubstring(`map scenario "invalid"`)))
		Expect(app.Stop(ctx)).To(Succeed())
		Expect(localRepo.Create(ctx, science)).Error().To(MatchError(ContainSubstring("register scenario")))
		Expect(localRepo.List(ctx)).Error().To(MatchError(ContainSubstring("list scenarios")))
		Expect(localRepo.SetStatus(ctx, science.Name, scenario.Disabled)).To(MatchError(ContainSubstring("set status for")))
		Expect(localRepo.Remove(ctx, science.Name)).Error().To(MatchError(ContainSubstring("remove")))
	})

	It("returns database errors when the database is closed", func(ctx SpecContext) {
		Expect(db.Close()).To(Succeed())

		Expect(repo.Create(ctx, science)).To(MatchError(ContainSubstring("register scenario \"science-report\": sql: database is closed")))
		Expect(repo.List(ctx)).Error().To(MatchError(ContainSubstring("list scenarios: sql: database is closed")))
		Expect(repo.ListActive(ctx)).Error().To(MatchError(ContainSubstring("list scenarios: sql: database is closed")))
		Expect(repo.SetStatus(ctx, science.Name, scenario.Disabled)).To(MatchError(ContainSubstring("set status for scenario \"science-report\": sql: database is closed")))
		Expect(repo.Remove(ctx, science.Name)).To(MatchError(ContainSubstring("remove scenario \"science-report\": begin transaction: sql: database is closed")))
	})
})
