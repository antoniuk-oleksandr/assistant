package notification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	"github.com/alex/assistent/internal/domain/teacher"
	"github.com/alex/assistent/internal/platform/config"
)

const googleSheetLinkTag = "<google_sheet_link>"

type ReportReadyNotifier struct {
	client     smtpSender
	from       string
	template   string
	recipients ports.TeacherRepository
	telegram   ports.ReportReadyTelegramSender
}

func NewReportReadyNotifier(cfg config.EmailConfig, recipients ports.TeacherRepository) ports.ReportReadyNotifier {
	return newReportReadyNotifier(cfg, recipients, nil)
}

func NewReportReadyNotifierWithTelegram(cfg config.EmailConfig, recipients ports.TeacherRepository, telegram ports.ReportReadyTelegramSender) ports.ReportReadyNotifier {
	return newReportReadyNotifier(cfg, recipients, telegram)
}

func newReportReadyNotifier(cfg config.EmailConfig, recipients ports.TeacherRepository, telegram ports.ReportReadyTelegramSender) ports.ReportReadyNotifier {
	if cfg.SMTPUsername == "" || cfg.SMTPPassword == "" {
		return noopReportReadyNotifier{}
	}
	from := cfg.From
	if from == "" {
		from = cfg.SMTPUsername
	}
	return &ReportReadyNotifier{client: newGoogleSMTPClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword), from: from, template: resolveTemplatePath(cfg.TemplatePath), recipients: recipients, telegram: telegram}
}

func resolveTemplatePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	directory, err := os.Getwd()
	if err != nil {
		return path
	}
	for {
		candidate := filepath.Join(directory, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return path
		}
		directory = parent
	}
}

func (n *ReportReadyNotifier) Enabled() bool { return true }

func (n *ReportReadyNotifier) Notify(ctx context.Context, _ report.Result, link string) error {
	body, err := n.renderMessage(link)
	if err != nil {
		return err
	}
	if n.recipients == nil {
		return nil
	}
	people, err := n.recipients.List(ctx)
	if err != nil {
		return fmt.Errorf("list report recipients: %w", err)
	}
	for _, person := range people {
		if person.Status != teacher.Active {
			continue
		}
		if person.Email == "" {
			continue
		}
		if err := n.client.Send(ctx, n.from, person.Email, "Scientific activity report is ready", body); err != nil {
			return fmt.Errorf("send report email to %s with Google SMTP: %w", person.Email, err)
		}
	}
	if n.telegram != nil {
		for _, person := range people {
			if person.Status != teacher.Active {
				continue
			}
			channels, err := n.recipients.ListActiveChannels(ctx, person.ID)
			if err != nil {
				return fmt.Errorf("list report channels for %d: %w", person.ID, err)
			}
			for _, channel := range channels {
				if channel.Kind != "phone" || channel.Destination == "" {
					continue
				}
				if err := n.telegram.SendReportReady(ctx, "", body); err != nil {
					return fmt.Errorf("send report Telegram message: %w", err)
				}
				return nil
			}
		}
	}
	return nil
}

func (n *ReportReadyNotifier) renderMessage(link string) (string, error) {
	body, err := os.ReadFile(n.template)
	if err != nil {
		return "", fmt.Errorf("read report email template: %w", err)
	}
	message := string(body)
	if !strings.Contains(message, googleSheetLinkTag) {
		return "", errors.New("report email template must contain <google_sheet_link>")
	}
	return strings.ReplaceAll(message, googleSheetLinkTag, link), nil
}

type noopReportReadyNotifier struct{}

func (noopReportReadyNotifier) Enabled() bool                                       { return false }
func (noopReportReadyNotifier) Notify(context.Context, report.Result, string) error { return nil }
