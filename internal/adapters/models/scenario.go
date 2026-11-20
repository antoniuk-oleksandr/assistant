package models

import "database/sql"

// ScenarioRow is a SQLite row. Times are stored as ISO-8601 text in UTC.
type ScenarioRow struct {
	ID                 int64
	Name               string
	ImplementationType string
	Status             string
	CreatedAt          string
	UpdatedAt          string
	LastRunAt          sql.NullString
}
