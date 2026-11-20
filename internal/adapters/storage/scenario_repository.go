package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alex/assistent/internal/adapters/mappers"
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/scenario"
)

const scenarioColumns = "id, name, implementation_type, status, created_at, updated_at, last_run_at"
const sqliteNowUTC = "strftime('%Y-%m-%dT%H:%M:%fZ', 'now')"

type SQLiteScenarioRepository struct {
	db *sql.DB
}

func NewScenarioRepository(db *sql.DB) ports.ScenarioRepository {
	return &SQLiteScenarioRepository{db: db}
}

func (r *SQLiteScenarioRepository) Create(ctx context.Context, implementation scenario.Implementation) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO scenarios (name, implementation_type, status, created_at, updated_at)
        VALUES (?, ?, ?, `+sqliteNowUTC+`, `+sqliteNowUTC+`) ON CONFLICT(name) DO NOTHING`,
		implementation.Name, implementation.Type, scenario.Active)
	if err != nil {
		return fmt.Errorf("register scenario %q: %w", implementation.Name, err)
	}
	return nil
}

func (r *SQLiteScenarioRepository) List(ctx context.Context) ([]scenario.Scenario, error) {
	return r.list(ctx, `SELECT `+scenarioColumns+` FROM scenarios ORDER BY name`)
}

func (r *SQLiteScenarioRepository) ListActive(ctx context.Context) ([]scenario.Scenario, error) {
	return r.list(ctx, `SELECT `+scenarioColumns+` FROM scenarios WHERE status = ? ORDER BY name`, scenario.Active)
}

func (r *SQLiteScenarioRepository) list(ctx context.Context, query string, args ...any) ([]scenario.Scenario, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list scenarios: %w", err)
	}
	defer rows.Close()

	results := make([]scenario.Scenario, 0)
	for rows.Next() {
		var row models.ScenarioRow
		if err := rows.Scan(&row.ID, &row.Name, &row.ImplementationType, &row.Status,
			&row.CreatedAt, &row.UpdatedAt, &row.LastRunAt); err != nil {
			return nil, fmt.Errorf("scan scenario: %w", err)
		}
		item, err := mappers.ToScenario(row)
		if err != nil {
			return nil, fmt.Errorf("map scenario %q: %w", row.Name, err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read scenarios: %w", err)
	}
	return results, nil
}

func (r *SQLiteScenarioRepository) SetStatus(ctx context.Context, name string, status scenario.Status) error {
	result, err := r.db.ExecContext(ctx, `UPDATE scenarios SET status = ?1,
        updated_at = CASE WHEN status <> ?1 THEN `+sqliteNowUTC+` ELSE updated_at END WHERE name = ?2`, status, name)
	return checkedScenarioWrite("set status for", name, result, err)
}

func (r *SQLiteScenarioRepository) Remove(ctx context.Context, name string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM scenarios WHERE name = ?`, name)
	return checkedScenarioWrite("remove", name, result, err)
}

func checkedScenarioWrite(action, name string, result sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("%s scenario %q: %w", action, name, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s scenario %q: %w", action, name, err)
	}
	if affected == 0 {
		return scenario.NotFound(name)
	}
	return nil
}
