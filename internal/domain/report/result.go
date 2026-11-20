package report

import (
	"time"

	"github.com/alex/assistent/internal/domain/scenario"
)

type ResultStatus string

const (
	ResultProcessing ResultStatus = "processing"
	ResultPartial    ResultStatus = "partial"
	ResultCompleted  ResultStatus = "completed"
	ResultFailed     ResultStatus = "failed"
)

// Result is the versioned, persisted business result of processing a collection.
// The renderer is responsible for external representations; this entity keeps
// the structured result and its lifecycle queryable locally.
type Result struct {
	ID           int64
	RunID        int64
	ScenarioID   int64
	TaskID       int64
	Version      int
	Status       ResultStatus
	Partial      bool
	Data         scenario.ReportData
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewResult(runID, scenarioID, taskID int64, data scenario.ReportData, now time.Time) Result {
	return Result{RunID: runID, ScenarioID: scenarioID, TaskID: taskID, Version: 1, Status: ResultProcessing, Data: data, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
}

func (r *Result) Complete(status ResultStatus, now time.Time, message string) {
	r.Status = status
	r.Partial = status == ResultPartial
	r.ErrorMessage = message
	r.UpdatedAt = now.UTC()
}
