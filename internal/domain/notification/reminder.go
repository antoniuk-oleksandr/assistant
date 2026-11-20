package notification

import "time"

type Status string

const (
	Pending    Status = "pending"
	Sent       Status = "sent"
	Failed     Status = "failed"
	DeadLetter Status = "dead_letter"
)

type Reminder struct {
	ID            int64
	TaskID        int64
	PersonID      string
	Channel       string
	Destination   string
	Kind          string
	Status        Status
	Error         string
	Attempts      int
	NextAttemptAt *time.Time
	AttemptedAt   *time.Time
	CreatedAt     time.Time
}
