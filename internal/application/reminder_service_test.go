package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

type reminderRepository struct {
	items                []notification.Reminder
	lookupErr, updateErr error
}

func (r *reminderRepository) Create(_ context.Context, item *notification.Reminder) error {
	item.ID = int64(len(r.items) + 1)
	r.items = append(r.items, *item)
	return nil
}
func (r *reminderRepository) Update(_ context.Context, item *notification.Reminder) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	for i := range r.items {
		if r.items[i].ID == item.ID {
			r.items[i] = *item
		}
	}
	return nil
}
func (r *reminderRepository) Get(_ context.Context, taskID int64, personID, channel, kind string) (notification.Reminder, error) {
	if r.lookupErr != nil {
		return notification.Reminder{}, r.lookupErr
	}
	for _, item := range r.items {
		if item.TaskID == taskID && item.PersonID == personID && item.Channel == channel && item.Kind == kind {
			return item, nil
		}
	}
	return notification.Reminder{}, ports.ErrReminderNotFound
}
func (r *reminderRepository) ListByTask(context.Context, int64) ([]notification.Reminder, error) {
	return r.items, nil
}

type reminderProvider struct {
	name  string
	err   error
	calls int
}

func (p *reminderProvider) Name() string { return p.name }
func (p *reminderProvider) SendReminder(context.Context, notification.Reminder) error {
	p.calls++
	return p.err
}

func TestReminderServiceExcludesSubmittedAndPreventsDuplicates(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	repository := &reminderRepository{}
	provider := &reminderProvider{name: "email"}
	service := application.NewReminderServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return now })
	taskItem := task.Task{ID: 4, DueAt: "2026-11-20T00:00:00Z"}
	targets := []ports.ReminderTarget{{PersonID: "submitted", Channel: "email", Destination: "s@example.com"}, {PersonID: "missing", Channel: "email", Destination: "m@example.com"}}
	items, err := service.Schedule(context.Background(), taskItem, []submission.Submission{{SubmitterID: "submitted", State: submission.Submitted}}, targets, 72*time.Hour)
	if err != nil || len(items) != 1 || items[0].PersonID != "missing" || items[0].Status != notification.Sent || provider.calls != 1 {
		t.Fatalf("items=%#v calls=%d err=%v", items, provider.calls, err)
	}
	items, err = service.Schedule(context.Background(), taskItem, nil, targets[1:], 72*time.Hour)
	if err != nil || len(items) != 1 || provider.calls != 1 {
		t.Fatalf("duplicate items=%#v calls=%d err=%v", items, provider.calls, err)
	}
}

func TestReminderServiceUsesProductionConstructor(t *testing.T) {
	service := application.NewReminderService(&reminderRepository{}, []ports.ReminderProvider{})
	if service == nil {
		t.Fatal("NewReminderService() returned nil")
	}
}

func TestReminderServiceHonorsWindowAndRecordsFailure(t *testing.T) {
	deadline := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	provider := &reminderProvider{name: "telegram", err: errors.New("telegram unavailable")}
	repository := &reminderRepository{}
	current := deadline.Add(-96 * time.Hour)
	service := application.NewReminderServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return current })
	item := task.Task{ID: 5, DueAt: deadline.Format(time.RFC3339)}
	items, err := service.Schedule(context.Background(), item, nil, []ports.ReminderTarget{{PersonID: "p", Channel: "telegram", Destination: "chat"}}, 72*time.Hour)
	if err != nil || len(items) != 0 {
		t.Fatalf("outside window items=%#v err=%v", items, err)
	}
	current = deadline.Add(-time.Hour)
	items, err = service.Schedule(context.Background(), item, nil, []ports.ReminderTarget{{PersonID: "p", Channel: "telegram", Destination: "chat"}}, 72*time.Hour)
	if err != nil || len(items) != 1 || items[0].Status != notification.Failed || items[0].Error != "telegram unavailable" {
		t.Fatalf("failed items=%#v err=%v", items, err)
	}
	_, err = service.Schedule(context.Background(), item, nil, []ports.ReminderTarget{{PersonID: "x", Channel: "sms", Destination: "x"}}, 72*time.Hour)
	if err == nil {
		t.Fatal("unknown provider unexpectedly succeeded")
	}
	bad := task.Task{ID: 6, DueAt: "bad"}
	_, err = service.Schedule(context.Background(), bad, nil, nil, 72*time.Hour)
	if err == nil {
		t.Fatal("invalid deadline unexpectedly succeeded")
	}
}
