package ports

import "context"

type MissingSubmitter struct {
	TaskID            int64
	ExternalUserID    string
	Name              string
	Email             string
	NotificationState string
}

type MissingSubmitterRepository interface {
	Reconcile(context.Context, int64, []MissingSubmitter) error
	List(context.Context, int64) ([]MissingSubmitter, error)
}
