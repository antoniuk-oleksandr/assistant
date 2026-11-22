package notification

import (
	"context"
	"errors"

	"github.com/alex/assistent/internal/domain/notification"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/platform/config"
)

type reminderEmailProvider struct {
	client smtpSender
	from   string
}

// NewReminderEmailProvider adapts Google SMTP to the domain reminder port. An empty
// configuration returns a disabled provider so local polling remains usable.
func NewReminderEmailProvider(cfg config.EmailConfig) ports.ReminderProvider {
	if cfg.SMTPUsername == "" || cfg.SMTPPassword == "" {
		return noopReminderProvider{}
	}
	from := cfg.From
	if from == "" {
		from = cfg.SMTPUsername
	}
	return &reminderEmailProvider{client: newGoogleSMTPClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword), from: from}
}

func (p *reminderEmailProvider) Name() string { return "email" }

func (p *reminderEmailProvider) SendReminder(ctx context.Context, item notification.Reminder) error {
	return p.client.Send(ctx, p.from, item.Destination, item.Subject, item.Message)
}

type noopReminderProvider struct{}

func (noopReminderProvider) Name() string { return "email" }
func (noopReminderProvider) SendReminder(context.Context, notification.Reminder) error {
	return errors.New("Google SMTP reminder email is not configured")
}
