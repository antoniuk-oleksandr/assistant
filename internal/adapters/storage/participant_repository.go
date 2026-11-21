package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/task"
)

type SQLiteParticipantRosterRepository struct{ db *sql.DB }

func NewParticipantRosterRepository(db *sql.DB) ports.ParticipantRosterRepository {
	return &SQLiteParticipantRosterRepository{db: db}
}

func (r *SQLiteParticipantRosterRepository) Replace(ctx context.Context, taskID int64, participants []task.Participant) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin participant roster replacement: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_participants WHERE task_id=?`, taskID); err != nil {
		return fmt.Errorf("clear participant roster: %w", err)
	}
	for _, participant := range participants {
		if participant.ExternalID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_participants (task_id, external_user_id, name, email, updated_at) VALUES (?, ?, ?, ?, `+sqliteNowUTC+`)`, taskID, participant.ExternalID, participant.Name, participant.Email); err != nil {
			return fmt.Errorf("store participant %q: %w", participant.ExternalID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit participant roster: %w", err)
	}
	return nil
}

func (r *SQLiteParticipantRosterRepository) List(ctx context.Context, taskID int64) ([]task.Participant, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT external_user_id, name, email FROM task_participants WHERE task_id=? ORDER BY external_user_id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list participant roster: %w", err)
	}
	defer rows.Close()
	result := make([]task.Participant, 0)
	for rows.Next() {
		var item task.Participant
		if err := rows.Scan(&item.ExternalID, &item.Name, &item.Email); err != nil {
			return nil, fmt.Errorf("scan participant roster: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read participant roster: %w", err)
	}
	return result, nil
}
