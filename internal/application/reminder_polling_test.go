package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/domain/teacher"
)

type reminderPollingMissingRepository struct {
	items []ports.MissingSubmitter
	err   error
}

func (r reminderPollingMissingRepository) Reconcile(context.Context, int64, []ports.MissingSubmitter) error {
	return nil
}
func (r reminderPollingMissingRepository) List(context.Context, int64) ([]ports.MissingSubmitter, error) {
	return r.items, r.err
}

type reminderPollingRepository struct{}

func (reminderPollingRepository) Create(_ context.Context, item *notification.Reminder) error {
	item.ID = 1
	return nil
}
func (reminderPollingRepository) Update(context.Context, *notification.Reminder) error { return nil }
func (reminderPollingRepository) Get(context.Context, int64, string, string, string) (notification.Reminder, error) {
	return notification.Reminder{}, ports.ErrReminderNotFound
}
func (reminderPollingRepository) GetByID(context.Context, int64) (notification.Reminder, error) {
	return notification.Reminder{}, ports.ErrReminderNotFound
}
func (reminderPollingRepository) ListByTask(context.Context, int64) ([]notification.Reminder, error) {
	return nil, nil
}
func (reminderPollingRepository) ListRetryable(context.Context, time.Time) ([]notification.Reminder, error) {
	return nil, nil
}

type reminderPollingProvider struct{}

func (reminderPollingProvider) Name() string                                              { return "email" }
func (reminderPollingProvider) SendReminder(context.Context, notification.Reminder) error { return nil }

type reminderPollingTeacherRepository struct{}

func (reminderPollingTeacherRepository) Create(context.Context, *teacher.Recipient) error { return nil }
func (reminderPollingTeacherRepository) Update(context.Context, *teacher.Recipient) error { return nil }
func (reminderPollingTeacherRepository) Get(context.Context, int64) (teacher.Recipient, error) {
	return teacher.Recipient{ID: 7, ClassroomID: "user-1", Status: teacher.Active}, nil
}
func (reminderPollingTeacherRepository) List(context.Context) ([]teacher.Recipient, error) {
	return []teacher.Recipient{{ID: 7, ClassroomID: "user-1", Status: teacher.Active}}, nil
}
func (reminderPollingTeacherRepository) CreateChannel(context.Context, *teacher.Channel) error {
	return nil
}
func (reminderPollingTeacherRepository) UpdateChannel(context.Context, *teacher.Channel) error {
	return nil
}
func (reminderPollingTeacherRepository) ListActiveChannels(context.Context, int64) ([]teacher.Channel, error) {
	return []teacher.Channel{{RecipientID: 7, Kind: "telegram", Destination: "chat-7", Status: teacher.Active}}, nil
}

type reminderPollingRecordingProvider struct {
	name  string
	items []notification.Reminder
}

func (p *reminderPollingRecordingProvider) Name() string { return p.name }
func (p *reminderPollingRecordingProvider) SendReminder(_ context.Context, item notification.Reminder) error {
	p.items = append(p.items, item)
	return nil
}

func TestScheduleMissingRemindersBuildsEmailTargets(t *testing.T) {
	service := &PollingService{
		missing:   reminderPollingMissingRepository{items: []ports.MissingSubmitter{{ExternalUserID: "user-1", Email: "teacher@example.com"}, {ExternalUserID: "no-email"}}},
		reminders: NewReminderServiceWithClock(reminderPollingRepository{}, []ports.ReminderProvider{reminderPollingProvider{}}, func() time.Time { return time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC) }),
		head:      NewHeadNotificationService(headTeacherRepository{}, NewReminderServiceWithClock(reminderPollingRepository{}, []ports.ReminderProvider{reminderPollingProvider{}}, func() time.Time { return time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC) })),
	}
	item := task.Task{ID: 1, ExternalID: "task-1", CourseID: "course-1", Title: "Report", DueAt: "2026-11-20T00:00:00Z"}
	if err := service.scheduleMissingReminders(context.Background(), item); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleMissingRemindersBuildsEmailAndTelegramTargets(t *testing.T) {
	email := &reminderPollingRecordingProvider{name: "email"}
	telegram := &reminderPollingRecordingProvider{name: "telegram"}
	service := &PollingService{
		missing:  reminderPollingMissingRepository{items: []ports.MissingSubmitter{{ExternalUserID: "user-1", Email: "teacher@example.com"}}},
		teachers: reminderPollingTeacherRepository{},
		reminders: NewReminderServiceWithClock(
			reminderPollingRepository{},
			[]ports.ReminderProvider{email, telegram},
			func() time.Time { return time.Date(2026, 11, 22, 13, 43, 0, 0, time.UTC) },
		),
		deadlines: task.DeadlineWindows{PreliminaryBefore: 2 * time.Minute, RefreshBefore: time.Minute},
	}

	if err := service.scheduleMissingReminders(context.Background(), task.Task{ID: 1, Title: "Report", CourseID: "course-1", ExternalID: "task-1", DueAt: "2026-11-22T13:45:00Z"}); err != nil {
		t.Fatal(err)
	}
	if len(email.items) != 1 || len(telegram.items) != 1 {
		t.Fatalf("email reminders=%d, Telegram reminders=%d; want one each", len(email.items), len(telegram.items))
	}
	if telegram.items[0].Destination != "chat-7" {
		t.Fatalf("Telegram destination = %q, want chat-7", telegram.items[0].Destination)
	}
}

func TestScheduleMissingRemindersPropagatesRepositoryError(t *testing.T) {
	service := &PollingService{missing: reminderPollingMissingRepository{err: errors.New("list failed")}, reminders: NewReminderService(reminderPollingRepository{}, nil)}
	err := service.scheduleMissingReminders(context.Background(), task.Task{ID: 1, DueAt: "2026-11-20T00:00:00Z"})
	if err == nil || err.Error() != "list missing submitters for reminders: list failed" {
		t.Fatalf("error = %v", err)
	}
}

func TestScheduleMissingRemindersSkipsTasksWithoutDueDate(t *testing.T) {
	service := &PollingService{missing: reminderPollingMissingRepository{err: errors.New("should not be read")}, reminders: NewReminderService(reminderPollingRepository{}, nil)}
	if err := service.scheduleMissingReminders(context.Background(), task.Task{ID: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestPollingServiceNormalizesDeadlineWindows(t *testing.T) {
	service := &PollingService{}
	service.SetDeadlineWindows(task.DeadlineWindows{PreliminaryBefore: 5 * time.Minute, RefreshBefore: 2 * time.Minute})
	if service.deadlines.PreliminaryBefore != 5*time.Minute || service.deadlines.RefreshBefore != 2*time.Minute {
		t.Fatalf("deadline windows = %#v", service.deadlines)
	}
	service.SetDeadlineWindows(task.DeadlineWindows{})
	if service.deadlines != task.DefaultDeadlineWindows {
		t.Fatalf("default deadline windows = %#v", service.deadlines)
	}
	service.SetDeadlineWindows(task.DeadlineWindows{PreliminaryBefore: time.Minute, RefreshBefore: 2 * time.Minute})
	if service.deadlines.RefreshBefore != task.DefaultDeadlineWindows.RefreshBefore {
		t.Fatalf("normalized refresh window = %#v", service.deadlines)
	}
}

func TestPollingServicePreliminaryWindowUsesConfiguredClock(t *testing.T) {
	service := &PollingService{deadlines: task.DeadlineWindows{PreliminaryBefore: 2 * time.Minute, RefreshBefore: time.Minute}, clock: func() time.Time {
		return time.Date(2026, 11, 22, 13, 43, 0, 0, time.UTC)
	}}
	item := task.Task{DueAt: "2026-11-22T13:45:00Z"}
	if !service.inPreliminaryWindow(item) {
		t.Fatal("expected task to be in preliminary window")
	}
	service.clock = func() time.Time { return time.Date(2026, 11, 22, 13, 44, 0, 0, time.UTC) }
	if service.inPreliminaryWindow(item) {
		t.Fatal("expected task to be outside preliminary window at refresh boundary")
	}
	if !service.inPreliminaryWindow(task.Task{}) {
		t.Fatal("tasks without deadlines should be eligible")
	}
}
