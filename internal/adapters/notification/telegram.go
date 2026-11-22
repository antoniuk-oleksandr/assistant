package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

func (p *telegramProvider) SendMessage(ctx context.Context, chatID, message string) error {
	return p.send(ctx, chatID, message)
}

func (p *telegramProvider) GetUpdates(ctx context.Context, offset int64, timeout int) ([]ports.TelegramUpdate, error) {
	if p.token == "" {
		return nil, fmt.Errorf("Telegram bot token is not configured")
	}
	endpoint, err := url.Parse(p.baseURL + "/bot" + p.token + "/getUpdates")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("offset", strconv.FormatInt(offset, 10))
	query.Set("timeout", strconv.Itoa(timeout))
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Telegram API returned %s", response.Status)
	}
	var payload struct {
		OK     bool `json:"ok"`
		Result []struct {
			ID      int64 `json:"update_id"`
			Message struct {
				Text string `json:"text"`
				Chat struct {
					ID   int64  `json:"id"`
					Type string `json:"type"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if !payload.OK {
		return nil, fmt.Errorf("Telegram API returned an unsuccessful response")
	}
	updates := make([]ports.TelegramUpdate, 0, len(payload.Result))
	for _, item := range payload.Result {
		update := ports.TelegramUpdate{ID: item.ID, ChatID: strconv.FormatInt(item.Message.Chat.ID, 10), ChatType: item.Message.Chat.Type, Text: item.Message.Text}
		updates = append(updates, update)
	}
	return updates, nil
}

func (p *telegramProvider) destination(destination string) string {
	if strings.TrimSpace(destination) != "" {
		return destination
	}
	return p.chatID
}

func (p *telegramProvider) send(ctx context.Context, chatID, text string) error {
	return p.sendWithMarkup(ctx, chatID, text, "")
}

func (p *telegramProvider) sendWithMarkup(ctx context.Context, chatID, text, markup string) error {
	if p.token == "" {
		return fmt.Errorf("Telegram bot token is not configured")
	}
	payloadMap := map[string]any{"chat_id": chatID, "text": text}
	if markup != "" {
		var keyboard any
		if err := json.Unmarshal([]byte(markup), &keyboard); err != nil {
			return err
		}
		payloadMap["reply_markup"] = keyboard
	}
	payload, err := json.Marshal(payloadMap)
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
var _ ports.TelegramUpdateSource = (*telegramProvider)(nil)
var _ ports.TelegramPairingMessenger = (*telegramProvider)(nil)
