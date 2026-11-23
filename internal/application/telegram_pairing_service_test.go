package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
func (r *pairingRepository) ConsumeTelegramPairing(_ context.Context, id int64, usedAt time.Time) error {
	if r.consumeErr != nil {
		return r.consumeErr
	}
	if id != r.item.ID {
		return errors.New("wrong pairing")
	}
	r.consumed = true
	r.item.UsedAt = &usedAt
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

var _ = Describe("Telegram pairing service", func() {
	It("binds a private chat from a pairing link", func() {
		repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3, Email: "admin@example.com", Status: teacher.Active}}
		pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, TokenHash: "hash", ExpiresAt: time.Now().Add(time.Hour)}}
		messenger := &pairingMessenger{}
		ctx, cancel := context.WithCancel(context.Background())
		service := application.NewTelegramPairingService(repository, pairings, &pairingUpdates{cancel: cancel, items: []ports.TelegramUpdate{
			{ID: 1, ChatID: "483920123", ChatType: "private", Text: "/start pair_token"},
		}}, messenger)
		Expect(errors.Is(service.Run(ctx), context.Canceled)).To(BeTrue())
		Expect(repository.channels).To(HaveLen(1))
		Expect(repository.channels[0].Kind).To(Equal("telegram"))
		Expect(repository.channels[0].Destination).To(Equal("483920123"))
		Expect(pairings.consumed).To(BeTrue())
	})

	It("consumes a pairing when the Telegram chat is already bound", func() {
		repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}, channels: []teacher.Channel{{RecipientID: 3, Kind: "telegram", Destination: "483920123", Status: teacher.Active}}}
		pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(time.Hour)}}
		ctx, cancel := context.WithCancel(context.Background())
		service := application.NewTelegramPairingService(repository, pairings, &pairingUpdates{cancel: cancel, items: []ports.TelegramUpdate{{ID: 1, ChatID: "483920123", ChatType: "private", Text: "/start pair_token"}}}, &pairingMessenger{})
		Expect(errors.Is(service.Run(ctx), context.Canceled)).To(BeTrue())
		Expect(repository.channels).To(HaveLen(1))
		Expect(pairings.consumed).To(BeTrue())
	})

	It("creates a Telegram pairing link", func() {
		repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3, ClassroomID: "classroom-user-3"}}
		pairings := &pairingRepository{}
		service := application.NewTeacherServiceWithDirectoryAndTelegram(repository, nil, pairings, application.TelegramSettings{BotUsername: "@my_bot"})
		link, err := service.CreateTelegramPairing(context.Background(), "classroom-user-3")
		Expect(err).NotTo(HaveOccurred())
		Expect(link).To(HavePrefix("https://t.me/my_bot?start=pair_"))
		Expect(pairings.item.RecipientID).To(Equal(int64(3)))
		Expect(pairings.item.TokenHash).NotTo(BeEmpty())
	})

	It("waits for a Telegram pairing to be consumed", func() {
		usedAt := time.Now()
		pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, ExpiresAt: time.Now().Add(time.Hour), UsedAt: &usedAt}}
		service := application.NewTeacherServiceWithDirectoryAndTelegram(&pairingTeacherRepository{}, nil, pairings, application.TelegramSettings{BotUsername: "bot"})
		Expect(service.WaitForTelegramPairing(context.Background(), "https://t.me/bot?start=pair_token")).To(Succeed())
	})

	It("rejects malformed and expired Telegram pairing links", func() {
		service := application.NewTeacherServiceWithDirectoryAndTelegram(&pairingTeacherRepository{}, nil, &pairingRepository{}, application.TelegramSettings{BotUsername: "bot"})
		Expect(service.WaitForTelegramPairing(context.Background(), "not-a-link")).To(MatchError(ContainSubstring("does not contain")))

		pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, ExpiresAt: time.Now().Add(-time.Hour)}}
		service = application.NewTeacherServiceWithDirectoryAndTelegram(&pairingTeacherRepository{}, nil, pairings, application.TelegramSettings{BotUsername: "bot"})
		Expect(service.WaitForTelegramPairing(context.Background(), "https://t.me/bot?start=pair_token")).To(MatchError("Telegram pairing link expired"))
	})

	It("rejects an unconfigured Telegram pairing", func() {
		service := application.NewTeacherServiceWithDirectoryAndTelegram(&pairingTeacherRepository{}, nil, nil, application.TelegramSettings{})
		_, err := service.CreateTelegramPairing(context.Background(), "teacher")
		Expect(err).To(MatchError(ContainSubstring("not configured")))
	})

	It("resolves numeric and missing pairing recipients", func() {
		repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3, ClassroomID: "classroom-user-3"}}
		pairings := &pairingRepository{}
		service := application.NewTeacherServiceWithDirectoryAndTelegram(repository, nil, pairings, application.TelegramSettings{BotUsername: "bot"})
		_, err := service.CreateTelegramPairing(context.Background(), "3")
		Expect(err).NotTo(HaveOccurred())
		_, err = service.CreateTelegramPairing(context.Background(), "missing")
		Expect(err).To(HaveOccurred())
		repository.listErr = errors.New("list failed")
		_, err = service.CreateTelegramPairing(context.Background(), "not-a-number")
		Expect(err).To(MatchError(ContainSubstring("list failed")))
	})

	It("reports Telegram pairing persistence failures", func() {
		repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3, ClassroomID: "classroom-user-3"}}
		service := application.NewTeacherServiceWithDirectoryAndTelegram(repository, nil, &pairingRepository{createErr: errors.New("pairing write failed")}, application.TelegramSettings{BotUsername: "bot"})
		_, err := service.CreateTelegramPairing(context.Background(), "classroom-user-3")
		Expect(err).To(MatchError(ContainSubstring("pairing write failed")))
	})

	It("rejects invalid or expired pairings", func() {
		repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}}
		pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(-time.Hour)}}
		messenger := &pairingMessenger{}
		service := application.NewTelegramPairingService(repository, pairings, nil, messenger)
		// Run is intentionally disabled without an update source; the service must be a no-op.
		Expect(service.Run(context.Background())).To(Succeed())
	})

	It("ignores non-private messages", func() {
		repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}}
		pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(time.Hour)}}
		messenger := &pairingMessenger{}
		ctx, cancel := context.WithCancel(context.Background())
		updates := &pairingUpdates{cancel: cancel, items: []ports.TelegramUpdate{{ID: 1, ChatID: "group", ChatType: "group", Text: "/start pair_token"}}}
		service := application.NewTelegramPairingService(repository, pairings, updates, messenger)
		Expect(errors.Is(service.Run(ctx), context.Canceled)).To(BeTrue())
		Expect(repository.channels).To(BeEmpty())
		Expect(messenger.messages).To(BeEmpty())
	})

	It("ignores non-pairing private messages", func() {
		ctx, cancel := context.WithCancel(context.Background())
		updates := &pairingUpdates{cancel: cancel, items: []ports.TelegramUpdate{{ID: 1, ChatID: "chat", ChatType: "private", Text: "hello"}}}
		service := application.NewTelegramPairingService(&pairingTeacherRepository{}, &pairingRepository{}, updates, &pairingMessenger{})
		Expect(errors.Is(service.Run(ctx), context.Canceled)).To(BeTrue())
	})

	It("reports expired pairings", func() {
		repository := &pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}}
		pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(-time.Hour)}}
		messenger := &pairingMessenger{}
		ctx, cancel := context.WithCancel(context.Background())
		updates := &pairingUpdates{cancel: cancel, items: []ports.TelegramUpdate{{ID: 1, ChatID: "chat", ChatType: "private", Text: "/start pair_token"}}}
		service := application.NewTelegramPairingService(repository, pairings, updates, messenger)
		Expect(errors.Is(service.Run(ctx), context.Canceled)).To(BeTrue())
		Expect(messenger.messages).To(HaveLen(1))
		Expect(messenger.messages[0]).To(ContainSubstring("invalid or expired"))
	})

	It("returns update errors", func() {
		want := errors.New("updates unavailable")
		service := application.NewTelegramPairingService(&pairingTeacherRepository{}, &pairingRepository{}, &pairingUpdates{err: want}, &pairingMessenger{})
		Expect(errors.Is(service.Run(context.Background()), want)).To(BeTrue())
	})

	It("returns pairing consume errors", func() {
		pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(time.Hour)}, consumeErr: errors.New("consume failed")}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		updates := &pairingUpdates{items: []ports.TelegramUpdate{{ID: 1, ChatID: "chat", ChatType: "private", Text: "/start pair_token"}}, cancel: cancel}
		service := application.NewTelegramPairingService(&pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}}, pairings, updates, &pairingMessenger{})
		Expect(service.Run(ctx)).To(MatchError(ContainSubstring("consume failed")))
	})

	It("rechecks the pairing before binding", func() {
		pairings := &pairingRepository{item: teacher.TelegramPairing{ID: 9, RecipientID: 3, ExpiresAt: time.Now().Add(time.Hour)}, getErrAfter: 1}
		updates := &pairingUpdates{items: []ports.TelegramUpdate{{ID: 1, ChatID: "chat", ChatType: "private", Text: "/start pair_token"}}}
		service := application.NewTelegramPairingService(&pairingTeacherRepository{recipient: teacher.Recipient{ID: 3}}, pairings, updates, &pairingMessenger{})
		Expect(service.Run(context.Background())).To(MatchError(ContainSubstring("pairing changed")))
	})

	It("propagates binding errors", func() {
		basePairing := teacher.TelegramPairing{ID: 9, RecipientID: 3, TokenHash: "hash", ExpiresAt: time.Now().Add(time.Hour)}
		for name, repository := range map[string]*pairingTeacherRepository{
			"teacher lookup": {recipient: teacher.Recipient{ID: 3}, getErr: errors.New("teacher lookup failed")},
			"channel create": {recipient: teacher.Recipient{ID: 3}, channelErr: errors.New("channel failed")},
		} {
			By(name)
			pairings := &pairingRepository{item: basePairing}
			if name == "channel create" {
				repository.channelErr = errors.New("channel failed")
			}
			service := application.NewTelegramPairingService(repository, pairings, &pairingUpdates{items: []ports.TelegramUpdate{{ID: 1, ChatID: "chat", ChatType: "private", Text: "/start pair_token"}}}, &pairingMessenger{})
			Expect(service.Run(context.Background())).To(HaveOccurred())
		}
	})
})
