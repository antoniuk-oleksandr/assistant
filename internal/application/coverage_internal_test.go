package application

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/domain/teacher"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type coverageMissingRepository struct {
	items []ports.MissingSubmitter
	err   error
}

func (r coverageMissingRepository) Reconcile(context.Context, int64, []ports.MissingSubmitter) error {
	return nil
}
func (r coverageMissingRepository) List(context.Context, int64) ([]ports.MissingSubmitter, error) {
	return r.items, r.err
}

type coverageProfileSource struct{ err error }

func (r coverageProfileSource) GetProfile(context.Context, string) (teacher.Profile, error) {
	return teacher.Profile{}, r.err
}

type coveragePreliminaryNotifier struct{ err error }

func (n coveragePreliminaryNotifier) Notify(context.Context, task.Task, []ports.MissingSubmitter) error {
	return n.err
}

type coverageReminderRepository struct{ updateErr error }

func (coverageReminderRepository) Create(context.Context, *notification.Reminder) error { return nil }
func (r coverageReminderRepository) Update(context.Context, *notification.Reminder) error {
	return r.updateErr
}
func (coverageReminderRepository) Get(context.Context, int64, string, string, string) (notification.Reminder, error) {
	return notification.Reminder{}, ports.ErrReminderNotFound
}
func (coverageReminderRepository) GetByID(context.Context, int64) (notification.Reminder, error) {
	return notification.Reminder{}, ports.ErrReminderNotFound
}
func (coverageReminderRepository) ListByTask(context.Context, int64) ([]notification.Reminder, error) {
	return nil, nil
}
func (coverageReminderRepository) ListRetryable(context.Context, time.Time) ([]notification.Reminder, error) {
	return nil, nil
}

type coverageReminderProvider struct{}

func (coverageReminderProvider) Name() string { return "email" }
func (coverageReminderProvider) SendReminder(context.Context, notification.Reminder) error {
	return nil
}

type coverageTeacherRepository struct{}

func (coverageTeacherRepository) Create(context.Context, *teacher.Recipient) error { return nil }
func (coverageTeacherRepository) Update(context.Context, *teacher.Recipient) error { return nil }
func (coverageTeacherRepository) Get(context.Context, int64) (teacher.Recipient, error) {
	return teacher.Recipient{}, nil
}
func (coverageTeacherRepository) List(context.Context) ([]teacher.Recipient, error) {
	return []teacher.Recipient{{ID: 1, ClassroomID: "user-1", Status: teacher.Active}}, nil
}
func (coverageTeacherRepository) CreateChannel(context.Context, *teacher.Channel) error { return nil }
func (coverageTeacherRepository) UpdateChannel(context.Context, *teacher.Channel) error { return nil }
func (coverageTeacherRepository) ListActiveChannels(context.Context, int64) ([]teacher.Channel, error) {
	return []teacher.Channel{{Kind: "email"}, {Kind: "telegram"}}, nil
}

type coverageMaterialRetriever struct{}

func (coverageMaterialRetriever) Retrieve(context.Context, submission.Material) submission.Material {
	return submission.Material{}
}

type coverageAnalyzer struct{ cancel context.CancelFunc }

func (a coverageAnalyzer) Analyze(context.Context, submission.Material, submission.ParseContext) (submission.ExtractionProposal, error) {
	a.cancel()
	return submission.ExtractionProposal{}, nil
}

var _ = Describe("polling internal helpers", func() {
	It("handle invalid and empty values", func() {
		service := &PollingService{deadlines: task.DeadlineWindows{PreliminaryBefore: time.Minute, RefreshBefore: time.Second}}
		Expect(service.inPreliminaryWindow(task.Task{})).To(BeTrue())
		filtered, err := service.excludeAdministrator(context.Background(), []ports.MissingSubmitter{})
		Expect(err).NotTo(HaveOccurred())
		Expect(filtered).NotTo(BeNil())
		Expect(submissionsFingerprint(nil)).NotTo(BeEmpty())
	})

	It("reports a missing reminder template", func() {
		GinkgoT().Setenv("REMINDER_TEMPLATE_PATH", "definitely-missing-reminder-template.txt")
		_, err := renderReminderMessage("Task", "2026-11-30T00:00:00Z", "https://classroom.example/task", time.Minute)
		Expect(err).To(HaveOccurred())
	})

	It("stops material processing after cancellation", func() {
		service := &PollingService{materials: coverageMaterialRetriever{}}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := service.processMaterials(ctx, task.Binding{Task: task.Task{CourseID: "course", ExternalID: "task"}}, submission.Submission{Materials: []submission.Material{{Name: "file"}}})
		Expect(err).To(MatchError(context.Canceled))
	})

	It("stops material parsing when the analyzer cancels", func() {
		ctx, cancel := context.WithCancel(context.Background())
		parsing := NewMaterialParsingWithAnalyzer(coverageAnalyzer{cancel: cancel})
		_, err := parsing.Parse(ctx, submission.Material{Name: "file.bin"}, submission.ParseContext{})
		Expect(err).To(MatchError(context.Canceled))
	})

	It("returns notification dependency errors", func() {
		service := &PollingService{
			missing:   coverageMissingRepository{err: errors.New("missing failed")},
			profile:   coverageProfileSource{err: errors.New("profile failed")},
			deadlines: task.DefaultDeadlineWindows,
		}
		Expect(service.scheduleMissingReminders(context.Background(), task.Task{ID: 1, DueAt: "2026-11-30T00:00:00Z"})).To(HaveOccurred())
		service = &PollingService{missing: coverageMissingRepository{}, profile: coverageProfileSource{err: errors.New("profile failed")}, deadlines: task.DefaultDeadlineWindows}
		Expect(service.scheduleMissingReminders(context.Background(), task.Task{ID: 1, DueAt: "2026-11-30T00:00:00Z"})).To(HaveOccurred())
		service.missing = coverageMissingRepository{}
		Expect(service.notifyPreliminarySummary(context.Background(), task.Task{ID: 1})).To(HaveOccurred())
		service = &PollingService{missing: coverageMissingRepository{err: errors.New("missing failed")}}
		Expect(service.notifyPreliminarySummary(context.Background(), task.Task{ID: 1})).To(HaveOccurred())
		service = &PollingService{}
		Expect(service.notifyPreliminarySummary(context.Background(), task.Task{ID: 1})).NotTo(HaveOccurred())
		service = &PollingService{
			missing: coverageMissingRepository{items: []ports.MissingSubmitter{{ExternalUserID: "user-1", Email: "user@example.com"}}}, teachers: coverageTeacherRepository{},
			reminders: NewReminderService(coverageReminderRepository{updateErr: errors.New("update failed")}, []ports.ReminderProvider{coverageReminderProvider{}}), deadlines: task.DefaultDeadlineWindows,
		}
		Expect(service.scheduleMissingReminders(context.Background(), task.Task{ID: 1, ExternalID: "task", CourseID: "course", Title: "Task", DueAt: "2026-11-30T00:00:00Z"})).NotTo(HaveOccurred())
	})
})
