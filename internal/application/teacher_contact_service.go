package application

import (
	"context"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
)

type TeacherContactResult struct {
	Recipient teacher.Recipient
	Channel   teacher.Channel
}

type TeacherContactService struct {
	contacts ports.TeacherContactRepository
	profiles ports.ClassroomProfileSource
	clock    func() time.Time
}

func NewTeacherContactService(contacts ports.TeacherContactRepository, profiles ports.ClassroomProfileSource) *TeacherContactService {
	return &TeacherContactService{contacts: contacts, profiles: profiles, clock: func() time.Time { return time.Now().UTC() }}
}

func (s *TeacherContactService) RegisterPhone(ctx context.Context, classroomUserID, phone string) (TeacherContactResult, error) {
	profile, err := s.profiles.GetProfile(ctx, classroomUserID)
	if err != nil {
		return TeacherContactResult{}, fmt.Errorf("resolve Classroom profile: %w", err)
	}
	now := s.clock()
	recipient := teacher.Recipient{Name: profile.Name, Email: profile.Email, ClassroomID: profile.ClassroomID, Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	channel := teacher.Channel{Kind: "phone", Destination: phone, Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	if err := s.contacts.RegisterContact(ctx, &recipient, &channel); err != nil {
		return TeacherContactResult{}, fmt.Errorf("register teacher contact: %w", err)
	}
	return TeacherContactResult{Recipient: recipient, Channel: channel}, nil
}
