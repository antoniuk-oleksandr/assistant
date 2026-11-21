package notification

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/platform/config"
)

type reminderEmailProvider struct {
	client resendClient
	from   string
}

// NewReminderEmailProvider adapts Resend to the domain reminder port. An empty
// configuration returns a disabled provider so local polling remains usable.
func NewReminderEmailProvider(cfg config.EmailConfig) ports.ReminderProvider {
	if cfg.APIKey == "" || cfg.From == "" {
		return noopReminderProvider{}
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.resend.com"
	}
	return &reminderEmailProvider{client: &resendHTTPClient{baseURL: strings.TrimRight(baseURL, "/"), apiKey: cfg.APIKey, client: http.DefaultClient}, from: cfg.From}
}

func (p *reminderEmailProvider) Name() string { return "email" }

func (p *reminderEmailProvider) SendReminder(ctx context.Context, item notification.Reminder) error {
	return p.client.Send(ctx, p.from, item.Destination, item.Subject, item.Message)
}

type noopReminderProvider struct{}

func (noopReminderProvider) Name() string { return "email" }
func (noopReminderProvider) SendReminder(context.Context, notification.Reminder) error {
	return errors.New("Resend reminder email is not configured")
}
