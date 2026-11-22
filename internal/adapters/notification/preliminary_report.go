package notification

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/task"
	"github.com/alex/assistent/internal/domain/teacher"
	"github.com/alex/assistent/internal/platform/config"
)

type preliminaryReportNotifier struct {
	client     smtpSender
	from       string
	recipients ports.TeacherRepository
	telegram   ports.ReportReadyTelegramSender
	profile    ports.ClassroomProfileSource
	mu         sync.Mutex
	sent       map[string]struct{}
}

func NewPreliminaryReportNotifier(cfg config.EmailConfig, recipients ports.TeacherRepository, telegram ports.ReportReadyTelegramSender) ports.PreliminaryReportNotifier {
	return NewPreliminaryReportNotifierWithProfile(cfg, recipients, telegram, nil)
}

func NewPreliminaryReportNotifierWithProfile(cfg config.EmailConfig, recipients ports.TeacherRepository, telegram ports.ReportReadyTelegramSender, profile ports.ClassroomProfileSource) ports.PreliminaryReportNotifier {
	if cfg.SMTPUsername == "" || cfg.SMTPPassword == "" {
		return noopPreliminaryReportNotifier{}
	}
	from := cfg.From
	if from == "" {
		from = cfg.SMTPUsername
	}
	return &preliminaryReportNotifier{
		client: newGoogleSMTPClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword),
		from:   from, recipients: recipients, telegram: telegram, profile: profile, sent: make(map[string]struct{}),
	}
}

func (n *preliminaryReportNotifier) Notify(ctx context.Context, item task.Task, missing []ports.MissingSubmitter) error {
	if n.recipients == nil {
		return nil
	}
	admin, err := n.findAdmin(ctx)
	if err != nil {
		return err
	}
	if admin.ID == 0 || admin.Email == "" {
		return nil
	}
	body := preliminaryMessage(item, missing)
	key := fmt.Sprintf("%d:%s", admin.ID, body)
	n.mu.Lock()
	if _, ok := n.sent[key]; ok {
		n.mu.Unlock()
		return nil
	}
	n.mu.Unlock()
	if err := n.client.Send(ctx, n.from, admin.Email, fmt.Sprintf("Submission status: %s", item.Title), body); err != nil {
		return fmt.Errorf("send preliminary summary email to %s with Google SMTP: %w", admin.Email, err)
	}
	if n.telegram != nil {
		destination, err := n.findTelegramDestination(ctx, admin.ID)
		if err != nil {
			return err
		}
		if err := n.telegram.SendReportReady(ctx, destination, body); err != nil {
			return fmt.Errorf("send preliminary summary Telegram message: %w", err)
		}
	}
	n.mu.Lock()
	if n.sent == nil {
		n.sent = make(map[string]struct{})
	}
	n.sent[key] = struct{}{}
	n.mu.Unlock()
	return nil
}

func (n *preliminaryReportNotifier) findTelegramDestination(ctx context.Context, recipientID int64) (string, error) {
	channels, err := n.recipients.ListActiveChannels(ctx, recipientID)
	if err != nil {
		return "", fmt.Errorf("list preliminary Telegram channels for %d: %w", recipientID, err)
	}
	for _, channel := range channels {
		if channel.Kind == "telegram" && strings.TrimSpace(channel.Destination) != "" {
			return channel.Destination, nil
		}
	}
	return "", nil
}

func (n *preliminaryReportNotifier) findAdmin(ctx context.Context) (teacher.Recipient, error) {
	var classroomAdmin string
	if n.profile != nil {
		profile, err := n.profile.GetProfile(ctx, "me")
		if err != nil {
			return teacher.Recipient{}, fmt.Errorf("resolve preliminary report administrator: %w", err)
		}
		classroomAdmin = profile.ClassroomID
	}
	people, err := n.recipients.List(ctx)
	if err != nil {
		return teacher.Recipient{}, fmt.Errorf("list preliminary report recipients: %w", err)
	}
	for _, person := range people {
		if person.Status != teacher.Active {
			continue
		}
		if classroomAdmin != "" {
			if person.ClassroomID == classroomAdmin {
				return person, nil
			}
			continue
		}
		channels, err := n.recipients.ListActiveChannels(ctx, person.ID)
		if err != nil {
			return teacher.Recipient{}, fmt.Errorf("list preliminary report channels for %d: %w", person.ID, err)
		}
		for _, channel := range channels {
			if channel.Kind == "telegram" && strings.TrimSpace(channel.Destination) != "" {
				return person, nil
			}
		}
	}
	return teacher.Recipient{}, nil
}

func preliminaryMessage(item task.Task, missing []ports.MissingSubmitter) string {
	if len(missing) == 0 {
		return fmt.Sprintf("Everybody has already sent the files for %s.", item.Title)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "Users who have not sent their files for %s:\n\n", item.Title)
	for _, person := range missing {
		fmt.Fprintf(&builder, "- %s <%s>\n", person.Name, person.Email)
	}
	return builder.String()
}

type noopPreliminaryReportNotifier struct{}

func (noopPreliminaryReportNotifier) Notify(context.Context, task.Task, []ports.MissingSubmitter) error {
	return nil
}
