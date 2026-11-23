package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("retry service", func() {
	It("uses delays and moves exhausted failures to the dead-letter queue", func() {
		now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
		repository := &reminderRepository{items: []notification.Reminder{{ID: 1, Channel: "email", Status: notification.Failed, Error: "old", CreatedAt: now}}}
		provider := &reminderProvider{name: "email", err: errors.New("still down")}
		service := application.NewRetryServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return now })
		item, err := service.Retry(context.Background(), 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Attempts).To(Equal(1))
		Expect(item.Status).To(Equal(notification.Failed))
		Expect(item.NextAttemptAt).NotTo(BeNil())
		Expect(*item.NextAttemptAt).To(Equal(now.Add(5 * time.Second)))
		now = now.Add(5 * time.Second)
		item, err = service.Retry(context.Background(), 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Attempts).To(Equal(2))
		Expect(item.NextAttemptAt).NotTo(BeNil())
		Expect(*item.NextAttemptAt).To(Equal(now.Add(10 * time.Second)))
		now = now.Add(10 * time.Second)
		item, err = service.Retry(context.Background(), 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Attempts).To(Equal(3))
		Expect(item.Status).To(Equal(notification.DeadLetter))
		Expect(item.NextAttemptAt).To(BeNil())
		item, err = service.Retry(context.Background(), 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Attempts).To(Equal(3))
		Expect(provider.calls).To(Equal(3))
	})

	It("uses the production constructor", func() {
		repository := &reminderRepository{items: []notification.Reminder{{ID: 1, Channel: "email", Status: notification.Failed}}}
		service := application.NewRetryService(repository, []ports.ReminderProvider{&reminderProvider{name: "email"}})
		Expect(service).NotTo(BeNil())
		_, err := service.Retry(context.Background(), 1)
		Expect(err).NotTo(HaveOccurred())
	})

	It("retries in bulk and handles pending or provider errors", func() {
		now := time.Now().UTC()
		repository := &reminderRepository{items: []notification.Reminder{{ID: 1, Channel: "email", Status: notification.Failed, CreatedAt: now}}}
		provider := &reminderProvider{name: "email"}
		service := application.NewRetryServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return now })
		items, err := service.RetryAll(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Status).To(Equal(notification.Sent))
		repository.items[0].Status = notification.Failed
		repository.items[0].NextAttemptAt = func() *time.Time { value := now.Add(time.Hour); return &value }()
		item, err := service.Retry(context.Background(), 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Status).To(Equal(notification.Failed))
		Expect(provider.calls).To(Equal(1))
		repository.items[0].Channel = "missing"
		repository.items[0].Status = notification.Failed
		repository.items[0].NextAttemptAt = nil
		_, err = service.Retry(context.Background(), 1)
		Expect(err).To(HaveOccurred())
	})

	It("reports repository failures", func() {
		now := time.Now().UTC()
		provider := &reminderProvider{name: "email"}
		service := application.NewRetryServiceWithClock(&reminderRepository{getIDErr: errors.New("read failed")}, []ports.ReminderProvider{provider}, func() time.Time { return now })
		_, err := service.Retry(context.Background(), 1)
		Expect(err).To(MatchError("read failed"))
		service = application.NewRetryServiceWithClock(&reminderRepository{listRetryErr: errors.New("list failed")}, []ports.ReminderProvider{provider}, func() time.Time { return now })
		_, err = service.RetryAll(context.Background())
		Expect(err).To(MatchError("list failed"))
		repository := &reminderRepository{items: []notification.Reminder{{ID: 1, Channel: "email", Status: notification.Failed}}, updateErr: errors.New("write failed")}
		service = application.NewRetryServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return now })
		_, err = service.Retry(context.Background(), 1)
		Expect(err).To(MatchError("persist retry result: write failed"))
	})

	It("stops bulk retry on item failure", func() {
		repository := &reminderRepository{items: []notification.Reminder{{ID: 1, Channel: "missing", Status: notification.Failed}}}
		service := application.NewRetryServiceWithClock(repository, nil, func() time.Time { return time.Now().UTC() })
		_, err := service.RetryAll(context.Background())
		Expect(err).To(MatchError(ContainSubstring("not configured")))
	})
})
