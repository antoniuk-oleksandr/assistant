package ports

import (
	"context"

	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

// SubmissionSource is the provider boundary. Provider response models must be
// mapped to these domain types before entering the application.
type SubmissionSource interface {
	ListTasks(context.Context, string) ([]task.Task, error)
	ListSubmissions(context.Context, string) ([]submission.Submission, error)
}

type SubmissionRepository interface {
	Upsert(context.Context, submission.Submission) error
	ListByTask(context.Context, int64) ([]submission.Submission, error)
}
