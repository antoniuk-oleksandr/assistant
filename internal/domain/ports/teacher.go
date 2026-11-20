package ports

import (
	"context"
	"errors"
	"github.com/alex/assistent/internal/domain/teacher"
)

var ErrTeacherNotFound = errors.New("teacher not found")

type TeacherRepository interface {
	Create(context.Context, *teacher.Recipient) error
	Update(context.Context, *teacher.Recipient) error
	Get(context.Context, int64) (teacher.Recipient, error)
	List(context.Context) ([]teacher.Recipient, error)
	CreateChannel(context.Context, *teacher.Channel) error
	UpdateChannel(context.Context, *teacher.Channel) error
	ListActiveChannels(context.Context, int64) ([]teacher.Channel, error)
}
