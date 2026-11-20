package notification

import "time"

type Status string

const (
	Pending Status = "pending"
	Sent    Status = "sent"
	Failed  Status = "failed"
)

type Reminder struct {
	ID          int64
	TaskID      int64
	PersonID    string
	Channel     string
	Destination string
	Kind        string
	Status      Status
	Error       string
	AttemptedAt *time.Time
	CreatedAt   time.Time
}
