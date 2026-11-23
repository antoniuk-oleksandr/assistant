package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alex/assistent/internal/domain/classroom"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
)

type TeacherDirectoryEntry struct {
	ID          int64
	Name        string
	Email       string
	ClassroomID string
	Status      string
}

type TelegramSettings struct {
	BotUsername string
}

type TeacherService struct {
	repository          ports.TeacherRepository
	clock               func() time.Time
	people              ports.ParticipantSource
	pairings            ports.TelegramPairingRepository
	telegramBotUsername string
}

func NewTeacherService(repository ports.TeacherRepository) *TeacherService {
	return &TeacherService{repository: repository, clock: func() time.Time { return time.Now().UTC() }}
}

func NewTeacherServiceWithDirectory(repository ports.TeacherRepository, people ports.ParticipantSource) *TeacherService {
	service := NewTeacherService(repository)
	service.people = people
	return service
}

func NewTeacherServiceWithDirectoryAndTelegram(repository ports.TeacherRepository, people ports.ParticipantSource, pairings ports.TelegramPairingRepository, telegram TelegramSettings) *TeacherService {
	service := NewTeacherServiceWithDirectory(repository, people)
	service.pairings, service.telegramBotUsername = pairings, strings.TrimPrefix(telegram.BotUsername, "@")
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
		entries = append(entries, TeacherDirectoryEntry{ID: current.ID, Name: current.Name, Email: current.Email, ClassroomID: current.ClassroomID, Status: string(current.Status)})
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
	if kind != "email" && kind != "telegram" {
		return teacher.Channel{}, fmt.Errorf("unsupported notification channel %q", kind)
	}
	now := s.clock()
	v := teacher.Channel{RecipientID: recipientID, Kind: kind, Destination: destination, Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	if err := s.repository.CreateChannel(ctx, &v); err != nil {
		return v, fmt.Errorf("add notification channel: %w", err)
	}
	return v, nil
}

func (s *TeacherService) CreateTelegramPairing(ctx context.Context, identifier string) (string, error) {
	if s.pairings == nil || s.telegramBotUsername == "" {
		return "", errors.New("Telegram bot username is not configured")
	}
	item, err := s.resolveRecipient(ctx, identifier)
	if err != nil {
		return "", err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate Telegram pairing token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	pairing := teacher.TelegramPairing{RecipientID: item.ID, TokenHash: fmt.Sprintf("%x", hash), ExpiresAt: s.clock().Add(15 * time.Minute)}
	if err := s.pairings.CreateTelegramPairing(ctx, &pairing); err != nil {
		return "", fmt.Errorf("store Telegram pairing: %w", err)
	}
	return fmt.Sprintf("https://t.me/%s?start=pair_%s", s.telegramBotUsername, token), nil
}

// WaitForTelegramPairing waits until the pairing represented by link is
// consumed by the Telegram update listener.
func (s *TeacherService) WaitForTelegramPairing(ctx context.Context, link string) error {
	if s.pairings == nil {
		return errors.New("Telegram pairing repository is not configured")
	}
	parsed, err := url.Parse(link)
	if err != nil {
		return fmt.Errorf("parse Telegram pairing link: %w", err)
	}
	token := strings.TrimPrefix(parsed.Query().Get("start"), "pair_")
	if token == "" {
		return errors.New("Telegram pairing link does not contain a pairing token")
	}
	hash := sha256.Sum256([]byte(token))
	tokenHash := fmt.Sprintf("%x", hash)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		pairing, err := s.pairings.GetTelegramPairing(ctx, tokenHash)
		if err != nil {
			return fmt.Errorf("check Telegram pairing: %w", err)
		}
		if pairing.UsedAt != nil {
			return nil
		}
		if !s.clock().Before(pairing.ExpiresAt) {
			return errors.New("Telegram pairing link expired")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *TeacherService) resolveRecipient(ctx context.Context, identifier string) (teacher.Recipient, error) {
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil && id > 0 {
		if item, lookupErr := s.repository.Get(ctx, id); lookupErr == nil {
			return item, nil
		}
	}
	items, err := s.repository.List(ctx)
	if err != nil {
		return teacher.Recipient{}, fmt.Errorf("find teacher %q: %w", identifier, err)
	}
	for _, item := range items {
		if item.ClassroomID == identifier {
			return item, nil
		}
	}
	return teacher.Recipient{}, fmt.Errorf("teacher %q was not found", identifier)
}
func (s *TeacherService) SetChannelStatus(ctx context.Context, v teacher.Channel, status teacher.Status) error {
	v.Status = status
	v.UpdatedAt = s.clock()
	return s.repository.UpdateChannel(ctx, &v)
}
func (s *TeacherService) ActiveChannels(ctx context.Context, id int64) ([]teacher.Channel, error) {
	return s.repository.ListActiveChannels(ctx, id)
}
