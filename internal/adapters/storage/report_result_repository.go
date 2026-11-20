package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
)

type SQLiteReportResultRepository struct{ db *sql.DB }

func NewReportResultRepository(db *sql.DB) ports.ReportResultRepository {
	return &SQLiteReportResultRepository{db: db}
}

func (r *SQLiteReportResultRepository) Create(ctx context.Context, result *report.Result) error {
	data, err := json.Marshal(result.Data)
	if err != nil {
		return fmt.Errorf("marshal report result: %w", err)
	}
	err = r.db.QueryRowContext(ctx, `INSERT INTO report_results (report_run_id, scenario_id, task_id, version, status, is_partial, result_json, error_message, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?) RETURNING id`, result.RunID, result.ScenarioID, result.TaskID, result.Version, result.Status, result.Partial, data, result.ErrorMessage, result.CreatedAt.UTC().Format(time.RFC3339Nano), result.UpdatedAt.UTC().Format(time.RFC3339Nano)).Scan(&result.ID)
	if err != nil {
		return fmt.Errorf("insert report result: %w", err)
	}
	return nil
}

func (r *SQLiteReportResultRepository) Update(ctx context.Context, result *report.Result) error {
	data, err := json.Marshal(result.Data)
	if err != nil {
		return fmt.Errorf("marshal report result: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `UPDATE report_results SET status=?, is_partial=?, result_json=?, error_message=NULLIF(?, ''), updated_at=? WHERE id=?`, result.Status, result.Partial, data, result.ErrorMessage, result.UpdatedAt.UTC().Format(time.RFC3339Nano), result.ID)
	if err != nil {
		return fmt.Errorf("update report result %d: %w", result.ID, err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("report result %d rows affected: %w", result.ID, err)
	}
	if count == 0 {
		return fmt.Errorf("report result %d not found", result.ID)
	}
	return nil
}

func (r *SQLiteReportResultRepository) GetLatestByRun(ctx context.Context, runID int64) (report.Result, error) {
	var result report.Result
	var raw []byte
	var message sql.NullString
	var created, updated string
	err := r.db.QueryRowContext(ctx, `SELECT id, scenario_id, task_id, version, status, is_partial, result_json, COALESCE(error_message, ''), created_at, updated_at FROM report_results WHERE report_run_id=? ORDER BY version DESC LIMIT 1`, runID).Scan(&result.ID, &result.ScenarioID, &result.TaskID, &result.Version, &result.Status, &result.Partial, &raw, &message, &created, &updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return report.Result{}, fmt.Errorf("%w: run %d", ports.ErrReportResultNotFound, runID)
		}
		return report.Result{}, fmt.Errorf("get report result for run %d: %w", runID, err)
	}
	if err := json.Unmarshal(raw, &result.Data); err != nil {
		return report.Result{}, fmt.Errorf("decode report result %d: %w", result.ID, err)
	}
	result.RunID = runID
	result.ErrorMessage = message.String
	result.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return report.Result{}, err
	}
	result.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return result, err
}
