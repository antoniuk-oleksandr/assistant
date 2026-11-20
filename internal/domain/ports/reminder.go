package ports

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/domain/notification"
)

var ErrReminderNotFound = errors.New("reminder not found")

type ReminderTarget struct {
	PersonID    string
	Channel     string
	Destination string
}

type ReminderRepository interface {
	Create(context.Context, *notification.Reminder) error
	Update(context.Context, *notification.Reminder) error
	Get(context.Context, int64, string, string, string) (notification.Reminder, error)
	GetByID(context.Context, int64) (notification.Reminder, error)
	ListByTask(context.Context, int64) ([]notification.Reminder, error)
	ListRetryable(context.Context, time.Time) ([]notification.Reminder, error)
}

type ReminderProvider interface {
	Name() string
	SendReminder(context.Context, notification.Reminder) error
}
