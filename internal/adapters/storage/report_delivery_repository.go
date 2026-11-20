package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
)

type SQLiteReportDeliveryRepository struct{ db *sql.DB }

func NewReportDeliveryRepository(db *sql.DB) ports.ReportDeliveryRepository {
	return &SQLiteReportDeliveryRepository{db: db}
}

func (r *SQLiteReportDeliveryRepository) Create(ctx context.Context, delivery *report.Delivery) error {
	err := r.db.QueryRowContext(ctx, `INSERT INTO report_deliveries (result_id, channel, destination, status, error_message, attempted_at, created_at) VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?) RETURNING id`, delivery.ResultID, delivery.Channel, delivery.Destination, delivery.Status, delivery.Error, nullableTime(delivery.AttemptedAt), delivery.CreatedAt.UTC().Format(time.RFC3339Nano)).Scan(&delivery.ID)
	if err != nil { return fmt.Errorf("insert report delivery: %w", err) }
	return nil
}

func (r *SQLiteReportDeliveryRepository) Update(ctx context.Context, delivery *report.Delivery) error {
	result, err := r.db.ExecContext(ctx, `UPDATE report_deliveries SET status=?, error_message=NULLIF(?, ''), attempted_at=? WHERE id=?`, delivery.Status, delivery.Error, nullableTime(delivery.AttemptedAt), delivery.ID)
	if err != nil { return fmt.Errorf("update report delivery %d: %w", delivery.ID, err) }
	count, err := result.RowsAffected()
	if err != nil { return fmt.Errorf("report delivery %d rows affected: %w", delivery.ID, err) }
	if count == 0 { return fmt.Errorf("report delivery %d not found", delivery.ID) }
	return nil
}

func (r *SQLiteReportDeliveryRepository) GetByResult(ctx context.Context, resultID int64) ([]report.Delivery, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, channel, destination, status, COALESCE(error_message, ''), attempted_at, created_at FROM report_deliveries WHERE result_id=? ORDER BY id`, resultID)
	if err != nil { return nil, fmt.Errorf("list report deliveries for result %d: %w", resultID, err) }
	defer rows.Close()
	items := make([]report.Delivery, 0)
	for rows.Next() {
		var item report.Delivery
		var attempted sql.NullString
		var created string
		if err := rows.Scan(&item.ID, &item.Channel, &item.Destination, &item.Status, &item.Error, &attempted, &created); err != nil { return nil, fmt.Errorf("scan report delivery: %w", err) }
		item.ResultID = resultID
		item.AttemptedAt, err = parseTime(attempted)
		if err != nil { return nil, err }
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil { return nil, err }
		items = append(items, item)
	}
	if err := rows.Err(); err != nil { return nil, fmt.Errorf("iterate report deliveries: %w", err) }
	return items, nil
}

func (r *SQLiteReportDeliveryRepository) GetByResultTarget(ctx context.Context, resultID int64, channel, destination string) (report.Delivery, error) {
	var item report.Delivery
	var attempted sql.NullString
	var created string
	err := r.db.QueryRowContext(ctx, `SELECT id, status, COALESCE(error_message, ''), attempted_at, created_at FROM report_deliveries WHERE result_id=? AND channel=? AND destination=?`, resultID, channel, destination).Scan(&item.ID, &item.Status, &item.Error, &attempted, &created)
	if err != nil {
		if err == sql.ErrNoRows { return report.Delivery{}, fmt.Errorf("%w: result %d channel %s destination %s", ports.ErrDeliveryNotFound, resultID, channel, destination) }
		return report.Delivery{}, fmt.Errorf("find report delivery: %w", err)
	}
	item.ResultID, item.Channel, item.Destination = resultID, channel, destination
	item.AttemptedAt, err = parseTime(attempted)
	if err != nil { return report.Delivery{}, err }
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return item, err
}
