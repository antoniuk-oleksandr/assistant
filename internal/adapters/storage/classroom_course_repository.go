package storage

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/alex/assistent/internal/adapters/mappers"
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/ports"
)

type SQLiteClassroomCourseRepository struct{ db *sql.DB }

func NewClassroomCourseRepository(db *sql.DB) ports.ClassroomCourseRepository {
	return &SQLiteClassroomCourseRepository{db: db}
}

func (r *SQLiteClassroomCourseRepository) Register(ctx context.Context, course classroom.Course) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO classroom_courses (provider, external_course_id, source_reference, status, created_at, updated_at)
        VALUES ('google-classroom', ?, ?, ?, `+sqliteNowUTC+`, `+sqliteNowUTC+`)
        ON CONFLICT(external_course_id) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at`, course.ExternalID, course.SourceReference, classroom.Active)
	if err != nil {
		return fmt.Errorf("register classroom course %q: %w", course.ExternalID, err)
	}
	return nil
}

func (r *SQLiteClassroomCourseRepository) List(ctx context.Context) ([]classroom.Course, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, external_course_id, source_reference, status, created_at, updated_at FROM classroom_courses ORDER BY external_course_id`)
	if err != nil {
		return nil, fmt.Errorf("list classroom courses: %w", err)
	}
	defer rows.Close()
	result := make([]classroom.Course, 0)
	for rows.Next() {
		var row models.ClassroomCourseRow
		if err := rows.Scan(&row.ID, &row.ExternalID, &row.SourceReference, &row.Status, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan classroom course: %w", err)
		}
		course, err := mappers.ToClassroomCourse(row)
		if err != nil {
			return nil, fmt.Errorf("map classroom course %q: %w", row.ExternalID, err)
		}
		result = append(result, course)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read classroom courses: %w", err)
	}
	return result, nil
}

func (r *SQLiteClassroomCourseRepository) Disable(ctx context.Context, externalID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE classroom_courses SET status = ?, updated_at = CASE WHEN status <> ? THEN `+sqliteNowUTC+` ELSE updated_at END WHERE external_course_id = ?`, classroom.Disabled, classroom.Disabled, externalID)
	if err != nil {
		return fmt.Errorf("disable classroom course %q: %w", externalID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("disable classroom course %q: %w", externalID, err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: %q", classroom.ErrInvalidCourseReference, externalID)
	}
	return nil
}
