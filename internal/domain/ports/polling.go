package ports

import (
	"context"
	"time"
)

// PollingStateRepository stores the last completed polling checkpoint.
type PollingStateRepository interface {
	LastCheckpoint(context.Context) (time.Time, error)
	SaveCheckpoint(context.Context, time.Time) error
}
