package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex/assistent/internal/platform/config"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/fx"
)

const createScenarios = `CREATE TABLE IF NOT EXISTS scenarios (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    implementation_type TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_run_at TEXT
)`

const createClassroomCourses = `CREATE TABLE IF NOT EXISTS classroom_courses (
    id INTEGER PRIMARY KEY,
    provider TEXT NOT NULL,
    external_course_id TEXT UNIQUE NOT NULL,
    source_reference TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
)`

const createTasks = `CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY,
    provider TEXT NOT NULL,
    external_task_id TEXT NOT NULL,
    course_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    created_at_external TEXT,
    updated_at_external TEXT,
    due_date TEXT,
    due_time TEXT,
    time_zone TEXT,
    due_at TEXT,
    discovered_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('discovered', 'bound', 'closed', 'ignored')),
    UNIQUE(provider, course_id, external_task_id)
)`

const createScenarioTasks = `CREATE TABLE IF NOT EXISTS scenario_tasks (
    scenario_id INTEGER NOT NULL REFERENCES scenarios(id),
    task_id INTEGER NOT NULL REFERENCES tasks(id),
    bound_at TEXT NOT NULL,
    unbound_at TEXT,
    status TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    PRIMARY KEY (scenario_id, task_id)
)`

const createScenarioCourses = `CREATE TABLE IF NOT EXISTS scenario_courses (
    scenario_id INTEGER NOT NULL REFERENCES scenarios(id),
    course_id INTEGER NOT NULL REFERENCES classroom_courses(id),
    bound_at TEXT NOT NULL,
    unbound_at TEXT,
    status TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    PRIMARY KEY (scenario_id, course_id)
)`

const createScenarioCourseData = `CREATE TABLE IF NOT EXISTS scenario_course_data (
    scenario_id INTEGER NOT NULL REFERENCES scenarios(id),
    course_id INTEGER NOT NULL REFERENCES classroom_courses(id),
    field_name TEXT NOT NULL,
    field_value TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (scenario_id, course_id, field_name)
)`

const createPollState = `CREATE TABLE IF NOT EXISTS poll_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    last_check_timestamp TEXT NOT NULL,
    updated_at TEXT NOT NULL
)`

const createSubmissions = `CREATE TABLE IF NOT EXISTS submissions (
    id INTEGER PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES tasks(id),
    external_submission_id TEXT NOT NULL,
    submitter_id TEXT NOT NULL DEFAULT '',
    submitter_email TEXT NOT NULL DEFAULT '',
    submitter_name TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN ('new', 'created', 'submitted', 'returned', 'unknown')),
    submitted_at TEXT,
    created_at_external TEXT,
    updated_at_external TEXT,
    late INTEGER NOT NULL DEFAULT 0,
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    UNIQUE(task_id, external_submission_id)
)`

const createMaterials = `CREATE TABLE IF NOT EXISTS materials (
    id INTEGER PRIMARY KEY,
    submission_id INTEGER NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    external_material_id TEXT,
    name TEXT NOT NULL,
    mime_type TEXT,
    size_bytes INTEGER CHECK (size_bytes >= 0),
    source_uri TEXT,
    local_path TEXT,
    appendix_number INTEGER,
    role TEXT NOT NULL DEFAULT 'supporting_evidence',
    status TEXT NOT NULL CHECK (status IN ('pending', 'parsed', 'invalid', 'access_denied', 'not_found', 'failed')),
    parse_result TEXT,
    created_at TEXT NOT NULL
)`

const createReportRuns = `CREATE TABLE IF NOT EXISTS report_runs (
    id INTEGER PRIMARY KEY,
    scenario_id INTEGER NOT NULL REFERENCES scenarios(id),
    task_id INTEGER NOT NULL REFERENCES tasks(id),
    status TEXT NOT NULL CHECK (status IN ('scheduled', 'collecting', 'processing', 'waiting_for_deadline', 'completed', 'failed')),
    started_at TEXT,
    finished_at TEXT,
    is_partial INTEGER NOT NULL DEFAULT 0,
    result_path TEXT,
    error_message TEXT,
    created_at TEXT NOT NULL
)`

const createReportRunIndexes = `CREATE UNIQUE INDEX IF NOT EXISTS ux_report_runs_one_active
ON report_runs (scenario_id, task_id)
WHERE status IN ('scheduled', 'collecting', 'processing', 'waiting_for_deadline')`

const createReportOutputs = `CREATE TABLE IF NOT EXISTS report_outputs (
    id INTEGER PRIMARY KEY,
    report_run_id INTEGER NOT NULL REFERENCES report_runs(id),
    format TEXT NOT NULL,
    path TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL
)`

const createReportResults = `CREATE TABLE IF NOT EXISTS report_results (
    id INTEGER PRIMARY KEY,
    report_run_id INTEGER NOT NULL REFERENCES report_runs(id),
    scenario_id INTEGER NOT NULL REFERENCES scenarios(id),
    task_id INTEGER NOT NULL REFERENCES tasks(id),
    version INTEGER NOT NULL CHECK (version > 0),
    status TEXT NOT NULL CHECK (status IN ('processing', 'partial', 'completed', 'failed')),
    is_partial INTEGER NOT NULL DEFAULT 0,
    result_json TEXT NOT NULL,
    error_message TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (report_run_id, version)
)`

const createReportDeliveries = `CREATE TABLE IF NOT EXISTS report_deliveries (
    id INTEGER PRIMARY KEY,
    result_id INTEGER NOT NULL REFERENCES report_results(id),
    channel TEXT NOT NULL,
    destination TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'sent', 'failed')),
    error_message TEXT,
    attempted_at TEXT,
    created_at TEXT NOT NULL,
    UNIQUE (result_id, channel, destination)
)`

const createReminders = `CREATE TABLE IF NOT EXISTS reminders (
    id INTEGER PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES tasks(id),
    person_id TEXT NOT NULL,
    channel TEXT NOT NULL,
    destination TEXT NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'dead_letter')),
    error_message TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT,
    attempted_at TEXT,
    created_at TEXT NOT NULL,
    UNIQUE (task_id, person_id, channel, kind)
)`

const createTeachers = `CREATE TABLE IF NOT EXISTS teachers (
 id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL, classroom_id TEXT, status TEXT NOT NULL CHECK(status IN ('active','disabled')), created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(email))`
const createNotificationChannels = `CREATE TABLE IF NOT EXISTS notification_channels (
 id INTEGER PRIMARY KEY, teacher_id INTEGER NOT NULL REFERENCES teachers(id), kind TEXT NOT NULL, destination TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('active','disabled')), created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(kind,destination))`

const createMaterialParses = `CREATE TABLE IF NOT EXISTS material_parses (
    material_id INTEGER NOT NULL REFERENCES materials(id) ON DELETE CASCADE,
    scenario_id INTEGER NOT NULL,
    snapshot_path TEXT NOT NULL,
    outcome TEXT NOT NULL,
    PRIMARY KEY (material_id, scenario_id, snapshot_path)
)`

const createSubmissionIndexes = `CREATE INDEX IF NOT EXISTS ix_submissions_task_state ON submissions(task_id, state);
CREATE UNIQUE INDEX IF NOT EXISTS ux_materials_external ON materials(submission_id, external_material_id) WHERE external_material_id IS NOT NULL`

func NewSQLite(lifecycle fx.Lifecycle, cfg config.DatabaseConfig) (*sql.DB, error) {
	dsn, err := fileDSN(cfg.SQLitePath)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if _, err := db.ExecContext(ctx, createScenarios); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite scenario schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createClassroomCourses); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite classroom course schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createTasks); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite task schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createScenarioTasks); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite task binding schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createScenarioCourses); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite scenario course schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createScenarioCourseData); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite scenario course data schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createPollState); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite polling state schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createSubmissions); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite submission schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createMaterials); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite material schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createReportRuns); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite report run schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createReportRunIndexes); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite report run indexes: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createReportOutputs); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite report output schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createReportResults); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite report result schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createReportDeliveries); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite report delivery schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createReminders); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite reminder schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createTeachers); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite teacher schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createNotificationChannels); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite notification channel schema: %w", err), db.Close())
			}
			if err := migrateMaterialColumns(ctx, db); err != nil {
				return errors.Join(fmt.Errorf("migrate SQLite material columns: %w", err), db.Close())
			}
			if err := migrateTaskColumns(ctx, db); err != nil {
				return errors.Join(fmt.Errorf("migrate SQLite task columns: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createMaterialParses); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite material parse schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createSubmissionIndexes); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite submission indexes: %w", err), db.Close())
			}
			return nil
		},
		OnStop: func(context.Context) error { return db.Close() },
	})
	return db, nil
}

func migrateTaskColumns(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, column := range []struct{ name, definition string }{
		{"updated_at_external", "TEXT"},
		{"due_date", "TEXT"},
		{"due_time", "TEXT"},
		{"time_zone", "TEXT"},
	} {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pragma_table_info('tasks') WHERE name = ?)`, column.name).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN `+column.name+` `+column.definition); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func migrateMaterialColumns(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, column := range []struct{ name, definition string }{
		{"size_bytes", "INTEGER CHECK (size_bytes >= 0)"},
		{"parse_result", "TEXT"},
	} {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pragma_table_info('materials') WHERE name = ?)`, column.name).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, `ALTER TABLE materials ADD COLUMN `+column.name+` `+column.definition); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func fileDSN(path string) (string, error) {
	if path == ":memory:" {
		return path + "?_foreign_keys=on", nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve SQLite path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return "", fmt.Errorf("create SQLite directory: %w", err)
	}
	slashPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	uri := url.URL{Scheme: "file", Path: slashPath, RawQuery: "_foreign_keys=on&_busy_timeout=5000"}
	return uri.String(), nil
}
