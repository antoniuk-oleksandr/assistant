package ports

import (
	"context"

	"github.com/alex/assistent/internal/domain/teacher"
)

type ClassroomProfileSource interface {
	GetProfile(context.Context, string) (teacher.Profile, error)
}

type TeacherContactRepository interface {
	RegisterContact(context.Context, *teacher.Recipient, *teacher.Channel) error
}
