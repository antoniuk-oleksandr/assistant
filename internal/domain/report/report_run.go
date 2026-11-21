package report

import (
	"errors"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/scenario"
)

var ErrInvalidTransition = errors.New("InvalidReportRunTransition")
var ErrInvalidOutcome = errors.New("InvalidReportOutcome")

type Status string

const (
	Scheduled          Status = "scheduled"
	Collecting         Status = "collecting"
	Processing         Status = "processing"
	WaitingForDeadline Status = "waiting_for_deadline"
	Completed          Status = "completed"
	Failed             Status = "failed"
)

type Run struct {
	ID               int64
	ScenarioID       int64
	TaskID           int64
	Status           Status
	StartedAt        *time.Time
	FinishedAt       *time.Time
	Partial          bool
	ResultPath       string
	ErrorMessage     string
	InputFingerprint string
	CreatedAt        time.Time
}

type Output struct {
	Format    string
	Path      string
	Version   int
	CreatedAt time.Time
}

func NewRun(scenarioID, taskID int64, now time.Time) Run {
	return Run{ScenarioID: scenarioID, TaskID: taskID, Status: Scheduled, CreatedAt: now.UTC()}
}

func (r *Run) Transition(next Status, now time.Time) error {
	if !validTransition(r.Status, next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, r.Status, next)
	}
	if r.StartedAt == nil && next == Collecting {
		value := now.UTC()
		r.StartedAt = &value
	}
	if next == Completed || next == Failed {
		value := now.UTC()
		r.FinishedAt = &value
	}
	r.Status = next
	return nil
}

func (r *Run) ApplyResult(result scenario.ProcessResult, now time.Time) error {
	if result.Outcome != scenario.Partial && result.Outcome != scenario.Failed && result.Outcome != scenario.Completed {
		return fmt.Errorf("%w: %q", ErrInvalidOutcome, result.Outcome)
	}
	if result.ResultPath != "" {
		r.ResultPath = result.ResultPath
	}
	if result.ErrorMessage != "" {
		r.ErrorMessage = result.ErrorMessage
	}
	r.Partial = result.Outcome == scenario.Partial
	next := Completed
	if result.Outcome == scenario.Failed {
		next = Failed
	}
	return r.Transition(next, now)
}

func validTransition(from, to Status) bool {
	if from == to {
		return true
	}
	switch from {
	case Scheduled:
		return to == Collecting || to == Failed
	case Collecting:
		return to == Processing || to == Failed
	case Processing:
		return to == Completed || to == Failed || to == WaitingForDeadline
	case WaitingForDeadline:
		return to == Processing || to == Completed || to == Failed
	default:
		return false
	}
}
