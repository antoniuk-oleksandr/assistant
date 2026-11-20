package task

import "errors"

var ErrNotFound = errors.New("TaskNotFound")
var ErrAmbiguous = errors.New("TaskAmbiguous")

type Status string

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
	DueAt             string
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
