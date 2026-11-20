package mappers

import (
	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/presentation/dto"
	"time"
)

func ClassroomCourseToDTO(item application.ClassroomCourseResult) dto.ClassroomCourse {
	return dto.ClassroomCourse{ExternalID: item.ExternalID, SourceReference: item.SourceReference, Status: item.Status, CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: item.UpdatedAt.UTC().Format(time.RFC3339Nano)}
}
