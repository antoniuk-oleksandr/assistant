package storage

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
	"time"
)

type SQLiteTeacherRepository struct{ db *sql.DB }

func NewTeacherRepository(db *sql.DB) ports.TeacherRepository {
	return &SQLiteTeacherRepository{db: db}
}

func NewTelegramPairingRepository(db *sql.DB) ports.TelegramPairingRepository {
	return &SQLiteTeacherRepository{db: db}
}

func (r *SQLiteTeacherRepository) Create(ctx context.Context, item *teacher.Recipient) error {
	now := item.CreatedAt.UTC().Format(time.RFC3339Nano)
	err := r.db.QueryRowContext(ctx, `INSERT INTO teachers (name,email,classroom_id,status,created_at,updated_at) VALUES (?,?,?,?,?,?) RETURNING id`, item.Name, item.Email, item.ClassroomID, item.Status, now, now).Scan(&item.ID)
	if err != nil {
		return fmt.Errorf("insert teacher: %w", err)
	}
	return nil
}
func (r *SQLiteTeacherRepository) Update(ctx context.Context, item *teacher.Recipient) error {
	res, err := r.db.ExecContext(ctx, `UPDATE teachers SET name=?, email=?, classroom_id=?, status=?, updated_at=? WHERE id=?`, item.Name, item.Email, item.ClassroomID, item.Status, item.UpdatedAt.UTC().Format(time.RFC3339Nano), item.ID)
	if err != nil {
		return fmt.Errorf("update teacher %d: %w", item.ID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("teacher %d not found", item.ID)
	}
	return nil
}
func (r *SQLiteTeacherRepository) Get(ctx context.Context, id int64) (teacher.Recipient, error) {
	var v teacher.Recipient
	var created, updated string
	err := r.db.QueryRowContext(ctx, `SELECT id,name,email,COALESCE(classroom_id,''),status,created_at,updated_at FROM teachers WHERE id=?`, id).Scan(&v.ID, &v.Name, &v.Email, &v.ClassroomID, &v.Status, &created, &updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return v, fmt.Errorf("%w: %d", ports.ErrTeacherNotFound, id)
		}
		return v, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return v, nil
}
func (r *SQLiteTeacherRepository) List(ctx context.Context) ([]teacher.Recipient, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,email,COALESCE(classroom_id,''),status,created_at,updated_at FROM teachers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []teacher.Recipient{}
	for rows.Next() {
		var v teacher.Recipient
		var c, u string
		if err := rows.Scan(&v.ID, &v.Name, &v.Email, &v.ClassroomID, &v.Status, &c, &u); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, u)
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *SQLiteTeacherRepository) CreateChannel(ctx context.Context, item *teacher.Channel) error {
	err := r.db.QueryRowContext(ctx, `INSERT INTO notification_channels (teacher_id,kind,destination,status,created_at,updated_at) VALUES (?,?,?,?,?,?) RETURNING id`, item.RecipientID, item.Kind, item.Destination, item.Status, item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano)).Scan(&item.ID)
	if err != nil {
		return fmt.Errorf("insert notification channel: %w", err)
	}
	return nil
}
func (r *SQLiteTeacherRepository) UpdateChannel(ctx context.Context, item *teacher.Channel) error {
	res, err := r.db.ExecContext(ctx, `UPDATE notification_channels SET kind=?,destination=?,status=?,updated_at=? WHERE id=?`, item.Kind, item.Destination, item.Status, item.UpdatedAt.UTC().Format(time.RFC3339Nano), item.ID)
	if err != nil {
		return fmt.Errorf("update notification channel %d: %w", item.ID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("notification channel %d not found", item.ID)
	}
	return nil
}
func (r *SQLiteTeacherRepository) ListActiveChannels(ctx context.Context, id int64) ([]teacher.Channel, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,teacher_id,kind,destination,status,created_at,updated_at FROM notification_channels WHERE teacher_id=? AND status='active' ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []teacher.Channel{}
	for rows.Next() {
		var v teacher.Channel
		var c, u string
		if err := rows.Scan(&v.ID, &v.RecipientID, &v.Kind, &v.Destination, &v.Status, &c, &u); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, u)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *SQLiteTeacherRepository) CreateTelegramPairing(ctx context.Context, item *teacher.TelegramPairing) error {
	err := r.db.QueryRowContext(ctx, `INSERT INTO telegram_pairings (teacher_id, token_hash, expires_at) VALUES (?, ?, ?) RETURNING id`, item.RecipientID, item.TokenHash, item.ExpiresAt.UTC().Format(time.RFC3339Nano)).Scan(&item.ID)
	if err != nil {
		return fmt.Errorf("insert Telegram pairing: %w", err)
	}
	return nil
}

func (r *SQLiteTeacherRepository) GetTelegramPairing(ctx context.Context, tokenHash string) (teacher.TelegramPairing, error) {
	var item teacher.TelegramPairing
	var expires, used string
	err := r.db.QueryRowContext(ctx, `SELECT id,teacher_id,token_hash,expires_at,COALESCE(used_at,'') FROM telegram_pairings WHERE token_hash=?`, tokenHash).Scan(&item.ID, &item.RecipientID, &item.TokenHash, &expires, &used)
	if err != nil {
		return item, err
	}
	item.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expires)
	if used != "" {
		value, _ := time.Parse(time.RFC3339Nano, used)
		item.UsedAt = &value
	}
	return item, nil
}

func (r *SQLiteTeacherRepository) ConsumeTelegramPairing(ctx context.Context, id int64, usedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE telegram_pairings SET used_at=? WHERE id=? AND used_at IS NULL`, usedAt.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("consume Telegram pairing: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ports.ErrTeacherNotFound
	}
	return nil
}
