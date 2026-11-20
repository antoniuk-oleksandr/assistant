package report

import "time"

type DeliveryStatus string

const (
	DeliveryPending DeliveryStatus = "pending"
	DeliverySent    DeliveryStatus = "sent"
	DeliveryFailed  DeliveryStatus = "failed"
)

type Delivery struct {
	ID          int64
	ResultID    int64
	Channel     string
	Destination string
	Status      DeliveryStatus
	Error       string
	AttemptedAt *time.Time
	CreatedAt   time.Time
}
