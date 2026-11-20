package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
)

type SQLitePollingStateRepository struct{ db *sql.DB }

func NewPollingStateRepository(db *sql.DB) ports.PollingStateRepository {
	return &SQLitePollingStateRepository{db: db}
}

func (r *SQLitePollingStateRepository) LastCheckpoint(ctx context.Context) (time.Time, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `SELECT last_check_timestamp FROM poll_state WHERE id=1`).Scan(&value)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("read polling checkpoint: %w", err)
	}
	checkpoint, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse polling checkpoint: %w", err)
	}
	return checkpoint, nil
}

func (r *SQLitePollingStateRepository) SaveCheckpoint(ctx context.Context, checkpoint time.Time) error {
	value := checkpoint.UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(ctx, `INSERT INTO poll_state (id, last_check_timestamp, updated_at) VALUES (1, ?, ?) ON CONFLICT(id) DO UPDATE SET last_check_timestamp=excluded.last_check_timestamp, updated_at=excluded.updated_at`, value, value)
	if err != nil {
		return fmt.Errorf("save polling checkpoint: %w", err)
	}
	return nil
}
