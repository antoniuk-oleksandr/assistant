package history

import "time"

type Entry struct {
	RunID                              int64
	ScenarioID, TaskID                 int64
	RunStatus                          string
	ResultStatus                       string
	Partial                            bool
	ErrorMessage                       string
	ResultPath                         string
	DeliveryCount, FailedDeliveryCount int
	ReminderCount, FailedReminderCount int
	CreatedAt                          time.Time
}
