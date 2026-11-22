package ports

import (
	"context"
	"errors"
	"github.com/alex/assistent/internal/domain/teacher"
	"time"
)

var ErrTeacherNotFound = errors.New("teacher not found")

type ClassroomProfileSource interface {
	GetProfile(context.Context, string) (teacher.Profile, error)
}

type TeacherRepository interface {
	Create(context.Context, *teacher.Recipient) error
	Update(context.Context, *teacher.Recipient) error
	Get(context.Context, int64) (teacher.Recipient, error)
	List(context.Context) ([]teacher.Recipient, error)
	CreateChannel(context.Context, *teacher.Channel) error
	UpdateChannel(context.Context, *teacher.Channel) error
	ListActiveChannels(context.Context, int64) ([]teacher.Channel, error)
}

type TelegramPairingRepository interface {
	CreateTelegramPairing(context.Context, *teacher.TelegramPairing) error
	GetTelegramPairing(context.Context, string) (teacher.TelegramPairing, error)
	ConsumeTelegramPairing(context.Context, int64, time.Time) error
}
