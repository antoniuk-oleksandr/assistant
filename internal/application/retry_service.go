package application

import (
	"context"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
)

const maxReminderAttempts = 3

type RetryService struct {
	repository ports.ReminderRepository
	providers  map[string]ports.ReminderProvider
	clock      func() time.Time
}

func NewRetryService(repository ports.ReminderRepository, providers []ports.ReminderProvider) *RetryService {
	return NewRetryServiceWithClock(repository, providers, func() time.Time { return time.Now().UTC() })
}

func NewRetryServiceWithClock(repository ports.ReminderRepository, providers []ports.ReminderProvider, clock func() time.Time) *RetryService {
	resolved := make(map[string]ports.ReminderProvider, len(providers))
	for _, provider := range providers {
		resolved[provider.Name()] = provider
	}
	return &RetryService{repository: repository, providers: resolved, clock: clock}
}

func (s *RetryService) Retry(ctx context.Context, id int64) (notification.Reminder, error) {
	item, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return notification.Reminder{}, err
	}
	if item.Status == notification.Sent || item.Status == notification.DeadLetter {
		return item, nil
	}
	if item.NextAttemptAt != nil && s.clock().Before(*item.NextAttemptAt) {
		return item, nil
	}
	provider, ok := s.providers[item.Channel]
	if !ok {
		return item, fmt.Errorf("reminder provider %q is not configured", item.Channel)
	}
	item.Attempts++
	now := s.clock()
	item.AttemptedAt = &now
	if err := provider.SendReminder(ctx, item); err != nil {
		item.Error = err.Error()
		next := now.Add(retryDelay(item.Attempts))
		if item.Attempts >= maxReminderAttempts {
			item.Status, item.NextAttemptAt = notification.DeadLetter, nil
		} else {
			item.Status = notification.Failed
			item.NextAttemptAt = &next
		}
	} else {
		item.Status, item.Error, item.NextAttemptAt = notification.Sent, "", nil
	}
	if err := s.repository.Update(ctx, &item); err != nil {
		return item, fmt.Errorf("persist retry result: %w", err)
	}
	return item, nil
}

func (s *RetryService) RetryAll(ctx context.Context) ([]notification.Reminder, error) {
	items, err := s.repository.ListRetryable(ctx, s.clock())
	if err != nil {
		return nil, err
	}
	result := make([]notification.Reminder, 0, len(items))
	for _, item := range items {
		retried, err := s.Retry(ctx, item.ID)
		if err != nil {
			return result, err
		}
		result = append(result, retried)
	}
	return result, nil
}

func retryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 5 * time.Second
	case 2:
		return 10 * time.Second
	default:
		return 30 * time.Second
	}
}
