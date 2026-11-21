package ports

import (
	"context"
	"time"

	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

// SubmissionSource is the provider boundary. Provider response models must be
// mapped to these domain types before entering the application.
type SubmissionSource interface {
	ListTasks(context.Context, string) ([]task.Task, error)
	ListSubmissions(context.Context, string) ([]submission.Submission, error)
}

// TaskCourseRegistrar supplies persisted task context to sources that fetch
// submissions without first listing coursework.
type TaskCourseRegistrar interface {
	RegisterTaskCourse(string, string)
}

type IncrementalSubmissionSource interface {
	SubmissionSource
	ListTasksSince(context.Context, string, time.Time) ([]task.Task, error)
}

// MaterialRetriever enriches one attachment and records its retrieval outcome
// on the material, independently of the Classroom submission state.
type MaterialRetriever interface {
	Retrieve(context.Context, submission.Material) submission.Material
}

type SubmissionRepository interface {
	Upsert(context.Context, submission.Submission) error
	ListByTask(context.Context, int64) ([]submission.Submission, error)
}
