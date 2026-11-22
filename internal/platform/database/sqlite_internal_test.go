package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/alex/assistent/internal/platform/config"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/fx"
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
	for _, column := range []string{"updated_at_external", "due_date", "due_time", "time_zone", "phase", "phase_at"} {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pragma_table_info('tasks') WHERE name = ?)`, column).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("column %q was not added", column)
		}
	}
}

func TestMigrationsAddMissingColumnsAndRemainIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE report_runs (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE reminders (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE tasks (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE materials (id INTEGER PRIMARY KEY)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	for _, migrate := range []func(context.Context, *sql.DB) error{
		migrateReportRunColumns,
		migrateReminderColumns,
		migrateTaskColumns,
		migrateMaterialColumns,
	} {
		if err := migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
		if err := migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	for table, columns := range map[string][]string{
		"report_runs": {"input_fingerprint"},
		"reminders":   {"subject", "message", "due_at", "classroom_link"},
		"tasks":       {"updated_at_external", "due_date", "due_time", "time_zone", "phase", "phase_at"},
		"materials":   {"size_bytes", "parse_result"},
	} {
		for _, column := range columns {
			var exists bool
			if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pragma_table_info(?) WHERE name = ?)`, table, column).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if !exists {
				t.Errorf("column %s.%s was not added", table, column)
			}
		}
	}
}

func TestMigrationsReturnErrorsFromClosedDatabase(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, migrate := range []func(context.Context, *sql.DB) error{migrateReportRunColumns, migrateReminderColumns, migrateTaskColumns, migrateMaterialColumns} {
		if err := migrate(context.Background(), db); err == nil {
			t.Fatal("migration on closed database unexpectedly succeeded")
		}
	}
}

func TestMigrationsReturnSchemaErrorsWhenTargetTablesAreMissing(t *testing.T) {
	for name, migrate := range map[string]func(context.Context, *sql.DB) error{
		"report runs": migrateReportRunColumns,
		"reminders":   migrateReminderColumns,
		"tasks":       migrateTaskColumns,
		"materials":   migrateMaterialColumns,
	} {
		t.Run(name, func(t *testing.T) {
			db, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := migrate(context.Background(), db); err == nil {
				t.Fatal("migration without target table unexpectedly succeeded")
			}
		})
	}
}

type captureLifecycle struct{ hook fx.Hook }

func (l *captureLifecycle) Append(hook fx.Hook) { l.hook = hook }

func TestSQLiteReportsSchemaInitializationErrorsAtDifferentStages(t *testing.T) {
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
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			seed, err := sql.Open("sqlite3", path)
			if err != nil {
				t.Fatal(err)
			}
			for _, statement := range test.prefix {
				if _, err := seed.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := seed.Exec("CREATE TABLE conflict_source (id INTEGER); CREATE INDEX " + test.object + " ON conflict_source(id)"); err != nil {
				t.Fatal(err)
			}
			if err := seed.Close(); err != nil {
				t.Fatal(err)
			}
			lifecycle := &captureLifecycle{}
			db, err := NewSQLite(lifecycle, config.DatabaseConfig{SQLitePath: path})
			if err != nil {
				t.Fatal(err)
			}
			if lifecycle.hook.OnStart(context.Background()) == nil {
				t.Fatal("schema conflict was not reported")
			}
			_ = db.Close()
		})
	}
}
