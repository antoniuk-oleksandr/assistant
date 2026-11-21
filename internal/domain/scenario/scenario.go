package scenario

import (
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a scenario is not compiled or registered.
var ErrNotFound = errors.New("ScenarioNotFound")

// NotFound identifies the missing compiled implementation or SQLite registration.
func NotFound(name string) error {
	return fmt.Errorf("%w: %q", ErrNotFound, name)
}

type Status string

const (
	Active   Status = "active"
	Disabled Status = "disabled"
)

type Implementation struct {
	Name string
	Type string
}

type Scenario struct {
	ID             int64
	Implementation Implementation
	Status         Status
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastRunAt      *time.Time
}

type CourseBinding struct {
	ScenarioName string
	ScenarioType string
	CourseID     string
	Status       string
}

type CourseDataField struct {
	ScenarioName string
	CourseID     string
	Name         string
	Value        string
}
