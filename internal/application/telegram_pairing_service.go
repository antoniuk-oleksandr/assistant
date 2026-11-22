package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/teacher"
)

type TelegramPairingService struct {
	teachers  ports.TeacherRepository
	pairings  ports.TelegramPairingRepository
	updates   ports.TelegramUpdateSource
	messenger ports.TelegramPairingMessenger
	clock     func() time.Time
	mu        sync.Mutex
	pending   map[string]pendingTelegramPairing
}

type pendingTelegramPairing struct {
	id   int64
	hash string
}

func NewTelegramPairingService(teachers ports.TeacherRepository, pairings ports.TelegramPairingRepository, updates ports.TelegramUpdateSource, messenger ports.TelegramPairingMessenger) *TelegramPairingService {
	return &TelegramPairingService{teachers: teachers, pairings: pairings, updates: updates, messenger: messenger, clock: func() time.Time { return time.Now().UTC() }, pending: make(map[string]pendingTelegramPairing)}
}

func (s *TelegramPairingService) Run(ctx context.Context) error {
	if s.updates == nil || s.messenger == nil {
		return nil
	}
	var offset int64
	for {
		updates, err := s.updates.GetUpdates(ctx, offset, 50)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		for _, update := range updates {
			if err := s.handle(ctx, update); err != nil {
				return err
			}
			if update.ID >= offset {
				offset = update.ID + 1
			}
		}
	}
}

func (s *TelegramPairingService) handle(ctx context.Context, update ports.TelegramUpdate) error {
	if update.ChatType != "private" || update.ChatID == "" {
		return nil
	}
	if strings.HasPrefix(update.Text, "/start pair_") {
		token := strings.TrimPrefix(update.Text, "/start pair_")
		hash := sha256.Sum256([]byte(token))
		pairing, err := s.pairings.GetTelegramPairing(ctx, fmt.Sprintf("%x", hash))
		if err != nil || pairing.UsedAt != nil || !s.clock().Before(pairing.ExpiresAt) {
			return s.messenger.SendMessage(ctx, update.ChatID, "This Telegram pairing link is invalid or expired.")
		}
		s.mu.Lock()
		s.pending[update.ChatID] = pendingTelegramPairing{id: pairing.ID, hash: fmt.Sprintf("%x", hash)}
		s.mu.Unlock()
		return s.bind(ctx, update.ChatID, pairing)
	}
	return nil
}

func (s *TelegramPairingService) bind(ctx context.Context, chatID string, pairing teacher.TelegramPairing) error {
	s.mu.Lock()
	pending, ok := s.pending[chatID]
	if ok {
		delete(s.pending, chatID)
	}
	s.mu.Unlock()
	if ok {
		var err error
		// The pairing repository is queried again so expiry and one-time use are
		// enforced even when the command is processed after a delay.
		pairing, err = s.pairings.GetTelegramPairing(ctx, pending.hash)
		if err != nil {
			return err
		}
	}
	recipient, err := s.teachers.Get(ctx, pairing.RecipientID)
	if err != nil {
		return err
	}
	now := s.clock()
	channel := teacher.Channel{RecipientID: recipient.ID, Kind: "telegram", Destination: chatID, Status: teacher.Active, CreatedAt: now, UpdatedAt: now}
	if err := s.teachers.CreateChannel(ctx, &channel); err != nil {
		return err
	}
	if err := s.pairings.ConsumeTelegramPairing(ctx, pairing.ID, now); err != nil {
		return err
	}
	return s.messenger.SendMessage(ctx, chatID, "Your Telegram chat is connected.")
}
