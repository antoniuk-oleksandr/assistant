package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	"github.com/alex/assistent/internal/platform/config"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

type captureLifecycle struct{ hook fx.Hook }

func (l *captureLifecycle) Append(hook fx.Hook) { l.hook = hook }

var _ = Describe("SQLite migrations", func() {
	It("rejects directory creation below a file", func() {
		file := filepath.Join(GinkgoT().TempDir(), "not-a-directory")
		Expect(os.WriteFile(file, []byte("file"), 0600)).To(Succeed())

		_, err := fileDSN(filepath.Join(file, "state.db"))
		Expect(err).To(HaveOccurred())
	})

	It("adds lifecycle metadata to existing task tables", func() {
		db, err := sql.Open("sqlite3", filepath.Join(GinkgoT().TempDir(), "state.db"))
		Expect(err).NotTo(HaveOccurred())
		defer db.Close()
		_, err = db.Exec(`CREATE TABLE tasks (id INTEGER PRIMARY KEY, created_at_external TEXT, due_at TEXT, discovered_at TEXT NOT NULL, last_seen_at TEXT NOT NULL)`)
		Expect(err).NotTo(HaveOccurred())
		Expect(migrateTaskColumns(context.Background(), db)).To(Succeed())
		for _, column := range []string{"updated_at_external", "due_date", "due_time", "time_zone", "phase", "phase_at"} {
			var exists bool
			Expect(db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pragma_table_info('tasks') WHERE name = ?)`, column).Scan(&exists)).To(Succeed())
			Expect(exists).To(BeTrue())
		}
	})

	It("adds missing columns idempotently", func() {
		db, err := sql.Open("sqlite3", ":memory:")
		Expect(err).NotTo(HaveOccurred())
		defer db.Close()
		for _, statement := range []string{
			`CREATE TABLE report_runs (id INTEGER PRIMARY KEY)`,
			`CREATE TABLE reminders (id INTEGER PRIMARY KEY)`,
			`CREATE TABLE tasks (id INTEGER PRIMARY KEY)`,
			`CREATE TABLE materials (id INTEGER PRIMARY KEY)`,
		} {
			_, err := db.Exec(statement)
			Expect(err).NotTo(HaveOccurred())
		}
		ctx := context.Background()
		for _, migrate := range []func(context.Context, *sql.DB) error{
			migrateReportRunColumns,
			migrateReminderColumns,
			migrateTaskColumns,
			migrateMaterialColumns,
		} {
			Expect(migrate(ctx, db)).To(Succeed())
			Expect(migrate(ctx, db)).To(Succeed())
		}
		for table, columns := range map[string][]string{
			"report_runs": {"input_fingerprint"},
			"reminders":   {"subject", "message", "due_at", "classroom_link"},
			"tasks":       {"updated_at_external", "due_date", "due_time", "time_zone", "phase", "phase_at"},
			"materials":   {"size_bytes", "parse_result"},
		} {
			for _, column := range columns {
				var exists bool
				Expect(db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pragma_table_info(?) WHERE name = ?)`, table, column).Scan(&exists)).To(Succeed())
				Expect(exists).To(BeTrue())
			}
		}
	})

	It("returns errors from closed databases", func() {
		db, err := sql.Open("sqlite3", ":memory:")
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Close()).To(Succeed())
		for _, migrate := range []func(context.Context, *sql.DB) error{migrateReportRunColumns, migrateReminderColumns, migrateTaskColumns, migrateMaterialColumns} {
			Expect(migrate(context.Background(), db)).To(HaveOccurred())
		}
	})

	It("returns schema errors when target tables are missing", func() {
		for name, migrate := range map[string]func(context.Context, *sql.DB) error{
			"report runs": migrateReportRunColumns,
			"reminders":   migrateReminderColumns,
			"tasks":       migrateTaskColumns,
			"materials":   migrateMaterialColumns,
		} {
			By(name)
			db, err := sql.Open("sqlite3", ":memory:")
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()
			Expect(migrate(context.Background(), db)).To(HaveOccurred())
		}
	})

	It("reports schema initialization errors at different stages", func() {
		tests := []struct {
			name   string
			prefix []string
			object string
		}{
			{"scenarios", nil, "scenarios"},
			{"classroom courses", []string{createScenarios}, "classroom_courses"},
			{"tasks", []string{createScenarios, createClassroomCourses}, "tasks"},
			{"scenario tasks", []string{createScenarios, createClassroomCourses, createTasks}, "scenario_tasks"},
			{"task participants", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks}, "task_participants"},
			{"missing submitters", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants}, "missing_submitters"},
			{"scenario courses", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters}, "scenario_courses"},
			{"scenario course data", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses}, "scenario_course_data"},
			{"poll state", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData}, "poll_state"},
			{"submissions", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData, createPollState}, "submissions"},
			{"materials", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData, createPollState, createSubmissions}, "materials"},
			{"report runs", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData, createPollState, createSubmissions, createMaterials}, "report_runs"},
			{"report results", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData, createPollState, createSubmissions, createMaterials, createReportRuns, createReportRunIndexes, createReportOutputs}, "report_results"},
			{"report deliveries", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData, createPollState, createSubmissions, createMaterials, createReportRuns, createReportRunIndexes, createReportOutputs, createReportResults}, "report_deliveries"},
			{"reminders", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData, createPollState, createSubmissions, createMaterials, createReportRuns, createReportRunIndexes, createReportOutputs, createReportResults, createReportDeliveries}, "reminders"},
			{"teachers", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData, createPollState, createSubmissions, createMaterials, createReportRuns, createReportRunIndexes, createReportOutputs, createReportResults, createReportDeliveries, createReminders}, "teachers"},
			{"notification channels", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData, createPollState, createSubmissions, createMaterials, createReportRuns, createReportRunIndexes, createReportOutputs, createReportResults, createReportDeliveries, createReminders, createTeachers}, "notification_channels"},
			{"telegram pairings", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData, createPollState, createSubmissions, createMaterials, createReportRuns, createReportRunIndexes, createReportOutputs, createReportResults, createReportDeliveries, createReminders, createTeachers, createNotificationChannels}, "telegram_pairings"},
			{"material parses", []string{createScenarios, createClassroomCourses, createTasks, createScenarioTasks, createTaskParticipants, createMissingSubmitters, createScenarioCourses, createScenarioCourseData, createPollState, createSubmissions, createMaterials, createReportRuns, createReportRunIndexes, createReportOutputs, createReportResults, createReportDeliveries, createReminders, createTeachers, createNotificationChannels, createTelegramPairings}, "material_parses"},
		}
		for _, test := range tests {
			By(test.name)
			path := filepath.Join(GinkgoT().TempDir(), "state.db")
			seed, err := sql.Open("sqlite3", path)
			Expect(err).NotTo(HaveOccurred())
			for _, statement := range test.prefix {
				_, err := seed.Exec(statement)
				Expect(err).NotTo(HaveOccurred())
			}
			_, err = seed.Exec("CREATE TABLE conflict_source (id INTEGER); CREATE INDEX " + test.object + " ON conflict_source(id)")
			Expect(err).NotTo(HaveOccurred())
			Expect(seed.Close()).To(Succeed())
			lifecycle := &captureLifecycle{}
			db, err := NewSQLite(lifecycle, config.DatabaseConfig{SQLitePath: path})
			Expect(err).NotTo(HaveOccurred())
			Expect(lifecycle.hook.OnStart(context.Background())).To(HaveOccurred())
			_ = db.Close()
		}
	})

})
