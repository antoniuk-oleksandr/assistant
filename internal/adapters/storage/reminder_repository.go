package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
)

type SQLiteReminderRepository struct{ db *sql.DB }

func NewReminderRepository(db *sql.DB) ports.ReminderRepository {
	return &SQLiteReminderRepository{db: db}
}

func (r *SQLiteReminderRepository) Create(ctx context.Context, item *notification.Reminder) error {
	err := r.db.QueryRowContext(ctx, `INSERT INTO reminders (task_id, person_id, channel, destination, kind, subject, message, due_at, classroom_link, status, error_message, attempts, next_attempt_at, attempted_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?) RETURNING id`, item.TaskID, item.PersonID, item.Channel, item.Destination, item.Kind, item.Subject, item.Message, item.DueAt, item.ClassroomLink, item.Status, item.Error, item.Attempts, nullableTime(item.NextAttemptAt), nullableTime(item.AttemptedAt), item.CreatedAt.UTC().Format(time.RFC3339Nano)).Scan(&item.ID)
	if err != nil {
		return fmt.Errorf("insert reminder: %w", err)
	}
	return nil
}

func (r *SQLiteReminderRepository) Update(ctx context.Context, item *notification.Reminder) error {
	result, err := r.db.ExecContext(ctx, `UPDATE reminders SET status=?, error_message=NULLIF(?, ''), attempts=?, next_attempt_at=?, attempted_at=? WHERE id=?`, item.Status, item.Error, item.Attempts, nullableTime(item.NextAttemptAt), nullableTime(item.AttemptedAt), item.ID)
	if err != nil {
		return fmt.Errorf("update reminder %d: %w", item.ID, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("reminder %d rows affected: %w", item.ID, err)
	}
	if count == 0 {
		return fmt.Errorf("reminder %d not found", item.ID)
	}
	return nil
}

func (r *SQLiteReminderRepository) Get(ctx context.Context, taskID int64, personID, channel, kind string) (notification.Reminder, error) {
	var item notification.Reminder
	var attempted, next sql.NullString
	var created string
	err := r.db.QueryRowContext(ctx, `SELECT id, destination, subject, message, due_at, classroom_link, status, COALESCE(error_message, ''), attempts, next_attempt_at, attempted_at, created_at FROM reminders WHERE task_id=? AND person_id=? AND channel=? AND kind=?`, taskID, personID, channel, kind).Scan(&item.ID, &item.Destination, &item.Subject, &item.Message, &item.DueAt, &item.ClassroomLink, &item.Status, &item.Error, &item.Attempts, &next, &attempted, &created)
	if err != nil {
		if err == sql.ErrNoRows {
			return notification.Reminder{}, fmt.Errorf("%w: task %d person %s", ports.ErrReminderNotFound, taskID, personID)
		}
		return notification.Reminder{}, fmt.Errorf("find reminder: %w", err)
	}
	item.TaskID, item.PersonID, item.Channel, item.Kind = taskID, personID, channel, kind
	item.AttemptedAt, err = parseTime(attempted)
	if err != nil {
		return notification.Reminder{}, err
	}
	item.NextAttemptAt, err = parseTime(next)
	if err != nil {
		return notification.Reminder{}, err
	}
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return item, err
}

func (r *SQLiteReminderRepository) ListByTask(ctx context.Context, taskID int64) ([]notification.Reminder, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, person_id, channel, destination, kind, subject, message, due_at, classroom_link, status, COALESCE(error_message, ''), attempts, next_attempt_at, attempted_at, created_at FROM reminders WHERE task_id=? ORDER BY id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list reminders for task %d: %w", taskID, err)
	}
	defer rows.Close()
	items := make([]notification.Reminder, 0)
	for rows.Next() {
		var item notification.Reminder
		var attempted, next sql.NullString
		var created string
		if err := rows.Scan(&item.ID, &item.PersonID, &item.Channel, &item.Destination, &item.Kind, &item.Subject, &item.Message, &item.DueAt, &item.ClassroomLink, &item.Status, &item.Error, &item.Attempts, &next, &attempted, &created); err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		item.TaskID = taskID
		item.AttemptedAt, err = parseTime(attempted)
		if err != nil {
			return nil, err
		}
		item.NextAttemptAt, err = parseTime(next)
		if err != nil {
			return nil, err
		}
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reminders: %w", err)
	}
	return items, nil
}

func (r *SQLiteReminderRepository) GetByID(ctx context.Context, id int64) (notification.Reminder, error) {
	var item notification.Reminder
	var attempted, next sql.NullString
	var created string
	err := r.db.QueryRowContext(ctx, `SELECT task_id, person_id, channel, destination, kind, subject, message, due_at, classroom_link, status, COALESCE(error_message, ''), attempts, next_attempt_at, attempted_at, created_at FROM reminders WHERE id=?`, id).Scan(&item.TaskID, &item.PersonID, &item.Channel, &item.Destination, &item.Kind, &item.Subject, &item.Message, &item.DueAt, &item.ClassroomLink, &item.Status, &item.Error, &item.Attempts, &next, &attempted, &created)
	if err != nil {
		if err == sql.ErrNoRows {
			return notification.Reminder{}, ports.ErrReminderNotFound
		}
		return notification.Reminder{}, fmt.Errorf("get reminder %d: %w", id, err)
	}
	item.ID = id
	item.NextAttemptAt, err = parseTime(next)
	if err != nil {
		return notification.Reminder{}, err
	}
	item.AttemptedAt, err = parseTime(attempted)
	if err != nil {
		return notification.Reminder{}, err
	}
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return item, err
}

func (r *SQLiteReminderRepository) ListRetryable(ctx context.Context, now time.Time) ([]notification.Reminder, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM reminders WHERE status='failed' AND (next_attempt_at IS NULL OR next_attempt_at <= ?) ORDER BY id`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("list retryable reminders: %w", err)
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]notification.Reminder, 0, len(ids))
	for _, id := range ids {
		item, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
