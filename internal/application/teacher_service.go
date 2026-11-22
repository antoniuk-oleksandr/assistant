package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
)

type TeacherDirectoryEntry struct {
	Recipient teacher.Recipient
	Phone     string
}

type TeacherService struct {
	repository ports.TeacherRepository
	clock      func() time.Time
	people     ports.ParticipantSource
}

func NewTeacherService(repository ports.TeacherRepository) *TeacherService {
	return &TeacherService{repository: repository, clock: func() time.Time { return time.Now().UTC() }}
}

func NewTeacherServiceWithDirectory(repository ports.TeacherRepository, people ports.ParticipantSource) *TeacherService {
	service := NewTeacherService(repository)
	service.people = people
	return service
}

func (s *TeacherService) List(ctx context.Context) ([]teacher.Recipient, error) {
	return s.repository.List(ctx)
}

func (s *TeacherService) ListForCourse(ctx context.Context, reference string) ([]TeacherDirectoryEntry, error) {
	if s.people == nil {
		return nil, errors.New("Classroom participant source is not configured")
	}
	courseID, err := classroom.NormalizeReference(reference)
	if err != nil {
		return nil, err
	}
	people, err := s.people.ListParticipants(ctx, courseID)
	if err != nil {
		return nil, fmt.Errorf("list teachers for course %q: %w", courseID, err)
	}
	existing, err := s.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list persisted teachers: %w", err)
	}
	entries := make([]TeacherDirectoryEntry, 0, len(people))
	seen := make(map[string]struct{}, len(people))
	for _, person := range people {
		if _, ok := seen[person.ExternalID]; ok {
			continue
		}
		seen[person.ExternalID] = struct{}{}
		current := teacher.Recipient{}
		for _, item := range existing {
			if item.ClassroomID == person.ExternalID {
				current = item
				break
			}
		}
		if current.ID == 0 {
			current = teacher.Recipient{Name: person.Name, Email: person.Email, ClassroomID: person.ExternalID, Status: teacher.Active, CreatedAt: s.clock(), UpdatedAt: s.clock()}
			if err := s.repository.Create(ctx, &current); err != nil {
				return nil, fmt.Errorf("store Classroom teacher %q: %w", person.ExternalID, err)
			}
			existing = append(existing, current)
		} else if current.Name != person.Name || current.Email != person.Email {
			current.Name, current.Email, current.UpdatedAt = person.Name, person.Email, s.clock()
			if err := s.repository.Update(ctx, &current); err != nil {
				return nil, fmt.Errorf("update Classroom teacher %q: %w", person.ExternalID, err)
			}
		}
		channels, err := s.repository.ListActiveChannels(ctx, current.ID)
		if err != nil {
			return nil, fmt.Errorf("list channels for Classroom teacher %q: %w", person.ExternalID, err)
		}
		entry := TeacherDirectoryEntry{Recipient: current}
		for _, channel := range channels {
			if channel.Kind == "phone" {
				entry.Phone = channel.Destination
				break
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
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

// SetPhoneByIdentifier accepts either the local teacher ID or the stable
// Google Classroom user ID shown by `teacher list`.
func (s *TeacherService) SetPhoneByIdentifier(ctx context.Context, identifier, phone string) (teacher.Channel, error) {
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil && id > 0 {
		if channel, lookupErr := s.SetPhone(ctx, id, phone); lookupErr == nil {
			return channel, nil
		}
	}
	items, err := s.repository.List(ctx)
	if err != nil {
		return teacher.Channel{}, fmt.Errorf("find teacher %q: %w", identifier, err)
	}
	for _, item := range items {
		if item.ClassroomID == identifier {
			return s.SetPhone(ctx, item.ID, phone)
		}
	}
	return teacher.Channel{}, fmt.Errorf("teacher %q was not found", identifier)
}
func (s *TeacherService) SetChannelStatus(ctx context.Context, v teacher.Channel, status teacher.Status) error {
	v.Status = status
	v.UpdatedAt = s.clock()
	return s.repository.UpdateChannel(ctx, &v)
}
func (s *TeacherService) ActiveChannels(ctx context.Context, id int64) ([]teacher.Channel, error) {
	return s.repository.ListActiveChannels(ctx, id)
}
