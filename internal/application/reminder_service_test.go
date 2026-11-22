package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

type reminderRepository struct {
	items                  []notification.Reminder
	lookupErr, updateErr   error
	createErr              error
	getIDErr, listRetryErr error
}

func (r *reminderRepository) Create(_ context.Context, item *notification.Reminder) error {
	if r.createErr != nil {
		return r.createErr
	}
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
func (r *reminderRepository) GetByID(_ context.Context, id int64) (notification.Reminder, error) {
	if r.getIDErr != nil {
		return notification.Reminder{}, r.getIDErr
	}
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return notification.Reminder{}, ports.ErrReminderNotFound
}
func (r *reminderRepository) ListRetryable(context.Context, time.Time) ([]notification.Reminder, error) {
	if r.listRetryErr != nil {
		return nil, r.listRetryErr
	}
	return r.items, nil
}
func (r *reminderRepository) ListByTask(context.Context, int64) ([]notification.Reminder, error) {
	return r.items, nil
}

type reminderProvider struct {
	name  string
	err   error
	calls int
}

func TestReminderServiceUsesProductionConstructor(t *testing.T) {
	service := application.NewReminderService(&reminderRepository{}, []ports.ReminderProvider{})
	if service == nil {
		t.Fatal("NewReminderService() returned nil")
	}
	_, err := service.Schedule(context.Background(), task.Task{DueAt: "2026-11-20T00:00:00Z"}, nil, nil, 72*time.Hour)
	if err != nil {
		t.Fatalf("Schedule() with production clock: %v", err)
	}
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
	taskItem := task.Task{ID: 4, DueAt: "2026-11-20T00:00:00", TimeZone: "UTC"}
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

func TestReminderServiceRendersReminderDetails(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	repository := &reminderRepository{}
	provider := &reminderProvider{name: "email"}
	service := application.NewReminderServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return now })
	item := task.Task{ID: 4, Title: "Weekly report", DueAt: "2026-11-20T00:00:00Z", CourseID: "course-1", ExternalID: "task-1"}
	items, err := service.Schedule(context.Background(), item, nil, []ports.ReminderTarget{{PersonID: "missing", Channel: "email", Destination: "m@example.com", TaskTitle: item.Title, ClassroomLink: "https://classroom/task-1"}}, 72*time.Hour)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if items[0].Subject != "Reminder: Weekly report" || items[0].DueAt != item.DueAt || items[0].ClassroomLink != "https://classroom/task-1" {
		t.Fatalf("reminder metadata=%#v", items[0])
	}
	for _, value := range []string{"Weekly report", item.DueAt, "48h0m0s", "https://classroom/task-1"} {
		if !strings.Contains(items[0].Message, value) {
			t.Fatalf("message %q does not contain %q", items[0].Message, value)
		}
	}
}

func TestReminderServicePropagatesUnexpectedReminderLookupErrors(t *testing.T) {
	service := application.NewReminderServiceWithClock(&reminderRepository{lookupErr: errors.New("lookup failed")}, []ports.ReminderProvider{&reminderProvider{name: "email"}}, func() time.Time { return time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC) })
	_, err := service.Schedule(context.Background(), task.Task{ID: 1, DueAt: "2026-11-20T00:00:00Z"}, nil, []ports.ReminderTarget{{PersonID: "person", Channel: "email", Destination: "person@example.com"}}, 72*time.Hour)
	if err == nil || err.Error() != "lookup failed" {
		t.Fatalf("Schedule() error = %v", err)
	}
}

func TestReminderServiceReportsMissingTemplate(t *testing.T) {
	t.Setenv("REMINDER_TEMPLATE_PATH", "/tmp/does-not-exist-reminder-template")
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	service := application.NewReminderServiceWithClock(&reminderRepository{}, []ports.ReminderProvider{&reminderProvider{}}, func() time.Time { return now })
	_, err := service.Schedule(context.Background(), task.Task{ID: 1, DueAt: "2026-11-20T00:00:00Z"}, nil, []ports.ReminderTarget{{PersonID: "p", Channel: "", Destination: "x"}}, 72*time.Hour)
	if err == nil || !strings.Contains(err.Error(), "read reminder template") {
		t.Fatalf("error = %v", err)
	}
}

func TestReminderServiceReportsPersistenceErrors(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	service := application.NewReminderServiceWithClock(&reminderRepository{createErr: errors.New("create failed")}, []ports.ReminderProvider{&reminderProvider{name: "email"}}, func() time.Time { return now })
	_, err := service.Schedule(context.Background(), task.Task{ID: 1, DueAt: "2026-11-20T00:00:00Z"}, nil, []ports.ReminderTarget{{PersonID: "p", Channel: "email", Destination: "p@example.com"}}, 72*time.Hour)
	if err == nil || err.Error() != "persist reminder: create failed" {
		t.Fatalf("create error = %v", err)
	}
	repository := &reminderRepository{updateErr: errors.New("update failed")}
	service = application.NewReminderServiceWithClock(repository, []ports.ReminderProvider{&reminderProvider{name: "email"}}, func() time.Time { return now })
	_, err = service.Schedule(context.Background(), task.Task{ID: 1, DueAt: "2026-11-20T00:00:00Z"}, nil, []ports.ReminderTarget{{PersonID: "p", Channel: "email", Destination: "p@example.com"}}, 72*time.Hour)
	if err == nil || err.Error() != "persist reminder status: update failed" {
		t.Fatalf("update error = %v", err)
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
