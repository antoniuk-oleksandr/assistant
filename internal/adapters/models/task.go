package models

type TaskRow struct {
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
	Phase             string
	PhaseAt           string
	Status            string
}

type TaskBindingRow struct {
	ScenarioID        int64
	ScenarioName      string
	ScenarioType      string
	TaskID            int64
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
	Phase             string
	PhaseAt           string
	TaskStatus        string
	Status            string
	BoundAt           string
	UnboundAt         string
}
