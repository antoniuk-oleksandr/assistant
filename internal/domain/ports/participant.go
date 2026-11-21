package ports

import (
	"context"

	"github.com/alex/assistent/internal/domain/task"
)

type ParticipantRosterRepository interface {
	Replace(context.Context, int64, []task.Participant) error
	List(context.Context, int64) ([]task.Participant, error)
}

type ParticipantSource interface {
	ListParticipants(context.Context, string) ([]task.Participant, error)
}
