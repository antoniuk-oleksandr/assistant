package teacher

import "time"

type Status string

const (
	Active   Status = "active"
	Disabled Status = "disabled"
)

type Recipient struct {
	ID                       int64
	Name, Email, ClassroomID string
	Status                   Status
	CreatedAt, UpdatedAt     time.Time
}
type Channel struct {
	ID, RecipientID      int64
	Kind, Destination    string
	Status               Status
	CreatedAt, UpdatedAt time.Time
}
