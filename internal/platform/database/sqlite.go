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
