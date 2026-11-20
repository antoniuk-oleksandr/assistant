package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
)

type SQLiteReportOutputRepository struct{ db *sql.DB }

func NewReportOutputRepository(db *sql.DB) ports.ReportOutputRepository {
	return &SQLiteReportOutputRepository{db: db}
}

func (r *SQLiteReportOutputRepository) Create(ctx context.Context, output ports.ReportOutput) error {
	if output.Version <= 0 {
		output.Version = 1
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO report_outputs (report_run_id, format, path, version, created_at) VALUES (?, ?, ?, ?, ?)`, output.ReportRunID, output.Format, output.Path, output.Version, output.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert report output: %w", err)
	}
	return nil
}
