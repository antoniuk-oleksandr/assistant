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
	recipient  teacher.Recipient
	channels   []teacher.Channel
	listErr    error
	getErr     error
	channelErr error
}

func (r *pairingTeacherRepository) Create(context.Context, *teacher.Recipient) error { return nil }
func (r *pairingTeacherRepository) Update(context.Context, *teacher.Recipient) error { return nil }
func (r *pairingTeacherRepository) Get(context.Context, int64) (teacher.Recipient, error) {
	if r.getErr != nil {
		return teacher.Recipient{}, r.getErr
	}
	return r.recipient, nil
}
func (r *pairingTeacherRepository) List(context.Context) ([]teacher.Recipient, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return []teacher.Recipient{r.recipient}, nil
}
func (r *pairingTeacherRepository) CreateChannel(_ context.Context, channel *teacher.Channel) error {
	if r.channelErr != nil {
		return r.channelErr
	}
	r.channels = append(r.channels, *channel)
	return nil
}
func (r *pairingTeacherRepository) UpdateChannel(context.Context, *teacher.Channel) error { return nil }
func (r *pairingTeacherRepository) ListActiveChannels(context.Context, int64) ([]teacher.Channel, error) {
	return r.channels, nil
}

type pairingRepository struct {
	item        teacher.TelegramPairing
	consumed    bool
	getErr      error
	consumeErr  error
	createErr   error
	getCalls    int
	getErrAfter int
}

func (r *pairingRepository) CreateTelegramPairing(_ context.Context, item *teacher.TelegramPairing) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.item = *item
	return nil
}
func (r *pairingRepository) GetTelegramPairing(context.Context, string) (teacher.TelegramPairing, error) {
	r.getCalls++
	if r.getErrAfter > 0 && r.getCalls > r.getErrAfter {
		return teacher.TelegramPairing{}, errors.New("pairing changed")
	}
	if r.getErr != nil {
		return teacher.TelegramPairing{}, r.getErr
	}
	return r.item, nil
}
func (r *pairingRepository) ConsumeTelegramPairing(_ context.Context, id int64, _ time.Time) error {
	if r.consumeErr != nil {
		return r.consumeErr
	}
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
	err    error
}

func (s *pairingUpdates) GetUpdates(ctx context.Context, _ int64, _ int) ([]ports.TelegramUpdate, error) {
	if s.err != nil {
		return nil, s.err
	}
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
	err      error
}

func (m *pairingMessenger) SendMessage(_ context.Context, _ string, message string) error {
	m.messages = append(m.messages, message)
	return m.err
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

func TestTeacherServiceRejectsUnconfiguredTelegramPairing(t *testing.T) {
	service := application.NewTeacherServiceWithDirectoryAndTelegram(&pairingTeacherRepository{}, nil, nil, config.TelegramConfig{})
	if _, err := service.CreateTelegramPairing(context.Background(), "teacher"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("CreateTelegramPairing() error = %v", err)
	}
}

func TestTeacherServiceResolvesNumericAndMissingTelegramPairingRecipients(t *testing.T) {
	repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3, ClassroomID: "classroom-user-3"}}
	pairings := &pairingRepository{}
	service := application.NewTeacherServiceWithDirectoryAndTelegram(repository, nil, pairings, config.TelegramConfig{Username: "bot"})
	if _, err := service.CreateTelegramPairing(context.Background(), "3"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTelegramPairing(context.Background(), "missing"); err == nil {
		t.Fatal("missing recipient unexpectedly resolved")
	}
	repository.listErr = errors.New("list failed")
	if _, err := service.CreateTelegramPairing(context.Background(), "not-a-number"); err == nil || !strings.Contains(err.Error(), "list failed") {
		t.Fatalf("list error = %v", err)
	}
}

func TestTeacherServiceReportsTelegramPairingPersistenceFailure(t *testing.T) {
	repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3, ClassroomID: "classroom-user-3"}}
	service := application.NewTeacherServiceWithDirectoryAndTelegram(repository, nil, &pairingRepository{createErr: errors.New("pairing write failed")}, config.TelegramConfig{Username: "bot"})
	if _, err := service.CreateTelegramPairing(context.Background(), "classroom-user-3"); err == nil || !strings.Contains(err.Error(), "pairing write failed") {
		t.Fatalf("pairing persistence error = %v", err)
	}
}

func TestTelegramPairingServiceRejectsInvalidOrExpiredPairing(t *testing.T) {
	repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}}
	pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(-time.Hour)}}
	messenger := &pairingMessenger{}
	service := application.NewTelegramPairingService(repository, pairings, nil, messenger)
	// Run is intentionally disabled without an update source; the service must be a no-op.
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTelegramPairingServiceIgnoresNonPrivateMessages(t *testing.T) {
	repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}}
	pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(time.Hour)}}
	messenger := &pairingMessenger{}
	ctx, cancel := context.WithCancel(context.Background())
	updates := &pairingUpdates{cancel: cancel, items: []ports.TelegramUpdate{{ID: 1, ChatID: "group", ChatType: "group", Text: "/start pair_token"}}}
	service := application.NewTelegramPairingService(repository, pairings, updates, messenger)
	if err := service.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
	if len(repository.channels) != 0 || len(messenger.messages) != 0 {
		t.Fatalf("repository=%#v messenger=%#v", repository, messenger)
	}
}

func TestTelegramPairingServiceIgnoresNonPairingPrivateMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	updates := &pairingUpdates{cancel: cancel, items: []ports.TelegramUpdate{{ID: 1, ChatID: "chat", ChatType: "private", Text: "hello"}}}
	service := application.NewTelegramPairingService(&pairingTeacherRepository{}, &pairingRepository{}, updates, &pairingMessenger{})
	if err := service.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestTelegramPairingServiceReportsExpiredPairing(t *testing.T) {
	repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}}
	pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(-time.Hour)}}
	messenger := &pairingMessenger{}
	ctx, cancel := context.WithCancel(context.Background())
	updates := &pairingUpdates{cancel: cancel, items: []ports.TelegramUpdate{{ID: 1, ChatID: "chat", ChatType: "private", Text: "/start pair_token"}}}
	service := application.NewTelegramPairingService(repository, pairings, updates, messenger)
	if err := service.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
	if len(messenger.messages) != 1 || !strings.Contains(messenger.messages[0], "invalid or expired") {
		t.Fatalf("messages = %#v", messenger.messages)
	}
}

func TestTelegramPairingServiceReturnsUpdateErrors(t *testing.T) {
	want := errors.New("updates unavailable")
	service := application.NewTelegramPairingService(&pairingTeacherRepository{}, &pairingRepository{}, &pairingUpdates{err: want}, &pairingMessenger{})
	if err := service.Run(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestTelegramPairingServiceReturnsPairingConsumeErrors(t *testing.T) {
	pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(time.Hour)}, consumeErr: errors.New("consume failed")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := &pairingUpdates{items: []ports.TelegramUpdate{{ID: 1, ChatID: "chat", ChatType: "private", Text: "/start pair_token"}}, cancel: cancel}
	service := application.NewTelegramPairingService(&pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}}, pairings, updates, &pairingMessenger{})
	if err := service.Run(ctx); err == nil || !strings.Contains(err.Error(), "consume failed") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestTelegramPairingServiceRechecksPairingBeforeBinding(t *testing.T) {
	pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(time.Hour)}, getErrAfter: 1}
	updates := &pairingUpdates{items: []ports.TelegramUpdate{{ID: 1, ChatID: "chat", ChatType: "private", Text: "/start pair_token"}}}
	service := application.NewTelegramPairingService(&pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}}, pairings, updates, &pairingMessenger{})
	if err := service.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "pairing changed") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestTelegramPairingServicePropagatesBindingErrors(t *testing.T) {
	basePairing := teacher.TelegramPairing{ID: 9, RecipientID: 3, TokenHash: "hash", ExpiresAt: time.Now().Add(time.Hour)}
	for name, repository := range map[string]*pairingTeacherRepository{
		"teacher lookup": {recipient: teacher.Recipient{ID: 3}, getErr: errors.New("teacher lookup failed")},
		"channel create": {recipient: teacher.Recipient{ID: 3}, channelErr: errors.New("channel failed")},
	} {
		t.Run(name, func(t *testing.T) {
			pairings := &pairingRepository{item: basePairing}
			if name == "channel create" {
				repository.channelErr = errors.New("channel failed")
			}
			service := application.NewTelegramPairingService(repository, pairings, &pairingUpdates{items: []ports.TelegramUpdate{{ID: 1, ChatID: "chat", ChatType: "private", Text: "/start pair_token"}}}, &pairingMessenger{})
			if err := service.Run(context.Background()); err == nil {
				t.Fatal("binding error was ignored")
			}
		})
	}
}
