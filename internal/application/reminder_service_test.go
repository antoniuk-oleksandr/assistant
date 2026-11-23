package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

func (p *reminderProvider) Name() string { return p.name }
func (p *reminderProvider) SendReminder(context.Context, notification.Reminder) error {
	p.calls++
	return p.err
}

var _ = Describe("reminder service", func() {
	It("uses the production constructor", func() {
		service := application.NewReminderService(&reminderRepository{}, []ports.ReminderProvider{})
		Expect(service).NotTo(BeNil())
		_, err := service.Schedule(context.Background(), task.Task{DueAt: "2026-11-20T00:00:00Z"}, nil, nil, 72*time.Hour)
		Expect(err).NotTo(HaveOccurred())
	})

	It("excludes submitted users and prevents duplicates", func() {
		now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
		repository := &reminderRepository{}
		provider := &reminderProvider{name: "email"}
		service := application.NewReminderServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return now })
		taskItem := task.Task{ID: 4, DueAt: "2026-11-20T00:00:00", TimeZone: "UTC"}
		targets := []ports.ReminderTarget{{PersonID: "submitted", Channel: "email", Destination: "s@example.com"}, {PersonID: "missing", Channel: "email", Destination: "m@example.com"}}
		items, err := service.Schedule(context.Background(), taskItem, []submission.Submission{{SubmitterID: "submitted", State: submission.Submitted}}, targets, 72*time.Hour)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].PersonID).To(Equal("missing"))
		Expect(items[0].Status).To(Equal(notification.Sent))
		Expect(provider.calls).To(Equal(1))
		items, err = service.Schedule(context.Background(), taskItem, nil, targets[1:], 72*time.Hour)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(provider.calls).To(Equal(1))
	})

	It("renders reminder details", func() {
		now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
		repository := &reminderRepository{}
		provider := &reminderProvider{name: "email"}
		service := application.NewReminderServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return now })
		item := task.Task{ID: 4, Title: "Weekly report", DueAt: "2026-11-20T00:00:00Z", CourseID: "course-1", ExternalID: "task-1"}
		items, err := service.Schedule(context.Background(), item, nil, []ports.ReminderTarget{{PersonID: "missing", Channel: "email", Destination: "m@example.com", TaskTitle: item.Title, ClassroomLink: "https://classroom/task-1"}}, 72*time.Hour)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Subject).To(Equal("Reminder: Weekly report"))
		Expect(items[0].DueAt).To(Equal(item.DueAt))
		Expect(items[0].ClassroomLink).To(Equal("https://classroom/task-1"))
		for _, value := range []string{"Weekly report", item.DueAt, "48h0m0s", "https://classroom/task-1"} {
			Expect(items[0].Message).To(ContainSubstring(value))
		}
	})

	It("propagates unexpected lookup errors", func() {
		service := application.NewReminderServiceWithClock(&reminderRepository{lookupErr: errors.New("lookup failed")}, []ports.ReminderProvider{&reminderProvider{name: "email"}}, func() time.Time { return time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC) })
		_, err := service.Schedule(context.Background(), task.Task{ID: 1, DueAt: "2026-11-20T00:00:00Z"}, nil, []ports.ReminderTarget{{PersonID: "person", Channel: "email", Destination: "person@example.com"}}, 72*time.Hour)
		Expect(err).To(MatchError("lookup failed"))
	})

	It("reports a missing template", func() {
		GinkgoT().Setenv("REMINDER_TEMPLATE_PATH", "/tmp/does-not-exist-reminder-template")
		now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
		service := application.NewReminderServiceWithClock(&reminderRepository{}, []ports.ReminderProvider{&reminderProvider{}}, func() time.Time { return now })
		_, err := service.Schedule(context.Background(), task.Task{ID: 1, DueAt: "2026-11-20T00:00:00Z"}, nil, []ports.ReminderTarget{{PersonID: "p", Channel: "", Destination: "x"}}, 72*time.Hour)
		Expect(err).To(MatchError(ContainSubstring("read reminder template")))
	})

	It("reports persistence errors", func() {
		now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
		service := application.NewReminderServiceWithClock(&reminderRepository{createErr: errors.New("create failed")}, []ports.ReminderProvider{&reminderProvider{name: "email"}}, func() time.Time { return now })
		_, err := service.Schedule(context.Background(), task.Task{ID: 1, DueAt: "2026-11-20T00:00:00Z"}, nil, []ports.ReminderTarget{{PersonID: "p", Channel: "email", Destination: "p@example.com"}}, 72*time.Hour)
		Expect(err).To(MatchError("persist reminder: create failed"))
		repository := &reminderRepository{updateErr: errors.New("update failed")}
		service = application.NewReminderServiceWithClock(repository, []ports.ReminderProvider{&reminderProvider{name: "email"}}, func() time.Time { return now })
		_, err = service.Schedule(context.Background(), task.Task{ID: 1, DueAt: "2026-11-20T00:00:00Z"}, nil, []ports.ReminderTarget{{PersonID: "p", Channel: "email", Destination: "p@example.com"}}, 72*time.Hour)
		Expect(err).To(MatchError("persist reminder status: update failed"))
	})

	It("honors the window and records failures", func() {
		deadline := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
		provider := &reminderProvider{name: "telegram", err: errors.New("telegram unavailable")}
		repository := &reminderRepository{}
		current := deadline.Add(-96 * time.Hour)
		service := application.NewReminderServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return current })
		item := task.Task{ID: 5, DueAt: deadline.Format(time.RFC3339)}
		items, err := service.Schedule(context.Background(), item, nil, []ports.ReminderTarget{{PersonID: "p", Channel: "telegram", Destination: "chat"}}, 72*time.Hour)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		current = deadline.Add(-time.Hour)
		items, err = service.Schedule(context.Background(), item, nil, []ports.ReminderTarget{{PersonID: "p", Channel: "telegram", Destination: "chat"}}, 72*time.Hour)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Status).To(Equal(notification.Failed))
		Expect(items[0].Error).To(Equal("telegram unavailable"))
		_, err = service.Schedule(context.Background(), item, nil, []ports.ReminderTarget{{PersonID: "x", Channel: "sms", Destination: "x"}}, 72*time.Hour)
		Expect(err).To(HaveOccurred())
		bad := task.Task{ID: 6, DueAt: "bad"}
		_, err = service.Schedule(context.Background(), bad, nil, nil, 72*time.Hour)
		Expect(err).To(HaveOccurred())
	})
})
