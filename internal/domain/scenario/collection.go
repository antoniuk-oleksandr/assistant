package scenario

import "github.com/alex/assistent/internal/domain/submission"

// Collection is the scenario-owned input to a processing run. It deliberately
// contains normalized submissions rather than Classroom or provider models.
type Collection struct {
	ScenarioID int64
	TaskID     int64
	Items      []submission.Submission
}

type Outcome string

const (
	Partial   Outcome = "partial"
	Failed    Outcome = "failed"
	Completed Outcome = "completed"
)

// ProcessResult is returned by a scenario implementation after applying its
// own rules to a collection.
type ProcessResult struct {
	Outcome      Outcome
	Processed    int
	Failed       int
	ReportData   *ReportData
	ResultPath   string
	ErrorMessage string
}

// CollectionProcessor keeps scenario-specific rules out of polling and
// Classroom code.
type CollectionProcessor interface {
	Process(Collection) (ProcessResult, error)
}
