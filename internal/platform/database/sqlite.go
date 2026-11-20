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
    source_uri TEXT,
    local_path TEXT,
    appendix_number INTEGER,
    role TEXT NOT NULL DEFAULT 'supporting_evidence',
    status TEXT NOT NULL CHECK (status IN ('pending', 'parsed', 'invalid', 'access_denied', 'not_found', 'failed')),
    created_at TEXT NOT NULL
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
			if _, err := db.ExecContext(ctx, createPollState); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite polling state schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createSubmissions); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite submission schema: %w", err), db.Close())
			}
			if _, err := db.ExecContext(ctx, createMaterials); err != nil {
				return errors.Join(fmt.Errorf("initialize SQLite material schema: %w", err), db.Close())
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
