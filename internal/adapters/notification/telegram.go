package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/platform/config"
)

type telegramProvider struct {
	token, baseURL, chatID string
	client                 *http.Client
}

func NewTelegramProvider(cfg config.TelegramConfig) *telegramProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	return &telegramProvider{token: cfg.Token, baseURL: strings.TrimRight(baseURL, "/"), chatID: cfg.ChatID, client: http.DefaultClient}
}

func (p *telegramProvider) Name() string { return "telegram" }

func (p *telegramProvider) SendReminder(ctx context.Context, item notification.Reminder) error {
	return p.send(ctx, p.destination(item.Destination), item.Subject+"\n\n"+item.Message)
}

func (p *telegramProvider) Deliver(ctx context.Context, result report.Result, destination string) error {
	return p.send(ctx, p.destination(destination), fmt.Sprintf("Final scientific activity report is ready for task %d.", result.Data.TaskID))
}

func (p *telegramProvider) SendReportReady(ctx context.Context, destination, message string) error {
	return p.send(ctx, p.destination(destination), message)
}

func (p *telegramProvider) destination(destination string) string {
	if strings.TrimSpace(destination) != "" {
		return destination
	}
	return p.chatID
}

func (p *telegramProvider) send(ctx context.Context, chatID, text string) error {
	if p.token == "" {
		return fmt.Errorf("Telegram bot token is not configured")
	}
	payload, err := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/bot"+p.token+"/sendMessage", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Telegram API returned %s", response.Status)
	}
	return nil
}

var _ ports.ReminderProvider = (*telegramProvider)(nil)
var _ ports.ResultDeliveryProvider = (*telegramProvider)(nil)
var _ ports.ReportReadyTelegramSender = (*telegramProvider)(nil)
