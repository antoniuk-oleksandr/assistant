package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/task"
)

type HeadNotificationService struct {
	teachers  ports.TeacherRepository
	reminders *ReminderService
}

func NewHeadNotificationService(teachers ports.TeacherRepository, reminders *ReminderService) *HeadNotificationService {
	return &HeadNotificationService{teachers: teachers, reminders: reminders}
}

func (s *HeadNotificationService) Schedule(ctx context.Context, item task.Task, missing []ports.MissingSubmitter) error {
	if s.teachers == nil || s.reminders == nil || len(missing) == 0 {
		return nil
	}
	recipients, err := s.teachers.List(ctx)
	if err != nil {
		return fmt.Errorf("list head recipients: %w", err)
	}
	message := missingMessage(item, missing)
	targets := make([]ports.ReminderTarget, 0)
	for _, recipient := range recipients {
		if recipient.Status != "active" {
			continue
		}
		channels, err := s.teachers.ListActiveChannels(ctx, recipient.ID)
		if err != nil {
			return fmt.Errorf("list channels for head %d: %w", recipient.ID, err)
		}
		for _, channel := range channels {
			targets = append(targets, ports.ReminderTarget{PersonID: fmt.Sprint(recipient.ID), Channel: channel.Kind, Destination: channel.Destination, TaskTitle: item.Title, Kind: "missing_submitters_head", Subject: fmt.Sprintf("Missing submissions: %s", item.Title), Message: message})
		}
	}
	if _, err := s.reminders.Schedule(ctx, item, nil, targets, 72*time.Hour); err != nil {
		return fmt.Errorf("schedule head notifications: %w", err)
	}
	return nil
}

func missingMessage(item task.Task, missing []ports.MissingSubmitter) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Missing submissions for %s\n\nDue date: %s\n\n", item.Title, item.DueAt)
	for _, person := range missing {
		fmt.Fprintf(&builder, "- %s <%s>\n", person.Name, person.Email)
	}
	return builder.String()
}
