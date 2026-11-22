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
	profile    ports.ClassroomProfileSource
}

func NewReportReadyNotifier(cfg config.EmailConfig, recipients ports.TeacherRepository) ports.ReportReadyNotifier {
	return newReportReadyNotifier(cfg, recipients, nil, nil)
}

func NewReportReadyNotifierWithTelegram(cfg config.EmailConfig, recipients ports.TeacherRepository, telegram ports.ReportReadyTelegramSender) ports.ReportReadyNotifier {
	return newReportReadyNotifier(cfg, recipients, telegram, nil)
}

func NewReportReadyNotifierWithTelegramAndProfile(cfg config.EmailConfig, recipients ports.TeacherRepository, telegram ports.ReportReadyTelegramSender, profile ports.ClassroomProfileSource) ports.ReportReadyNotifier {
	return newReportReadyNotifier(cfg, recipients, telegram, profile)
}

func newReportReadyNotifier(cfg config.EmailConfig, recipients ports.TeacherRepository, telegram ports.ReportReadyTelegramSender, profile ports.ClassroomProfileSource) ports.ReportReadyNotifier {
	if cfg.SMTPUsername == "" || cfg.SMTPPassword == "" {
		return noopReportReadyNotifier{}
	}
	from := cfg.From
	if from == "" {
		from = cfg.SMTPUsername
	}
	return &ReportReadyNotifier{client: newGoogleSMTPClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword), from: from, template: resolveTemplatePath(cfg.TemplatePath), recipients: recipients, telegram: telegram, profile: profile}
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
	admin, err := n.findAdministrator(ctx, people)
	if err != nil {
		return err
	}
	if admin.ID == 0 {
		return nil
	}
	if admin.Email != "" {
		if err := n.client.Send(ctx, n.from, admin.Email, "Scientific activity report is ready", body); err != nil {
			return fmt.Errorf("send report email to %s with Google SMTP: %w", admin.Email, err)
		}
	}
	if n.telegram != nil {
		channels, err := n.recipients.ListActiveChannels(ctx, admin.ID)
		if err != nil {
			return fmt.Errorf("list report channels for %d: %w", admin.ID, err)
		}
		for _, channel := range channels {
			if channel.Kind == "telegram" && channel.Destination != "" {
				if err := n.telegram.SendReportReady(ctx, channel.Destination, body); err != nil {
					return fmt.Errorf("send report Telegram message: %w", err)
				}
				break
			}
		}
	}
	return nil
}

func (n *ReportReadyNotifier) findAdministrator(ctx context.Context, people []teacher.Recipient) (teacher.Recipient, error) {
	classroomID := ""
	if n.profile != nil {
		profile, err := n.profile.GetProfile(ctx, "me")
		if err != nil {
			return teacher.Recipient{}, fmt.Errorf("resolve report administrator: %w", err)
		}
		classroomID = profile.ClassroomID
	}
	for _, person := range people {
		if person.Status != teacher.Active {
			continue
		}
		if classroomID == "" || person.ClassroomID == classroomID {
			return person, nil
		}
	}
	return teacher.Recipient{}, nil
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
