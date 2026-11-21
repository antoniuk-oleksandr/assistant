package storage

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/alex/assistent/internal/domain/ports"
)

type SQLiteMissingSubmitterRepository struct{ db *sql.DB }

func NewMissingSubmitterRepository(db *sql.DB) ports.MissingSubmitterRepository {
	return &SQLiteMissingSubmitterRepository{db: db}
}

func (r *SQLiteMissingSubmitterRepository) Reconcile(ctx context.Context, taskID int64, items []ports.MissingSubmitter) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin missing submitter reconciliation: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM missing_submitters WHERE task_id=?`, taskID); err != nil {
		return fmt.Errorf("clear missing submitters: %w", err)
	}
	for _, item := range items {
		if item.ExternalUserID == "" {
			continue
		}
		if item.NotificationState == "" {
			item.NotificationState = "pending"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO missing_submitters (task_id, external_user_id, name, email, notification_state, updated_at) VALUES (?, ?, ?, ?, ?, `+sqliteNowUTC+`)`, taskID, item.ExternalUserID, item.Name, item.Email, item.NotificationState); err != nil {
			return fmt.Errorf("store missing submitter %q: %w", item.ExternalUserID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit missing submitters: %w", err)
	}
	return nil
}

func (r *SQLiteMissingSubmitterRepository) List(ctx context.Context, taskID int64) ([]ports.MissingSubmitter, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT task_id, external_user_id, name, email, notification_state FROM missing_submitters WHERE task_id=? ORDER BY external_user_id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list missing submitters: %w", err)
	}
	defer rows.Close()
	result := make([]ports.MissingSubmitter, 0)
	for rows.Next() {
		var item ports.MissingSubmitter
		if err := rows.Scan(&item.TaskID, &item.ExternalUserID, &item.Name, &item.Email, &item.NotificationState); err != nil {
			return nil, fmt.Errorf("scan missing submitter: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read missing submitters: %w", err)
	}
	return result, nil
}
