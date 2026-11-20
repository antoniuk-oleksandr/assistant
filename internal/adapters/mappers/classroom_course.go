package mappers

import (
	"fmt"
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/classroom"
	"time"
)

func ToClassroomCourse(row models.ClassroomCourseRow) (classroom.Course, error) {
	created, err := time.Parse(time.RFC3339Nano, row.CreatedAt)
	if err != nil {
		return classroom.Course{}, fmt.Errorf("parse course created_at: %w", err)
	}
	updated, err := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	if err != nil {
		return classroom.Course{}, fmt.Errorf("parse course updated_at: %w", err)
	}
	return classroom.Course{ID: row.ID, ExternalID: row.ExternalID, SourceReference: row.SourceReference, Status: classroom.Status(row.Status), CreatedAt: created, UpdatedAt: updated}, nil
}
