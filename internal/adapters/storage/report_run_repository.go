package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
)

type SQLiteReportRunRepository struct{ db *sql.DB }

func NewReportRunRepository(db *sql.DB) ports.ReportRunRepository {
	return &SQLiteReportRunRepository{db: db}
}

func (r *SQLiteReportRunRepository) Create(ctx context.Context, run *report.Run) error {
	err := r.db.QueryRowContext(ctx, `INSERT INTO report_runs (scenario_id, task_id, status, started_at, finished_at, is_partial, result_path, error_message, created_at) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?) RETURNING id`, run.ScenarioID, run.TaskID, run.Status, nullableTime(run.StartedAt), nullableTime(run.FinishedAt), run.Partial, run.ResultPath, run.ErrorMessage, run.CreatedAt.UTC().Format(time.RFC3339Nano)).Scan(&run.ID)
	if err != nil {
		return fmt.Errorf("insert report run: %w", err)
	}
	return nil
}

func (r *SQLiteReportRunRepository) Update(ctx context.Context, run *report.Run) error {
	result, err := r.db.ExecContext(ctx, `UPDATE report_runs SET status=?, started_at=?, finished_at=?, is_partial=?, result_path=NULLIF(?, ''), error_message=NULLIF(?, '') WHERE id=?`, run.Status, nullableTime(run.StartedAt), nullableTime(run.FinishedAt), run.Partial, run.ResultPath, run.ErrorMessage, run.ID)
	if err != nil {
		return fmt.Errorf("update report run %d: %w", run.ID, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("report run %d not found", run.ID)
	}
	return nil
}

func (r *SQLiteReportRunRepository) Get(ctx context.Context, id int64) (report.Run, error) {
	var run report.Run
	var started, finished, resultPath, errorMessage sql.NullString
	var created string
	err := r.db.QueryRowContext(ctx, `SELECT scenario_id, task_id, status, started_at, finished_at, is_partial, COALESCE(result_path,''), COALESCE(error_message,''), created_at FROM report_runs WHERE id=?`, id).Scan(&run.ScenarioID, &run.TaskID, &run.Status, &started, &finished, &run.Partial, &resultPath, &errorMessage, &created)
	if err != nil {
		return report.Run{}, fmt.Errorf("get report run %d: %w", id, err)
	}
	run.ID, run.ResultPath, run.ErrorMessage = id, resultPath.String, errorMessage.String
	run.StartedAt, err = parseTime(started)
	if err != nil {
		return report.Run{}, err
	}
	run.FinishedAt, err = parseTime(finished)
	if err != nil {
		return report.Run{}, err
	}
	run.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return run, err
}

func (r *SQLiteReportRunRepository) GetByScenarioTask(ctx context.Context, scenarioID, taskID int64) (report.Run, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM report_runs WHERE scenario_id=? AND task_id=? ORDER BY id DESC LIMIT 1`, scenarioID, taskID).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return report.Run{}, fmt.Errorf("%w: scenario %d task %d", ports.ErrReportRunNotFound, scenarioID, taskID)
		}
		return report.Run{}, fmt.Errorf("find report run for scenario %d task %d: %w", scenarioID, taskID, err)
	}
	return r.Get(ctx, id)
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
func parseTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	return &parsed, err
}
