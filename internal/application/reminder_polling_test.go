package application

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/domain/teacher"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

type reminderPollingFailingTeacherRepository struct {
	listErr    bool
	channelErr bool
}

func (reminderPollingFailingTeacherRepository) Create(context.Context, *teacher.Recipient) error {
	return nil
}
func (reminderPollingFailingTeacherRepository) Update(context.Context, *teacher.Recipient) error {
	return nil
}
func (reminderPollingFailingTeacherRepository) Get(context.Context, int64) (teacher.Recipient, error) {
	return teacher.Recipient{}, nil
}

func (r reminderPollingFailingTeacherRepository) List(context.Context) ([]teacher.Recipient, error) {
	if r.listErr {
		return nil, errors.New("recipient list failed")
	}
	return []teacher.Recipient{{ID: 7, ClassroomID: "user-1", Status: teacher.Active}}, nil
}
func (reminderPollingFailingTeacherRepository) CreateChannel(context.Context, *teacher.Channel) error {
	return nil
}
func (reminderPollingFailingTeacherRepository) UpdateChannel(context.Context, *teacher.Channel) error {
	return nil
}
func (r reminderPollingFailingTeacherRepository) ListActiveChannels(context.Context, int64) ([]teacher.Channel, error) {
	if r.channelErr {
		return nil, errors.New("channel list failed")
	}
	return nil, nil
}

type reminderPollingRecordingProvider struct {
	name  string
	items []notification.Reminder
}

type reminderPollingProfile struct {
	profile teacher.Profile
	err     error
}

func (p reminderPollingProfile) GetProfile(context.Context, string) (teacher.Profile, error) {
	return p.profile, p.err
}

type reminderPollingPreliminaryNotifier struct {
	missing []ports.MissingSubmitter
}

func (n *reminderPollingPreliminaryNotifier) Notify(_ context.Context, _ task.Task, missing []ports.MissingSubmitter) error {
	n.missing = missing
	return nil
}

func (p *reminderPollingRecordingProvider) Name() string { return p.name }
func (p *reminderPollingRecordingProvider) SendReminder(_ context.Context, item notification.Reminder) error {
	p.items = append(p.items, item)
	return nil
}

var _ = Describe("polling reminders", func() {
	It("builds email targets", func() {
		service := &PollingService{
			missing:   reminderPollingMissingRepository{items: []ports.MissingSubmitter{{ExternalUserID: "user-1", Email: "teacher@example.com"}, {ExternalUserID: "no-email"}}},
			reminders: NewReminderServiceWithClock(reminderPollingRepository{}, []ports.ReminderProvider{reminderPollingProvider{}}, func() time.Time { return time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC) }),
		}
		item := task.Task{ID: 1, ExternalID: "task-1", CourseID: "course-1", Title: "Report", DueAt: "2026-11-20T00:00:00Z"}
		Expect(service.scheduleMissingReminders(context.Background(), item)).To(Succeed())
	})

	It("builds email and Telegram targets", func() {
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

		Expect(service.scheduleMissingReminders(context.Background(), task.Task{ID: 1, Title: "Report", CourseID: "course-1", ExternalID: "task-1", DueAt: "2026-11-22T13:45:00Z"})).To(Succeed())
		Expect(email.items).To(HaveLen(1))
		Expect(telegram.items).To(HaveLen(1))
		Expect(telegram.items[0].Destination).To(Equal("chat-7"))
	})

	It("reports teacher channel failures", func() {
		service := &PollingService{
			missing:  reminderPollingMissingRepository{items: []ports.MissingSubmitter{{ExternalUserID: "user-1", Email: "teacher@example.com"}}},
			teachers: reminderPollingFailingTeacherRepository{channelErr: true},
			reminders: NewReminderServiceWithClock(
				reminderPollingRepository{}, []ports.ReminderProvider{reminderPollingProvider{}},
				func() time.Time { return time.Date(2026, 11, 22, 13, 43, 0, 0, time.UTC) },
			),
		}
		err := service.scheduleMissingReminders(context.Background(), task.Task{ID: 1, CourseID: "course-1", ExternalID: "task-1", DueAt: "2026-11-22T13:45:00Z"})
		Expect(err).To(HaveOccurred())
	})

	It("reports teacher list failures", func() {
		service := &PollingService{
			missing:   reminderPollingMissingRepository{items: []ports.MissingSubmitter{{ExternalUserID: "user-1", Email: "teacher@example.com"}}},
			teachers:  reminderPollingFailingTeacherRepository{listErr: true},
			reminders: NewReminderService(reminderPollingRepository{}, []ports.ReminderProvider{reminderPollingProvider{}}),
		}
		Expect(service.scheduleMissingReminders(context.Background(), task.Task{ID: 1, DueAt: "2026-11-22T13:45:00Z"})).To(HaveOccurred())
	})

	It("filters the administrator from preliminary summaries", func() {
		notifier := &reminderPollingPreliminaryNotifier{}
		service := &PollingService{
			missing:     reminderPollingMissingRepository{items: []ports.MissingSubmitter{{ExternalUserID: "admin"}, {ExternalUserID: "student"}}},
			profile:     reminderPollingProfile{profile: teacher.Profile{ClassroomID: "admin"}},
			preliminary: notifier,
		}
		Expect(service.notifyPreliminarySummary(context.Background(), task.Task{ID: 1})).To(Succeed())
		Expect(notifier.missing).To(HaveLen(1))
		Expect(notifier.missing[0].ExternalUserID).To(Equal("student"))
	})

	It("reports profile failures", func() {
		service := &PollingService{
			missing:     reminderPollingMissingRepository{items: []ports.MissingSubmitter{{ExternalUserID: "student"}}},
			profile:     reminderPollingProfile{err: errors.New("profile failed")},
			preliminary: &reminderPollingPreliminaryNotifier{},
		}
		Expect(service.notifyPreliminarySummary(context.Background(), task.Task{ID: 1})).To(HaveOccurred())
	})

	It("propagates repository errors", func() {
		service := &PollingService{missing: reminderPollingMissingRepository{err: errors.New("list failed")}, reminders: NewReminderService(reminderPollingRepository{}, nil)}
		err := service.scheduleMissingReminders(context.Background(), task.Task{ID: 1, DueAt: "2026-11-20T00:00:00Z"})
		Expect(err).To(MatchError("list missing submitters for reminders: list failed"))
	})

	It("skips tasks without due dates", func() {
		service := &PollingService{missing: reminderPollingMissingRepository{err: errors.New("should not be read")}, reminders: NewReminderService(reminderPollingRepository{}, nil)}
		Expect(service.scheduleMissingReminders(context.Background(), task.Task{ID: 1})).To(Succeed())
	})

	It("normalizes deadline windows", func() {
		service := &PollingService{}
		service.SetDeadlineWindows(task.DeadlineWindows{PreliminaryBefore: 5 * time.Minute, RefreshBefore: 2 * time.Minute})
		Expect(service.deadlines).To(Equal(task.DeadlineWindows{PreliminaryBefore: 5 * time.Minute, RefreshBefore: 2 * time.Minute}))
		service.SetDeadlineWindows(task.DeadlineWindows{})
		Expect(service.deadlines).To(Equal(task.DefaultDeadlineWindows))
		service.SetDeadlineWindows(task.DeadlineWindows{PreliminaryBefore: time.Minute, RefreshBefore: 2 * time.Minute})
		Expect(service.deadlines.RefreshBefore).To(Equal(task.DefaultDeadlineWindows.RefreshBefore))
	})

	It("uses the configured clock for preliminary windows", func() {
		service := &PollingService{deadlines: task.DeadlineWindows{PreliminaryBefore: 2 * time.Minute, RefreshBefore: time.Minute}, clock: func() time.Time {
			return time.Date(2026, 11, 22, 13, 43, 0, 0, time.UTC)
		}}
		item := task.Task{DueAt: "2026-11-22T13:45:00Z"}
		Expect(service.inPreliminaryWindow(item)).To(BeTrue())
		service.clock = func() time.Time { return time.Date(2026, 11, 22, 13, 44, 0, 0, time.UTC) }
		Expect(service.inPreliminaryWindow(item)).To(BeFalse())
		Expect(service.inPreliminaryWindow(task.Task{})).To(BeTrue())
	})
})
