package storage

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/alex/assistent/internal/domain/history"
	"github.com/alex/assistent/internal/domain/ports"
	"time"
)

type SQLiteHistoryRepository struct{ db *sql.DB }

func NewHistoryRepository(db *sql.DB) ports.HistoryRepository {
	return &SQLiteHistoryRepository{db: db}
}
func (r *SQLiteHistoryRepository) ListHistory(ctx context.Context) ([]history.Entry, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT rr.id,rr.scenario_id,rr.task_id,rr.status,rr.is_partial,COALESCE(rr.error_message,''),COALESCE(rr.result_path,''),COALESCE((SELECT status FROM report_results res WHERE res.report_run_id=rr.id ORDER BY version DESC LIMIT 1),''), (SELECT count(*) FROM report_deliveries d WHERE d.result_id=(SELECT id FROM report_results res2 WHERE res2.report_run_id=rr.id ORDER BY version DESC LIMIT 1)), (SELECT count(*) FROM report_deliveries d2 WHERE d2.result_id=(SELECT id FROM report_results res3 WHERE res3.report_run_id=rr.id ORDER BY version DESC LIMIT 1) AND d2.status IN ('failed','pending')), (SELECT count(*) FROM reminders rm WHERE rm.task_id=rr.task_id), (SELECT count(*) FROM reminders rm2 WHERE rm2.task_id=rr.task_id AND rm2.status IN ('failed','dead_letter')), rr.created_at FROM report_runs rr ORDER BY rr.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list processing history: %w", err)
	}
	defer rows.Close()
	out := []history.Entry{}
	for rows.Next() {
		var v history.Entry
		var created string
		if err := rows.Scan(&v.RunID, &v.ScenarioID, &v.TaskID, &v.RunStatus, &v.Partial, &v.ErrorMessage, &v.ResultPath, &v.ResultStatus, &v.DeliveryCount, &v.FailedDeliveryCount, &v.ReminderCount, &v.FailedReminderCount, &created); err != nil {
			return nil, fmt.Errorf("scan processing history: %w", err)
		}
		v.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
