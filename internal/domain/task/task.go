package task

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("TaskNotFound")
var ErrAmbiguous = errors.New("TaskAmbiguous")

type Status string

type Phase string

const (
	PhaseDiscovery       Phase = "discovery"
	PhaseThreeDaysBefore Phase = "three_days_before"
	PhaseOneDayBefore    Phase = "one_day_before"
	PhaseDeadlineReached Phase = "deadline_reached"
	PhaseCompleted       Phase = "completed"
)

const (
	Discovered Status = "discovered"
	Bound      Status = "bound"
	Closed     Status = "closed"
	Ignored    Status = "ignored"
)

type Task struct {
	ID                int64
	Provider          string
	ExternalID        string
	CourseID          string
	Title             string
	Description       string
	CreatedAtExternal string
	UpdatedAtExternal string
	DueDate           string
	DueTime           string
	TimeZone          string
	DueAt             string
	DiscoveredAt      string
	LastSeenAt        string
	Phase             Phase
	PhaseAt           string
	Status            Status
}

type Binding struct {
	ScenarioName string
	ScenarioType string
	ScenarioID   int64
	Task         Task
	Status       string
	BoundAt      string
	UnboundAt    string
}

type DeadlineWindows struct {
	PreliminaryBefore time.Duration
	RefreshBefore     time.Duration
}

var DefaultDeadlineWindows = DeadlineWindows{PreliminaryBefore: 72 * time.Hour, RefreshBefore: 24 * time.Hour}

// DeterminePhase calculates the current deadline phase from the task's
// persisted due metadata and the supplied clock value.
func DeterminePhase(item Task, now time.Time) Phase {
	return DeterminePhaseWithWindows(item, now, DefaultDeadlineWindows)
}

func DeterminePhaseWithWindows(item Task, now time.Time, windows DeadlineWindows) Phase {
	if windows.PreliminaryBefore <= 0 {
		windows.PreliminaryBefore = DefaultDeadlineWindows.PreliminaryBefore
	}
	if windows.RefreshBefore <= 0 || windows.RefreshBefore > windows.PreliminaryBefore {
		windows.RefreshBefore = DefaultDeadlineWindows.RefreshBefore
	}
	deadline, ok := deadline(item)
	if !ok {
		return PhaseDiscovery
	}
	if now.Before(deadline.Add(-windows.PreliminaryBefore)) {
		return PhaseDiscovery
	}
	if now.Before(deadline.Add(-windows.RefreshBefore)) {
		return PhaseThreeDaysBefore
	}
	if now.Before(deadline) {
		return PhaseOneDayBefore
	}
	if now.Equal(deadline) {
		return PhaseDeadlineReached
	}
	return PhaseCompleted
}

func deadline(item Task) (time.Time, bool) {
	location := time.UTC
	if item.TimeZone != "" {
		loaded, err := time.LoadLocation(item.TimeZone)
		if err == nil {
			location = loaded
		}
	}
	if item.DueDate != "" {
		value := item.DueDate
		if item.DueTime != "" {
			value += " " + item.DueTime
		}
		layout := "2006-01-02"
		if item.DueTime != "" {
			layout = "2006-01-02 15:04:05"
		}
		parsed, err := time.ParseInLocation(layout, value, location)
		return parsed, err == nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
		parsed, err := time.ParseInLocation(layout, item.DueAt, location)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
