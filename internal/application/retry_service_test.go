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
)

func TestRetryServiceUsesDelaysAndMovesExhaustedFailuresToDLQ(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	repository := &reminderRepository{items: []notification.Reminder{{ID: 1, Channel: "email", Status: notification.Failed, Error: "old", CreatedAt: now}}}
	provider := &reminderProvider{name: "email", err: errors.New("still down")}
	service := application.NewRetryServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return now })
	item, err := service.Retry(context.Background(), 1)
	if err != nil || item.Attempts != 1 || item.Status != notification.Failed || item.NextAttemptAt == nil || !item.NextAttemptAt.Equal(now.Add(5*time.Second)) {
		t.Fatalf("first retry=%#v err=%v", item, err)
	}
	now = now.Add(5 * time.Second)
	item, err = service.Retry(context.Background(), 1)
	if err != nil || item.Attempts != 2 || item.NextAttemptAt == nil || !item.NextAttemptAt.Equal(now.Add(10*time.Second)) {
		t.Fatalf("second retry=%#v err=%v", item, err)
	}
	now = now.Add(10 * time.Second)
	item, err = service.Retry(context.Background(), 1)
	if err != nil || item.Attempts != 3 || item.Status != notification.DeadLetter || item.NextAttemptAt != nil {
		t.Fatalf("dead letter=%#v err=%v", item, err)
	}
	item, err = service.Retry(context.Background(), 1)
	if err != nil || item.Attempts != 3 || provider.calls != 3 {
		t.Fatalf("dead letter retried=%#v calls=%d err=%v", item, provider.calls, err)
	}
}

func TestRetryServiceUsesProductionConstructor(t *testing.T) {
	repository := &reminderRepository{items: []notification.Reminder{{ID: 1, Channel: "email", Status: notification.Failed}}}
	service := application.NewRetryService(repository, []ports.ReminderProvider{&reminderProvider{name: "email"}})
	if service == nil {
		t.Fatal("NewRetryService() returned nil")
	}
	if _, err := service.Retry(context.Background(), 1); err != nil {
		t.Fatalf("Retry() with production clock: %v", err)
	}
}

func TestRetryServiceRetriesBulkAndHandlesPendingOrProviderErrors(t *testing.T) {
	now := time.Now().UTC()
	repository := &reminderRepository{items: []notification.Reminder{{ID: 1, Channel: "email", Status: notification.Failed, CreatedAt: now}}}
	provider := &reminderProvider{name: "email"}
	service := application.NewRetryServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return now })
	items, err := service.RetryAll(context.Background())
	if err != nil || len(items) != 1 || items[0].Status != notification.Sent {
		t.Fatalf("bulk=%#v err=%v", items, err)
	}
	repository.items[0].Status = notification.Failed
	repository.items[0].NextAttemptAt = func() *time.Time { value := now.Add(time.Hour); return &value }()
	item, err := service.Retry(context.Background(), 1)
	if err != nil || item.Status != notification.Failed || provider.calls != 1 {
		t.Fatalf("scheduled retry=%#v calls=%d err=%v", item, provider.calls, err)
	}
	repository.items[0].Channel = "missing"
	repository.items[0].Status = notification.Failed
	repository.items[0].NextAttemptAt = nil
	_, err = service.Retry(context.Background(), 1)
	if err == nil {
		t.Fatal("missing provider unexpectedly succeeded")
	}
}

func TestRetryServiceReportsRepositoryFailures(t *testing.T) {
	now := time.Now().UTC()
	provider := &reminderProvider{name: "email"}
	service := application.NewRetryServiceWithClock(&reminderRepository{getIDErr: errors.New("read failed")}, []ports.ReminderProvider{provider}, func() time.Time { return now })
	if _, err := service.Retry(context.Background(), 1); err == nil || err.Error() != "read failed" {
		t.Fatalf("get error=%v", err)
	}
	service = application.NewRetryServiceWithClock(&reminderRepository{listRetryErr: errors.New("list failed")}, []ports.ReminderProvider{provider}, func() time.Time { return now })
	if _, err := service.RetryAll(context.Background()); err == nil || err.Error() != "list failed" {
		t.Fatalf("list error=%v", err)
	}
	repository := &reminderRepository{items: []notification.Reminder{{ID: 1, Channel: "email", Status: notification.Failed}}, updateErr: errors.New("write failed")}
	service = application.NewRetryServiceWithClock(repository, []ports.ReminderProvider{provider}, func() time.Time { return now })
	if _, err := service.Retry(context.Background(), 1); err == nil || err.Error() != "persist retry result: write failed" {
		t.Fatalf("update error=%v", err)
	}
}

func TestRetryServiceStopsBulkRetryOnItemFailure(t *testing.T) {
	repository := &reminderRepository{items: []notification.Reminder{{ID: 1, Channel: "missing", Status: notification.Failed}}}
	service := application.NewRetryServiceWithClock(repository, nil, func() time.Time { return time.Now().UTC() })
	if _, err := service.RetryAll(context.Background()); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("RetryAll() error = %v", err)
	}
}
