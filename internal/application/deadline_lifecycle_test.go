package application

import (
	"context"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type lifecycleDeliveryRepository struct{ items []report.Delivery }

func (r *lifecycleDeliveryRepository) Create(_ context.Context, item *report.Delivery) error {
	item.ID = int64(len(r.items) + 1)
	r.items = append(r.items, *item)
	return nil
}
func (r *lifecycleDeliveryRepository) Update(_ context.Context, item *report.Delivery) error {
	for index := range r.items {
		if r.items[index].ID == item.ID {
			r.items[index] = *item
		}
	}
	return nil
}
func (r *lifecycleDeliveryRepository) GetByResult(context.Context, int64) ([]report.Delivery, error) {
	return r.items, nil
}
func (r *lifecycleDeliveryRepository) GetByResultTarget(_ context.Context, resultID int64, channel, destination string) (report.Delivery, error) {
	for _, item := range r.items {
		if item.ResultID == resultID && item.Channel == channel && item.Destination == destination {
			return item, nil
		}
	}
	return report.Delivery{}, ports.ErrDeliveryNotFound
}

type lifecycleDeliveryProvider struct{ calls int }

func (p *lifecycleDeliveryProvider) Name() string { return "email" }
func (p *lifecycleDeliveryProvider) Deliver(context.Context, report.Result, string) error {
	p.calls++
	return nil
}

var _ = Describe("deadline lifecycle", func() {
	It("uses a fake clock and persists duplicate-safe notifications", func() {
		deadline := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
		item := task.Task{ID: 9, Title: "Department report", DueAt: deadline.Format(time.RFC3339), CourseID: "course-1", ExternalID: "task-9"}
		threeDays := deadline.Add(-72 * time.Hour)
		Expect(task.DeterminePhase(item, threeDays)).To(Equal(task.PhaseThreeDaysBefore))
		Expect(task.DeterminePhase(item, deadline.Add(-24*time.Hour))).To(Equal(task.PhaseOneDayBefore))

		reminderRepository := &reminderPollingRepository{}
		reminderProvider := &reminderPollingProvider{}
		reminders := NewReminderServiceWithClock(reminderRepository, []ports.ReminderProvider{reminderProvider}, func() time.Time { return threeDays })
		target := ports.ReminderTarget{PersonID: "teacher-1", Channel: "email", Destination: "teacher@example.com", TaskTitle: item.Title}
		got, err := reminders.Schedule(context.Background(), item, nil, []ports.ReminderTarget{target}, 72*time.Hour)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(HaveLen(1))
		got, err = reminders.Schedule(context.Background(), item, []submission.Submission{{SubmitterID: "teacher-1", State: submission.Submitted}}, []ports.ReminderTarget{target}, 72*time.Hour)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeEmpty())

		deliveryProvider := &lifecycleDeliveryProvider{}
		deliveries := NewDeliveryService(&lifecycleDeliveryRepository{}, []ports.ResultDeliveryProvider{deliveryProvider})
		result := report.Result{ID: 12, TaskID: item.ID}
		deliveryTarget := ports.DeliveryTarget{Channel: "email", Destination: "head@example.com"}
		_, err = deliveries.Deliver(context.Background(), result, []ports.DeliveryTarget{deliveryTarget})
		Expect(err).NotTo(HaveOccurred())
		Expect(deliveryProvider.calls).To(Equal(1))
	})
})
