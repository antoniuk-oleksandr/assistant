package models

type TaskRow struct {
	ID                int64
	Provider          string
	ExternalID        string
	CourseID          string
	Title             string
	Description       string
	CreatedAtExternal string
	DueAt             string
	DiscoveredAt      string
	LastSeenAt        string
	Status            string
}

type TaskBindingRow struct {
	ScenarioID   int64
	ScenarioName string
	ScenarioType string
	TaskID       int64
	Provider     string
	ExternalID   string
	CourseID     string
	Title        string
	Description  string
	DueAt        string
	TaskStatus   string
	Status       string
	BoundAt      string
	UnboundAt    string
}
