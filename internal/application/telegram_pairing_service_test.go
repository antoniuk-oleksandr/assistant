package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
	"github.com/alex/assistent/internal/platform/config"
)

type pairingTeacherRepository struct {
	recipient teacher.Recipient
	channels  []teacher.Channel
}

func (r *pairingTeacherRepository) Create(context.Context, *teacher.Recipient) error { return nil }
func (r *pairingTeacherRepository) Update(context.Context, *teacher.Recipient) error { return nil }
func (r *pairingTeacherRepository) Get(context.Context, int64) (teacher.Recipient, error) {
	return r.recipient, nil
}
func (r *pairingTeacherRepository) List(context.Context) ([]teacher.Recipient, error) {
	return []teacher.Recipient{r.recipient}, nil
}
func (r *pairingTeacherRepository) CreateChannel(_ context.Context, channel *teacher.Channel) error {
	r.channels = append(r.channels, *channel)
	return nil
}
func (r *pairingTeacherRepository) UpdateChannel(context.Context, *teacher.Channel) error { return nil }
func (r *pairingTeacherRepository) ListActiveChannels(context.Context, int64) ([]teacher.Channel, error) {
	return r.channels, nil
}

type pairingRepository struct {
	item     teacher.TelegramPairing
	consumed bool
}

func (r *pairingRepository) CreateTelegramPairing(_ context.Context, item *teacher.TelegramPairing) error {
	r.item = *item
	return nil
}
func (r *pairingRepository) GetTelegramPairing(context.Context, string) (teacher.TelegramPairing, error) {
	return r.item, nil
}
func (r *pairingRepository) ConsumeTelegramPairing(_ context.Context, id int64, _ time.Time) error {
	if id != r.item.ID {
		return errors.New("wrong pairing")
	}
	r.consumed = true
	return nil
}

type pairingUpdates struct {
	items  []ports.TelegramUpdate
	called bool
	cancel context.CancelFunc
}

func (s *pairingUpdates) GetUpdates(ctx context.Context, _ int64, _ int) ([]ports.TelegramUpdate, error) {
	if !s.called {
		s.called = true
		return s.items, nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	return nil, ctx.Err()
}

type pairingMessenger struct {
	messages []string
}

func (m *pairingMessenger) SendMessage(_ context.Context, _ string, message string) error {
	m.messages = append(m.messages, message)
	return nil
}
func TestTelegramPairingServiceBindsPrivateChatFromPairingLink(t *testing.T) {
	repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3, Email: "admin@example.com", Status: teacher.Active}}
	pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, TokenHash: "hash", ExpiresAt: time.Now().Add(time.Hour)}}
	messenger := &pairingMessenger{}
	ctx, cancel := context.WithCancel(context.Background())
	service := application.NewTelegramPairingService(repository, pairings, &pairingUpdates{cancel: cancel, items: []ports.TelegramUpdate{
		{ID: 1, ChatID: "483920123", ChatType: "private", Text: "/start pair_token"},
	}}, messenger)
	if err := service.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
	if len(repository.channels) != 1 || repository.channels[0].Kind != "telegram" || repository.channels[0].Destination != "483920123" || !pairings.consumed {
		t.Fatalf("repository=%#v pairing=%#v messenger=%#v", repository, pairings, messenger)
	}
}

func TestTeacherServiceCreatesTelegramPairingLink(t *testing.T) {
	repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3, ClassroomID: "classroom-user-3"}}
	pairings := &pairingRepository{}
	service := application.NewTeacherServiceWithDirectoryAndTelegram(repository, nil, pairings, config.TelegramConfig{Username: "@my_bot"})
	link, err := service.CreateTelegramPairing(context.Background(), "classroom-user-3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "https://t.me/my_bot?start=pair_") || pairings.item.RecipientID != 3 || pairings.item.TokenHash == "" {
		t.Fatalf("link=%q pairing=%#v", link, pairings.item)
	}
}
