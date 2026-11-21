package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
	"github.com/alex/assistent/internal/domain/task"
)

type ReminderService struct {
	repository ports.ReminderRepository
	providers  map[string]ports.ReminderProvider
	clock      func() time.Time
}

func NewReminderService(repository ports.ReminderRepository, providers []ports.ReminderProvider) *ReminderService {
	return NewReminderServiceWithClock(repository, providers, func() time.Time { return time.Now().UTC() })
}

func NewReminderServiceWithClock(repository ports.ReminderRepository, providers []ports.ReminderProvider, clock func() time.Time) *ReminderService {
	resolved := make(map[string]ports.ReminderProvider, len(providers))
	for _, provider := range providers {
		resolved[provider.Name()] = provider
	}
	return &ReminderService{repository: repository, providers: resolved, clock: clock}
}

func (s *ReminderService) Schedule(ctx context.Context, item task.Task, submissions []submission.Submission, targets []ports.ReminderTarget, lead time.Duration) ([]notification.Reminder, error) {
	deadline, err := time.Parse(time.RFC3339, item.DueAt)
	if err != nil {
		return nil, fmt.Errorf("parse task deadline: %w", err)
	}
	now := s.clock()
	if now.Before(deadline.Add(-lead)) || !now.Before(deadline) {
		return []notification.Reminder{}, nil
	}
	submitted := make(map[string]bool, len(submissions))
	for _, value := range submissions {
		if value.State == submission.Submitted {
			submitted[value.SubmitterID] = true
			submitted[value.ExternalID] = true
		}
	}
	reminders := make([]notification.Reminder, 0)
	for _, target := range targets {
		if submitted[target.PersonID] {
			continue
		}
		provider, ok := s.providers[target.Channel]
		if !ok {
			return reminders, fmt.Errorf("reminder provider %q is not configured", target.Channel)
		}
		existing, err := s.repository.Get(ctx, item.ID, target.PersonID, target.Channel, "missing_submission")
		if err == nil && existing.Status == notification.Sent {
			reminders = append(reminders, existing)
			continue
		}
		if err != nil && !errors.Is(err, ports.ErrReminderNotFound) {
			return reminders, err
		}
		reminder := existing
		if err != nil {
			reminder = notification.Reminder{TaskID: item.ID, PersonID: target.PersonID, Channel: target.Channel, Destination: target.Destination, Kind: "missing_submission", Status: notification.Pending, CreatedAt: now}
			reminder.Subject = fmt.Sprintf("Reminder: %s", target.TaskTitle)
			reminder.DueAt = item.DueAt
			reminder.ClassroomLink = target.ClassroomLink
			message, templateErr := renderReminderMessage(target.TaskTitle, item.DueAt, target.ClassroomLink, deadline.Sub(now))
			if templateErr != nil {
				return reminders, templateErr
			}
			reminder.Message = message
			if err := s.repository.Create(ctx, &reminder); err != nil {
				return reminders, fmt.Errorf("persist reminder: %w", err)
			}
		}
		attempted := s.clock()
		reminder.AttemptedAt = &attempted
		if err := provider.SendReminder(ctx, reminder); err != nil {
			reminder.Attempts++
			reminder.Status, reminder.Error = notification.Failed, err.Error()
			next := s.clock().Add(retryDelay(reminder.Attempts))
			reminder.NextAttemptAt = &next
		} else {
			reminder.Status, reminder.Error = notification.Sent, ""
		}
		if err := s.repository.Update(ctx, &reminder); err != nil {
			return reminders, fmt.Errorf("persist reminder status: %w", err)
		}
		reminders = append(reminders, reminder)
	}
	return reminders, nil
}

func renderReminderMessage(title, dueAt, link string, remaining time.Duration) (string, error) {
	path := os.Getenv("REMINDER_TEMPLATE_PATH")
	if path == "" {
		path = "templates/missing-submission-reminder.txt"
	}
	if !filepath.IsAbs(path) {
		if cwd, err := os.Getwd(); err == nil {
			for {
				candidate := filepath.Join(cwd, path)
				if _, err := os.Stat(candidate); err == nil {
					path = candidate
					break
				}
				parent := filepath.Dir(cwd)
				if parent == cwd {
					break
				}
				cwd = parent
			}
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read reminder template: %w", err)
	}
	message := string(body)
	for tag, value := range map[string]string{
		"<task_title>":     title,
		"<due_at>":         dueAt,
		"<remaining_time>": remaining.Round(time.Minute).String(),
		"<classroom_link>": link,
	} {
		message = strings.ReplaceAll(message, tag, value)
	}
	return message, nil
}
