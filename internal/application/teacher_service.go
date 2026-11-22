package application

import (
	"context"
	"fmt"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
	"time"
)

type TeacherService struct {
	repository ports.TeacherRepository
	clock      func() time.Time
}

func NewTeacherService(repository ports.TeacherRepository) *TeacherService {
	return &TeacherService{repository: repository, clock: func() time.Time { return time.Now().UTC() }}
}

func (s *TeacherService) List(ctx context.Context) ([]teacher.Recipient, error) {
	return s.repository.List(ctx)
}
func (s *TeacherService) Add(ctx context.Context, name, email, classroomID string) (teacher.Recipient, error) {
	now := s.clock()
	v := teacher.Recipient{Name: name, Email: email, ClassroomID: classroomID, Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	if err := s.repository.Create(ctx, &v); err != nil {
		return v, fmt.Errorf("add teacher: %w", err)
	}
	return v, nil
}
func (s *TeacherService) Edit(ctx context.Context, v teacher.Recipient) error {
	v.UpdatedAt = s.clock()
	return s.repository.Update(ctx, &v)
}
func (s *TeacherService) SetStatus(ctx context.Context, id int64, status teacher.Status) error {
	v, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	v.Status = status
	return s.Edit(ctx, v)
}
func (s *TeacherService) AddChannel(ctx context.Context, recipientID int64, kind, destination string) (teacher.Channel, error) {
	if kind != "phone" && kind != "email" && kind != "telegram" {
		return teacher.Channel{}, fmt.Errorf("unsupported notification channel %q", kind)
	}
	now := s.clock()
	v := teacher.Channel{RecipientID: recipientID, Kind: kind, Destination: destination, Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	if err := s.repository.CreateChannel(ctx, &v); err != nil {
		return v, fmt.Errorf("add notification channel: %w", err)
	}
	return v, nil
}

func (s *TeacherService) SetPhone(ctx context.Context, recipientID int64, phone string) (teacher.Channel, error) {
	if recipientID <= 0 || phone == "" {
		return teacher.Channel{}, fmt.Errorf("teacher ID and phone number are required")
	}
	if _, err := s.repository.Get(ctx, recipientID); err != nil {
		return teacher.Channel{}, fmt.Errorf("find teacher %d: %w", recipientID, err)
	}
	now := s.clock()
	channel := teacher.Channel{RecipientID: recipientID, Kind: "phone", Destination: phone, Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	if err := s.repository.CreateChannel(ctx, &channel); err != nil {
		return teacher.Channel{}, fmt.Errorf("set teacher phone: %w", err)
	}
	return channel, nil
}
func (s *TeacherService) SetChannelStatus(ctx context.Context, v teacher.Channel, status teacher.Status) error {
	v.Status = status
	v.UpdatedAt = s.clock()
	return s.repository.UpdateChannel(ctx, &v)
}
func (s *TeacherService) ActiveChannels(ctx context.Context, id int64) ([]teacher.Channel, error) {
	return s.repository.ListActiveChannels(ctx, id)
}
