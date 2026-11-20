package ports

import (
	"context"
	"github.com/alex/assistent/internal/domain/classroom"
)

type ClassroomCourseRepository interface {
	Register(context.Context, classroom.Course) error
	List(context.Context) ([]classroom.Course, error)
	Disable(context.Context, string) error
}
