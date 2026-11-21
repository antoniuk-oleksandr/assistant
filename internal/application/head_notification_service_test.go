package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/domain/teacher"
)

type headTeacherRepository struct {
	listErr, channelErr error
}

func (headTeacherRepository) Create(context.Context, *teacher.Recipient) error { return nil }
func (headTeacherRepository) Update(context.Context, *teacher.Recipient) error { return nil }
func (headTeacherRepository) Get(context.Context, int64) (teacher.Recipient, error) {
	return teacher.Recipient{}, ports.ErrTeacherNotFound
}
func (r headTeacherRepository) List(context.Context) ([]teacher.Recipient, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return []teacher.Recipient{{ID: 7, Name: "Head", Email: "head@example.com", Status: teacher.Active}}, nil
}
func (headTeacherRepository) CreateChannel(context.Context, *teacher.Channel) error { return nil }
func (headTeacherRepository) UpdateChannel(context.Context, *teacher.Channel) error { return nil }
func (r headTeacherRepository) ListActiveChannels(context.Context, int64) ([]teacher.Channel, error) {
	if r.channelErr != nil {
		return nil, r.channelErr
	}
	return []teacher.Channel{{RecipientID: 7, Kind: "email", Destination: "head@example.com", Status: teacher.Active}}, nil
}

func TestHeadNotificationServiceSchedulesMissingPeople(t *testing.T) {
	service := NewHeadNotificationService(headTeacherRepository{}, NewReminderServiceWithClock(reminderPollingRepository{}, []ports.ReminderProvider{reminderPollingProvider{}}, func() time.Time { return time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC) }))
	err := service.Schedule(context.Background(), task.Task{ID: 1, Title: "Report", DueAt: "2026-11-20T00:00:00Z"}, []ports.MissingSubmitter{{Name: "Teacher", Email: "teacher@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHeadNotificationServiceSkipsEmptyInput(t *testing.T) {
	service := NewHeadNotificationService(nil, nil)
	if err := service.Schedule(context.Background(), task.Task{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestHeadNotificationServicePropagatesRecipientErrors(t *testing.T) {
	item := task.Task{ID: 1, Title: "Report", DueAt: "2026-11-20T00:00:00Z"}
	service := NewHeadNotificationService(headTeacherRepository{listErr: errors.New("teacher list failed")}, NewReminderService(reminderPollingRepository{}, nil))
	if err := service.Schedule(context.Background(), item, []ports.MissingSubmitter{{Name: "Teacher"}}); err == nil {
		t.Fatal("recipient error was ignored")
	}
	service = NewHeadNotificationService(headTeacherRepository{channelErr: errors.New("channel list failed")}, NewReminderService(reminderPollingRepository{}, nil))
	if err := service.Schedule(context.Background(), item, []ports.MissingSubmitter{{Name: "Teacher"}}); err == nil {
		t.Fatal("channel error was ignored")
	}
}
