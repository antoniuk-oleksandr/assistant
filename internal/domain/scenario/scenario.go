package scenario

import (
	"errors"
	"fmt"
	"time"
)

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
